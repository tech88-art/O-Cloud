# O-Cloud Demo Frontend

React 18 + TypeScript 5 + Vite + AntD 5 SPA for the O-Cloud edge cloud platform demo.

See `CLAUDE.md` in this directory for module conventions, directory layout, and the full list of dos and don'ts. Do not skip it before adding code.

## Quick start

```bash
pnpm install
pnpm run gen:types   # regenerate src/services/types.ts from docs/api-contract.yaml
pnpm run dev         # http://localhost:3000
```

## Scripts

| command | purpose |
|---|---|
| `pnpm run dev` | Vite dev server |
| `pnpm run build` | typecheck + production build |
| `pnpm run preview` | preview the built bundle |
| `pnpm run test` | Vitest single run |
| `pnpm run lint` | ESLint (zero-warning policy) |
| `pnpm run typecheck` | `tsc -b --noEmit` |
| `pnpm run gen:types` | regenerate API types from `docs/api-contract.yaml` |

## Layout (high level)

```
src/
  pages/                5 routes — Overview / Workloads / Deploy / Metrics / Logs
  components/Layout     AntD Sider + Header + Content shell
  services/             axios instance, react-query client, auto-gen API types
  store/                Zustand UI store
  hooks/                shared hooks (incl. useWebSocket stub)
  i18n/                 react-i18next (zh-CN default, en-US)
  config/               /config.json runtime loader
  styles/               AntD theme tokens + global reset
public/config.json      runtime config (dev defaults)
tests/                  Vitest + RTL
```

See `CLAUDE.md` §3 for the full target tree.
