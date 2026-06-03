# P12-fix-014 · e2e-kind CI flake 硬化(拉取重试 + 释放磁盘)

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-02
- **Trigger**: `e2e-kind` 连续两次 infra flake(均非产品代码):
  - 上次(run 26820630940):kind-smoke 死在 Phase 6 "build + load scheduler-plugin image",~1min 骤死、无 step 日志 → runner **磁盘/OOM**(scheduler-plugin vendor 整个 k8s.io/kubernetes,构建重)。
  - 本次(run 26858766479):step 8 `kind create` 失败 —— `docker pull kindest/node:v1.34.3` → `registry-1.docker.io ... context deadline exceeded`(**Docker Hub 拉取超时**)。
  两次都发生在任何项目代码跑之前,CI(lint/test/build)+ 前端 mock Playwright job 均 success。

## 改动(均 CI-infra · 不碰产品代码)

- **`tests/e2e/kind/install.sh`**:
  - 加 `retry <attempts> <sleep> <cmd...>` helper(set -e 安全 · cmd 在 if 条件里)。
  - `kind_create_with_retry`:`kind create` 失败 → `kind delete` 清理 + 重试(3 次 · 线性 backoff)→ 治 kindest/node 拉取超时。
  - cert-manager 5 个镜像预拉:`docker pull` 包 `retry 3 8`(治 quay.io 抖动)。
- **`.github/workflows/e2e-kind.yml`**:kind-smoke job 在 checkout 后、构建前加 "free up runner disk" 步 —— `rm -rf` 掉 dotnet/android/ghc/CodeQL/boost 预装工具链,回收 ~20-30GB → 治 Phase 6 重镜像构建的磁盘/OOM 骤死。

## Verification

- `bash -n tests/e2e/kind/install.sh` ✓。
- `.github/workflows/e2e-kind.yml` YAML 合法 ✓(free-disk 步插在 checkout 后第 2 步 · jobs 不变)。
- 真效果由本 commit 触发的新 e2e-kind run 验证(用户:commit 触发即当 re-run,不单独 rerun)。

## Notes

- 这两个 flake 是 CI 环境概率性抖动(网络/磁盘),非确定性 bug;硬化是降复发率,非"修一个必现 bug"。
- 仍可能有新的 runner 抖动;若 Docker Hub rate-limit 持续,后续可加 registry mirror / 镜像缓存(更大改动,暂不做)。
- 本地 commit · push 触发 CI gate(用户控制 push)。
