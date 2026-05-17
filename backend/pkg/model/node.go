package model

// Node mirrors components.schemas.Node.
type Node struct {
	Name          string            `json:"name"`
	ClusterID     string            `json:"clusterId,omitempty"`
	Role          []string          `json:"role,omitempty"` // control-plane | worker | edge
	Status        string            `json:"status"`         // Ready | NotReady | Unknown
	CPU           *Quantity         `json:"cpu,omitempty"`
	Memory        *Quantity         `json:"memory,omitempty"`
	Arch          string            `json:"arch,omitempty"`
	KernelVersion string            `json:"kernelVersion,omitempty"`
	OS            string            `json:"os,omitempty"`
	KubeletVer    string            `json:"kubeletVersion,omitempty"`
	NPUCount      int               `json:"npuCount,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
}

// NodeDetail mirrors components.schemas.NodeDetail (allOf of Node + extras).
//
// The contract uses allOf; in Go we compose by embedding Node.
type NodeDetail struct {
	Node
	NUMA              []NumaNode         `json:"numa,omitempty"`
	NetworkInterfaces []NetworkInterface `json:"networkInterfaces,omitempty"`
	Storage           []StorageDevice    `json:"storage,omitempty"`
	Taints            []Taint            `json:"taints,omitempty"`
}

// NumaNode mirrors components.schemas.NumaNode.
type NumaNode struct {
	ID     int       `json:"id"`
	CPUs   []int     `json:"cpus,omitempty"`
	Memory *Quantity `json:"memory,omitempty"`
	NPUs   []string  `json:"npus,omitempty"`
}

// NetworkInterface mirrors the inline networkInterfaces[] object on NodeDetail.
type NetworkInterface struct {
	Name  string   `json:"name,omitempty"`
	MAC   string   `json:"mac,omitempty"`
	IPs   []string `json:"ips,omitempty"`
	Speed string   `json:"speed,omitempty"`
}

// StorageDevice mirrors the inline storage[] object on NodeDetail.
type StorageDevice struct {
	Device string    `json:"device,omitempty"`
	Size   *Quantity `json:"size,omitempty"`
	Type   string    `json:"type,omitempty"` // ssd | hdd | nvme
}

// Taint is a free-form taint object (contract uses `type: object` only).
type Taint = map[string]interface{}

// NodeFilter is the query filter passed to Source.ListNodes (not on the wire —
// it is a Go-side aggregate of the GET /nodes query params).
type NodeFilter struct {
	ClusterID string
	PoolName  string
}
