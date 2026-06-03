package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// templateDef mirrors mock.templateDef but with the value-synthesis
// fields trimmed — the real Prometheus is the value-of-truth here.
// promQL carries the `$var` placeholders that QueryMetric substitutes
// before forwarding to the upstream HTTP API.
type templateDef struct {
	id       string
	promQL   string
	required []string // names of vars that MUST be present in vars[]
}

// templates is the same white-list mock.Source advertises (P1-T-204
// id strings), kept in sync by inspection. ADR-0006 / contract regen
// did NOT touch this list; if a new template ships we extend BOTH
// mock + prometheus together (Phase 3 work to unify into a shared
// pkg is tracked in known-issues / phase2-plan §"capability vs
// content").
var templates = []templateDef{
	{
		id:       "node_cpu_util",
		promQL:   `100 - (avg by (instance) (rate(node_cpu_seconds_total{mode="idle",instance="$node"}[1m])) * 100)`,
		required: []string{"node"},
	},
	{
		id:       "node_mem_util",
		promQL:   `(1 - node_memory_MemAvailable_bytes{instance="$node"} / node_memory_MemTotal_bytes{instance="$node"}) * 100`,
		required: []string{"node"},
	},
	{
		id:     "npu_aicore_util",
		promQL: `ascend_npu_aicore_utilization{instance="$node",npu_id="$npu",slice_id="$slice"}`,
	},
	{
		id:     "npu_vram_util",
		promQL: `ascend_npu_vram_utilization{instance="$node",npu_id="$npu",slice_id="$slice"}`,
	},
	{
		id:     "npu_bandwidth_util",
		promQL: `ascend_npu_bandwidth_utilization{instance="$node",npu_id="$npu",slice_id="$slice"}`,
	},
	{
		id:       "workload_throughput",
		promQL:   `sum by (workload) (rate(mindie_tokens_total{workload="$workload"}[1m]))`,
		required: []string{"workload"},
	},
	{
		id:       "workload_ttft",
		promQL:   `histogram_quantile(0.95, sum by (le, workload) (rate(mindie_ttft_seconds_bucket{workload="$workload"}[1m]))) * 1000`,
		required: []string{"workload"},
	},
	{
		id:       "workload_itl",
		promQL:   `histogram_quantile(0.95, sum by (le, workload) (rate(mindie_itl_seconds_bucket{workload="$workload"}[1m]))) * 1000`,
		required: []string{"workload"},
	},
	{
		id:       "workload_e2e_latency",
		promQL:   `histogram_quantile(0.95, sum by (le, workload) (rate(mindie_e2e_seconds_bucket{workload="$workload"}[1m]))) * 1000`,
		required: []string{"workload"},
	},
}

var templatesByID = func() map[string]*templateDef {
	m := make(map[string]*templateDef, len(templates))
	for i := range templates {
		m[templates[i].id] = &templates[i]
	}
	return m
}()

// QueryMetric expands the named template with the supplied vars,
// posts /api/v1/query_range against the configured Prometheus URL,
// and returns the matrix as model.MetricQueryResponse.
//
// Errors:
//   - ErrUnknownTemplate      — templateID not in white-list (400)
//   - ErrMissingVariable      — required var absent in vars       (400)
//   - ctx.Err()               — context cancelled / timed out
//   - ErrUpstream wrapped     — non-2xx Prometheus response       (502)
//   - any other plumbing err  — caller renders 500
//
// Default time range when timeRange is zero: last 5 minutes ending
// now, step=30s. The frontend always passes a range today (see
// services/metrics.ts), so this default is mostly a defensive
// fallback for ad-hoc REST callers.
func (s *Source) QueryMetric(ctx context.Context, templateID string, vars map[string]string, timeRange model.TimeRange) (*model.MetricQueryResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tpl, ok := templatesByID[templateID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownTemplate, templateID)
	}
	for _, name := range tpl.required {
		if v := vars[name]; v == "" {
			return nil, fmt.Errorf("%w: %s", ErrMissingVariable, name)
		}
	}

	expanded := expandPromQL(tpl.promQL, vars)
	start, end, step := resolveTimeRange(timeRange)

	q := url.Values{}
	q.Set("query", expanded)
	q.Set("start", start)
	q.Set("end", end)
	q.Set("step", step)
	endpoint := s.url + "/api/v1/query_range?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("prometheus: build request: %w", err)
	}
	// Resolve the bearer token per-request: the in-cluster ServiceAccount
	// token is auto-rotated by the kubelet, so a token cached once at
	// startup would go stale (P13-T-201 / ADR-0025 §2 Decision A). The
	// fileTokenSource bounds the disk reads with a short TTL.
	token, err := s.tokens.Token()
	if err != nil {
		return nil, fmt.Errorf("prometheus: load bearer token: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prometheus: http call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("%w: status=%d body=%s", ErrUpstream, resp.StatusCode, string(body))
	}

	return parsePromResponse(resp.Body)
}

