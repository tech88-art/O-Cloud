// Package crd implements a Phase 2 CRD-backed datasource.Source.
// Lands per docs/phase2-plan.md P2-T-101 (and P2-T-102 for slice
// resolution). Today's surface is ListNPUSlicePools — the only pool
// method on the Source interface; ClusterPool / NodePool / NPUPool
// reads are exposed as concrete methods for the topology aggregator
// to call after P2-T-101 (those kinds don't have a model DTO yet, so
// they're returned as []map[string]any until P2-T-102 promotes them).
//
// Architecture: we use the dynamic client rather than typed clients
// to avoid pulling the operators/pool-operator module into the
// backend's go.mod. The operators module is independent (own
// go.mod); a cross-module typed import would force a replace
// directive on every backend build. Dynamic + unstructured round-
// trip via JSON keeps the module boundary clean.
//
// Phase 3+ may revisit: when the operators module ships a vendor-
// ready binary (Phase 5 inference-operator), backend can adopt the
// typed client through a shared types-only sub-module.
package crd

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// Pool CRD GroupVersion. Mirrors
// operators/pool-operator/api/v1alpha1/groupversion_info.go:
//
//   group:   ims.ocloud.edge.example.com
//   version: v1alpha1
//
// Resource names are the conventional plural form kubebuilder
// generates (clusterpools / nodepools / npupools / npuslicepools).
const (
	poolGroup   = "ims.ocloud.edge.example.com"
	poolVersion = "v1alpha1"
)

var (
	gvrClusterPools = schema.GroupVersionResource{
		Group: poolGroup, Version: poolVersion, Resource: "clusterpools",
	}
	gvrNodePools = schema.GroupVersionResource{
		Group: poolGroup, Version: poolVersion, Resource: "nodepools",
	}
	gvrNPUPools = schema.GroupVersionResource{
		Group: poolGroup, Version: poolVersion, Resource: "npupools",
	}
	gvrNPUSlicePools = schema.GroupVersionResource{
		Group: poolGroup, Version: poolVersion, Resource: "npuslicepools",
	}
)

// Options bundles construction-time knobs. KubeconfigPath empty →
// in-cluster ServiceAccount; non-empty → loader path.
type Options struct {
	KubeconfigPath string

	// SlicePoolNamespace narrows the NPUSlicePool list to a single
	// namespace. NPUSlicePool is Namespaced (per operators/CLAUDE.md
	// §"4 级 CRD 关系"); operators are advised to put all of them in
	// `ocloud-system` until ValidatingAdmissionPolicy lands in Phase
	// 3. Empty → list across all namespaces.
	SlicePoolNamespace string
}

// Source talks to a Kubernetes apiserver via the dynamic client and
// surfaces the 4-level pool CRDs as model.NPUSlicePool (and friends).
type Source struct {
	client    dynamic.Interface
	sliceNS   string
}

// NewSource builds a production Source against a real apiserver.
// Tests should prefer NewSourceWithClient.
func NewSource(opts Options) (*Source, error) {
	cfg, err := loadRESTConfig(opts.KubeconfigPath)
	if err != nil {
		return nil, err
	}
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return NewSourceWithClient(client, opts), nil
}

// NewSourceWithClient lets unit tests pass a fake dynamic client.
func NewSourceWithClient(client dynamic.Interface, opts Options) *Source {
	return &Source{
		client:  client,
		sliceNS: opts.SlicePoolNamespace,
	}
}

// Compile-time check: *Source must satisfy datasource.Source.
var _ datasource.Source = (*Source)(nil)

// ---- Identity ----

func (s *Source) Name() string { return "crd" }

func (s *Source) Capabilities() datasource.Capabilities {
	return datasource.Capabilities{
		Pools: true,
	}
}

// ---- Stubs returning ErrCapabilityUnavailable ----

func (s *Source) ListClusters(_ context.Context) ([]*model.Cluster, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) GetCluster(_ context.Context, _ string) (*model.Cluster, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) GetTopology(_ context.Context, _, _ string) (*model.Topology, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) GetTopologyWithFabric(_ context.Context, _, _ string, _ datasource.TopologyOptions) (*model.Topology, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) ListNodes(_ context.Context, _ model.NodeFilter) ([]*model.Node, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) GetNodeDetail(_ context.Context, _ string) (*model.NodeDetail, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) ListNPUs(_ context.Context, _ string) ([]*model.NPU, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) ListWorkloads(_ context.Context, _ model.WorkloadFilter) ([]*model.Workload, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) GetWorkloadDetail(_ context.Context, _, _ string) (*model.WorkloadDetail, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) GetWorkloadLogs(_ context.Context, _, _ string, _ model.LogOptions) (*model.LogPage, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) StreamWorkloadLogs(_ context.Context, _, _ string, _ model.LogStreamOptions) (<-chan *model.LogLine, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) ListPresets(_ context.Context) ([]*model.Preset, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) GetPreset(_ context.Context, _ string) (*model.PresetDetail, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) Deploy(_ context.Context, _ *model.DeployRequest) (*model.DeployResponse, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) DeleteDeploy(_ context.Context, _ string) error {
	return datasource.ErrCapabilityUnavailable
}
func (s *Source) QueryMetric(_ context.Context, _ string, _ map[string]string, _ model.TimeRange) (*model.MetricQueryResponse, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) StreamEvents(_ context.Context, _ model.StreamEventsOptions) (<-chan *model.WSMessage, error) {
	return nil, datasource.ErrCapabilityUnavailable
}

// ---- Errors ----

// ErrResourceNotFound mirrors the k8s.Source sentinel — handlers
// chain errors.Is across sibling sources to flip a 404.
var ErrResourceNotFound = errors.New("crd: resource not found")

// loadRESTConfig is the same dual-mode loader as k8s.loadRESTConfig
// (in-cluster ↔ kubeconfig path). Duplicated rather than imported
// from the k8s package to avoid coupling the two source packages.
func loadRESTConfig(path string) (*rest.Config, error) {
	if path == "" {
		return rest.InClusterConfig()
	}
	return clientcmd.BuildConfigFromFlags("", path)
}

// translateAPIError normalises NotFound onto our sentinel.
func translateAPIError(err error) error {
	if err == nil {
		return nil
	}
	if apierrors.IsNotFound(err) {
		return ErrResourceNotFound
	}
	return err
}
