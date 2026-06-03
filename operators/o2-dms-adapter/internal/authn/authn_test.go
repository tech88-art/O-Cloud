/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package authn

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
	authnv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

// --- ExtractBearerToken + PlaceholderBearer (Phase 9/10 carry) ---

func TestExtractBearerTokenMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := ExtractBearerToken(req); !errors.Is(err, ErrMissingAuthHeader) {
		t.Fatalf("err = %v, want ErrMissingAuthHeader", err)
	}
}

func TestExtractBearerTokenInvalidScheme(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic foo")
	if _, err := ExtractBearerToken(req); !errors.Is(err, ErrInvalidAuthScheme) {
		t.Fatalf("err = %v, want ErrInvalidAuthScheme", err)
	}
}

func TestExtractBearerTokenValid(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer abc123")
	got, err := ExtractBearerToken(req)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "abc123" {
		t.Fatalf("token = %q, want abc123", got)
	}
}

func TestPlaceholderBearerValidatorMatchingToken(t *testing.T) {
	v := &PlaceholderBearerValidator{ExpectedToken: "secret-token"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	if err := v.Validate(req); err != nil {
		t.Fatalf("Validate matching token: %v", err)
	}
}

func TestPlaceholderBearerValidatorMismatch(t *testing.T) {
	v := &PlaceholderBearerValidator{ExpectedToken: "secret-token"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	if err := v.Validate(req); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestPlaceholderBearerValidatorEmptyExpected(t *testing.T) {
	v := &PlaceholderBearerValidator{ExpectedToken: ""}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer any")
	if err := v.Validate(req); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken (empty Expected fails closed)", err)
	}
}

// --- OIDCValidator: JWKS mock server + RS256-signed JWT ---

const (
	testIssuer   = "https://oidc.test.local"
	testAudience = "o2-dms-adapter"
	testKeyID    = "test-key-1"
)

// jwksServer signs JWTs with a fresh RSA key and serves the matching public
// JWKS over HTTP. Mirrors what go-oidc's RemoteKeySet expects.
type jwksServer struct {
	priv   *rsa.PrivateKey
	signer jose.Signer
	srv    *httptest.Server
}

func newJWKSServer(t *testing.T) *jwksServer {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: priv},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", testKeyID),
	)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	j := &jwksServer{priv: priv, signer: signer}

	jwks := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key:       priv.Public(),
		KeyID:     testKeyID,
		Algorithm: "RS256",
		Use:       "sig",
	}}}
	j.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(j.srv.Close)
	return j
}

// sign mints an RS256 JWT carrying the standard registered claims plus any
// extra string claims (e.g. groups).
func (j *jwksServer) sign(t *testing.T, claims josejwt.Claims, extra map[string]any) string {
	t.Helper()
	builder := josejwt.Signed(j.signer).Claims(claims)
	if extra != nil {
		builder = builder.Claims(extra)
	}
	tok, err := builder.Serialize()
	if err != nil {
		t.Fatalf("serialize JWT: %v", err)
	}
	return tok
}

// validator builds an OIDCValidator whose verifier is wired to this mock
// JWKS server (skipping OIDC discovery via SetVerifier).
func (j *jwksServer) validator(audience string, allowed map[string]string) *OIDCValidator {
	keySet := oidc.NewRemoteKeySet(context.Background(), j.srv.URL)
	verifier := oidc.NewVerifier(testIssuer, keySet, &oidc.Config{ClientID: audience})
	v := &OIDCValidator{IssuerURL: testIssuer, Audience: audience, AllowedClaims: allowed}
	v.SetVerifier(verifier)
	return v
}

