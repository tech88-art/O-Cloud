// preset_stress emits set-c-stress (known-issues #4): a 1-cluster /
// 100-node / 800-NPU fixture sized to exercise the ADR-0005 推翻条件
// (FPS ≥ 15 with workload toggle ON). The dataset stays schema-valid
// at every level but is intentionally lean on narrative (a few demo
// workloads + a fabric tree, no events.json storyline) — the value is
// in the topology graph node count, not the events.
//
// Topology summary:
//
//   1 cluster (`cluster-stress-a`)
//   100 nodes (`worker-stress-000` .. `worker-stress-099`)
//   800 NPUs (8 per node, 2 NUMA domains, 2 HCCS groups each)
//   0 slices (whole-NPU mode everywhere for size economy)
//   20 workloads spread across the 100 nodes:
//       1 large PD pair (10 prefill + 10 decode pods on 20 NPUs)
//       4 medium inference jobs (3 pods each, single-node)
//       15 small inference jobs (1 pod each)
//       — total ~50 pods with bindings
//   4 presets (same shape as set-a-small for swap parity)
//   1 spine + 4 leaf + 100 ToR-uplink links (fabric:
//       spine ←→ leaf-a..d, each leaf ←→ 25 nodes)
//   3 events (cluster-up + 1 deploy + 1 steady-state delta)
//
// Reproducibility: gofakeit seed = 4242; fixed t0 = 2026-05-17T12:00:00Z.
package main

import (
	"fmt"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/example/ocloud-edge/configs/mock-data/generator/pkg/builder"
	"github.com/example/ocloud-edge/configs/mock-data/generator/pkg/model"
)

const (
	seedStress      = 4242
	stressNodes     = 100
	stressClusterID = "cluster-stress-a"
)

