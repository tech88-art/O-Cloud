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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestPrometheusIngestor_StubServerWindowAverage covers P8-T-006 case 1/5:
// happy path against a fake Prometheus /api/v1/query endpoint returning a
// single vector entry — PrometheusIngestor unwraps it into IngestorResult{
// Value: ..., NoData: false}.
func TestPrometheusIngestor_StubServerWindowAverage(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		fmt.Fprintln(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"namespace":"ai-edge-demo","model_service":"qwen-pd"},"value":[1750000000,"78.5"]}]}}`)
	}))
	defer srv.Close()

	ing := NewPrometheusIngestor(IngestorOpts{PrometheusURL: srv.URL})
	res := ing.Query(context.Background(), IngestorQuery{
		Namespace:     "ai-edge-demo",
		ModelService:  "qwen-pd",
		WindowSeconds: 300,
	})
	if res.Err != nil {
		t.Fatalf("Query err: %v", res.Err)
	}
	if res.NoData {
		t.Fatalf("Query NoData=true unexpected")
	}
	if res.Value != 78.5 {
		t.Fatalf("Query value = %v, want 78.5", res.Value)
	}
	wantPromQL := `avg_over_time(ascend_npu_utilization_percent{namespace="ai-edge-demo", model_service="qwen-pd"}[300s])`
	if capturedQuery != wantPromQL {
		t.Fatalf("PromQL shape mismatch:\n  got:  %q\n  want: %q", capturedQuery, wantPromQL)
	}
}

// TestPrometheusIngestor_EmptyVectorIsNoData covers P8-T-006 case 2/5:
// Prometheus returns success + empty result → IngestorResult{NoData: true,
// Err: nil}. Per ADR-0012 §1 reconcile step 3 contract.
func TestPrometheusIngestor_EmptyVectorIsNoData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	}))
	defer srv.Close()

	ing := NewPrometheusIngestor(IngestorOpts{PrometheusURL: srv.URL})
	res := ing.Query(context.Background(), IngestorQuery{
		Namespace:     "ai-edge-demo",
		ModelService:  "qwen-pd",
		WindowSeconds: 300,
	})
	if res.Err != nil {
		t.Fatalf("Query err: %v", res.Err)
	}
	if !res.NoData {
		t.Fatalf("Query NoData=false, want true (empty vector)")
	}
	if res.Value != 0 {
		t.Fatalf("Query value = %v, want 0 on NoData", res.Value)
	}
}

// TestPrometheusIngestor_EmptyURLIsNoData covers P8-T-006 case 3/5:
// PrometheusURL unset → IngestorResult{NoData: true} without performing
// any HTTP I/O. Mirrors the chart `metrics.prometheusURL=""` operator
// degraded-mode case.
func TestPrometheusIngestor_EmptyURLIsNoData(t *testing.T) {
	ing := NewPrometheusIngestor(IngestorOpts{})
	res := ing.Query(context.Background(), IngestorQuery{
		Namespace:     "ai-edge-demo",
		ModelService:  "qwen-pd",
		WindowSeconds: 300,
	})
	if res.Err != nil {
		t.Fatalf("Query err: %v", res.Err)
	}
	if !res.NoData {
		t.Fatalf("Query NoData=false, want true (empty PrometheusURL)")
	}
}

// TestPrometheusIngestor_TransportErrorFoldsToNoData covers P8-T-006 case
// 4/5: network unreachable / DNS failure / connection-refused → folded
// into NoData=true (NOT Err) per ADR-0012 §1 reconcile step 3 contract.
// Implemented via a server we close BEFORE the request.
func TestPrometheusIngestor_TransportErrorFoldsToNoData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Won't actually serve — we close first.
		fmt.Fprintln(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	}))
	addr := srv.URL
	srv.Close() // Close immediately so connection is refused.

	ing := NewPrometheusIngestor(IngestorOpts{
		PrometheusURL: addr,
		QueryTimeout:  500 * time.Millisecond,
	})
	res := ing.Query(context.Background(), IngestorQuery{
		Namespace:     "ai-edge-demo",
		ModelService:  "qwen-pd",
		WindowSeconds: 300,
	})
	if res.Err != nil {
		t.Fatalf("Query err = %v, want nil (connection error should fold into NoData)", res.Err)
	}
	if !res.NoData {
		t.Fatalf("Query NoData=false, want true (connection refused)")
	}
}

// TestPrometheusIngestor_4xxReturnsErr covers P8-T-006 case 5/5: 4xx
// status (typically 400 invalid PromQL) → IngestorResult{Err: ...} so the
// controller surfaces the bug instead of silently treating it as NoData.
// 5xx is folded into NoData (transient); only 4xx is genuine.
func TestPrometheusIngestor_4xxReturnsErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"status":"error","errorType":"bad_data","error":"parse error: missing label"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	ing := NewPrometheusIngestor(IngestorOpts{PrometheusURL: srv.URL})
	res := ing.Query(context.Background(), IngestorQuery{
		Namespace:     "ai-edge-demo",
		ModelService:  "qwen-pd",
		WindowSeconds: 300,
	})
	if res.Err == nil {
		t.Fatalf("Query err=nil, want non-nil on 4xx response")
	}
	if !strings.Contains(res.Err.Error(), "400") {
		t.Fatalf("Query err message %q does not mention status 400", res.Err.Error())
	}
	if res.NoData {
		t.Fatalf("Query NoData=true on 4xx, want false (4xx is a genuine bug)")
	}
}

// TestFakeIngestor_ImplementsInterface ensures FakeIngestor satisfies the
// Ingestor contract at compile-time + behaves per spec when called.
// Bonus coverage on top of the 5 required cases.
func TestFakeIngestor_ImplementsInterface(t *testing.T) {
	// Compile-time guarantee is in fake_ingestor.go (`var _ Ingestor =
	// (*FakeIngestor)(nil)`); this test guards against runtime semantics
	// regression.
	canned := []IngestorResult{
		{Value: 50.0},
		{Value: 80.0},
		{NoData: true},
	}
	f := NewFakeIngestor(canned)

	q := IngestorQuery{Namespace: "ns", ModelService: "ms", WindowSeconds: 300}
	r1 := f.Query(context.Background(), q)
	r2 := f.Query(context.Background(), q)
	r3 := f.Query(context.Background(), q)
	r4 := f.Query(context.Background(), q) // exhausted → saturates at last canned

	if r1.Value != 50.0 || r2.Value != 80.0 || !r3.NoData || !r4.NoData {
		t.Fatalf("FakeIngestor canned sequence mismatch: %+v %+v %+v %+v",
			r1, r2, r3, r4)
	}
	calls := f.CalledWith()
	if len(calls) != 4 {
		t.Fatalf("FakeIngestor CalledWith len = %d, want 4", len(calls))
	}
	if calls[0].Namespace != "ns" || calls[0].ModelService != "ms" || calls[0].WindowSeconds != 300 {
		t.Fatalf("FakeIngestor.CalledWith[0] mismatch: %+v", calls[0])
	}

	f.Reset()
	if len(f.CalledWith()) != 0 {
		t.Fatalf("FakeIngestor.Reset did not clear queries")
	}
}

