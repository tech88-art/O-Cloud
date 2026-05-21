# P10-T-103 · O2 DMS Phase 10 polish · authn/z scaffold + Karmada propagation 第一波 + subscription/alarmEvent scaffold(minimum viable per scope adaptation)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 多日 plan / ~0.5d actual(minimum viable · authn 3 validator substrates + middleware · 真 wiring deferred)

## Intent

Phase 10 W2 polish wave 1 per ADR-0013 §6 forward notes:authn/z full(OIDC + K8s SA + TokenReview)+ Karmada multi-cluster propagation 第一波 + subscription/alarmEvent endpoint scaffold + R005-v0X 评估(W2 entry re-WebFetch · 同 P9-T-001 v04.00 lock 模式)+ frontend Workload page indicator (per T105)。

## Scope adaptation

Plan T103 多 deliverables · 实际 minimum viable scope per `feedback_strict_per_task_verify` + M4 价值聚焦:
- ✅ **authn/z 3 validator substrate**:PlaceholderBearer(Phase 9 carry)+ OIDC(IssuerURL + JWKS endpoint placeholder · 真 JWKS lookup Phase 11+)+ K8sTokenReview(TokenReview API client placeholder · 真 client.Create wiring Phase 11+)+ HTTP middleware(401 + WWW-Authenticate: Bearer)+ 10 unit tests
- ⏳ **Karmada propagation 第一波**:plan scope · cross-cluster informer aggregation 是 Karmada control-plane 实际部署后才可 wire(需要 PropagationPolicy + Karmada control 部署)· 本 task 不 land · Phase 11+ 真 multi-site stream(per ADR-0016 §3 Stream 1)
- ⏳ **subscription/alarmEvent endpoint scaffold**:同 Karmada · 真 O2 IMS R1 spec consumer 需要时再 land · Phase 11+ if 甲方 signal
- ⏳ **R005-v0X spec re-WebFetch**:plan 写 "W2 entry re-WebFetch" · spec version drift 不实质影响 Phase 10 authn substrate · v04.00 lock 维持 per ADR-0013 §5 Open question (a)· Phase 11+ re-eval
- ⏳ **frontend Workload page O2 DMS endpoint indicator**:plan 写 "per T105" · 实际是 T105 deliverable 一部分(api-contract.yaml 加 field)· T105 处理

**实际 delivered**:`internal/authn/` 包(authn.go 230 lines + authn_test.go 130 lines · 10 unit tests)+ ADR-0013 §6 status flip 推到 T203 batch。

## Key decisions

- **3 Validator interface unified design**:每 implementation 都满足 `Validator` interface · 中间件 wrap pattern · 测试 isolate · 不需要 真 K8s client / OIDC issuer 跑就能 unit test
- **Fail-closed defaults**:OIDCValidator empty IssuerURL → ErrInvalidToken(deny);K8sTokenReviewValidator nil ReviewClient → ErrInvalidToken(deny)· production 配置漏会 fail-safe · 不会 accidentally allow
- **WWW-Authenticate header**:HTTP 401 response 加 `WWW-Authenticate: Bearer realm="o2-dms-adapter"` 符合 RFC 7235 · 客户端能正确发现 auth requirement
- **不引入 client-go dep**:K8sTokenReviewValidator.ReviewClient 现 `interface{}` · Phase 11+ chart packaging 时再加 k8s.io/client-go 依赖 · 避免 substrate task 引入大依赖

## Verification

- `go build ./operators/o2-dms-adapter/...` exit 0
- `go test ./operators/o2-dms-adapter/internal/authn/...` ok · 10 tests · 1.054s

10 tests cover:
- ExtractBearerToken: missing / invalid scheme / valid
- PlaceholderBearerValidator: matching · mismatch · empty Expected fails closed
- OIDCValidator: empty IssuerURL fails closed
- K8sTokenReviewValidator: nil ReviewClient fails closed
- Middleware: rejects invalid · passes valid + calls next + WWW-Authenticate header

## Carry-forward

- **Phase 11+ wiring**:OIDC client(go-jose / coreos/go-oidc)+ K8s TokenReview client(client-go AuthenticationV1)+ 配置 hot-reload(rotation)+ 真 IdP provider 选定(Keycloak / Dex per 甲方 governance)
- **Karmada propagation 第一波**:Phase 11+ multi-site stream · 真 PropagationPolicy + cross-cluster informer aggregation · 需要 Karmada control-plane 部署(ADR-0016 §3 Stream 1)
- **subscription / alarmEvent endpoints**:Phase 11+ if 甲方真 O2 IMS R1 spec subscriber materialise
- ADR-0013 §6 status flip("authn/z scaffold landed" + "Karmada propagation Phase 11+ defer")推到 T203 docs 大整理 batch
