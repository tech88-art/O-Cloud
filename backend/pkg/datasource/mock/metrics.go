// Package mock — metrics fixtures (P1-T-204, RFC-003 hardened with var-slice).
//
// Synthesizes deterministic time series for a closed white-list of PromQL
// templates. Templates cover node / npu / workload dimensions plus the
// **var-slice** dimension (spec F4a "最细切分粒度的信息") that the AC for
// P1-T-204 demands.
//
// Determinism: each series is seeded by sha256(templateId + sorted-vars-JSON)
// so the same `(templateId, vars)` pair always yields the same series within
// a process. Tests can therefore assert specific sample values.
//
// Mock series shape:
//   - 200 samples per series, evenly spaced across [start, end]
//   - timestamps are unix-seconds floats (Prometheus convention)
//   - values are decimal strings rendered with %.3f precision
//
// var-slice wiring: when a template lists "slice" as one of its vars, the
// loader reads slices.json + npus.json (via existing lazy caches) and derives
// per-slice utilization bands from the slice status / parent NPU. The slice
// id is the source of truth — handler validation rejects unknown ids with
// 400 BadRequest.
package mock

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strconv"
	"time"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// ---- Errors ----

// ErrUnknownTemplate is returned when the requested templateId is not in the
// white-list. Handler maps it to 400 BadRequest (per AC).
var ErrUnknownTemplate = errors.New("mock metrics: unknown template id")

// ErrMissingVariable is returned when a required template variable is absent
// or empty. Handler maps it to 400 BadRequest (per AC).
var ErrMissingVariable = errors.New("mock metrics: missing required variable")

// ErrUnknownSlice is returned when var-slice references a slice id absent
// from slices.json. Handler maps it to 400 BadRequest (per AC — unknown
// dimension target is still a client error).
var ErrUnknownSlice = errors.New("mock metrics: unknown slice id")

// ---- Template white-list ----

// VarSpec describes a single template variable.
type VarSpec struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

// templateDef is an internal definition of one white-listed PromQL template.
//
// The `promQL` field is what would be sent upstream in PHASE-2 (real
// prometheus source). The mock ignores it for value generation — only `id`
// and the resolved vars matter — but we keep it so the contract response
// can echo back the expanded query for transparency.
type templateDef struct {
	id          string
	displayName string
	unit        string
	description string
	promQL      string
	vars        []VarSpec

	// valueBand picks the [min, max] utilization range a series ranges across.
	// For workload latency templates the unit is non-percent, so the band is
	// in absolute units (ms / req/s) — see Description.
	valueBand band
}

// band is a closed interval [Min, Max] used as the synthesis range.
type band struct{ Min, Max float64 }

