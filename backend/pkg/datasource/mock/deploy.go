// Package mock — Deploy mutators (P1-T-202).
//
// Deploy / DeleteDeploy are the only datasource methods that MUTATE the mock
// caches. Two invariants the rest of the package relies on:
//
//  1. We never write through the loader's nil → empty-slice fast path. If a
//     fixture set ships with zero workloads (e.g. tests that only need a
//     presets.json), we lazily allocate the workloads slice on first Deploy
//     so the appended entry persists.
//
//  2. Mutations are guarded by s.deployMu. Loaders (loadWorkloads, loadNPUs)
//     own their own sync.Once and return BEFORE we lock — so the lock is held
//     only across cache mutations, never disk I/O.
//
// Slice-conflict semantics:
//   - Manual mode: each requested npuSliceId is checked against the NPU cache
//     (loaded by loadNPUs which folded slices.json into NPU.Slices). If any
//     slice has status != "available", we return ErrSliceConflict so the
//     handler can emit 409 with the offending slice id in details.
//   - Auto mode: we walk the NPU cache looking for the first req.Replicas
//     available slices and lay them down — no real scheduler, just enough to
//     surface a deployId for the demo.
//
// The newly created workload is appended in-memory only; restarting the demo
// resets to the on-disk fixture. That's by design (backend has no DB per
// project root §5).
package mock

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// ErrPresetNotFoundForDeploy is the sentinel Deploy returns when req.PresetID
// does not exist in the presets fixture. Distinct from preset.go's
// ErrPresetNotFound only in semantics: handlers map this one onto 400/404 in
// the deploy context, where the user-supplied body is at fault rather than a
// path parameter.
var ErrPresetNotFoundForDeploy = errors.New("mock source: deploy preset not found")

// ErrSliceConflict surfaces when a manual placement references a slice that
// is already allocated (or faulty). The handler maps it to 409 with the
// offending slice id in details.
var ErrSliceConflict = errors.New("mock source: slice already allocated")

// ErrInsufficientCapacity surfaces when auto-mode cannot find enough available
// slices to satisfy req.Replicas. Maps to 409 (the contract treats "resource
// unavailable" alongside the manual-mode conflict case).
var ErrInsufficientCapacity = errors.New("mock source: insufficient available slices")

// ErrDeployNotFound is the DELETE sentinel — unknown deployId. Handler maps
// to 404.
var ErrDeployNotFound = errors.New("mock source: deploy not found")

// ErrInvalidDeployRequest covers structural issues we catch before touching
// any cache (replicas <= 0, manual mode without a placement). Handler maps to
// 400.
var ErrInvalidDeployRequest = errors.New("mock source: invalid deploy request")

// schedulingModeAuto / Manual mirror the contract enum on
// DeployRequest.scheduling.mode. Empty string defaults to auto per the OpenAPI
// `default: auto`.
const (
	schedulingModeAuto   = "auto"
	schedulingModeManual = "manual"
)

