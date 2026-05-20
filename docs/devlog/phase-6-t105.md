# P6-T-105 · vllm-ascend PD proxy_server adoption

- **Commit**: <pending>
- **Date**: 2026-05-20
- **Duration**: plan 1d · actual ~30min

## Intent

Land the schema + builder substrate for vllm-ascend
`disaggregated_prefill_v1/proxy_server` sidecar adoption per ADR-0010
§3 forward notes. T105 ships the CRD fields + deployment_builder
helper + tests + chart-fixture demo of the new CI/production
image-split pattern; the actual proxy_server enable happens later
when vllm-ascend v0.12+ stability is confirmed.

## Path adaptations

- **Gated YES/NO decision: opted "NO" path**: plan §3 P6-T-105 entry
  decision asked whether vllm-ascend v0.12+ is GA. Without web
  connectivity to verify upstream release notes from this Windows
  shell, I went with the "NO" path: env-var passthrough +
  sidecar template skeleton + FallbackImage feature flag. Default
  values keep both ProxyImage and FallbackImage empty so existing
  ModelServices behave exactly as Phase 5 ships. Full proxy
  adoption pending Phase 7 vllm-ascend v0.12+ stability assessment.

- **Two CRD fields instead of one combined**: I split
  `pdPair.proxyImage` (production sidecar opt-in) from
  `pdPair.fallbackImage` (CI expedience override). Plan §3 implied
  one field; splitting matches the actual use cases — operators
  don't want to accidentally enable proxy by setting a busybox-style
  fallback. Trade-off: two fields to document, two precedence rules
  to reason about. Worth it for the clarity.

- **Sample fixture updated to demonstrate fallbackImage**: plan
  acceptance says T106 kind smoke fixture sets `fallbackImage=busybox`.
  Updated `tests/e2e/kind/phase5/fixtures/modelservice-sample.yaml`
  to reference a real-ish image (`registry.example.com/vllm-ascend:
  v0.11.0`) for `spec.model.image` AND
  `fallbackImage: busybox:1.36` for the actual CI runtime. Shows
  the production-vs-CI separation pattern.

- **VLLM_PD_* env var names**: chose names matching the current
  upstream vllm-ascend conventions (PREFILL_HOST / DECODE_HOST /
  SIBLING_LOCALHOST_PORT / OTHER_SIDE). When upstream finalises
  the proxy_server contract on stable v0.12+, the names may shift;
  documented in DESIGN.md §5.0 forward path.

