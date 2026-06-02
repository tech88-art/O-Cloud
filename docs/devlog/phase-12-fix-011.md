# P12-fix-011 · 下钻 NPU 分层视图(NPU 卡片 + 切片网格)

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-02
- **Trigger**: 人工检查反馈(第 2 轮)#3 —— "站点和切片显示太简洁,910B 和 NPU 切片要能明显区分,建议分层显示,渲染你来设计,目标简洁明了排布整齐"。

## verify-before-design(probe 看清真问题)

下钻 NPU(`worker-site-a-01-npu-2`)的实际渲染:NPU 只是一个孤零零绿圆点在顶部,4 条边向下散开,但**切片被 fix-008 的 `rowSpread(slices, bottomY+340)` 推到 y≈750 屏幕外**(看不见)→ 一个点 + 散向虚空的边。这就是"太简洁/没分层"的根因。

## 设计 + 改动(`TopologyGraph.tsx`)

下钻 NPU 时(`focusedNodeId` 指向 npu)走专门的 **2 层 detail 布局**:
- **NPU = 带标签卡片**(`NpuDotNode` 新增 `asAnchor` 分支):cyan 边框卡(`NPU_CARD_W×H` 184×58),内容 = 状态点 + `Ascend910B#2` + 副标题 `NPU · Ascend 910B`。非 anchor 仍是原圆点。`sizeFor`/`dataFor` 按 `npuAnchorId` 给卡片尺寸 + `asAnchor` 标志。
- **切片 = 正下方整齐网格**:layout 末尾覆盖 `rowSpread`,把该 NPU 的切片排成 ≤`SLICE_GRID_COLS`(4)/行的网格,居中于 NPU 卡正下方(`gy=WORKER_Y+NPU_CARD_H+70`)。切片仍用 TopoNode(cyan + allocated/available 状态标签)。
- NPU→slice 的 `contains` 边(短竖边)从卡片扇出到网格 → 明确父子两层。

效果:910B(卡片)与切片(cells)形状/层级清晰可分,排布整齐。

## Verification(strict · per-task)

- typecheck ✓ · lint ✓ · vitest **84 passed**(无回归)· e2e **10/10**(含 dbl-click NPU→切片 + 下钻+面包屑)。
- **render-verify**(probe):下钻 npu-2 → NPU 渲染宽度 **195**(卡片,非 28 圆点)· 4 切片**全部位于 NPU 下方**(4/4)。证据 `docs/screenshots/phase12/after-npu-drilled-layered.png`(NPU cyan 卡 + 4 切片卡 allocated/available 一行 + 扇形短边)。

## Notes / carry

- 切片标签较长(`worker-site-a-01-npu-2-slice-0`)在卡内省略;如需更短显示(仅 `slice-0`)后续可加。
- 仅处理"下钻单 NPU"态(唯一会显示切片的场景);site/worker 态切片本就隐藏,`rowSpread(slices)` 对它们 no-op。
- 人工检查第 2 轮收官(#1 fix-009 · #2 fix-010 · #3 fix-011)· 本地 commit · **未 push**。