// Deploy materializes a new mock workload from req and returns a synthetic
// DeployResponse. The mutation is best-effort and in-memory only: the demo
// fixture file is never rewritten.
//
// Sequence:
//  1. Validate req structurally (presetId, replicas, mode).
//  2. Resolve preset → required slice count per replica (defaults to 1 when
//     the preset omits requirements.npuCount, matching the contract default).
//  3. Pick slices:
//     - manual: every requested slice must be `available`; otherwise 409.
//     - auto:   find any req.Replicas * npuPerReplica available slices.
//  4. Append a new WorkloadDetail to the cache + flip every chosen slice's
//     status to `allocated`. Update s.deployIndex.
//  5. Return DeployResponse{deployId, workloadName, namespace, status:
//     "accepted", scheduledNodes}.
//
// ctx cancellation is honored at the start; once we begin mutating, we run to
// completion.
func (s *Source) Deploy(ctx context.Context, req *model.DeployRequest) (*model.DeployResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("%w: nil request", ErrInvalidDeployRequest)
	}
	if strings.TrimSpace(req.PresetID) == "" {
		return nil, fmt.Errorf("%w: presetId is required", ErrInvalidDeployRequest)
	}
	if strings.TrimSpace(req.Namespace) == "" {
		return nil, fmt.Errorf("%w: namespace is required", ErrInvalidDeployRequest)
	}
	replicas := req.Replicas
	if replicas == 0 {
		replicas = 1 // contract default
	}
	if replicas < 0 {
		return nil, fmt.Errorf("%w: replicas must be >= 1 (got %d)", ErrInvalidDeployRequest, replicas)
	}

	mode := schedulingModeAuto
	if req.Scheduling != nil && req.Scheduling.Mode != "" {
		mode = req.Scheduling.Mode
	}
	if mode != schedulingModeAuto && mode != schedulingModeManual {
		return nil, fmt.Errorf("%w: scheduling.mode must be auto|manual (got %q)", ErrInvalidDeployRequest, mode)
	}

	// Resolve preset to determine required slices per replica + naming default.
	presetList, err := s.loadPresets()
	if err != nil {
		return nil, err
	}
	var preset *model.PresetDetail
	for _, p := range presetList {
		if p != nil && p.ID == req.PresetID {
			preset = p
			break
		}
	}
	if preset == nil {
		return nil, fmt.Errorf("%w: presetId=%s", ErrPresetNotFoundForDeploy, req.PresetID)
	}
	npuPerReplica := 1
	if preset.Requirements != nil && preset.Requirements.NPUCount > 0 {
		npuPerReplica = preset.Requirements.NPUCount
	}

	// Load NPU cache up front (loadNPUs is sync.Once gated, runs disk I/O
	// before we acquire deployMu).
	if _, err := s.loadNPUs(); err != nil {
		return nil, err
	}
	// Workloads cache may be empty for fixture-light test setups (deploy_test
	// writes only presets.json + slices/npus). loadWorkloads returns nil + nil
	// in that case and we lazily allocate below.
	if _, err := s.loadWorkloads(); err != nil {
		return nil, err
	}

	s.deployMu.Lock()
	defer s.deployMu.Unlock()

	// Pick the slices to allocate. We point at the cache directly (not copies)
	// so flipping NPUSlice.Status is observable to subsequent ListNPUs /
	// GetTopology requests — that's the whole point of the in-memory mutation.
	picked, scheduledNodes, err := s.pickSlicesLocked(mode, replicas, npuPerReplica, req)
	if err != nil {
		return nil, err
	}

	// Synthesize the workload name. Contract: req.Name optional; default to
	// "<presetId>-<deployCounter>" so subsequent deploys of the same preset
	// don't clash. Lowercase + dash-only to satisfy K8s name conventions on
	// real downstream sources.
	s.deployCounter++
	counter := s.deployCounter
	workloadName := strings.TrimSpace(req.Name)
	if workloadName == "" {
		workloadName = fmt.Sprintf("%s-%d", req.PresetID, counter)
	}
	deployID := fmt.Sprintf("d-%d", counter)

	now := time.Now().UTC()
	wd := model.WorkloadDetail{
		Workload: model.Workload{
			Name:      workloadName,
			Namespace: req.Namespace,
			Type:      workloadTypeFromPreset(preset),
			Kind:      "Deployment",
			Status:    "pending", // accepted/pending — a real source would advance via informer
			Replicas: &model.ReplicaStatus{
				Desired: replicas,
				Ready:   0,
			},
			NodeNames: dedupSorted(scheduledNodes),
			NPUUsage: &model.WorkloadNPUUsage{
				Allocated: len(picked),
				Slices:    sliceIDs(picked),
			},
			CreatedAt: &now,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mock-deploy",
				"deploy.ocloud.io/id":          deployID,
			},
		},
	}
	// Mutate slice status now that we know the workload identity. Mark each
	// picked slice as allocated to this new workload's first pod-shaped
	// container.
	for _, sl := range picked {
		sl.Status = "allocated"
		sl.AllocatedTo = &model.SliceAllocation{
			Namespace:     req.Namespace,
			PodName:       workloadName + "-0",
			ContainerName: "main",
		}
	}

	// Append to workloads cache. If loader returned nil (no fixture file or
	// empty array) we still need a backing slice — allocate it now so
	// subsequent ListWorkloads sees the new entry.
	s.workloads = append(s.workloads, wd)

	if s.deployIndex == nil {
		s.deployIndex = make(map[string]string, 1)
	}
	s.deployIndex[deployID] = req.Namespace + "/" + workloadName

	return &model.DeployResponse{
		DeployID:       deployID,
		WorkloadName:   workloadName,
		Namespace:      req.Namespace,
		Status:         "accepted",
		ScheduledNodes: dedupSorted(scheduledNodes),
		Message:        fmt.Sprintf("workload created with %d slices (mode=%s)", len(picked), mode),
	}, nil
}

