// preset_small builds the set-a-small dataset: 1 cluster / 3 nodes /
// 24 NPUs / mixed workloads. See configs/CLAUDE.md §3.4.
//
// Determinism: gofakeit is seeded with seedSmall, and t0 is fixed at
// 2026-05-17T12:00:00Z. Two consecutive runs produce identical files.
package main

import (
	"fmt"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/example/ocloud-edge/configs/mock-data/generator/pkg/builder"
	"github.com/example/ocloud-edge/configs/mock-data/generator/pkg/model"
)

const (
	seedSmall = int64(42)
	// t0Small is the fixed anchor for createdAt/event timestamps. Two
	// generator runs produce byte-identical output.
	t0SmallRFC = "2026-05-17T12:00:00Z"
)

func buildSetASmall() *model.Dataset {
	gofakeit.Seed(seedSmall)
	t0, _ := time.Parse(time.RFC3339, t0SmallRFC)

	const clusterID = "cluster-prod-a-01"
	const clusterName = "cluster-prod-a-01"

	nodeNames := []string{"worker-site-a-01", "worker-site-a-02", "worker-site-a-03"}

	// ---- clusters ----
	clusters := []model.Cluster{
		{
			ID:                clusterID,
			Name:              clusterName,
			Role:              "edge-single",
			Location:          "site-a-shanghai",
			Status:            "healthy",
			KubernetesVersion: "v1.31.0",
			NodeCount:         len(nodeNames),
			NPUCount:          len(nodeNames) * builder.NPUsPerNode,
			Labels: map[string]string{
				"site":        "a",
				"environment": "prod",
				"region":      "cn-east-1",
			},
			CreatedAt: builder.SkewCreatedAt(t0, 28, 4),
		},
	}

	// ---- nodes ----
	nodes := make([]model.Node, 0, len(nodeNames))
	for i, name := range nodeNames {
		numa := make([]model.NumaNode, builder.NumaPerNode)
		for n := 0; n < builder.NumaPerNode; n++ {
			numa[n] = model.NumaNode{
				ID:   n,
				CPUs: rangeInts(n*48, 48),
				Memory: &model.Quantity{
					Raw: "384Gi",
				},
				NPUs: builder.NPUsForNUMA(name, n),
			}
		}
		nodes = append(nodes, model.Node{
			Name:           name,
			ClusterID:      clusterID,
			Role:           []string{"worker"},
			Status:         "Ready",
			CPU:            model.Quantity{Raw: "96"},
			Memory:         model.Quantity{Raw: "768Gi"},
			Arch:           "amd64",
			KernelVersion:  "5.15.0-105-generic",
			OS:             "Ubuntu 22.04.4 LTS",
			KubeletVersion: "v1.31.0",
			NPUCount:       builder.NPUsPerNode,
			NUMA:           numa,
			NetworkInterfaces: []model.NetworkInterface{
				{
					Name:  "eth0",
					MAC:   fmt.Sprintf("aa:bb:cc:dd:ee:%02x", i*2),
					IPs:   []string{fmt.Sprintf("10.0.10.%d", 11+i)},
					Speed: "25Gbps",
				},
				{
					Name:  "eth1",
					MAC:   fmt.Sprintf("aa:bb:cc:dd:ee:%02x", i*2+1),
					IPs:   []string{fmt.Sprintf("10.0.20.%d", 11+i)},
					Speed: "25Gbps",
				},
			},
			Storage: []model.StorageDevice{
				{
					Device: "/dev/nvme0n1",
					Size:   model.Quantity{Raw: "2Ti"},
					Type:   "nvme",
				},
			},
			Labels: map[string]string{
				"node.kubernetes.io/instance-type": "atlas-800t-a2",
				"topology.kubernetes.io/zone":      fmt.Sprintf("zone-%d", i+1),
				"npu.huawei.com/model":             "Ascend910B",
			},
		})
	}

	// ---- NPUs ----
	// Per-NPU sliceMode pattern drives realism: per node, NPU 0..1 whole,
	// 2..5 fixed-template, 6..7 dynamic. Utilisation distribution covers
	// idle / light / busy / hot to avoid the all-50% smell test.
	utilLevels := []struct {
		ai, vram float64
		usedMiB  int
	}{
		{0.0, 0.0, 0},      // idle
		{22.0, 18.0, 11796},
		{31.0, 28.0, 18350},
		{45.0, 41.0, 26869},
		{63.0, 58.0, 38010},
		{72.0, 67.0, 43909},
		{81.0, 75.0, 49152},
		{12.0, 10.0, 6554},
	}

	npus := make([]model.NPU, 0, len(nodeNames)*builder.NPUsPerNode)
	for _, name := range nodeNames {
		for _, l := range builder.LayoutNode(name) {
			sliceMode := pickSliceMode(l.Index)
			status := "healthy"
			// Make one card per node degraded for variety.
			if name == "worker-site-a-02" && l.Index == 5 {
				status = "degraded"
			}
			u := utilLevels[l.Index]
			power := 250.0 + float64(l.Index)*8.5
			temp := 42.0 + float64(l.Index)*1.7
			bw := u.ai * 0.9
			usage := &model.NPUUsage{
				AICoreUtilization:          u.ai,
				VRAMUtilization:            u.vram,
				VRAMUsedMiB:                u.usedMiB,
				MemoryBandwidthUtilization: &bw,
				Temperature:                &temp,
				Power:                      &power,
			}
			npus = append(npus, model.NPU{
				ID:          fmt.Sprintf("%s-npu-%d", name, l.Index),
				NodeName:    name,
				Model:       "Ascend910B",
				Index:       l.Index,
				VRAMMiB:     builder.NPUVRAMMiB,
				AICoreTotal: builder.NPUAICoreTotal,
				NumaNode:    l.NumaNode,
				HCCSGroup:   l.HCCSGroup,
				Status:      status,
				SliceMode:   sliceMode,
				Usage:       usage,
			})
		}
	}

	// ---- Slices ----
	// Only fixed-template / dynamic NPUs produce slice entries. Whole-card
	// NPUs are consumed by single-card workloads via npuUsage.allocated.
	slices := buildSlicesForSmall(npus)

	// ---- Workloads ----
	workloads := buildWorkloadsForSmall(t0, nodeNames, slices)

	// ---- Pools ----
	pools := buildPoolsForSmall(clusterID, nodeNames, npus, slices)

	// ---- Presets ----
	presets := buildPresetsForSmall()

	// ---- Events ----
	events := buildEventsForSmall(t0, nodeNames, slices, workloads)

	return &model.Dataset{
		Meta: model.Meta{
			Name:        "set-a-small",
			Version:     "0.1.0",
			GeneratedAt: t0.UTC().Format(time.RFC3339),
			Description: "Small single-cluster demo: 1 cluster / 3 nodes / 24 NPUs / mixed workloads. Reproducible (seed=42).",
			Scenario:    "small-cluster",
		},
		Clusters:  clusters,
		Nodes:     nodes,
		NPUs:      npus,
		Slices:    slices,
		Workloads: workloads,
		Pools:     pools,
		Presets:   presets,
		Events:    events,
	}
}

