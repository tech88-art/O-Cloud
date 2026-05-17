// Package k8s will hold the K8s-apiserver-backed datasource.Source
// implementation, slated for Phase 2.
//
// PHASE-1 scope (P1-T-005): this file exists solely so client-go is referenced
// in go.mod / go.sum at the agreed pinned version. Once T-Phase-2 lands the
// real implementation, this placeholder is replaced by the actual NewSource
// constructor and informers wiring.
package k8s

import (
	// Underscore import keeps the dependency pinned. Replace with a normal
	// import when the implementation lands.
	_ "k8s.io/client-go/kubernetes"
)
