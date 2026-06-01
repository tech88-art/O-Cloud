// Package model contains DTOs that mirror configs/mock-data/schema.json $defs.
//
// Naming and JSON tags must match the schema 1:1 (camelCase). Each top-level
// type corresponds to one JSON file written by pkg/writer.
package model

// Quantity matches $defs/Quantity.
type Quantity struct {
	Raw   string `json:"raw"`
	Bytes *int64 `json:"bytes,omitempty"`
}

// NumaNode matches $defs/NumaNode.
type NumaNode struct {
	ID     int       `json:"id"`
	CPUs   []int     `json:"cpus,omitempty"`
	Memory *Quantity `json:"memory,omitempty"`
	NPUs   []string  `json:"npus,omitempty"`
}

// NetworkInterface matches $defs/NetworkInterface.
type NetworkInterface struct {
	Name  string   `json:"name"`
	MAC   string   `json:"mac,omitempty"`
	IPs   []string `json:"ips,omitempty"`
	Speed string   `json:"speed,omitempty"`
}

// StorageDevice matches $defs/StorageDevice.
type StorageDevice struct {
	Device string   `json:"device"`
	Size   Quantity `json:"size"`
	Type   string   `json:"type"`
}

// Cluster matches $defs/Cluster.
type Cluster struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Role              string            `json:"role"`
	Location          string            `json:"location,omitempty"`
	Status            string            `json:"status"`
	KubernetesVersion string            `json:"kubernetesVersion,omitempty"`
	NodeCount         int               `json:"nodeCount,omitempty"`
	NPUCount          int               `json:"npuCount,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	CreatedAt         string            `json:"createdAt,omitempty"`
}

// Node matches $defs/Node.
type Node struct {
	Name              string             `json:"name"`
	ClusterID         string             `json:"clusterId"`
	Role              []string           `json:"role,omitempty"`
	Status            string             `json:"status"`
	CPU               Quantity           `json:"cpu"`
	Memory            Quantity           `json:"memory"`
	Arch              string             `json:"arch"`
	KernelVersion     string             `json:"kernelVersion,omitempty"`
	OS                string             `json:"os,omitempty"`
	KubeletVersion    string             `json:"kubeletVersion,omitempty"`
	NPUCount          int                `json:"npuCount,omitempty"`
	NUMA              []NumaNode         `json:"numa,omitempty"`
	NetworkInterfaces []NetworkInterface `json:"networkInterfaces,omitempty"`
	Storage           []StorageDevice    `json:"storage,omitempty"`
	Labels            map[string]string  `json:"labels,omitempty"`
}

// NPUUsage matches $defs/NPUUsage.
type NPUUsage struct {
	AICoreUtilization          float64  `json:"aiCoreUtilization,omitempty"`
	VRAMUtilization            float64  `json:"vramUtilization,omitempty"`
	VRAMUsedMiB                int      `json:"vramUsedMiB,omitempty"`
	MemoryBandwidthUtilization *float64 `json:"memoryBandwidthUtilization,omitempty"`
	Temperature                *float64 `json:"temperature,omitempty"`
	Power                      *float64 `json:"power,omitempty"`
}

// NPU matches $defs/NPU.
type NPU struct {
	ID          string    `json:"id"`
	NodeName    string    `json:"nodeName"`
	Model       string    `json:"model"`
	Index       int       `json:"index"`
	VRAMMiB     int       `json:"vramMiB,omitempty"`
	AICoreTotal int       `json:"aiCoreTotal,omitempty"`
	NumaNode    int       `json:"numaNode"`
	HCCSGroup   string    `json:"hccsGroup,omitempty"`
	// ADR-0021 拓扑全保真: host↔NPU PCIe 带宽 (GB/s) + 同组 npu↔npu HCCS 带宽 (GB/s).
	PCIeBandwidthGBps float64   `json:"pcieBandwidthGBps,omitempty"`
	HCCSBandwidthGBps float64   `json:"hccsBandwidthGBps,omitempty"`
	Status            string    `json:"status"`
	SliceMode         string    `json:"sliceMode"`
	Usage             *NPUUsage `json:"usage,omitempty"`
}

// AllocatedTo matches the inline shape of $defs/Slice.allocatedTo.
type AllocatedTo struct {
	Namespace     string `json:"namespace,omitempty"`
	PodName       string `json:"podName,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
}