// pickSliceMode returns the sliceMode for NPU index i within a node.
// 0,1 -> whole; 2..5 -> fixed-template; 6,7 -> dynamic.
func pickSliceMode(i int) string {
	switch {
	case i <= 1:
		return "whole"
	case i <= 5:
		return "fixed-template"
	default:
		return "dynamic"
	}
}

func rangeInts(start, count int) []int {
	out := make([]int, count)
	for i := 0; i < count; i++ {
		out[i] = start + i
	}
	return out
}

// buildSlicesForSmall generates slice entries for the 12 fixed-template
// NPUs (3 nodes * 4) and the 6 dynamic NPUs (3 nodes * 2). Whole-card
// NPUs do not appear here.
//
// Fixed-template policy: pick one of vir01/vir02/vir04 per NPU so the
// dataset shows a mix of granularities. Each NPU yields 1-4 slices.
//
// Dynamic policy: 2 slices per NPU at irregular sizes to demonstrate the
// custom AI-core/memory shapes.
func buildSlicesForSmall(npus []model.NPU) []model.Slice {
	const aiCoreFull = builder.NPUAICoreTotal
	const vramFullMiB = builder.NPUVRAMMiB

	// (template, fraction). vir01 = 1/8, vir02 = 2/8, vir04 = 4/8.
	type tmpl struct {
		name     string
		fraction int // numerator over 8
	}
	templatesByIndex := map[int]tmpl{
		2: {"vir02", 2},
		3: {"vir04", 4},
		4: {"vir01", 1},
		5: {"vir02", 2},
	}

	out := make([]model.Slice, 0, 48)

	for _, npu := range npus {
		switch npu.SliceMode {
		case "fixed-template":
			t, ok := templatesByIndex[npu.Index]
			if !ok {
				continue
			}
			sliceCount := 8 / t.fraction
			aiPer := aiCoreFull * t.fraction / 8
			vramPer := vramFullMiB * t.fraction / 8
			for k := 0; k < sliceCount; k++ {
				sl := model.Slice{
					ID:        fmt.Sprintf("%s-slice-%d", npu.ID, k),
					ParentNPU: npu.ID,
					Template:  t.name,
					AICore:    aiPer,
					VRAMMiB:   vramPer,
					Status:    "available",
				}
				// Allocate ~50% of slices so the dashboard isn't blank.
				if (npu.Index+k)%2 == 0 {
					sl.Status = "allocated"
					sl.AllocatedTo = &model.AllocatedTo{
						Namespace:     "ai-inference",
						PodName:       fmt.Sprintf("pod-%s-%d", t.name, k),
						ContainerName: "main",
					}
					used := vramPer * 70 / 100
					ai := float64(aiPer) * 0.55
					vr := 70.0
					sl.Usage = &model.NPUUsage{
						AICoreUtilization: ai * 100.0 / float64(aiPer),
						VRAMUtilization:   vr,
						VRAMUsedMiB:       used,
					}
				}
				out = append(out, sl)
			}
		case "dynamic":
			// Two slices per dynamic NPU: a 12-core / 24 GiB and a
			// 8-core / 16 GiB. Both allocated.
			shapes := []struct {
				ai      int
				vramMiB int
			}{
				{12, 24576},
				{8, 16384},
			}
			for k, s := range shapes {
				sl := model.Slice{
					ID:        fmt.Sprintf("%s-slice-%d", npu.ID, k),
					ParentNPU: npu.ID,
					AICore:    s.ai,
					VRAMMiB:   s.vramMiB,
					Status:    "allocated",
					AllocatedTo: &model.AllocatedTo{
						Namespace:     "ai-inference",
						PodName:       fmt.Sprintf("pod-dyn-%s-%d", npu.ID, k),
						ContainerName: "main",
					},
				}
				usedMiB := s.vramMiB * 65 / 100
				sl.Usage = &model.NPUUsage{
					AICoreUtilization: 58.0,
					VRAMUtilization:   65.0,
					VRAMUsedMiB:       usedMiB,
				}
				out = append(out, sl)
			}
		}
	}

	return out
}

