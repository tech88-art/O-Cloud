package configmap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

func mkPresetsCM(ns, name string, entries map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Data:       entries,
	}
}

func TestListPresets_HappyPath_LexSorted(t *testing.T) {
	cm := mkPresetsCM("ocloud-system", "ocloud-presets", map[string]string{
		// Intentionally out of lex order to verify sort.
		"qwen-14b":     `{"id":"qwen-14b","name":"Qwen 14B","kind":"inference"}`,
		"pi-3b":        `{"id":"pi-3b","name":"Pi 3B","kind":"inference"}`,
		"qwen-8b-pd":   `{"id":"qwen-8b-pd","name":"Qwen 8B PD","kind":"inference-pd"}`,
		"deepseek-20b": `{"id":"deepseek-20b","name":"DeepSeek 20B","kind":"inference"}`,
	})
	client := fake.NewSimpleClientset(cm)
	src := NewSourceWithClient(client, Options{})

	presets, err := src.ListPresets(context.Background())
	require.NoError(t, err)
	require.Len(t, presets, 4)

	got := make([]string, 0, 4)
	for _, p := range presets {
		got = append(got, p.ID)
	}
	assert.Equal(t,
		[]string{"deepseek-20b", "pi-3b", "qwen-14b", "qwen-8b-pd"},
		got, "lex sort by id")
}

func TestListPresets_KeyAsFallbackID(t *testing.T) {
	// JSON missing the inline `id` field — should fall back to the
	// ConfigMap data key.
	cm := mkPresetsCM("ocloud-system", "ocloud-presets", map[string]string{
		"hand-edited": `{"name":"Hand Edited Preset","kind":"inference"}`,
	})
	client := fake.NewSimpleClientset(cm)
	src := NewSourceWithClient(client, Options{})

	presets, err := src.ListPresets(context.Background())
	require.NoError(t, err)
	require.Len(t, presets, 1)
	assert.Equal(t, "hand-edited", presets[0].ID)
	assert.Equal(t, "Hand Edited Preset", presets[0].Name)
}

func TestListPresets_MalformedEntrySilentlyDropped(t *testing.T) {
	cm := mkPresetsCM("ocloud-system", "ocloud-presets", map[string]string{
		"good": `{"id":"good","name":"OK","kind":"inference"}`,
		"bad":  `{not-json`,
	})
	client := fake.NewSimpleClientset(cm)
	src := NewSourceWithClient(client, Options{})

	presets, err := src.ListPresets(context.Background())
	require.NoError(t, err)
	require.Len(t, presets, 1)
	assert.Equal(t, "good", presets[0].ID)
}

func TestListPresets_ConfigMapMissing_ReturnsSentinel(t *testing.T) {
	client := fake.NewSimpleClientset() // no CM seeded
	src := NewSourceWithClient(client, Options{})

	_, err := src.ListPresets(context.Background())
	assert.ErrorIs(t, err, ErrConfigMapNotFound)
}

func TestListPresets_CustomNamespaceAndName(t *testing.T) {
	cm := mkPresetsCM("custom-ns", "presets-v2", map[string]string{
		"only": `{"id":"only","name":"Only","kind":"inference"}`,
	})
	client := fake.NewSimpleClientset(cm)
	src := NewSourceWithClient(client, Options{
		Namespace: "custom-ns",
		Name:      "presets-v2",
	})

	presets, err := src.ListPresets(context.Background())
	require.NoError(t, err)
	require.Len(t, presets, 1)
}

func TestListPresets_EmptyConfigMap_ReturnsEmptyList(t *testing.T) {
	cm := mkPresetsCM("ocloud-system", "ocloud-presets", map[string]string{})
	client := fake.NewSimpleClientset(cm)
	src := NewSourceWithClient(client, Options{})

	presets, err := src.ListPresets(context.Background())
	require.NoError(t, err)
	assert.Empty(t, presets)
}

