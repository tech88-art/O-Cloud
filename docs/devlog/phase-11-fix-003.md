# P11-fix-003 · dashboard template-var queries pointing at non-existent metric

- **Date**: 2026-05-22
- **Duration**: ~0.1d
- **Trigger**: After P11-fix-002 landed, user opened node-detail / npu-detail
  dashboards and both showed No-data — even though Prom side had real series
  flowing for `ascend_npu_utilization_percent` (24) and `node_cpu_seconds_total`
  (64). URL inspection: `var-node=` empty.

## Root cause

Both dashboards' template variables were querying a metric that **never
existed in the ascend-npu-exporter-plus output**:

```
var node:  label_values(ascend_npu_info, node)
var npu :  label_values(ascend_npu_info{node=~"$node"}, npu)
```

`ascend_npu_info` was a Phase 1-3 era placeholder; the actual exporter
never shipped that family. With the query returning empty, `$node` and
`$npu` resolved to empty strings, so every panel's `node=~"$node"` regex
matched zero series → No-data.

Compounding: panel queries used `npu=~"$npu"` (label name `npu`) but
exporter emits `npu_id` — a second mismatch hidden behind the first.

## Fix

- `node-detail.json` · 1 var: `ascend_npu_info` → `ascend_npu_utilization_percent`
- `npu-detail.json` · 2 vars: same + `npu` → `npu_id` label
- `npu-detail.json` · 5 panel exprs: `npu=~"$npu"` → `npu_id=~"$npu"`

## Validation

User screenshot after Grafana provisioning reload:
- node-detail @ worker-site-a-01: CPU busy% mean 4.92 / max 89.3,
  memory used% ~20, network rx 31.7 B/s / tx 444 B/s
- npu-detail @ worker-site-a-01/npu-0: AICore ~40% sinusoid,
  VRAM 8 / 64 GiB, temperature 70.7°C, power 300-330W

## P7 self-audit

P11-fix-002 verified the panel `expr` strings against Prom — but template
variable queries are a **separate** category and I missed them. Going
forward, dashboard fixes need 3-layer verify:
(a) panel expr metric exists in Prom
(b) template var queries hit a real metric and return non-empty
(c) panel selector label names match exporter-emitted label names

The bug stayed hidden because cluster-overview has no `$node` / `$npu`
variables (all panels global) — so the first round of testing on cluster-
overview gave false confidence about the other dashboards.

## Refs

- P11-fix-001 (path A bringup) · P11-fix-002 (exporter +5 metric)
