# P13-T-202 · [A1] Dex 参考 IdP 部署 + RBAC manifest + helm authz wiring

- **Commit**: this commit (main agent verified + committed)
- **Date**: 2026-06-03
- **Duration**: plan 1.5d vs actual ~0.5d (offline layer only; Dex token→OIDCValidator end-to-end + real-cluster apply are lab-gated)

## Intent

Close build-doc §5 item 1 (认证授权) deploy-side per ADR-0025 §2 Decision A: (1) a right-sized
**Dex reference OIDC IdP** (`deploy/idp/`) as the issuer the real-profile o2-dms `OIDCValidator`
(P13-T-201) validates against, with a documented customer swap point; (2) **least-privilege RBAC**
across the component charts CLOSING the T103 / T104 / T201 carries + the T204 ClusterQuota-read
forward-provision; (3) authz/IdP wiring into the real profile overlays.

## Path adaptations

- **No `helm` on the dev box.** CI pins helm v3.16.0 (`helm-lint.yml:41`); downloaded that exact
  binary into a scratch dir (NOT committed) so lint/template ran as CI does. Also `go install`-ed
  **kubeconform v0.7.0** for offline K8s-schema validation of the raw Dex manifest.
- **Dex = raw manifests, not a vendored chart** — directly YAML-parse + kubeconform-verifiable
  offline, self-documents the swap point, and right-sizes a *reference* issuer. Image pinned
  `ghcr.io/dexidp/dex:v2.41.1` (multi-arch). **Storage = `memory`** → Dex never calls the apiserver
  → ZERO RBAC on it (plain SA, `automountServiceAccountToken: false`).
- **`kubectl --dry-run=client` can't run fully offline** (needs a server for OpenAPI + REST-mapping;
  verified). Authoritative offline checks = Python `yaml.safe_load_all` + kubeconform; the
  `--dry-run=server` apply is the T301 lab step (documented in the README).
- **No o2-dms real-profile values existed** → created `deploy/profiles/real/o2-dms-adapter.values.yaml`.
- **values.yaml edits beyond the literal Allowed Paths** (`templates/rbac.yaml` only): the new RBAC
  gate flags (`rbac.clusterRead/deployWrite`, `auth.tokenReview.rbac`) MUST have defaults in each
  chart's `values.yaml` — `helm lint --strict` FAILS on an undefined referenced value (verified via
  stash test: demo-backend went red without them). The values are the configuration surface of the
  rbac template, so this is an inseparable corollary; flagged for the main agent.

## Debugging trail

- **🔴 Env-name drift (o2-dms OIDC issuer) — #1 finding.** Chart `deployment.yaml` emits
  `O2DMS_OIDC_ISSUER_URL`, but T201 code (`cmd/main.go:155`) reads `O2DMS_OIDC_ISSUER` (no `_URL`) —
  grep-confirmed, and the rendered real profile shows the `_URL` key. So `auth.oidc.issuerUrl` does
  NOT activate the OIDCValidator today; selection falls through to the in-cluster TokenReview
  validator. `deployment.yaml` is OUTSIDE T202 Allowed Paths → documented (README + both profile
  headers), not silently edited. This is WHY I grant TokenReview RBAC even on the OIDC profile.
- **TokenReview RBAC was gated on `auth.mode=="tokenReview"`**, but T201 made TokenReview the
  in-cluster PRIMARY path (active whenever `O2DMS_OIDC_ISSUER` is unset — independent of `auth.mode`).
  A `mode: oidc` profile would have had NO `tokenreviews: create` verb while running the TokenReview
  validator → 401s. Fix: added `auth.tokenReview.rbac` toggle granting the single verb INDEPENDENTLY
  of mode (`or (eq .mode "tokenReview") .tokenReview.rbac`). One verb on one resource — NOT the broad
  `system:auth-delegator` ClusterRole (the ADR-0025 §2 Decision A alternative).
- **Python YAML `UnicodeDecodeError` ×2** — Windows GBK-console artifacts on the Chinese `·` comments,
  NOT YAML errors. Forcing `encoding='utf-8'` and parsing from a UTF-8 file (not the console pipe)
  parsed clean; kubeconform (reads bytes) validated the same renders throughout.