// Slice matches $defs/Slice.
type Slice struct {
	ID          string       `json:"id"`
	ParentNPU   string       `json:"parentNPU"`
	Template    string       `json:"template,omitempty"`
	AICore      int          `json:"aiCore"`
	VRAMMiB     int          `json:"vramMiB"`
	Status      string       `json:"status"`
	AllocatedTo *AllocatedTo `json:"allocatedTo,omitempty"`
	Usage       *NPUUsage    `json:"usage,omitempty"`
}

// WorkloadReplicas matches $defs/Workload.replicas.
type WorkloadReplicas struct {
	Desired int `json:"desired"`
	Ready   int `json:"ready"`
}

// WorkloadNPUUsage matches $defs/Workload.npuUsage.
type WorkloadNPUUsage struct {
	Allocated int      `json:"allocated"`
	Slices    []string `json:"slices,omitempty"`
}

// ContainerResources matches $defs/Pod.containers[].resources.
type ContainerResources struct {
	CPU       string   `json:"cpu,omitempty"`
	Memory    string   `json:"memory,omitempty"`
	NPUSlices []string `json:"npuSlices,omitempty"`
}

// Container matches $defs/Pod.containers[].
type Container struct {
	Name      string              `json:"name"`
	Image     string              `json:"image"`
	Command   []string            `json:"command,omitempty"`
	Args      []string            `json:"args,omitempty"`
	Resources *ContainerResources `json:"resources,omitempty"`
}

// Pod matches $defs/Pod.
type Pod struct {
	Name       string       `json:"name"`
	Namespace  string       `json:"namespace"`
	NodeName   string       `json:"nodeName,omitempty"`
	Status     string       `json:"status"`
	Containers []Container  `json:"containers,omitempty"`
	Bindings   []PodBinding `json:"bindings,omitempty"`
}

// PodBinding matches $defs/Pod.bindings[] (T013 schema add, ADR-0006
// surfaced through OpenAPI). One entry per (pod, slice) consumed slot.
type PodBinding struct {
	SliceID    string `json:"sliceId"`
	Role       string `json:"role,omitempty"`
	IndexInPod int    `json:"indexInPod,omitempty"`
}

// NetworkSwitch matches $defs/NetworkSwitch (ADR-0004 inter-node fabric).
type NetworkSwitch struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Type          string   `json:"type"` // tor | leaf | spine | access
	Location      string   `json:"location,omitempty"`
	PortsTotal    int      `json:"portsTotal,omitempty"`
	PortsUsed     int      `json:"portsUsed,omitempty"`
	BandwidthGbps int      `json:"bandwidthGbps,omitempty"`
	VLANs         []string `json:"vlans,omitempty"`
	Status        string   `json:"status"` // up | degraded | down
}

