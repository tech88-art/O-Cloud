# ADR-0020: aarch64 鲲鹏(Kunpeng 920)+ openEuler 真实目标平台(supersede ADR-0001 §13 amd64-only)

- **状态**:Accepted(Phase 12 entry · per ADR-0019 §2 Decision A Track A · Phase 12 plan P12-T-002 · 2026-06-01)
- **日期**:2026-06-01
- **决策者**:协调者(用户 · 2026-06-01 chat entry meeting 决策 1 真实化目标平台)
- **相关**:**ADR-0001 §13 "不支持 ARM / 鲲鹏"(本 ADR SUPERSEDES · §13 status flip 至 "SUPERSEDED by ADR-0020")** / ADR-0019 §2 Decision A Track A 平台真实化(本 ADR 是该 track 的 platform target codification)+ §2 Decision C Track A 6th carry(本 ADR §2 Decision A 构建 target 反转 ≠ 真硬件运行验证 lab carry)/ `docs/cann-driver-matrix.md`(本 ADR §2 Decision C CANN aarch64 行落点 · 已含 openEuler 22.03/24.03 兼容标注)/ ADR-0011 §3 lab gating(本 ADR §4 真硬件验证 lab-gated 沿用)/ `docs/phase12-plan.md` §1 Track A + §8 render-verify(平台化与渲染验证解耦)

---

## §1 Context

### §1.1 ADR-0001 §13 amd64-only 是 Phase 0 简化决策

ADR-0001 §13 "不支持 ARM / 鲲鹏"(Phase 0 · 2025)记:
- **原因**:用户明确仅支持 amd64 / x86_64 + 昇腾 910B
- **影响**:所有镜像构建仅生成 amd64 layer;mock 数据 `arch: amd64` 硬约束

这是 Phase 0 "4 周内交付可演示原型" 时的**简化决策** —— 当时聚焦演示样机最短路径,平台维度暂收敛到单架构降低构建/CI 复杂度。

### §1.2 用户 2026-06-01 翻转为真实目标平台

用户 2026-06-01 chat Phase 12 entry meeting 决策 1:把 amd64-only 简化决策**翻转为真实目标平台 aarch64 鲲鹏(Kunpeng)+ openEuler**。

**rationale = 华为 Atlas 800 推理服务器原生配置**:
- **Host CPU**:Kunpeng 920(鲲鹏 920 · aarch64 / arm64 架构)
- **NPU**:昇腾 910B(不变 · 仍是 NPU target)
- **节点 OS**:openEuler(华为主导的服务器 Linux 发行版 · Atlas 系列原生适配)

让样机贴近**真实部署形态** —— 真实昇腾推理集群多为 Kunpeng host + openEuler,而非 x86 + Ubuntu。Phase 0 的 amd64 简化在样机走向真实化的 Phase 12 不再适配。

### §1.3 平台真实化 ≠ 真硬件运行验证(关键边界 · per ADR-0019 §2 Decision C)

本 ADR codify 的是**构建 / 部署 target** 反转(amd64-only → arm64/openEuler):
- 交叉编译(`GOARCH=arm64 go build`)+ buildx 双架构镜像 + helm template arch 亲和渲染校验 —— **全可在 amd64 开发机完成**(不需真 arm64 硬件)
- **≠** 真鲲鹏 920 + 昇腾 910B 集群运行验证(`helm install` + Pod Ready + 真 NPU 拓扑)—— 那是 ADR-0019 §2 Decision C Track A **6th carry · lab-gated** · 留 Phase 13+

### §1.4 本 ADR 不涉及

- 各 cascade task 详 Allowed Paths / Acceptance — `docs/phase12-plan.md` §4 P12-T-101/102/103/105 承载(本 ADR §3 仅 enumerate cascade scope)
- 拓扑数据全保真 / one-page UI — 拆 ADR-0021 / ADR-0022(Track B / C · 与平台维度正交)
- lab gating policy core — ADR-0011 §3 不变(本 ADR §4 真硬件验证沿用 lab-gated posture)

---

## §2 Decision

### §2.1 Decision A:目标平台 = linux/arm64(aarch64 Kunpeng 920)+ openEuler 节点 OS

