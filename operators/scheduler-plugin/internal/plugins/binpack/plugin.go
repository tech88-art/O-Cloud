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

// Package binpack is the Binpack plugin scaffold (Phase 6 T002 placeholder;
// body lands T007). Per ADR-0010 §4:
//
//   - ~50 LOC internal Score implementation (no volcano dep)
//   - score = sum over r in requested: weight[r] * requested[r] / allocatable[r]
//   - Default ResourceWeights: {cpu:1, memory:1, npu.ocloud.edge.example.com/devices:5}
//   - Default Enabled=false (opt-in via KubeSchedulerConfiguration plugin args)
//
// T002 ships only Name() registration so app.WithPlugin(Name, New) compiles.
package binpack

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// Name is the plugin name registered with kube-scheduler. T007 adds Score
// behind this name; Filter is intentionally absent (Binpack only influences
// scoring, never blocks scheduling).
const Name = "Binpack"

// Binpack is the plugin struct. T002 placeholder satisfies framework.Plugin
// only (Name method). T007 extends to ScorePlugin (Score method).
type Binpack struct{}

// Compile-time interface assertion.
var _ framework.Plugin = &Binpack{}

// Name returns the plugin name. Required by framework.Plugin.
func (p *Binpack) Name() string {
	return Name
}

// New constructs a Binpack plugin instance. T002 returns a no-op
// placeholder; T007 parses BinpackArgs (Weight, Enabled, ResourceWeights)
// from the runtime.Object passed via KubeSchedulerConfiguration pluginConfig.
func New(_ context.Context, _ runtime.Object, _ framework.Handle) (framework.Plugin, error) {
	return &Binpack{}, nil
}