func TestGetPreset_HappyPath_ReturnsDetail(t *testing.T) {
	cm := mkPresetsCM("ocloud-system", "ocloud-presets", map[string]string{
		"qwen-8b-pd": `{
			"id":"qwen-8b-pd",
			"name":"Qwen 8B PD",
			"kind":"inference-pd",
			"runtime":"vllm-ascend",
			"manifest":"helm://qwen-8b-pd.yaml",
			"requirements":{"npuCount":2,"npuModel":"Ascend910B","vramMiBPerNPU":65536}
		}`,
	})
	client := fake.NewSimpleClientset(cm)
	src := NewSourceWithClient(client, Options{})

	d, err := src.GetPreset(context.Background(), "qwen-8b-pd")
	require.NoError(t, err)
	assert.Equal(t, "qwen-8b-pd", d.ID)
	assert.Equal(t, "Qwen 8B PD", d.Name)
	assert.Equal(t, "inference-pd", d.Kind)
	assert.Equal(t, "vllm-ascend", d.Runtime)
	assert.Equal(t, "helm://qwen-8b-pd.yaml", d.Manifest)
	require.NotNil(t, d.Requirements)
	assert.Equal(t, 2, d.Requirements.NPUCount)
	assert.Equal(t, "Ascend910B", d.Requirements.NPUModel)
}

func TestGetPreset_NotFound(t *testing.T) {
	cm := mkPresetsCM("ocloud-system", "ocloud-presets", map[string]string{
		"pi-3b": `{"id":"pi-3b","name":"Pi 3B","kind":"inference"}`,
	})
	client := fake.NewSimpleClientset(cm)
	src := NewSourceWithClient(client, Options{})

	_, err := src.GetPreset(context.Background(), "no-such-preset")
	assert.ErrorIs(t, err, ErrPresetNotFound)
}

func TestGetPreset_EmptyID(t *testing.T) {
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{})

	_, err := src.GetPreset(context.Background(), "")
	assert.ErrorIs(t, err, ErrPresetNotFound)
}

func TestGetPreset_ConfigMapMissing(t *testing.T) {
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{})

	_, err := src.GetPreset(context.Background(), "anything")
	assert.ErrorIs(t, err, ErrConfigMapNotFound)
}

func TestCapabilities_OnlyPresets(t *testing.T) {
	src := NewSourceWithClient(fake.NewSimpleClientset(), Options{})
	caps := src.Capabilities()
	assert.True(t, caps.Presets)
	assert.False(t, caps.Clusters)
	assert.False(t, caps.Workloads)
	assert.False(t, caps.Pools)
	assert.False(t, caps.Metrics)
}

func TestSource_StubsReturnErrCapabilityUnavailable(t *testing.T) {
	src := NewSourceWithClient(fake.NewSimpleClientset(), Options{})
	ctx := context.Background()

	_, e := src.ListClusters(ctx)
	assert.ErrorIs(t, e, datasource.ErrCapabilityUnavailable)
	_, e = src.ListNodes(ctx, model.NodeFilter{})
	assert.ErrorIs(t, e, datasource.ErrCapabilityUnavailable)
	_, e = src.ListNPUs(ctx, "x")
	assert.ErrorIs(t, e, datasource.ErrCapabilityUnavailable)
	_, e = src.ListWorkloads(ctx, model.WorkloadFilter{})
	assert.ErrorIs(t, e, datasource.ErrCapabilityUnavailable)
	_, e = src.QueryMetric(ctx, "t", nil, model.TimeRange{})
	assert.ErrorIs(t, e, datasource.ErrCapabilityUnavailable)
}

func TestName_IsConfigMap(t *testing.T) {
	src := NewSourceWithClient(fake.NewSimpleClientset(), Options{})
	assert.Equal(t, "configmap", src.Name())
}

func TestSatisfiesDatasourceSource(t *testing.T) {
	var _ datasource.Source = NewSourceWithClient(fake.NewSimpleClientset(), Options{})
}

func TestDefaultsApplied(t *testing.T) {
	src := NewSourceWithClient(fake.NewSimpleClientset(), Options{})
	// Construct against the default ConfigMap location — should error
	// with ErrConfigMapNotFound (defaults are `ocloud-system`/`ocloud-presets`,
	// and nothing's seeded).
	_, err := src.ListPresets(context.Background())
	require.ErrorIs(t, err, ErrConfigMapNotFound)
	assert.Contains(t, err.Error(), "ocloud-system/ocloud-presets")
}
