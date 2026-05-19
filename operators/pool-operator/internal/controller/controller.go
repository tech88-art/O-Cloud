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

// Package controller hosts the reconcilers for the four pool CRDs
// defined under operators/pool-operator/api/v1alpha1.
//
// Phase 3 scope: scaffolding only — empty Reconcile bodies behind a
// --enable-controllers bitmask defaulting to none. Per-CRD Reconcile
// logic lands in P3-T-002..T005 + T105.
package controller

// ControllerBit identifies one Reconciler in the --enable-controllers bitmask.
type ControllerBit uint32

const (
	// BitNPUSlicePool enables the NPUSlicePool reconciler (P3-T-002).
	BitNPUSlicePool ControllerBit = 1 << iota
	// BitNPUPool enables the NPUPool reconciler (P3-T-003).
	BitNPUPool
	// BitNodePool enables the NodePool reconciler (P3-T-004).
	BitNodePool
	// BitClusterPool enables the ClusterPool reconciler (P3-T-105).
	BitClusterPool
)

// IsEnabled reports whether bit is set in the bitmask flag value.
func IsEnabled(mask uint32, bit ControllerBit) bool {
	return mask&uint32(bit) != 0
}
