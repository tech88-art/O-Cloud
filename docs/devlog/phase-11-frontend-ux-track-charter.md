# Phase 11 Frontend UX Track-2 charter (P11-T-F01..F04 · parallel · conditional)

- **Date**: 2026-05-22
- **Trigger**: Phase 10 demo verify completed today (path A bringup + 3
  rounds dashboard fix P11-fix-001/002/003 all landed). User walked the
  5 pages + Grafana dashboards and flagged 3 categories of POC-vs-design-
  ideal UX gaps not covered by Phase 11 main scope (T001-T204).

## User feedback (raw)

> 1. 概览的资源树可视化图标不如手册中呈现直观
> 2. 部署图示也不如手册美观和专业
> 3. 无"深度对比信息"(D6 NUMA+HCCS)
>
> 决策原则:**技术选型从长期演进、可维护性上看哪个更优 · 如果 G6 更
> 合理 · 那选 G6**

## Decision: 立 Track-2 sub-track · 不在 ReactFlow 路径 hack

Considered 4 paths post-feedback (c0 不修 / c1 dagre tune / c2
post-process / c3 ReactFlow group nodes / c4 G6 重写). G6 vs ReactFlow
7-dimension comparison (compound graph · scalability · layout engine ·
renderer · CLAUDE.md contract · 中文生态 · sunk cost):

| 维度 | G6 5.x 优 | ReactFlow 12 优 |
|---|---|---|
| Compound graph native | ✅ | ❌ |
| 大规模(Karmada multi-site)| ✅ Canvas / WebGL | ❌ DOM 渲染 · 1000+ 卡 |
| Layout 引擎内置 | ✅ dagre/force/circular/radial/grid | ❌ hand-roll |
| `frontend/CLAUDE.md §7` mandate | ✅ "封装 G6" | ❌ 已 violate (技术债) |
| 中文生态 + 商业 license | ✅ AntV 蚂蚁系列 | ⚠ Pro features 商业 |
| 已有 sunk cost | 164 行 G6POC | 550 行 ReactFlow + 7 feature |

**6/7 维度 G6 长期更优** · 按用户原则切回 G6 · 不在 ReactFlow 上 expedient
打补丁。

## Track-2 4 tasks landed in plan

- **F01** TopologyGraph G6 5.x 重写 + compound graph + 7 feature parity ·
  5-7d · frontend · `phase11-plan.md` §5.5
- **F02** Deploy preset card 重设计(NPU dot + 动效) · 2d · frontend
- **F03** D6 NUMA+HCCS 对比专题 panel · 2-3d · frontend + backend small
- **F04** Backend `workload_*` histogram + counter emit · 3-4d · backend ·
  unblocks workload-business 全 panel + workload-resource 部分 panel

Total: **12-16d frontend + 3-4d backend** · parallel to W1-W3 main
scope · capacity-dependent.

## Why "conditional · carry to Phase 12 if not all land"

Phase 11 main scope 已 5-6 weeks + medium-high uncertainty profile (chart
packaging × 5 modules · Karmada propagation · LAB 5th attempt · etc).
Track-2 12-16d frontend 估计在 main 工期内并行 land 取决于 frontend
agent 容量。**不 over-commit** · 4 task 都立 carry posture · `phase-11-
complete` tag 后任一 task 仍可 carry to Phase 12 first slot · 不阻塞
M5 真生产化 foundation milestone 关闭。

## Plan extension (4 edits to `docs/phase11-plan.md`)

1. §2 task count `20 tasks` → `20 main + 4 Frontend UX sub-track = 24
   tasks`
2. §5 W3 closer 之后插入 §5.5 完整 Track-2 spec(4 task package · ~200
   lines including background + decision basis + ownership + per-task
   Allowed Paths / Acceptance / Dependencies / Estimated effort)
3. §6 DoD 加 Track-2 conditional checklist section(5 items · 含 carry
   posture)
4. §6 Out of scope 加 1 entry · F01-F04 任一 not landed by tag → carry

Plan 1866 → 2142 lines (+276).

## P11-fix series convention 不延用

This is **plan extension**(立项) · not a fix · commit prefix
`docs(plan):` not `fix(...)`。不计入 P11-fix-NNN 系列(那系列专门 for
post-tag CI gate fixes 或 demo bringup hotfixes)。

## Refs

- `docs/phase11-plan.md` §5.5 + §6 Track-2 DoD + §6 Out-of-scope carry
- `frontend/CLAUDE.md §7` G6 mandate
- P11-fix-001 / 002 / 003 (3 prior dev-stack bringup commits on
  `fix/p11-fix-001-dev-stack` branch · 演示 verify 路径打通)
- User feedback 2026-05-22 chat: "技术选型从长期演进、可维护性看 ·
  G6 更合理就选 G6"
