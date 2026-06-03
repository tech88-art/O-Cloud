# P13-T-201 · OIDC/TokenReview validator 真体 + middleware wiring + backend SA token

- **Commit**: <sha> (main agent stamps · branched from 5d30865)
- **Date**: 2026-06-03
- **Duration**: plan 2.5d vs actual ~0.4d (substrate was well-shaped by P10-T-103; the work was filling bodies + tests, not design)

## Intent

Close build-doc §5 item 1 (认证授权) for the real profile, per ADR-0025 §2 Decision A.
Three placeholders that fail-closed-but-do-nothing became real:
(1) o2-dms `OIDCValidator.Validate` — OIDC discovery → JWKS → JWT signature + iss/aud/exp + AllowedClaims;
(2) o2-dms `K8sTokenReviewValidator.Validate` — real `authentication.k8s.io/v1.TokenReview` API call;
(3) o2-dms `cmd/main.go` — wire `authn.Middleware`, config-select validator, drop PlaceholderBearer as default;
(4) backend prometheus — static Bearer → in-cluster ServiceAccount token (auto-rotate aware).
**This task spans TWO modules** (`operators/o2-dms-adapter` + `backend/pkg/datasource/prometheus`)
— its Allowed Paths explicitly cover both.

## Path adaptations (if any)

- **JWT/OIDC lib**: chose `github.com/coreos/go-oidc/v3` v3.18.0 (+ transitive
  `github.com/go-jose/go-jose/v4` v4.1.4, used directly for test JWT signing). Neither was
  present in either go.mod; added to o2-dms via `go get`. go-oidc does discovery + RemoteKeySet
  JWKS fetch/rotate + signature/iss/aud/exp verification in one `Verifier.Verify` call — exactly
  right-sized per ADR-0025 §3 (not a full IAM SDK). **Pure Go → arm64 cross-compile clean, no
  build-tag isolation needed** (contrast the DCMI/cgo concern called out for T101/T102 in plan §9).
- **`ReviewClient interface{}` → typed**: promoted to `kubernetes.Interface` (exposed as
  `TokenReviewClient` alias) now that client-go is in scope (it already was — o2-dms is a
  controller-runtime adapter). Real `*kubernetes.Clientset` and `fake.Clientset` both satisfy it.
- **middleware wiring point**: `internal/api/routes.go` (`NewRouter`) is NOT in Allowed Paths, and
  it constructs the chi router internally. Wired the middleware in `cmd/main.go` instead by wrapping
  `authn.Middleware(validator)(api.NewRouter(handler))` — cleaner anyway (router stays auth-agnostic).
- **backend SA-token wiring**: the prometheus `Options` plumbing in `backend/cmd/demo-backend/main.go`
  and `pkg/config/config.go` + `config.real.yaml` are OUT of Allowed Paths (T201 names only
  `prometheus/source.go` + `metrics.go`). So the Source **auto-detects** the in-cluster SA mount:
  when `BearerToken` is empty and the projected token file exists at construction, it reads the SA
  token per-request. This makes the production path work through the *unchanged* main.go (which
  passes only `URL`), while dev compose (no mount) degrades to no Authorization header. The seam is
  fully testable via explicit `ServiceAccountTokenPath`. **Carry**: a follow-up may surface this as
  an explicit config field — flagged below.
- **test home for SA-token**: added cases to the existing `prometheus/metrics_test.go` (not a new
  file) — it already owned `TestQueryMetric_BearerTokenHeader`. New-file creation would have
  stretched Allowed Paths further than editing the adjacent same-module test.

## Debugging trail

