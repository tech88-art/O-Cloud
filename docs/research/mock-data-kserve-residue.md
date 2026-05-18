# 已知设计漂移:mock-data 残留 KServe InferenceService

> **类型**:已知漂移记录(Deferred Cleanup,**不立即触发修改**)
> **日期**:2026-05-17
> **触发**:ADR-0002 实施后,P4 横向扫描漏检发现 mock-data 残留
> **关联**:ADR-0002 / `spec/OR-requirements.md` / `CLAUDE.md` §4(共享契约清单)+ §8(RFC 流程)
> **状态**:Deferred — Phase 1 W4 验收完成、所有 W2-W3 开发 agent 收尾后,由协调者单独派单 configs agent 执行
>
> ## ⚠ 执行时机约束
>
> **当前 Phase 1 W2-W3 期间,开发 agent 正在并行执行 P1-T-2xx / P1-T-3xx 任务包,涉及 `configs/mock-data/`、`backend/`、`frontend/`。本文档为「记录」性质,不要据此触发任何修改,以避免与开发 agent 冲突**。
>
> 协调者应在以下条件**同时满足**后,再据本文档派单:
> - Phase 1 W2 / W3 / W4 任务包全部进入 review / merged 状态
> - 没有 in-flight PR 涉及 `configs/mock-data/`
> - configs agent 当前 idle,可单独承接

---

## 1. 上下文

`docs/adr/0002-no-kserve.md`(commit `e30da89`)已从技术栈基线移除 KServe,推理服务全部基于 vllm-ascend Deployment + 自研 inference-operator。Plan 评审时声明「代码层 / 契约层 / Phase 1 任务包无影响」,**但漏检了 mock-data 层**。

补充 grep 显示 mock-data 仍有 11 处 `InferenceService` 字面引用:

| 文件 | 行号 | 上下文 |
|---|---|---|
| `configs/mock-data/schema.json` | 291 | `Workload.kind` enum 含 `"InferenceService"` 与 `"ModelService"`(过渡期共存) |
| `configs/mock-data/set-a-small/workloads.json` | 35 / 143 / 236 / 649 | 4 处 `"kind": "InferenceService"`(qwen-8b-pd / deepseek-20b / qwen-14b / internlm-7b 等推理实例) |
| `configs/mock-data/generator/preset_small.go` | 366 / 420 / 424 / 444 | 4 处 `mk(..., "InferenceService", ...)` 调用 |
| `configs/mock-data/generator/preset_small.go` | 431 | 1 处注释 `// job-like InferenceService used for load` |
| `configs/mock-data/generator/README.md` | 78 | 文档列举 `kind` 时含 `InferenceService` |

`configs/mock-data/schema.json` 在 `CLAUDE.md §4` 明列为**共享契约**,修改必走 RFC。

---

## 2. 提议

### 2.1 schema.json:`Workload.kind` enum 收敛
```diff
-"enum": ["Deployment", "StatefulSet", "Job", "DaemonSet", "InferenceService", "ModelService"]
+"enum": ["Deployment", "StatefulSet", "Job", "DaemonSet", "ModelService"]
```

### 2.2 workloads.json:4 处 `"kind"` 值改写
| Workload | 原 | 提议 | 理由 |
|---|---|---|---|
| `qwen-8b-pd`(line 35) | `InferenceService` | `ModelService` | PD pair,inference-operator 管理 |
| `deepseek-20b`(line 143) | `InferenceService` | `ModelService` | 单实例推理 |
| `qwen-14b`(line 236) | `InferenceService` | `ModelService` | 单实例推理 |
| `internlm-7b`(line 649,pending) | `InferenceService` | `ModelService` | 单实例推理 |

### 2.3 preset_small.go:4 处函数调用 + 1 注释
- 4 处 `mk(..., "InferenceService", ...)` → `mk(..., "ModelService", ...)`
- 注释 `// job-like InferenceService used for load` → `// job-like benchmark workload used for load`(注:对应的 `vllm-bench` 已用 `kind: Job`,注释里 InferenceService 是误描述)

