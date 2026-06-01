# P12-T-101 · 10 Dockerfile multi-arch(aarch64 鲲鹏 + 昇腾 910B 部署 target)

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 1d plan / ~0.7d actual(10 Dockerfile + exporters/CLAUDE.md cascade + arm64 交叉编译验证)

## Intent

per ADR-0020 §2 Decision B,把全部容器镜像构建从 amd64-only/单架构改为 **multi-arch buildx**(`linux/amd64,linux/arm64`)· arm64 = 部署 target(aarch64 鲲鹏 host)· amd64 保留本机 dev/CI/render-verify(`make dev-up` 不破)。build stage `GOARCH=${TARGETARCH}`(buildx 注入)· CGO_ENABLED=0 静态跨编译 · 无需换 base。

## Path adaptations(plan literal vs codebase reality)

1. **10 Dockerfile 而非 plan 标题的 "9"**:plan T101 标题 "9 Dockerfile" · 但 Allowed Paths 列 backend + 8 operator + exporter = **10**(实测 glob 10 个 · 全有 Dockerfile · 无 scaffold-missing)。按 Allowed Paths(10)处理 · "9" 是标题估算偏差。
2. **7 operator 已 Kubebuilder multi-arch 就绪**:pool / npu-dra-driver / inference / scheduler-plugin / node-lifecycle / software-mgmt / bare-metal 已有 `ARG TARGETOS/TARGETARCH` + `GOARCH=${TARGETARCH}`(Kubebuilder 默认 Dockerfile)· 功能上已可 buildx 跨编译 · 仅需补 buildx 注释 + 改 stale "amd64 only" 注释。
3. **backend + o2-dms-adapter 缺 TARGETARCH**:build line 无 GOARCH(host 默认)· 补 `ARG TARGETOS/TARGETARCH` + `GOARCH=${TARGETARCH}`(Kubebuilder 同 pattern)。
4. **exporter 硬 pin linux/amd64**:`FROM --platform=linux/amd64`(build + runtime 两处)+ `GOARCH=amd64` 硬编码 → 全移除 · 改 `GOARCH=${TARGETARCH}` + ARG。
5. **exporters/CLAUDE.md §3.3/§7 P4 cascade(超 T101 literal Allowed Paths · 同模块)**:§7 禁止行为表显式 forbids "跨平台二进制混合发布(ARM + amd64)· 仅 linux/amd64" —— 与 exporter Dockerfile multi-arch 改动 + ADR-0020 **直接矛盾**(且 exporter 必须跑在 aarch64 NPU 节点上)。P4 单一真实源:同模块 module CLAUDE.md 的平台 policy 必须随 Dockerfile 改 · 否则模块规则自相矛盾。改 §3.3 目标二进制(amd64-only → multi-arch)+ §7 禁止行(仅 amd64 → multi-arch buildx)+ 目录树注释。documented extension(同 exporters 模块 · ADR-0020 landed cascade)。
6. **stale "arch §1.2: no arm64" 注释**:4 个 operator Dockerfile(npu-dra-driver / scheduler-plugin / inference / node-lifecycle)注释引 "arch §1.2: no arm64 / 鲲鹏 support" —— arch §1.2 实为 "两大能力底座" 非平台 · 且 amd64-only 已 ADR-0020 翻转 · 全改 multi-arch 注释。
7. **Makefile multi-arch 不在 T101 scope**:exporter Makefile `make build`/`make docker-build` 仍产 host arch(本机 dev)· buildx Makefile target 留 Track A 后续(T101 仅 Dockerfile · Makefile 不在 Allowed Paths · exporters/CLAUDE.md §3.3 已注此)。

## Key decisions

