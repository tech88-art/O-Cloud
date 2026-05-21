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

// Package v1alpha1 contains the node-lifecycle-operator CRD types per
// ADR-0003 v2 IMS 7 services phasing.
//
// Phase 9 P9-T-105 ships scaffold-only (api types only · no controller
// body) per CLAUDE.md §14.2 scaffold pattern. Phase 10 controller body
// + helm chart + reconcile loops land per ADR-0003 v2 forward plan.
//
// +groupName=lifecycle.ocloud.edge.example.com
// +kubebuilder:object:generate=true
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{
		Group:   "lifecycle.ocloud.edge.example.com",
		Version: "v1alpha1",
	}
)
