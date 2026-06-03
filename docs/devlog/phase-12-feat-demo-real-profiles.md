# P12-feat · demo/real 两个交付版本的架构分离(config + deploy profile)

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-02
- **Trigger**: 用户要求把 demo(验证数据)与真机(物理数据)做成**两个可选交付版本**,通过架构分离、由用户决定验证哪个;真机**功能体**按计划留 Phase 13(新 session)。
- **Plan**: `~/.claude/plans/sprightly-meandering-melody.md`(plan 模式批准)。

## 方案

单一代码库 + 运行时数据源抽象(`mapping.<resource>`)+ 既有 chart values(`publisher.sourceType` / `simulator.enabled` / demo-backend 内联 `config.yaml` / 默认 soft arm64 亲和)→ **profile 分离,不 fork、几乎不动 chart 模板**。

### 新建
- `backend/configs/config.real.yaml` — 真源 mapping(clusters/nodes/npus→k8s · pools→crd · metrics→prometheus · presets→configmap · logs→k8s;**topology/deploy 仍 mock**)。
- `deploy/profiles/{demo,real}/{demo-backend,ascend-npu-exporter-plus,npu-dra-driver,inference-operator}.values.yaml` + 各一份 `README.md`(real README 列 Phase-13 边界 + 前置 + mock-data 缺口)。

### 修改
- `scripts/install.sh` — 加 `--profile demo|real`(默认 demo)· `profile_values_arg()` helper 把 `--values=deploy/profiles/<p>/<chart>.values.yaml` 串进 exporter + npu-dra-driver 的 helm 步 · real 选 npu-dra 时 warn `ErrNotImplemented` · usage/summary 更新。
- `backend/configs/config.dev.yaml` — 头注标"= demo profile"。
- `docs/build-and-production-validation.md` — §0.1 两版对照表 + 选择方式 · §3.2 overlay 说明。

## 关键边界(如实 · 写进 config/README/指南)

Real 版**已接线、可部署、多数资源接真源**,但:`topology`+`deploy` 仍 mock(`GetTopology`/`Deploy` 真体未建)· `real-ascend` NPU 源 `ErrNotImplemented`(npu-smi 解析器已写测、待真机接线)· OIDC/Karmada/Vault/配额强制/SLA 未实现。**全是 Phase-13**(不是 bug)。

## Verification(strict · 全 dry-run · 无需真机)

- `bash -n scripts/install.sh` ✓ · 全部新建 YAML `yaml.safe_load_all` ✓。
- `helm template` render-verify(helm v3.16):
  - demo-backend real → ConfigMap 渲染 k8s/crd/prometheus/configmap + `metrics: prometheus`/`pools: crd`/`topology: mock` + `edition: real`;demo → `replicas:1`+`edition: demo`。
  - npu-dra-driver real → `--source-type=real-ascend`;demo → `--source-type=mock-json`。
  - exporter real → 无 `--simulator`(真 DCMI)+ 保留 `huawei.com/Ascend910B` nodeSelector;demo → 无 nodeSelector(任意节点)。
  - inference-operator real → `prometheusURL` 指 in-cluster KPS。
- **render-verify 抓到并修复**:demo exporter `nodeSelector: {}` 因 helm map **合并**(非替换)清不掉 chart 默认的 Ascend910B → 改 `null` 才生效;另修一处不准注释("镜像内置 testdata" → 实为 DCMI-stub)。
- 未改 frontend/backend 代码 → vitest/go test 不受影响。

## Notes / carry

- demo-backend chart 只挂 config、不挂 mock-data fixtures → real 版 topology/deploy 的 mock fixtures 需另挂 ConfigMap(kind 做法)· 把 mock-data 卷接进 chart = 近期 follow-up(已在 real/README 标注)。
- edition 标识走 helm `podLabels`(`ocloud.edge.example.com/edition`)+ install.sh summary,**不碰 API 契约**(加 /version 字段需 RFC)。前端可见 badge = 可选后续。
- 真机功能体(real-Ascend body / 真拓扑 / OIDC / Karmada / Vault / SLA)→ **Phase 13 新 session**。
- 本地 commit · **未 push**(`feedback_push_at_phase_tag_only` · 用户控制)。
