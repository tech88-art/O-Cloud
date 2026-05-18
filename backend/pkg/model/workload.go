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
type Pod struct {
	Name       string      `json:"name,omitempty"`
	Namespace  string      `json:"namespace,omitempty"`
	NodeName   string      `json:"nodeName,omitempty"`
	Status     string      `json:"status,omitempty"`
	Containers []Container `json:"containers,omitempty"`
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
