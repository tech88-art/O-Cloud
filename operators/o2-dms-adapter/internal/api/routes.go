/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package api

import (
	"net/http"

	chi "github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Compile-time ensure chi router import is used in routes.go (the
// IDE / `goimports` would otherwise drop it).
var _ = chi.NewRouter

// BasePath is the O2 IMS R1 NB base path per ADR-0013 §2 Decision B.
const BasePath = "/o2dms/v1"

// NewRouter wires the 7 ADR-0013 §4 catalog endpoints under BasePath.
// Returns an http.Handler ready to mount on the server (cmd/main.go).
//
// Middleware:
//   - chi/middleware.RequestID
//   - chi/middleware.Recoverer (panic recovery → 500 Internal Server Error)
//
// Phase 9 P9-T-008 scaffold: all 7 handlers return 501 Not Implemented
// per ADR-0013 §4 stub semantics. Phase 9 W2 P9-T-104 body fills in
// actual logic.
func NewRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Route(BasePath, func(r chi.Router) {
		r.Route("/deploymentItems", func(r chi.Router) {
			r.Post("/", h.CreateDeploymentItem)
			r.Get("/", h.ListDeploymentItems)
			r.Get("/{id}", h.GetDeploymentItem)
			r.Delete("/{id}", h.DeleteDeploymentItem)
		})
		r.Get("/inventory", h.GetInventory)
		r.Get("/deploymentManagers", h.ListDeploymentManagers)
		r.Get("/lifecycleOperations/{id}", h.GetLifecycleOperation)
	})

	return r
}
