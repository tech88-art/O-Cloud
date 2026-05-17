package model

// Preset mirrors components.schemas.Preset (one entry in the preset catalog).
type Preset struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Kind         string              `json:"kind"`                  // inference | benchmark | inference-pd
	ModelSize    string              `json:"modelSize,omitempty"`   // "8B", "20B", ...
	Runtime      string              `json:"runtime,omitempty"`     // mindie | vllm | triton
	Requirements *PresetRequirements `json:"requirements,omitempty"`
	Description  string              `json:"description,omitempty"`
	Tags         []string            `json:"tags,omitempty"`
}

// PresetRequirements is Preset.requirements inline.
type PresetRequirements struct {
	NPUCount       int       `json:"npuCount,omitempty"`
	NPUModel       string    `json:"npuModel,omitempty"`
	VRAMMiBPerNPU  int       `json:"vramMiBPerNPU,omitempty"`
	CPU            *Quantity `json:"cpu,omitempty"`
	Memory         *Quantity `json:"memory,omitempty"`
}

// PresetDetail mirrors components.schemas.PresetDetail (allOf of Preset + extras).
type PresetDetail struct {
	Preset
	Manifest   string            `json:"manifest,omitempty"`
	Parameters []PresetParameter `json:"parameters,omitempty"`
}

// PresetParameter describes a single tunable parameter on a preset.
type PresetParameter struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Default     interface{} `json:"default,omitempty"`
	Description string      `json:"description,omitempty"`
}

// DeployRequest mirrors components.schemas.DeployRequest.
type DeployRequest struct {
	PresetID   string                 `json:"presetId"`
	Namespace  string                 `json:"namespace"`
	Name       string                 `json:"name,omitempty"`
	Replicas   int                    `json:"replicas,omitempty"`
	Scheduling *DeployScheduling      `json:"scheduling,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// DeployScheduling is DeployRequest.scheduling inline.
type DeployScheduling struct {
	Mode             string             `json:"mode,omitempty"` // auto | manual
	ManualPlacement  []ManualPlacement  `json:"manualPlacement,omitempty"`
	Affinity         *SchedulingAffinity `json:"affinity,omitempty"`
}

// ManualPlacement is one entry in DeployScheduling.manualPlacement.
type ManualPlacement struct {
	NodeName     string   `json:"nodeName,omitempty"`
	NPUSliceIDs  []string `json:"npuSliceIds,omitempty"`
}

// SchedulingAffinity is DeployScheduling.affinity inline.
type SchedulingAffinity struct {
	NUMA bool `json:"numa"`
	HCCS bool `json:"hccs"`
}

// DeployResponse mirrors components.schemas.DeployResponse.
type DeployResponse struct {
	DeployID       string   `json:"deployId"`
	WorkloadName   string   `json:"workloadName,omitempty"`
	Namespace      string   `json:"namespace,omitempty"`
	Status         string   `json:"status"` // accepted | scheduling | running | failed
	ScheduledNodes []string `json:"scheduledNodes,omitempty"`
	Message        string   `json:"message,omitempty"`
}

// ====== Metrics DTOs ======

// TimeRange is the Go-side aggregate of MetricQueryRequest.range.
type TimeRange struct {
	Start string `json:"start,omitempty"` // ISO-8601 timestamp (raw passthrough)
	End   string `json:"end,omitempty"`
	Step  string `json:"step,omitempty"` // e.g. "30s"
}

// MetricQueryResponse mirrors components.schemas.MetricQueryResponse.
type MetricQueryResponse struct {
	Result []MetricSeries `json:"result"`
}

// MetricSeries is one entry in MetricQueryResponse.result.
//
// Values is `[][]any` because the contract specifies `[timestamp, value]` —
// timestamp is float seconds, value is string per Prometheus convention.
type MetricSeries struct {
	Metric map[string]string `json:"metric"`
	Values [][]interface{}   `json:"values"`
}
