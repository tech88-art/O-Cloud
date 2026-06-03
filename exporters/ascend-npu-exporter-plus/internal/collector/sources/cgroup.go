// PID-level workload correlation source (T102 · real-/proc body
// P13-T-102 · ADR-0024 §2 Decision D).
//
// CgroupSource derives pod identity for active NPU-using processes by
// reading /proc/[pid]/cgroup and matching the kubepods slice convention.
// Two read paths share one parser (parseCgroupLine):
//
//   - simulator fake-fs (SimRoot set): walks SimRoot/proc/<pid>/cgroup,
//     resolves friendly identity from SimRoot/identity.json. demo profile
//     + unit tests use this (kept exactly as Phase 3 shipped).
//   - real /proc (SimRoot empty): walks ProcRoot/<pid>, keeps a pid only
//     when it holds an open /dev/davinci* handle (an NPU-using process),
//     and resolves pod identity (namespace / podUID / container) from the
//     kubepods cgroup path. This is the Phase 13 light-up of the Phase 3
//     stub (ADR-0011 §3 lab-gating flipped by ADR-0024 §2 Decision A).
//
// **No cgo** (ADR-0024 §4(c)): the real reader is pure os / filepath —
// /proc + /proc/<pid>/fd symlink readlinks — so CGO_ENABLED=0
// GOARCH=arm64 cross-compile (ADR-0020) stays green without a build tag.
//
// **Decoupling-seam invariant** (ADR-0024 §2 Decision G): WorkloadCollector
// consumes the same WorkloadSample shape from either path; the demo / real
// difference is the SimRoot value chosen by sources.Select, not a branch
// in the collector layer.

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
	"sort"
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
// The real-/proc reader (empty SimRoot · P13-T-102) instead walks
// ProcRoot and returns whatever NPU-using containers it observes (or an
// empty slice on a host with none).
var ErrCgroupFsMissing = errors.New("cgroup simulator fs path does not exist")

// CgroupSource reads /proc/[pid]/cgroup (or a simulator fake-fs rooted
// at SimRoot) to derive pod identity for active NPU-using processes.
type CgroupSource struct {
	// SimRoot is the simulator fake-fs root. When set, ReadWorkloads
	// walks SimRoot/proc/<pid>/cgroup and resolves identity from
	// SimRoot/identity.json. When empty, ReadWorkloads walks the real
	// /proc (ProcRoot) for NPU-using processes (P13-T-102).
	SimRoot string

	// ProcRoot is the real procfs mount the real-/proc reader walks when
	// SimRoot is empty. Empty → "/proc" (the production default; the
	// DaemonSet mounts host /proc with hostPID). Tests point it at a
	// synthetic tree so the real path runs without a lab host.
	ProcRoot string

	mu       sync.Mutex
	identity map[string]podIdentity // keyed by podUID
}

// defaultProcRoot is the procfs mount the real reader walks when
// CgroupSource.ProcRoot is unset.
const defaultProcRoot = "/proc"

// npuDeviceFDMarker is the /dev path prefix whose presence in a process's
// open file descriptors marks it as an NPU-using process. Ascend exposes
// per-device char devices at /dev/davinci<N> (+ /dev/davinci_manager);
// a container holding one of these fds is actively bound to that NPU.
const npuDeviceFDMarker = "/dev/davinci"

type podIdentity struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
}

// NewCgroupSource constructs a CgroupSource. simRoot is the path to the
// simulator fake-fs; empty selects the real-/proc reader (P13-T-102),
// which walks the production ProcRoot (defaults to /proc).
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

// ReadWorkloads derives one WorkloadSample per NPU-using container. It
// dispatches on SimRoot: the simulator fake-fs reader (SimRoot set · demo
// profile) or the real-/proc reader (SimRoot empty · real profile · walks
// ProcRoot). Per-pid read failures are silently skipped so a single bad
// pid does not poison the whole scrape.
func (s *CgroupSource) ReadWorkloads(ctx context.Context) ([]WorkloadSample, error) {
	if s.SimRoot == "" {
		return s.readRealProc(ctx)
	}
	return s.readSimFS(ctx)
}

