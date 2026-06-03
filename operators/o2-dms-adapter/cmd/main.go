/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Command o2-dms-adapter is the HTTP server binary for the O-RAN O2 DMS
// Adapter per ADR-0013. Phase 9 P9-T-008 scaffold binds 0.0.0.0:8088 +
// wires the chi router with 7 stub handlers (501 Not Implemented) under
// /o2dms/v1; P9-T-104 body landing fills inventory + lifecycle logic.
//
// Phase 13 P13-T-201 (ADR-0025 §2 Decision A) wires real authentication:
// every request to the NB API now passes through authn.Middleware. The
// validator is config-selected — K8s TokenReview (in-cluster primary) or
// OIDC (external IdP) — with the static PlaceholderBearer reserved for
// explicit local-dev (O2DMS_AUTH_MODE=local-dev) only.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/tech88-art/O-Cloud/operators/o2-dms-adapter/internal/api"
	"github.com/tech88-art/O-Cloud/operators/o2-dms-adapter/internal/authn"
	"github.com/tech88-art/O-Cloud/operators/o2-dms-adapter/internal/inventory"
)

func main() {
	var addr string
	var shutdownTimeout time.Duration

	flag.StringVar(&addr, "addr", ":8088", "HTTP server bind address (per ADR-0013 §2 Decision B default).")
	flag.DurationVar(&shutdownTimeout, "shutdown-timeout", 10*time.Second, "Graceful shutdown timeout.")
	flag.Parse()

	// Construct K8s clients · per ADR-0013 §2 Decision D dynamic + core
	// client wiring. In-cluster default; KUBECONFIG fallback for local dev.
	// Failure to construct → NoopClient (degraded · log warning) so binary
	// still serves /o2dms/v1/* (returns empty inventories) for testing.
	// The core clientset (when available) doubles as the TokenReview client
	// for authn (P13-T-201).
	cfg := buildK8sConfig(os.Getenv("KUBECONFIG"))
	inv, core := buildInventoryClient(cfg, os.Getenv("O2DMS_CLUSTER_ID"))

	handler := api.NewHandler(inv)
	router := api.NewRouter(handler)

	// P13-T-201: select + wire the authn validator (ADR-0025 §2 Decision A).
	validator := buildValidator(core)
	authedRouter := authn.Middleware(validator)(router)
	log.Printf("o2-dms-adapter: authn validator=%s wired on %s", validator.Name(), api.BasePath)

	srv := &http.Server{
		Addr:              addr,
		Handler:           authedRouter,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	idleClosed := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Printf("o2-dms-adapter: shutdown signal received, draining within %s", shutdownTimeout)
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("o2-dms-adapter: shutdown error: %v", err)
		}
		close(idleClosed)
	}()

	log.Printf("o2-dms-adapter: listening on %s · ADR-0013 · 7 handlers + authn under %s", addr, api.BasePath)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("o2-dms-adapter: ListenAndServe: %v", err)
	}
	<-idleClosed
	log.Printf("o2-dms-adapter: shutdown complete")
}

// buildK8sConfig resolves a *rest.Config from KUBECONFIG (path-on-disk) or
// the in-cluster ServiceAccount. Returns nil (not an error) when neither is
// available so callers can degrade gracefully.
func buildK8sConfig(kubeconfig string) *rest.Config {
	var cfg *rest.Config
	var err error
	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		log.Printf("o2-dms-adapter: K8s config not available (%v)", err)
		return nil
	}
	return cfg
}

// buildInventoryClient constructs a DynamicClient from the supplied config.
// On any failure (or nil config) returns NoopClient (degraded mode) so the
// binary still serves the NB API surface for testing. The second return is
// the core clientset (nil in degraded mode) reused as the TokenReview client.
func buildInventoryClient(cfg *rest.Config, clusterID string) (inventory.Client, kubernetes.Interface) {
	if cfg == nil {
		log.Printf("o2-dms-adapter: no K8s config · using NoopClient (degraded · inventory returns empty)")
		return inventory.NewNoopClient(), nil
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		log.Printf("o2-dms-adapter: dynamic client construction failed (%v) · NoopClient", err)
		return inventory.NewNoopClient(), nil
	}
	core, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Printf("o2-dms-adapter: core client construction failed (%v) · NoopClient", err)
		return inventory.NewNoopClient(), nil
	}
	log.Printf("o2-dms-adapter: K8s clients wired · cluster=%s", clusterID)
	return inventory.NewDynamicClient(dyn, core, clusterID), core
}

// buildValidator selects the authn validator per ADR-0025 §2 Decision A.
//
// Selection (env-driven · helm injects via values.auth):
//   - O2DMS_OIDC_ISSUER set      → OIDCValidator (external IdP path · JWKS+JWT)
//   - O2DMS_AUTH_MODE=local-dev  → PlaceholderBearerValidator (static
//     O2DMS_BEARER_TOKEN · dev only · NEVER the production default)
//   - otherwise (default)        → K8sTokenReviewValidator (in-cluster
//     ServiceAccount · primary path)
//
// The K8s TokenReview path requires the in-cluster core clientset; if it is
// nil (degraded · no K8s config) and no other mode is configured, we still
// return the TokenReview validator with a nil ReviewClient, which fails
// closed (ErrNotConfigured → 401). Auth never silently falls open.
func buildValidator(core kubernetes.Interface) authn.Validator {
	if issuer := os.Getenv("O2DMS_OIDC_ISSUER"); issuer != "" {
		v := &authn.OIDCValidator{
			IssuerURL:     issuer,
			Audience:      os.Getenv("O2DMS_OIDC_AUDIENCE"),
			AllowedClaims: parseAllowedClaims(os.Getenv("O2DMS_OIDC_ALLOWED_CLAIMS")),
		}
		log.Printf("o2-dms-adapter: authn mode=oidc issuer=%s audience=%s", issuer, v.Audience)
		return v
	}
	if strings.EqualFold(os.Getenv("O2DMS_AUTH_MODE"), "local-dev") {
		log.Printf("o2-dms-adapter: authn mode=local-dev (PlaceholderBearer · DEV ONLY · do not use in production)")
		return &authn.PlaceholderBearerValidator{ExpectedToken: os.Getenv("O2DMS_BEARER_TOKEN")}
	}
	audiences := splitNonEmpty(os.Getenv("O2DMS_TOKENREVIEW_AUDIENCES"), ",")
	log.Printf("o2-dms-adapter: authn mode=k8s-tokenreview audiences=%v", audiences)
	return &authn.K8sTokenReviewValidator{
		Audiences:    audiences,
		ReviewClient: core,
	}
}

// parseAllowedClaims parses a "key=value,key2=value2" string into the
// AllowedClaims map. Empty input → nil (no extra claim constraints).
func parseAllowedClaims(s string) map[string]string {
	pairs := splitNonEmpty(s, ",")
	if len(pairs) == 0 {
		return nil
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		if k, v, ok := strings.Cut(p, "="); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// splitNonEmpty splits s on sep and drops empty / whitespace-only fields.
func splitNonEmpty(s, sep string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := parts[:0]
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
