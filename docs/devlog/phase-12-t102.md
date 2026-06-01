# P12-T-102 · CI arm64 交叉编译矩阵

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 0.5-1d plan / ~0.4d actual(ci.yml 1 job + detect-changes filter · 无新逻辑)

## Intent

per ADR-0020 + plan T102,在 CI 追加 arm64 交叉编译校验(**追加 · 不替换** amd64)· 覆盖全 10 个 image Go 模块 · 确保 Track A 平台真实化后 arm64 部署 target 持续可构建(回归防护)。

## Path adaptations(plan literal vs codebase reality)

1. **用专用 matrix job 而非往各 job 加 step**(plan "backend job ... step 加入 **或 matrix arch**" 的 matrix 选项):现有 CI Go job 分散且不全 —— `backend`(matrix lint/test/build via make)· `operators`(仅 pool)· `ims-scaffold`(3 IMS matrix)· `o2-dms-adapter`(独立)· **npu-dra-driver / inference-operator / scheduler-plugin / exporter 无任何 build job**。往各 job 塞 arm64 step 会:① 漏掉 4 个无 job 的模块 ② backend matrix 3 task 各跑 1 次 arm64(冗余)③ 改动碰已调优的既有 job(go-version-file pin / make target)风险高。**专用 `cross-compile-arm64` matrix job(10 模块)additive · 零碰既有 job · 全覆盖** —— 选此(plan "matrix arch" 选项)。
2. **加 `exporters` detect-changes filter**:exporter 此前无 CI job + 无 detect-changes filter(gating 盲区)· 加 `exporters: exporters/**` output + filter · cross-compile gate 含 exporters · 修正盲区(单一真实源 "exporter changed")。
3. **build-images workflow = e2e-kind.yml · 但 kind 集群 amd64 · arm64 buildx 不适用**:`e2e-kind.yml` 有 `install.sh build-images`(docker build host amd64 + `kind load`)· kind 节点 amd64 · 加载 arm64 镜像到 amd64 kind 集群无意义(无法跑 arm64 pod)· **buildx 多架构镜像 push 留 Track A 真 CI/CD pipeline(Phase 13+ · 与真 aarch64 集群验证同期)**(per plan T102 acceptance + ADR-0020 §4 (a))。e2e-kind 不动。

## Key decisions

- **追加不替换**:amd64 校验仍由 backend/operators/ims-scaffold/o2-dms-adapter job 跑 · 新 job 只加 arm64 · 双架构都校验(ADR-0020 multi-arch · 非 arm64-only)
- **matrix fail-fast: false**:10 模块独立报 · 一个 arm64 fail 不掩盖其余(便于定位 arch-specific 问题)
- **gate = backend||operators||exporters||deploy**:任一相关代码改动触发 · deploy 含 `.github/**`(CI 自改时自验)
- **CGO_ENABLED=0 GOOS=linux GOARCH=arm64**:与 Dockerfile build stage(`GOARCH=${TARGETARCH}` · buildx 注入 arm64)同条件 · CI 交叉编译 = Dockerfile arm64 layer 构建的等价前置校验(daemon-free · 比 buildx 快)
- **ci-pass 加 cross-compile-arm64 needs**:skipped(gate false)→ result 'skipped' ≠ 'failure' → ci-pass 仍过(与既有 gated job 同 pattern · `contains(needs.*.result,'failure')` 不因 skip 失败)

## Verification

P3 三项验证:

- **存在性**:ci.yml `cross-compile-arm64` job + `exporters` filter + ci-pass needs 落地
- **完整性**:plan T102 acceptance 核 — ① backend + 全模块 arm64 cross step 加入(matrix 10 模块 · 含 backend)✓ ② operators/ims-scaffold/o2-dms-adapter/exporter 同覆盖(matrix 含全部)✓ ③ syntactic 通(下方)✓ ④ 无 build-images arm64 → devlog 注 Phase 13+(上方 Path adaptation 3)✓
- **正确性**:
  - `python -c "yaml.safe_load(ci.yml)"` → **parses OK** · jobs 列表含 `cross-compile-arm64`(12 jobs 总)· 既有 11 job 名不变(additive 未破)
  - 10 matrix 模块 `go.mod + go.sum` 全 present(setup-go cache-dependency-path 有效)
  - **job 的 `GOARCH=arm64 go build ./...` 逻辑已实证**:T101 verification 本机跑全 10 模块 `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...` 全 OK(含 scheduler-plugin k8s.io/kubernetes tree)→ CI job 必通(同命令同模块)
  - 既有 job(backend/frontend/operators/...)YAML 未改 · 仅 detect-changes outputs/filters +2 行 + 新 job + ci-pass needs +1 行(diff 局部 · 不破现有)

## Carry-forward

- **P12-T-103 helm**:`nodeAffinity kubernetes.io/arch=arm64` · helm-lint.yml(独立 workflow)校验 chart 渲染
- **P12-T-301 e2e**:kind smoke 仍 amd64(kind 节点 amd64)· arm64 真集群 smoke 留 Phase 13+
- **buildx 多架构镜像 push CI**(Track A 后续 · Phase 13+):真 CI/CD pipeline 加 `docker buildx build --push --platform linux/amd64,linux/arm64` → registry · 与真 aarch64 集群验证同期(ADR-0020 §4 (a))· 需 registry secret + QEMU setup-buildx-action
- **arm64 runner**(可选优化):GitHub 现有 arm64 runner · 后续可加 native arm64 `go test`(非仅 cross-compile build)· 现 amd64 runner cross-compile 够(无 CGO)

## §0a.11 compliance

- §0a.11:main agent 直接做(deploy `.github/` · 单 workflow 改 · 无 subagent)· strict verify = YAML parse + go.mod/go.sum 核 + arm64 build 逻辑 T101 已实证
- rhythm(v2):commit 后续 T103(Track A 末)· 不停 · 不 push(累积到 phase-12-complete tag)
