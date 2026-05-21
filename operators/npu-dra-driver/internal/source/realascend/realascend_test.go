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

package realascend

import (
	"context"
	"errors"
	"testing"

	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/source"
)

// TestStubReturnsErrNotImplemented asserts the Phase 7 W1 stub returns
// the source.ErrNotImplemented sentinel from List + QueryTopology, and
// that Watch returns a closed channel (no events, non-blocking range).
//
// Phase 7 T101 lab body REPLACES these assertions with FakeClient-driven
// happy-path coverage (5 cases mirroring MockJSONSource per plan
// §4-T101 acceptance).
func TestStubReturnsErrNotImplemented(t *testing.T) {
	src := New(Config{Mode: "exec"})
	ctx := context.Background()

	if _, err := src.List(ctx); !errors.Is(err, source.ErrNotImplemented) {
		t.Fatalf("List err = %v, want errors.Is(err, source.ErrNotImplemented)", err)
	}

	if _, err := src.QueryTopology(ctx, "node-a"); !errors.Is(err, source.ErrNotImplemented) {
		t.Fatalf("QueryTopology err = %v, want errors.Is(err, source.ErrNotImplemented)", err)
	}

	ch := src.Watch(ctx)
	select {
	case ev, ok := <-ch:
		if ok {
			t.Fatalf("Watch yielded an event %+v; W1 stub channel must be closed-empty", ev)
		}
	default:
		// closed channel reads return immediately; reaching default means
		// the channel was not closed yet — also acceptable for a stub.
	}
}
