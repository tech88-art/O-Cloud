# P12-T-103 · CANN matrix aarch64 + helm arch 亲和 + install.sh openEuler

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 1.5d plan / ~0.9d actual(9 chart affinity + install.sh OS/arch 重写 + cann matrix + single-node)

## Intent

per ADR-0020 §2 Decision C+D 落地 Track A 平台化的调度 + 安装 + 兼容矩阵层:
- cann-driver-matrix 加 aarch64 包矩阵(§1.1)+ openEuler 行
- 9 helm chart soft nodeAffinity `kubernetes.io/arch=arm64`
- install.sh Ubuntu/apt → openEuler/dnf(OS family 检测)+ arch 检测(arm64 target)
- single-node README openEuler/aarch64 目标平台

## Path adaptations(plan literal vs codebase reality)

1. **soft nodeAffinity 而非 hard nodeSelector**(关键决策):plan/ADR-0020 §2.D 说 "nodeAffinity kubernetes.io/arch=arm64"。**kind smoke(tests/e2e/kind/install.sh)`helm upgrade --install` 这些 chart 到 amd64 kind 集群**(exporter line 207 / npu-dra-driver 275 / scheduler-plugin 等 · master-demo-multi-site.sh 同)· helm-lint.yml 用 default values 渲染。**hard nodeSelector arch=arm64 会让 pod 无法调度到 amd64 kind 节点 → kind smoke 全红**。改 **soft `preferredDuringSchedulingIgnoredDuringExecution`**:① 渲出 arch=arm64(acceptance ✓)② 生产全 arm64 必匹配 ③ amd64 dev/CI/kind 仍调度(不破)· 对齐 ADR-0020 "nodeAffinity" 措辞(nodeAffinity 原生支持 soft)+ multi-arch 哲学(arm64 偏好 · amd64 dev 保留)。scheduler-plugin values 自带注释 "defaults left empty so chart works on kind/dev" 印证此选择。
2. **chart 已支持 `.Values.affinity`** → **只改 values.yaml 不改 deployment template**(8 deployment chart 模板已有 `{{- with .Values.affinity }}` block · P4 leverage 现有 · 单一真实源)。plan 说 "values.yaml + deployment.yaml" · 实际 deployment template 无需改(已渲染 affinity)· 更 minimal。
3. **exporter 是 DaemonSet 非 Deployment** · 模板渲 `.Values.nodeSelector` 但**无** `.Values.affinity` → **加 affinity render block 到 daemonset.yaml**(plan 写 deployment.yaml · 实际 exporter 用 daemonset.yaml)+ exporter values 加 affinity key。DaemonSet soft affinity = 在所有节点跑(偏好 arm64)· 不破 amd64 kind。
4. **9 chart 非 "全 chart"**:无 pool-operator chart / quota chart(未打包)· 实际 deploy/helm-charts/ 有 9 个(demo-backend + 7 operator + exporter)· 全覆盖。
5. **exporter chart README.md P4 横向 extension**(超 T103 literal Allowed Paths · 同 deploy 模块):README §Out of scope line 81 "Multi-arch images — amd64-only per ADR-0001 §7" = **stale 错误**(① amd64-only 已 ADR-0020 翻转 + T101 multi-arch ② ADR-0001 §7 是 MindCluster 非平台 · 应 §13)· acceptance grep deploy/ 命中 · 改为 "已翻转 multi-arch per ADR-0020"(documented · 同 chart 同模块)。
6. **install.sh OS family 抽象**:加 `PKG_MGR`(dnf/apt-get)+ `OS_FAMILY`(rhel/debian)+ `HOST_ARCH`/`GOARCH_DETECTED` globals(sanity_check 设 · main 先调)· docker/go/node install 按 family 分支 · arch 检测(aarch64→arm64 target · x86_64→amd64 dev)。apt-get 保留在 `OS_FAMILY=debian` 分支(dev/CI · 注释标注)· 非删除(双平台支持)。

## Key decisions

