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

package metrics

import (
	"context"
	"sync"
)

// FakeIngestor returns canned IngestorResult values per call. Phase 8
// T007 NPUVerticalScaler controller tests use this to drive Reconcile
// loop branches (busy threshold cross → patch / idle → patch / cooldown
// active blocks scaling / scaleHistory rotation / etc.) without needing
// a real Prometheus.
//
// Compile-time interface assertion is at the bottom of this file.
type FakeIngestor struct {
	mu      sync.Mutex
	results []IngestorResult
	idx     int
	queries []IngestorQuery
}

// NewFakeIngestor constructs a FakeIngestor seeded with the given canned
// results. Subsequent Query() calls consume them in order; once exhausted,
// further calls return the last seeded result (so a single canned result
// works for "always return X" tests).
//
// Pass nil/empty to get an ingestor that always returns
// IngestorResult{NoData: true}.
func NewFakeIngestor(canned []IngestorResult) *FakeIngestor {
	return &FakeIngestor{results: canned}
}

// Query returns the next canned result; records the query for assertions.
func (f *FakeIngestor) Query(_ context.Context, q IngestorQuery) IngestorResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, q)
	if len(f.results) == 0 {
		return IngestorResult{NoData: true}
	}
	if f.idx >= len(f.results) {
		// Saturate at the last canned result.
		return f.results[len(f.results)-1]
	}
	r := f.results[f.idx]
	f.idx++
	return r
}

// CalledWith returns a snapshot of all queries received in call order.
// Tests use this to assert the controller passes the right namespace +
// model_service + windowSeconds.
func (f *FakeIngestor) CalledWith() []IngestorQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]IngestorQuery, len(f.queries))
	copy(out, f.queries)
	return out
}

// Reset clears the call counter + recorded queries; useful between
// reconcile-loop assertions in a multi-step test case.
func (f *FakeIngestor) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.idx = 0
	f.queries = nil
}

// Compile-time interface assertion (P8-T-006 acceptance: FakeIngestor
// implements Ingestor interface).
var _ Ingestor = (*FakeIngestor)(nil)
