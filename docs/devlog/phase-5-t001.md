# P5-T-001 · npu-dra-driver DeviceClass helm template registration

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 0.5d · actual ~45min

## Intent

Phase 5 entry task. The npu-dra-driver publisher emits ResourceSlices
with `driver = npu.ocloud.edge.example.com` already (Phase 4 T005), but
no DeviceClass exists for claim authors to reference. T001 lands the
DeviceClass registration via the helm chart (per ADR-0009 §5 step 1 +
DESIGN.md §6.2). Three classes are rendered behind values toggles:
bare class matches everything; `.whole` adds a FixedTemplate selector;
`.dynamic` adds a Dynamic selector.

## Path adaptations

- **`/whole` sub-class name → `.whole`**. The plan acceptance text says
  "an additional `/whole` class is rendered". K8s `metadata.name` must
  be an RFC 1123 subdomain — `/` is not in the allowed character set,
  so the name `npu.ocloud.edge.example.com/whole` is rejected by API
  server validation (helm lint --strict caught this at first try). The
  plan's dynamic sub-class already used `.dynamic` (asymmetric — likely
  a docs typo). The claim controller `isOurClass()` matches both `/`
  and `.` prefix shapes (see `claim_controller.go:173-175` +
  `claim_controller_test.go:192-196` test cases), so forward
  compatibility is preserved: a claim authored with `/whole` still
  binds correctly even though the DeviceClass is named `.whole`.

## Debugging trail

- **helm lint --strict rejected `npu.ocloud.edge.example.com/whole`**:
  output `metadata.name: Invalid value: "npu.ocloud.edge.example.com
  /whole": a lowercase RFC 1123 subdomain must consist of lower case
  alphanumeric characters, '-' or '.', and must start and end with an
  alphanumeric character`. Resolution: change to `.whole` form.
  Updated the YAML, template comment, README table, and README
  ResourceClaim example to align.
- **kubectl apply --dry-run=client fails without live cluster**: same
  pattern as Phase 4 T101. `kubectl --validate=false` still errors
  with "unable to recognize STDIN". This is expected — kind smoke
  (T106) validates end-to-end. As a substitute, parsed the rendered
  YAML through `python -c "yaml.safe_load_all(...)"` (with
  `encoding='utf-8'` because the rendered ConfigMap embeds the
  544-line npus.json with UTF-8 Chinese characters that crash the
  Windows GBK default codec).
- **Toggle matrix verified**: `helm template ... --set
  deviceClass.create=false` → 0 DeviceClass docs; default → 3;
  `--set deviceClass.subClasses.{whole,dynamic}=false` (both off) →
  1 (bare class only).

## Key decisions

- **CEL-based selectors over attribute predicates**. DeviceClass v1beta1
  supports a list of selectors; the CEL form lets us combine
  driver-name and slice-strategy filters in a single expression. The
  bare class deliberately omits the slice-strategy filter so the
  Phase 4 publisher's FixedTemplate-only output stays selectable
  without churning when Phase 5+ adds Dynamic devices.
- **Sub-class toggles default true**. The plan default; turn off via
  `--set deviceClass.subClasses.whole=false` for restricted topology.
  Phase 7 Partitionable Devices may want to add `.partition` — the
  toggle pattern extends naturally.
- **No RBAC change**. The chart applies the DeviceClass at install
  time using the chart-deploying user's credentials (typically cluster
  admin), not the npu-dra-driver SA. The SA already has `get/list/
  watch` on `deviceclasses` from Phase 4 T101 — sufficient for the
  Phase 5 allocator to read DeviceClass selectors during reconcile
  (T002).

## Verification

P3 三维度:
- **Existence**: `git diff --stat` shows 1 new file
  (`templates/deviceclass.yaml`) + edits in `values.yaml`, `README.md`.
- **Completeness**: `helm lint --strict` → `1 chart(s) linted, 0
  chart(s) failed`. Three toggle scenarios all render expected counts
  (0 / 1 / 3 DeviceClasses).
- **Correctness**: parsed YAML shows all three names (`npu.ocloud.edge
  .example.com`, `npu.ocloud.edge.example.com.whole`, `npu.ocloud
  .edge.example.com.dynamic`) with the documented CEL expressions
  injected verbatim.

## Carry-forward

- T002 allocator will read DeviceClass selectors and filter
  ResourceSlices accordingly; the bare-class match-all selector keeps
  the Phase 5 simple greedy first-fit working against the Phase 4
  publisher output without further filtering.
- T106 kind smoke must verify `kubectl get deviceclasses` shows all
  three names post-install.
- Phase 7 Partitionable Devices: add a `.partition` sub-class behind
  `deviceClass.subClasses.partition` toggle following the same
  pattern.
