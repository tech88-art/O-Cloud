# Phase 12 · 中段交接 (W1+W2 → W3) — for the next execute session

> **2026-06-01** · 本 session 连续执行 T001→T105(W1 Foundation + W2 Track A/B)· 全部 strict-verify + 本地 commit。下一 session 从 **P12-T-201** 起执行 W3 Track C(前端)+ closer。本文是接手入口,读完即可开工。

---

## 0. TL;DR

- **进度**:**10/16 task 完成**(W1 4 + W2 6)· 非 UI 半(平台翻转 / 后端拓扑 / mock 数据 / 全契约)**已全部落地 + 验证**。
- **Git**:branch `dev` · HEAD `c102015` · **工作树干净**(全提交)· **领先 origin/dev 13 commit**(4 plan + 9 task)· **未 push**(按 push-at-phase-tag 协议累积)。
- **下一步**:T201→T205(前端 one-page workspace 重构)→ T301(e2e)→ T302(checkpoint + tag `phase-12-complete` + push → CI gate)。
- **关键**:契约 / 后端 / mock 数据**都已 ready**,前端只管渲染 —— 数据管线已通,render-verify 数据已就位。

---

## 1. 本 session 已完成 commit 链(本地 · 未 push)

```
c102015 feat(configs): P12-T-105 mock arch/os arm64 openEuler + PCIE/HCCS/network 带宽 fixtures
fb68979 feat(backend): P12-T-104 aggregator emit network/hccs/runs-on 边 + PCIE (ADR-0021)
b026616 feat(deploy): P12-T-103 CANN aarch64 matrix + helm arch 亲和 + install.sh openEuler
d628117 ci(deploy):   P12-T-102 arm64 cross-compile matrix job (追加 · 不替换 amd64)
40e3a97 feat(deploy): P12-T-101 10 Dockerfile multi-arch (aarch64 鲲鹏 + 昇腾 910B target)
9dd75ee docs(adr):    P12-T-004 ADR-0022 one-page workspace UI IA + frontend DESIGN
4bc7097 docs(contract):P12-T-003 ADR-0021 + api-contract 拓扑数据全保真扩展 + gen:types
5e94265 docs(adr):    P12-T-002 ADR-0020 aarch64 鲲鹏 + openEuler target · supersede ADR-0001 §13
95eb824 docs(adr):    P12-T-001 ADR-0019 Phase 12 entry decisions
```
每个 task 的调试轨迹 / 路径偏差 / 决策 see `docs/devlog/phase-12-t0NN.md`(t101-t105 有 Carry-forward 段直接指向前端)。

---

## 2. 下一 session 待执行(顺序 · plan = `docs/phase12-plan.md`)

| Task | 内容 | plan §detail | per-task gate |
|---|---|---|---|
| **T201** | Layout shell:删 AntSider nav 菜单 + App.tsx 路由收敛 + **AntD Splitter** 可隐藏可拖拽(左树 + 右栏 · 替换 grid) | §487 | `pnpm typecheck && lint && test` + **render-verify** |
| **T202** | 拓扑增强:**绿色 network 连线** + edge hover **带宽 tooltip**(自定义 edge)+ **PCIE/HCCS 渲染** + workload→node(runs-on)连线 + focus/isolate 过滤 | §522 | 同上 + render-verify |
| **T203** | 右栏 DetailPanel:吸收 workload/pod + 指标 section(资源/业务 grafana toggle)+ 日志 section(容器选择) | §560 | 同上 + render-verify |
| **T204** | 顶栏预置应用 bar(PresetGrid/DeployWizard fold + hover 详情 + deploy) | §~590 | 同上 + render-verify |
| **T205** | 退役 `/workloads /deploy /metrics /logs` 路由 + i18n cleanup + Vitest 更新(3 状态 + 关键交互) | §~615 | + `pnpm build`(无 unused/死码) |
| **T301** | Playwright e2e one-page flow 重写 + kind smoke arch 校验(若适用) | §~660 | `npx playwright test` or syntactic + CI gate |
| **T302** | docs 大整理 + checkpoint-phase12 + **tag `phase-12-complete`** + M6 milestone announce | §~668 | → **push → CI gate** |

> **T201 是 gate**(layout shell)· T202-T205 在其上叠加 · 严格按序。

---

## 3. 前端接手关键上下文(数据管线已通)

下一 session **不需要再碰契约/后端/数据** —— 它们已 ready:

1. **契约 + TS 类型已扩**(T003 · 已 `gen:types`):`frontend/src/services/types.ts` **已含** 新 edge type union(`network` / `hccs` / `runs-on`)+ `pcieBandwidthGBps` 等。CI 有 gen:types drift gate —— 若动 `api-contract.yaml` 必 `cd frontend && pnpm run gen:types` 重生 + commit;**不动契约就无需重生**。
2. **后端 aggregator 已 emit**(T104 · `backend/pkg/aggregator/topology.go`):
   - `network` 边 = node↔node(both 端点是 node)· attrs `bandwidthGBps`/`medium`/`utilization`
   - `hccs` 边 = npu↔npu(同 node + hccsGroup · ring)· attrs `bandwidthGBps`/`hccsGroup`
   - `runs-on` 边 = 非 NPU pod→node(pod 无 slice binding 时)· attrs `workload`
   - **NPU 节点新属性** `pcieBandwidthGBps`
   - **gating(重要)**:`network`+`hccs` 跟 **IncludeFabric**(前端 `showFabric` toggle)· `runs-on` 跟 **IncludeWorkloads**(`showWorkloads` toggle)· `pcie` 属性始终在(数据有就渲)。前端 toggle 行为要对齐。