// templates is the white-list. The order is the order returned by
// ListTemplates so client code can rely on a stable list shape.
//
// Naming convention follows Prometheus best practice (snake_case, no units in
// id — units go in the metadata). Each entry below maps to a real template
// the frontend Metrics page consumes in P1-T-208.
var templates = []templateDef{
	{
		id:          "node_cpu_util",
		displayName: "Node CPU Utilization",
		unit:        "%",
		description: "Aggregated CPU utilization across all cores on a node",
		promQL:      `100 - (avg by (instance) (rate(node_cpu_seconds_total{mode="idle",instance="$node"}[1m])) * 100)`,
		vars:        []VarSpec{{Name: "node", Required: true, Description: "Node name"}},
		valueBand:   band{Min: 12, Max: 78},
	},
	{
		id:          "node_mem_util",
		displayName: "Node Memory Utilization",
		unit:        "%",
		description: "Used memory / total memory on a node",
		promQL:      `(1 - node_memory_MemAvailable_bytes{instance="$node"} / node_memory_MemTotal_bytes{instance="$node"}) * 100`,
		vars:        []VarSpec{{Name: "node", Required: true, Description: "Node name"}},
		valueBand:   band{Min: 22, Max: 71},
	},
	{
		id:          "npu_aicore_util",
		displayName: "NPU AI Core Utilization",
		unit:        "%",
		description: "AI Core busy ratio — supports node / npu / slice dimensions (spec F4a 最细切分粒度)",
		promQL:      `ascend_npu_aicore_utilization{instance="$node",npu_id="$npu",slice_id="$slice"}`,
		vars: []VarSpec{
			{Name: "node", Required: false, Description: "Node name (filter)"},
			{Name: "npu", Required: false, Description: "NPU id (filter)"},
			{Name: "slice", Required: false, Description: "Slice id (最细切分粒度)"},
		},
		valueBand: band{Min: 8, Max: 92},
	},
	{
		id:          "npu_vram_util",
		displayName: "NPU VRAM Utilization",
		unit:        "%",
		description: "VRAM used / total — supports node / npu / slice dimensions",
		promQL:      `ascend_npu_vram_utilization{instance="$node",npu_id="$npu",slice_id="$slice"}`,
		vars: []VarSpec{
			{Name: "node", Required: false, Description: "Node name (filter)"},
			{Name: "npu", Required: false, Description: "NPU id (filter)"},
			{Name: "slice", Required: false, Description: "Slice id (最细切分粒度)"},
		},
		valueBand: band{Min: 15, Max: 88},
	},
	{
		id:          "npu_bandwidth_util",
		displayName: "NPU Memory Bandwidth Utilization",
		unit:        "%",
		description: "HBM/DDR bandwidth busy ratio — supports node / npu / slice dimensions",
		promQL:      `ascend_npu_bandwidth_utilization{instance="$node",npu_id="$npu",slice_id="$slice"}`,
		vars: []VarSpec{
			{Name: "node", Required: false, Description: "Node name (filter)"},
			{Name: "npu", Required: false, Description: "NPU id (filter)"},
			{Name: "slice", Required: false, Description: "Slice id (最细切分粒度)"},
		},
		valueBand: band{Min: 5, Max: 78},
	},
	{
		id:          "workload_throughput",
		displayName: "Workload Throughput",
		unit:        "tokens/s",
		description: "Input + output token throughput aggregated across replicas",
		promQL:      `sum by (workload) (rate(mindie_tokens_total{workload="$workload"}[1m]))`,
		vars:        []VarSpec{{Name: "workload", Required: true, Description: "Workload name (namespace/name)"}},
		valueBand:   band{Min: 80, Max: 1850},
	},
	{
		id:          "workload_ttft",
		displayName: "Workload TTFT (Time To First Token)",
		unit:        "ms",
		description: "First-token latency p95 — lower is better",
		promQL:      `histogram_quantile(0.95, sum by (le, workload) (rate(mindie_ttft_seconds_bucket{workload="$workload"}[1m]))) * 1000`,
		vars:        []VarSpec{{Name: "workload", Required: true, Description: "Workload name"}},
		valueBand:   band{Min: 120, Max: 480},
	},
	{
		id:          "workload_itl",
		displayName: "Workload ITL (Inter-Token Latency)",
		unit:        "ms",
		description: "Per-token decode latency p95 — lower is better",
		promQL:      `histogram_quantile(0.95, sum by (le, workload) (rate(mindie_itl_seconds_bucket{workload="$workload"}[1m]))) * 1000`,
		vars:        []VarSpec{{Name: "workload", Required: true, Description: "Workload name"}},
		valueBand:   band{Min: 18, Max: 96},
	},
	{
		id:          "workload_e2e_latency",
		displayName: "Workload End-to-End Latency",
		unit:        "ms",
		description: "Full request latency p95",
		promQL:      `histogram_quantile(0.95, sum by (le, workload) (rate(mindie_e2e_seconds_bucket{workload="$workload"}[1m]))) * 1000`,
		vars:        []VarSpec{{Name: "workload", Required: true, Description: "Workload name"}},
		valueBand:   band{Min: 250, Max: 1900},
	},
}

// templatesByID is a lookup helper built from the templates slice at package
// init time. Keeps lookups O(1) without sacrificing the ordered list above.
var templatesByID = func() map[string]*templateDef {
	m := make(map[string]*templateDef, len(templates))
	for i := range templates {
		m[templates[i].id] = &templates[i]
	}
	return m
}()

// ---- Public template metadata accessor ----

// MetricTemplateMeta is the wire shape returned by /metrics/templates. Lives
// here (not in model) because it is mock-internal until P1-T-204 wires a
// matching contract type. Handlers convert to map[string]any for the JSON
// shape required by docs/api-contract.yaml components.schemas.MetricTemplate.
type MetricTemplateMeta struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Unit        string    `json:"unit,omitempty"`
	Description string    `json:"description,omitempty"`
	PromQL      string    `json:"promQL,omitempty"`
	Variables   []VarSpec `json:"variables"`
}

// ListTemplates returns the white-listed template metadata. The slice is a
// fresh copy so callers cannot mutate the package-level definitions.
//
// Capability: enabled whenever the Source instance is constructed — does not
// depend on the fixture path (templates are constants).
func (s *Source) ListTemplates(_ context.Context) []MetricTemplateMeta {
	out := make([]MetricTemplateMeta, 0, len(templates))
	for i := range templates {
		t := &templates[i]
		vars := make([]VarSpec, len(t.vars))
		copy(vars, t.vars)
		out = append(out, MetricTemplateMeta{
			ID:          t.id,
			Name:        t.displayName,
			Unit:        t.unit,
			Description: t.description,
			PromQL:      t.promQL,
			Variables:   vars,
		})
	}
	return out
}

