# P12-T-002 · ADR-0020 aarch64 鲲鹏 + openEuler target(supersede ADR-0001 §13)

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 1d plan / ~0.6d actual(docs-only · 含全仓库 amd64/鲲鹏 横向扫)

## Intent

把 ADR-0001 §13 "仅支持 amd64 / x86_64" 的 Phase 0 简化决策,翻转为真实目标平台 **aarch64 鲲鹏(Kunpeng 920)+ openEuler**(华为 Atlas 800 原生配置),codify 为 ADR-0020 + 横向扫(P4)所有 docs/CLAUDE.md 的 amd64-only 硬约束 supersede / 历史标注。为 Track A cascade(T101 Dockerfile / T102 CI / T103 CANN+helm+install / T105 mock)提供 platform target lock。

**关键边界(写入 ADR-0020 §1.3 + §2 Decision A)**:平台真实化 = **构建/部署 target** 反转(交叉编译 + buildx 双架构 + helm template 校验 · amd64 开发机可完成)· **≠** 真硬件运行验证(lab-gated · 6th carry · Phase 13+)。镜像做 **multi-arch(amd64+arm64)** 而非 arm64-only —— amd64 保留本机 dev/CI/render-verify(per plan §8 渲染验证与 CPU 架构解耦)。

## Path adaptations(plan literal vs codebase reality)

1. **CLAUDE.md §5 无独立 "目标硬件" 行**:plan T002 Allowed Paths 写 "§2/§5 目标硬件 改写"。实际 §2 项目快照有目标硬件行(line 52)· §5 技术栈基线是 后端/前端/K8s/NPU/监控/重大决策 子节 · **无**独立目标硬件行。解决(P4 单一真实源 · 不重复):§2 line 52 改写目标硬件(删 "不支持 ARM/鲲鹏")· §5 在 **重大决策(不可逆 记入 ADR)** 子节加 1 条 platform target bullet(ADR-0020 ref · 这是平台决策的自然归属 · 与 §2 snapshot 一行互补不重复)。
2. **arch.md 无 "目标硬件: amd64" 显式行**:amd64-only 硬约束只在 ADR-0001 §13 + CLAUDE.md · arch.md 无直接声明。plan 说 "目标硬件 / 部署形态 章节 arch 改写" → 落点 = §9 部署形态(§9.1 box + Phase 12 forward note)+ §14.2 "是否需要支持 ARM 节点" open question RESOLVED(纵向 cascade · 决策回填 prior open question)。
3. **docs/research/ascend-device-plugin.md 不在 T002 literal Allowed Paths · 但 acceptance grep `docs/` scope 命中**(line 48/73 "本项目 amd64 only")· 同 docs 模块。**P4 横向 extension**(documented · 同模块 · acceptance grep 要求 docs/ clean · 1-2 行历史标注非语义重写)· 解决 plan Allowed Paths 文件清单未枚举此 research doc 的 gap(per `feedback_strict_per_task_verify` "何时不停":单纯 plan 字面偏差但有合理本地映射 → 直接适配 + 文档化)。
4. **README.md:86-87 "不支持 ARM/鲲鹏" 留 T302**(per plan T302 Allowed Paths README 目标硬件 update · 不在 T002 acceptance grep 的 `docs/ CLAUDE.md` scope · 故 T002 不碰 · T302 docs 大整理处理)。

## Debugging trail

纯 docs · 无 build/test fail。1 个语义判断反复:

1. **research doc HAMi "amd64 only" 标注的正确性(P3 honesty · 防误导)** — line 48/73 原结论 "HAMi 仅 ARM 平台 · 本项目 amd64 only → 不可用"。ARM 翻转后此前提**反转**(HAMi 平台轴不再 block)· 若只标 "superseded" 会误导成 "HAMi 现在可用了"。实际:本项目用**自研动态切分**(ADR-0001 §7 不引入 MindCluster + ADR-0011 自研切分)· 不引入 HAMi 是**另一个理由**(自研路径)而非 amd64。修法:标注明确写 "amd64-only 前提已翻转 · 但自研动态切分不引入 HAMi · HAMi 适用性留 Phase 13+ 真硬件评估"(保留历史文本 + 删除线 + 括注)· 不下 "HAMi 可用" 伪结论。

## Key decisions

