/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

// Package authn implements OIDC + K8s ServiceAccount + TokenReview-based
// authentication for the O2 DMS Adapter north-bound API per ADR-0013 §6
// forward note (Phase 10 P10-T-103 polish wave 1).
//
// Phase 9 P9-T-008 + P9-T-104 ship the Adapter with Bearer-token env-var
// placeholder (development-only · operator pre-shares static token via env).
// Phase 10 polish adds:
//   1. OIDC client (token issuer verification via JWKS endpoint)
//   2. K8s ServiceAccount token validation (TokenReview API)
//   3. Per-request authn middleware
//
// Phase 10 W1 minimum viable: substrate + middleware shape · Phase 11+
// chart packaging wires real OIDC provider config + Secret with JWKS cache
// + ServiceAccount RBAC.
package authn

import (
	"errors"
	"net/http"
	"strings"
)

// Validator is the auth strategy interface. Phase 10 W1 ships 3
// implementations:
//   - PlaceholderBearerValidator (Phase 9 carry · static token from env)
//   - OIDCValidator (Phase 10 polish wave 1 · JWKS endpoint verification)
//   - K8sTokenReviewValidator (Phase 10 polish wave 1 · TokenReview API)
//
// Phase 11+ adds: federation provider integration (Keycloak / Dex /
// AD/LDAP / Google Workspace).
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
// (operator pre-shares · development-only). Phase 9 carry · Phase 11+
// chart packaging may keep this for env-driven local-dev mode while
// preferring OIDC / TokenReview in production.
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

// OIDCValidator validates JWT tokens against an OIDC issuer's JWKS
// endpoint per ADR-0013 §6 forward note. Phase 10 W1 ships substrate
// only · IssuerURL + JWKS fetcher wiring at Phase 11+ chart packaging.
type OIDCValidator struct {
	// IssuerURL is the OIDC issuer (used to fetch JWKS via
	// /.well-known/openid-configuration discovery).
	IssuerURL string

	// Audience is the expected audience claim ("aud" in JWT).
	Audience string

	// AllowedClaims is the set of additional claims expected to match
	// (e.g. {"groups": "o2-admin"}).
	AllowedClaims map[string]string
}

// Name returns the validator identity.
func (v *OIDCValidator) Name() string { return "oidc" }

// Validate verifies the JWT signature + claims. Phase 10 W1 returns
// ErrInvalidToken when not configured (IssuerURL empty) · real JWKS
// lookup + signature verification at Phase 11+ wiring.
func (v *OIDCValidator) Validate(r *http.Request) error {
	if v.IssuerURL == "" {
		return ErrInvalidToken
	}
	_, err := ExtractBearerToken(r)
	if err != nil {
		return err
	}
	// Phase 10 W1 placeholder · Phase 11+ wires:
	//   - fetch JWKS via IssuerURL + http GET
	//   - parse JWT · verify signature against JWKS
	//   - validate iss / aud / exp / iat claims
	//   - apply AllowedClaims allowlist
	// Until Phase 11+ wiring lands, OIDCValidator returns ErrInvalidToken
	// (fail-safe · denies all when issuer not properly configured).
	return ErrInvalidToken
}

// K8sTokenReviewValidator validates K8s ServiceAccount tokens via the
// `authentication.k8s.io/v1.TokenReview` API. Phase 10 W1 substrate ·
// real TokenReview RBAC + client.Create call at Phase 11+ wiring.
type K8sTokenReviewValidator struct {
	// Audiences is the expected aud claim for ServiceAccount tokens
	// (default `https://kubernetes.default.svc.cluster.local`).
	Audiences []string

	// ReviewClient is the K8s client used to issue TokenReview requests.
	// Phase 10 W1 keeps untyped (interface{}) to avoid bringing client-go
	// dependency until Phase 11+ chart packaging.
	ReviewClient interface{}
}

// Name returns the validator identity.
func (v *K8sTokenReviewValidator) Name() string { return "k8s-tokenreview" }

// Validate sends the bearer token to TokenReview API. Phase 10 W1 returns
// ErrInvalidToken when ReviewClient is nil (substrate-only state).
func (v *K8sTokenReviewValidator) Validate(r *http.Request) error {
	if v.ReviewClient == nil {
		return ErrInvalidToken
	}
	_, err := ExtractBearerToken(r)
	if err != nil {
		return err
	}
	// Phase 10 W1 placeholder · Phase 11+ wires:
	//   - construct TokenReview spec.token = bearer + spec.audiences
	//   - reviewClient.AuthenticationV1().TokenReviews().Create(ctx, tr, opts)
	//   - response.Status.Authenticated == true → success
	//   - else ErrInvalidToken
	return ErrInvalidToken
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
