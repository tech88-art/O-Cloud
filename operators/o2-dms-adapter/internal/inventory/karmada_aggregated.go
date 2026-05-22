/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

// karmada_aggregated.go is the cross-cluster aggregated inventory helper
// per ADR-0018 §2 Decision D(lifted-informer pattern via Karmada
// karmada-aggregated-apiserver).
//
// Phase 11 P11-T-103: this helper builds the client config pointing at
// the karmada-aggregated-apiserver kubeconfig instead of any single
// member cluster. List() / Watch() calls automatically fan out across
// joined member clusters per Karmada's standard lifted-informer
// semantics.
//
// O2 DMS Adapter uses this for the `GET /o2dms/v1/deploymentManagers`
// + `GET /o2dms/v1/deploymentItems` endpoints to surface a unified
// cross-cluster view(per ADR-0013 §6 forward note + ADR-0018 §2
// Decision D O2 DMS Adapter implementation contract).
package inventory

import (
	"errors"
	"fmt"
	"os"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KarmadaAggregatedConfig describes how to reach the Karmada
// karmada-aggregated-apiserver. Phase 11 ships file-based kubeconfig
// (deploy/karmada/install.sh writes /tmp/karmada-apiserver.conf);
// Phase 12+ may add in-cluster service-account path when O2 DMS
// Adapter runs inside the host cluster with proper RBAC.
type KarmadaAggregatedConfig struct {
	// KubeconfigPath is the path to the Karmada apiserver kubeconfig
	// file. Defaults to /tmp/karmada-apiserver.conf(install.sh write
	// location)when empty.
	KubeconfigPath string

	// Enabled toggles the aggregated-apiserver path. When false the
	// O2 DMS Adapter falls back to single-cluster mode(legacy Phase
	// 10 behavior · host cluster only).
	Enabled bool
}

// DefaultKubeconfigPath is the conventional location install.sh writes.
const DefaultKubeconfigPath = "/tmp/karmada-apiserver.conf"

// ErrAggregatedAPIDisabled is returned when callers ask for an
// aggregated client but the config has Enabled=false.
var ErrAggregatedAPIDisabled = errors.New("Karmada aggregated-apiserver disabled in config")

// NewAggregatedRestConfig builds a rest.Config pointing at the
// Karmada karmada-aggregated-apiserver. Returns ErrAggregatedAPIDisabled
// when Enabled=false. Returns wrap-err when the kubeconfig file is
// missing or unparseable.
func NewAggregatedRestConfig(cfg KarmadaAggregatedConfig) (*rest.Config, error) {
	if !cfg.Enabled {
		return nil, ErrAggregatedAPIDisabled
	}
	path := cfg.KubeconfigPath
	if path == "" {
		path = DefaultKubeconfigPath
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("karmada kubeconfig %q: %w", path, err)
	}
	rc, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, fmt.Errorf("build rest.Config from karmada kubeconfig: %w", err)
	}
	// Karmada's aggregated apiserver enforces its own audit + ratelimit
	// stack · keep the client lean(no proxy injection).
	return rc, nil
}

// FromEnv builds a KarmadaAggregatedConfig from O2 DMS Adapter env vars:
//
//	O2DMS_KARMADA_AGGREGATED_ENABLED  true | false(default false)
//	O2DMS_KARMADA_KUBECONFIG          path to karmada kubeconfig file
//	                                  (default /tmp/karmada-apiserver.conf)
//
// Empty `O2DMS_KARMADA_AGGREGATED_ENABLED` keeps Phase 10 single-cluster
// behavior. Operators flip to "true" after install.sh + apply policies.
func FromEnv() KarmadaAggregatedConfig {
	v := os.Getenv("O2DMS_KARMADA_AGGREGATED_ENABLED")
	enabled := v == "true" || v == "1"
	return KarmadaAggregatedConfig{
		KubeconfigPath: os.Getenv("O2DMS_KARMADA_KUBECONFIG"),
		Enabled:        enabled,
	}
}