- First `go build ./internal/authn` failed: `missing go.sum entry for go-jose/go-jose/v4`
  (go-oidc's transitive dep). Fixed with `go get github.com/coreos/go-oidc/v3/oidc@v3.18.0`
  (package-path form pulls the transitive into go.sum).
- First test compile failed: leftover `metav1` import in `authn_test.go` after I dropped the
  explicit `ObjectMeta{}` from the test path. Removed the unused import.
- gofmt flagged all 6 edited files — NOT a CRLF issue (files are LF, verified via byte count).
  Modern gofmt reindents multi-line numbered-list doc comments (`//  1.` two-space form). Ran
  `gofmt -w`; re-verified LF preserved + tests still green.
- go-oidc's `Verify` correctly rejected expired (exp), wrong-aud (ClientID mismatch), and
  bad-signature (different RSA key behind same kid) tokens — all three surface as wrapped
  `ErrInvalidToken`. No leeway surprises (default ~1min leeway didn't mask the 1h-expired case).

## Key decisions

- **fail-closed semantics + ErrNotConfigured**: added a distinct `ErrNotConfigured` sentinel so an
  operator-misconfigured validator (empty IssuerURL / nil ReviewClient) is distinguishable in logs
  from a caller-token rejection (`ErrInvalidToken`). Both still 401 at the middleware — auth never
  falls open. The K8s-TokenReview default with a nil ReviewClient (degraded · no K8s config) returns
  ErrNotConfigured → 401, not silent pass.
- **validator selection** (ADR-0025 §2 Decision A): env-driven. `O2DMS_OIDC_ISSUER` set → OIDC;
  `O2DMS_AUTH_MODE=local-dev` → PlaceholderBearer (DEV ONLY · never the default); otherwise →
  K8sTokenReview (in-cluster primary). PlaceholderBearer is retained but is no longer the default.
- **OIDC verifier built once** (sync.Once) so the RemoteKeySet's JWKS cache/rotation is reused
  across requests; building per-request would re-fetch JWKS every call. `SetVerifier` is the test
  seam (skips discovery, injects a verifier wired to the mock JWKS server).
- **SA token read per-request with a short TTL** (default 1min, well under the ~1h bound-token
  lifetime). NOT cached once at startup (a startup cache goes stale on kubelet rotation → 401s).
  On a refresh read error mid-rotation, the last-good token is served (don't 500 on a transient
  missing file).
- **AllowedClaims** matches scalar string claims and array-of-string membership (e.g.
  `groups: ["viewer","o2-admin"]` satisfies `groups=o2-admin`).

## Verification

Both modules, real commands (P3 三项验证: 存在性 build, 完整性 grep, 正确性 test output):

**o2-dms-adapter** (`operators/o2-dms-adapter`):
- `go build ./...` → exit 0
- `go vet ./...` → exit 0
- `go test ./...` → all packages `ok` (authn: 19 tests incl. the 6 required states —
  OIDC valid/expired/wrong-aud + TokenReview valid/not-authenticated/wrong-aud — plus
  AllowedClaims, bad-signature, not-configured edge cases)
- `CGO_ENABLED=0 GOARCH=arm64 GOOS=linux go build ./...` → exit 0 (go-oidc/go-jose pure Go)
- `gofmt -l` on edited files → clean
- `go mod tidy` → diff is exactly +go-oidc/v3 +go-jose/v4 (direct) + oauth2 0.34→0.36 (transitive)

**backend** (`backend`):
- `go build ./...` → exit 0
- `go test ./pkg/datasource/prometheus/...` → `ok` (5 SA-token tests: ReadAndSent, AutoRotate,
  RefreshFailureKeepsLastGood, NoToken_NoAuthHeader, + pre-existing BearerTokenHeader static path)
- `CGO_ENABLED=0 GOARCH=arm64 GOOS=linux go build ./...` → exit 0
- `go mod tidy` → no diff (stdlib-only additions: os/sync/path-filepath/time)

**Horizontal scan (P4)**:
`grep -rn "PlaceholderBearer\|静态.*[Tt]oken\|Bearer \"+s.bearer" operators/o2-dms-adapter backend/`
→ `Bearer "+s.bearer` (old backend static-token usage): **0 hits** (superseded by tokenSource).
`PlaceholderBearer` hits remain only in: the type definition (authn.go · intentionally retained
local-dev validator), main.go's explicit `local-dev` branch + doc comments, and the test file.
All superseded/local-dev-only per spec.

## Carry-forward

- **T202 (Dex IdP + RBAC)**: consumes `OIDCValidator` (set `O2DMS_OIDC_ISSUER`/`O2DMS_OIDC_AUDIENCE`
  via helm values) + must add a TokenReview RBAC ClusterRole/Binding for the o2-dms ServiceAccount
  (`system:auth-delegator` or a `tokenreviews: create` Role) so the in-cluster primary path works.
  Also needs the backend ServiceAccount projected-token volume + (optionally) a Prometheus-side
  RBAC/bearer policy. **RBAC/Secret-mount drift is a carry for T202/T203** — NOT wired here
  (those paths are out of T201 scope).
- **backend prometheus config surface**: SA-token use is currently auto-detected (mount exists →
  use it). If a future task wants an explicit knob, add a `prometheus.serviceAccountTokenPath` /
  `prometheus.useInClusterToken` field to `pkg/config` + `config.real.yaml` + wire in
  `cmd/demo-backend/main.go` (all out of T201 Allowed Paths). The Source already exposes
  `Options.ServiceAccountTokenPath` / `DisableInClusterTokenAutodetect` / `TokenRefreshTTL`.
- **real-IdP integration (Dex token end-to-end)**: deferred to T301 lab verification (真机层) —
  the offline layer here proves the JWKS+JWT machinery against a mock issuer; a live Dex token
  round-trip through `OIDCValidator` is the T301 integration stamp.
- **What the main agent should double-check**: (a) the backend SA-token auto-detect strategy is the
  right scope call given main.go/config are out of Allowed Paths (alternative: expand scope to wire
  an explicit config field); (b) o2-dms go.mod now has 2 new direct deps — confirm acceptable for
  the module; (c) golangci-lint (CI-only, not run locally) on the new code — gofmt+vet are clean.
