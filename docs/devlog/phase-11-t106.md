# P11-T-106 · O2 DMS authn chart wiring(OIDC client + TokenReview SA env injection)

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 1d plan / ~0.2d actual(chart edit + RBAC conditional)

## Intent

ADR-0017 §2 Decision A Stream 5 part 1 substrate · P10-T-103 已 ship 3 Validator interface(Placeholder/OIDC/TokenReview)+ middleware substrate · 本 task land chart wiring 让 operators 通过 chart values 选 authn mode + 注入 OIDC issuer/audience + TokenReview SA name + RBAC conditional gate(tokenReview mode 才加 authentication.k8s.io/v1.tokenreviews create verb)。完整 OIDC IdP 部署(Keycloak/Dex)留 Phase 12+ per ADR-0017 §2 Decision A "Stream 5 part 2" not in scope。

## Path adaptations

无。chart wiring 路径与 ADR-0013 §6 forward note + ADR-0017 §2 Decision A Stream 5 part 1 align。

## Debugging trail

无 build/test fail · 一次 helm lint + template clean。

## Key decisions

- **`auth.mode` enum 字段**(placeholder/oidc/tokenReview)· operators 选一种 · 不同 mode 注入不同 env · 后端 cmd/main.go startup 走 `O2DMS_AUTH_MODE` env 分发到对应 Validator(P10-T-103 substrate 已 ship interface · 此处 chart 端 wire)
- **`O2DMS_OIDC_ISSUER_URL` + `O2DMS_OIDC_AUDIENCE` 仅 mode=oidc 时注入**(条件 env block)· 同样 `O2DMS_TOKENREVIEW_SA_NAME` 仅 mode=tokenReview。避免不相关 env 污染。
- **`authentication.k8s.io/v1.tokenreviews create` RBAC 条件 add**(`{{- if eq .Values.auth.mode "tokenReview" }}`)· placeholder/oidc mode 不加这 verb · 最小 blast radius
- **保留 `auth.bearerToken` 字段 backward compat**(Phase 9 placeholder mode 路径)· placeholder mode operators 旧 chart values 不破
- **OIDC client 真 wire 留 后续**:cmd/main.go 读 env 后 构造 OIDC Validator(P10-T-103 已 ship 接口 + Validator impl)· 当前 chart 仅 plumbing · 真 dial issuer 留 mid-Phase 11 / Phase 12+ 完整 IdP deploy 时 verify。

## Verification

- 存在性:
  - chart values.yaml `auth.mode/oidc/tokenReview` 三段 ✓
  - deployment.yaml env block 加 5 conditional env(O2DMS_AUTH_MODE + 4 conditional)✓
  - rbac.yaml ClusterRole 加 conditional tokenreviews verb ✓
- 完整性:
  - `helm lint --strict deploy/helm-charts/o2-dms-adapter/` clean
  - `helm template --set auth.mode=tokenReview` → tokenreviews resource + O2DMS_AUTH_MODE env render
  - `helm template --set auth.mode=oidc --set auth.oidc.issuerUrl=https://kc.example.com --set auth.oidc.audience=ocloud` → OIDC env render(verified via grep · render OK)
  - `helm template`(default · mode=placeholder)→ 仅 O2DMS_AUTH_MODE env(无 OIDC/TokenReview env · 无 tokenreviews RBAC verb)
- 正确性:RBAC conditional gate align with mode 字段值 · 不会出现 mode=placeholder 时还 grant tokenreviews verb(降级 防御)

## Carry-forward

- **cmd/main.go OIDC + TokenReview Validator wiring**:P10-T-103 Validator interface 已 ship · 此 task ship 的是 chart 端 plumbing · main.go 加 `os.Getenv("O2DMS_AUTH_MODE")` switch → 构造对应 Validator(OIDCValidator with issuer/audience · TokenReviewValidator with SA)· 留 后续 task / Phase 12+
- **P11-T-201 master-demo-multi-site.sh**:可 demo `--set auth.mode=tokenReview` 路径 · 演示 K8s SA token validation 端到端
- **Phase 12+ 完整 OIDC IdP deploy**(per ADR-0017 §2 Decision A Stream 5 part 2 · 主线 spine not in Phase 11 scope):Keycloak / Dex chart deploy + 真 federation provider integration(Active Directory / LDAP / Google Workspace)· 与 Vault Secret(ADR-0018 §4 (c))一起 cohort

## §0a 续 autonomous · 继续 T107 kind smoke E2E extension
