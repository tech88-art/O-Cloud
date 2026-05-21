/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
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