func buildSetCStress() (*model.Dataset, error) {
	gofakeit.Seed(seedStress)
	t0, _ := time.Parse(time.RFC3339, t0SmallRFC) // reuse anchor for stable diffs

	nodeNames := make([]string, stressNodes)
	for i := 0; i < stressNodes; i++ {
		nodeNames[i] = fmt.Sprintf("worker-stress-%03d", i)
	}

	// ---- clusters ----
	clusters := []model.Cluster{
		{
			ID:                stressClusterID,
			Name:              stressClusterID,
			Role:              "small-cluster",
			Location:          "site-stress-shanghai",
			Status:            "healthy",
			KubernetesVersion: "v1.31.0",
			NodeCount:         stressNodes,
			NPUCount:          stressNodes * builder.NPUsPerNode,
			Labels: map[string]string{
				"site":        "stress",
				"environment": "stress",
				"region":      "cn-east-1",
			},
			CreatedAt: builder.SkewCreatedAt(t0, 14, 0),
		},
	}

	// ---- nodes ----
	nodes := make([]model.Node, 0, stressNodes)
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
		// Stress dataset uses synthetic /24 ranges so test fixtures don't
		// confuse them with set-a-small reservations.
		ipOctet := 20 + (i % 200)
		nodes = append(nodes, model.Node{
			Name:           name,
			ClusterID:      stressClusterID,
			Role:           []string{"worker"},
			Status:         "Ready",
			CPU:            model.Quantity{Raw: "96"},
			Memory:         model.Quantity{Raw: "768Gi"},
			Arch:           "arm64",
			KernelVersion:  "5.10.0-153.12.0.92.oe2203sp3.aarch64",
			OS:             "openEuler 22.03 LTS SP3",
			KubeletVersion: "v1.31.0",
			NPUCount:       builder.NPUsPerNode,
			NUMA:           numa,
			NetworkInterfaces: []model.NetworkInterface{
				{
					Name:  "eth0",
					IPs:   []string{fmt.Sprintf("10.30.%d.%d", i/256, ipOctet)},
					Speed: "25Gbps",
				},
			},
			Labels: map[string]string{
				"node.kubernetes.io/instance-type": "atlas-800t-a2",
				"topology.kubernetes.io/zone":      fmt.Sprintf("zone-%d", (i%4)+1),
				"npu.huawei.com/model":             "Ascend910B",
			},
		})
	}

	// ---- NPUs (800 total) ----
	npus := make([]model.NPU, 0, stressNodes*builder.NPUsPerNode)
	for _, name := range nodeNames {
		for _, l := range builder.LayoutNode(name) {
			npus = append(npus, model.NPU{
				ID:          fmt.Sprintf("%s-npu-%d", name, l.Index),
				NodeName:    name,
				Model:       "Ascend910B",
				Index:       l.Index,
				VRAMMiB:     builder.NPUVRAMMiB,
				AICoreTotal: builder.NPUAICoreTotal,
				NumaNode:    l.NumaNode,
				HCCSGroup:   l.HCCSGroup,
				// ADR-0021: fixed hardware bandwidths (PCIe Gen4 ≈ 32 · HCCS ≈ 56 GB/s).
				PCIeBandwidthGBps: 32.0,
				HCCSBandwidthGBps: 56.0,
				Status:            "healthy",
				SliceMode:         "whole",
				// Skip Usage to keep the file small — 800 NPUs × 6 usage
				// fields would balloon npus.json into the megabytes for
				// no visual benefit at this scale.
			})
		}
	}

	// ---- Workloads (20 total, ~47 pods) ----
	workloads := buildStressWorkloads(t0, nodeNames)

	// ---- Pools ----
	pools := buildStressPools(stressClusterID, nodeNames)

	// ---- Presets (reuse small) ----
	presets := buildPresetsForSmall()

	// ---- Events (minimal) ----
	events := []model.Event{
		{
			Timestamp: builder.EventTimestamp(t0, 0),
			Type:      "topology.update",
			Payload: map[string]any{
				"clusters": 1, "nodes": stressNodes, "npus": stressNodes * builder.NPUsPerNode,
				"snapshot": "initial", "stage": "stress-cluster-up",
			},
		},
		{
			Timestamp: builder.EventTimestamp(t0, 30),
			Type:      "workload.created",
			Payload: map[string]any{
				"name": "stress-pd-large", "namespace": "stress", "preset": "qwen-8b-pd",
			},
		},
		{
			Timestamp: builder.EventTimestamp(t0, 60),
			Type:      "topology.update",
			Payload: map[string]any{
				"delta": "20 stress workloads steady-state", "stage": "stress-steady",
			},
		},
	}

	// ---- Fabric (1 spine + 4 leaf + 100 ToR-uplink links) ----
	switches, links := buildStressFabric(nodeNames)

	return &model.Dataset{
		Meta: model.Meta{
			Name:        "set-c-stress",
			Version:     "0.1.0",
			GeneratedAt: t0.UTC().Format(time.RFC3339),
			Description: fmt.Sprintf("Stress single-cluster: 1 cluster / %d nodes / %d NPUs / 20 workloads / spine+4leaf fabric. ADR-0005 推翻条件 验证 dataset. Reproducible (seed=%d).",
				stressNodes, stressNodes*builder.NPUsPerNode, seedStress),
			Scenario: "stress",
		},
		Clusters:        clusters,
		Nodes:           nodes,
		NPUs:            npus,
		Slices:          []model.Slice{}, // whole-NPU mode
		Workloads:       workloads,
		Pools:           pools,
		Presets:         presets,
		Events:          events,
		NetworkSwitches: switches,
		NetworkLinks:    links,
	}, nil
}

