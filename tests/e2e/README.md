# O-Cloud Edge — Playwright E2E suite

Owned by the **deploy** module. Phase 1 entry point: `tests/e2e/tests/topology.spec.ts`
covers the W2 Overview page main flow.

## Layout

```
tests/e2e/
├── package.json              standalone pnpm module
├── playwright.config.ts      webServer-driven (auto-starts backend + frontend)
├── tests/
│   └── topology.spec.ts      W2 main flow (3 active + 1 skipped scenario)
├── fixtures/                 (placeholder; helpers can live here)
├── playwright-report/        HTML report (gitignored)
└── test-results/             screenshots / traces / videos (gitignored)
```

## Prerequisites

- **Node.js 20+** (we test on Node 24 locally)
- **pnpm 9+**
- **Backend binary built**: `cd backend && make build` (produces `backend/bin/demo-backend.exe`
  on Windows or `backend/bin/demo-backend` elsewhere)
- **Frontend deps installed**: `cd frontend && pnpm install`

The Playwright config's `webServer` auto-starts both servers if they aren't
already responding, so a one-time `make build` + `pnpm install` is all the
setup required.

## Run locally

```bash
# 1. Install Node deps for the e2e module (first time only)
cd tests/e2e
pnpm install

# 2. Download chromium (first time only — ~150 MB)
pnpm exec playwright install chromium

# 3. Run the suite. Backend on :8080 and frontend on :3000 will be
#    auto-started if not already up; if either is already running,
#    Playwright reuses it (reuseExistingServer: true).
pnpm exec playwright test --project=chromium

# 4. View the HTML report
pnpm exec playwright show-report
# or just open `playwright-report/index.html`
```

If you prefer the interactive UI runner:

```bash
pnpm exec playwright test --ui
```

## What the suite asserts

### Scenario 1 — "open /overview → tree + graph render against mock backend"

- Three Overview panes (tree / center / detail) mount
- AntD `<Tree>` populates from `/api/v1/clusters/:id/topology`
- ReactFlow canvas renders, with the cluster node and the first worker
  node (`worker-site-a-01`) addressable by id
- At least 8 NPU-type nodes are visible (the dataset has 24)
- WebSocket handshake at `/ws/topology` succeeds — the WsStatusChip's
  `data-status` attribute flips to `open`

### Scenario 2 — "tree click → topology highlight + DetailPanel sync"

- Clicking `worker-site-a-01` in the left tree
  - highlights the matching ReactFlow node (CSS class `selected`)
  - flips the right panel to `data-testid="detail-panel-node"`

### Scenario 3 — "dbl-click NPU → slice children visible in the graph"

- Dbl-clicking `worker-site-a-01-npu-2` (an NPU that owns slices in
  `set-a-small`)
  - reveals at least one `data-testid="topo-node-slice"` node
  - DetailPanel flips to the NPU card

### Scenario 4 — "WS event drives status change" (`test.skip`)

Documented but skipped pending a server-side fastforward query param on
`/ws/topology`. See the TODO comment in `tests/topology.spec.ts`.

## Troubleshooting

### "browserType.launch: Executable doesn't exist at …chromium"

Run `pnpm exec playwright install chromium` (one-time).

### Backend webServer fails: "command not found: ./bin/demo-backend.exe"

Build the backend once:
```bash
cd ../../backend
make build
```

(Inside this directory, the path is `D:/code/ai-edge/backend` — Playwright's
config uses an absolute path resolved against the worktree root.)

### Frontend webServer times out

The first Vite cold start can exceed 60s on a slow filesystem. The config
allows 120s. If it still fails:

```bash
cd ../../frontend
pnpm install
pnpm dev
# open http://localhost:3000 — once Vite shows "ready in NNNms" you can
# re-run playwright in another shell.
```

### "WsStatusChip never flips to open"

Verify the backend log shows the `/ws/topology` upgrade. If the backend was
started without `mapping.events` configured, the chip stays at
`connecting`. The default `backend/configs/config.dev.yaml` has the mock
events source enabled.

## CI integration

The CI job `e2e` in `.github/workflows/ci.yml` is currently a stub
(introduced by P1-T-007). Replacing it with a real Playwright invocation
needs:

1. A Linux runner image with chromium + glibc (Playwright's
   `mcr.microsoft.com/playwright:v1.48.2-jammy` image is the easy choice).
2. `pnpm install --frozen-lockfile` in `tests/e2e/`.
3. `pnpm exec playwright install --with-deps chromium` (the `--with-deps`
   flag installs the system libs chromium needs on a bare image).
4. Start the backend + frontend (or rely on `webServer` — works in CI too
   since `reuseExistingServer: true` falls through to fresh starts).
5. `pnpm exec playwright test --project=chromium`.

That hookup is tracked as a follow-up; see the P1-T-110 commit message.

## Refs

- Task package: `docs/phase1-plan.md` §3 / `P1-T-110`
- Page UX: `docs/architecture.md` §8.2 (Overview page)
- Mock dataset: `configs/mock-data/set-a-small/`
- Vitest counterpart (interactions mirrored): `frontend/tests/Overview.test.tsx`
