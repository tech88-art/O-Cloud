/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

// Package authn implements OIDC + K8s ServiceAccount + TokenReview-based
// authentication for the O2 DMS Adapter north-bound API per ADR-0013 §6
// forward note and ADR-0025 §2 Decision A (Phase 13 P13-T-201 production
// hardening · authz model).
//
// Phase 9 P9-T-008 + P9-T-104 shipped the Adapter with a Bearer-token
// env-var placeholder (development-only · operator pre-shares static token
// via env). Phase 10 P10-T-103 shipped the substrate (Validator interface +
// Middleware + three impl shells). Phase 13 P13-T-201 fills the real bodies:
//  1. OIDCValidator      — OIDC discovery → JWKS fetch → JWT signature +
//     iss/aud/exp/iat verification + AllowedClaims
//     allowlist (via github.com/coreos/go-oidc/v3).
//  2. K8sTokenReviewValidator — in-cluster ServiceAccount token validation
//     via authentication.k8s.io/v1.TokenReview API
//     (typed client-go kubernetes.Interface).
//  3. Middleware         — per-request authn wrapper (unchanged from P10).
//
// authz model (ADR-0025 §2 Decision A): K8sTokenReviewValidator is the
// in-cluster primary path (集群内组件互信免外部 IdP); OIDCValidator is the
// external IdP path (Dex 参考 IdP · 甲方 swap point). PlaceholderBearer
// remains available for explicit local-dev only (cmd/main.go no longer
// selects it by default).
package authn

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	authnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Validator is the auth strategy interface. Phase 13 P13-T-201 ships three
// real implementations:
//   - K8sTokenReviewValidator (in-cluster ServiceAccount · TokenReview API · primary)
//   - OIDCValidator (external IdP · JWKS endpoint + JWT verification)
//   - PlaceholderBearerValidator (static token from env · local-dev only)
type Validator interface {
	// Name identifies the validator for log + metric correlation.
	Name() string

	// Validate inspects the request Authorization header and returns nil
	// on success or a non-nil error describing the failure mode.
	Validate(r *http.Request) error
}

// ErrMissingAuthHeader indicates the request lacks an Authorization header.
var ErrMissingAuthHeader = errors.New("authn: missing Authorization header")

// ErrInvalidAuthScheme indicates the Authorization header value doesn't
// start with the expected scheme (e.g. "Bearer ").
var ErrInvalidAuthScheme = errors.New("authn: invalid Authorization scheme · want Bearer")

// ErrInvalidToken indicates the token is well-formed but rejected by the
// validator (expired · revoked · audience mismatch · signature invalid).
var ErrInvalidToken = errors.New("authn: invalid or expired token")

// ErrUnauthorized indicates the request did not pass authentication.
// Surfaces in HTTP 401 response per RFC 7235.
var ErrUnauthorized = errors.New("authn: unauthorized")

// ErrNotConfigured indicates a validator was constructed without the
// configuration it needs to operate (e.g. OIDCValidator with empty
// IssuerURL · K8sTokenReviewValidator with nil ReviewClient). Distinct from
// ErrInvalidToken: a misconfigured validator is an operator error, not a
// caller-token rejection. Both still fail closed (deny) at the middleware.
var ErrNotConfigured = errors.New("authn: validator not configured")

// ExtractBearerToken strips the "Bearer " prefix from an Authorization
// header value and returns the bare token, or empty + error.
func ExtractBearerToken(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", ErrMissingAuthHeader
	}
	if !strings.HasPrefix(h, "Bearer ") {
		return "", ErrInvalidAuthScheme
	}
	return strings.TrimPrefix(h, "Bearer "), nil
}

// PlaceholderBearerValidator validates a fixed token from configuration
// (operator pre-shares · development-only). Phase 9 carry · retained for
// env-driven local-dev mode only (ADR-0025 §2 Decision A). Production paths
// use OIDCValidator or K8sTokenReviewValidator.
type PlaceholderBearerValidator struct {
	ExpectedToken string
}

// Name returns the validator identity for log + metric correlation.
func (v *PlaceholderBearerValidator) Name() string { return "placeholder-bearer" }

// Validate compares the bearer token to the expected static value.
func (v *PlaceholderBearerValidator) Validate(r *http.Request) error {
	token, err := ExtractBearerToken(r)
	if err != nil {
		return err
	}
	if v.ExpectedToken == "" {
		return ErrInvalidToken
	}
	if token != v.ExpectedToken {
		return ErrInvalidToken
	}
	return nil
}

