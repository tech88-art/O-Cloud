// PID-level workload correlation source (T102).
//
// CgroupSource derives pod identity for active NPU-using processes by
// reading /proc/[pid]/cgroup and matching the kubepods slice convention.
// Phase 3 ships the simulator fake-fs path only; the real-/proc reader
// returns ErrSourceNotAvailable until inference-operator wires the
// `--enable-workload-correlation` flag in Phase 5+ (ADR-0008).

package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// WorkloadSample is one PID-bound observation: a container's per-NPU
// utilization slice for the current scrape interval.
type WorkloadSample struct {
	Namespace string
	Pod       string
	Container string
	// NPUSecondsDelta is the wall-clock seconds during which this
	// container had AT LEAST ONE slice running on the labeled NPU during
	// the current scrape cycle. Phase 3 simulator emits 1.0; Phase 5+
	// real-/proc reader accumulates true wall-clock.
	NPUSecondsDelta float64
	// NPUID identifies the NPU this delta is attributable to.
	NPUID string
	// ActiveSlices is the number of slices this container currently has
	// bound to ANY NPU at sample time.
	ActiveSlices int
}

// ErrCgroupFsMissing is returned by NewCgroupSource when the simulator
// fake-fs root is provided but does not contain a valid identity.json.
// The Phase 4+ real-/proc reader returns ErrSourceNotAvailable
// (declared in sources.go) instead when invoked in Phase 3.
var ErrCgroupFsMissing = errors.New("cgroup simulator fs path does not exist")

// CgroupSource reads /proc/[pid]/cgroup (or a simulator fake-fs rooted
// at SimRoot) to derive pod identity for active NPU-using processes.
//
// Phase 3 scope: simulator fs only. The real-/proc reader path is
// reserved for Phase 4+ when inference-operator consumes these series
// for PD Router routing decisions.
type CgroupSource struct {
	// SimRoot is the simulator fake-fs root. When set, ReadWorkloads
	// walks SimRoot/proc/<pid>/cgroup and resolves identity from
	// SimRoot/identity.json. When empty, ReadWorkloads returns
	// ErrSourceNotAvailable (real-/proc reader is Phase 4+).
	SimRoot string

	mu       sync.Mutex
	identity map[string]podIdentity // keyed by podUID
}

type podIdentity struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
}

// NewCgroupSource constructs a CgroupSource. simRoot is the path to the
// simulator fake-fs; empty = real-/proc path (Phase 4+ stub).
//
// Returns ErrCgroupFsMissing if simRoot is non-empty but the directory
// or identity.json file is absent.
func NewCgroupSource(simRoot string) (*CgroupSource, error) {
	s := &CgroupSource{SimRoot: simRoot}
	if simRoot != "" {
		if err := s.loadIdentity(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *CgroupSource) loadIdentity() error {
	path := filepath.Join(s.SimRoot, "identity.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrCgroupFsMissing, path)
		}
		return fmt.Errorf("read identity file %q: %w", path, err)
	}
	var m map[string]podIdentity
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parse identity file %q: %w", path, err)
	}
	s.mu.Lock()
	s.identity = m
	s.mu.Unlock()
	return nil
}

// ReadWorkloads walks proc/<pid>/cgroup under SimRoot and derives one
// WorkloadSample per pid whose podUID resolves via identity.json.
//
// Returns ErrSourceNotAvailable when SimRoot is empty (Phase 4+
// real-/proc reader path is not implemented in Phase 3). Per-pid read
// failures (missing cgroup file, unparseable line, podUID not in
// identity map) are silently skipped so a single bad pid does not
// poison the whole scrape.
func (s *CgroupSource) ReadWorkloads(ctx context.Context) ([]WorkloadSample, error) {
	if s.SimRoot == "" {
		return nil, ErrSourceNotAvailable
	}
	procDir := filepath.Join(s.SimRoot, "proc")
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, fmt.Errorf("read proc dir %q: %w", procDir, err)
	}

	out := make([]WorkloadSample, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid := e.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue // not a numeric pid dir
		}
		cgPath := filepath.Join(procDir, pid, "cgroup")
		cg, err := os.ReadFile(cgPath)
		if err != nil {
			continue // best-effort per pid
		}
		podUID, container, ok := parseCgroupLine(string(cg))
		if !ok {
			continue
		}
		s.mu.Lock()
		ident, hit := s.identity[podUID]
		s.mu.Unlock()
		if !hit {
			continue
		}
		// Phase 3: NPUSecondsDelta + ActiveSlices are synthesized so the
		// metric families have shape parity with the Phase 5+ real-data
		// path. Real wall-clock accumulation lands when inference-operator
		// wires the workload-correlation flag.
		// Container label prefers the friendly name from identity.json
		// (Phase 5+ will source this from kubelet pod-status). The
		// parsed scope-name hash is only used if identity didn't pin
		// a name — useful for sidecars whose CRI scope is observable
		// but whose human label is not yet wired.
		out = append(out, WorkloadSample{
			Namespace:       ident.Namespace,
			Pod:             ident.Pod,
			Container:       firstNonEmpty(ident.Container, container),
			NPUSecondsDelta: 1.0,
			NPUID:           defaultNPUForPod(ident.Pod),
			ActiveSlices:    1,
		})
	}
	return out, nil
}

// cgroupPodRe matches the kubepods pod-slice marker. Real podUIDs are
// RFC 4122 UUIDs (hex + dash), but the testdata uses semantic suffixes
// like "3a4b5c6d-prefill" for readability; Phase 5+ K8s drops the
// strict UUID at the runtime layer too. We accept any non-dot,
// non-slash payload up to the ".slice" terminator.
//
// The leading `[-/]` boundary disambiguates "pod<UID>.slice" from the
// false-positive "kubepod**s**.slice" prefix (Go regex has no
// lookbehind). The boundary is dropped from the capture group.
var (
	cgroupPodRe       = regexp.MustCompile(`[-/]pod([^./]+)\.slice`)
	cgroupContainerRe = regexp.MustCompile(`(?:cri-containerd|crio|docker)-([0-9a-zA-Z]+)\.scope`)
)

// parseCgroupLine extracts (podUID, containerID) from a cgroup v2 line.
//
// cgroup v2 emits one "0::<path>" entry per /proc/<pid>/cgroup; the
// kubepods slice path encodes pod identity as a "pod<UID>.slice" segment
// and container identity as a "<runtime>-<containerID>.scope" leaf. The
// runtime prefix varies across CRI implementations:
//
//	cri-containerd-<id>.scope (containerd)
//	crio-<id>.scope           (CRI-O)
//	docker-<id>.scope         (dockershim, deprecated but still seen)
//
// All three resolve to the .scope basename. Returns ok=false when the
// line is not a cgroup v2 unified entry or does not contain a kubepods
// pod slice marker.
func parseCgroupLine(s string) (podUID, containerID string, ok bool) {
	line := strings.TrimSpace(s)
	if !strings.HasPrefix(line, "0::") {
		return "", "", false
	}
	path := strings.TrimPrefix(line, "0::")

	pm := cgroupPodRe.FindStringSubmatch(path)
	if len(pm) < 2 {
		return "", "", false
	}
	cm := cgroupContainerRe.FindStringSubmatch(path)
	var cID string
	if len(cm) >= 2 {
		cID = cm[1]
	}
	return pm[1], cID, true
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// defaultNPUForPod is a Phase-3 placeholder that returns a canonical
// NPU id label for any pod. Phase 5+ will read real NPU binding
// annotations (set by ResourceClaim status + inference-operator).
func defaultNPUForPod(pod string) string {
	return "worker-site-a-01-npu-0"
}