// buildWorkloadsForSmall produces 10 workloads with the mix:
//
//	6 running, 2 pending, 1 succeeded, 1 failed
//
// Types and kinds span inference / benchmark / training to exercise the UI.
func buildWorkloadsForSmall(t0 time.Time, nodeNames []string, slices []model.Slice) []model.Workload {
	allocatedByNode := map[string][]string{}
	for _, s := range slices {
		if s.Status != "allocated" {
			continue
		}
		node := nodeOf(s.ParentNPU)
		allocatedByNode[node] = append(allocatedByNode[node], s.ID)
	}

	mk := func(name, ns, t, kind, status string, replicas, ready int, nodes []string, allocated int, sliceIDs []string, daysAgo, hoursAgo int) model.Workload {
		w := model.Workload{
			Name:      name,
			Namespace: ns,
			Type:      t,
			Kind:      kind,
			Status:    status,
			Replicas:  &model.WorkloadReplicas{Desired: replicas, Ready: ready},
			NodeNames: nodes,
			CreatedAt: builder.SkewCreatedAt(t0, daysAgo, hoursAgo),
			Labels: map[string]string{
				"app.kubernetes.io/name": name,
				"team":                   "ai-platform",
			},
		}
		if allocated > 0 || len(sliceIDs) > 0 {
			w.NPUUsage = &model.WorkloadNPUUsage{
				Allocated: allocated,
				Slices:    sliceIDs,
			}
		}
		return w
	}

	wl := []model.Workload{}

	// 1. running: Qwen 8B PD (pd-pair, 2 pods)
	wl = append(wl, mk("qwen-8b-pd", "ai-inference", "inference", "InferenceService", "running",
		2, 2, []string{"worker-site-a-01", "worker-site-a-02"}, 2, []string{}, 6, 2))
	wl[0].Relations = []model.Relation{
		{From: "qwen-8b-pd-prefill-0", To: "qwen-8b-pd-decode-0", Type: "pd-pair"},
	}
	wl[0].Pods = []model.Pod{
		{
			Name: "qwen-8b-pd-prefill-0", Namespace: "ai-inference", NodeName: "worker-site-a-01", Status: "Running",
			Containers: []model.Container{
				{
					Name:  "prefill",
					Image: "mindie/vllm-ascend:0.11.0",
					Resources: &model.ContainerResources{
						CPU:       "16",
						Memory:    "64Gi",
						NPUSlices: []string{"worker-site-a-01-npu-0"},
					},
				},
			},
		},
		{
			Name: "qwen-8b-pd-decode-0", Namespace: "ai-inference", NodeName: "worker-site-a-02", Status: "Running",
			Containers: []model.Container{
				{
					Name:  "decode",
					Image: "mindie/vllm-ascend:0.11.0",
					Resources: &model.ContainerResources{
						CPU:       "16",
						Memory:    "64Gi",
						NPUSlices: []string{"worker-site-a-02-npu-1"},
					},
				},
			},
		},
	}
	bw0p50, bw0p90, bw0p99 := 110.0, 180.0, 260.0
	itl50, itl90, itl99 := 12.0, 18.0, 28.0
	wl[0].Metrics = &model.WorkloadMetrics{
		Throughput: &model.Throughput{
			InputTokensPerSec:  2400,
			OutputTokensPerSec: 1100,
			RequestsPerSec:     8.5,
		},
		Latency: &model.Latency{
			TTFTMs: &model.Percentile{P50: bw0p50, P90: bw0p90, P99: bw0p99},
			ITLMs:  &model.Percentile{P50: itl50, P90: itl90, P99: itl99},
		},
		ResourceUsage: &model.WorkloadResourceUsage{
			NPUUtilization:  65.0,
			VRAMUtilization: 58.0,
		},
	}

	// 2. running: DeepSeek 20B (single replica, 2 slices on a single node)
	wl = append(wl, mk("deepseek-20b", "ai-inference", "inference", "InferenceService", "running",
		1, 1, []string{"worker-site-a-03"}, 0, pickSliceIDsForNode(slices, "worker-site-a-03", 2), 4, 6))

	// 3. running: Qwen 14B (1 replica, fixed-template slices)
	wl = append(wl, mk("qwen-14b", "ai-inference", "inference", "InferenceService", "running",
		1, 1, []string{"worker-site-a-01"}, 0, pickSliceIDsForNode(slices, "worker-site-a-01", 2), 3, 12))

	// 4. running: Pi 3B (3 replicas, fixed-template)
	wl = append(wl, mk("pi-3b", "ai-inference", "inference", "Deployment", "running",
		3, 3, []string{"worker-site-a-02", "worker-site-a-03"}, 0, pickSliceIDsForNode(slices, "worker-site-a-02", 3), 2, 3))

	// 5. running: vllm-bench (job-like InferenceService used for load)
	wl = append(wl, mk("vllm-bench", "benchmark", "benchmark", "Job", "running",
		1, 1, []string{"worker-site-a-01"}, 0, pickSliceIDsForNode(slices, "worker-site-a-01", 1), 0, 4))

	// 6. running: Pi 3B canary on second node
	wl = append(wl, mk("pi-3b-canary", "ai-inference", "inference", "Deployment", "running",
		1, 1, []string{"worker-site-a-02"}, 0, pickSliceIDsForNode(slices, "worker-site-a-02", 1), 1, 2))

	// 7. pending: training job awaiting NPUs
	wl = append(wl, mk("llama2-7b-finetune", "training", "training", "Job", "pending",
		1, 0, []string{}, 0, nil, 0, 1))

	// 8. pending: another inference rollout
	wl = append(wl, mk("internlm-7b", "ai-inference", "inference", "InferenceService", "pending",
		1, 0, []string{}, 0, nil, 0, 0))

	// 9. succeeded: prior benchmark
	wl = append(wl, mk("bench-historical", "benchmark", "benchmark", "Job", "succeeded",
		1, 1, []string{"worker-site-a-03"}, 1, nil, 8, 3))

	// 10. failed: model that OOMed on launch
	wl = append(wl, mk("oom-test-model", "ai-inference", "inference", "Deployment", "failed",
		1, 0, []string{"worker-site-a-01"}, 0, nil, 5, 1))

	return wl
}

