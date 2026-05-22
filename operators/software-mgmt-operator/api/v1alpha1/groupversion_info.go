/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package v1alpha1 contains the software-mgmt-operator CRD types per
// ADR-0003 v2 IMS 7 services phasing.
//
// Phase 9 P9-T-105 ships scaffold-only · Phase 10 P10-T-008 lands
// controller body · Phase 11 P11-T-005 lands chart + cmd controller-
// runtime wire (per ADR-0017 §2 Decision D 3rd).
//
// +groupName=softwaremgmt.ocloud.edge.example.com
// +kubebuilder:object:generate=true
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion = schema.GroupVersion{
		Group:   "softwaremgmt.ocloud.edge.example.com",
		Version: "v1alpha1",
	}

	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(&SoftwareBundle{}, &SoftwareBundleList{})
}

func Resource(resource string) schema.GroupResource {
	return GroupVersion.WithResource(resource).GroupResource()
}

var _ = runtime.Object(&SoftwareBundle{})
