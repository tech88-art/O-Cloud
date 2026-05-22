/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
)

func TestEffectiveProxyImagePerCRWins(t *testing.T) {
	defer setDefaultProxyImage("global-default:v1")()

	ms := &inferencev1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{Name: "qwen-pd"},
		Spec: inferencev1alpha1.ModelServiceSpec{
			PDPair: inferencev1alpha1.PDPairSpec{
				ProxyImage: "per-cr-override:v2",
			},
		},
	}
	if got := EffectiveProxyImage(ms); got != "per-cr-override:v2" {
		t.Fatalf("EffectiveProxyImage = %q, want per-cr-override:v2(per-CR field wins over global default)", got)
	}
}

func TestEffectiveProxyImageFallsBackToDefault(t *testing.T) {
	defer setDefaultProxyImage("global-default:v1")()

	ms := &inferencev1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{Name: "qwen-pd"},
		Spec: inferencev1alpha1.ModelServiceSpec{
			PDPair: inferencev1alpha1.PDPairSpec{},
		},
	}
	if got := EffectiveProxyImage(ms); got != "global-default:v1" {
		t.Fatalf("EffectiveProxyImage = %q, want global-default:v1(fallback)", got)
	}
}

func TestEffectiveProxyImageEmptyReturnsEmpty(t *testing.T) {
	defer setDefaultProxyImage("")()

	ms := &inferencev1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{Name: "qwen-pd"},
		Spec: inferencev1alpha1.ModelServiceSpec{
			PDPair: inferencev1alpha1.PDPairSpec{},
		},
	}
	if got := EffectiveProxyImage(ms); got != "" {
		t.Fatalf("EffectiveProxyImage = %q, want \"\"(Phase 7-8 no-sidecar behavior preserved)", got)
	}
}

func TestEffectiveProxyImageNilMS(t *testing.T) {
	defer setDefaultProxyImage("global-default:v1")()

	if got := EffectiveProxyImage(nil); got != "global-default:v1" {
		t.Fatalf("EffectiveProxyImage(nil) = %q, want global-default:v1(defensive fallback)", got)
	}
}

// setDefaultProxyImage installs a test-only DefaultProxyImage value and
// returns a cleanup function for test isolation.
func setDefaultProxyImage(v string) func() {
	prev := DefaultProxyImage
	DefaultProxyImage = v
	return func() { DefaultProxyImage = prev }
}

// TestDefaultProxyImageEnvInjection mirrors the cmd/main.go P11-T-008
// startup hook(`if v := os.Getenv("DEFAULT_PROXY_IMAGE"); v != "" {
// controller.DefaultProxyImage = v }`)to prove the env-injection path
// reaches EffectiveProxyImage without a per-CR override. ADR-0017 §2
// Decision D 6th 优先级 acceptance("integration test: env var
// injection path · setenv → main.go startup hook → controller.
// DefaultProxyImage updated · EffectiveProxyImage helper returns chart-
// injected value").
func TestDefaultProxyImageEnvInjection(t *testing.T) {
	// Save + restore env so parallel tests stay isolated.
	prevEnv, prevSet := os.LookupEnv("DEFAULT_PROXY_IMAGE")
	t.Cleanup(func() {
		if prevSet {
			_ = os.Setenv("DEFAULT_PROXY_IMAGE", prevEnv)
		} else {
			_ = os.Unsetenv("DEFAULT_PROXY_IMAGE")
		}
	})
	defer setDefaultProxyImage("")()

	_ = os.Setenv("DEFAULT_PROXY_IMAGE", "chart-injected:v3")

	// Simulate cmd/main.go startup hook(per main.go P11-T-008 section).
	if v := os.Getenv("DEFAULT_PROXY_IMAGE"); v != "" {
		DefaultProxyImage = v
	}

	ms := &inferencev1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{Name: "qwen-pd"},
		Spec: inferencev1alpha1.ModelServiceSpec{
			PDPair: inferencev1alpha1.PDPairSpec{}, // no per-CR override
		},
	}
	if got := EffectiveProxyImage(ms); got != "chart-injected:v3" {
		t.Fatalf("EffectiveProxyImage after env-inject = %q, want chart-injected:v3(P11-T-008 wire path)", got)
	}
}
