package model

// NPU mirrors components.schemas.NPU.
type NPU struct {
	ID          string `json:"id"`
	NodeName    string `json:"nodeName,omitempty"`
	Model       string `json:"model"`
	Index       int    `json:"index,omitempty"`
	VRAMMiB     int    `json:"vramMiB,omitempty"`
	AICoreTotal int    `json:"aiCoreTotal,omitempty"`
	NumaNode    int    `json:"numaNode,omitempty"`
	HCCSGroup   string `json:"hccsGroup,omitempty"`
	// PCIeBandwidthGBps is the host↔NPU PCIe link bandwidth in GB/s
	// (gigabytes/second). ADR-0021 Track B 拓扑全保真. Pointer so absent in a
	// fixture round-trips as nil (the topology aggregator only stamps the
	// `pcieBandwidthGBps` node attribute when non-nil → byte-equivalent for
	// pre-ADR-0021 fixtures). PCIe Gen4 x16 ≈ 32 · Gen5 x16 ≈ 64.
	PCIeBandwidthGBps *float64 `json:"pcieBandwidthGBps,omitempty"`
	// HCCSBandwidthGBps is the per-NPU HCCS link bandwidth in GB/s. ADR-0021:
	// the topology aggregator emits `hccs` edges between same-node same-
	// hccsGroup NPUs, stamping this value as the edge's bandwidthGBps. Pointer
	// for the same absent→nil round-trip reason. 910B HCCS ≈ 56 GB/s.
	HCCSBandwidthGBps *float64   `json:"hccsBandwidthGBps,omitempty"`
	Status            string     `json:"status"`              // healthy | degraded | faulty | offline
	SliceMode         string     `json:"sliceMode,omitempty"` // whole | fixed-template | dynamic
	Slices            []NPUSlice `json:"slices,omitempty"`
	Usage             *NPUUsage  `json:"usage,omitempty"`
}

// NPUSlice mirrors components.schemas.NPUSlice.
type NPUSlice struct {
	ID          string           `json:"id"`
	ParentNPU   string           `json:"parentNPU,omitempty"`
	Template    string           `json:"template,omitempty"`
	AICore      int              `json:"aiCore,omitempty"`
	VRAMMiB     int              `json:"vramMiB,omitempty"`
	Status      string           `json:"status"` // available | allocated | faulty
	AllocatedTo *SliceAllocation `json:"allocatedTo,omitempty"`
	Usage       *NPUUsage        `json:"usage,omitempty"`
}

// SliceAllocation mirrors NPUSlice.allocatedTo (inline object, nullable in
// contract). Zero value = unallocated.
type SliceAllocation struct {
	Namespace     string `json:"namespace,omitempty"`
	PodName       string `json:"podName,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
}

// NPUUsage mirrors components.schemas.NPUUsage. The nullable fields are
// pointers so contract `nullable: true` round-trips correctly.
type NPUUsage struct {
	AICoreUtilization          float64  `json:"aiCoreUtilization,omitempty"`
	VRAMUtilization            float64  `json:"vramUtilization,omitempty"`
	VRAMUsedMiB                int      `json:"vramUsedMiB,omitempty"`
	MemoryBandwidthUtilization *float64 `json:"memoryBandwidthUtilization,omitempty"`
	Temperature                *float64 `json:"temperature,omitempty"`
	Power                      *float64 `json:"power,omitempty"`
}

// ====== Pool DTOs (NPUSlicePool / NPUPool / NodePool / ClusterPool) ======

// NPUSlicePool mirrors components.schemas.NPUSlicePool.
type NPUSlicePool struct {
	Name           string                `json:"name"`
	Namespace      string                `json:"namespace,omitempty"`
	NPUPoolRef     string                `json:"npuPoolRef,omitempty"`
	Strategy       string                `json:"strategy"` // FixedTemplate | Dynamic
	FixedTemplates []FixedTemplate       `json:"fixedTemplates,omitempty"`
	DynamicSlicing *DynamicSlicingPolicy `json:"dynamicSlicing,omitempty"`
	Status         *NPUSlicePoolStatus   `json:"status,omitempty"`
}

// FixedTemplate is a single entry in NPUSlicePool.fixedTemplates.
type FixedTemplate struct {
	Name        string `json:"name,omitempty"`
	AICoreCount int    `json:"aiCoreCount,omitempty"`
	MemoryMiB   int    `json:"memoryMiB,omitempty"`
}

// DynamicSlicingPolicy is NPUSlicePool.dynamicSlicing.
type DynamicSlicingPolicy struct {
	MinAICore            int  `json:"minAICore,omitempty"`
	MaxAICore            int  `json:"maxAICore,omitempty"`
	MemoryGranularityMiB int  `json:"memoryGranularityMiB,omitempty"`
	AllowAggregation     bool `json:"allowAggregation,omitempty"`
}

// NPUSlicePoolStatus is NPUSlicePool.status.
type NPUSlicePoolStatus struct {
	TotalSlices     int `json:"totalSlices,omitempty"`
	AllocatedSlices int `json:"allocatedSlices,omitempty"`
	AvailableSlices int `json:"availableSlices,omitempty"`
}
