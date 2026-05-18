# Single-node O-Cloud Edge deployment

Pragmatic single-host deployment of the demo stack — backend + frontend +
Grafana + Prometheus + (optional) Loki. Targets a fresh Ubuntu 22.04 LTS
box, gets to a running demo in under 30 minutes (P1-T-304 AC).

For development workflow with hot reload, see `../dev/README.md` instead.
This directory is for the "one-shot install on a demo machine" use case.

## Quick start (one-liner)

From the repo root:

```bash
./scripts/install.sh
```

That's it. The script:

1. Validates the host (Ubuntu 22.04, amd64).
2. Installs Docker Engine + Compose plugin (if absent).
3. Installs Go 1.22+ and Node 20+ via official tarballs / NodeSource.
4. Builds the backend binary and the frontend production bundle.
5. Brings up the `deploy/dev/docker-compose.yaml` stack.
6. Polls health endpoints; aborts if either never responds.
7. Prints access URLs.

Re-run `./scripts/install.sh` any time — it's idempotent.

## Flag reference

| Flag             | Purpose                                                    |
|------------------|------------------------------------------------------------|
| `--image-only`   | Skip the source build; just pull pre-built images.         |
| `--no-start`     | Build artifacts but don't bring up the compose stack.      |
| `--uninstall`    | Tear down the compose stack (leaves the repo dir alone).   |
| `--help` / `-h`  | Show usage.                                                |

Override via env:

| Variable             | Default                            |
|----------------------|------------------------------------|
| `OCEDGE_REPO_DIR`    | `$(dirname $0)/..`                 |
| `OCEDGE_COMPOSE`     | `deploy/dev/docker-compose.yaml`   |

## Verifying the install

After the installer prints the summary banner:

```bash
# Backend healthz (should return: {"status":"ok"})
curl http://localhost:8080/api/v1/healthz

# Frontend (should return HTML for the React app)
curl -sS http://localhost:3000 | head -5

# Topology API smoke
curl "http://localhost:8080/api/v1/clusters/cluster-prod-a-01/topology?depth=slice" | jq '.nodes | length'
#   → expect 94 (1 cluster + 3 nodes + 24 NPUs + 66 slices, set-a-small)
```

Open `http://<host>:3000/overview` in a browser; the five demo pages
(Overview / Workloads / Deploy / Metrics / Logs) should all render.

## Swapping the active dataset

P1-T-303 cements the "config-only" dataset swap. To point the running
stack at a different mock fixture:

```bash
# Edit the path in backend/configs/config.dev.yaml:
#   datasources.mock.path: "../configs/mock-data/set-b-small"
# Then restart just the backend container:
docker compose -f deploy/dev/docker-compose.yaml restart backend
```

The frontend re-fetches automatically (react-query staleTime is 30s).

## Production hardening (out of scope for Phase 1)

The dev compose file is geared for demos: permissive CORS, no TLS, no
auth, mock data sources. Productionizing this is Phase 9 work:

- TLS termination (Caddy / Traefik in front of the compose stack)
- Auth + RBAC (the OpenAPI spec already carries a placeholder bearer
  scheme; backend handlers will start enforcing it in Phase 2)
- Replace mock datasource with k8s / Prometheus / CRD sources (Phase 2,
  no frontend changes required per the P1-T-303 swap contract)
- Image signing (cosign) + provenance attestations

If you need any of the above today, deploy via the Helm chart in
`deploy/helm-charts/` instead (Phase 2 delivery).

## Tested matrix

| OS                  | arch  | Docker     | Status   |
|---------------------|-------|------------|----------|
| Ubuntu 22.04 LTS    | amd64 | 27.x       | OK       |
| Ubuntu 24.04 LTS    | amd64 | 27.x       | OK       |
| Debian 12           | amd64 | 27.x       | Best-effort (the installer warns) |
| Ubuntu on ARM       | arm64 | 27.x       | Demo-only — NPU features skipped  |

Other distros / WSL are unsupported in Phase 1; use the dev compose
stack directly.