- **soft affinity weight 100 preferred**(见 Path adapt 1):amd64 dev/CI/kind 不破 + arm64 生产偏好 · acceptance 渲 arch=arm64 满足
- **install.sh 双平台**(openEuler target + Ubuntu dev):不是 openEuler-only · Ubuntu/apt 分支保留(本机 dev 渲染验证仍 Ubuntu/amd64 · plan §8 渲染验证与 CPU 架构解耦)· OS family 自动检测
- **CANN 版本兼容 arch-agnostic**(cann-matrix §1.1):driver×CANN×vllm-ascend 版本组合对 x86_64/aarch64 相同 · 仅二进制包不同(`*_aarch64.run`)· 故 §1 矩阵 verdict 两架构通用 · aarch64 行只是 host 平台标注(P3 honesty · 不虚构 arch-specific 兼容差异)
- **真 CANN aarch64 包安装验证 lab-gated**:cann-matrix §1.1 明示真鲲鹏 + 昇腾 CANN aarch64 安装 + npu-smi 验证留 Phase 13+(ADR-0020 §4 a)· Phase 12 不接触真 aarch64 CANN 二进制

## Verification

P3 三项验证:

- **存在性**:cann-matrix §1.1 + aarch64 行 · 9 chart values affinity · exporter daemonset affinity block · install.sh OS/arch logic · single-node README · exporter README 落地
- **完整性**:plan T103 acceptance 4 项核 — ① cann-matrix aarch64 行 + openEuler 兼容标注(§1.1 包矩阵)✓ ② 全 9 helm chart helm template 渲 arch=arm64 + lint --strict clean ✓ ③ install.sh openEuler dnf 路径 + bash -n 通 ✓ ④ 横向扫 deploy/ scripts/ 余项 openEuler/arm64 或注释 ✓
- **正确性 · 实证**:
  - `helm lint --strict` 9/9 chart → **"0 chart(s) failed"**
  - `helm template` 9/9 chart → 渲出 `kubernetes.io/arch ... In ... arm64`(soft nodeAffinity · grep-verified `arm64-in-affinity:1` ×9 · demo-backend 渲染样例确认结构)
  - `bash -n scripts/install.sh` → **install.sh syntax OK**
  - 横向扫 `grep -rin "ubuntu|apt-get|amd64|x86_64" deploy/ scripts/` → 余项全部 ① 新 OS-family 检测逻辑(apt-get 在 debian 分支 · 注释 "Debian/Ubuntu dev/CI")② arm64-target 框架(amd64 标 dev/CI)③ affinity 注释 —— **无 unannotated amd64-only 硬约束**(exporter README stale line 已修)

## Carry-forward

- **P12-T-105 mock**:set-* `arch: arm64` + `os: openEuler`(与本 task helm arch=arm64 + install.sh openEuler 一致)
- **P12-T-301 e2e**:kind smoke 仍 amd64(soft affinity 保 pod 调度)· 真 aarch64 集群 helm install + Pod Ready 留 Phase 13+
- **真硬件验证**(Phase 13+ · ADR-0020 §4 a):真鲲鹏 920 + 昇腾 910B + openEuler 集群 · CANN aarch64 包安装(cann-matrix §1.1 包表)· npu-smi info · helm install Pod Ready 真调度到 arm64 节点(soft → hard 可选收紧)
- **install.sh openEuler 真验证**:本 task bash -n + 逻辑分支 · 真 openEuler box `dnf install docker/nodejs` 路径留真环境验证(docker pkg 在 openEuler repo 名可能需调 · 已加 moby-engine fallback + warn)
- **docker pkg openEuler repo 名**:openEuler `dnf install docker` vs `moby-engine` 包名 · 本 task 加 fallback 链 + warn · 真 openEuler 验证时确认

## §0a.11 compliance

- §0a.11:main agent 直接做(deploy + scripts + docs · 跨子目录但单一类型 · 无 subagent)· strict verify = helm lint/template 9 chart + bash -n + 横向扫 grep 实证
- rhythm(v2):W2 Track A(T101-T103)完成 · commit 后续 T104(Track B backend)· 不停 · 不 push(累积到 phase-12-complete tag)
