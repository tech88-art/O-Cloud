/*
Copyright 2026.

Licensed under the Apache License, Version 2.0.
*/

// Package v1alpha1 contains the bare-metal-provisioning-operator CRD
// types per ADR-0003 v2 IMS 7 services phasing.
//
// Phase 9 P9-T-105 scaffold · Phase 10 P10-T-101 controller body ·
// Phase 11 P11-T-006 chart + cmd controller-runtime wire + Redfish/IPMI
// stub client + Secret 解析(per ADR-0017 §2 Decision D 4th).
//
// +groupName=provisioning.ocloud.edge.example.com
// +kubebuilder:object:generate=true
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion = schema.GroupVersion{
		Group:   "provisioning.ocloud.edge.example.com",
		Version: "v1alpha1",
	}

	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(&BareMetalNode{}, &BareMetalNodeList{})
}

func Resource(resource string) schema.GroupResource {
	return GroupVersion.WithResource(resource).GroupResource()
}

var _ = runtime.Object(&BareMetalNode{})
