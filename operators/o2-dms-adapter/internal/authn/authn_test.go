/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package authn

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestOIDCValidatorEmptyIssuerFailsClosed(t *testing.T) {
	v := &OIDCValidator{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer any")
	if err := v.Validate(req); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("OIDC empty IssuerURL must fail-safe: %v", err)
	}
}

func TestK8sTokenReviewValidatorEmptyClientFailsClosed(t *testing.T) {
	v := &K8sTokenReviewValidator{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer any")
	if err := v.Validate(req); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("TokenReview nil client must fail-safe: %v", err)
	}
}

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