// expandPromQL substitutes `$<var>` tokens with the value from vars.
// Missing optional vars (npu/slice/...) are replaced with `.*`
// regex so the Prometheus query degrades to a "filter ignored"
// shape instead of `label="empty"` which would match nothing.
//
// Substitution is left-to-right and non-overlapping; var names are
// taken from the union of (vars map keys) and the template's required
// list to keep behaviour predictable.
func expandPromQL(promQL string, vars map[string]string) string {
	// Collect candidate var names. Order doesn't matter — substitution
	// of "$node" doesn't introduce another "$".
	out := promQL
	// Walk the string finding every `$<word>`.
	for {
		idx := strings.IndexByte(out, '$')
		if idx < 0 {
			break
		}
		// Collect var name following the '$'.
		end := idx + 1
		for end < len(out) {
			c := out[end]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
				end++
				continue
			}
			break
		}
		name := out[idx+1 : end]
		var replacement string
		if v, ok := vars[name]; ok && v != "" {
			replacement = v
		} else {
			// `.*` so the upstream Prometheus regex match drops the
			// constraint cleanly; preferring "filter ignored" over
			// "filter matches nothing".
			replacement = ".*"
		}
		out = out[:idx] + replacement + out[end:]
	}
	return out
}

// resolveTimeRange fills in defaults when the caller didn't pass an
// explicit range. Step defaults to 30s; start/end default to the last
// 5 minutes ending at server-now.
func resolveTimeRange(tr model.TimeRange) (start, end, step string) {
	step = tr.Step
	if step == "" {
		step = "30s"
	}
	now := time.Now().UTC()
	if tr.End == "" {
		end = strconvI64(now.Unix())
	} else {
		end = tr.End
	}
	if tr.Start == "" {
		start = strconvI64(now.Add(-5 * time.Minute).Unix())
	} else {
		start = tr.Start
	}
	return
}

// strconvI64 — int64 → ASCII decimal. We avoid strconv to keep this
// file tight; the values are always positive Unix seconds so the
// conversion is straight.
func strconvI64(v int64) string {
	if v == 0 {
		return "0"
	}
	// 21 chars covers int64 range plus sign.
	buf := [21]byte{}
	pos := len(buf)
	for v > 0 {
		pos--
		buf[pos] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[pos:])
}

// promResponse is the subset of Prometheus's /api/v1/query_range
// response we care about. Matrix result is the only shape we accept
// (it's what query_range returns); vector / scalar are rejected via
// the missing fields silently producing an empty result list.
type promResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]any           `json:"values"`
		} `json:"result"`
	} `json:"data"`
	ErrorType string `json:"errorType,omitempty"`
	Error     string `json:"error,omitempty"`
}

// parsePromResponse decodes the JSON body and maps it onto the
// model.MetricQueryResponse shape. Status != "success" → ErrUpstream
// wrapped with the error text.
func parsePromResponse(r io.Reader) (*model.MetricQueryResponse, error) {
	var pr promResponse
	dec := json.NewDecoder(r)
	if err := dec.Decode(&pr); err != nil {
		return nil, fmt.Errorf("prometheus: decode body: %w", err)
	}
	if pr.Status != "success" {
		return nil, fmt.Errorf("%w: %s: %s", ErrUpstream, pr.ErrorType, pr.Error)
	}
	out := &model.MetricQueryResponse{
		Result: make([]model.MetricSeries, 0, len(pr.Data.Result)),
	}
	for _, r := range pr.Data.Result {
		out.Result = append(out.Result, model.MetricSeries{
			Metric: r.Metric,
			Values: r.Values,
		})
	}
	return out, nil
}
