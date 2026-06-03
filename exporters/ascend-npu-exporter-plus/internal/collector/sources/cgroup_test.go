package sources

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCgroupSource_RealProc_MissingProcRoot asserts the real reader
// (empty SimRoot) surfaces a read error when ProcRoot does not exist —
// the production default /proc is always present, so this only fires on a
// mis-configured ProcRoot, and the collector turns the error into zero
// metrics rather than a panic.
func TestCgroupSource_RealProc_MissingProcRoot(t *testing.T) {
	s, err := NewCgroupSource("")
	require.NoError(t, err)
	s.ProcRoot = filepath.Join(os.TempDir(), "ascend-exporter-no-such-procroot-xyz789")
	_, err = s.ReadWorkloads(context.Background())
	require.Error(t, err)
}

// TestCgroupSource_RealProc_EmptyProc asserts the real reader returns an
// empty slice (no error) when ProcRoot exists but holds no NPU-using
// processes — the common idle-host case.
func TestCgroupSource_RealProc_EmptyProc(t *testing.T) {
	s, err := NewCgroupSource("")
	require.NoError(t, err)
	s.ProcRoot = t.TempDir() // exists, no pid dirs
	samples, err := s.ReadWorkloads(context.Background())
	require.NoError(t, err)
	assert.Empty(t, samples)
}

// buildRealProcFS materialises a synthetic /proc tree with NPU-using and
// non-NPU pids so the real reader runs without a lab host — on EVERY
// platform. Real /proc/<pid>/fd entries are symlinks, but creating
// /dev/davinci* symlinks needs privileges on Windows, so instead we write
// plain placeholder files for each fd and override the package readlinkFn
// (restored on test cleanup) to map each fd path back to its target. This
// exercises the production readRealProc / npuDeviceIndices logic exactly;
// only the os.Readlink syscall is stubbed.
func buildRealProcFS(t *testing.T, pids []realProcPid) string {
	t.Helper()
	root := t.TempDir()
	targets := map[string]string{} // absolute fd path -> /dev target

	for _, p := range pids {
		dir := filepath.Join(root, p.pid)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "cgroup"), []byte(p.cgroup), 0o644))
		fdDir := filepath.Join(dir, "fd")
		require.NoError(t, os.MkdirAll(fdDir, 0o755))
		for i, target := range p.fdTargets {
			fdPath := filepath.Join(fdDir, intName(i))
			require.NoError(t, os.WriteFile(fdPath, []byte{}, 0o644))
			targets[fdPath] = target
		}
	}

	orig := readlinkFn
	readlinkFn = func(name string) (string, error) {
		if tgt, ok := targets[name]; ok {
			return tgt, nil
		}
		return "", os.ErrInvalid
	}
	t.Cleanup(func() { readlinkFn = orig })
	return root
}

type realProcPid struct {
	pid       string
	cgroup    string
	fdTargets []string
}

func intName(i int) string {
	return []string{"0", "1", "2", "3", "4", "5"}[i]
}