// TestWindowedAverage_EmptyIsNoData covers the WindowedAverage helper —
// empty input → NoData=true. Tiny but exercises an exported helper that
// the production ingestor uses to fold multi-series results.
func TestWindowedAverage_EmptyIsNoData(t *testing.T) {
	if res := WindowedAverage(nil); !res.NoData {
		t.Fatalf("WindowedAverage(nil) NoData=%v, want true", res.NoData)
	}
	if res := WindowedAverage([]MetricSample{}); !res.NoData {
		t.Fatalf("WindowedAverage(empty) NoData=%v, want true", res.NoData)
	}
	samples := []MetricSample{{Value: 40}, {Value: 60}}
	res := WindowedAverage(samples)
	if res.NoData {
		t.Fatal("WindowedAverage(non-empty) NoData=true, want false")
	}
	if res.Value != 50.0 {
		t.Fatalf("WindowedAverage value = %v, want 50.0", res.Value)
	}
}

// TestPrometheusIngestor_CustomPromQL covers P9-T-007 acceptance case 1/2
// (ingestor): when IngestorQuery.CustomPromQL is non-empty, the ingestor
// sends it verbatim to /api/v1/query (no namespace/model_service label
// injection · operator owns label scoping). ADR-0012 §7 forward note.
func TestPrometheusIngestor_CustomPromQL(t *testing.T) {
	const wantPromQL = `avg_over_time(custom_kv_cache_hit_rate{model_service="qwen-pd"}[5m])`
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		fmt.Fprintln(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"model_service":"qwen-pd"},"value":[1750000000,"0.87"]}]}}`)
	}))
	defer srv.Close()

	ing := NewPrometheusIngestor(IngestorOpts{PrometheusURL: srv.URL})
	res := ing.Query(context.Background(), IngestorQuery{
		CustomPromQL: wantPromQL,
	})
	if res.Err != nil {
		t.Fatalf("Query err: %v", res.Err)
	}
	if res.NoData {
		t.Fatalf("Query NoData=true unexpected for non-empty result")
	}
	if res.Value != 0.87 {
		t.Fatalf("Query value = %v, want 0.87", res.Value)
	}
	if capturedQuery != wantPromQL {
		t.Fatalf("custom PromQL mismatch:\n  got:  %q\n  want: %q", capturedQuery, wantPromQL)
	}
}

// TestPrometheusIngestor_CustomPromQLInvalid4xx covers P9-T-007 acceptance
// case 2/2 (ingestor): invalid PromQL returns 400 from Prometheus → Err
// surfaced (NOT NoData · 4xx is a genuine bug per ADR-0012 §1 reconcile
// step 3 contract).
func TestPrometheusIngestor_CustomPromQLInvalid4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, `parse error: unexpected end of input`)
	}))
	defer srv.Close()

	ing := NewPrometheusIngestor(IngestorOpts{PrometheusURL: srv.URL})
	res := ing.Query(context.Background(), IngestorQuery{
		CustomPromQL: `this_is_not_valid_promql{`,
	})
	if res.Err == nil {
		t.Fatalf("Query Err=nil; want non-nil for 4xx invalid PromQL")
	}
	if res.NoData {
		t.Fatalf("Query NoData=true; 4xx should surface Err not NoData")
	}
}
