# P4-T-004 · npu-dra-driver Ascend ResourceSlice + ResourceClaim types (api/v1alpha1)

- **Commit**: 023f89a
- **Date**: 2026-05-19
- **Duration**: plan 1d · actual ~1h

## Intent

Define Ocloud-specific Go helpers that project to/from upstream `resource.k8s.io/v1beta1.{ResourceSlice,ResourceClaim}` shapes. Ship 6 attribute QualifiedNames (npu.huawei.com/{index,health,slice-strategy,ai-cores,numa-node,hccs-ring}) + 1 capacity QualifiedName (npu.huawei.com/slice-aicore) + 2 annotation keys (ocloud.edge.example.com/{model-service-ref,preferred-pool}) + typed-view structs with round-trip helpers + 10 sub-tests.

## Path adaptations

**PROJECT no-CRD entry**: plan said "register `api/v1alpha1/` resource entry per Kubebuilder convention". But Phase 4 ships no actual CRDs (uses upstream resource.k8s.io types) — only typed-view helper structs. Resolution: added a resources entry but **omitted `crdVersion`** so `make manifests` does not try to generate empty CRD YAMLs. Documented this inline in PROJECT with a 5-line comment.

## Debugging trail

- Verified `k8s.io/api@v0.35.0` ships `resource/v1` + `resource/v1alpha3` + `resource/v1beta1` + `resource/v1beta2`. Plan said use v1beta1 — followed plan. v1 is the GA path (K8s 1.34) but v1beta1 is still on by default everywhere.
- Read v1beta1 types.go for actual `Device` shape — confirmed `Device.Basic.Attributes` is `map[QualifiedName]DeviceAttribute` (not just `map[string]string`). Attribute is union of IntValue/BoolValue/StringValue/VersionValue.
- Tested empty round-trip case explicitly: `AscendDevice{}` zero-value goes through ToUpstream → FromUpstream without erroring, but fails ValidateAttributes (empty Health = invalid enum). Test asserts this contract explicitly.
- Unknown-attribute warn-not-error: injected `npu.huawei.com/future-phase5-field` into upstream Attributes map → FromUpstream should still return non-error. Test passes.

## Key decisions

- **`AscendDevice` is a typed wrapper, not a CRD root**: no `metav1.TypeMeta` + no `metav1.ObjectMeta`. It's a Go struct with `+kubebuilder:object:generate=true` so controller-gen emits DeepCopy. The struct never travels over the API — it projects to/from `resource.k8s.io/v1beta1.Device.Basic` at the publisher boundary.
- **Same pattern for `AscendClaimAnnotations`**: typed view of metadata.annotations. ToMap omits empty fields (so the resulting map only contains keys the caller actually set); FromMap tolerates unknown keys.
- **DriverName const at package level**: `DriverName = "npu.ocloud.edge.example.com"`. Phase 4 publisher (T005) + Phase 4 pool-operator cross-watch (T102) + Phase 5 inference-operator all read this. Centralised once.
- **controller-gen run command** (cached for future module work):
  ```
  GOSUMDB=off GOPROXY=https://goproxy.cn,direct \
    go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.20.1 \
    object:headerFile="hack/boilerplate.go.txt",year=2026 paths="./api/v1alpha1/..."
  ```
  Initial attempt with default GOPROXY failed at sum.golang.org lookup (EOF). The Chinese mirror + GOSUMDB off bypassed the dependency-verification network requirement.

## Verification

P3 三维度:
- Existence: `git ls-files operators/npu-dra-driver/api/v1alpha1/` → 5 files (types.go + resourceslice_types.go + resourceclaim_types.go + types_test.go + zz_generated.deepcopy.go)
- Completeness: `go test ./api/v1alpha1/... -v -run TestRoundTrip` → 5/5 sub-tests PASS (empty / single-device / multi-device / Dynamic-strategy / unknown-attribute-warn-not-error)
- Correctness: + bonus `TestClaimAnnotationsRoundTrip` 5 sub-tests PASS; `go vet ./...` clean; `go build` still produces 58 MB manager binary

## Carry-forward

- T005 imports `v1alpha1.AscendDevice` + `v1alpha1.DriverName` + the attribute QualifiedName consts. Publisher's `buildSlice` function emits one upstream `Device` per `AscendDevice` via `AscendDevice.ToUpstream()`.
- T006 imports `v1alpha1.DriverName` for the class-name prefix filter. Also imports `v1alpha1.AscendClaimAnnotations` types — actually doesn't, since Phase 4 claim controller uses annotation key constants directly (encoded into `metadata.annotations` map). Phase 5 may switch to AscendClaimAnnotations typed view if it grows beyond 2 keys.
- T102 pool-operator can't import these types (no cross-module imports per operators/CLAUDE.md §1) — pool-operator hardcodes the same `npu.ocloud.edge.example.com` string in `npuDraDriverName` const. Acceptable duplication; documented inline.
