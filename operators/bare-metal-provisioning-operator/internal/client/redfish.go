/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

// Package client contains BMC client interfaces + Phase 11 stub
// implementations per P11-T-006 chart packaging acceptance. Real Redfish
// + IPMI SDK calls are Phase 12+; this stub mirrors the wire contract so
// the controller-runtime Reconciler + chart Pod boot wire end-to-end
// without a live BMC.
package client

import (
	"context"
	"fmt"
	"sync"
)

// PowerState mirrors Redfish PowerState enum + ipmitool power status
// output. Phase 11 stub returns deterministic state from in-memory map.
type PowerState string

const (
	PowerOn      PowerState = "On"
	PowerOff     PowerState = "Off"
	PowerUnknown PowerState = "Unknown"
)

// BMC is the abstraction the BareMetalNode Reconciler talks to. Both
// Redfish + IPMI implementations satisfy this interface. The controller
// picks impl via Spec.BMC.Type discrimination(per ADR-0003 v2 IMS-3
// schema).
type BMC interface {
	// PowerStatus returns the current power state of the bare-metal
	// node. Reconcile uses this to drive ProvisioningState transitions
	// (Inspecting → Provisioned → Ready when On + driver attached).
	PowerStatus(ctx context.Context) (PowerState, error)

	// PowerOn requests power-on. Returns once the BMC accepts the
	// request — actual hardware boot may take 30-90s and is observed
	// via subsequent PowerStatus polls.
	PowerOn(ctx context.Context) error

	// PowerOff requests graceful power-off. Reconciler uses this on
	// Deprovisioning transitions.
	PowerOff(ctx context.Context) error

	// Reset requests a power cycle (hard reset). Used on stuck states +
	// firmware update flows (Phase 12+).
	Reset(ctx context.Context) error
}

// RedfishConfig holds endpoint + credentials for a Redfish-capable BMC.
// Phase 11 stub uses only Endpoint to namespace its in-memory state;
// real Redfish HTTPS calls land Phase 12+.
type RedfishConfig struct {
	Endpoint string // e.g. https://192.0.2.10/redfish/v1
	Username string
	Password string
}

// RedfishStub is a Phase 11 in-memory BMC. Stores power state per
// Endpoint + returns deterministic responses for unit + chart smoke
// tests. Same fixture-driven approach P10-T-101 17 unit tests use.
type RedfishStub struct {
	cfg RedfishConfig

	mu    sync.RWMutex
	state PowerState
}

// NewRedfishStub returns a stub whose initial PowerState is Unknown
// until the first PowerOn/PowerOff call.
func NewRedfishStub(cfg RedfishConfig) *RedfishStub {
	return &RedfishStub{cfg: cfg, state: PowerUnknown}
}

// PowerStatus returns the in-memory state. Real Redfish: GET
// /redfish/v1/Systems/<id> · parse PowerState field.
func (s *RedfishStub) PowerStatus(_ context.Context) (PowerState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state, nil
}

// PowerOn transitions stub state. Real Redfish: POST
// /redfish/v1/Systems/<id>/Actions/ComputerSystem.Reset {ResetType: On}.
func (s *RedfishStub) PowerOn(_ context.Context) error {
	if s.cfg.Username == "" {
		return fmt.Errorf("redfish stub: username required at %s", s.cfg.Endpoint)
	}
	s.mu.Lock()
	s.state = PowerOn
	s.mu.Unlock()
	return nil
}

// PowerOff transitions stub state.
func (s *RedfishStub) PowerOff(_ context.Context) error {
	s.mu.Lock()
	s.state = PowerOff
	s.mu.Unlock()
	return nil
}

// Reset cycles the stub: Off → On in a single call.
func (s *RedfishStub) Reset(_ context.Context) error {
	s.mu.Lock()
	s.state = PowerOff
	s.state = PowerOn
	s.mu.Unlock()
	return nil
}

// Endpoint exposes the configured endpoint for ops logging.
func (s *RedfishStub) Endpoint() string {
	return s.cfg.Endpoint
}
