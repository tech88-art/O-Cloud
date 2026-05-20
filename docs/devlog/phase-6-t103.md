# P6-T-103 · Frontend Workloads page refresh — slice bindings

- **Commit**: <pending>
- **Date**: 2026-05-20
- **Duration**: plan 1d · actual ~45min

## Intent

Surface T102's new `Workload.sliceBindings[]` field on the frontend
Workloads page:
- New `SliceBindingBadge` component (`<Tag>`-based) renders one
  binding per badge with role-colored chip + structured tooltip.
- WorkloadTable adds a "Slice Bindings" column visible ONLY when at
  least one workload has bindings populated — keeps the table compact
  for pre-Phase-6 environments.
- WorkloadDetailDrawer adds a new section: when prefill+decode both
  present, renders a 2-column PD-pair grid; falls back to flat list
  for other layouts.
- Page always opts in to `?includeSliceBindings=true` so the column
  appears when relevant.
- 3 new Vitest cases + 2 fixed existing assertions to honor the
  always-opt-in query parameter.

## Path adaptations

- **`pnpm run gen:types` ran clean**: the T102 OpenAPI additions
  flowed through to `frontend/src/services/types.ts` without manual
  intervention. The new `SliceBinding` schema appeared as a typed
  component automatically.

- **Always-opt-in for `includeSliceBindings`**: page-level decision.
  Backend default is opt-out so the wire format stays unchanged for
  non-Phase-6 callers; this page explicitly asks for the data
  because the table column conditional-renders anyway. Cleaner UX
  than adding a separate toggle in the filter bar — operators see
  bindings when they exist, the column is hidden when they don't.

- **`WorkloadTable` column hidden when no workload has bindings**:
  `anyHasSliceBindings = data.some(...)` decides whether to push the
  column into `columns[]`. Avoids a visually empty trailing column
  in pre-Phase-6 environments where no PD Router has stamped
  bindings yet.

- **PD-pair grid in Drawer falls back to flat list**: when the
  workload has bindings but they don't include both prefill +
  decode (e.g. a single-side ModelService or a non-PD workload),
  the section renders a flat `<Space wrap>` of badges. Decision:
  PD-pair grid adds value only when both sides are present;
  forcing the layout otherwise creates an awkward "1 column +
  empty column" presentation.

- **2 existing tests updated for always-opt-in**: `params: {}` →
  `params: { includeSliceBindings: true }` and similarly for the
  status-filter test. New behavior is the new contract.

## Debugging trail

- **Test setup time long but tests pass**: full Workloads.test.tsx
  suite (13 tests) runs in ~49s including the new T103 cases. Most
  of that is component mounting + AntD rendering, not actual test
  execution. Expected for React + AntD test stacks.

- **No surprises in WorkloadTable hidden column logic**: pushing
  to `columns[]` array dynamically works cleanly because AntD's
  Table renders only the configured columns.

- **`SliceBindingBadge` accepts both `binding.podName` and the
  no-podName case**: tooltip only renders when at least one of
  `podName / pool / role` is set. Tag without tooltip renders
  cleanly.

## Key decisions

- **Component lives in `components/`, not `pages/Workloads/`**:
  matches the project's MetricChip / StatusTag pattern — small
  reusable badges live at the top-level `components/`. Other pages
  may want to render sliceBindings later (Metrics page, Topology
  hover card, etc.).

- **Role color mapping**: prefill=magenta, decode=purple,
  primary=blue, sidecar=cyan, init=geekblue, peer=green. Consistent
  with the existing `relationTone()` map in
  WorkloadDetailDrawer.tsx for adjacent UI affordances.

- **Tooltip content is structured plain text**: `pod: X · pool: Y ·
  role: Z`. Operators reading the badge get the full context without
  the badge label growing unbounded.

- **Page-level opt-in vs filter-bar toggle**: page-level is the
  right shape because (a) the column is auto-hidden when no data,
  (b) adding a filter toggle for an opt-in field is friction with
  no UX gain (users don't know about the field until they see it).

- **CSS: `.pdPairGrid` uses 2-column grid + `.pdPairColumn` uses
  dashed border**: subtle visual separation between prefill /
  decode columns without dominant boxes. Other-roles column wraps
  below as a 3rd grid cell.

## Verification

P3 三维度:

- **Existence**:
  - `git status --short` →
    - M `frontend/src/i18n/{zh-CN,en-US}.json` (5 new i18n keys)
    - M `frontend/src/pages/Workloads/{index,WorkloadTable,
      WorkloadDetailDrawer}.tsx`
    - M `frontend/src/pages/Workloads/styles.module.css`
      (.pdPairGrid + .pdPairColumn)
    - M `frontend/src/services/types.ts` (regen)
    - M `frontend/src/services/workload.ts` (SliceBinding type +
      filter.includeSliceBindings)
    - M `frontend/tests/Workloads.test.tsx` (2 fixes + 3 new tests
      + workloadListWithBindings fixture)
    - A `frontend/src/components/SliceBindingBadge/index.tsx`
    - A `docs/devlog/phase-6-t103.md`

- **Completeness** (plan §4 P6-T-103 acceptance):
  - Workloads list shows "Slice Bindings" column when any workload
    has sliceBindings populated ✅
  - Column hidden when no workload has bindings (cleaner default)
    ✅
  - Workloads detail shows PDPairCard equivalent (PD-pair grid)
    when both prefill+decode present ✅
  - Each badge renders as `node/device (cores)c` ✅
    (tooltip exposes raw annotation context)
  - i18n strings present in en + zh locales ✅ (5 new keys each)
  - Vitest + RTL tests pass ✅ (109 total · 3 new T103 + 2 fixed
    existing)
  - Coverage ≥ 50% per backend/CLAUDE.md §9 ✅ (no regression)

- **Correctness**:
  - `pnpm typecheck` exit 0
  - `pnpm lint` exit 0
  - `pnpm test --run tests/Workloads.test.tsx` exit 0
    (13 tests across 13 files, 109 passing total)
  - `pnpm run gen:types` exit 0 (types refresh from updated
    api-contract.yaml)
  - DOM accessibility: SliceBindingBadge uses AntD `<Tag>` + AntD
    `<Tooltip>` (both ARIA-friendly)

## Carry-forward

- **Phase 6 polish opportunities** (out of P6-T-103 scope):
  - Workloads page could add a filter for "show only workloads
    with sliceBindings" — useful for operators looking at NPU-
    bound workloads specifically.
  - SliceBindingBadge could grow a click handler that navigates
    to the corresponding NPU device on the Topology page.
  - PDPairCard grid could add a "ring" badge alongside each
    binding once T106 fixture populates ring info via
    NPUSliceAllocation join.

- **kind smoke (P6-T-106) screenshot**: a follow-up could capture
  the new Workloads page render via Playwright + e2e-kind smoke
  to demonstrate the Phase 6 end-to-end story.

- **Backend crd source extension** (T102 carry-forward): when the
  crd source learns to populate sliceBindings via NPUSliceAllocation
  list, the frontend page automatically lights up — no frontend
  change needed.

- **Phase 8 vertical scaling polish**: workload UI may grow a
  "scaling state" indicator showing whether a workload is in the
  active or idle pool. Phase 8 task, out of T103 scope.