// TestCgroupSource_RealProc_HappyPath builds a synthetic /proc with two
// NPU-using containers (one holding 2 davinci devices, one holding 1) and
// one non-NPU process; the real reader emits exactly the two NPU samples
// with correct ActiveSlices + QoS namespace.
func TestCgroupSource_RealProc_HappyPath(t *testing.T) {
	root := buildRealProcFS(t, []realProcPid{
		{
			pid:       "1001",
			cgroup:    "0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod3a4b5c6d-prefill.slice/cri-containerd-abc123.scope",
			fdTargets: []string{"/dev/davinci0", "/dev/davinci1", "/dev/davinci_manager", "/dev/null"},
		},
		{
			pid:       "1002",
			cgroup:    "0::/kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-pod9f8e7d6c-decode.slice/crio-def456.scope",
			fdTargets: []string{"/dev/davinci2", "/dev/davinci_manager"},
		},
		{
			// non-NPU process: no davinci fd → skipped.
			pid:       "1003",
			cgroup:    "0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podaaaa-side.slice/cri-containerd-zzz.scope",
			fdTargets: []string{"/dev/null", "/dev/davinci_manager"},
		},
	})

	s, err := NewCgroupSource("")
	require.NoError(t, err)
	s.ProcRoot = root

	samples, err := s.ReadWorkloads(context.Background())
	require.NoError(t, err)
	require.Len(t, samples, 2)

	sort.Slice(samples, func(i, j int) bool { return samples[i].Pod < samples[j].Pod })

	// pod3a4b5c6d-prefill holds davinci0 + davinci1 → ActiveSlices 2,
	// burstable QoS, NPUID from lowest index (0).
	assert.Equal(t, "3a4b5c6d-prefill", samples[0].Pod)
	assert.Equal(t, "burstable", samples[0].Namespace)
	assert.Equal(t, 2, samples[0].ActiveSlices)
	assert.Equal(t, "npu-0", samples[0].NPUID)
	assert.Equal(t, 1.0, samples[0].NPUSecondsDelta)
	assert.Equal(t, "abc123", samples[0].Container)

	// pod9f8e7d6c-decode holds davinci2 only → ActiveSlices 1, besteffort.
	assert.Equal(t, "9f8e7d6c-decode", samples[1].Pod)
	assert.Equal(t, "besteffort", samples[1].Namespace)
	assert.Equal(t, 1, samples[1].ActiveSlices)
	assert.Equal(t, "npu-2", samples[1].NPUID)
}

// TestCgroupSource_RealProc_NonKubepodsSkipped asserts an NPU-using
// process NOT under kubepods (e.g. a host daemon) is skipped — only
// container workloads are reported.
func TestCgroupSource_RealProc_NonKubepodsSkipped(t *testing.T) {
	root := buildRealProcFS(t, []realProcPid{
		{
			pid:       "2001",
			cgroup:    "0::/system.slice/some-host-daemon.service",
			fdTargets: []string{"/dev/davinci0"},
		},
	})
	s, err := NewCgroupSource("")
	require.NoError(t, err)
	s.ProcRoot = root
	samples, err := s.ReadWorkloads(context.Background())
	require.NoError(t, err)
	assert.Empty(t, samples)
}

// TestCgroupSource_SimRootMissing asserts that a non-empty SimRoot
// pointing to a non-existent directory returns ErrCgroupFsMissing.
func TestCgroupSource_SimRootMissing(t *testing.T) {
	missing := filepath.Join(os.TempDir(), "ascend-npu-exporter-plus-definitely-not-real-cgroupfs-xyz123")
	_, err := NewCgroupSource(missing)
	require.ErrorIs(t, err, ErrCgroupFsMissing)
}

// TestCgroupSource_HappyPath_3Pods uses the committed testdata fixture
// to assert all 3 pods (prefill / decode / single) resolve correctly.
func TestCgroupSource_HappyPath_3Pods(t *testing.T) {
	// Locate the testdata fixture relative to the package directory.
	// internal/collector/sources -> ../../../testdata/simulator-cgroup-fs.
	root := filepath.Join("..", "..", "..", "testdata", "simulator-cgroup-fs")
	s, err := NewCgroupSource(root)
	require.NoError(t, err)

	samples, err := s.ReadWorkloads(context.Background())
	require.NoError(t, err)
	require.Len(t, samples, 3)

	// Stable sort by Pod for deterministic assertions.
	sort.Slice(samples, func(i, j int) bool { return samples[i].Pod < samples[j].Pod })

	// qwen-3b-single-0 / qwen-8b-decode-0 / qwen-8b-prefill-0 (alpha)
	assert.Equal(t, "default", samples[0].Namespace)
	assert.Equal(t, "qwen-3b-single-0", samples[0].Pod)
	assert.Equal(t, "vllm-runtime", samples[0].Container)

	assert.Equal(t, "ocloud-system", samples[1].Namespace)
	assert.Equal(t, "qwen-8b-decode-0", samples[1].Pod)
	assert.Equal(t, "vllm-decode", samples[1].Container)

	assert.Equal(t, "ocloud-system", samples[2].Namespace)
	assert.Equal(t, "qwen-8b-prefill-0", samples[2].Pod)
	assert.Equal(t, "vllm-prefill", samples[2].Container)

	for _, s := range samples {
		assert.Equal(t, 1.0, s.NPUSecondsDelta)
		assert.Equal(t, 1, s.ActiveSlices)
		assert.NotEmpty(t, s.NPUID)
	}
}

