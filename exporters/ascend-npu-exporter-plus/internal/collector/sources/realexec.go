package sources

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// realexec.go — os/exec wrapper for shelling out to `npu-smi`
// (P13-T-102 · ADR-0024 §2 Decision D + §4(c) no-cgo). Mirrors
// operators/npu-dra-driver/.../npusmi/exec.go (the T101 sibling): context
// timeout, missing-binary → ErrNoCommand, non-zero exit wrapped with the
// command line for diagnosis. Lives in the exporter module because the
// two modules do not cross-import (exporters/CLAUDE.md §3.1).

// defaultNPUSMIBinary is the PATH lookup name when no absolute path is
// configured. Lab deploys typically hostPath-mount the Ascend tools dir
// and set the absolute path "/usr/local/Ascend/driver/tools/npu-smi".
const defaultNPUSMIBinary = "npu-smi"

// defaultExecTimeout bounds a single npu-smi invocation. npu-smi can hang
// when a device is mid hot-reset; the timeout lets a scrape degrade
// gracefully (the device is skipped, dashboards keep moving) instead of
// stalling the /metrics handler.
const defaultExecTimeout = 5 * time.Second

// ErrNoCommand is returned when the npu-smi binary is not found on PATH or
// at the configured path. The most common cause is a node that picked the
// real source by accident (no Ascend driver installed) — callers surface
// it loudly rather than emitting fabricated samples.
var ErrNoCommand = errors.New("sources: npu-smi binary not found")

// npuSMIRunner shells out to npu-smi. The interface is the test seam:
// table-tests inject a canned runner so the real source bodies are
// exercised without a lab npu-smi binary.
type npuSMIRunner interface {
	// run executes `npu-smi <args...>` and returns stdout. A missing
	// binary maps to ErrNoCommand; a non-zero exit is wrapped.
	run(ctx context.Context, args ...string) (string, error)
}

// execRunner is the production npuSMIRunner backed by os/exec.
type execRunner struct {
	// binaryPath is the absolute path to npu-smi; empty → PATH lookup of
	// defaultNPUSMIBinary.
	binaryPath string
	// timeout bounds each invocation; zero → defaultExecTimeout.
	timeout time.Duration
}

func (e *execRunner) binary() string {
	if e.binaryPath != "" {
		return e.binaryPath
	}
	return defaultNPUSMIBinary
}

// run implements npuSMIRunner against the host npu-smi binary.
func (e *execRunner) run(ctx context.Context, args ...string) (string, error) {
	if _, err := exec.LookPath(e.binary()); err != nil {
		return "", ErrNoCommand
	}
	timeout := e.timeout
	if timeout <= 0 {
		timeout = defaultExecTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, e.binary(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrNoCommand
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("sources: %s %s: %w (%s)", e.binary(), strings.Join(args, " "), err, msg)
		}
		return "", fmt.Errorf("sources: %s %s: %w", e.binary(), strings.Join(args, " "), err)
	}
	return stdout.String(), nil
}
