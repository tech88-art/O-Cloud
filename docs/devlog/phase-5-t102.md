# P5-T-102 · PD Router admission webhook scaffold

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 0.5d · actual ~40min

## Intent

Land the PD Router mutating admission webhook scaffold: handler that
always returns Allowed without a patch, registered in the same
manager binary as the controller, with K8s objects (Service +
MutatingWebhookConfiguration) wired through the helm chart and TLS
sourced from the T101 cert-manager Certificate. T103 will replace
the always-Allow body with real annotation-injection logic; T104
adds envtest coverage.

## Path adaptations

- **Added new `Service` template not explicitly in T102 path list**.
  Plan acceptance "helm template renders MutatingWebhookConfiguration"
  is unachievable without a Service for `clientConfig.service` to
  reference. Service template is the natural T102 companion of the
  MutatingWebhookConfiguration template.
- **Added port 9443 + TLS volumeMount to deployment.yaml**. T102 Allowed
  Paths list says "+kubebuilder:scaffold:builder" register webhook —
  but that requires the manager to actually listen on a port and
  have the cert on disk. Both unavoidable additions.

## Debugging trail

- **`json` import shadow in pd_router_test.go**. Tried
  `import "encoding/json"` and `import "k8s.io/.../json"` — the
  `serializer/json` package collides with the stdlib name. Renamed
  the import to `k8sjson` + dropped the unused `encoding/json`.
  Build error became "k8sjson.SerializerOptions undefined"... wait
  no, that worked. The issue was the stdlib `encoding/json` import
  was actually unused after the rename. Removed it.
- **Webhook server port 9443**. controller-runtime default; the
  Service template targets port 9443 explicitly. Pod ports list
  includes `webhook: 9443` gated by `pdRouter.enabled`.
- **`cert-manager.io/inject-ca-from` annotation requires the
  Certificate Namespace + Name**. Format:
  `<namespace>/<certificate-name>`. Helm template uses
  `{{ .Release.Namespace }}/{{ include "inference-operator.fullname" . }}-webhook`.
  cert-manager's webhook (running in cert-manager namespace) walks
  this reference at admission-config creation time to populate
  caBundle.

## Key decisions

- **failurePolicy=Fail default per ADR-0008 §"Fail-closed default"**.
  Production phased-rollouts override via
  `--set pdRouter.failurePolicy=Ignore` until webhook health is
  verified.
- **objectSelector filters by label**. Most Pods in a cluster don't
  carry the `inference.ocloud.edge.example.com/model-service` label;
  pre-filtering at API server saves a network round-trip per Pod
  admission. The handler still has a defense-in-depth label check
  for the rare race where K8s sends a non-matching Pod.
- **T102 handler always returns Allowed without patch**. Plan
  acceptance "admission.Request for a Pod NOT matching the
  objectSelector → handler not invoked (envtest deferred to T104)"
  is the relaxed scaffold contract. T103 will tighten to actually
  mutate. T104 will introduce envtest coverage.
- **Webhook server lives in the same binary as the controller**.
  ADR-0008 design choice — one container, one cert lifecycle, one
  helm release. The trade-off (single point of failure for both
  webhook + controllers) is documented in DESIGN.md §4.3.
- **TLS cert mount at `/tmp/k8s-webhook-server/serving-certs/`**.
  controller-runtime webhook server default; chart's webhook-tls
  Secret (created by cert-manager) projects into this path.

## Verification

P3 三维度:
- **Existence**:
  - `git status` shows: 2 new files (pd_router.go, pd_router_test.go),
    3 new helm templates (service.yaml, mutatingwebhookconfiguration
    .yaml), 4 modified (cmd/main.go, deployment.yaml, values.yaml,
    DESIGN.md), devlog
- **Completeness**:
  - `go build ./...` → clean (webhook code links into manager binary)
  - `go vet ./...` → clean
  - `go test ./internal/webhook/... -v` → 5 tests pass:
    - ScaffoldAllowsAll (Pod with model-service label)
    - NonMSPod_Allowed (Pod without label)
    - BadPodPayload_AllowedNotDenied
    - LabelModelService_MatchesControllerConstant (drift guard)
    - AnnotationSliceBindings_FormatStable (drift guard)
  - `helm lint --strict deploy/helm-charts/inference-operator` →
    `1 chart(s) linted, 0 chart(s) failed`
  - `helm template test ...` renders 8 kinds:
    Certificate / ClusterRole / ClusterRoleBinding / Deployment /
    Issuer / MutatingWebhookConfiguration / Service / ServiceAccount
- **Correctness**: Plan T102 acceptance bullets
  - `make manager` builds with webhook code linked ✓ (go build
    clean; manager binary registers webhook on startup)
  - `helm template` renders MutatingWebhookConfiguration with
    `cert-manager.io/inject-ca-from` annotation ✓ (verified via
    grep)
  - `failurePolicy=Fail` per ADR-0008 ✓ (verified via grep)
  - Unit test: non-MS Pod handler not invoked → Allowed without
    patch ✓ (TestHandle_NonMSPod_Allowed)

## Carry-forward

- T103 will:
  - Add `inferencev1alpha1.AddToScheme` to webhook Scheme so handler
    can decode ModelService objects
  - Add npu-dra-driver scheme to mgr scheme so handler can read
    NPUSliceAllocation
  - Replace Handle body's always-Allow with: decode Pod → label
    check → look up ModelService → list NPUSliceAllocation by
    claim-namespace + claim-name labels → compose slice-bindings
    annotation value → return admission.Patched
- T104 will:
  - Add envtest-style integration test starting a webhook server
    + posting admission requests via httptest
  - Cover: happy / non-MS Pod / empty pool / cert failure
- T106 kind smoke installs cert-manager + this chart + creates a
  ModelService; webhook must inject annotations onto Prefill/Decode
  Pods within the smoke timeout.
- ADR-0008's "fail-closed default" produces a chicken-and-egg
  problem on cluster bootstrap: the inference-operator's own Pods
  also pass through admission. The objectSelector's label match
  saves us — operator Pods don't carry the model-service label so
  they bypass the webhook. Documented in DESIGN.md §4.3.