- **multi-arch 而非 arm64-only**(§2 Decision A+B):arm64=部署 target · amd64 保留本机 dev/CI/render-verify · 镜像 buildx `linux/amd64,linux/arm64` 双 manifest · 保 `make dev-up` 本机 amd64 渲染验证不破(plan §8 渲染验证与 CPU 架构解耦)
- **无需换 base 镜像**(§2 Decision B):CGO_ENABLED=0 静态跨编译 + `GOARCH=${TARGETARCH}` buildx 注入 · 假设各 Dockerfile base(distroless/static · golang-alpine)已 multi-arch manifest → **T101 起手核查实际 base arm64 manifest 存在性**(P3 verify-before-claim · 不凭印象断言 base 都 multi-arch · 写入 ADR-0020 §2.2 + §5 下游)
- **节点 OS openEuler ≠ 容器 base openEuler**(§4 (c)):Phase 12 节点 OS openEuler · 容器 base 仍官方 multi-arch base · 二者解耦 · openEuler base 镜像留真硬件验证评估
- **§13 supersede 用 banner + 保留历史原文**:不删 §13 历史文本(ADR 是 append-only audit trail)· 加 ⚠ SUPERSEDED banner + 历史标注(P3 honesty · 决策演进可追溯)

## Verification

P3 三项验证维度(docs-only · 无 compile/test):

- **存在性**:ADR-0020 + ADR-0001 2 edits + CLAUDE.md 2 edits + arch.md 2 edits + research doc 2 edits + 本 devlog 落地 · 引用的 ADR-0019/0011 + cann-driver-matrix(已含 openEuler 列 · grep-verified line 21/24)真存在
- **完整性**:plan T002 acceptance 5 项逐项核 — ① ADR §2 Decision A-D(arm64+openEuler target / multi-arch buildx / CANN aarch64 / helm arch 亲和)✓ ② ADR-0001 §13 status SUPERSEDED by ADR-0020 stamp ✓ ③ root CLAUDE.md §2 目标硬件改 aarch64 鲲鹏 + 删 "不支持 ARM/鲲鹏" + §5 重大决策加 platform bullet ✓ ④ arch 部署形态(§9)+ ARM open question(§14.2)改写 ✓ ⑤ 横向扫 grep(下方实证)✓
- **正确性 · 横向扫 P4 实证**:`grep -rin "不支持.*鲲鹏\|amd64.*only\|仅.*amd64" docs/ CLAUDE.md` → 余项(ADR-0001 §13 历史段 · research doc 历史段 · Phase 12 docs 描述翻转)**全部带 SUPERSEDED / 历史 / 翻转 标注 or 是描述 flip 本身**(无 unannotated 硬约束)。CLAUDE.md line 52 "不支持 ARM/鲲鹏" 已删(grep 余 0)。

## Carry-forward

- **P12-T-101 9 Dockerfile**:ADR-0020 §2 Decision B · **起手必核查各 Dockerfile 实际 base 的 arm64 manifest**(distroless/static · golang-alpine 是否真 multi-arch · 不凭印象)· `ARG TARGETARCH` + `GOARCH=${TARGETARCH}` + buildx 注释 · 横向扫 `--platform linux/amd64` 单架构余项
- **P12-T-102 CI**:`GOARCH=arm64 go build ./...` 追加(不替换 amd64 · 双校验)
- **P12-T-103 CANN+helm+install**:cann-driver-matrix aarch64 行(已有 openEuler 列)· helm `nodeAffinity kubernetes.io/arch=arm64` 横向扫全 chart · install.sh openEuler dnf
- **P12-T-105 mock**:set-* `arch: arm64` + `os: openEuler` 替换 amd64(横向扫 `grep amd64 configs/mock-data` 余 0)· schema.json os enum
- **P12-T-302 docs 大整理**:README.md:86-87 "昇腾 910B(amd64/x86_64)" + "不支持 ARM/鲲鹏" 改写(本 task 未碰 · 留 T302 per plan Allowed Paths)
- **Phase 13+**:真鲲鹏 920 + 昇腾 910B 集群运行验证(ADR-0020 §4 (a) · Track A lab carry)· openEuler base 镜像引入评估(§4 (c))· HAMi 适用性 re-eval(research doc 标注)

## §0a.11 compliance

- §0a.11 docs-only 例外:main agent 直接做 · 无 subagent · strict verify = acceptance 5 项 + 横向扫 grep 实证 + 各 edit 落点确认
- rhythm(per `feedback_strict_per_task_verify` v2):commit 后续 T003 · 不停(用户 "执行...从 T001 开始" mandate)· 不 push(累积到 phase-12-complete tag)