### 2.4 README.md:文档同步
- line 78 `(\`Deployment\` / \`InferenceService\` / \`Job\`)` → `(\`Deployment\` / \`ModelService\` / \`Job\`)`

---

## 3. 影响范围核查

### 3.1 已核查(0 残留)
- `docs/api-contract.yaml`:不含 `InferenceService`(grep 0 matches)→ 契约不变
- `backend/`:不含 `InferenceService`(grep 0 matches)→ 后端代码不变
- `frontend/`:不含 `InferenceService`(grep 0 matches)→ 前端无渲染分支依赖此字面

### 3.2 待核查(执行 agent 应自查)
- 任意 e2e / 集成测试是否 hardcode `"kind": "InferenceService"`(grep `tests/`、`*.test.*`、`*.spec.*`)
- Grafana dashboard JSON 是否用 `kind` 作为过滤(目前 dashboards 尚在 Phase 1 W3,可能未引用)

---

## 4. 验证

```bash
# 期望:configs/mock-data/ 不再出现 InferenceService 字面(除已声明的历史引用)
grep -rn "InferenceService" configs/mock-data/
# → 0 matches

# 后端 mock 源加载无错(P1-T-204 已实现 mock-source)
go test ./backend/internal/mock/...

# 前端拓扑/工作负载页运行,workloads 显示 kind 标签为 ModelService
npm run dev --prefix frontend
# 手动访问 / → 拓扑 → 双击节点 → workloads,确认无 InferenceService 字样
```

---

## 5. 估时
0.5 工日(1 个 configs agent 执行 + 1 个 frontend agent 抽检)

---

## 6. 已拒方案

### 方案 B:保留 InferenceService 在 enum,数据全部迁 ModelService
- 优点:enum 向后兼容,旧测试不破
- 缺点:enum 留死字段,违反「不引入 KServe」决策的字面;ADR-0002 §影响范围内 mock 数据应同步收敛

### 方案 C:暂不动,Phase 5 真实接入推理框架时再统一改
- 优点:零成本
- 缺点:Phase 1 演示阶段就用错的 kind 标签,前端开发同学会困惑;脱离 ADR-0002 的 baseline 漂移

---

## 7. 执行前检查清单(给协调者 + configs agent)

**协调者前置确认**(派单前):
- [ ] Phase 1 W2 / W3 / W4 任务包全部 merged
- [ ] 没有 in-flight PR 涉及 `configs/mock-data/`
- [ ] configs agent idle

**configs agent 执行清单**:
- [ ] 检查 `tests/` 是否有 `"kind": "InferenceService"` 硬编码
- [ ] 检查 Grafana dashboards(P1-T-209)是否有 `kind` filter
- [ ] 修改 schema.json + workloads.json + preset_small.go + README.md
- [ ] 跑 `go generate ./configs/mock-data/generator/...`(若有重生成入口)
- [ ] 跑 §4 verification
- [ ] 单一 commit:`refactor(configs): mock-data align with ADR-0002 — InferenceService → ModelService`

---

## 8. 备注:为什么本次 ADR-0002 commit 没顺手改

ADR-0002 commit(`e30da89`)的 plan 评审时声明的影响范围是「文档层」。Mock-data 属共享契约层,按 `CLAUDE.md §8` 必须**单独走 RFC**,不能搭便车在文档对齐 commit 里。本文档为该独立流程的**前置记录**——记录已知漂移、备查、待时机成熟时再触发。

## 9. 修订历史

- **2026-05-17 v1**(初版):以 RFC 草案形式起草,建议立即执行
- **2026-05-17 v2**(本版):降级为「已知漂移记录」,**执行时机推迟至 Phase 1 W4 收尾后**——根据协调者反馈「已有 agent 在执行开发任务,避免修改冲突」
