/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package v1alpha1 contains the bare-metal-provisioning-operator CRD
// types per ADR-0003 v2 IMS 7 services phasing.
//
// Phase 9 P9-T-105 ships scaffold-only · controller body deferred to
// Phase 10 per CLAUDE.md §14.2 scaffold pattern.
//
// +groupName=provisioning.ocloud.edge.example.com
// +kubebuilder:object:generate=true
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion = schema.GroupVersion{
		Group:   "provisioning.ocloud.edge.example.com",
		Version: "v1alpha1",
	}
)
