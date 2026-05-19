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

package webhook

import "testing"

func TestEncodeBindings_DeterministicSort(t *testing.T) {
	// Same bindings, different input order — encoded value must be
	// identical because the encoder sorts.
	a := []SliceBinding{
		{Node: "nodeB", Pool: "nodeB", Device: "nodeB-npu-0", AICores: 16, Phase: phaseAllocated},
		{Node: "nodeA", Pool: "nodeA", Device: "nodeA-npu-1", AICores: 32, Phase: phaseAllocated},
		{Node: "nodeA", Pool: "nodeA", Device: "nodeA-npu-0", AICores: 32, Phase: phaseAllocated},
	}
	b := []SliceBinding{
		{Node: "nodeA", Pool: "nodeA", Device: "nodeA-npu-0", AICores: 32, Phase: phaseAllocated},
		{Node: "nodeA", Pool: "nodeA", Device: "nodeA-npu-1", AICores: 32, Phase: phaseAllocated},
		{Node: "nodeB", Pool: "nodeB", Device: "nodeB-npu-0", AICores: 16, Phase: phaseAllocated},
	}
	got1 := EncodeBindings(a)
	got2 := EncodeBindings(b)
	if got1 != got2 {
		t.Errorf("encoder non-deterministic: %q vs %q", got1, got2)
	}
	want := "nodeA/nodeA/nodeA-npu-0:32,nodeA/nodeA/nodeA-npu-1:32,nodeB/nodeB/nodeB-npu-0:16"
	if got1 != want {
		t.Errorf("encoded value: want %q, got %q", want, got1)
	}
}

func TestEncodeBindings_Empty(t *testing.T) {
	if got := EncodeBindings(nil); got != "" {
		t.Errorf("nil → empty; got %q", got)
	}
	if got := EncodeBindings([]SliceBinding{}); got != "" {
		t.Errorf("empty slice → empty; got %q", got)
	}
}

func TestFilterAllocated_DropsNonAllocated(t *testing.T) {
	in := []SliceBinding{
		{Device: "a", Phase: phaseAllocated},
		{Device: "b", Phase: phaseOrphaned},
		{Device: "c", Phase: phaseAllocated},
		{Device: "d", Phase: ""},
	}
	out := FilterAllocated(in)
	if len(out) != 2 {
		t.Errorf("expected 2 Allocated; got %d (%+v)", len(out), out)
	}
	if out[0].Device != "a" || out[1].Device != "c" {
		t.Errorf("filter preserved wrong items: %+v", out)
	}
}

func TestCountByPhase(t *testing.T) {
	in := []SliceBinding{
		{Phase: phaseAllocated},
		{Phase: phaseAllocated},
		{Phase: phaseOrphaned},
		{Phase: phaseReleased},
		{Phase: ""},
	}
	a, o, x := CountByPhase(in)
	if a != 2 {
		t.Errorf("allocated: want 2, got %d", a)
	}
	if o != 1 {
		t.Errorf("orphaned: want 1, got %d", o)
	}
	if x != 2 {
		t.Errorf("other (released + empty): want 2, got %d", x)
	}
}
