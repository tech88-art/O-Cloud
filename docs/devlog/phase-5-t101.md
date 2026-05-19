# P5-T-101 · cert-manager helm dep + Certificate CRD wiring

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 0.5d · actual ~20min

## Intent

Extend the T006 inference-operator helm chart with cert-manager
Certificate + (self-signed) Issuer templates so the PD Router
admission webhook (T102+) has a TLS cert ready when it lands.
Pre-install convention chosen over subchart dependency (per Phase 5
entry meeting recommendation): cert-manager must be installed in the
cluster before the chart is rendered; the chart only renders the
cert-manager CRs that point at it.

## Path adaptations

- **No subchart `dependencies:` block in Chart.yaml**. The plan
  acceptance mentions both options ("subchart dependency vs pre-
  install requirement"). Subchart binds a specific cert-manager
  version to the inference-operator chart's release — convenient for
  green-field installs but creates upgrade coupling cluster
  operators may not want. Pre-install is the path most production
  deployments take. The chart documents the requirement
  prominently (README + known-issues #10) but does not enforce it
  via Chart.yaml.

## Debugging trail

- **`helm lint --strict` clean on first try**. The Certificate +
  Issuer templates are well-trodden cert-manager patterns —
  reproduced from the cert-manager docs sample without surprise.
- **Toggle matrix tested**:
  - `--set certManager.enabled=true` (default) → 2 cert-manager
    kinds rendered (Certificate + Issuer) + 4 K8s base kinds
  - `--set certManager.enabled=false` → 0 cert-manager kinds; only
    4 base kinds (ClusterRole/RB + Deployment + ServiceAccount)

## Key decisions

- **Self-signed Issuer by default + opt-out flag**. Phase 5 simplicity:
  CI/kind environments use the self-signed Issuer; production deploys
  override `certManager.issuer.create=false` + point
  `certManager.issuer.kind=ClusterIssuer` + `name=<their-issuer>`.
- **dnsNames cover Service + Service.namespace.svc + .svc.cluster.local
  shapes**. The PD Router webhook (T102) will receive Pod admission
  requests via a Service named `<release>-inference-operator-webhook`.
  All three DNS forms are needed because the K8s API server resolves
  webhook URLs differently depending on cluster networking.
- **Certificate duration 8760h (1 year) with 720h (30 day) renew
  window**. cert-manager defaults; explicit in values.yaml so CI can
  shrink for short-lived test clusters.
- **Issuer named `<release>-inference-operator-selfsigned`**. Includes
  the release name for multi-release coexistence in the same
  namespace (rare but supported).

## Verification

P3 三维度:
- **Existence**:
  - `git status` shows: 2 new files (templates/certificate.yaml,
    templates/issuer.yaml), 3 modified (values.yaml, README.md,
    docs/known-issues.md), devlog
- **Completeness**:
  - `helm lint --strict deploy/helm-charts/inference-operator` →
    `1 chart(s) linted, 0 chart(s) failed`
- **Correctness**: Plan acceptance bullets
  - `helm template ... --set certManager.enabled=true` renders both
    Certificate + Issuer ✓ (6 total kinds: 4 base + 2 cert-manager)
  - `helm template ... --set certManager.enabled=false` renders
    neither ✓ (4 total kinds, no cert-manager)
  - `helm lint --strict` passes ✓
  - Pre-install ordering documented in README + docs/known-issues.md
    entry #10 (cert-manager → wait → inference-operator)

## Carry-forward

- T102 webhook scaffold:
  - Add `--enable-pd-router-webhook` flag to manager (default true)
  - Register admission.Handler boilerplate in
    `internal/webhook/pd_router.go`
  - Add Service for the webhook (port 443, targetPort 9443) and
    expose `internal/webhook` package
  - Add `templates/mutatingwebhookconfiguration.yaml` that references
    the Certificate's caBundle via `cert-manager.io/inject-ca-from`
    annotation
  - Webhook Pod Spec needs the TLS secret mounted at
    `/tmp/k8s-webhook-server/serving-certs`
- T106 kind smoke must install cert-manager before this chart:
  `kubectl apply -f https://github.com/cert-manager/cert-manager/
  releases/download/v1.16.0/cert-manager.yaml` + wait for the
  cert-manager deployment.
- Production override path documented in README values table —
  cluster operators with a real CA point at their own Issuer/ClusterIssuer.
