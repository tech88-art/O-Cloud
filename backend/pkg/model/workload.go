package model

import "time"

// Workload mirrors components.schemas.Workload.
type Workload struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Type      string            `json:"type"`           // inference | benchmark | training | other
	Kind      string            `json:"kind,omitempty"` // Deployment, StatefulSet, ...
	Status    string            `json:"status"`         // pending | running | succeeded | failed | unknown
	Replicas  *ReplicaStatus    `json:"replicas,omitempty"`
	NodeNames []string          `json:"nodeNames,omitempty"`
	NPUUsage  *WorkloadNPUUsage `json:"npuUsage,omitempty"`
	CreatedAt *time.Time        `json:"createdAt,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`

	// SliceBindings (P6-T-102) lists the NPU slice allocations bound
	// to this workload's pods. Sourced from the
	// `npu.huawei.com/slice-bindings` annotation Phase 5 PD Router
	// stamps onto PD-pair Pods + cross-referenced against
	// NPUSliceAllocation audit objects when available (crd source).
	//
	// **omitempty + opt-in default-off**: the list endpoint
	// `GET /api/v1/workloads` excludes this field unless the caller
	// passes `?includeSliceBindings=true`. The detail endpoint
	// `GET /api/v1/workloads/{ns}/{name}` always populates when the
	// underlying source has data.
	SliceBindings []SliceBinding `json:"sliceBindings,omitempty"`

	// O2DMSExposed (P11-T-105) is true if the workload was instantiated
	// via the O2 DMS Adapter NB endpoint. Backend determines this by
	// checking the owner ModelService for annotation
	// `dms.o2.ocloud.edge.example.com/exposed: "true"` (the O2 DMS
	// Adapter stamps it on its created ModelServices). Populated when
	// the caller passes `?includeO2DMSExposed=true` (list endpoint) or
	// always on detail endpoint when source provides it.
	O2DMSExposed *bool `json:"o2DMSExposed,omitempty"`

	// QuotaUsage (P11-T-105) summarises the workload's namespace Quota
	// consumption. Populated when `?includeQuotaUsage=true`. Joined from
	// inference-operator Quota CRD + NPUVerticalScaler scaleHistory
	// ring-buffer when crd source is configured.
	QuotaUsage *QuotaUsageSummary `json:"quotaUsage,omitempty"`

	// ScaleHistory (P11-T-105) is the rolling 10-entry scale-event log
	// for the workload's bound NPUVerticalScaler. Populated when
	// `?includeScaleHistory=true`. Empty when workload has no bound
	// scaler.
	ScaleHistory []ScaleEvent `json:"scaleHistory,omitempty"`
}

// QuotaUsageSummary (P11-T-105 · per ADR-0014 §7 forward note + Phase 11
// frontend Workload page progress bar surface). cap = Quota.spec.
// enforcement.maxSliceAllocations · used = Quota.status.usage.current
// SliceAllocations · remaining = max(0, cap - used).
type QuotaUsageSummary struct {
	Cap                 int32  `json:"cap"`
	Used                int32  `json:"used"`
	Remaining           int32  `json:"remaining"`
	ScaleEventsLast5min int32  `json:"scaleEventsLast5min,omitempty"`
	RateAlgorithm       string `json:"rateAlgorithm,omitempty"` // tokenBucket | slidingWindow
}

// ScaleEvent (P11-T-105 · per ADR-0012 §5 NPUVerticalScaler.scaleHistory
// forward note + Phase 11 frontend ECharts timeline surface). One entry
// of NPUVerticalScaler.status.scaleHistory rolling ring-buffer.
type ScaleEvent struct {
	Timestamp    time.Time `json:"timestamp"`
	Direction    string    `json:"direction"` // up | down
	FromTemplate string    `json:"fromTemplate,omitempty"`
	ToTemplate   string    `json:"toTemplate,omitempty"`
	Reason       string    `json:"reason,omitempty"`
}

// SliceBinding mirrors components.schemas.SliceBinding (P6-T-102).
//
// Each entry corresponds to one NPU slice allocation observed for a
// pod of the workload. Format mirrors the
// `npu.huawei.com/slice-bindings` annotation
// (`<node>/<pool>/<device>:<aiCores>`) the Phase 5 PD Router webhook
// writes, with each component split into its own struct field for
// easier frontend rendering.
type SliceBinding struct {
	// PodName identifies the pod the binding originated from. Useful
	// for grouping (frontend Workloads page T103 renders per-pod
	// chips).
	PodName string `json:"podName,omitempty"`

	// NodeName is the node hosting the device.
	NodeName string `json:"nodeName"`

	// Pool is the ResourcePool name (by convention = nodeName per
	// npu-dra-driver Phase 5 publisher).
	Pool string `json:"pool,omitempty"`

	// Device is the device identifier inside the pool (e.g.
	// `worker-site-a-01-npu-0`).
	Device string `json:"device"`

	// AICores is the allocated AI-core count for this slice.
	AICores int32 `json:"aiCores,omitempty"`

	// Role optionally identifies the PD-pair side (prefill / decode)
	// when the binding came from a ModelService Pod.
	Role string `json:"role,omitempty"`
}

