# deploy/dev — Local Docker Compose Stack

One command brings up the full local development environment:

```bash
make dev-up         # from repo root
# equivalent to:
docker compose -f deploy/dev/docker-compose.yaml up -d
```

Tear down:

```bash
make dev-down
```

Follow logs:

```bash
make dev-logs
```

---

## Services & Ports

| Service | Host Port | Container Port | Notes |
|---|---|---|---|
| backend | 8080 | 8080 | demo-backend with mock datasource |
| frontend | 3000 | 3000 | Vite dev server with HMR |
| prometheus | 9090 | 9090 | scrapes self + backend:8080/metrics |
| grafana | **3001** | 3000 | mapped to 3001 to avoid clash with frontend |
| loki | 3100 | 3100 | opt-in (`--profile logs`) |
| promtail | — | 9080 | opt-in, no host port |

All containers share the `ocloud-dev` bridge network, so service-to-service
references use the service name (`http://backend:8080`, `http://prometheus:9090`).

### Opt-in: logs stack

Loki + Promtail are gated behind the `logs` Compose profile so a default
`make dev-up` stays light. Enable them explicitly:

```bash
docker compose -f deploy/dev/docker-compose.yaml --profile logs up -d
```

---

## Access

| URL | What |
|---|---|
| http://localhost:3000 | Frontend (Vite) |
| http://localhost:8080 | Backend REST API |
| ws://localhost:8080 | Backend WebSocket |
| http://localhost:9090 | Prometheus UI |
| http://localhost:3001 | Grafana UI |
| http://localhost:3100 | Loki HTTP API (with `--profile logs`) |

### Grafana credentials

| User | Password |
|---|---|
| `admin` | `admin` |

Anonymous access is enabled with the `Viewer` role so the T010 iframe POC
in the frontend can embed dashboards without a login flow.

Override the admin credentials by setting env vars before `docker compose up`:

```bash
GF_ADMIN_USER=alice GF_ADMIN_PASSWORD=secret docker compose -f deploy/dev/docker-compose.yaml up -d
```

---

## Hot reload

### Frontend — yes

Vite is run with `--host 0.0.0.0`. The `frontend/` directory is bind-mounted
into the container, so file edits on your host trigger HMR in the browser.
Note `node_modules` is an anonymous volume — to refresh dependencies after
editing `package.json`, restart the frontend container:

```bash
docker compose -f deploy/dev/docker-compose.yaml restart frontend
```

### Backend — no (by design)

The backend image is built from the production multi-stage Dockerfile
(`backend/Dockerfile`), whose runtime layer is `distroless/static` — no
shell, no Go toolchain, no `air`. This is intentional: we want the dev
image to be the prod image, so anything that runs here also runs in CI.

For backend hot reload, run the binary **outside** Compose:

```bash
# Terminal 1 — keep the supporting services running:
docker compose -f deploy/dev/docker-compose.yaml up -d prometheus grafana
# (optional: also start frontend if you want full UI)

# Terminal 2 — backend with live reload:
cd backend
make run                 # uses configs/config.dev.yaml
# or with air if you have it installed:
# air -c .air.toml
```

The backend service inside Compose can be stopped without affecting the
others:

```bash
docker compose -f deploy/dev/docker-compose.yaml stop backend
```

---

## Configuration

### Backend

- Image is built from `../../backend` (Compose build context).
- `backend/configs/config.dev.yaml` is bind-mounted to `/app/configs/config.yaml`.
  All datasources resolve to `mock`.
- Mock data files (`configs/mock-data/`) are bind-mounted at
  `/app/configs/mock-data` — edit them on the host and they appear
  immediately inside the container (backend re-reads on each request).

### Prometheus

- Scrape config: `./prometheus/prometheus.yml`
- Two jobs: `prometheus` (self) + `backend` (the demo backend).
- Until P1-T-204 adds `/metrics` to the backend, the backend job will show
  `up=0`. Grafana panels render "No data" gracefully — nothing crashes.

### Grafana

- Provisioning: `./grafana/provisioning/` (mounted read-only).
  - `datasources/datasources.yaml` — pre-wires Prometheus + Loki.
  - `dashboards/default.yaml` — watches `./grafana/dashboards/` for JSON.
- Dashboards: `./grafana/dashboards/` (empty today; P1-T-209 will populate).
- Embedding is enabled (`GF_SECURITY_ALLOW_EMBEDDING=true`) for the T010
  iframe POC.

### Loki & Promtail (opt-in)

- Loki uses local filesystem storage in the `loki-data` volume — no S3.
- Promtail tails Docker container logs via `/var/run/docker.sock`.

---

## Troubleshooting

### Port already in use

Stop whatever is on the host port (commonly Vite running standalone on
3000, or a global Grafana on 3001). Find it:

```bash
# Linux / macOS
lsof -i :3000
# Windows (PowerShell)
Get-NetTCPConnection -LocalPort 3000
```

### Frontend says `pnpm: command not found`

The image is `node:24-alpine`; corepack enables pnpm at container start.
If the entrypoint fails, rebuild without cache:

```bash
docker compose -f deploy/dev/docker-compose.yaml up --force-recreate frontend
```

### Backend container restarts repeatedly

Check the config mount and the mock data path:

```bash
docker compose -f deploy/dev/docker-compose.yaml logs backend
```

The most common cause is `configs/mock-data/set-a-small/` not yet
existing (P1-T-011 ships it).

### Grafana shows "datasource unreachable: Loki"

Expected if you ran `make dev-up` without `--profile logs`. Either start
the logs stack or ignore — Prometheus panels still work.

### "No data" everywhere in Prometheus / Grafana

That's expected pre-T204 — the backend has no `/metrics` endpoint yet.
The scrape error is logged but does not block anything.

### `docker compose config` warnings

Run it to validate the file:

```bash
docker compose -f deploy/dev/docker-compose.yaml config
docker compose -f deploy/dev/docker-compose.yaml --profile logs config
```

Both should print the resolved spec with no errors. If you see warnings
about `version: ...` being obsolete, ignore them — modern Compose ignores
the top-level `version` field, and this file omits it deliberately.

---

## Security notes (dev only)

The defaults in this directory are for local development **only**:

- Grafana admin password is `admin` (env-overridable).
- Anonymous viewer access is enabled.
- Embedding (iframes) is allowed from anywhere.
- No TLS, no auth on backend or Prometheus.

Do not copy this stack to a shared host without changing all of the above.
Production deployment lives under `deploy/single-node/` and the Helm chart
under `deploy/helm-charts/ocloud-edge/`.

---

## Related tasks

- **P1-T-008** (this) — Compose stack scaffolding
- **P1-T-010** — Grafana iframe POC (consumes Grafana service)
- **P1-T-204** — backend `/metrics` endpoint (makes the Prometheus scrape green)
- **P1-T-209** — Grafana dashboards JSON (fills `grafana/dashboards/`)
