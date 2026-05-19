package crd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

func TestResolveSlicesForNPUs_FixedTemplate_HappyPath(t *testing.T) {
	pool := mkSlicePool("ns", "slice-pool-1", "FixedTemplate", 0, 0, 0)
	client := fakeDynamicClient(pool)
	src := NewSourceWithClient(client, Options{})

	npus := []*model.NPU{
		{
			ID:        "worker-a-01-npu-0",
			NodeName:  "worker-a-01",
			Model:     "Ascend910B",
			HCCSGroup: "worker-a-01-hccs-0",
			SliceMode: "fixed-template",
		},
		{
			ID:        "worker-a-01-npu-1",
			NodeName:  "worker-a-01",
			Model:     "Ascend910B",
			HCCSGroup: "worker-a-01-hccs-0",
			SliceMode: "fixed-template",
		},
	}

	slices, err := src.ResolveSlicesForNPUs(context.Background(), npus)
	require.NoError(t, err)
	// 2 NPUs × 1 template each (mkSlicePool seeds one vir04 entry) = 2 slices.
	require.Len(t, slices, 2)
	assert.Equal(t, "worker-a-01-npu-0-slice-0", slices[0].ID)
	assert.Equal(t, "worker-a-01-npu-0", slices[0].ParentNPU)
	assert.Equal(t, "vir04", slices[0].Template)
	assert.Equal(t, 4, slices[0].AICore)
	assert.Equal(t, 16384, slices[0].VRAMMiB)
	assert.Equal(t, "available", slices[0].Status)
	assert.Equal(t, "worker-a-01-npu-1-slice-0", slices[1].ID)
}

func TestResolveSlicesForNPUs_Dynamic_PlaceholderTwoSlices(t *testing.T) {
	pool := mkSlicePool("ns", "dyn-pool", "Dynamic", 0, 0, 0)
	client := fakeDynamicClient(pool)
	src := NewSourceWithClient(client, Options{})

	npus := []*model.NPU{{
		ID:        "worker-a-01-npu-0",
		NodeName:  "worker-a-01",
		SliceMode: "dynamic",
	}}
	slices, err := src.ResolveSlicesForNPUs(context.Background(), npus)
	require.NoError(t, err)
	require.Len(t, slices, 2)
	assert.Equal(t, "worker-a-01-npu-0-slice-0", slices[0].ID)
	assert.Equal(t, "dynamic", slices[0].Template)
	assert.Equal(t, "worker-a-01-npu-0-slice-1", slices[1].ID)
}

func TestResolveSlicesForNPUs_WholeNPU_NoSlices(t *testing.T) {
	pool := mkSlicePool("ns", "fixed-pool", "FixedTemplate", 0, 0, 0)
	client := fakeDynamicClient(pool)
	src := NewSourceWithClient(client, Options{})

	npus := []*model.NPU{{
		ID:        "worker-a-01-npu-0",
		NodeName:  "worker-a-01",
		SliceMode: "whole",
	}}
	slices, err := src.ResolveSlicesForNPUs(context.Background(), npus)
	require.NoError(t, err)
	assert.Empty(t, slices, "whole-NPU mode must not generate per-slice entries")
}

func TestResolveSlicesForNPUs_NoPools_ReturnsEmpty(t *testing.T) {
	src := NewSourceWithClient(fakeDynamicClient(), Options{})
	npus := []*model.NPU{{ID: "n", SliceMode: "fixed-template"}}
	slices, err := src.ResolveSlicesForNPUs(context.Background(), npus)
	require.NoError(t, err)
	assert.Empty(t, slices)
}

func TestResolveSlicesForNPUs_NoNPUs_ReturnsEmpty(t *testing.T) {
	pool := mkSlicePool("ns", "fixed-pool", "FixedTemplate", 0, 0, 0)
	client := fakeDynamicClient(pool)
	src := NewSourceWithClient(client, Options{})
	slices, err := src.ResolveSlicesForNPUs(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, slices)
}

func TestResolveSlicesForNPUs_NilNPUEntrySkipped(t *testing.T) {
	pool := mkSlicePool("ns", "fixed-pool", "FixedTemplate", 0, 0, 0)
	client := fakeDynamicClient(pool)
	src := NewSourceWithClient(client, Options{})
	npus := []*model.NPU{
		nil,
		{ID: "real", NodeName: "n", SliceMode: "fixed-template"},
	}
	slices, err := src.ResolveSlicesForNPUs(context.Background(), npus)
	require.NoError(t, err)
	require.Len(t, slices, 1)
	assert.Equal(t, "real-slice-0", slices[0].ID)
}

