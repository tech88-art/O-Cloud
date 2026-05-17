# Mock Data Generator (P1-T-011)

Go CLI that emits the Phase 1 mock JSON datasets that the demo backend
reads. Output validates against `configs/mock-data/schema.json` (CI enforces).

## Quick start

```bash
cd configs/mock-data/generator
go mod tidy
go run . --preset small --output ../set-a-small/
```

Or via Makefile:

```bash
make gen-small      # writes ../set-a-small/
make validate       # ajv-validate every set-*/*.json against schema.json
make all            # gen-small + validate
```

## CLI flags

| Flag        | Default               | Meaning                                  |
| ----------- | --------------------- | ---------------------------------------- |
| `--preset`  | `small`               | One of `small` / `multi` / `stress`      |
| `--output`  | `../set-a-small/`     | Output directory (created if missing)    |

Only `--preset small` is implemented today. `multi` and `stress` return an
explicit error and are tracked under P1-T-307.

## Output (9 files)

```
set-a-small/
├── meta.json
├── clusters.json
├── nodes.json
├── npus.json
├── slices.json
├── workloads.json
├── pools.json
├── presets.json
└── events.json
```

### Why each file repeats `meta` and `clusters`

`configs/mock-data/schema.json` has eight required top-level keys
(`meta`, `clusters`, `nodes`, `npus`, `slices`, `workloads`, `pools`,
`presets`) with `clusters.minItems = 1`. Validating one section per file
against this root schema requires every file to carry the required
shell. The writer therefore emits a "fat" wrapper around each section —
the focal section is populated, the others are empty arrays, and the
single cluster is replicated into every file so `minItems` holds.

The duplication adds ~400 B per file; in exchange, ajv validates every
split file independently and the backend can load any single section
without first reading the others. This is the cheapest way to satisfy
both the directory-layout AC and the schema contract without touching
`schema.json` (RFC-only).

## Data realism (set-a-small)

Conforms to `configs/CLAUDE.md` §3.3 and §3.4:

- **1 cluster** `cluster-prod-a-01` (role `edge-single`, status `healthy`).
- **3 worker nodes** `worker-site-a-01..03`, `arch=amd64`, 96 CPU / 768 Gi
  memory each, 2 NUMA domains per node.
- **24 NPUs** (Ascend910B, 64 GiB VRAM, 32 AI cores). Each node has:
  - **HCCS grouping** — 2 full-mesh groups of 4 cards (`<node>-hccs-0/-1`).
  - **NUMA layout** — NPU 0..3 on NUMA 0, NPU 4..7 on NUMA 1.
  - **Slice mode mix** — 2 whole + 4 fixed-template + 2 dynamic per node.
- **~40 slices** total (vir01 / vir02 / vir04 + 6 dynamic shapes), with ~50%
  allocated to demo workloads.
- **10 workloads** with status mix 6 running / 2 pending / 1 succeeded /
  1 failed; one Qwen 8B PD pair with `pd-pair` relation; mixed kinds
  (`Deployment` / `InferenceService` / `Job`).
- **Realistic utilisation** — per-NPU `aiCoreUtilization` ranges 0%, 12%,
  22%, 31%, 45%, 63%, 72%, 81% (deliberately not all-50%).
- **Pools** — 1 ClusterPool, 1 NodePool, 1 NPUPool (with HCCS topology),
  2 NPUSlicePool (one `FixedTemplate`, one `Dynamic`).
- **4 deploy presets** — Pi 3B / Qwen 8B PD / DeepSeek 20B / Qwen 14B
  (matches `docs/architecture.md` §3 deploy presets).
- **21 events** spaced 0..130 s at 5-10 s cadence per
  `configs/CLAUDE.md` §3.3.

## Reproducibility

`gofakeit` is seeded at `42` and a fixed `t0 = 2026-05-17T12:00:00Z` is
used for every timestamp. Two consecutive `go run .` invocations produce
byte-identical files.

## Module layout

```
configs/mock-data/generator/
├── main.go                 CLI (cobra)
├── preset_small.go         set-a-small builder
├── preset_multi.go         set-b-multi-site placeholder (P1-T-307)
├── preset_stress.go        set-c-stress placeholder (P1-T-307)
├── pkg/
│   ├── model/              Go types matching schema.json $defs
│   ├── builder/            HCCS / NUMA / time-skew helpers
│   └── writer/             Fans Dataset out to 9 JSON files
├── Makefile
├── go.mod
└── go.sum
```

## Toolchain

- Go 1.22+ (project uses 1.26.3)
- `npx ajv-cli@5` for schema validation (no local install needed)

## Dependencies

- `github.com/brianvoe/gofakeit/v7` — seeded fake data
- `github.com/spf13/cobra` — CLI