- **CPU 架构**:aarch64 / arm64(Kunpeng 920)· 替换 ADR-0001 §13 amd64-only
- **NPU**:昇腾 910B(不变)
- **节点 OS**:openEuler(替换 install.sh / single-node 的 Ubuntu 22.04 假设 · mock `os: openEuler`)
- **mock 数据**:`arch: arm64` + `os: openEuler`(替换 `arch: amd64` 硬约束 · T105)

**双轨保留 amd64 for dev/CI/render-verify**(非 arm64-only · per plan §8):
- arm64 = **部署 target**(真实生产形态)
- amd64 = **保留**本机 dev / CI / `make dev-up` 渲染验证(开发机多为 amd64 Windows/Mac/Linux · 前端浏览器渲染 + 后端 mock 数据与开发机 CPU 架构无关)
- → 镜像做 **multi-arch**(amd64+arm64)而非 arm64-only(§2 Decision B)· 保证 `make dev-up` 本机 amd64 镜像照常构建运行 · 渲染验证不被平台真实化破坏

### §2.2 Decision B:容器镜像 = multi-arch buildx(linux/amd64,linux/arm64)· 无需换 base

- **构建方式**:`docker buildx build --platform linux/amd64,linux/arm64`(双架构 manifest list)
- **GOARCH 注入**:build stage 加 `ARG TARGETARCH` + `GOARCH=${TARGETARCH}`(buildx 自动注入 per-platform TARGETARCH)
- **CGO**:`CGO_ENABLED=0` 已是静态二进制跨编译(各 Go 模块现状)· 无 cgo 依赖 → arm64 交叉编译无需 C 工具链
- **base 镜像**:假设各 Dockerfile 现用 base(distroless/static · golang-alpine 等官方镜像)已是 multi-arch manifest(amd64+arm64 均有 layer)→ **无需换 base** · **T101 起手核查各 Dockerfile 实际 base 的 arm64 manifest 存在性**(P3 verify-before-claim · 若某 base 无 arm64 manifest 则 T101 文档化偏离)
- 默认 `docker build`(无 --platform)= host amd64 单架构(本机 dev 不变)

### §2.3 Decision C:CANN driver = aarch64 包(cann-driver-matrix 加 aarch64 行)

- 昇腾 NPU 在 aarch64 host 上的 CANN toolkit / driver 包是 **aarch64 变体**(`*_aarch64.run` · 区别于 x86_64 包)
- `docs/cann-driver-matrix.md` 加 aarch64 行(已含 openEuler 22.03 / 24.03 兼容列 · T103 补 aarch64 包标注)
- driver / firmware 与 NPU 耦合不变(NPU-Exporter ≥24.x 要求等)· 仅 host 架构包变 aarch64

### §2.4 Decision D:helm 调度 = nodeAffinity kubernetes.io/arch=arm64 · 节点 OS openEuler

- **helm charts**:各 chart deployment 加 `nodeAffinity`(或 `nodeSelector`)`kubernetes.io/arch: arm64`(K8s well-known label · 调度到 arm64 节点)· T103 横向扫全 chart 一致
- **install.sh / single-node**:Ubuntu 22.04 → openEuler · 包管理 `apt` → `dnf/yum` · arch 检测(T103)
- **mock `os`**:set-* fixtures `os: openEuler`(T105)

---

## §3 Cascade scope(per ADR-0019 §2 Decision D · 触发 Track A execution)

| task | cascade | Allowed Paths(详 plan §4) |
|---|---|---|
| **T101** | 9 Dockerfile multi-arch(`ARG TARGETARCH` + GOARCH + buildx 注释) | backend + 8 operator + exporter Dockerfile |
| **T102** | CI arm64 交叉编译矩阵(`GOARCH=arm64 go build ./...` 追加 · 不替换 amd64)+ build-images buildx(若 workflow 存在) | `.github/workflows/*.yml` |
| **T103** | CANN matrix aarch64 行 + helm `nodeAffinity arch=arm64` + install.sh/single-node openEuler | `docs/cann-driver-matrix.md` + `deploy/helm-charts/*` + `scripts/install.sh` + `deploy/single-node/` |
| **T105** | mock set-a/b/c `arch: arm64` + `os: openEuler`(替换 amd64 · 横向扫)+ schema.json | `configs/mock-data/*` |