// pickSlicesLocked resolves the slices to allocate for this deploy. Must be
// called with s.deployMu held. Returns pointers into s.npus[*].Slices so the
// caller can mutate status in-place.
//
// Manual mode: each placement must reference at least one slice; every slice
// must exist in the cache AND be `available`. The number of slices supplied
// must total replicas*npuPerReplica — partial placements aren't supported
// (kept simple for the demo).
//
// Auto mode: walk every NPU in fixture order, picking the first
// replicas*npuPerReplica `available` slices. Returns ErrInsufficientCapacity
// when the fixture doesn't have enough free slices.
func (s *Source) pickSlicesLocked(mode string, replicas, npuPerReplica int, req *model.DeployRequest) ([]*model.NPUSlice, []string, error) {
	need := replicas * npuPerReplica
	if need == 0 {
		return nil, nil, fmt.Errorf("%w: nothing to schedule", ErrInvalidDeployRequest)
	}

	// Build slice id → (*NPUSlice, parent-node) index over the cache. We can't
	// safely use the flat s.slices (topology.go) here because that's a value
	// slice — mutations wouldn't reach the npu cache. The loadNPUs path
	// attached slices onto NPU.Slices by value too, so we need pointers into
	// that nested slice. Take a pointer into the underlying array via
	// indexing (s.npus[i].Slices[j]).
	type sliceRef struct {
		slice  *model.NPUSlice
		parent *model.NPU // NPU that owns the slice → NodeName
	}
	idx := make(map[string]sliceRef, 64)
	for i := range s.npus {
		npu := s.npus[i]
		if npu == nil {
			continue
		}
		for j := range npu.Slices {
			idx[npu.Slices[j].ID] = sliceRef{slice: &npu.Slices[j], parent: npu}
		}
	}

	if mode == schedulingModeManual {
		if req.Scheduling == nil || len(req.Scheduling.ManualPlacement) == 0 {
			return nil, nil, fmt.Errorf("%w: manual mode requires scheduling.manualPlacement", ErrInvalidDeployRequest)
		}
		var picked []*model.NPUSlice
		var nodes []string
		var seen = make(map[string]struct{}, need)
		for _, p := range req.Scheduling.ManualPlacement {
			for _, id := range p.NPUSliceIDs {
				if _, dup := seen[id]; dup {
					return nil, nil, fmt.Errorf("%w: duplicate slice in manualPlacement: %s",
						ErrInvalidDeployRequest, id)
				}
				seen[id] = struct{}{}
				ref, ok := idx[id]
				if !ok {
					return nil, nil, fmt.Errorf("%w: unknown sliceId %q",
						ErrInvalidDeployRequest, id)
				}
				if ref.slice.Status != "available" {
					return nil, nil, fmt.Errorf("%w: sliceId=%s currentStatus=%s",
						ErrSliceConflict, id, ref.slice.Status)
				}
				picked = append(picked, ref.slice)
				if ref.parent.NodeName != "" {
					nodes = append(nodes, ref.parent.NodeName)
				}
			}
		}
		if len(picked) != need {
			return nil, nil, fmt.Errorf("%w: manualPlacement supplied %d slices, need %d (replicas=%d * npuCount=%d)",
				ErrInvalidDeployRequest, len(picked), need, replicas, npuPerReplica)
		}
		return picked, nodes, nil
	}

	// Auto mode: greedy first-fit. Walk in fixture order so the test
	// (and the demo) sees deterministic placement.
	var picked []*model.NPUSlice
	var nodes []string
	for i := range s.npus {
		npu := s.npus[i]
		if npu == nil {
			continue
		}
		for j := range npu.Slices {
			if npu.Slices[j].Status == "available" {
				picked = append(picked, &npu.Slices[j])
				if npu.NodeName != "" {
					nodes = append(nodes, npu.NodeName)
				}
				if len(picked) == need {
					return picked, nodes, nil
				}
			}
		}
	}
	return nil, nil, fmt.Errorf("%w: need %d available slices, found %d",
		ErrInsufficientCapacity, need, len(picked))
}

