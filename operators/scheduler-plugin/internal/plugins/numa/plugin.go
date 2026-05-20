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

// Package numa is the NumaAffinity plugin scaffold (Phase 6 T002 placeholder;
// body lands T006 by wrapping sigs.k8s.io/scheduler-plugins/pkg/
// noderesourcetopology). Per ADR-0010 §3:
//
//   - No Ascend-specific logic — pure host-level NUMA scheduling
//   - Wrap upstream (don't fork) to keep upgrade path open
//   - Local plugin name "NumaAffinity" rebrands upstream
//     "NodeResourceTopologyMatch" for chart readability
//
// T002 ships only Name() registration so app.WithPlugin(Name, New) compiles.
package numa

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// Name is the plugin name registered with kube-scheduler. T006 binds this
// name to upstream noderesourcetopology Filter+Score implementations.
const Name = "NumaAffinity"

// NumaAffinity is the plugin struct. T002 placeholder satisfies
// framework.Plugin only (Name method). T006 extends to FilterPlugin +
// ScorePlugin via embedded upstream impl.
type NumaAffinity struct{}

// Compile-time interface assertion.
var _ framework.Plugin = &NumaAffinity{}

// Name returns the plugin name. Required by framework.Plugin.
func (p *NumaAffinity) Name() string {
	return Name
}

// New constructs a NumaAffinity plugin instance. T002 returns a no-op
// placeholder; T006 calls into
// sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology to build the real
// FilterPlugin + ScorePlugin and wraps the result with the local plugin Name.
func New(_ context.Context, _ runtime.Object, _ framework.Handle) (framework.Plugin, error) {
	return &NumaAffinity{}, nil
}
