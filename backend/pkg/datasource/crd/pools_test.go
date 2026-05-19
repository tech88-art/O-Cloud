package crd

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// mkSlicePool builds an unstructured NPUSlicePool with the given
// fields. Spec.Strategy + status carry the values we assert on.
func mkSlicePool(ns, name, strategy string, total, allocated, available int) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group: poolGroup, Version: poolVersion, Kind: "NPUSlicePool",
	})
	u.SetNamespace(ns)
	u.SetName(name)
	spec := map[string]any{
		"strategy": strategy,
	}
	if strategy == "FixedTemplate" {
		spec["fixedTemplates"] = []any{
			map[string]any{"name": "vir04", "aiCoreCount": int64(4), "memoryMiB": int64(16384)},
		}
	} else {
		spec["dynamicSlicing"] = map[string]any{
			"minAICore":            int64(1),
			"maxAICore":            int64(8),
			"memoryGranularityMiB": int64(1024),
			"allowAggregation":     true,
		}
	}
	spec["npuPoolRef"] = map[string]any{"name": "npupool-a"}
	u.Object["spec"] = spec
	u.Object["status"] = map[string]any{
		"totalSlices":     int64(total),
		"allocatedSlices": int64(allocated),
		"availableSlices": int64(available),
	}
	return u
}

// fakeDynamicClient builds the dynamic.fake client with the supplied
// objects pre-seeded. The fake needs a scheme that knows the list
// kind for our pool GVR — register an empty list type so it can
// shape the response correctly.
func fakeDynamicClient(objs ...runtime.Object) *fake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	// Register list kinds for each pool GVR — the fake dynamic
	// client looks them up by GVR → kind via the listKinds map.
	listKinds := map[schema.GroupVersionResource]string{
		gvrClusterPools:  "ClusterPoolList",
		gvrNodePools:     "NodePoolList",
		gvrNPUPools:      "NPUPoolList",
		gvrNPUSlicePools: "NPUSlicePoolList",
	}
	return fake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objs...)
}

func TestListNPUSlicePools_HappyPath_AllNamespaces(t *testing.T) {
	pool1 := mkSlicePool("ocloud-system", "slice-pool-a", "FixedTemplate", 12, 4, 8)
	pool2 := mkSlicePool("alt-system", "slice-pool-b", "Dynamic", 6, 0, 6)
	client := fakeDynamicClient(pool1, pool2)
	src := NewSourceWithClient(client, Options{}) // no namespace filter

	pools, err := src.ListNPUSlicePools(context.Background())
	require.NoError(t, err)
	require.Len(t, pools, 2)

	// Lex (namespace, name) order: alt-system/slice-pool-b first.
	assert.Equal(t, "alt-system", pools[0].Namespace)
	assert.Equal(t, "slice-pool-b", pools[0].Name)
	assert.Equal(t, "Dynamic", pools[0].Strategy)
	require.NotNil(t, pools[0].DynamicSlicing)
	assert.Equal(t, 1, pools[0].DynamicSlicing.MinAICore)
	assert.Equal(t, 8, pools[0].DynamicSlicing.MaxAICore)

	assert.Equal(t, "ocloud-system", pools[1].Namespace)
	assert.Equal(t, "slice-pool-a", pools[1].Name)
	assert.Equal(t, "FixedTemplate", pools[1].Strategy)
	require.Len(t, pools[1].FixedTemplates, 1)
	assert.Equal(t, "vir04", pools[1].FixedTemplates[0].Name)
	assert.Equal(t, 4, pools[1].FixedTemplates[0].AICoreCount)
	require.NotNil(t, pools[1].Status)
	assert.Equal(t, 12, pools[1].Status.TotalSlices)
	assert.Equal(t, 4, pools[1].Status.AllocatedSlices)
	assert.Equal(t, 8, pools[1].Status.AvailableSlices)

	// NPUPoolRef is the .name field on the LocalObjectReference.
	assert.Equal(t, "npupool-a", pools[0].NPUPoolRef)
	assert.Equal(t, "npupool-a", pools[1].NPUPoolRef)
}

func TestListNPUSlicePools_NamespaceFilter_ScopesToOne(t *testing.T) {
	pool1 := mkSlicePool("ocloud-system", "slice-pool-a", "FixedTemplate", 12, 4, 8)
	pool2 := mkSlicePool("alt-system", "slice-pool-b", "Dynamic", 6, 0, 6)
	client := fakeDynamicClient(pool1, pool2)
	src := NewSourceWithClient(client, Options{SlicePoolNamespace: "ocloud-system"})

	pools, err := src.ListNPUSlicePools(context.Background())
	require.NoError(t, err)
	require.Len(t, pools, 1)
	assert.Equal(t, "ocloud-system", pools[0].Namespace)
	assert.Equal(t, "slice-pool-a", pools[0].Name)
}

