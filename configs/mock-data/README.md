# Mock Data

Mock 数据集与 JSON Schema 工作目录。

## 文件结构

```
configs/mock-data/
├── schema.json                ★ JSON Schema (draft 2020-12). 唯一真理.
├── README.md                  本文件
├── examples/
│   └── minimal-valid.json     最小 happy-path 样例 (T004 自测用)
├── generator/                 数据生成器 (P1-T-011 创建, Go 实现)
├── set-a-small/               单节点演示 (P1-T-011/T-109 输出)
├── set-b-multi-site/          多站点联邦演示 (P1-T-307 输出)
└── set-c-stress/              压测演示 (P1-T-307 输出)
```

## Schema 校验命令

### 编译 schema(检查 schema 本身合法)

```bash
npx ajv-cli@5 ajv compile \
  --spec=draft2020 \
  --strict=false \
  -s configs/mock-data/schema.json
```

### 校验数据集

```bash
npx ajv-cli@5 ajv validate \
  --spec=draft2020 \
  --strict=false \
  -s configs/mock-data/schema.json \
  -d "configs/mock-data/set-*/**/*.json"
```

### Flags 解释

- `--spec=draft2020`:schema 使用 JSON Schema draft 2020-12,ajv-cli 默认 draft-07
- `--strict=false`:允许非标准 keyword(schema 顶层有 `version` 字段作为版本标识,非 JSON Schema 标准 keyword)

### 已知 warning(无害)

ajv 报告:
- `unknown format "int64"` — JSON Schema 不定义 int64,OpenAPI 约定。可忽略或装 `ajv-formats` 插件。
- `unknown format "date-time"` — 同上,ajv-formats 解决。

## 数据集字段对齐(T004 sealed-for-review 时的核对)

- 与 `docs/api-contract.yaml` `components.schemas` 字段名 1:1 一致(Cluster / Node / NPU / Workload / Preset / Slice 等)
- 与 `operators/pool-operator/api/v1alpha1/*.go` CRD 类型字段对齐(P1-T-003 完成时验证)

字段命名规范(camelCase):JSON / TypeScript / Go(经 `json:` tag)统一。

## 数据真实度

见 `configs/CLAUDE.md §3.3`(命名规范、HCCS 组、NUMA、利用率分布、事件流时间表)。

## 修改本 Schema

走 `docs/agent-coordination.md §0a.5` 流程(chat + ADR),不允许直接改 schema 不告知。