// buildStressWorkloads emits 20 workloads totaling ~47 pods scattered
// across the 100 stress nodes. Pod counts:
//
//   stress-pd-large    20 pods (10 prefill + 10 decode, PD pair)
//   stress-medium-NN    3 pods each × 4 = 12 pods
//   stress-small-NN     1 pod each  × 15 = 15 pods
func buildStressWorkloads(t0 time.Time, nodeNames []string) []model.Workload {
	wl := []model.Workload{}

	// 1. The large PD pair: 10 prefill on nodes 0..9, 10 decode on 10..19.
	pdPair := model.Workload{
		Name:      "stress-pd-large",
		Namespace: "stress",
		Type:      "inference",
		Kind:      "InferenceService",
		Status:    "running",
		Replicas:  &model.WorkloadReplicas{Desired: 20, Ready: 20},
		NodeNames: nodeNames[0:20],
		NPUUsage:  &model.WorkloadNPUUsage{Allocated: 20},
		CreatedAt: builder.SkewCreatedAt(t0, 3, 0),
		Labels: map[string]string{
			"app.kubernetes.io/name": "stress-pd-large",
			"team":                   "stress-team",
		},
	}
	pdSlices := make([]string, 0, 20)
	relations := make([]model.Relation, 0, 10)
	pods := make([]model.Pod, 0, 20)
	for i := 0; i < 10; i++ {
		prefillSlice := fmt.Sprintf("%s-npu-0", nodeNames[i])
		decodeSlice := fmt.Sprintf("%s-npu-0", nodeNames[i+10])
		pdSlices = append(pdSlices, prefillSlice, decodeSlice)
		prefillName := fmt.Sprintf("stress-pd-large-prefill-%d", i)
		decodeName := fmt.Sprintf("stress-pd-large-decode-%d", i)
		relations = append(relations, model.Relation{From: prefillName, To: decodeName, Type: "pd-pair"})
		pods = append(pods,
			stressPod(prefillName, "stress", nodeNames[i], "prefill", prefillSlice),
			stressPod(decodeName, "stress", nodeNames[i+10], "decode", decodeSlice),
		)
	}
	pdPair.NPUUsage.Slices = pdSlices
	pdPair.Relations = relations
	pdPair.Pods = pods
	wl = append(wl, pdPair)

	// 2. 4 medium inference jobs — 3 pods each, single-node, npus 1..3.
	for i := 0; i < 4; i++ {
		nodeIdx := 20 + i*5
		name := fmt.Sprintf("stress-medium-%02d", i)
		nodeName := nodeNames[nodeIdx]
		mediumPods := make([]model.Pod, 3)
		mediumSlices := make([]string, 3)
		for r := 0; r < 3; r++ {
			sliceID := fmt.Sprintf("%s-npu-%d", nodeName, r+1)
			mediumSlices[r] = sliceID
			mediumPods[r] = stressPod(
				fmt.Sprintf("%s-%d", name, r), "stress", nodeName, "primary", sliceID,
			)
		}
		wl = append(wl, model.Workload{
			Name:      name,
			Namespace: "stress",
			Type:      "inference",
			Kind:      "Deployment",
			Status:    "running",
			Replicas:  &model.WorkloadReplicas{Desired: 3, Ready: 3},
			NodeNames: []string{nodeName},
			NPUUsage: &model.WorkloadNPUUsage{
				Allocated: 3,
				Slices:    mediumSlices,
			},
			Pods:      mediumPods,
			CreatedAt: builder.SkewCreatedAt(t0, 2, i),
			Labels: map[string]string{
				"app.kubernetes.io/name": name,
				"team":                   "stress-team",
			},
		})
	}

	// 3. 15 small inference jobs — 1 pod each, scattered nodes 40..99.
	for i := 0; i < 15; i++ {
		nodeIdx := 40 + i*4
		name := fmt.Sprintf("stress-small-%02d", i)
		nodeName := nodeNames[nodeIdx]
		sliceID := fmt.Sprintf("%s-npu-7", nodeName) // npu-7 on every chosen node
		wl = append(wl, model.Workload{
			Name:      name,
			Namespace: "stress",
			Type:      "inference",
			Kind:      "Deployment",
			Status:    "running",
			Replicas:  &model.WorkloadReplicas{Desired: 1, Ready: 1},
			NodeNames: []string{nodeName},
			NPUUsage: &model.WorkloadNPUUsage{
				Allocated: 1,
				Slices:    []string{sliceID},
			},
			Pods: []model.Pod{
				stressPod(fmt.Sprintf("%s-0", name), "stress", nodeName, "primary", sliceID),
			},
			CreatedAt: builder.SkewCreatedAt(t0, 1, i),
			Labels: map[string]string{
				"app.kubernetes.io/name": name,
				"team":                   "stress-team",
			},
		})
	}

	return wl
}

// stressPod factors the common Pod construction: stress namespace,
// running status, single container against one NPU slice, one binding.
func stressPod(name, namespace, nodeName, role, sliceID string) model.Pod {
	return model.Pod{
		Name:      name,
		Namespace: namespace,
		NodeName:  nodeName,
		Status:    "Running",
		Containers: []model.Container{
			{
				Name:  role,
				Image: "mindie/vllm-ascend:0.11.0",
				Resources: &model.ContainerResources{
					CPU:       "16",
					Memory:    "64Gi",
					NPUSlices: []string{sliceID},
				},
			},
		},
		Bindings: []model.PodBinding{
			{SliceID: sliceID, Role: role, IndexInPod: 0},
		},
	}
}