// ---- QueryMetric ----

// defaultRangeWindow is the synthetic range used when the caller omits the
// time range entirely (e.g. healthcheck-style preview queries). 1 hour back
// from now → 200 points → ~18s step. Picked to match the typical Grafana
// panel range without ballooning the response.
const defaultRangeWindow = time.Hour

// sampleCount is the fixed series length the AC pins (200 points).
const sampleCount = 200

// QueryMetric is the white-listed metric query entry point.
//
// Steps:
//  1. Look up the template by id. Unknown → ErrUnknownTemplate (handler 400).
//  2. Validate required variables. Missing → ErrMissingVariable (handler 400).
//  3. If var-slice is set, verify the slice id exists in slices.json. Unknown
//     → ErrUnknownSlice (handler 400). This is the F4a "最细切分粒度" gate.
//  4. Synthesize a deterministic 200-point series for the time range. Range
//     defaults to "last 1h ending now" when start/end are blank.
//
// The returned MetricQueryResponse has exactly one series; multi-series
// templates (e.g. throughput by replica) are not in P1-T-204 scope — the
// frontend page renders one line per query.
func (s *Source) QueryMetric(ctx context.Context, templateID string, vars map[string]string, timeRange model.TimeRange) (*model.MetricQueryResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tpl, ok := templatesByID[templateID]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownTemplate, templateID)
	}

	// Required vars gate. Handler maps to 400.
	for _, v := range tpl.vars {
		if !v.Required {
			continue
		}
		if val, ok := vars[v.Name]; !ok || val == "" {
			return nil, fmt.Errorf("%w: template %q needs %q", ErrMissingVariable, templateID, v.Name)
		}
	}

	// var-slice validation. We accept missing slice for templates where slice
	// is optional (the series falls back to NPU/node aggregation). But when
	// the caller does provide a slice, it must exist in slices.json — the AC
	// example pings worker-site-a-01-npu-3-slice-0, which must round-trip.
	if sliceID, ok := vars["slice"]; ok && sliceID != "" {
		if templateHasVar(tpl, "slice") {
			if err := s.checkSliceID(sliceID); err != nil {
				return nil, err
			}
		}
	}

	start, end, _ := resolveRange(timeRange)

	// Per-slice band shaping: if a known slice id was supplied, narrow the
	// band to the slice's status-derived range so the response visibly
	// differentiates allocated / available / faulty slices. Falls back to the
	// template's default band when no slice id is provided.
	band := tpl.valueBand
	if sliceID, ok := vars["slice"]; ok && sliceID != "" && templateHasVar(tpl, "slice") {
		if narrowed, derived := s.sliceDerivedBand(sliceID, tpl); derived {
			band = narrowed
		}
	}

	series := synthesizeSeries(templateID, vars, band, start, end)

	// Metric labels — echo back the resolved vars so consumers can correlate.
	// Add `__name__` for Prometheus parity (real prometheus surfaces it).
	labels := make(map[string]string, len(vars)+1)
	labels["__name__"] = templateID
	for k, v := range vars {
		if v == "" {
			continue
		}
		labels[k] = v
	}

	return &model.MetricQueryResponse{
		Result: []model.MetricSeries{
			{Metric: labels, Values: series},
		},
	}, nil
}

// templateHasVar tells whether a template declares a variable with this name.
// Used to gate the var-slice slice-id check (only enforce when the template
// supports slice dimension).
func templateHasVar(tpl *templateDef, name string) bool {
	for _, v := range tpl.vars {
		if v.Name == name {
			return true
		}
	}
	return false
}

// checkSliceID verifies the slice id exists in slices.json. Falls back to a
// no-op (returns nil) when fixturesPath is empty — tests build a Source
// without disk fixtures and still want the 200-path to work for non-slice
// queries.
func (s *Source) checkSliceID(sliceID string) error {
	if s.fixturesPath == "" {
		// No fixture loaded — we cannot validate. Treat as unknown so the
		// handler returns 400, matching production behavior where slices.json
		// is always present.
		return fmt.Errorf("%w: %q", ErrUnknownSlice, sliceID)
	}
	slices, err := s.loadSlicesFlat()
	if err != nil {
		return err
	}
	for _, sl := range slices {
		if sl.ID == sliceID {
			return nil
		}
	}
	return fmt.Errorf("%w: %q", ErrUnknownSlice, sliceID)
}

