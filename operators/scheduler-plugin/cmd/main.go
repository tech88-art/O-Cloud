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

// Package main is the scheduler-plugin entrypoint. It wraps the upstream
// kube-scheduler command via app.NewSchedulerCommand and registers our
// HCCSTopology + NumaAffinity + Binpack plugins. Per ADR-0010 §1 the binary
// runs as a standalone kube-scheduler (Pods opt in via spec.schedulerName);
// the default scheduler is unaffected.
//
// Phase 6 T002 (this commit) ships the scaffold:
//   - Plugin factories registered, each returning a placeholder Plugin whose
//     Name() satisfies framework.Plugin.
//   - No Filter/Score body — bodies arrive T004 (HCCS Filter), T005 (HCCS
//     Score), T006 (NUMA wrap), T007 (Binpack).
//   - `--help` prints upstream kube-scheduler flags + ConfigMap path; the
//     custom plugin args (HCCSTopologyArgs/NumaAffinityArgs/BinpackArgs)
//     are introduced T004-T007 alongside the plugin bodies.
package main

import (
	"os"

	"k8s.io/component-base/cli"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"

	"github.com/tech88-art/O-Cloud/operators/scheduler-plugin/internal/plugins/binpack"
	"github.com/tech88-art/O-Cloud/operators/scheduler-plugin/internal/plugins/hccs"
	"github.com/tech88-art/O-Cloud/operators/scheduler-plugin/internal/plugins/numa"
)

func main() {
	// Out-of-tree plugin registry per upstream pattern. Each plugin's New
	// constructor is registered with the kube-scheduler app so the binary
	// can be invoked with a KubeSchedulerConfiguration that references
	// these plugin names in its profile plugins.{filter,score}.enabled[]
	// lists. T004-T007 fill in the bodies; T002 placeholders just register
	// so `kube-scheduler --help` works.
	command := app.NewSchedulerCommand(
		app.WithPlugin(hccs.Name, hccs.New),
		app.WithPlugin(numa.Name, numa.New),
		app.WithPlugin(binpack.Name, binpack.New),
	)

	code := cli.Run(command)
	os.Exit(code)
}