// pickSliceIDsForNode returns up to limit allocated slice ids belonging
// to nodeName. Stable order (slices are already sorted by parent NPU id).
func pickSliceIDsForNode(slices []model.Slice, nodeName string, limit int) []string {
	out := []string{}
	for _, s := range slices {
		if s.Status != "allocated" {
			continue
		}
		if nodeOf(s.ParentNPU) != nodeName {
			continue
		}
		out = append(out, s.ID)
		if len(out) == limit {
			break
		}
	}
	return out
}

// nodeOf extracts the node name out of an NPU id like "worker-site-a-01-npu-3".
func nodeOf(npuID string) string {
	for i := len(npuID) - 1; i >= 0; i-- {
		if npuID[i] == '-' {
			// strip "-npu-N"
			if i >= 4 && npuID[i-4:i] == "-npu" {
				return npuID[:i-4]
			}
		}
	}
	return npuID
}

// buildPoolsForSmall returns the 4 pool sections expected by the schema:
//   - 1 ClusterPool (the lone edge cluster as a member with Push sync)
//   - 1 NodePool   (all 3 worker nodes, role=edge)
//   - 1 NPUPool    (24 NPUs, with HCCS topology groups)
//   - 2 NPUSlicePool (one FixedTemplate, one Dynamic)
func buildPoolsForSmall(clusterID string, nodeNames []string, npus []model.NPU, slices []model.Slice) model.Pools {
	healthy := 0
	allocated := 0
	for _, n := range npus {
		if n.Status == "healthy" {
			healthy++
		}
	}
	for _, s := range slices {
		if s.Status == "allocated" {
			allocated++
		}
	}

	// HCCS topology: 6 groups (3 nodes * 2 groups per node).
	var hccsGroups []model.HCCSGroup
	for _, name := range nodeNames {
		grouped := builder.HCCSGroupsForNode(name)
		for gid, ids := range grouped {
			hccsGroups = append(hccsGroups, model.HCCSGroup{
				ID:       gid,
				NPUs:     ids,
				Topology: "full-mesh",
			})
		}
	}
	// Stable order by group id.
	sortHCCSGroups(hccsGroups)

	fixedSliceCount := 0
	dynSliceCount := 0
	fixedAllocated := 0
	dynAllocated := 0
	for _, s := range slices {
		if s.Template != "" {
			fixedSliceCount++
			if s.Status == "allocated" {
				fixedAllocated++
			}
		} else {
			dynSliceCount++
			if s.Status == "allocated" {
				dynAllocated++
			}
		}
	}

	return model.Pools{
		Cluster: []model.ClusterPool{
			{
				Name: "edge-cluster-pool-a",
				Clusters: []model.ClusterPoolMember{
					{ClusterID: clusterID, SyncPolicy: "Push"},
				},
				Status: &model.ClusterPoolStatus{
					HealthyClusters: []string{clusterID},
				},
			},
		},
		Node: []model.NodePool{
			{
				Name:           "site-a-worker-pool",
				ClusterPoolRef: "edge-cluster-pool-a",
				Role:           "edge",
				Location:       "site-a-shanghai",
				Selector: map[string]any{
					"matchLabels": map[string]any{
						"npu.huawei.com/model": "Ascend910B",
					},
				},
				Status: &model.NodePoolStatus{
					Nodes:       nodeNames,
					TotalCPU:    &model.Quantity{Raw: "288"},
					TotalMemory: &model.Quantity{Raw: "2304Gi"},
				},
			},
		},
		NPU: []model.NPUPool{
			{
				Name:          "ascend-910b-pool-a",
				NodePoolRef:   "site-a-worker-pool",
				NPUModel:      "Ascend910B",
				SliceStrategy: "FixedTemplate+Dynamic",
				Status: &model.NPUPoolStatus{
					TotalNPUs:     len(npus),
					HealthyNPUs:   healthy,
					AllocatedNPUs: allocated,
					HCCSTopology: &model.HCCSTopology{
						Groups: hccsGroups,
					},
				},
			},
		},
		NPUSlice: []model.NPUSlicePool{
			{
				Name:       "fixed-template-slice-pool",
				NPUPoolRef: "ascend-910b-pool-a",
				Strategy:   "FixedTemplate",
				FixedTemplates: []model.FixedTemplate{
					{Name: "vir01", AICoreCount: 4, MemoryMiB: 8192},
					{Name: "vir02", AICoreCount: 8, MemoryMiB: 16384},
					{Name: "vir04", AICoreCount: 16, MemoryMiB: 32768},
				},
				Status: &model.NPUSlicePoolStatus{
					TotalSlices:     fixedSliceCount,
					AllocatedSlices: fixedAllocated,
					AvailableSlices: fixedSliceCount - fixedAllocated,
				},
			},
			{
				Name:       "dynamic-slice-pool",
				NPUPoolRef: "ascend-910b-pool-a",
				Strategy:   "Dynamic",
				DynamicSlicing: &model.DynamicSlicing{
					MinAICore:            2,
					MaxAICore:            32,
					MemoryGranularityMiB: 4096,
					AllowAggregation:     false,
				},
				Status: &model.NPUSlicePoolStatus{
					TotalSlices:     dynSliceCount,
					AllocatedSlices: dynAllocated,
					AvailableSlices: dynSliceCount - dynAllocated,
				},
			},
		},
	}
}

