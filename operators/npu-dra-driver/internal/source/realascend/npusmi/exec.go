/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package npusmi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ExecClient implements Client by shelling out to the npu-smi binary
// on the host. Phase 7 W1 ships the type + minimal wrapper; Phase 7
// T101 lab body hardens it with context timeout, non-zero exit handling,
// stderr capture, and DCMI fallback.
//
// **W1 status**: compiles + unit-testable (BinaryPath defaults catch
// missing binary as ErrNoCommand). Phase 7 CI does NOT exercise this
// type (no npu-smi binary in CI runners — chart's sourceType=mock-json
// default avoids the path).
type ExecClient struct {
	// BinaryPath is the absolute path to npu-smi. Defaults to
	// "npu-smi" (looked up via PATH) when empty. Lab deploys typically
	// set "/usr/local/Ascend/driver/tools/npu-smi" per Ascend docs.
	BinaryPath string

	// NodeID is stamped on every TopoEntry's NodeID field. Caller
	// typically passes $NODE_NAME / kubelet downward API value.
	// Defaults to "" when empty (caller must verify).
	NodeID string
}

// Compile-time assertion: ExecClient satisfies Client.
var _ Client = &ExecClient{}

// binary returns the configured BinaryPath or "npu-smi" default for
// PATH lookup.
func (e *ExecClient) binary() string {
	if e.BinaryPath != "" {
		return e.BinaryPath
	}
	return "npu-smi"
}

// QueryTopo runs `npu-smi info -t topo` and pipes output through
// ParseTopoMatrix. Phase 7 W1 minimal wrapper; T101 lab body adds
// retry-on-ErrParse + stderr-capture-on-non-zero-exit.
func (e *ExecClient) QueryTopo(ctx context.Context) ([]TopoEntry, error) {
	cmd := exec.CommandContext(ctx, e.binary(), "info", "-t", "topo")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, ErrNoCommand
		}
		return nil, fmt.Errorf("npusmi ExecClient: %s info -t topo: %w", e.binary(), err)
	}
	entries, err := ParseTopoMatrix(stdout.String())
	if err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].NodeID = e.NodeID
	}
	return entries, nil
}

// QueryDeviceInfo runs `npu-smi info -t board -i <devID>` and parses the
// board view (chip name, AI-core count, memory, NUMA, driver/firmware
// versions) via ParseBoardInfo (P13-T-101 · ADR-0024 §2 Decision B). The
// board view is a flat "Key : Value" list; lab driver-version variants in
// key spelling extend ParseBoardInfo's alias sets (ADR-0024 §3 fallback).
func (e *ExecClient) QueryDeviceInfo(ctx context.Context, devID int) (*DeviceInfo, error) {
	out, err := e.run(ctx, "info", "-t", "board", "-i", strconv.Itoa(devID))
	if err != nil {
		return nil, err
	}
	return ParseBoardInfo(out, devID)
}

// QueryHealth runs the cheap `npu-smi info -t health -i <devID>` view and
// parses the health state via ParseHealth (P13-T-101). Callers (Source.
// Watch) treat any error as "health query unsupported on this driver" and
// degrade to timer-tick reconcile (ADR-0024 §2 Decision B fallback).
func (e *ExecClient) QueryHealth(ctx context.Context, devID int) (HealthState, error) {
	out, err := e.run(ctx, "info", "-t", "health", "-i", strconv.Itoa(devID))
	if err != nil {
		return HealthUnknown, err
	}
	return ParseHealth(out)
}

// run executes npu-smi with the given args, capturing stdout. It maps a
// missing binary to ErrNoCommand so callers can distinguish "no driver
// install" from "command failed", and wraps other failures with the
// command line for diagnosis.
func (e *ExecClient) run(ctx context.Context, args ...string) (string, error) {
	if _, err := exec.LookPath(e.binary()); err != nil {
		return "", ErrNoCommand
	}
	cmd := exec.CommandContext(ctx, e.binary(), args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrNoCommand
		}
		return "", fmt.Errorf("npusmi ExecClient: %s %s: %w", e.binary(), strings.Join(args, " "), err)
	}
	return stdout.String(), nil
}
