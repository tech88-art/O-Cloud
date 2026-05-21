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
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tech88-art/O-Cloud/operators/o2-dms-adapter/internal/api"
)

func main() {
	var addr string
	var shutdownTimeout time.Duration

	flag.StringVar(&addr, "addr", ":8088", "HTTP server bind address (per ADR-0013 §2 Decision B default).")
	flag.DurationVar(&shutdownTimeout, "shutdown-timeout", 10*time.Second, "Graceful shutdown timeout.")
	flag.Parse()

	handler := api.NewHandler()
	router := api.NewRouter(handler)

	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
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

	log.Printf("o2-dms-adapter: listening on %s · ADR-0013 P9-T-008 scaffold · 7 stub handlers under %s", addr, api.BasePath)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("o2-dms-adapter: ListenAndServe: %v", err)
	}
	<-idleClosed
	log.Printf("o2-dms-adapter: shutdown complete")
}