// buildPresetsForSmall returns the 4 deploy presets (Pi 3B / Qwen 8B PD /
// DeepSeek 20B / Qwen 14B) listed in docs/architecture.md §...
func buildPresetsForSmall() []model.Preset {
	return []model.Preset{
		{
			ID:        "pi-3b",
			Name:      "Pi 3B Inference",
			Kind:      "inference",
			ModelSize: "3B",
			Runtime:   "mindie",
			Requirements: model.PresetRequirements{
				NPUCount:      1,
				NPUModel:      "Ascend910B",
				VRAMMiBPerNPU: 8192,
				CPU:           &model.Quantity{Raw: "4"},
				Memory:        &model.Quantity{Raw: "16Gi"},
			},
			Description: "Lightweight 3B inference. Single NPU slice (vir01), suitable for chat / edge demo.",
			Tags:        []string{"inference", "small", "single-card"},
			Manifest:    "placeholder://helm/pi-3b-values.yaml",
		},
		{
			ID:        "qwen-8b-pd",
			Name:      "Qwen 8B PD Disaggregated",
			Kind:      "inference-pd",
			ModelSize: "8B",
			Runtime:   "vllm",
			Requirements: model.PresetRequirements{
				NPUCount:      2,
				NPUModel:      "Ascend910B",
				VRAMMiBPerNPU: 65536,
				CPU:           &model.Quantity{Raw: "16"},
				Memory:        &model.Quantity{Raw: "64Gi"},
			},
			Description: "Qwen 8B with prefill / decode disaggregation (1P1D). Uses vllm-ascend + Mooncake KV transfer.",
			Tags:        []string{"inference", "pd", "vllm"},
			Manifest:    "placeholder://helm/qwen-8b-pd-values.yaml",
		},
		{
			ID:        "deepseek-20b",
			Name:      "DeepSeek 20B Inference",
			Kind:      "inference",
			ModelSize: "20B",
			Runtime:   "mindie",
			Requirements: model.PresetRequirements{
				NPUCount:      2,
				NPUModel:      "Ascend910B",
				VRAMMiBPerNPU: 65536,
				CPU:           &model.Quantity{Raw: "16"},
				Memory:        &model.Quantity{Raw: "64Gi"},
			},
			Description: "DeepSeek 20B colocated inference on 2 whole 910B cards with HCCS full-mesh group.",
			Tags:        []string{"inference", "large", "hccs"},
			Manifest:    "placeholder://helm/deepseek-20b-values.yaml",
		},
		{
			ID:        "qwen-14b",
			Name:      "Qwen 14B Inference",
			Kind:      "inference",
			ModelSize: "14B",
			Runtime:   "mindie",
			Requirements: model.PresetRequirements{
				NPUCount:      1,
				NPUModel:      "Ascend910B",
				VRAMMiBPerNPU: 32768,
				CPU:           &model.Quantity{Raw: "8"},
				Memory:        &model.Quantity{Raw: "32Gi"},
			},
			Description: "Qwen 14B inference on a single 910B card with vir04 slice.",
			Tags:        []string{"inference", "medium", "single-card"},
			Manifest:    "placeholder://helm/qwen-14b-values.yaml",
		},
	}
}

