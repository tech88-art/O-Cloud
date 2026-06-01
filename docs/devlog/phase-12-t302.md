# P12-T-302 · docs 大整理 + checkpoint-phase12 + tag phase-12-complete + CI gate

- **Commit**: (this commit + tag)
- **Date**: 2026-06-01
- **Duration**: 1d plan / ~0.5d actual(+ CI gate tail)

## Intent

per plan §6 T302 closer:Phase 12 docs 落定 + checkpoint + tag + push → CI gate。

## 交付(docs)

1. **`docs/checkpoint-phase12.md`(new)**:16/16 task table(commit hash)+ 3-track 摘要(平台/拓扑/前端)+ ADR forward notes(0019-0022 + 0001 §13 superseded)+ test posture(80 unit + 9 e2e + 11 render-verify + helm arm64 渲染)+ scope adaptations(render-verify 抓的 3 真 bug:canvas balloon / 边 handle 缺失 / 右栏 stale)+ Phase 13+ handoff(production-hardening cohort + 真 aarch64 carry + 3 carry tracks + G6 重写)。
2. **`docs/architecture.md` §13**:Phase 12 row `in flight` → **`landed phase-12-complete`** + Outcome 摘要 + checkpoint ref。
3. **`README.md`**:当前阶段 → Phase 12 complete(承 Phase 11)· 上一阶段 → Phase 11 · 下一阶段 → **Phase 13+**(原 M6 production-hardening 顺延)· **§3 硬件改写**(910B amd64 / 不支持鲲鹏 → aarch64 鲲鹏 + 昇腾 910B + openEuler · per ADR-0020 · 修正与 ADR-0020 矛盾的 stale 断言)· §4.4 演示(5 页 → one-page workspace)· §6 frontend(5 页 → one-page)· §7 路线图加 M5/M6/M7+。
4. **`docs/phase12-candidate-streams.md`**:Phase 12 spine 落定 stamp(选定 spine + production-hardening 顺延 Phase 13+ + 3 carry tracks 6th/4th/3rd + 新增 Phase 13+ carry)。
5. **`docs/screenshots/phase12/README.md`**:after-* 段(11 截图 → before 对照表 + DATA 对比)· T301 deferred 项补。

## Push 协议 + CI gate(memory `feedback_push_at_phase_tag_only` + `feedback_post_tag_ci_gate`)

- 全 phase(T001-T302 + checkpoint)**累积本地** · **不每 task push** · phase tag 落定时**一次 push** 触发 CI gate。
- `git tag phase-12-complete <T302 commit>` → `git push origin dev --tags`(creds Windows cmgr · 直接通)。
- **CI gate**:push 后看 GitHub Actions 全 workflow · 修所有 ❌ 直到 **dev HEAD 全绿**。本 phase 新增 `cross-compile-arm64` matrix(T102 · 10 模块)首次 CI 跑 —— 已 T301 helm render + 本机交叉编译实证 · CI 首跑留意。修复链 P12-fix-NNN(若需)。

## Verification

- docs 一致性:checkpoint 16 task ↔ git log commit hash 对齐 · arch §13 row landed · README 硬件/阶段/演示全改 · i18n 0 orphan(T205 实证)· e2e 9/9 + build ✓(T301/T205 实证)。
- tag + push + CI gate:见本 devlog 末 CI gate 段(post-push 追记)。

## §0a.11 compliance

- closer task · strict verify(docs 一致性核 · 各前置 task 已 strict-verify)· **push at phase tag**(唯一 push 点)· CI gate 直到 dev 全绿。
- 无 subagent · 无共享契约改动(契约在 T003 已定)。
- Phase 12 完 · M6 兑现 · 下一 phase(13+ production-hardening)另开 plan session。