- **multi-arch 而非 arm64-only**(ADR-0020 §2.A):buildx 双 manifest · 默认 `docker build`(无 --platform)= host amd64 → `make dev-up` 本机栈不破(plan §8 渲染验证与 CPU 架构解耦)
- **不加 `--platform=$BUILDPLATFORM` 到 builder FROM**:沿用 7 operator 既有 Kubebuilder pattern(`ARG TARGETARCH` + `GOARCH=${TARGETARCH}` · 无 BUILDPLATFORM pin)· 此 pattern buildx 下经 emulation 跨编译(慢但正确)· 加 BUILDPLATFORM 是 native 优化但改既有 working pattern + plain docker build 兼容风险 → M4 不 over-engineer · 保一致(P4 单一 pattern)。native 优化留后续 if CI 构建慢 materialise
- **无需换 base**(ADR-0020 §2.B · P3 verify):base = `golang:1.24/1.25/1.26(-alpine)` + `gcr.io/distroless/static:nonroot` —— 均官方 multi-arch manifest(amd64+arm64 layer)· 无需换。**docker daemon 本机未运行**(实测 `docker version` daemon 连接失败)→ buildx 无法本地跑 → 实际 base arm64 manifest 由 CI/手动 buildx enforce(buildx 缺 arm64 layer 即 fail)· 本 task 用 **go 交叉编译** 作 arm64 兼容性主证(下方 Verification · 证 Go 代码无 CGO/arch-specific block)
- **`GOARCH=${TARGETARCH:-amd64}` vs `${TARGETARCH}` 默认**:7 operator 用 `:-amd64`(plain build 默认 amd64)· backend/o2/exporter 我用 `${TARGETARCH}`(空=host arch · pool-operator 同)· 两者 plain build 都产 host/amd64 · buildx 下都用注入 TARGETARCH · 均正确(不强求统一默认形式 · 保各自既有风格)

## Verification

P3 三项验证维度:

- **存在性**:10 Dockerfile + exporters/CLAUDE.md edits 落地 · 全有 buildx multi-arch 注释
- **完整性 · 横向扫 P4**:`grep -rin "amd64 only|no arm64|GOARCH=amd64|--platform=linux/amd64|不支持.*ARM" --include=Dockerfile` 余项**全部**是 ① buildx 命令(`--platform linux/amd64,linux/arm64` 双架构)或 ② "翻转此前 amd64-only" 标注 —— **无 unannotated 硬 amd64 pin / no-arm64 断言**。exporter 2 处 `--platform=linux/amd64` 硬 pin + `GOARCH=amd64` 全移除(grep-verified)。
- **正确性 · arm64 交叉编译实证**(`CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...` 各模块):
  ```
  backend                                    arm64 OK
  exporters/ascend-npu-exporter-plus         arm64 OK
  operators/pool-operator                    arm64 OK
  operators/npu-dra-driver                   arm64 OK
  operators/inference-operator               arm64 OK
  operators/o2-dms-adapter                   arm64 OK
  operators/node-lifecycle-operator          arm64 OK
  operators/software-mgmt-operator           arm64 OK
  operators/bare-metal-provisioning-operator arm64 OK
  operators/scheduler-plugin                 arm64 OK   (k8s.io/kubernetes tree · 最重)
  ```
  10/10 image 模块 arm64 交叉编译通过 · 证 CGO_ENABLED=0 静态二进制无 arch-specific/CGO 依赖阻塞 arm64。**buildx 双架构镜像构建**(daemon 本机未起)留 CI(T102)/ 手动验证 · per ADR-0020 §2.B + plan T102 "buildx multi-arch push 留 Track A 真 CI/CD Phase 13+"。

## Carry-forward

- **P12-T-102 CI**:`GOARCH=arm64 go build ./...` 入 CI 矩阵(追加 amd64 · 不替换)· build-images workflow(若存在)加 buildx `--platform`
- **P12-T-103 helm**:`nodeAffinity kubernetes.io/arch=arm64` · 与本 task 镜像 multi-arch 配套
- **Makefile buildx target**(Track A 后续 · 非 T101):exporter / backend / operator Makefile `docker-build` 加 buildx multi-arch target(本 task 仅 Dockerfile · Makefile 不在 Allowed Paths)
- **真 buildx 双架构镜像验证**:docker daemon 本机未起 → 真 buildx 双 manifest 构建留 CI/真硬件验证(Phase 13+ · ADR-0020 §4 (a))
- **base arm64 manifest**:golang + distroless/static 官方 multi-arch(well-known)· buildx 会在缺 arm64 layer 时 fail · CI buildx 是终极验证

## §0a.11 compliance

- §0a.11:main agent 直接做(跨模块 deploy+backend+operators+exporters · 仅 Dockerfile · 单一类型改动 · 无 subagent)· strict verify = 10 模块 arm64 交叉编译 + 横向扫 grep 实证
- rhythm(v2):commit 后续 T102(Track A)· 不停 · 不 push(累积到 phase-12-complete tag)