// DeleteDeploy removes the workload associated with deployID from the in-
// memory cache and releases its slices back to `available`. Returns
// ErrDeployNotFound on unknown id so the handler can emit 404.
func (s *Source) DeleteDeploy(ctx context.Context, deployID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(deployID) == "" {
		return fmt.Errorf("%w: deployId is required", ErrInvalidDeployRequest)
	}
	// Loaders run outside the lock (sync.Once gated, may do disk I/O).
	if _, err := s.loadWorkloads(); err != nil {
		return err
	}
	if _, err := s.loadNPUs(); err != nil {
		return err
	}

	s.deployMu.Lock()
	defer s.deployMu.Unlock()

	key, ok := s.deployIndex[deployID]
	if !ok {
		return ErrDeployNotFound
	}
	parts := strings.SplitN(key, "/", 2)
	if len(parts) != 2 {
		// shouldn't happen — we control the key shape on Deploy
		return fmt.Errorf("mock source: malformed deploy index entry %q", key)
	}
	ns, name := parts[0], parts[1]

	// Locate + drop the workload + free its slices.
	for i := range s.workloads {
		w := &s.workloads[i]
		if w.Namespace != ns || w.Name != name {
			continue
		}
		if w.NPUUsage != nil {
			for _, sid := range w.NPUUsage.Slices {
				if ref := findSlicePtrLocked(s, sid); ref != nil {
					ref.Status = "available"
					ref.AllocatedTo = nil
				}
			}
		}
		// Drop by index — order doesn't matter, swap-with-last.
		s.workloads[i] = s.workloads[len(s.workloads)-1]
		s.workloads = s.workloads[:len(s.workloads)-1]
		delete(s.deployIndex, deployID)
		return nil
	}
	// Index points at a name no longer in the slice — fixture / test
	// out-of-band mutation; treat as 404 (already gone).
	delete(s.deployIndex, deployID)
	return ErrDeployNotFound
}

// findSlicePtrLocked returns a pointer to the NPUSlice with the given id, or
// nil if no NPU in the cache owns it. MUST be called with deployMu held.
func findSlicePtrLocked(s *Source, sliceID string) *model.NPUSlice {
	for i := range s.npus {
		if s.npus[i] == nil {
			continue
		}
		for j := range s.npus[i].Slices {
			if s.npus[i].Slices[j].ID == sliceID {
				return &s.npus[i].Slices[j]
			}
		}
	}
	return nil
}

// workloadTypeFromPreset maps preset.kind → workload.type per contract enums.
// inference / inference-pd → "inference"; benchmark stays "benchmark"; anything
// else falls through to "other" (defensible default, matches Workload.type
// enum's catchall).
func workloadTypeFromPreset(p *model.PresetDetail) string {
	if p == nil {
		return "other"
	}
	switch p.Kind {
	case "inference", "inference-pd":
		return "inference"
	case "benchmark":
		return "benchmark"
	case "training":
		return "training"
	default:
		return "other"
	}
}

// sliceIDs projects []*NPUSlice → []string of slice ids, preserving order.
func sliceIDs(ss []*model.NPUSlice) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.ID
	}
	return out
}

// dedupSorted returns in-fixture-order but deduplicated. Stable on first
// occurrence so the auto-mode tests see deterministic NodeNames lists.
func dedupSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