func TestPickPoolForNPU_PrefixMatchOnNodeName(t *testing.T) {
	a := &model.NPUSlicePool{Name: "pool-a", NPUPoolRef: "worker-a-01", Strategy: "FixedTemplate"}
	b := &model.NPUSlicePool{Name: "pool-b", NPUPoolRef: "worker-b-01", Strategy: "FixedTemplate"}
	npu := &model.NPU{NodeName: "worker-a-01", HCCSGroup: "irrelevant", Model: "X"}
	got := pickPoolForNPU(npu, []*model.NPUSlicePool{a, b})
	require.NotNil(t, got)
	assert.Equal(t, "pool-a", got.Name)
}

func TestPickPoolForNPU_SuffixMatchOnHCCSGroup(t *testing.T) {
	pool := &model.NPUSlicePool{Name: "pool", NPUPoolRef: "hccs-0", Strategy: "FixedTemplate"}
	npu := &model.NPU{HCCSGroup: "worker-a-01-hccs-0", Model: "X", NodeName: "Y"}
	got := pickPoolForNPU(npu, []*model.NPUSlicePool{pool})
	require.NotNil(t, got)
	assert.Equal(t, "pool", got.Name)
}

func TestPickPoolForNPU_ModelEquality(t *testing.T) {
	pool := &model.NPUSlicePool{Name: "pool", NPUPoolRef: "Ascend910B", Strategy: "FixedTemplate"}
	npu := &model.NPU{Model: "Ascend910B", HCCSGroup: "Y", NodeName: "Z"}
	got := pickPoolForNPU(npu, []*model.NPUSlicePool{pool})
	require.NotNil(t, got)
	assert.Equal(t, "pool", got.Name)
}

func TestPickPoolForNPU_FallbackToFirstPool(t *testing.T) {
	pool := &model.NPUSlicePool{Name: "z-pool", NPUPoolRef: "no-match", Strategy: "FixedTemplate"}
	npu := &model.NPU{Model: "X", HCCSGroup: "Y", NodeName: "Z"}
	got := pickPoolForNPU(npu, []*model.NPUSlicePool{pool})
	require.NotNil(t, got)
	assert.Equal(t, "z-pool", got.Name)
}

func TestPickPoolForNPU_EmptyPoolsReturnsNil(t *testing.T) {
	got := pickPoolForNPU(&model.NPU{ID: "x"}, nil)
	assert.Nil(t, got)
}

func TestGenerateSlicesForNPU_UnknownStrategyEmits(t *testing.T) {
	pool := &model.NPUSlicePool{Name: "weird", Strategy: "MysteryStrategy"}
	npu := &model.NPU{ID: "n"}
	assert.Nil(t, generateSlicesForNPU(npu, pool))
}

func TestNPUSliceID_Format(t *testing.T) {
	assert.Equal(t, "n-slice-0", npuSliceID("n", 0))
	assert.Equal(t, "n-slice-12", npuSliceID("n", 12))
}

func TestResolveSlicesForNPUs_CtxCanceled(t *testing.T) {
	src := NewSourceWithClient(fakeDynamicClient(), Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := src.ResolveSlicesForNPUs(ctx, []*model.NPU{{ID: "x"}})
	require.Error(t, err)
}

func TestResolveSlicesForNPUs_StableSliceIDOrdering(t *testing.T) {
	pool := mkSlicePool("ns", "fixed-pool", "FixedTemplate", 0, 0, 0)
	client := fakeDynamicClient(pool)
	src := NewSourceWithClient(client, Options{})

	// Intentionally pass NPUs out of order; resolution should sort
	// by slice id.
	npus := []*model.NPU{
		{ID: "z-npu", NodeName: "z", SliceMode: "fixed-template"},
		{ID: "a-npu", NodeName: "a", SliceMode: "fixed-template"},
	}
	slices, err := src.ResolveSlicesForNPUs(context.Background(), npus)
	require.NoError(t, err)
	require.Len(t, slices, 2)
	assert.Equal(t, "a-npu-slice-0", slices[0].ID)
	assert.Equal(t, "z-npu-slice-0", slices[1].ID)
}
