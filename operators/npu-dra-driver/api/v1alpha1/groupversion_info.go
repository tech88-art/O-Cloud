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

// Package v1alpha1 — group registration entrypoint.
//
// Phase 5 T004 ships the first Ocloud CRD in this package
// (NPUSliceAllocation). Phase 4 deliberately omitted this file because
// no CRD existed yet — the AscendDevice / AscendClaimAnnotations
// helpers are typed projections of upstream resource.k8s.io types and
// do not require a SchemeBuilder of their own. With T004 we register
// the group so the manager can read/write NPUSliceAllocation through
// controller-runtime's typed client.
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// SchemeBuilder is used to add this group's types to a runtime.Scheme.
//
// GroupVersion itself is declared in types.go (next to DriverName)
// because it pre-dates this file (Phase 4 introduced it to anchor the
// `+groupName=` controller-gen directive). T004 keeps that declaration
// authoritative and only adds the SchemeBuilder + AddToScheme wiring.
var SchemeBuilder = &scheme.Builder{
	GroupVersion: schema.GroupVersion{Group: GroupVersion.Group, Version: GroupVersion.Version},
}

// AddToScheme registers all known v1alpha1 types onto the supplied Scheme.
// Callers (cmd/main.go init, test suites) must invoke this so the
// manager's client knows about NPUSliceAllocation.
var AddToScheme = SchemeBuilder.AddToScheme
