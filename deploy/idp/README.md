# `deploy/idp/` — Dex REFERENCE OIDC Identity Provider

> **Dex here is a REFERENCE implementation, not a production IdP.** It exists so
> the real-profile OIDC authentication path has a working issuer to validate
> tokens against — end-to-end, with no external dependency. **The customer
> replaces it with their own IdP** (Keycloak / Microsoft Entra ID / Okta / Ping
> / any OIDC-compliant provider) by re-pointing three things: `issuerURL`,
> `audience`, and the client registration. See [§ Swapping in your own IdP](#swapping-in-your-own-idp).
>
> Per **ADR-0025 §2 Decision A** (authz model · Dex 参考 IdP) + **§3 right-sizing**
> (Dex is NOT a full IAM — no Keycloak federation / AD / LDAP / SSO) + **§4(a)**
> (甲方 IdP swap point · default 兜底 · 不 block 收口).

---

## What this is

| File | Purpose |
|---|---|
| `dex.yaml` | All Dex objects in one manifest: ServiceAccount, config ConfigMap, client-secret Secret (reference placeholders), Deployment (1 replica), Service (ClusterIP). |
| `README.md` | This file — reference posture + the customer swap point. |

**Right-sized (reference-grade, ADR-0025 §3):**
- **single replica** — no HA, no leader-election;
- **in-memory storage** (`storage.type: memory`) — no CRDs, no database, and
  therefore **no Kubernetes RBAC** on `dex.coreos.com` (Dex never calls the
  apiserver; its ServiceAccount has no Role/ClusterRole bound, and
  `automountServiceAccountToken: false`);
- **in-cluster issuer** (ClusterIP, no Ingress / external TLS / cert
  federation);
- **two static OIDC clients** (`o2-dms`, `ocloud-backend`) + **one static demo
  user** (`admin@ocloud.local` / `password` — DEMO ONLY).

---

## The in-cluster issuer convention

Dex's `issuer` (the single source of truth) is:

```
http://dex.ocloud-system.svc.cluster.local:5556/dex
```

- `dex.ocloud-system.svc.cluster.local` — the `dex` Service in the
  `ocloud-system` namespace (matches the core-component namespace convention).
- `:5556` — Dex HTTP port (telemetry/health is `:5558`).
- `/dex` — Dex's conventional path prefix. The OIDC discovery document is at
  `<issuer>/.well-known/openid-configuration`, i.e.
  `http://dex.ocloud-system.svc.cluster.local:5556/dex/.well-known/openid-configuration`.

The issuer string MUST be byte-identical everywhere it appears: Dex's own
config, the `iss` claim Dex stamps into every token, and the value clients
discover against. Change it in exactly one place per consumer (below).

---

## How o2-dms points at Dex (P13-T-201 + T202 wiring)

The o2-dms adapter's `OIDCValidator` (P13-T-201) reads two env vars at startup
(`operators/o2-dms-adapter/cmd/main.go`):

| Env var (code reads) | Value for Dex | Meaning |
|---|---|---|
| `O2DMS_OIDC_ISSUER` | `http://dex.ocloud-system.svc.cluster.local:5556/dex` | OIDC discovery base. **Set ⇒ OIDCValidator is the active validator.** |
| `O2DMS_OIDC_AUDIENCE` | `o2-dms` | The `aud` claim the validator enforces (= the Dex static-client `id`). |
| `O2DMS_OIDC_ALLOWED_CLAIMS` | *(optional)* e.g. `groups=o2-admin` | Extra claim constraints. |

These are wired through the o2-dms helm chart values on the **real profile**
(`deploy/profiles/real/o2-dms-adapter.values.yaml`):

```yaml
auth:
  mode: oidc
  oidc:
    issuerUrl: http://dex.ocloud-system.svc.cluster.local:5556/dex
    audience: o2-dms
  tokenReview:
    rbac: true   # in-cluster TokenReview fallback path (see below)
```

### ⚠ Chart env-name drift to fix (flagged for the main agent / T201 follow-up)

The o2-dms chart `templates/deployment.yaml` currently emits the OIDC issuer as
**`O2DMS_OIDC_ISSUER_URL`**, but the T201 code reads **`O2DMS_OIDC_ISSUER`**
(no `_URL`). With the chart as-is, setting `auth.oidc.issuerUrl` does NOT
activate the OIDCValidator — the code falls through to the in-cluster
TokenReview validator. `deployment.yaml` is **outside P13-T-202's Allowed
Paths** (T202 owns only `rbac.yaml` under the charts), so this README documents
the gap rather than silently editing the deployment template. **The chart's
env key must be renamed `O2DMS_OIDC_ISSUER_URL → O2DMS_OIDC_ISSUER`** (and add
`O2DMS_OIDC_ALLOWED_CLAIMS`) for the Dex/OIDC path to actually engage — a
one-line T201 follow-up or a separately-scoped chart fix.

### Why `tokenReview.rbac: true` is still set on the real profile

P13-T-201 made `K8sTokenReviewValidator` the **in-cluster primary path** — it
is the active validator whenever `O2DMS_OIDC_ISSUER` is unset (or while the
above drift is unfixed). So the o2-dms ServiceAccount needs
`authentication.k8s.io/v1 tokenreviews: create` regardless of whether OIDC is
also wired. The chart grants exactly that one verb (least-privilege; not the
broad built-in `system:auth-delegator` ClusterRole) via
`auth.tokenReview.rbac=true` — see
`deploy/helm-charts/o2-dms-adapter/templates/rbac.yaml`.

---

## How the backend relates to Dex

The demo-backend's only outbound auth today is its Prometheus scrape, which
P13-T-201 switched to an **in-cluster ServiceAccount token** (not an OIDC token
from Dex) — so the backend does not need a Dex client for its current paths.
The `ocloud-backend` static client is registered in Dex as the **audience** for
any future user-facing token validation the backend may add (e.g. a UI login
flow). It is provisioned now so the swap point is complete; nothing in the
current backend consumes it yet.

---

## Swapping in your own IdP

The reference exists to be replaced. To point O-Cloud at the customer IdP:

1. **Do NOT deploy `dex.yaml`.** Skip this directory entirely.
2. **Register two clients** (or reuse existing audiences) in the customer IdP,
   one whose audience O-Cloud's O2 DMS will enforce. Note that audience value.
3. **Re-point the issuer + audience** on the real profile values:
   - `deploy/profiles/real/o2-dms-adapter.values.yaml`:
     - `auth.oidc.issuerUrl` → the customer IdP issuer (e.g.
       `https://login.example.com/realms/ocloud`),
     - `auth.oidc.audience` → the customer audience for O2 DMS.
   - (and apply the `O2DMS_OIDC_ISSUER` chart env fix above so OIDC engages).
4. **Client secrets** come from the customer IdP. P13-T-203
   (Vault / external-secrets) replaces the reference `dex-client-secrets`
   Secret with an `ExternalSecret`; for a customer IdP, wire the customer's
   client secret through the same Vault path. **Never commit a real client
   secret to this repo** (root `CLAUDE.md` §6).
5. **TLS / Ingress** are the customer IdP's responsibility — the reference is
   in-cluster HTTP only. A real `https://` issuer needs the validator to trust
   the IdP's CA (standard system trust store or a mounted CA bundle).

The only O-Cloud code touchpoint is the issuer URL + audience — the
`OIDCValidator` does standard OIDC discovery + JWKS + JWT signature/`iss`/`aud`/
`exp` verification against whatever issuer it is pointed at. No code change is
required to swap IdPs.

---

## Offline verification (no cluster)

```bash
# YAML well-formedness (all 5 docs + the embedded Dex config block):
python -c "import yaml; list(yaml.safe_load_all(open('deploy/idp/dex.yaml', encoding='utf-8')))"

# Kubernetes schema validity (offline, against built-in schemas):
kubeconform -summary -kubernetes-version 1.31.0 deploy/idp/dex.yaml
#   → 5 resources, Valid: 5

# Structural client dry-run (needs a reachable cluster for REST-mapping;
# on a real cluster this is the T301 lab step):
kubectl apply -n ocloud-system --dry-run=server -f deploy/idp/dex.yaml
```

## End-to-end (lab-gated · P13-T-301)

A live token round-trip — obtain a Dex token for the `o2-dms` audience and
watch o2-dms's `OIDCValidator` accept it — is the **T301 real-cluster
integration stamp**, not done here. The offline layer proves the manifests are
schema-valid and the issuer/audience wiring is internally consistent; the lab
proves the JWKS discovery + JWT validation actually round-trips.