// buildEventsForSmall returns 21 events spaced over the first ~120 seconds
// of the demo (5-10s cadence per configs/CLAUDE.md §3.3).
func buildEventsForSmall(t0 time.Time, nodeNames []string, slices []model.Slice, workloads []model.Workload) []model.Event {
	type ev struct {
		offsetSec int
		typ       string
		payload   map[string]any
	}

	// Hand-curated story:
	//   t0+0    initial topology snapshot
	//   t0+5    workload created (rolling)
	//   t0+12   slice allocated for a Pi-3B replica
	//   t0+18   another slice allocated (Qwen 14B)
	//   t0+25   workload status: pending -> running for internlm
	//   t0+33   NPU heartbeat / status check
	//   t0+40   slice release (vllm-bench finishes a batch)
	//   t0+48   workload created: hot canary
	//   t0+56   topology update (new workload landed)
	//   ... continues to t0+125
	tape := []ev{
		{0, "topology.update", map[string]any{"snapshot": "initial", "clusters": 1, "nodes": 3, "npus": 24}},
		{5, "workload.created", map[string]any{"namespace": "ai-inference", "name": "pi-3b-canary", "preset": "pi-3b"}},
		{12, "slice.allocated", map[string]any{
			"sliceId": "worker-site-a-02-npu-2-slice-0",
			"to":      map[string]any{"namespace": "ai-inference", "podName": "pi-3b-canary-0"},
		}},
		{18, "slice.allocated", map[string]any{
			"sliceId": "worker-site-a-01-npu-3-slice-0",
			"to":      map[string]any{"namespace": "ai-inference", "podName": "qwen-14b-0"},
		}},
		{25, "workload.statusChanged", map[string]any{"namespace": "ai-inference", "name": "internlm-7b", "from": "pending", "to": "running"}},
		{33, "npu.statusChanged", map[string]any{"npuId": "worker-site-a-02-npu-5", "from": "healthy", "to": "degraded", "reason": "ECC error threshold exceeded"}},
		{40, "slice.released", map[string]any{
			"sliceId": "worker-site-a-01-npu-4-slice-1",
			"reason":  "batch complete",
		}},
		{48, "workload.created", map[string]any{"namespace": "ai-inference", "name": "qwen-8b-pd-trial", "preset": "qwen-8b-pd"}},
		{56, "topology.update", map[string]any{"delta": "added 1 inference-pd workload", "clusters": 1, "nodes": 3, "npus": 24}},
		{63, "slice.allocated", map[string]any{
			"sliceId": "worker-site-a-03-npu-6-slice-0",
			"to":      map[string]any{"namespace": "ai-inference", "podName": "qwen-8b-pd-trial-prefill-0"},
		}},
		{70, "slice.allocated", map[string]any{
			"sliceId": "worker-site-a-03-npu-7-slice-0",
			"to":      map[string]any{"namespace": "ai-inference", "podName": "qwen-8b-pd-trial-decode-0"},
		}},
		{78, "workload.statusChanged", map[string]any{"namespace": "ai-inference", "name": "qwen-8b-pd-trial", "from": "pending", "to": "running"}},
		{85, "npu.statusChanged", map[string]any{"npuId": "worker-site-a-01-npu-0", "from": "healthy", "to": "healthy", "reason": "heartbeat", "rxBytes": 12345678}},
		{92, "workload.statusChanged", map[string]any{"namespace": "ai-inference", "name": "oom-test-model", "from": "running", "to": "failed", "reason": "OOMKilled"}},
		{98, "workload.deleted", map[string]any{"namespace": "ai-inference", "name": "oom-test-model"}},
		{105, "slice.released", map[string]any{
			"sliceId": "worker-site-a-02-npu-2-slice-2",
			"reason":  "scale down",
		}},
		{110, "topology.update", map[string]any{"delta": "removed failed workload", "clusters": 1, "nodes": 3, "npus": 24}},
		{116, "workload.created", map[string]any{"namespace": "training", "name": "llama2-7b-finetune", "preset": "custom-job"}},
		{120, "workload.statusChanged", map[string]any{"namespace": "training", "name": "llama2-7b-finetune", "from": "pending", "to": "pending", "reason": "waiting for 4 NPUs"}},
		{125, "npu.statusChanged", map[string]any{"npuId": "worker-site-a-03-npu-1", "from": "healthy", "to": "healthy", "reason": "heartbeat"}},
		{130, "slice.allocated", map[string]any{
			"sliceId": "worker-site-a-01-npu-5-slice-1",
			"to":      map[string]any{"namespace": "ai-inference", "podName": "pi-3b-2"},
		}},
	}

	out := make([]model.Event, 0, len(tape))
	for _, e := range tape {
		out = append(out, model.Event{
			Timestamp: builder.EventTimestamp(t0, e.offsetSec),
			Type:      e.typ,
			Payload:   e.payload,
		})
	}
	return out
}

// sortHCCSGroups sorts in place by group id for deterministic output.
func sortHCCSGroups(g []model.HCCSGroup) {
	for i := 1; i < len(g); i++ {
		for j := i; j > 0 && g[j-1].ID > g[j].ID; j-- {
			g[j-1], g[j] = g[j], g[j-1]
		}
	}
}