3. **mock set-a 数据已就位**(T105):`configs/mock-data/set-a-small/` 的 npus(`pcieBandwidthGBps:32`/`hccsBandwidthGBps:56`)+ networkLinks(3 条 `net-*` node↔node `roce` 25 GB/s)→ **render-verify 时拓扑能真显示** network 绿边 + hccs + pcie hover。set-b/c 同步有。
4. **IA + 组件地图**:`docs/adr/0022-one-page-workspace-ui.md`(决策)+ `frontend/docs/one-page-workspace.md`(组件 DESIGN · 哪些组件吸收/新建/退役)· **T201-205 必读**。

---

## 4. Render-verify 怎么做(plan §8 · 每个 Track C task 必产 `after-*.png`)

Phase 11 closer 跳过了 render-verify(只跑 typecheck)→ Phase 12 §8 强制补。机制:

- **基线**:`docs/screenshots/phase12/before-*.png`(5 页 · phase-11-complete 状态 · 已在 repo)+ `README.md` 说明。
- **截图脚本**:`tests/e2e/baseline-screenshots.mjs`(独立 Playwright · 非 test runner · 自控)· 改成截 `after-*.png` 即可。
- **跑法**(plan §8 line 733+,与 CPU 架构解耦 · 本机 amd64 dev 照常):
  ```
  终端1: cd backend && go run ./cmd/demo-backend   # mock :8080 (config.dev.yaml → set-a-small)
  终端2: cd frontend && pnpm install && pnpm dev    # Vite :3000 (HMR · 改 .tsx 即时热刷)
  浏览器: http://localhost:3000  (apiBaseURL :8080 跨域直连 · 无需 vite proxy)
  截图: node tests/e2e/baseline-screenshots.mjs (改 URL/输出名截 after-*)
  ```
- ⚠️ **不要用 Preview MCP `preview_screenshot`**(plan §8 注:本 app WS 长连 · 5 次 1 成 · 不可靠)· **用 Playwright**。

---

## 5. ⚠️ Push 协议(CRITICAL · 别每 task push)

- **不要每 task push**(memory `feedback_push_at_phase_tag_only`):每 task commit + 停 · 不问 "push?" · 全链(task + fix + checkpoint + tag)**攒本地**。
- **phase-12-complete tag 落定时一次 push**(T302)→ 触发 CI gate。
- **push 后必走 CI gate**(memory `feedback_post_tag_ci_gate`):看 GitHub Actions · 修所有 ❌ 直到 **dev HEAD 全绿** 才算 phase 真完成(注意:本 session 新加 `cross-compile-arm64` matrix job 10 模块 · 首次 CI 会跑它 · 已本机验证全过但 CI 环境首跑留意)。
- creds:`reference_github_creds`(Windows cmgr 已存 · 直接 `git push` 通 · 别 embed token)。

---

## 6. 下一 session 启动 checklist

```
1. 读 docs/devlog/phase-12-handover.md(本文)
2. 读 CLAUDE.md(根 + frontend/CLAUDE.md)+ docs/agent-coordination.md §0a.10-12
3. 读 docs/adr/0022-one-page-workspace-ui.md + frontend/docs/one-page-workspace.md(IA + 组件地图)
4. 读 docs/phase12-plan.md §487+(T201-205 detail)+ §8(render-verify)
5. git fetch && git status(确认 dev 干净 · 领先 13 · 未 push)
6. 从 P12-T-201 起 · 单 task 串行 + strict verify(typecheck+lint+test+render-verify)+ commit + 不 push
```

---

## 7. W2 关键决策 / 坑(影响前端的)

- **多架构 ≠ arm64-only**:amd64 保留本机 dev/CI/render-verify(`make dev-up` 不破 · 前端 dev 照常 amd64)· arm64 是部署 target。render-verify 与 CPU 架构解耦。
- **soft nodeAffinity**(T103):helm 用 `preferredDuringScheduling` arm64(非 hard)· 因 kind smoke 在 amd64 集群跑 —— 前端不涉及,但 T301 kind smoke 注意仍 amd64。
- **mock 数据走 transform 非 regenerate**(T105):curated sets 是 "T307 polished" · regenerate 会丢数据(实测 -777 workloads)· 已回退。**前端别触发 generator regenerate**(会破坏 demo 数据)· 改数据走手动 + 同步 generator(configs/CLAUDE.md §8)。
- **aggregator 零回归**(T104):新边只在 toggle on 时 emit · `showFabric`/`showWorkloads` off → 图与旧版 byte-equivalent · 前端默认 toggle 状态要想清楚(Overview 默认显不显 network/hccs?见 ADR-0022)。
- **edge type 已进契约枚举**(T003)· 前端 `types.ts` 已有 · 渲染未知 type 也有 generic fallback(topology.go 注)。

---

## 8. 可复用验证命令(参考)

```
# 后端
cd backend && go test ./... && go test ./pkg/aggregator/... -cover   # 93.5%
cd backend && go vet ./... && gofmt -l pkg/
# 前端
cd frontend && pnpm typecheck && pnpm lint && pnpm test && pnpm build
cd frontend && pnpm run gen:types && git diff --quiet src/services/types.ts  # drift gate
# mock 数据(ajv = CI validate-mockdata)
npx -y -p ajv-cli@5 -p ajv-formats@3 ajv validate --spec=draft2020 --strict=false -s configs/mock-data/schema.json -d "configs/mock-data/set-a-small/*.json"
# arm64 交叉编译(任一模块)
cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...
# helm(任一 chart)
helm lint --strict deploy/helm-charts/demo-backend && helm template t deploy/helm-charts/demo-backend | grep arm64
```

---

**END · 接手从 P12-T-201 开始 · 数据管线已通 · 只剩前端渲染 + closer。**
