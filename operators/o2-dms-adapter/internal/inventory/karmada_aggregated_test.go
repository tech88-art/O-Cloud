/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package inventory

import (
	"errors"
	"os"
	"testing"
)

func TestNewAggregatedRestConfig_DisabledReturnsSentinel(t *testing.T) {
	_, err := NewAggregatedRestConfig(KarmadaAggregatedConfig{Enabled: false})
	if !errors.Is(err, ErrAggregatedAPIDisabled) {
		t.Fatalf("want ErrAggregatedAPIDisabled, got %v", err)
	}
}

func TestNewAggregatedRestConfig_MissingFileReturnsErr(t *testing.T) {
	_, err := NewAggregatedRestConfig(KarmadaAggregatedConfig{
		Enabled:        true,
		KubeconfigPath: "/nonexistent/path/karmada.conf",
	})
	if err == nil {
		t.Fatal("want error for missing file, got nil")
	}
	if errors.Is(err, ErrAggregatedAPIDisabled) {
		t.Fatalf("want file-stat error, got disabled sentinel: %v", err)
	}
}

func TestFromEnv_EmptyDisabled(t *testing.T) {
	saveAndRestoreEnv(t, "O2DMS_KARMADA_AGGREGATED_ENABLED")
	saveAndRestoreEnv(t, "O2DMS_KARMADA_KUBECONFIG")
	_ = os.Unsetenv("O2DMS_KARMADA_AGGREGATED_ENABLED")
	_ = os.Unsetenv("O2DMS_KARMADA_KUBECONFIG")

	cfg := FromEnv()
	if cfg.Enabled {
		t.Error("want Enabled=false when env vars unset")
	}
	if cfg.KubeconfigPath != "" {
		t.Errorf("want empty KubeconfigPath, got %q", cfg.KubeconfigPath)
	}
}

func TestFromEnv_EnabledTrue(t *testing.T) {
	saveAndRestoreEnv(t, "O2DMS_KARMADA_AGGREGATED_ENABLED")
	saveAndRestoreEnv(t, "O2DMS_KARMADA_KUBECONFIG")
	_ = os.Setenv("O2DMS_KARMADA_AGGREGATED_ENABLED", "true")
	_ = os.Setenv("O2DMS_KARMADA_KUBECONFIG", "/custom/karmada.conf")

	cfg := FromEnv()
	if !cfg.Enabled {
		t.Error("want Enabled=true when env=true")
	}
	if cfg.KubeconfigPath != "/custom/karmada.conf" {
		t.Errorf("want /custom/karmada.conf, got %q", cfg.KubeconfigPath)
	}
}

func TestFromEnv_EnabledOne(t *testing.T) {
	saveAndRestoreEnv(t, "O2DMS_KARMADA_AGGREGATED_ENABLED")
	_ = os.Setenv("O2DMS_KARMADA_AGGREGATED_ENABLED", "1")
	cfg := FromEnv()
	if !cfg.Enabled {
		t.Error(`want Enabled=true when env="1"`)
	}
}

func TestFromEnv_EnabledFalse(t *testing.T) {
	saveAndRestoreEnv(t, "O2DMS_KARMADA_AGGREGATED_ENABLED")
	_ = os.Setenv("O2DMS_KARMADA_AGGREGATED_ENABLED", "false")
	cfg := FromEnv()
	if cfg.Enabled {
		t.Error(`want Enabled=false when env="false"`)
	}
}

func saveAndRestoreEnv(t *testing.T, key string) {
	t.Helper()
	prev, set := os.LookupEnv(key)
	t.Cleanup(func() {
		if set {
			_ = os.Setenv(key, prev)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
