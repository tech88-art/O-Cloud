/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package hccs is the HCCSTopologyPlugin scaffold (Phase 6 T002 placeholder;
// Filter body lands T004; Score body lands T005). Per ADR-0010 §2:
//
//   - Filter reads ResourceSlice device attributes `npu.huawei.com/hccs_ring`
//     and matches against Pod's `npu.huawei.com/preferred-hccs-ring` annotation
//   - Score reads NPUSliceAllocation reverse-lookup to find sibling Pods of
//     the same ModelService and rewards co-location on the same HCCS ring
//
// T002 ships only Name() registration so app.WithPlugin(Name, New) compiles.
package hccs

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// Name is the plugin name registered with kube-scheduler. KubeSchedulerConfig
// profiles[*].plugins.{filter,score}.enabled[].name MUST match this string.
const Name = "HCCSTopology"

// HCCSTopology is the plugin struct. T002 placeholder satisfies framework.Plugin
// only (Name method). T004 adds FilterPlugin (Filter method); T005 adds
// ScorePlugin (Score method) and may add ScoreExtensions (NormalizeScore).
type HCCSTopology struct{}

// Compile-time interface assertion. T004/T005 extend to FilterPlugin +
// ScorePlugin once the bodies land.
var _ framework.Plugin = &HCCSTopology{}

// Name returns the plugin name. Required by framework.Plugin.
func (p *HCCSTopology) Name() string {
	return Name
}

// New constructs an HCCSTopology plugin instance. T002 returns a no-op
// placeholder; T004 wires args parsing + handle for ResourceSlice listing;
// T005 adds NPUSliceAllocation dynamic-client lister.
//
// Signature matches kube-scheduler's runtime.PluginFactory contract
// (sigs.k8s.io/scheduler-plugins v0.31.x · K8s 1.32 baseline). The
// context.Context arg replaces the older v0.30 args-and-handle-only
// signature per upstream PluginFactoryWithFts evolution.
func New(_ context.Context, _ runtime.Object, _ framework.Handle) (framework.Plugin, error) {
	return &HCCSTopology{}, nil
}
