# demo-backend

Go service behind the O-Cloud demo frontend. REST + WebSocket over `/api/v1`,
multi-source data abstraction (mock → k8s → prometheus → crd → configmap as
phases land).

**Source of truth for module rules:** [`./CLAUDE.md`](./CLAUDE.md) — every
contributor (human or AI agent) MUST read it before touching files here.

## Quick start

```bash
cd backend
make tidy
make build       # → bin/demo-backend
make run         # uses configs/config.dev.yaml, listens on :8080

# verify
curl -s localhost:8080/api/v1/healthz | jq
curl -s localhost:8080/api/v1/version | jq
```

## Layout

See `CLAUDE.md` §3. Don't reorder without an RFC.

## Phase 1 scaffold (P1-T-005)

This commit ships the scaffold only:

- `cmd/demo-backend/main.go` — Cobra + Viper entrypoint.
- `pkg/api/{system,router,errors}.go` — only `/healthz` and `/version`.
- `pkg/datasource/source.go` — full `Source` interface.
- `pkg/datasource/mock/source.go` — stub implementing every method as
  `ErrNotImplemented`. T101+ fills these in.
- `pkg/{model,config,cache,middleware}/` — supporting infrastructure.

Run `make test` to confirm `/healthz` and `/version` work end-to-end.
