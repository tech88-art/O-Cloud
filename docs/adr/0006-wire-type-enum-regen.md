# ADR-0006: Wire-type enum regen + Pod.bindings sync

- **状态**:Accepted (2026-05-18,用户在 chat 批准 §0a.5)
- **日期**:2026-05-18
- **决策者**:协调者(用户)
- **相关**:ADR-0004(inter-node fabric)/ ADR-0005(POD 融合)/ T013 / T211 / T212 / T213 / T214 / T301 / T302 / known-issues.md #2

---

## 上下文

Phase 1 的 OpenAPI 契约(`docs/api-contract.yaml`)在三处与运行时实际 wire shape 有 drift:

1. **TopologyNode.type enum** 仅含 `[cluster, nodepool, node, npu, slice, network]`,缺:
   - `switch`(T211 / ADR-0004 inter-node fabric)
   - `workload`(T213 / ADR-0005 workload fusion)
   - `pod`(T213 / ADR-0005 workload fusion)

2. **TopologyEdge.type enum** 仅含 `[contains, hccs, network, allocated]`,缺:
   - `fabric-link`(T211 / ADR-0004)
   - `binds-to`(T213 / ADR-0005)
   - `pd-pair`(T213 / ADR-0005)

3. **Pod schema** 缺 `bindings` 字段。T013 在 `configs/mock-data/schema.json` 加了 `Pod.bindings`,fixture 也写了实际值,但 OpenAPI 契约从未同步,backend `model.Pod` 也无该字段。聚合层 `aggregator.WorkloadInput.Pod` 临时 mirror 了 schema 形状以维持 `binds-to` 边生成。

后果(累计):
- 前端 `TopologyGraph` / `Overview/index.tsx` 两处使用 runtime-only 字符串 widening cast(`type as TopologyNodeType` 等)绕过生成类型不兼容
- 前端 Workloads Drawer 看不见 `Pod.bindings` — drawer 里 "pod 绑定哪几个 slice" 信息丢失
- 任何 ADR-0004 / ADR-0005 fixture 编辑都需要同步两处 schema(OpenAPI + mock-data),容易漂移

每个 T2xx-T3xx task 都选择"作为 runtime-only 字符串发,不阻塞当前任务",留待 RFC 批量同步。本 ADR 即批量同步。

---

## 决策

一次性同步 OpenAPI 契约与运行时 wire shape,**作为 Phase 2 入口门槛**(Phase 2 的真实 datasource 直接对齐新契约,避免双重迁移)。

### 契约变更明细

**`TopologyNode.type` enum**(原 6 项 → 9 项):
```
[cluster, nodepool, node, npu, slice, network, switch, workload, pod]
```

**`TopologyEdge.type` enum**(原 4 项 → 7 项):
```
[contains, hccs, network, allocated, fabric-link, binds-to, pd-pair]
```

**`Pod` schema** 加 `bindings` 数组(每项 `{sliceId, role, indexInPod}`):
```yaml
Pod:
  properties:
    # ... existing fields ...
    bindings:
      type: array
      items:
        type: object
        properties:
          sliceId: { type: string }
          role: { type: string }          # prefill | decode | primary | sidecar | ...
          indexInPod: { type: integer }
```

字段语义与 `aggregator.WorkloadInput.PodBinding`(T013)1:1 对齐。

### 实施步骤

1. ADR-0006(本文件)
2. 更新 `docs/api-contract.yaml` 三处
3. 前端 `pnpm gen:types` 重生 `services/types.ts`
4. 前端 drop 各 widening cast(`TopologyGraph.tsx` 顶部 + `Overview/index.tsx` 的 `isGraphOnlyType`)
5. 后端 `model.Pod` 加 `Bindings []PodBinding` 字段 + `model.PodBinding` 新类型
6. 后端 `datasource/mock/workload.go` loader 检验能解出 bindings(JSON tag 一致 → 应该免改)
7. 后端 `aggregator/topology.go` `WorkloadInput.Pod` 不再需要本地 mirror,可改为引用 `model.Pod`(可选清理,本 ADR 不强制)
8. 后端 tests 不需要改(model.Pod 加字段不破现有断言)
9. 标 `known-issues.md #2` resolved

### 不做(明示)

- **不变** WSMessage.type enum — `log.line` 已在 enum 中(T302 已对齐);`metric.sample` 暂不引入,T307 narrative 用 `workload.statusChanged` payload 携带 metrics 字段
- **不删** runtime-only widening cast 的注释 — 改为 "historical: T211/T213 had to widen at runtime before ADR-0006",保留考古价值
- **不重构** aggregator 的 `WorkloadInput.Pod` mirror — Phase 2 的真实数据源会重写这块,这里不为重构而重构(Karpathy "Simplicity First")

---

## 后果

### 正面
- 前端 / 后端 / mock-data schema / OpenAPI 契约一致
- Phase 2 真实 K8s source 直接对齐契约,无遗留迁移
- 类型安全:前端的 `n.type === 'workload'` 不再需要 `as string` cast
- Drawer 可呈现 pod 绑定的 slice 列表(spec F2 完整覆盖)

### 负面 / 风险
- 任何 cached / persisted 旧契约客户端可能见到不识别的 enum 值(Phase 1 无 cached 客户端,Phase 2+ 客户端读新契约)
- `Pod.bindings` 加进契约后,Phase 2 真实 K8s source 必须提供该字段(否则 frontend 渲染降级,加 placeholder)

## 推翻条件

如 Phase 2 真实 K8s source 实测发现 `bindings` 从 scheduler annotation / device-plugin 提取不来,可能改为后端从 NPU device-plugin 反查并填充(已在 ADR-0005 §"Phase 2" 提及)。Enum 值无回退路径 — 加上后即为终态。