func TestListNPUSlicePools_EmptyList_NoError(t *testing.T) {
	client := fakeDynamicClient()
	src := NewSourceWithClient(client, Options{})

	pools, err := src.ListNPUSlicePools(context.Background())
	require.NoError(t, err)
	assert.Empty(t, pools)
}

func TestListNPUSlicePools_StatusOmittedWhenAllZero(t *testing.T) {
	// Spec-only pool with no status reported yet (operator hasn't
	// reconciled). Status pointer should stay nil rather than
	// surface a zero-value struct.
	pool := mkSlicePool("ns", "pristine", "FixedTemplate", 0, 0, 0)
	client := fakeDynamicClient(pool)
	src := NewSourceWithClient(client, Options{})

	pools, err := src.ListNPUSlicePools(context.Background())
	require.NoError(t, err)
	require.Len(t, pools, 1)
	assert.Nil(t, pools[0].Status, "all-zero status should not allocate a struct")
}

func TestListNPUSlicePools_CtxAlreadyCanceled_Errors(t *testing.T) {
	src := NewSourceWithClient(fakeDynamicClient(), Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := src.ListNPUSlicePools(ctx)
	require.Error(t, err)
}

func TestListNPUSlicePools_NotFound_ReturnsSentinel(t *testing.T) {
	// Force the fake's reactor to return a NotFound on the List call.
	client := fakeDynamicClient()
	client.PrependReactor("list", "npuslicepools", func(_ clientgotesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(gvrNPUSlicePools.GroupResource(), "npuslicepools")
	})
	src := NewSourceWithClient(client, Options{})

	_, err := src.ListNPUSlicePools(context.Background())
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestListClusterPools_ReturnsUnstructured(t *testing.T) {
	cp := &unstructured.Unstructured{}
	cp.SetGroupVersionKind(schema.GroupVersionKind{
		Group: poolGroup, Version: poolVersion, Kind: "ClusterPool",
	})
	cp.SetName("cluster-pool-a")
	cp.Object["spec"] = map[string]any{
		"clusters": []any{
			map[string]any{"clusterId": "cluster-prod-a-01"},
		},
	}
	client := fakeDynamicClient(cp)
	src := NewSourceWithClient(client, Options{})

	out, err := src.ListClusterPools(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "cluster-pool-a", out[0].GetName())
}

func TestListNodePools_ReturnsUnstructured(t *testing.T) {
	np := &unstructured.Unstructured{}
	np.SetGroupVersionKind(schema.GroupVersionKind{
		Group: poolGroup, Version: poolVersion, Kind: "NodePool",
	})
	np.SetName("worker-pool")
	np.Object["spec"] = map[string]any{"role": "edge"}
	client := fakeDynamicClient(np)
	src := NewSourceWithClient(client, Options{})

	out, err := src.ListNodePools(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "worker-pool", out[0].GetName())
}

func TestListNPUPools_ReturnsUnstructured(t *testing.T) {
	npp := &unstructured.Unstructured{}
	npp.SetGroupVersionKind(schema.GroupVersionKind{
		Group: poolGroup, Version: poolVersion, Kind: "NPUPool",
	})
	npp.SetName("ascend-910b")
	npp.Object["spec"] = map[string]any{
		"npuModel": "Ascend910B",
	}
	client := fakeDynamicClient(npp)
	src := NewSourceWithClient(client, Options{})

	out, err := src.ListNPUPools(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "ascend-910b", out[0].GetName())
}

func TestCapabilities_OnlyPools(t *testing.T) {
	src := NewSourceWithClient(fakeDynamicClient(), Options{})
	caps := src.Capabilities()
	assert.True(t, caps.Pools)
	assert.False(t, caps.Clusters)
	assert.False(t, caps.Nodes)
	assert.False(t, caps.Workloads)
	assert.False(t, caps.Metrics)
}

func TestSource_StubsReturnErrCapabilityUnavailable(t *testing.T) {
	src := NewSourceWithClient(fakeDynamicClient(), Options{})
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
	_, e = src.StreamEvents(ctx, model.StreamEventsOptions{})
	assert.ErrorIs(t, e, datasource.ErrCapabilityUnavailable)
}

func TestName_IsCRD(t *testing.T) {
	src := NewSourceWithClient(fakeDynamicClient(), Options{})
	assert.Equal(t, "crd", src.Name())
}

func TestSatisfiesDatasourceSource(t *testing.T) {
	var _ datasource.Source = NewSourceWithClient(fakeDynamicClient(), Options{})
}

func TestProjectNPUSlicePool_NilInput(t *testing.T) {
	assert.Nil(t, projectNPUSlicePool(nil))
}

func TestTranslateAPIError_PassThroughOtherErrors(t *testing.T) {
	err := errors.New("transient connection reset")
	got := translateAPIError(err)
	assert.Equal(t, err, got, "non-NotFound errors pass through unchanged")
	assert.Nil(t, translateAPIError(nil), "nil in → nil out")
}