// sliceDerivedBand narrows the value band based on the slice's status. Only
// applies to percent-unit templates (npu_aicore_util / npu_vram_util /
// npu_bandwidth_util — the three that declare a slice variable). Returns
// (band, true) when the slice was found AND the template is percent-typed;
// (zero, false) otherwise.
//
// Bands mirror the AC's spirit: an allocated slice should look busy (high
// utilization), available looks idle (low), faulty is volatile.
func (s *Source) sliceDerivedBand(sliceID string, tpl *templateDef) (band, bool) {
	if tpl.unit != "%" {
		return band{}, false
	}
	slices, err := s.loadSlicesFlat()
	if err != nil {
		return band{}, false
	}
	for _, sl := range slices {
		if sl.ID != sliceID {
			continue
		}
		switch sl.Status {
		case "allocated":
			return band{Min: 55, Max: 92}, true
		case "available":
			return band{Min: 2, Max: 18}, true
		case "faulty":
			return band{Min: 0, Max: 5}, true
		default:
			return band{Min: 10, Max: 60}, true
		}
	}
	return band{}, false
}

// resolveRange parses the TimeRange. Empty start/end falls back to "last
// defaultRangeWindow ending now". Bad inputs fall back to the same default
// (we'd rather emit a valid series than a 500 for a typo). Returns wall-clock
// times in UTC plus the per-sample step.
func resolveRange(r model.TimeRange) (start, end time.Time, step time.Duration) {
	now := time.Now().UTC()
	end = now
	start = now.Add(-defaultRangeWindow)

	if r.End != "" {
		if t, err := time.Parse(time.RFC3339, r.End); err == nil {
			end = t.UTC()
		}
	}
	if r.Start != "" {
		if t, err := time.Parse(time.RFC3339, r.Start); err == nil {
			start = t.UTC()
		}
	}
	if !start.Before(end) {
		// Misordered or zero-duration → fall back so the synthesized series
		// always has a positive step.
		start = end.Add(-defaultRangeWindow)
	}
	step = end.Sub(start) / time.Duration(sampleCount-1)
	if step <= 0 {
		step = time.Second
	}
	return start, end, step
}

// synthesizeSeries builds a deterministic 200-point time series spanning
// [start, end]. Each sample is `[unix_seconds_float, value_string]` per the
// Prometheus convention echoed in the OpenAPI MetricQueryResponse.
//
// Determinism: seed = sha256(templateID + sorted-vars-JSON). math/rand/v2's
// ChaCha8 is seeded from the first 32 bytes of the digest. Two equal queries
// always yield the same series within the process.
//
// Shape: gentle sine wave + low-amplitude RNG jitter scaled into the band.
// This gives the frontend a realistic-looking curve without trending up or
// down (overlays look noisy without being misleading).
func synthesizeSeries(templateID string, vars map[string]string, b band, start, end time.Time) [][]interface{} {
	rng := newSeededRand(templateID, vars)

	out := make([][]interface{}, sampleCount)
	mid := (b.Min + b.Max) / 2
	half := (b.Max - b.Min) / 2
	if half < 0 {
		half = -half
	}

	totalDur := end.Sub(start)
	for i := 0; i < sampleCount; i++ {
		// Even time spacing.
		frac := float64(i) / float64(sampleCount-1)
		ts := start.Add(time.Duration(float64(totalDur) * frac))

		// Sine wave gives the curve "shape" so the UI looks alive; phase is
		// part of the deterministic seed so repeated queries are identical.
		theta := 2 * math.Pi * frac * 2 // two full waves across the window
		base := mid + 0.6*half*math.Sin(theta)
		jitter := (rng.Float64()*2 - 1) * 0.25 * half
		v := base + jitter
		if v < b.Min {
			v = b.Min
		}
		if v > b.Max {
			v = b.Max
		}

		out[i] = []interface{}{
			float64(ts.Unix()) + float64(ts.Nanosecond())/1e9,
			strconv.FormatFloat(v, 'f', 3, 64),
		}
	}
	return out
}

// newSeededRand returns a deterministic RNG keyed by `templateID + sorted
// JSON of vars`. Uses sha256 to fold the key down to 32 bytes, then ChaCha8
// (math/rand/v2's strongest source) for the PRNG itself.
func newSeededRand(templateID string, vars map[string]string) *rand.Rand {
	// Stable JSON of vars: sort keys then encode.
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	type kv struct{ K, V string }
	ordered := make([]kv, 0, len(keys))
	for _, k := range keys {
		ordered = append(ordered, kv{K: k, V: vars[k]})
	}
	payload, _ := json.Marshal(struct {
		Template string `json:"t"`
		Vars     []kv   `json:"v"`
	}{Template: templateID, Vars: ordered})

	sum := sha256.Sum256(payload)
	// ChaCha8 needs exactly [32]byte; sha256 already returns [32]byte, so a
	// type conversion is the cleanest seed pipeline.
	return rand.New(rand.NewChaCha8(sum))
}