// ReplicaStatus mirrors Workload.replicas inline.
type ReplicaStatus struct {
	Desired int `json:"desired"`
	Ready   int `json:"ready"`
}

// WorkloadNPUUsage mirrors Workload.npuUsage inline.
type WorkloadNPUUsage struct {
	Allocated int      `json:"allocated"`
	Slices    []string `json:"slices,omitempty"`
}

// WorkloadDetail mirrors components.schemas.WorkloadDetail (allOf of Workload + extras).
type WorkloadDetail struct {
	Workload
	Pods      []Pod                  `json:"pods,omitempty"`
	Relations []PodRelation          `json:"relations,omitempty"`
	Spec      map[string]interface{} `json:"spec,omitempty"`
}

// Pod mirrors components.schemas.Pod.
//
// ADR-0006: Bindings field promoted from the aggregator-local mirror
// (aggregator.WorkloadInput.Pod.Bindings) into the public DTO. The
// mock fixture (configs/mock-data/set-a-small/workloads.json) has been
// emitting these via T013 since W2; only the contract + DTO sides were
// out of sync.
type Pod struct {
	Name       string       `json:"name,omitempty"`
	Namespace  string       `json:"namespace,omitempty"`
	NodeName   string       `json:"nodeName,omitempty"`
	Status     string       `json:"status,omitempty"`
	Containers []Container  `json:"containers,omitempty"`
	Bindings   []PodBinding `json:"bindings,omitempty"`
}

// PodBinding mirrors components.schemas.Pod.bindings[] (ADR-0006).
//
// Each entry records that a specific pod consumes a specific NPU slice
// in a known role (prefill / decode / primary / sidecar / init / peer)
// at a known position within the pod's container array. The aggregator
// uses these to emit `binds-to` edges in the workload-fused topology
// (ADR-0005); the Workloads Drawer surfaces them as the slice list per
// pod card.
type PodBinding struct {
	SliceID    string `json:"sliceId"`
	Role       string `json:"role,omitempty"`
	IndexInPod int    `json:"indexInPod,omitempty"`
}

// Container is the inline containers[] object on Pod.
type Container struct {
	Name      string              `json:"name,omitempty"`
	Image     string              `json:"image,omitempty"`
	Command   []string            `json:"command,omitempty"`
	Args      []string            `json:"args,omitempty"`
	Resources *ContainerResources `json:"resources,omitempty"`
}

// ContainerResources is Container.resources.
type ContainerResources struct {
	CPU       string   `json:"cpu,omitempty"`
	Memory    string   `json:"memory,omitempty"`
	NPUSlices []string `json:"npuSlices,omitempty"`
}

// PodRelation describes Pod-to-Pod relations (e.g. prefill→decode for PD
// disaggregated inference).
type PodRelation struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"` // pd-pair | sidecar | init | peer
}

// WorkloadFilter is the Go-side aggregate of GET /workloads query params.
type WorkloadFilter struct {
	Namespace string
	Type      string
	Status    string

	// IncludeSliceBindings (P6-T-102) opts in to populating
	// Workload.SliceBindings on each returned entry. Default false —
	// keeps the list-endpoint response shape unchanged for callers
	// that don't ask for the data.
	IncludeSliceBindings bool

	// IncludeO2DMSExposed (P11-T-105) opts in to populating
	// Workload.O2DMSExposed. Mirror of api-contract.yaml
	// ?includeO2DMSExposed query param.
	IncludeO2DMSExposed bool

	// IncludeQuotaUsage (P11-T-105) opts in to populating
	// Workload.QuotaUsage. Mirror of api-contract.yaml
	// ?includeQuotaUsage query param.
	IncludeQuotaUsage bool

	// IncludeScaleHistory (P11-T-105) opts in to populating
	// Workload.ScaleHistory. Mirror of api-contract.yaml
	// ?includeScaleHistory query param.
	IncludeScaleHistory bool
}

// LogOptions is the Go-side aggregate of GET /workloads/.../logs query params.
type LogOptions struct {
	Container string
	Tail      int
	Since     string // ISO-8601, raw passthrough
}

// LogStreamOptions is the Go-side aggregate of /ws/logs/... query params
// (P1-T-301). Mirrors LogOptions but without Tail — streaming is forward
// only; the REST endpoint covers historical tail.
type LogStreamOptions struct {
	Container string
	Since     string // ISO-8601, raw passthrough
}

// LogPage mirrors components.schemas.LogPage.
type LogPage struct {
	Lines      []LogLine `json:"lines"`
	HasMore    bool      `json:"hasMore"`
	NextCursor *string   `json:"nextCursor,omitempty"`
}

// LogLine is one entry in LogPage.lines.
type LogLine struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level,omitempty"`
	Container string    `json:"container,omitempty"`
	Message   string    `json:"message"`
}
