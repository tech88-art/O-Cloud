# Frontend CLAUDE.md — 演示前端模块协作指南

> 前端 Agent 启动新会话时**先读这份**，再开任务包。

---

## 1. 模块定位

`frontend/` — React 18 + TypeScript SPA，演示前端。

**核心页面**（5 个）：
- `/overview` — 概览拓扑
- `/workloads` — 工作负载
- `/deploy` — 应用部署
- `/metrics` — 指标（Grafana 嵌入）
- `/logs` — 日志

**设计原则**：
- 与后端通过 OpenAPI 契约通信（`docs/api-contract.yaml`）
- 不做后端的活（数据聚合、业务逻辑），仅展示与交互
- 文本走 i18n，禁止硬编码中/英文字符串

---

## 2. 模块路径所有权（OWN）

```
frontend/**
```

**只读**：
- `docs/api-contract.yaml`（生成 TS 类型时使用）
- `configs/mock-data/**`（联调期可读，但不要在前端代码里引用）

---

## 3. 目录约定

```
frontend/
├── src/
│   ├── pages/
│   │   ├── Overview/
│   │   │   ├── index.tsx
│   │   │   ├── TopologyView.tsx
│   │   │   ├── DetailPanel.tsx
│   │   │   └── styles.module.css
│   │   ├── Workloads/
│   │   ├── Deploy/
│   │   ├── Metrics/
│   │   └── Logs/
│   ├── components/                 跨页通用组件
│   │   ├── TopologyGraph/          G6 封装
│   │   ├── ResourceCard/
│   │   ├── MetricChart/            ECharts 封装
│   │   ├── GrafanaPanel/           iframe 嵌入
│   │   ├── StatusTag/
│   │   ├── DeployWizard/
│   │   └── LogViewer/
│   ├── services/                   API 客户端
│   │   ├── api.ts                  axios 实例
│   │   ├── types.ts                auto-gen from OpenAPI
│   │   ├── cluster.ts
│   │   ├── workload.ts
│   │   └── ...
│   ├── store/                      Zustand
│   │   ├── topologyStore.ts
│   │   └── ...
│   ├── hooks/                      自定义 hook
│   │   ├── useWebSocket.ts
│   │   └── ...
│   ├── i18n/
│   │   ├── index.ts
│   │   ├── zh-CN.json
│   │   └── en-US.json
│   ├── config/
│   │   └── runtime.ts              运行时配置（API base URL 等）
│   ├── styles/
│   │   ├── theme.ts
│   │   └── global.css
│   ├── App.tsx
│   └── main.tsx
├── public/
├── tests/                          Vitest 单测
├── e2e/                            Playwright（如本模块负责则放这里）
├── vite.config.ts
├── tsconfig.json
├── package.json
├── .eslintrc.cjs
├── .prettierrc
└── README.md
```

---

## 4. 关键约定

### 4.1 类型从契约生成

```bash
# 启动前自动生成
pnpm run gen:types
```

底层：`openapi-typescript docs/api-contract.yaml -o src/services/types.ts`

**禁止手写 API 响应类型**。契约改了就重跑生成。CI 会校验生成结果与提交一致。

### 4.2 API 调用统一走 react-query

```tsx
// services/cluster.ts
import { useQuery } from '@tanstack/react-query';
import { api } from './api';
import type { components } from './types';

type Cluster = components['schemas']['Cluster'];

export function useClusters() {
  return useQuery({
    queryKey: ['clusters'],
    queryFn: async () => {
      const { data } = await api.get<Cluster[]>('/api/v1/clusters');
      return data;
    },
    staleTime: 30_000,
  });
}
```

**禁止**直接在组件里调 `axios`。

### 4.3 状态管理用 Zustand

```ts
// store/topologyStore.ts
import { create } from 'zustand';

interface TopologyState {
  selectedNodeId: string | null;
  setSelected: (id: string | null) => void;
}

export const useTopologyStore = create<TopologyState>((set) => ({
  selectedNodeId: null,
  setSelected: (id) => set({ selectedNodeId: id }),
}));
```

**禁止** Redux、Recoil 等其他状态库。

### 4.4 i18n

所有文本：

```tsx
import { useTranslation } from 'react-i18next';

function MyComponent() {
  const { t } = useTranslation();
  return <Button>{t('common.deploy')}</Button>;
}
```

key 命名：`<page>.<element>.<purpose>`，扁平结构。

每加 key **必须**同时更新 `zh-CN.json` 和 `en-US.json`。

### 4.5 UI 组件

- 优先用 Ant Design 5
- 复杂图（拓扑、自定义图）才上 G6
- 仪表盘类指标用 Grafana iframe，**不要**自己写 ECharts 大盘（除非 GrafanaPanel 不适用）
- 不引入其他 UI 库（Bootstrap、MUI 等禁止）

