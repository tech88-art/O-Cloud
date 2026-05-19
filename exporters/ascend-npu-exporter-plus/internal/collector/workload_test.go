package collector

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/collector/sources"
)

// buildSimulatorFS materialises a 3-pod simulator-cgroup-fs in tmp and
// returns the root. Mirrors testdata/simulator-cgroup-fs but keeps tests
// hermetic (independent of working directory).
func buildSimulatorFS(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	identity := []byte(`{
  "3a4b5c6d-prefill": {"namespace": "ocloud-system", "pod": "qwen-8b-prefill-0", "container": "vllm-prefill"},
  "3a4b5c6d-decode":  {"namespace": "ocloud-system", "pod": "qwen-8b-decode-0",  "container": "vllm-decode"},
  "9f8e7d6c-single":  {"namespace": "default",       "pod": "qwen-3b-single-0",  "container": "vllm-runtime"}
}`)
	require.NoError(t, os.WriteFile(filepath.Join(root, "identity.json"), identity, 0o644))

	mk := func(pid, line string) {
		dir := filepath.Join(root, "proc", pid)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "cgroup"), []byte(line), 0o644))
	}
	mk("1001", "0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod3a4b5c6d-prefill.slice/cri-containerd-abc123def456.scope")
	mk("1002", "0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod3a4b5c6d-decode.slice/cri-containerd-def456abc123.scope")
	mk("1003", "0::/kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-pod9f8e7d6c-single.slice/crio-fedcba987654.scope")
	return root
}

// TestWorkloadCollector_HappyPath_3Containers builds a hermetic 3-pod
// fake fs and asserts CollectAndCount sees 3 series per family.
func TestWorkloadCollector_HappyPath_3Containers(t *testing.T) {
	root := buildSimulatorFS(t)
	src, err := sources.NewCgroupSource(root)
	require.NoError(t, err)

	c := NewWorkloadCollector(src)
	reg := prometheus.NewPedanticRegistry()
	require.NoError(t, reg.Register(c))

	assert.Equal(t, 3, testutil.CollectAndCount(c, "ascend_workload_npu_seconds_total"))
	assert.Equal(t, 3, testutil.CollectAndCount(c, "ascend_workload_active_slices"))
}

// TestWorkloadCollector_SourceUnavailable_NoMetrics verifies that an
// unavailable source (empty SimRoot -> ErrSourceNotAvailable) results in
// zero metrics being emitted (and a logged error, but the collector does
// not panic or hang).
func TestWorkloadCollector_SourceUnavailable_NoMetrics(t *testing.T) {
	src, err := sources.NewCgroupSource("")
	require.NoError(t, err)

	c := NewWorkloadCollector(src)
	assert.Equal(t, 0, testutil.CollectAndCount(c, "ascend_workload_npu_seconds_total"))
	assert.Equal(t, 0, testutil.CollectAndCount(c, "ascend_workload_active_slices"))
}

// TestWorkloadCollector_NPUSecondsTotalIsCounter exercises the registry
// Gather() path so we can inspect MetricFamily.Type and confirm the
// family is a Counter (not a Gauge). This guards against accidental
// regressions where the family is silently downgraded.
func TestWorkloadCollector_NPUSecondsTotalIsCounter(t *testing.T) {
	root := buildSimulatorFS(t)
	src, err := sources.NewCgroupSource(root)
	require.NoError(t, err)

	c := NewWorkloadCollector(src)
	reg := prometheus.NewPedanticRegistry()
	require.NoError(t, reg.Register(c))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	var (
		secondsFam *dto.MetricFamily
		slicesFam  *dto.MetricFamily
	)
	for _, mf := range mfs {
		switch mf.GetName() {
		case "ascend_workload_npu_seconds_total":
			secondsFam = mf
		case "ascend_workload_active_slices":
			slicesFam = mf
		}
	}
	require.NotNil(t, secondsFam, "ascend_workload_npu_seconds_total must be emitted")
	require.NotNil(t, slicesFam, "ascend_workload_active_slices must be emitted")
	assert.Equal(t, dto.MetricType_COUNTER, secondsFam.GetType(),
		"npu_seconds_total must be a Counter; got %s", secondsFam.GetType().String())
	assert.Equal(t, dto.MetricType_GAUGE, slicesFam.GetType(),
		"active_slices must be a Gauge; got %s", slicesFam.GetType().String())
}

// TestWorkloadCollector_Describe asserts Describe emits exactly two
// Desc handles (matches Counter + Gauge family count).
func TestWorkloadCollector_Describe(t *testing.T) {
	src, err := sources.NewCgroupSource("")
	require.NoError(t, err)
	c := NewWorkloadCollector(src)

	ch := make(chan *prometheus.Desc, 8)
	c.Describe(ch)
	close(ch)
	count := 0
	for range ch {
		count++
	}
	assert.Equal(t, 2, count)
}
