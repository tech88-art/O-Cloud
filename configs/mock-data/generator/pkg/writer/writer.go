// Package writer fans a Dataset out into JSON files inside outDir.
// Field names match configs/mock-data/schema.json (camelCase, stable
// ordering). Each file is overwritten on every run.
//
// Two views of the same data are written so downstream tools have a
// choice:
//
//  1. Nine split files: meta / clusters / nodes / npus / slices /
//     workloads / pools / presets / events. Convenient for incremental
//     loading by the backend's mock.Source and for partial diffs in PR
//     review. Each split file embeds the dataset's `meta` block at the
//     top level so it remains schema-conformant on its own (the root
//     schema requires all top-level sections; split files supply empty
//     defaults for the rest). See configs/CLAUDE.md §3.2.
//
//  2. One combined `set.json` with every section. Useful for tooling
//     that wants the whole dataset in one read.
package writer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/ocloud-edge/configs/mock-data/generator/pkg/model"
)

// emptyPools returns the pools object with all four required slices
// initialised to empty arrays. Schema requires the pools key to exist
// with cluster/node/npu/npuSlice arrays even when they're empty.
func emptyPools() model.Pools {
	return model.Pools{
		Cluster:  []model.ClusterPool{},
		Node:     []model.NodePool{},
		NPU:      []model.NPUPool{},
		NPUSlice: []model.NPUSlicePool{},
	}
}

// section wraps each split file in the full top-level Dataset shape so
// it validates against the root schema even when only one section is
// populated. The `meta` block is replicated into every file (small cost,
// big win in tooling).
//
// `Events` is the only optional top-level key in the schema; we still
// emit `[]` when the section is empty to keep the shape stable.
func section(ds *model.Dataset, populate func(out *model.Dataset)) *model.Dataset {
	out := &model.Dataset{
		Meta:      ds.Meta,
		Clusters:  []model.Cluster{},
		Nodes:     []model.Node{},
		NPUs:      []model.NPU{},
		Slices:    []model.Slice{},
		Workloads: []model.Workload{},
		Pools:     emptyPools(),
		Presets:   []model.Preset{},
		Events:    []model.Event{},
	}
	populate(out)
	// Clusters is required to have minItems=1, so for non-cluster files
	// we re-inject the cluster list (it's small) to stay valid. This is
	// cheap and keeps every split file independently loadable.
	if len(out.Clusters) == 0 {
		out.Clusters = ds.Clusters
	}
	return out
}

// Write emits ten JSON files under outDir: the nine split files plus a
// combined set.json. outDir is created (mkdir -p) if missing.
func Write(outDir string, ds *model.Dataset) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outDir, err)
	}

	files := []struct {
		name string
		data any
	}{
		{"meta.json", section(ds, func(o *model.Dataset) {})},
		{"clusters.json", section(ds, func(o *model.Dataset) { o.Clusters = ds.Clusters })},
		{"nodes.json", section(ds, func(o *model.Dataset) { o.Nodes = ds.Nodes })},
		{"npus.json", section(ds, func(o *model.Dataset) { o.NPUs = ds.NPUs })},
		{"slices.json", section(ds, func(o *model.Dataset) { o.Slices = ds.Slices })},
		{"workloads.json", section(ds, func(o *model.Dataset) { o.Workloads = ds.Workloads })},
		{"pools.json", section(ds, func(o *model.Dataset) { o.Pools = ds.Pools })},
		{"presets.json", section(ds, func(o *model.Dataset) { o.Presets = ds.Presets })},
		{"events.json", section(ds, func(o *model.Dataset) { o.Events = ds.Events })},
	}

	for _, f := range files {
		path := filepath.Join(outDir, f.name)
		if err := writeJSON(path, f.data); err != nil {
			return fmt.Errorf("write %s: %w", f.name, err)
		}
	}
	return nil
}

func writeJSON(path string, v any) error {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	// Append trailing newline for POSIX-friendly diffs.
	buf = append(buf, '\n')
	return os.WriteFile(path, buf, 0o644)
}
