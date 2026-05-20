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

package numa

import "testing"

// TestNameConstants asserts the registration name and the upstream name
// constants stay in sync with ADR-0010 §3 + the upstream package's
// internal Name(). If upstream renames its plugin, this test should be
// updated together with `nrt` import bump in plugin.go.
func TestNameConstants(t *testing.T) {
	if Name != "NumaAffinity" {
		t.Fatalf("Name = %q, want \"NumaAffinity\" (ADR-0010 §3 rebrand)", Name)
	}
	if UpstreamName != "NodeResourceTopologyMatch" {
		t.Fatalf("UpstreamName = %q, want \"NodeResourceTopologyMatch\" (upstream constant)", UpstreamName)
	}
}

// TestNewProducesPlugin invokes the placeholder factory and asserts a
// non-nil framework.Plugin comes back with the correct Name(). When the
// upstream wrap lands (see plugin.go package doc), this test should be
// updated to also exercise the upstream Filter/Score paths.
func TestNewProducesPlugin(t *testing.T) {
	p, err := New(nil, nil, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if p == nil {
		t.Fatal("New returned nil plugin")
	}
	if p.Name() != Name {
		t.Fatalf("Name = %q, want %q", p.Name(), Name)
	}
}
