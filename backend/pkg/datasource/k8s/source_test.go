// Package k8s — unit tests for P2-T-001 (skeleton + clusters + nodes).
//
// Tests construct the Source via NewSourceWithClient + the fake
// clientset shipped in k8s.io/client-go. No apiserver / kubeconfig
// involvement. Verifies:
//   - Capabilities: Clusters + Nodes ON; everything else OFF.
//   - Methods declared OFF return datasource.ErrCapabilityUnavailable
//     (handler-side error.Is contract).
package k8s

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

func TestNewSourceWithClient_Capabilities_AfterT004(t *testing.T) {
	src := NewSourceWithClient(fake.NewSimpleClientset(), Options{})
	caps := src.Capabilities()

	// P2-T-001..005 + P2-T-004 turn on Clusters / Nodes / NPUs /
	// Workloads / Logs / Events.
	assert.True(t, caps.Clusters, "P2-T-001 turns Clusters ON")
	assert.True(t, caps.Nodes, "P2-T-001 turns Nodes ON")
	assert.True(t, caps.NPUs, "P2-T-002 turns NPUs ON")
	assert.True(t, caps.Workloads, "P2-T-003 turns Workloads ON")
	assert.True(t, caps.Logs, "P2-T-005 turns Logs ON")
	assert.True(t, caps.Events, "P2-T-004 turns Events ON")

	// Everything else stays OFF until its dedicated P2-T-00x lands.
	assert.False(t, caps.Topology, "Topology lands with later aggregator wiring")
	assert.False(t, caps.Pools, "Pools land with P2-T-101")
	assert.False(t, caps.Presets, "Presets land with P2-T-103")
	assert.False(t, caps.Deploy, "Deploy stays mock-only in Phase 2")
	assert.False(t, caps.Metrics, "Metrics land with P2-T-007 (prometheus.Source)")
}

func TestSource_SatisfiesDatasourceSource(t *testing.T) {
	// Compile-time check is already in source.go (`var _ datasource.Source`)
	// but exercise it at runtime to catch interface drift if someone adds
	// a new method.
	var _ datasource.Source = NewSourceWithClient(fake.NewSimpleClientset(), Options{})
}

func TestStubMethods_ReturnErrCapabilityUnavailable(t *testing.T) {
	src := NewSourceWithClient(fake.NewSimpleClientset(), Options{})
	ctx := context.Background()

	// NOTE (P13-T-103): GetTopology / GetTopologyWithFabric are NO LONGER
	// stubs — they now build the real cluster→node→npu HCCS graph from Nodes +
	// ResourceSlice attributes (topology.go). Their behaviour is covered by
	// topology_test.go; they were removed from this stub list when the real
	// body landed (ADR-0024 §2 Decision C).
	//
	// NOTE (P13-T-104): Deploy / DeleteDeploy are likewise NO LONGER stubs —
	// they apply / delete a Deployment+Service on the apiserver (deploy.go,
	// ADR-0024 §2 Decision E). Covered by deploy_test.go; removed from this stub
	// list when the real body landed. ListPresets / GetPreset stay stubs here
	// (presets are served by the configmap source, not the k8s source).
	tests := []struct {
		name string
		do   func() error
	}{
		{"ListNPUSlicePools", func() error {
			_, err := src.ListNPUSlicePools(ctx)
			return err
		}},
		{"ListPresets", func() error {
			_, err := src.ListPresets(ctx)
			return err
		}},
		{"GetPreset", func() error {
			_, err := src.GetPreset(ctx, "id")
			return err
		}},
		{"QueryMetric", func() error {
			_, err := src.QueryMetric(ctx, "t", nil, model.TimeRange{})
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.do()
			assert.True(t, errors.Is(err, datasource.ErrCapabilityUnavailable),
				"expected ErrCapabilityUnavailable, got: %v", err)
		})
	}
}

func TestName_IsK8s(t *testing.T) {
	src := NewSourceWithClient(fake.NewSimpleClientset(), Options{})
	assert.Equal(t, "k8s", src.Name())
}