// tokenVerifier is the subset of *oidc.IDTokenVerifier that OIDCValidator
// uses. Factoring it as an interface lets tests inject a verifier built
// against a mock JWKS server without standing up an OIDC discovery
// endpoint (the *oidc.IDTokenVerifier returned by go-oidc satisfies it).
type tokenVerifier interface {
	Verify(ctx context.Context, rawIDToken string) (*oidc.IDToken, error)
}

// OIDCValidator validates JWT bearer tokens against an OIDC issuer per
// ADR-0025 §2 Decision A (external IdP path). On first use it performs OIDC
// discovery against IssuerURL (`/.well-known/openid-configuration`), which
// yields the JWKS URI; go-oidc's RemoteKeySet fetches + caches the signing
// keys and refreshes them on rotation. Each Validate then verifies the JWT
// signature, issuer, audience and expiry, and applies the AllowedClaims
// allowlist.
//
// The verifier (and its underlying JWKS key set) is built lazily and reused
// across requests — building it per-request would re-fetch JWKS every time.
type OIDCValidator struct {
	// IssuerURL is the OIDC issuer (used for discovery →
	// /.well-known/openid-configuration → jwks_uri).
	IssuerURL string

	// Audience is the expected audience claim ("aud" in the JWT). Empty
	// audience is rejected as a misconfiguration (fail closed) rather than
	// silently skipping the aud check.
	Audience string

	// AllowedClaims is the set of additional string claims that must be
	// present and equal in the token (e.g. {"groups": "o2-admin"}). Empty
	// → no extra claim constraints beyond iss/aud/exp.
	AllowedClaims map[string]string

	// HTTPClient overrides the *http.Client used for OIDC discovery + JWKS
	// fetches (tests point this at an httptest server). Nil → a 10s-timeout
	// default. Ignored when verifier is injected directly.
	HTTPClient *http.Client

	// verifier, when non-nil, is used directly and discovery is skipped.
	// Production leaves this nil and lets initVerifier build it from
	// IssuerURL. Tests inject a verifier wired to a mock JWKS key set.
	verifier tokenVerifier

	once    sync.Once
	initErr error
}

// Name returns the validator identity.
func (v *OIDCValidator) Name() string { return "oidc" }

// SetVerifier injects a pre-built verifier (test seam). Once set, Validate
// skips OIDC discovery. Safe to call before the first Validate.
func (v *OIDCValidator) SetVerifier(tv tokenVerifier) {
	v.verifier = tv
	// Mark once as done so initVerifier doesn't overwrite the injected one.
	v.once.Do(func() {})
}

// initVerifier performs OIDC discovery against IssuerURL and builds the
// JWT verifier once. Subsequent calls are no-ops (sync.Once).
func (v *OIDCValidator) initVerifier() error {
	v.once.Do(func() {
		if v.verifier != nil {
			return
		}
		if v.IssuerURL == "" {
			v.initErr = fmt.Errorf("%w: OIDCValidator.IssuerURL is empty", ErrNotConfigured)
			return
		}
		if v.Audience == "" {
			v.initErr = fmt.Errorf("%w: OIDCValidator.Audience is empty", ErrNotConfigured)
			return
		}
		httpClient := v.HTTPClient
		if httpClient == nil {
			httpClient = &http.Client{Timeout: 10 * time.Second}
		}
		ctx := oidc.ClientContext(context.Background(), httpClient)
		provider, err := oidc.NewProvider(ctx, v.IssuerURL)
		if err != nil {
			v.initErr = fmt.Errorf("authn: OIDC discovery for %q: %w", v.IssuerURL, err)
			return
		}
		v.verifier = provider.Verifier(&oidc.Config{ClientID: v.Audience})
	})
	return v.initErr
}