func reqWithToken(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// State 1: valid token passes.
func TestOIDCValidator_Valid(t *testing.T) {
	j := newJWKSServer(t)
	v := j.validator(testAudience, nil)
	now := time.Now()
	tok := j.sign(t, josejwt.Claims{
		Issuer:    testIssuer,
		Subject:   "system:serviceaccount:ocloud-system:o2-client",
		Audience:  josejwt.Audience{testAudience},
		Expiry:    josejwt.NewNumericDate(now.Add(1 * time.Hour)),
		IssuedAt:  josejwt.NewNumericDate(now),
		NotBefore: josejwt.NewNumericDate(now.Add(-1 * time.Minute)),
	}, nil)

	if err := v.Validate(reqWithToken(tok)); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
}

// State 2: expired token is rejected as ErrInvalidToken.
func TestOIDCValidator_Expired(t *testing.T) {
	j := newJWKSServer(t)
	v := j.validator(testAudience, nil)
	past := time.Now().Add(-2 * time.Hour)
	tok := j.sign(t, josejwt.Claims{
		Issuer:   testIssuer,
		Subject:  "system:serviceaccount:ocloud-system:o2-client",
		Audience: josejwt.Audience{testAudience},
		Expiry:   josejwt.NewNumericDate(past.Add(1 * time.Hour)), // expired 1h ago
		IssuedAt: josejwt.NewNumericDate(past),
	}, nil)

	err := v.Validate(reqWithToken(tok))
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token err = %v, want ErrInvalidToken", err)
	}
}

// State 3: wrong-audience token is rejected as ErrInvalidToken.
func TestOIDCValidator_WrongAudience(t *testing.T) {
	j := newJWKSServer(t)
	v := j.validator(testAudience, nil)
	now := time.Now()
	tok := j.sign(t, josejwt.Claims{
		Issuer:   testIssuer,
		Subject:  "system:serviceaccount:ocloud-system:o2-client",
		Audience: josejwt.Audience{"some-other-audience"},
		Expiry:   josejwt.NewNumericDate(now.Add(1 * time.Hour)),
		IssuedAt: josejwt.NewNumericDate(now),
	}, nil)

	err := v.Validate(reqWithToken(tok))
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong-aud token err = %v, want ErrInvalidToken", err)
	}
}

// AllowedClaims allowlist: token missing the required group is rejected,
// token carrying it passes.
func TestOIDCValidator_AllowedClaims(t *testing.T) {
	j := newJWKSServer(t)
	v := j.validator(testAudience, map[string]string{"groups": "o2-admin"})
	now := time.Now()
	base := josejwt.Claims{
		Issuer:   testIssuer,
		Subject:  "system:serviceaccount:ocloud-system:o2-client",
		Audience: josejwt.Audience{testAudience},
		Expiry:   josejwt.NewNumericDate(now.Add(1 * time.Hour)),
		IssuedAt: josejwt.NewNumericDate(now),
	}

	// Missing groups → reject.
	if err := v.Validate(reqWithToken(j.sign(t, base, nil))); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("missing claim err = %v, want ErrInvalidToken", err)
	}
	// groups array containing o2-admin → pass.
	tok := j.sign(t, base, map[string]any{"groups": []string{"viewer", "o2-admin"}})
	if err := v.Validate(reqWithToken(tok)); err != nil {
		t.Fatalf("matching claim rejected: %v", err)
	}
}

// Empty IssuerURL fails closed with ErrNotConfigured (operator misconfig).
func TestOIDCValidator_EmptyIssuerNotConfigured(t *testing.T) {
	v := &OIDCValidator{}
	if err := v.Validate(reqWithToken("any.jwt.token")); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("empty IssuerURL err = %v, want ErrNotConfigured", err)
	}
}

// Tampered signature (signed by a different key) is rejected.
func TestOIDCValidator_BadSignature(t *testing.T) {
	good := newJWKSServer(t)
	rogue := newJWKSServer(t) // different RSA key, same kid
	v := good.validator(testAudience, nil)
	now := time.Now()
	tok := rogue.sign(t, josejwt.Claims{
		Issuer:   testIssuer,
		Audience: josejwt.Audience{testAudience},
		Expiry:   josejwt.NewNumericDate(now.Add(1 * time.Hour)),
		IssuedAt: josejwt.NewNumericDate(now),
	}, nil)
	if err := v.Validate(reqWithToken(tok)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("bad-signature token err = %v, want ErrInvalidToken", err)
	}
}

