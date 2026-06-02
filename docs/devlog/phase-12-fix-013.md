# P12-fix-013 · 显示优化:MIG 式切片分区 + 工作负载列头/PD 标

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-02
- **Trigger**: 人工检查反馈(第 3 轮)#2 —— "910B 和 NPU 切片显示就一行字+状态图,呈现不够直观;搜索业界优秀实践,优化显示"。用户选 **"两者都优化"**(NPU/切片下钻视图 + 底部工作负载/Pod 列)。

## 业界实践调研(WebSearch)

- **NPU 切片 ≈ NVIDIA MIG**(GPU 切分):业界用**按算力/显存占比分段的分区条**呈现,每段=一个实例(profile + size + 占用方),nvidia-smi 用 GI/CI 表列 显存+SM 分配。
- **Node→GPU→Pod**(Grafana/DCGM/Lens):显式映射 节点→加速器→Pod、按归属分组、状态着色、Pending/Failed 高亮、运行/待定一目了然。
- 数据核查:切片含 `template`(vir02)+ `aiCore`(8)+ `vramMiB`(16384)+ `allocatedTo.podName`;4×vir02 正好 = 32 核/64GB 满分 → 完美 MIG 素材。

## 改动(`TopologyGraph.tsx` + i18n)

**#2a · MIG 式切片分区(下钻 NPU)**
- 新 `SliceCellNode`(node type `slicecell`):每切片 = 分区 cell,显示 `slice-N` + 已分配/空闲徽章 + `template · N 核 · GB` + 占用 pod(▸ name)。已分配=cyan 实底/边,空闲=灰底/边。`rendererTypeFor` slice→slicecell,`dataFor` slice 分支取 template/aiCore/vramMiB/allocatedTo,`sizeFor` 给 cell 尺寸。
- layout:`npuAnchor` 块把切片排成**紧凑分区条**(SLICE_CELL_W 156 + gap 10)居中于 NPU 卡正下方;flowEdges **丢弃 NPU→slice contains 边**(分区条直接在卡下,无需连线,MIG 风格)。

**#2b · 工作负载列更直观**
- **列头 caption**:新 `CaptionNode`(synthetic 非交互节点),每个有内容的底部列上方加标签(worker 名 / 工作负载 / 未调度)。
- **PD 角色标**:`WorkloadDotNode` 对 prefill/decode pod 显示小 `P`(蓝)/`D`(紫)徽章 → 一眼看出 PD 分离。
- i18n:`topology.slice.{core,allocated,free}` · `topology.{workloadsGroup,unscheduledGroup}`。

## Verification(strict · per-task)

- typecheck ✓ · lint ✓ · vitest **84 passed**(无回归)· e2e **10/10**(含 dbl-click NPU→切片 + 下钻+面包屑)。
- **render-verify**:
  - #2a:下钻 npu-2 → 4 个切片 cell `slice-0 已分配 vir02·8核·16GB ▸pod-vir02-0` / `slice-1 空闲 … —` 排成分区条(`after-mig-slices.png`)。
  - #2b:workloads ON → 5 列头(worker-site-a-01/02/03 / 工作负载 / 未调度)+ PD 标 P=3/D=3(`after-wl-headers.png`)。

## Notes / carry

- MIG 切片当前等宽(vir02 均分);若将来有不等 profile,可按 aiCore 占比改成不等宽段。
- 本地 commit · **未 push**。