// Validate verifies the JWT signature + iss/aud/exp claims via the OIDC
// verifier, then enforces the AllowedClaims allowlist. Discovery failures
// surface as wrapped errors (operator misconfiguration); token rejections
// surface as ErrInvalidToken.
func (v *OIDCValidator) Validate(r *http.Request) error {
	token, err := ExtractBearerToken(r)
	if err != nil {
		return err
	}
	if err := v.initVerifier(); err != nil {
		return err
	}
	idToken, err := v.verifier.Verify(r.Context(), token)
	if err != nil {
		// Signature / issuer / audience / expiry failure → token rejected.
		return fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if len(v.AllowedClaims) > 0 {
		var claims map[string]any
		if err := idToken.Claims(&claims); err != nil {
			return fmt.Errorf("%w: decode claims: %v", ErrInvalidToken, err)
		}
		for k, want := range v.AllowedClaims {
			got, ok := claims[k]
			if !ok {
				return fmt.Errorf("%w: missing required claim %q", ErrInvalidToken, k)
			}
			if !claimMatches(got, want) {
				return fmt.Errorf("%w: claim %q mismatch", ErrInvalidToken, k)
			}
		}
	}
	return nil
}

// claimMatches reports whether a decoded JWT claim value equals the wanted
// string. Handles scalar string claims plus array-of-string claims (e.g.
// "groups": ["o2-admin","viewer"] matches want="o2-admin" if it's a member).
func claimMatches(got any, want string) bool {
	switch val := got.(type) {
	case string:
		return val == want
	case []any:
		for _, item := range val {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// TokenReviewClient is the narrow slice of kubernetes.Interface that
// K8sTokenReviewValidator needs: the ability to create a TokenReview. The
// real client-go clientset and the fake clientset both satisfy it via
// kubernetes.Interface, so production wiring passes a *kubernetes.Clientset
// and tests pass fake.NewSimpleClientset().
type TokenReviewClient = kubernetes.Interface

// K8sTokenReviewValidator validates K8s ServiceAccount tokens via the
// authentication.k8s.io/v1.TokenReview API per ADR-0025 §2 Decision A
// (in-cluster primary path). The bearer token is forwarded to the API
// server, which authenticates it and reports back whether it is valid +
// the authenticated user; we also assert the requested audiences are
// honoured when configured.
type K8sTokenReviewValidator struct {
	// Audiences is the expected aud claim for ServiceAccount tokens. When
	// set, the TokenReview request carries these audiences and the response
	// must echo at least one of them as authenticated. Empty → API server
	// default audience (the API server's own identifier).
	Audiences []string

	// ReviewClient is the typed K8s client used to issue TokenReview
	// requests (ADR-0025 §2 Decision A · Phase 13 promotes this from the
	// Phase 10 interface{} placeholder to kubernetes.Interface now that
	// client-go is in scope). Nil → not configured (fail closed).
	ReviewClient TokenReviewClient
}

// Name returns the validator identity.
func (v *K8sTokenReviewValidator) Name() string { return "k8s-tokenreview" }

// Validate sends the bearer token to the TokenReview API and returns nil
// only when the API server reports Status.Authenticated == true (and, when
// Audiences is configured, the response audiences intersect the request).
func (v *K8sTokenReviewValidator) Validate(r *http.Request) error {
	token, err := ExtractBearerToken(r)
	if err != nil {
		return err
	}
	if v.ReviewClient == nil {
		return fmt.Errorf("%w: K8sTokenReviewValidator.ReviewClient is nil", ErrNotConfigured)
	}

	tr := &authnv1.TokenReview{
		ObjectMeta: metav1.ObjectMeta{},
		Spec: authnv1.TokenReviewSpec{
			Token:     token,
			Audiences: v.Audiences,
		},
	}
	result, err := v.ReviewClient.AuthenticationV1().TokenReviews().Create(r.Context(), tr, metav1.CreateOptions{})
	if err != nil {
		// API-side failure (RBAC denied · API server unreachable). This is
		// an infrastructure error, not a token rejection — surface it so the
		// middleware fails closed but logs distinguishably.
		return fmt.Errorf("authn: TokenReview create: %w", err)
	}
	if !result.Status.Authenticated {
		if result.Status.Error != "" {
			return fmt.Errorf("%w: %s", ErrInvalidToken, result.Status.Error)
		}
		return ErrInvalidToken
	}
	// When the caller configured expected audiences, require the API server
	// to have validated the token against at least one of them. Without this
	// a token minted for a different audience could pass.
	if len(v.Audiences) > 0 && !audiencesIntersect(v.Audiences, result.Status.Audiences) {
		return fmt.Errorf("%w: token audiences %v do not satisfy expected %v",
			ErrInvalidToken, result.Status.Audiences, v.Audiences)
	}
	return nil
}

// audiencesIntersect reports whether want and got share at least one entry.
func audiencesIntersect(want, got []string) bool {
	set := make(map[string]struct{}, len(got))
	for _, a := range got {
		set[a] = struct{}{}
	}
	for _, a := range want {
		if _, ok := set[a]; ok {
			return true
		}
	}
	return false
}

// Middleware returns an http.Handler middleware that wraps the next
// handler with authentication. Returns 401 + WWW-Authenticate: Bearer on
// validation failure · proceeds to next on success.
func Middleware(v Validator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := v.Validate(r); err != nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="o2-dms-adapter"`)
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(err.Error() + "\n"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
