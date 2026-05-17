package model

import "time"

// Cluster mirrors components.schemas.Cluster.
type Cluster struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Role              string            `json:"role,omitempty"` // edge-single | small-cluster | multi-site-member
	Location          string            `json:"location,omitempty"`
	Status            string            `json:"status"` // healthy | degraded | unreachable
	KubernetesVersion string            `json:"kubernetesVersion,omitempty"`
	NodeCount         int               `json:"nodeCount,omitempty"`
	NPUCount          int               `json:"npuCount,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	CreatedAt         *time.Time        `json:"createdAt,omitempty"`
}

// Topology mirrors components.schemas.Topology.
type Topology struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
	Meta  *TopologyMeta  `json:"meta,omitempty"`
}

// TopologyMeta is the inline meta object on Topology.
type TopologyMeta struct {
	ClusterID   string     `json:"clusterId,omitempty"`
	GeneratedAt *time.Time `json:"generatedAt,omitempty"`
}

// TopologyNode mirrors components.schemas.TopologyNode.
//
// Attributes is intentionally typed as map[string]any — its keys depend on
// TopologyNode.Type (see contract for the node/npu/slice keys).
type TopologyNode struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"` // cluster | nodepool | node | npu | slice | network
	Label      string                 `json:"label"`
	Status     string                 `json:"status,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// TopologyEdge mirrors components.schemas.TopologyEdge.
type TopologyEdge struct {
	Source     string                 `json:"source"`
	Target     string                 `json:"target"`
	Type       string                 `json:"type"` // contains | hccs | network | allocated
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}