**横向扫(P4)**:T101-T105 各自 acceptance 含 `grep` 余项核查(Dockerfile `GOARCH=amd64`/`--platform linux/amd64` 单架构 · mock `amd64` 余 0 · deploy `ubuntu/apt` 余项 openEuler 化)。docs/CLAUDE.md amd64-only 硬约束横向扫由本 task(T002)处理(ADR-0001 §13 supersede + CLAUDE.md §2/§5 + arch §9/§14.2 + research doc 历史前提标注)。

---

## §4 Open questions

### (a) 真硬件验证 posture(lab-gated)

真鲲鹏 920 + 昇腾 910B 集群运行验证(`helm install` + Pod Ready + 真 NPU 拓扑)posture:
- Phase 12 仅交叉编译 + buildx 双架构 + helm template arch 亲和渲染校验(default · per ADR-0019 §2 Decision C Track A 6th carry + ADR-0011 §3 lab gating)
- 真硬件 stamp 留 Phase 13+(若 lab signal materialise → 加真硬件 smoke · §2 Decision C 唯一 conditional)

**当前倾向**:Phase 12 交叉编译 · 真硬件留 Phase 13+。

### (b) amd64 是否长期保留双架构 fallback

- Phase 12 multi-arch(amd64+arm64)· amd64 服务 dev/CI/render-verify(§2 Decision A 双轨)
- 后续若真实部署稳定在 arm64 · amd64 layer 是否 deprecate(减镜像体积 / 构建时间)留 Phase 13+ 决定

**当前倾向**:multi-arch 长期保留(amd64 dev 便利价值 > 双 layer 构建开销 · render-verify 依赖 amd64 本机镜像)· 不在本 ADR deprecate amd64。

### (c) openEuler base 镜像是否后续引入

- Phase 12 容器 base 仍用官方 multi-arch base(distroless/static · golang-alpine · §2 Decision B 无需换 base)· 节点 OS = openEuler(host 层 · 非容器 base)
- 后续若需容器内 openEuler userland(CANN runtime 依赖 openEuler glibc / 特定库)→ 引入 openEuler base 镜像 · 留 Phase 13+ 真硬件验证时定夺(与 Ascend Docker Runtime 节点初始化耦合)

**当前倾向**:Phase 12 不引入 openEuler base 镜像(节点 OS openEuler ≠ 容器 base openEuler · 二者解耦)· 留真硬件验证时评估。

---

## §5 引用

### 上游(本 ADR 决策依据)

- **ADR-0001 §13 "不支持 ARM / 鲲鹏"**(本 ADR SUPERSEDES · §13 status flip)
- ADR-0019 §2 Decision A Track A 平台真实化(本 ADR 是 platform target codification)+ §2 Decision C(平台化 ≠ 真硬件验证边界)
- `docs/cann-driver-matrix.md`(§2 Decision C aarch64 行落点)
- ADR-0011 §3 lab gating(§4 (a) 真硬件验证沿用 lab-gated)
- `docs/phase12-plan.md` §1 Track A + §8 render-verify(平台化与渲染验证解耦 · multi-arch 保本机 amd64 渲染)

### 下游(本 ADR 触发 Track A execution)

- P12-T-101 9 Dockerfile multi-arch(§2 Decision B · base arm64 manifest 核查)
- P12-T-102 CI arm64 交叉编译(§3 · GOARCH=arm64 追加)
- P12-T-103 CANN aarch64 + helm arch 亲和 + install.sh openEuler(§2 Decision C+D)
- P12-T-105 mock arch/os + schema(§2 Decision A · arch arm64 + os openEuler)
- Phase 13+ 真鲲鹏 + 昇腾集群运行验证(§4 (a) · Track A lab carry closer)

### 受本 ADR 影响的 docs(T002 横向扫 P4)

- `docs/adr/0001-phase0-key-decisions.md` §13 status → SUPERSEDED + 上下文 line 12 historical 标注
- `CLAUDE.md`(root)§2 项目快照 目标硬件改写 + §5 重大决策加平台 target bullet
- `docs/architecture.md` §9 部署形态 forward note + §14.2 "是否需要支持 ARM 节点" open question RESOLVED
- `docs/research/ascend-device-plugin.md` HAMi "amd64 only" 历史前提标注(平台翻转后前提失效 · 自研动态切分 ADR-0011 不依赖 HAMi)
- `README.md` 目标硬件 line — 留 P12-T-302 docs 大整理(per plan T302 Allowed Paths)

---

**END of ADR-0020**
