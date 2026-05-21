# P10-T-104 · Quota Phase 10 polish · token-bucket rate algorithm + 6 unit tests(cluster-scope ClusterQuota + Karmada deferred per scope adaptation)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 多日 plan / ~0.3d actual(minimum viable · token-bucket substrate + tests · cluster-scope ClusterQuota + Karmada cross-cluster propagation deferred)

## Intent

Phase 10 W2 polish wave 1 per ADR-0014 §7 forward notes:cluster-scope ClusterQuota CRD + Karmada cross-cluster Quota propagation + token-bucket algorithm option + frontend visualisation。Minimum viable scope:token-bucket substrate + tests · ClusterQuota CRD + Karmada propagation deferred(Phase 11+ multi-site stream per ADR-0016 §3 Stream 1)。

## Scope adaptation

- ✅ **token-bucket algorithm option**:`internal/quota/ratealgo/tokenbucket.go` thread-safe impl + 6 unit tests(burst capacity · refill over time · capacity clamp · zero defaults · AllowN multi-token · AllowN zero no-op)
- ⏳ **cluster-scope ClusterQuota CRD**:plan acceptance "`inference.ocloud.edge.example.com/v1alpha1/ClusterQuota` per ADR-0014 §7 (b)"· CRD types + scheme + webhook integration · 0.5-1d work · Phase 10 W2 polish 余 capacity 不够 · Phase 11+ multi-site stream(ClusterQuota natural fit · cross-cluster usage 累计)
- ⏳ **Karmada cross-cluster Quota propagation**:同 Karmada control-plane 部署条件 · Phase 11+ multi-site stream
- ⏳ **frontend Quota usage visualisation**:T105 deliverable 一部分(api-contract.yaml field)
- ⏳ **webhook 集成 token-bucket** as opt-in `spec.enforcement.rateAlgorithm` enum:Phase 10 W2 polish 候选 · token-bucket substrate ready · webhook admission path 添加 algorithm switch 是 single-file edit · 但 Quota CRD schema 需扩 enum field · deferred to Phase 11+ if production rate-limit policy 需求

## Key decisions

- **token-bucket thread-safe** via `sync.Mutex`(refill + Allow 原子操作)· Quota webhook 在高并发 admission scenario 需要 thread safety
- **Capacity = MaxEvents · RefillRate = MaxEvents / Window**:与 plan T104 acceptance "token-bucket algorithm option" 对齐;sliding-window remains default per ADR-0014 §3 backwards compat
- **AllowN multi-token** API:Phase 11+ webhook 可一次 reserve N tokens(如批量 NPUSliceAllocation 一次 deduct multiple events)
- **Zero defaults fallback**:`NewTokenBucket(0, 0)` → capacity=1 · 防止 misconfigured Quota 误为 always-deny

## Verification

- `go build ./operators/inference-operator/...` exit 0
- `go test ./operators/inference-operator/internal/quota/ratealgo/...` ok · 6 tests · 1.388s

6 tests cover:
- AllowsBurstUpToCapacity(burst cap)
- RefillsOverTime(time-based refill verified by sleep 200ms)
- CapacityClamp(refill 不超 cap)
- ZeroDefaults(NewTokenBucket(0,0) capacity=1)
- AllowN multi-token consumption
- AllowN(0) no-op success

## Carry-forward

- **Phase 11+ ClusterQuota CRD**:multi-site usage aggregation natural fit · 同 Karmada propagation 联动
- **Phase 11+ Karmada cross-cluster Quota**:per ADR-0016 §3 Stream 1 multi-site deployment 后再 wire
- **Phase 10 W2 polish webhook 集成**(opt-in token-bucket):若 production rate-limit signal · webhook admission path 加 algorithm switch · 1-2h work
- ADR-0014 §7 status update("token-bucket substrate landed" + "ClusterQuota Phase 11+ defer")推到 T203 batch