// buildStressPools returns the minimal pool set the schema requires.
// Stress fixture doesn't model the full pool topology — Phase 2 multi-
// site would extend this. ClusterPool / NodePool / NPUPool each with 1
// member; NPUSlicePool empty (whole-NPU mode).
func buildStressPools(clusterID string, nodeNames []string) model.Pools {
	return model.Pools{
		Cluster: []model.ClusterPool{
			{
				Name: "stress-cluster-pool",
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
				Name:           "stress-node-pool",
				ClusterPoolRef: "stress-cluster-pool",
				Role:           "edge",
				Status: &model.NodePoolStatus{
					Nodes: nodeNames,
				},
			},
		},
		NPU: []model.NPUPool{
			{
				Name:        "stress-npu-pool",
				NodePoolRef: "stress-node-pool",
				NPUModel:    "Ascend910B",
				Status: &model.NPUPoolStatus{
					TotalNPUs:     stressNodes * builder.NPUsPerNode,
					HealthyNPUs:   stressNodes * builder.NPUsPerNode,
					AllocatedNPUs: 47,
				},
			},
		},
		NPUSlice: []model.NPUSlicePool{},
	}
}

// buildStressFabric emits a 1-spine + 4-leaf + 100-uplink topology.
// Every node connects to one leaf (25 nodes per leaf); each leaf
// connects up to the spine. Mirrors a typical small-fabric DC layout.
func buildStressFabric(nodeNames []string) ([]model.NetworkSwitch, []model.NetworkLink) {
	switches := []model.NetworkSwitch{
		{
			ID:            "switch-spine-01",
			Name:          "switch-spine-01",
			Type:          "spine",
			Location:      "site-stress-shanghai-row-A",
			PortsTotal:    32,
			PortsUsed:     4,
			BandwidthGbps: 400,
			Status:        "up",
		},
	}
	leafNames := []string{"switch-leaf-a", "switch-leaf-b", "switch-leaf-c", "switch-leaf-d"}
	for _, lname := range leafNames {
		switches = append(switches, model.NetworkSwitch{
			ID:            lname,
			Name:          lname,
			Type:          "leaf",
			Location:      "site-stress-shanghai-row-A",
			PortsTotal:    48,
			PortsUsed:     25,
			BandwidthGbps: 100,
			Status:        "up",
		})
	}

	links := make([]model.NetworkLink, 0, len(leafNames)+len(nodeNames))
	// spine ↔ leaf uplinks
	for _, lname := range leafNames {
		links = append(links, model.NetworkLink{
			ID:            fmt.Sprintf("link-spine-01-%s", lname),
			From:          lname,
			To:            "switch-spine-01",
			BandwidthGbps: 400,
			Medium:        "optical",
			Utilization:   25.0,
			RTTUs:         0.5,
		})
	}
	// node ↔ leaf (25 nodes per leaf)
	for i, name := range nodeNames {
		leaf := leafNames[i/25]
		links = append(links, model.NetworkLink{
			ID:            fmt.Sprintf("link-%s-%s", leaf, name),
			From:          name,
			To:            leaf,
			BandwidthGbps: 25,
			Medium:        "dac",
			Utilization:   15.0,
			RTTUs:         0.8,
		})
	}
	// ADR-0021: node↔node inter-node `network` links (RoCE · GB/s) — both
	// endpoints are node ids, so the aggregator classifies them as `network`
	// (green) edges. Ring the nodes WITHIN each leaf group (intra-rack
	// interconnect) so the stress set exercises network-edge rendering at scale
	// without an N² mesh. bandwidthGBps ≈ 12.5 GB/s (~100 Gbps).
	for start := 0; start < len(nodeNames); start += 25 {
		end := start + 25
		if end > len(nodeNames) {
			end = len(nodeNames)
		}
		group := nodeNames[start:end]
		if len(group) < 2 {
			continue
		}
		for j := range group {
			from := group[j]
			to := group[(j+1)%len(group)]
			links = append(links, model.NetworkLink{
				ID:            fmt.Sprintf("net-%s-%s", from, to),
				From:          from,
				To:            to,
				BandwidthGbps: 100,
				BandwidthGBps: 12.5,
				Medium:        "roce",
				Utilization:   20.0,
				RTTUs:         1.5,
			})
		}
	}
	return switches, links
}