- **Dex bcrypt hash** = the canonical upstream Dex example hash for "password" (published reference
  value, structurally valid bcrypt, byte-identical to Dex's documented example) — used rather than a
  hash I couldn't verify offline (no `bcrypt` module). Marked DEMO ONLY.

## Key decisions

- **RBAC defaults OFF; real profile flips ON** (RBAC blast-radius mirrors demo/real isolation). Each
  toggle 1:1 maps a real datasource body: `rbac.clusterRead`→T103, `rbac.deployWrite`→T104,
  `auth.tokenReview.rbac`→T201. demo-default demo-backend renders Lease-only (verified).
- **demo-backend: ClusterRole for cluster-scoped reads, namespaced Role for namespaced writes.**
  T103 reads `resource.k8s.io/resourceslices` + `nodes` (cluster-scoped → ClusterRole, READ-ONLY).
  T104 writes `apps/deployments` + `core/services` (namespaced → Role in `workloadNamespace`,
  default release ns; verbs exactly what `deploy.go` issues incl. `list` for the label-selector
  delete). NO secrets/configmaps/RBAC-write.
- **inference-operator ClusterQuota = controller-owner verbs** (mirrors `quotas`): full CRUD +
  status + finalizers; cluster-scoped → in the existing ClusterRole. FORWARD-PROVISION — T204's code
  (the consumer) isn't landed yet (grep: 0 ClusterQuota reads in inference-operator/internal/ today).
- **NO Karmada cross-cluster RBAC PropagationPolicy (deliberate non-action).** Karmada policies
  propagate CRD *instances* only; components + their RBAC install per-cluster via component helm
  charts. Propagating these ClusterRoles would duplicate per-cluster helm RBAC AND intrude on T205's
  explicit "cross-cluster RBAC propagation" ownership. Left `deploy/karmada/policies/` untouched.
- **ascend-npu-exporter-plus has NO rbac.yaml — correctly.** Node-local DaemonSet, hostPath DCMI/
  npu-smi reads, no K8s client (grep: 0 `NewForConfig`/`InClusterConfig`). Zero apiserver RBAC is
  the right least-privilege state.

## Verification (offline · no real cluster · plan §8 layer 1)

- **helm lint --strict + helm template (CI-equivalent, helm v3.16.0) on ALL 9 charts** → every chart
  `lint=0 template=0`. Baseline (pre-edit) also all-green → no regression.
- **real-profile overlay renders** (demo-backend / o2-dms / inference-operator `-f profiles/real/...`) → exit 0.
- **kubeconform (offline, k8s 1.31)**: `deploy/idp/dex.yaml` 5 resources Valid:5; rendered RBAC —
  demo-backend 6 valid, o2-dms 2 valid, inference-operator 2 valid.
- **Python `yaml.safe_load_all`** (UTF-8): dex.yaml 5 docs + the embedded Dex config block parse;
  both new/edited real-profile values parse.
- **Wildcard scan (P4 headline least-privilege)**: `grep` for `verbs:["*"]`/`resources:["*"]`/`- "*"`
  across all chart rbac.yaml (source) AND rendered RBAC of the 3 touched charts → **0 hits**.
- **Exact rendered Role rules** (parsed from UTF-8 helm output):
  - demo-backend real: CR `resource.k8s.io/resourceslices`[get,list,watch] + `""/nodes`[get,list,watch];
    Role(ns=release) `apps/deployments`+`""/services`[get,list,watch,create,update,patch,delete]; Lease preserved.
  - o2-dms real: CR `authentication.k8s.io/tokenreviews`[create].
  - inference-operator real: CR `clusterquotas`[get,list,watch,create,update,patch,delete] + `/status`[get,update,patch] + `/finalizers`[update,patch].
  - deploy Role namespace honors `--namespace ocloud-system` + `rbac.workloadNamespace` override.

**Main-agent independent re-verification (P7 · method ≠ the subagent's helm path — helm scratch
was removed, so a DIFFERENT lens)**: (1) `yaml.safe_load_all` on dex.yaml (5 docs) + both real-profile
values → parse clean; (2) env-key drift re-confirmed by grep — code `cmd/main.go:155` reads
`O2DMS_OIDC_ISSUER`, chart `deployment.yaml:50` emits `O2DMS_OIDC_ISSUER_URL` (finding accurate;
correctly left as documented carry-forward, deployment.yaml ∉ T202 paths); (3) wildcard scan across
all `helm-charts/*/templates/rbac.yaml` → 0 hits; (4) `{{ if }}`/`{{ end }}` balanced in all 3 touched
templates (3/3, 2/2, 1/1); (5) all referenced value toggles have defaults (demo-backend `rbac.*` OFF,
o2-dms `auth.tokenReview.rbac` OFF). Full helm v3.16.0 render is re-run by the CI `helm-lint` gate at
the phase tag (per memory `feedback_post_tag_ci_gate`).

**Real-machine (LAB-GATED, deferred · ADR-0025 §4)**: Dex token → o2-dms OIDCValidator live round-trip
+ `--dry-run=server` apply = the T301 integration stamp.

## Carry-forward

- **🔴 #1 — o2-dms OIDC env key fix (T201 follow-up / separate scope).** Rename chart env
  `O2DMS_OIDC_ISSUER_URL → O2DMS_OIDC_ISSUER` in o2-dms `deployment.yaml` (+ add
  `O2DMS_OIDC_ALLOWED_CLAIMS`) so `auth.oidc.issuerUrl` activates the OIDCValidator against Dex.
  Until then the real profile runs the in-cluster TokenReview validator (RBAC granted via
  `tokenReview.rbac: true`, so auth WORKS — just via TokenReview not OIDC/Dex). Out of T202 paths.
- **T204**: inference-operator ClusterRole now grants `clusterquotas` read/write; T204 separately owns
  the kubebuilder `config/rbac/*.yaml` markers — keep the two surfaces consistent when it lands.
- **T205**: I added no cross-cluster RBAC policy (rationale above); if T205 needs control-plane RBAC
  propagated that's its call.
- **T203**: replace the reference `dex-client-secrets` Secret (inline placeholders) with an
  ExternalSecret → Vault (noted in dex.yaml + README).
- **Scratch tooling NOT committed**: `.tmp-helm/` (helm + kubeconform + render scratch) — removed
  before reporting; main agent confirm it's not staged.