// NetworkLink matches $defs/NetworkLink (ADR-0004).
type NetworkLink struct {
	ID            string  `json:"id"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	BandwidthGbps int     `json:"bandwidthGbps"`           // ADR-0004 legacy unit (Gbps · gigabits)
	BandwidthGBps float64 `json:"bandwidthGBps,omitempty"` // ADR-0021 unit (GB/s · gigabytes) · network/hccs/fabric hover
	Medium        string  `json:"medium,omitempty"`        // copper | fiber | dac | optical | eth | roce | ib
	Utilization   float64 `json:"utilization,omitempty"`   // 0-100
	RTTUs         float64 `json:"rttUs,omitempty"`
}

// Relation matches $defs/Workload.relations[].
type Relation struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// Percentile matches $defs/Percentile.
type Percentile struct {
	P50 float64 `json:"p50,omitempty"`
	P90 float64 `json:"p90,omitempty"`
	P99 float64 `json:"p99,omitempty"`
}

// Throughput matches $defs/WorkloadMetrics.throughput.
type Throughput struct {
	InputTokensPerSec  float64 `json:"inputTokensPerSec,omitempty"`
	OutputTokensPerSec float64 `json:"outputTokensPerSec,omitempty"`
	RequestsPerSec     float64 `json:"requestsPerSec,omitempty"`
}

// Latency matches $defs/WorkloadMetrics.latency.
type Latency struct {
	TTFTMs *Percentile `json:"ttftMs,omitempty"`
	ITLMs  *Percentile `json:"itlMs,omitempty"`
	E2EMs  *Percentile `json:"e2eMs,omitempty"`
}

// WorkloadResourceUsage matches $defs/WorkloadMetrics.resourceUsage.
type WorkloadResourceUsage struct {
	CPUUtilization    float64 `json:"cpuUtilization,omitempty"`
	MemoryUtilization float64 `json:"memoryUtilization,omitempty"`
	NPUUtilization    float64 `json:"npuUtilization,omitempty"`
	VRAMUtilization   float64 `json:"vramUtilization,omitempty"`
}

// WorkloadMetrics matches $defs/WorkloadMetrics.
type WorkloadMetrics struct {
	Throughput    *Throughput            `json:"throughput,omitempty"`
	Latency       *Latency               `json:"latency,omitempty"`
	ResourceUsage *WorkloadResourceUsage `json:"resourceUsage,omitempty"`
}

// Workload matches $defs/Workload.
type Workload struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Type      string            `json:"type"`
	Kind      string            `json:"kind"`
	Status    string            `json:"status"`
	Replicas  *WorkloadReplicas `json:"replicas,omitempty"`
	NodeNames []string          `json:"nodeNames,omitempty"`
	NPUUsage  *WorkloadNPUUsage `json:"npuUsage,omitempty"`
	Pods      []Pod             `json:"pods,omitempty"`
	Relations []Relation        `json:"relations,omitempty"`
	Spec      map[string]any    `json:"spec,omitempty"`
	Metrics   *WorkloadMetrics  `json:"metrics,omitempty"`
	CreatedAt string            `json:"createdAt,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// ClusterPoolMember matches a single entry in $defs/ClusterPool.clusters.
type ClusterPoolMember struct {
	ClusterID  string `json:"clusterId,omitempty"`
	SyncPolicy string `json:"syncPolicy,omitempty"`
}

// ClusterPoolStatus matches $defs/ClusterPool.status.
type ClusterPoolStatus struct {
	HealthyClusters   []string `json:"healthyClusters,omitempty"`
	UnhealthyClusters []string `json:"unhealthyClusters,omitempty"`
}

// ClusterPool matches $defs/ClusterPool.
type ClusterPool struct {
	Name     string              `json:"name"`
	Clusters []ClusterPoolMember `json:"clusters,omitempty"`
	Status   *ClusterPoolStatus  `json:"status,omitempty"`
}

// NodePoolStatus matches $defs/NodePool.status.
type NodePoolStatus struct {
	Nodes       []string  `json:"nodes,omitempty"`
	TotalCPU    *Quantity `json:"totalCPU,omitempty"`
	TotalMemory *Quantity `json:"totalMemory,omitempty"`
}

// NodePool matches $defs/NodePool.
type NodePool struct {
	Name           string          `json:"name"`
	ClusterPoolRef string          `json:"clusterPoolRef,omitempty"`
	Role           string          `json:"role"`
	Location       string          `json:"location,omitempty"`
	Selector       map[string]any  `json:"selector,omitempty"`
	Status         *NodePoolStatus `json:"status,omitempty"`
}

// HCCSGroup matches $defs/NPUPool.status.hccsTopology.groups[].
type HCCSGroup struct {
	ID       string   `json:"id"`
	NPUs     []string `json:"npus"`
	Topology string   `json:"topology,omitempty"`
}

// HCCSTopology matches $defs/NPUPool.status.hccsTopology.
type HCCSTopology struct {
	Groups []HCCSGroup `json:"groups,omitempty"`
}

// NPUPoolStatus matches $defs/NPUPool.status.
type NPUPoolStatus struct {
	TotalNPUs     int           `json:"totalNPUs"`
	HealthyNPUs   int           `json:"healthyNPUs"`
	AllocatedNPUs int           `json:"allocatedNPUs"`
	HCCSTopology  *HCCSTopology `json:"hccsTopology,omitempty"`
}

// NPUPool matches $defs/NPUPool.
type NPUPool struct {
	Name          string         `json:"name"`
	NodePoolRef   string         `json:"nodePoolRef,omitempty"`
	NPUModel      string         `json:"npuModel"`
	SliceStrategy string         `json:"sliceStrategy,omitempty"`
	Status        *NPUPoolStatus `json:"status,omitempty"`
}

// FixedTemplate matches $defs/NPUSlicePool.fixedTemplates[].
type FixedTemplate struct {
	Name        string `json:"name"`
	AICoreCount int    `json:"aiCoreCount"`
	MemoryMiB   int    `json:"memoryMiB"`
}

// DynamicSlicing matches $defs/NPUSlicePool.dynamicSlicing.
type DynamicSlicing struct {
	MinAICore            int  `json:"minAICore"`
	MaxAICore            int  `json:"maxAICore"`
	MemoryGranularityMiB int  `json:"memoryGranularityMiB"`
	AllowAggregation     bool `json:"allowAggregation"`
}

// NPUSlicePoolStatus matches $defs/NPUSlicePool.status.
type NPUSlicePoolStatus struct {
	TotalSlices     int `json:"totalSlices"`
	AllocatedSlices int `json:"allocatedSlices"`
	AvailableSlices int `json:"availableSlices"`
}

// NPUSlicePool matches $defs/NPUSlicePool.
type NPUSlicePool struct {
	Name           string              `json:"name"`
	NPUPoolRef     string              `json:"npuPoolRef,omitempty"`
	Strategy       string              `json:"strategy"`
	FixedTemplates []FixedTemplate     `json:"fixedTemplates,omitempty"`
	DynamicSlicing *DynamicSlicing     `json:"dynamicSlicing,omitempty"`
	Status         *NPUSlicePoolStatus `json:"status,omitempty"`
}

// PresetRequirements matches $defs/Preset.requirements.
type PresetRequirements struct {
	NPUCount      int       `json:"npuCount,omitempty"`
	NPUModel      string    `json:"npuModel,omitempty"`
	VRAMMiBPerNPU int       `json:"vramMiBPerNPU,omitempty"`
	CPU           *Quantity `json:"cpu,omitempty"`
	Memory        *Quantity `json:"memory,omitempty"`
}

// Preset matches $defs/Preset.
type Preset struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Kind         string             `json:"kind"`
	ModelSize    string             `json:"modelSize,omitempty"`
	Runtime      string             `json:"runtime"`
	Requirements PresetRequirements `json:"requirements"`
	Description  string             `json:"description,omitempty"`
	Tags         []string           `json:"tags,omitempty"`
	Manifest     string             `json:"manifest,omitempty"`
}