// readSimFS walks SimRoot/proc/<pid>/cgroup and derives one WorkloadSample
// per pid whose podUID resolves via identity.json (demo profile path,
// unchanged from Phase 3).
func (s *CgroupSource) readSimFS(_ context.Context) ([]WorkloadSample, error) {
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
		// Simulator path: NPUSecondsDelta + ActiveSlices are synthesized so
		// the metric families have shape parity with the real-data path.
		// Container label prefers the friendly name from identity.json; the
		// parsed scope name is only used if identity didn't pin a name.
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

// readRealProc walks the real procfs (ProcRoot, default /proc) and emits
// one WorkloadSample per pid that (a) holds an open /dev/davinci* handle
// (an NPU-using process) and (b) resolves to a kubepods pod slice. Pod
// identity (namespace QoS class + podUID + container id) comes from the
// cgroup path itself — there is no identity.json on a real host, so the
// Pod label carries the podUID and Container carries the CRI scope id.
//
// NPUSecondsDelta is emitted as 1.0 (one scrape interval the container was
// observed bound to the NPU); the metric is a Counter and Prometheus's
// rate() over it yields NPU-occupancy fraction. ActiveSlices counts the
// distinct /dev/davinci<N> handles the process holds.
func (s *CgroupSource) readRealProc(_ context.Context) ([]WorkloadSample, error) {
	procRoot := s.ProcRoot
	if procRoot == "" {
		procRoot = defaultProcRoot
	}
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, fmt.Errorf("read proc dir %q: %w", procRoot, err)
	}

	// De-dup per (podUID, container): multiple pids in the same container
	// (e.g. vllm worker + sidecar threads) collapse to one sample whose
	// ActiveSlices is the max davinci-fd count observed.
	type key struct{ podUID, container string }
	agg := map[key]*WorkloadSample{}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid := e.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue // not a numeric pid dir
		}
		devIndices := s.npuDeviceIndices(procRoot, pid)
		if len(devIndices) == 0 {
			continue // not an NPU-using process
		}
		cg, err := os.ReadFile(filepath.Join(procRoot, pid, "cgroup"))
		if err != nil {
			continue // best-effort per pid
		}
		podUID, container, ok := parseCgroupLine(string(cg))
		if !ok {
			continue // not a kubepods-managed process
		}
		namespace := qosFromCgroup(string(cg))
		k := key{podUID: podUID, container: container}
		if existing, hit := agg[k]; hit {
			if len(devIndices) > existing.ActiveSlices {
				existing.ActiveSlices = len(devIndices)
			}
			continue
		}
		agg[k] = &WorkloadSample{
			Namespace:       namespace,
			Pod:             podUID,
			Container:       firstNonEmpty(container, "unknown"),
			NPUSecondsDelta: 1.0,
			NPUID:           npuIDFromDeviceIndex(devIndices[0]), // lowest index
			ActiveSlices:    len(devIndices),
		}
	}

	out := make([]WorkloadSample, 0, len(agg))
	for _, ws := range agg {
		out = append(out, *ws)
	}
	return out, nil
}

// readlinkFn is the os.Readlink indirection used to resolve /proc/<pid>/fd
// symlinks. It is a package var so tests can drive the real readRealProc
// logic deterministically on platforms where creating /dev/davinci*
// symlinks needs privileges (Windows dev box) — production uses
// os.Readlink unchanged.
var readlinkFn = os.Readlink

// npuDeviceIndices returns the set of distinct /dev/davinci<N> device
// indices a pid holds open, by readlink-ing each /proc/<pid>/fd entry.
// /dev/davinci_manager and /dev/davinci_manager_docker are control
// handles, not per-device, so they are excluded.
func (s *CgroupSource) npuDeviceIndices(procRoot, pid string) []int {
	fdDir := filepath.Join(procRoot, pid, "fd")
	fds, err := os.ReadDir(fdDir)
	if err != nil {
		return nil // process exited / no fd visibility
	}
	seen := map[int]bool{}
	for _, fd := range fds {
		target, err := readlinkFn(filepath.Join(fdDir, fd.Name()))
		if err != nil {
			continue
		}
		if idx, ok := davinciDeviceIndex(target); ok {
			seen[idx] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]int, 0, len(seen))
	for idx := range seen {
		out = append(out, idx)
	}
	sort.Ints(out)
	return out
}

// davinciDeviceIndex extracts the per-device index N from a /dev/davinci<N>
// path, returning ok=false for the control handles (davinci_manager*) and
// any non-davinci target.
func davinciDeviceIndex(target string) (int, bool) {
	if !strings.HasPrefix(target, npuDeviceFDMarker) {
		return 0, false
	}
	suffix := strings.TrimPrefix(target, npuDeviceFDMarker)
	if suffix == "" || !isAllDigits(suffix) {
		return 0, false // davinci_manager / davinci_manager_docker etc.
	}
	n, err := strconv.Atoi(suffix)
	if err != nil {
		return 0, false
	}
	return n, true
}

// isAllDigits reports whether s is non-empty and every rune is 0-9.
func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// npuIDFromDeviceIndex maps a /dev/davinci index to the npu_id label
// convention. Index < 0 (no device resolved) → empty. NodeName is not in
// scope here (the cgroup path has no host name); the index alone keys the
// per-device series and the recording-rule join to ascend_npu_* carries
// the node.
func npuIDFromDeviceIndex(index int) string {
	if index < 0 {
		return ""
	}
	return "npu-" + strconv.Itoa(index)
}

// qosFromCgroup derives a coarse namespace bucket from the kubepods QoS
// class in the cgroup path (guaranteed pods sit directly under
// kubepods.slice; burstable / besteffort under their sub-slices). A real
// namespace requires a kubelet pod-status lookup (Phase 5+ inference-
// operator); until then the QoS class is the best path-only signal and is
// stable across scrapes.
func qosFromCgroup(cg string) string {
	switch {
	case strings.Contains(cg, "kubepods-besteffort"):
		return "besteffort"
	case strings.Contains(cg, "kubepods-burstable"):
		return "burstable"
	case strings.Contains(cg, "kubepods"):
		return "guaranteed"
	default:
		return ""
	}
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
