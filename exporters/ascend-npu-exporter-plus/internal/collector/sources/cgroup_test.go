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

// TestCgroupSource_RealProcStub asserts that constructing the source
// with an empty SimRoot succeeds (it is the Phase 4+ real-/proc path)
// but ReadWorkloads returns ErrSourceNotAvailable in Phase 3.
func TestCgroupSource_RealProcStub(t *testing.T) {
	s, err := NewCgroupSource("")
	require.NoError(t, err)
	_, err = s.ReadWorkloads(context.Background())
	require.ErrorIs(t, err, ErrSourceNotAvailable)
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
		name        string
		line        string
		wantPodUID  string
		wantCID     string
		wantOK      bool
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