// Event matches $defs/Event.
type Event struct {
	Timestamp string         `json:"timestamp"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
}

// Meta matches the top-level meta object.
type Meta struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	GeneratedAt string `json:"generatedAt"`
	Description string `json:"description,omitempty"`
	Scenario    string `json:"scenario,omitempty"`
}

// Pools is the top-level pools object (required: cluster/node/npu/npuSlice).
type Pools struct {
	Cluster  []ClusterPool  `json:"cluster"`
	Node     []NodePool     `json:"node"`
	NPU      []NPUPool      `json:"npu"`
	NPUSlice []NPUSlicePool `json:"npuSlice"`
}

// Dataset is the in-memory model of one set-*. Writer fans this out into
// JSON files (one per top-level section + a combined set.json).
//
// The fabric arrays (NetworkSwitches / NetworkLinks) are emitted to
// networkSwitches.json + networkLinks.json respectively. They're
// optional in the schema — when both are empty the files still exist
// (empty arrays) so the backend mock loader's sync.Once paths don't
// have to branch on file presence.
type Dataset struct {
	Meta            Meta            `json:"meta"`
	Clusters        []Cluster       `json:"clusters"`
	Nodes           []Node          `json:"nodes"`
	NPUs            []NPU           `json:"npus"`
	Slices          []Slice         `json:"slices"`
	Workloads       []Workload      `json:"workloads"`
	Pools           Pools           `json:"pools"`
	Presets         []Preset        `json:"presets"`
	Events          []Event         `json:"events"`
	NetworkSwitches []NetworkSwitch `json:"networkSwitches,omitempty"`
	NetworkLinks    []NetworkLink   `json:"networkLinks,omitempty"`
}
