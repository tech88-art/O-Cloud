# Screenshots placeholder

Phase 1 demo screenshots land here. AI agents can produce the demo
script + this placeholder; **video / screenshot recording is a manual
step** owned by the operator (see `docs/phase0-review.md` MUST-FIX #3
for the agreement on this split).

## Suggested file layout

```
docs/screenshots/
├── 01-overview-default.png         Overview, fabric+workloads toggles OFF
├── 02-overview-fabric-on.png       Overview with "Include Fabric"
├── 03-overview-workloads-on.png    Overview with "Include Workloads"
├── 04-workloads-table.png          /workloads filtered to status=running
├── 05-workloads-drawer-pdpair.png  Drawer showing qwen-8b-pd + PD-pair tag
├── 06-deploy-presets.png           /deploy with 4 preset cards
├── 07-deploy-wizard-step1.png      Wizard auto/manual radio
├── 08-deploy-wizard-step2.png      Wizard replicas + namespace form
├── 09-metrics-overview.png         /metrics cluster-overview tab + iframe
├── 10-metrics-npu-cascader.png     /metrics npu-detail cascader filled
├── 11-logs-tail.png                /logs 200-line tail of qwen-8b-pd
├── 12-logs-live-stream.png         /logs with Live stream ON
└── 13-d6-affinity-comparison.png   D6 narrative end-state (events.json)
```

## Capture recipe

1. `./scripts/install.sh` (or `make dev-up`) to bring the stack up.
2. Open `http://localhost:3000` in a 1440×900 browser window with the
   sidebar collapsed (cleaner crop).
3. Follow `docs/demo.md` step by step; screenshot at each numbered
   step.
4. For Live stream / event timing, wait until the WS status chip flips
   to `● open` and a `lastEventAt` timestamp appears before capturing.

## Format

PNG, ≤ 1 MB each. Keep the originals out of git history — drop into
`docs/screenshots/` and the `.gitignore` should pick them up via the
existing `*.png` rule (or add it if Phase 2 wants them tracked).
