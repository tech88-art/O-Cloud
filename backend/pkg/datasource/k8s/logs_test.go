package k8s

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// The fake clientset's Pod.GetLogs returns an empty stream (no log
// data path implementation) — the test surface we exercise here is
// the parser + resolveWorkloadPod / workloadSelector wiring. End-to-
// end log content against a real kubelet is operator-side, covered
// by the manual smoke-test in deploy/single-node/README.md.

func TestParseLogLine_WithTimestamp(t *testing.T) {
	line := parseLogLine("2026-05-18T09:36:45.218Z hello world", "main")
	assert.Equal(t, "main", line.Container)
	assert.Equal(t, "hello world", line.Message)
	assert.Equal(t, 2026, line.Timestamp.Year())
	assert.Equal(t, 5, int(line.Timestamp.Month()))
	assert.Equal(t, 18, line.Timestamp.Day())
}

func TestParseLogLine_MissingTimestamp_FallsBackToRaw(t *testing.T) {
	line := parseLogLine("no-prefix message body", "main")
	// Timestamp parse fails on "no-prefix"; falls back to zero ts + raw
	// message. The fallback preserves the whole input as message.
	assert.True(t, line.Timestamp.IsZero(), "ts should be zero on parse fail")
	assert.Equal(t, "no-prefix message body", line.Message)
}

func TestParseLogLine_LevelExtraction(t *testing.T) {
	cases := []struct {
		body  string
		level string
	}{
		{"INFO: server started", "INFO"},
		{"[ERROR] kernel launch failed", "ERROR"},
		{"WARN slow tokenizer", "WARN"},
		{"FATAL out of memory", "FATAL"},
		{"DEBUG batch_size=4", "DEBUG"},
		{"benign line with no level", ""},
	}
	for _, c := range cases {
		line := parseLogLine("2026-05-18T09:36:45.218Z "+c.body, "main")
		assert.Equal(t, c.level, line.Level, "input %q", c.body)
	}
}

func TestExtractLevelHint_EmptyBody(t *testing.T) {
	assert.Equal(t, "", extractLevelHint(""))
}

func TestReadLogLines_MultiLine(t *testing.T) {
	raw := strings.NewReader(strings.Join([]string{
		"2026-05-18T09:36:45.218Z INFO request handled in 50ms",
		"2026-05-18T09:36:46.218Z WARN slow tokenizer pass took 120ms",
		"2026-05-18T09:36:47.218Z ERROR npu kernel launch failed",
	}, "\n"))

	lines, err := readLogLines(raw, "main")
	require.NoError(t, err)
	require.Len(t, lines, 3)
	assert.Equal(t, "INFO", lines[0].Level)
	assert.Equal(t, "WARN", lines[1].Level)
	assert.Equal(t, "ERROR", lines[2].Level)
}

func TestGetWorkloadLogs_EmptyArgs_ReturnsNotFound(t *testing.T) {
	src := NewSourceWithClient(fake.NewSimpleClientset(), Options{})
	_, err := src.GetWorkloadLogs(context.Background(), "", "x", model.LogOptions{})
	assert.ErrorIs(t, err, ErrResourceNotFound)
	_, err = src.GetWorkloadLogs(context.Background(), "ns", "", model.LogOptions{})
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestGetWorkloadLogs_NotFound_NoSuchWorkload(t *testing.T) {
	src := NewSourceWithClient(fake.NewSimpleClientset(), Options{})
	_, err := src.GetWorkloadLogs(context.Background(), "ns", "missing", model.LogOptions{})
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestStreamWorkloadLogs_EmptyArgs_ReturnsNotFound(t *testing.T) {
	src := NewSourceWithClient(fake.NewSimpleClientset(), Options{})
	_, err := src.StreamWorkloadLogs(context.Background(), "", "x", model.LogStreamOptions{})
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestStreamWorkloadLogs_NotFound_NoSuchWorkload(t *testing.T) {
	src := NewSourceWithClient(fake.NewSimpleClientset(), Options{})
	_, err := src.StreamWorkloadLogs(context.Background(), "ns", "missing", model.LogStreamOptions{})
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestResolveWorkloadPod_DeploymentWithPods_PicksLexSmallest(t *testing.T) {
	dep := mkDeployment("ns", "wl-1", 2, 2, nil)
	pZ := mkPod("ns", "wl-1-z", "node-1", "Running",
		map[string]string{"app.kubernetes.io/name": "wl-1"}, nil)
	pA := mkPod("ns", "wl-1-a", "node-2", "Running",
		map[string]string{"app.kubernetes.io/name": "wl-1"}, nil)
	client := fake.NewSimpleClientset(dep, pZ, pA)
	src := NewSourceWithClient(client, Options{})

	pod, container, err := src.resolveWorkloadPod(context.Background(), "ns", "wl-1", "")
	require.NoError(t, err)
	assert.Equal(t, "wl-1-a", pod, "lex-smallest pod name selected")
	assert.Equal(t, "main", container, "first container by default")
}

func TestResolveWorkloadPod_ContainerOverride_Validated(t *testing.T) {
	dep := mkDeployment("ns", "wl-1", 1, 1, nil)
	p := mkPod("ns", "wl-1-a", "node-1", "Running",
		map[string]string{"app.kubernetes.io/name": "wl-1"}, nil)
	client := fake.NewSimpleClientset(dep, p)
	src := NewSourceWithClient(client, Options{})

	// Valid container override.
	_, c, err := src.resolveWorkloadPod(context.Background(), "ns", "wl-1", "main")
	require.NoError(t, err)
	assert.Equal(t, "main", c)

	// Invalid override → ErrResourceNotFound.
	_, _, err = src.resolveWorkloadPod(context.Background(), "ns", "wl-1", "nonexistent")
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestResolveWorkloadPod_NoPods_Returns404(t *testing.T) {
	dep := mkDeployment("ns", "wl-1", 0, 0, nil)
	client := fake.NewSimpleClientset(dep)
	src := NewSourceWithClient(client, Options{})

	_, _, err := src.resolveWorkloadPod(context.Background(), "ns", "wl-1", "")
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestWorkloadSelector_TriesAllKinds(t *testing.T) {
	dep := mkDeployment("ns-a", "dep-1", 1, 1, nil)
	ss := mkStatefulSet("ns-b", "ss-1", 1, 1, nil)
	job := mkJob("ns-c", "job-1", 0, 1, "", "", nil)
	client := fake.NewSimpleClientset(dep, ss, job)
	src := NewSourceWithClient(client, Options{})

	got, err := src.workloadSelector(context.Background(), "ns-a", "dep-1")
	require.NoError(t, err)
	assert.Contains(t, got, "app.kubernetes.io/name=dep-1")

	got, err = src.workloadSelector(context.Background(), "ns-b", "ss-1")
	require.NoError(t, err)
	assert.Contains(t, got, "app.kubernetes.io/name=ss-1")

	got, err = src.workloadSelector(context.Background(), "ns-c", "job-1")
	require.NoError(t, err)
	assert.Contains(t, got, "job-name=job-1")

	_, err = src.workloadSelector(context.Background(), "ns-none", "missing")
	assert.ErrorIs(t, err, ErrResourceNotFound)
}