// TestCgroupSource_MalformedCgroupLine_SkipsPid writes a synthetic
// /proc tree with one malformed cgroup line; the bad pid is silently
// skipped (no panic, returns 0 samples).
func TestCgroupSource_MalformedCgroupLine_SkipsPid(t *testing.T) {
	tmp := t.TempDir()
	// identity.json (empty map -> no pods resolve even if line parsed)
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "identity.json"), []byte("{}"), 0o644))
	// /proc/1/cgroup with garbage that does not start with "0::"
	procDir := filepath.Join(tmp, "proc", "1")
	require.NoError(t, os.MkdirAll(procDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(procDir, "cgroup"), []byte("garbage no slice here"), 0o644))

	s, err := NewCgroupSource(tmp)
	require.NoError(t, err)
	samples, err := s.ReadWorkloads(context.Background())
	require.NoError(t, err)
	assert.Empty(t, samples)
}

// TestCgroupSource_NonNumericPidDir_Skipped asserts that non-numeric
// directories under proc/ are ignored (per /proc convention).
func TestCgroupSource_NonNumericPidDir_Skipped(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "identity.json"), []byte("{}"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "proc", "self"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "proc", "self", "cgroup"),
		[]byte("0::/kubepods.slice/kubepods-pod3a4b5c6d-prefill.slice/cri-containerd-abc.scope"), 0o644))

	s, err := NewCgroupSource(tmp)
	require.NoError(t, err)
	samples, err := s.ReadWorkloads(context.Background())
	require.NoError(t, err)
	assert.Empty(t, samples)
}

// TestParseCgroupLine_ContainerdAndCriO covers the three CRI runtime
// scope-name prefixes (cri-containerd / crio / docker) plus a v1
// rejection case.
func TestParseCgroupLine_ContainerdAndCriO(t *testing.T) {
	cases := []struct {
		name       string
		line       string
		wantPodUID string
		wantCID    string
		wantOK     bool
	}{
		{
			name:       "containerd",
			line:       "0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod3a4b5c6d-prefill.slice/cri-containerd-abc123def456.scope",
			wantPodUID: "3a4b5c6d-prefill",
			wantCID:    "abc123def456",
			wantOK:     true,
		},
		{
			name:       "crio",
			line:       "0::/kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-pod9f8e7d6c-single.slice/crio-fedcba987654.scope",
			wantPodUID: "9f8e7d6c-single",
			wantCID:    "fedcba987654",
			wantOK:     true,
		},
		{
			name:       "docker",
			line:       "0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod3a4b5c6d-decode.slice/docker-123456abcdef.scope",
			wantPodUID: "3a4b5c6d-decode",
			wantCID:    "123456abcdef",
			wantOK:     true,
		},
		{
			name:   "cgroup-v1-multiline-rejected",
			line:   "12:cpu,cpuacct:/kubepods.slice/kubepods-pod3a4b5c6d-prefill.slice",
			wantOK: false,
		},
		{
			name:       "pod-without-container-scope",
			line:       "0::/kubepods.slice/kubepods-pod3a4b5c6d-prefill.slice",
			wantPodUID: "3a4b5c6d-prefill",
			wantCID:    "",
			wantOK:     true,
		},
		{
			name:   "garbage-no-prefix",
			line:   "garbage no slice here",
			wantOK: false,
		},
		{
			name:       "trailing-whitespace-trimmed",
			line:       "  0::/kubepods.slice/kubepods-pod3a4b5c6d-prefill.slice/cri-containerd-abc.scope  \n",
			wantPodUID: "3a4b5c6d-prefill",
			wantCID:    "abc",
			wantOK:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			podUID, cID, ok := parseCgroupLine(tc.line)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantPodUID, podUID)
				assert.Equal(t, tc.wantCID, cID)
			}
		})
	}
}
