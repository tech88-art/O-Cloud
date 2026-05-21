/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tech88-art/O-Cloud/operators/o2-dms-adapter/internal/types"
)

// caseTable lists all 7 ADR-0013 §4 catalog endpoints. Each case asserts
// stub-routing: handler reachable + responds 501 + Content-Type JSON +
// ErrorEnvelope body decoded successfully. P9-T-104 body landing will
// flip these expectations from 501 → 200/201/etc per endpoint semantics.
var caseTable = []struct {
	name     string
	method   string
	path     string
	endpoint string
}{
	{"CreateDeploymentItem", http.MethodPost, "/o2dms/v1/deploymentItems", "POST /o2dms/v1/deploymentItems"},
	{"ListDeploymentItems", http.MethodGet, "/o2dms/v1/deploymentItems", "GET /o2dms/v1/deploymentItems"},
	{"GetDeploymentItem", http.MethodGet, "/o2dms/v1/deploymentItems/some-id", "GET /o2dms/v1/deploymentItems/{id}"},
	{"DeleteDeploymentItem", http.MethodDelete, "/o2dms/v1/deploymentItems/some-id", "DELETE /o2dms/v1/deploymentItems/{id}"},
	{"GetInventory", http.MethodGet, "/o2dms/v1/inventory", "GET /o2dms/v1/inventory"},
	{"ListDeploymentManagers", http.MethodGet, "/o2dms/v1/deploymentManagers", "GET /o2dms/v1/deploymentManagers"},
	{"GetLifecycleOperation", http.MethodGet, "/o2dms/v1/lifecycleOperations/op-id-1", "GET /o2dms/v1/lifecycleOperations/{id}"},
}

// TestStubRoutingAll7 covers P9-T-008 acceptance · 7 stub-routing cases:
// each handler returns 501 with proper Content-Type + JSON envelope ·
// proves routing wired through chi router from cmd/main.go to the
// handler method.
func TestStubRoutingAll7(t *testing.T) {
	h := NewHandler()
	router := NewRouter(h)
	for _, tc := range caseTable {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("status = %d, want %d (501)", rec.Code, http.StatusNotImplemented)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json prefix", ct)
			}
			var env types.ErrorEnvelope
			if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
				t.Fatalf("decode envelope: %v · body=%q", err, rec.Body.String())
			}
			if env.Code != http.StatusNotImplemented {
				t.Errorf("envelope.code = %d, want 501", env.Code)
			}
			if !strings.Contains(env.Details, tc.endpoint) {
				t.Errorf("envelope.details = %q, want substring %q", env.Details, tc.endpoint)
			}
			if !strings.Contains(env.Details, "R003-v04.00") {
				t.Errorf("envelope.details = %q, want spec version substring R003-v04.00", env.Details)
			}
		})
	}
}

// TestRouter404OnUnknownPath covers defensive routing: unknown path under
// BasePath returns 404 (not the stub 501).
func TestRouter404OnUnknownPath(t *testing.T) {
	router := NewRouter(NewHandler())
	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/unknown-resource", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 on unknown path", rec.Code)
	}
}