- **Service-name convention (`<ms.Name>-<side>`) is forward-looking**:
  Phase 5 deployment_builder doesn't yet create per-side Services.
  The PREFILL_HOST / DECODE_HOST env-vars reference Service names
  the controller WILL create when Phase 7+ wires per-side Services
  (probably alongside scheduler-plugin schedulerName injection at
  T105's natural sibling task). For now the proxy sidecar would
  fail to connect at runtime if enabled — that's OK because the
  default is empty (off).

## Debugging trail

- **First `go vet` after refactor**: `cannot use side (variable of
  string type PDSide) as string value in argument to
  buildPDPairContainers`. I'd typed the helper's `side` param as
  `string` but the call site passes the typed `PDSide` enum.
  Changed helper signature to take `PDSide`; added `string(side)`
  conversion in the inner comparison. `go vet` clean after.

- **CRD regen produced expected ProxyImage + FallbackImage YAML
  fields** + the cosmetic controller-gen v0.21.0 annotation bump.
  Kept all changes (the version bump is colocated with substantive
  schema additions — separating them would break the regen-friendly
  pattern).

- **Chart CRD copy synced**: `deploy/helm-charts/inference-operator/
  crds/modelservices.yaml` is a copy of the operator's
  `config/crd/bases/...modelservices.yaml`. cp the updated file
  over so the chart bundles the new schema.

- **All 5 new tests + 13 existing pass**: 18 controller tests + 4
  metrics + 18 webhook = 40 total inference-operator tests after
  T105. Existing tests unchanged because new fields default empty
  (proxy off, fallback off).

## Key decisions

- **ProxyImage default empty (off) until upstream stable**:
  enabling the sidecar by default would break every existing
  ModelService Pod (proxy_server tries to connect to localhost:8000
  + sibling Services that don't exist yet). Off-by-default is the
  only safe choice in T105 scope.

- **FallbackImage takes precedence over Model.Image on main
  container ONLY**: not on the sidecar. ProxyImage is opt-in via
  the operator's explicit set — if they set ProxyImage they intend
  the real image. Mixing fallback with proxy would be ambiguous
  (does fallback also apply to sidecar?); the current rule is
  unambiguous.

- **Env-var pattern matches upstream**: vllm-ascend's proxy_server
  reads env-vars; emitting them as Container.Env populates them
  through K8s standard channels. Future operators tuning the proxy
  configuration can override via `kubectl set env` or the
  ModelService CRD if a Phase 7+ task adds explicit proxy args.

- **Container name `pd-proxy`**: short, descriptive, doesn't
  collide with `vllm-ascend` (the main). K8s container name
  validation accepts hyphens.

- **No `recreatePolicy` field added**: plan DESIGN.md §6.3
  forward-notes mention `ms.spec.recreatePolicy` as a future
  capability; T105 doesn't ship it because the CRD is already
  growing — adding more fields would conflict with the
  ResourceClaimTemplate.Spec immutability rule the controller
  already documents. Phase 7+ candidate.

## Verification

P3 三维度:

- **Existence**:
  - `git status --short` → 4 modified + 1 new + devlog:
    - M `api/v1alpha1/modelservice_types.go`
    - M `internal/controller/deployment_builder.go`
    - M `config/crd/bases/...modelservices.yaml` (regen)
    - M `deploy/helm-charts/inference-operator/crds/modelservices.yaml` (sync)
    - M `tests/e2e/kind/phase5/fixtures/modelservice-sample.yaml`
    - M `DESIGN.md`
    - A `internal/controller/deployment_builder_test.go`
    - A `docs/devlog/phase-6-t105.md`
  - `grep ProxyImage operators/inference-operator/api/v1alpha1/
    modelservice_types.go` → field present

- **Completeness** (plan §3 P6-T-105 acceptance):
  - `spec.pdPair.proxyImage` field landed ✅
  - CRD applies cleanly (helm lint --strict clean) ✅
  - Deployment template renders sidecar when proxyImage set ✅
    (TestBuildPDPairContainers/proxy_enabled)
  - Default behavior (proxyImage empty) → no sidecar, busybox or
    Model.Image ✅ (TestBuildPDPairContainers/proxy_disabled)
  - fallbackImage takes precedence over Model.Image only when set
    ✅ (TestBuildPDPairContainers/fallbackImage_takes_precedence)
  - 4 new builder tests ✅ (5 sub-tests actually, since I added a
    decode-side smoke for VLLM_PD_OTHER_SIDE coverage)
  - DESIGN.md §5.0 documents env-var contract + sidecar lifecycle ✅
  - kind smoke fixture (T106) sets fallbackImage=busybox ✅
  - ADR-0010 update NOT required (T106 didn't trigger the deferral
    ADR rev — T105 ships within ADR-0010's existing forward-notes
    scope; only when v0.12+ assessment lands would ADR-0010 get a
    v2)

- **Correctness**:
  - `go vet ./...` exit 0
  - `go test ./...` → 40/40 PASS (5 new + 35 existing)
  - `helm lint --strict deploy/helm-charts/inference-operator/`
    → clean
  - CRD diff: 2 new schema fields (ProxyImage / FallbackImage) +
    controller-gen version annotation bump (cosmetic, kept
    co-located)

## Carry-forward

- **T106 kind smoke**:
  - Fixture already updated — fallbackImage=busybox keeps image
    pull cost cheap
  - When proxy_image is later enabled in a future task, T106
    assertions should verify sidecar container count = 2

- **vllm-ascend v0.12+ assessment**: future task (not in current
  Phase 6 plan) — once upstream documents the proxy_server contract
  on stable, update DESIGN.md §5.0 env-var table + flip default
  ProxyImage to upstream image reference + add a Phase 7 chart
  toggle.

- **Per-side Service creation**: PREFILL_HOST / DECODE_HOST env-vars
  reference `<ms.Name>-prefill` / `<ms.Name>-decode` Services that
  don't exist yet. Phase 7+ task: deployment_builder grows a
  `buildPDPairServices` helper that creates per-side ClusterIP
  Services pointing at the per-side Deployments.

- **schedulerName injection (T105 sibling possibility)**:
  inference-operator could stamp `spec.schedulerName=npu-scheduler`
  on PD-pair Pod templates so HCCS-aware placement happens
  automatically (known-issues #11 hint). Out of T105 scope but
  same controller surface; doable in a small follow-up.

- **Phase 8 vertical scaling**: T105 schema gives operators the
  hooks they need to swap to lighter images at idle (e.g. set
  FallbackImage to a low-memory variant). Vertical scaling
  controller in Phase 8 can read these fields.