### 4.6 WebSocket

```ts
// hooks/useWebSocket.ts
export function useTopologyWS(clusterId: string) {
  // 自动重连、错误恢复
}
```

订阅消息更新 Zustand store，组件从 store 读。

### 4.7 配置驱动

`src/config/runtime.ts` 在运行时从 `/config.json` 加载，**不**编译时写死。

```ts
export interface RuntimeConfig {
  apiBaseURL: string;
  wsBaseURL: string;
  grafanaBaseURL: string;
}
```

部署时通过 ConfigMap 注入 `/config.json`。

### 4.8 加载、错误、空状态

每个数据展示组件**必须**处理三种状态：

```tsx
if (isLoading) return <Skeleton />;
if (error) return <ErrorState onRetry={refetch} />;
if (!data || data.length === 0) return <Empty />;
return <Content data={data} />;
```

不允许加载中显示空白 / 出错不提示。

---

## 5. 开发命令

```bash
cd frontend

pnpm install
pnpm run gen:types            # 从契约生成类型
pnpm run dev                  # 启动 dev server (localhost:3000)
pnpm run build                # 生产构建
pnpm run preview              # 预览构建
pnpm run test                 # Vitest 单测
pnpm run test:e2e             # Playwright（如配置）
pnpm run lint                 # eslint
pnpm run lint:fix
pnpm run format               # prettier
pnpm run typecheck            # tsc --noEmit
```

---

## 6. 样式约定

- 全局样式仅放主题 token（`styles/theme.ts`）和 reset（`global.css`）
- 组件样式用 CSS Modules（`*.module.css`）
- **禁止** `style={{ ... }}` 内联（除动态计算的位置）
- **禁止** 引入 styled-components、emotion 等 CSS-in-JS 库

---

## 7. 拓扑图组件（TopologyGraph）特别约定

- 封装 G6，对外提供受控 API
- 输入：`{ nodes: TopologyNode[], edges: TopologyEdge[] }`（对应 OpenAPI schema）
- 事件：`onNodeClick`, `onNodeDoubleClick`, `onEdgeClick`
- 不直接调 API（数据由父组件传入）

POC 阶段先做最小版（节点 + 边 + 点击高亮），逐步加：
- 双击 NPU 展开切片
- 力导向 / 分层布局切换
- 节点颜色按状态
- 缩放、平移
- 右键菜单

---

## 8. Grafana 嵌入（GrafanaPanel 组件）

```tsx
<GrafanaPanel
  dashboard="cluster_overview"
  variables={{ cluster: 'cluster-a' }}
  height={600}
/>
```

内部：
1. 调 `/api/v1/grafana/url?dashboard=...` 取签名 URL
2. iframe 加载
3. 处理跨域 / 鉴权失败 / 加载失败

W1 POC 阶段先打通最简单的 iframe，再迭代鉴权。

---

## 9. 单测

- Vitest + React Testing Library
- 覆盖关键 hook 和组件
- 覆盖率目标：≥ 50%（前端测试成本高，门槛低于后端）

---

## 10. 禁止行为

- ❌ 手写 API 响应类型（用 gen）
- ❌ 直接调 axios（用 react-query 包装）
- ❌ 引入未在 `package.json` 中声明的依赖
- ❌ 修改 `docs/api-contract.yaml`（走 RFC）
- ❌ 在组件里硬编码中/英文字符串
- ❌ 用 `any`（除非紧急临时，必须加 `// FIXME(@<task>)`）
- ❌ 跨模块改文件

---

## 11. 常用 Prompt 模板

### 新增一个页面

```
读完 frontend/CLAUDE.md 和 docs/agent-coordination.md 后，执行 P1-T-XXX。

具体：
1. 在 frontend/src/pages/<Page>/ 新建页面
2. 用 react-query 拉 /api/v1/<endpoint>
3. 拆分组件：主页面 + 子组件，每个组件单独文件
4. 加 loading / error / empty 状态
5. 文本全部走 i18n，更新 zh-CN.json 和 en-US.json
6. 加路由到 App.tsx
7. 加 Vitest 测试覆盖关键交互
8. 跑 pnpm lint、pnpm typecheck、pnpm test
9. 截图给我看

仅修改任务包 Allowed Paths 范围内的文件。
```

### 新增通用组件

```
读完模块文档后，执行 P1-T-XXX：
1. 在 frontend/src/components/<ComponentName>/ 新建组件
2. 提供 TypeScript 类型完整的 Props
3. 处理 loading / error / empty 状态（如适用）
4. 加 Storybook story（如有）
5. 加单测
6. 不改其他组件

完成后给一个使用示例。
```