// --- K8sTokenReviewValidator: fake clientset + TokenReview reactor ---

// tokenReviewReactor builds a fake clientset whose TokenReviews().Create
// returns the supplied status (echoing back the requested audiences when
// authenticated, to mirror API-server behaviour).
func tokenReviewClientset(authenticated bool, errMsg string, echoAudiences bool) *fake.Clientset {
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("create", "tokenreviews", func(action clienttesting.Action) (bool, runtime.Object, error) {
		create := action.(clienttesting.CreateAction)
		in := create.GetObject().(*authnv1.TokenReview)
		out := in.DeepCopy()
		out.Status.Authenticated = authenticated
		out.Status.Error = errMsg
		if authenticated {
			out.Status.User = authnv1.UserInfo{Username: "system:serviceaccount:ocloud-system:o2-client"}
			if echoAudiences {
				out.Status.Audiences = in.Spec.Audiences
			}
		}
		return true, out, nil
	})
	return cs
}

// State 1: authenticated token passes.
func TestK8sTokenReview_Valid(t *testing.T) {
	v := &K8sTokenReviewValidator{
		Audiences:    []string{"https://kubernetes.default.svc"},
		ReviewClient: tokenReviewClientset(true, "", true),
	}
	if err := v.Validate(reqWithToken("sa.jwt.token")); err != nil {
		t.Fatalf("authenticated token rejected: %v", err)
	}
}

// State 2: not-authenticated (expired/invalid per API server) → ErrInvalidToken.
func TestK8sTokenReview_NotAuthenticated(t *testing.T) {
	v := &K8sTokenReviewValidator{
		Audiences:    []string{"https://kubernetes.default.svc"},
		ReviewClient: tokenReviewClientset(false, "token has expired", false),
	}
	err := v.Validate(reqWithToken("sa.jwt.token"))
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("unauthenticated err = %v, want ErrInvalidToken", err)
	}
}

// State 3: authenticated but wrong audience (API server didn't echo the
// requested audience) → ErrInvalidToken.
func TestK8sTokenReview_WrongAudience(t *testing.T) {
	v := &K8sTokenReviewValidator{
		Audiences:    []string{"o2-dms-adapter"},
		ReviewClient: tokenReviewClientset(true, "", false), // authenticated, but no audiences echoed
	}
	err := v.Validate(reqWithToken("sa.jwt.token"))
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong-aud err = %v, want ErrInvalidToken", err)
	}
}

// Nil ReviewClient fails closed with ErrNotConfigured.
func TestK8sTokenReview_NilClientNotConfigured(t *testing.T) {
	v := &K8sTokenReviewValidator{}
	if err := v.Validate(reqWithToken("any")); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil client err = %v, want ErrNotConfigured", err)
	}
}

// No-audience configuration skips the audience intersection check.
func TestK8sTokenReview_NoAudienceConfigured(t *testing.T) {
	v := &K8sTokenReviewValidator{
		ReviewClient: tokenReviewClientset(true, "", false),
	}
	if err := v.Validate(reqWithToken("sa.jwt.token")); err != nil {
		t.Fatalf("no-audience config rejected valid token: %v", err)
	}
}

// --- Middleware (Phase 9/10 carry) ---

func TestMiddlewareRejectsInvalid(t *testing.T) {
	v := &PlaceholderBearerValidator{ExpectedToken: "secret"}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("middleware should NOT call next on auth failure")
	})
	mw := Middleware(v)(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("response code = %d, want 401", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("response missing WWW-Authenticate header")
	}
}

func TestMiddlewarePassesValid(t *testing.T) {
	v := &PlaceholderBearerValidator{ExpectedToken: "secret"}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	mw := Middleware(v)(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if !called {
		t.Fatal("middleware did not call next on valid auth")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("response code = %d, want 204 (from next handler)", rec.Code)
	}
}
