/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package client

import (
	"context"
	"fmt"
	"sync"
)

// IPMIConfig holds the IPMI-over-LAN connection parameters. Real impl
// shells out to `ipmitool -H host -p port -U user -P pass power status`
// or links the goipmi library (Phase 12+).
type IPMIConfig struct {
	Host     string // BMC IP / hostname
	Port     int    // typically 623
	Username string
	Password string
}

// IPMIStub is a Phase 11 in-memory BMC mirroring RedfishStub semantics.
type IPMIStub struct {
	cfg IPMIConfig

	mu    sync.RWMutex
	state PowerState
}

// NewIPMIStub returns a stub whose initial PowerState is Unknown.
func NewIPMIStub(cfg IPMIConfig) *IPMIStub {
	return &IPMIStub{cfg: cfg, state: PowerUnknown}
}

// PowerStatus mirrors `ipmitool power status`. Real impl parses
// "Chassis Power is on" / "off" lines.
func (s *IPMIStub) PowerStatus(_ context.Context) (PowerState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state, nil
}

// PowerOn mirrors `ipmitool power on`.
func (s *IPMIStub) PowerOn(_ context.Context) error {
	if s.cfg.Username == "" {
		return fmt.Errorf("ipmi stub: username required at %s:%d", s.cfg.Host, s.cfg.Port)
	}
	s.mu.Lock()
	s.state = PowerOn
	s.mu.Unlock()
	return nil
}

// PowerOff mirrors `ipmitool power soft`.
func (s *IPMIStub) PowerOff(_ context.Context) error {
	s.mu.Lock()
	s.state = PowerOff
	s.mu.Unlock()
	return nil
}

// Reset mirrors `ipmitool power cycle`.
func (s *IPMIStub) Reset(_ context.Context) error {
	s.mu.Lock()
	s.state = PowerOff
	s.state = PowerOn
	s.mu.Unlock()
	return nil
}

// Host exposes the configured host:port for ops logging.
func (s *IPMIStub) Host() string {
	return fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
}
