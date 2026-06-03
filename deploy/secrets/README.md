# `deploy/secrets/` — External Secrets Operator + Vault (REFERENCE)

> **This is a REFERENCE secret-management wiring, not a production secret
> backend.** It exists so the **real profile** sources every secret from an
> external manager (HashiCorp Vault) via the [External Secrets Operator][eso]
> (ESO) instead of inline/static YAML. **The customer swaps Vault** for their own
> Vault instance, or swaps the provider block entirely for AWS Secrets Manager /
> GCP Secret Manager / Azure Key Vault (all first-class ESO providers) — by
> re-pointing one `ClusterSecretStore`.
>
> Per **ADR-0025 §2 Decision B** (Secret 管理 = external-secrets + Vault) +
> **§3 right-sizing** (reference-grade — single Vault, KV v2, k8s-auth; NOT a
> multi-region HA Vault cluster) + **§4** (甲方 swap point) · 承 **ADR-0018 §4(c)**
> (BMC 动态凭据注入).

---

## What this is

| File | Purpose |
|---|---|
| `cluster-secret-store.yaml` | `ClusterSecretStore/ocloud-vault` — the single cluster-scoped binding to the Vault backend (KV v2, kubernetes auth). The one place a customer re-points. |
| `dex-client-secrets.externalsecret.yaml` | `ExternalSecret` materialising `dex-client-secrets` (Dex OIDC client secrets · P13-T-202) from Vault — **supersedes the inline demo Secret** in `deploy/idp/dex.yaml`. |
| `grafana-admin.externalsecret.yaml` | `ExternalSecret` materialising the `grafana-admin` Secret — replaces the demo `adminPassword: admin` in `deploy/single-node/values-kps.yaml` (set `grafana.admin.existingSecret=grafana-admin`). |
| `bmc-credentials.externalsecret.yaml` | `ExternalSecret` materialising per-node BMC credentials from Vault — closes the ADR-0018 §4(c) forward note (reference: node-01). |
| `README.md` | This file — reference posture + the customer swap point. |

**Right-sized (reference-grade, ADR-0025 §3):**
- **one `ClusterSecretStore`** (cluster-scoped) — not per-namespace stores;
- **Vault KV v2** at a single in-cluster address — not a multi-region / HA /
  auto-unseal Vault cluster (the customer brings platform Vault);
- **Kubernetes auth** (ESO controller SA → Vault token) — not AppRole / TLS-cert
  (a customer-policy choice; both are one-field swaps on the store).

---

## The static secrets this replaces

| Secret | Was (demo / static) | Now (real profile · Vault) |
|---|---|---|
| `dex-client-secrets` (`ocloud-system`) | inline `stringData` in `deploy/idp/dex.yaml`, labelled `DO-NOT-USE-IN-PROD` | `dex-client-secrets.externalsecret.yaml` → Vault `secret/ocloud/dex` |
| `grafana-admin` (`monitoring`) | `grafana.adminPassword: admin` in `deploy/single-node/values-kps.yaml` (demo `admin/admin`; out of T203 paths → documented swap, not edited) | `grafana-admin.externalsecret.yaml` → Vault `secret/ocloud/grafana`; real deploy sets `grafana.admin.existingSecret=grafana-admin` |
| BMC node creds (`bmc-node-01-creds`, …) | inline per-node Secret (doc default in the BMP chart values) | `bmc-credentials.externalsecret.yaml` → Vault `secret/ocloud/bmc/<node>` |
| `backend/configs/config.real.yaml` | — (no secret: Prometheus URL + Grafana base URL are **not** secrets) | unchanged — carries no secret; documented in that file's header |

> **No hardcoded secret remains on the real path.** `config.real.yaml` was
> already secret-free (a deliberate design: the backend is stateless, talks to
> in-cluster services by Service DNS, and authenticates with its projected
> ServiceAccount token — P13-T-201 — not a static bearer).

---

## Install (reference)

```bash
# One-shot via the project installer (installs ESO + applies the store +
# ExternalSecrets; assumes a reachable Vault — see prereqs below):
scripts/install.sh --with-secrets

# …or manually:
helm repo add external-secrets https://charts.external-secrets.io
helm upgrade --install external-secrets external-secrets/external-secrets \
  -n external-secrets --create-namespace --version 0.10.5 \
  --set installCRDs=true
kubectl apply -f deploy/secrets/cluster-secret-store.yaml
kubectl apply -f deploy/secrets/dex-client-secrets.externalsecret.yaml
kubectl apply -f deploy/secrets/grafana-admin.externalsecret.yaml     # if monitoring stack in use
kubectl apply -f deploy/secrets/bmc-credentials.externalsecret.yaml   # if BMP in use
```

### Vault prereqs (reference layout)

The reference `ClusterSecretStore` expects Vault with the `kubernetes` auth
method, a `secret/` KV v2 mount, and a role bound to the ESO controller SA:

```bash
vault auth enable kubernetes
vault write auth/kubernetes/role/external-secrets \
  bound_service_account_names=external-secrets \
  bound_service_account_namespaces=external-secrets \
  policies=ocloud-read ttl=1h
# Reference values — replace with real credentials from your IdP / BMCs:
vault kv put secret/ocloud/dex \
  o2dms-client-secret='<from-your-idp>' backend-client-secret='<from-your-idp>'
vault kv put secret/ocloud/grafana admin-user='admin' admin-password='<strong-pass>'
vault kv put secret/ocloud/bmc/node-01 username='<bmc-user>' password='<bmc-pass>'
```

> ESO API version: manifests use `external-secrets.io/v1beta1` (stable across
> ESO 0.5 → 0.10; the `v1` GA alias in ESO ≥0.14 is field-identical for these
> resources). The pinned chart `0.10.5` ships both.

---

## Dex secrets: ExternalSecret vs the inline demo Secret

`deploy/idp/dex.yaml` keeps an **inline** `dex-client-secrets` Secret so the
reference IdP is self-contained for offline render + the kind smoke. On a
**Vault-backed real install**, the `ExternalSecret` here owns that same-named
Secret — so **do not also apply the inline block** (they collide on the name).
`deploy/idp/` is P13-T-202's path; this directory (P13-T-203) provides the
production override without editing the demo manifest — apply the ExternalSecret
and skip the inline Secret. (A `helm`-templated Dex would gate the inline Secret
behind `externalSecrets.enabled=false`; the raw reference manifest documents the
choice instead, per ADR-0025 §3 right-sizing.)

---

## Swapping in your own secret backend

The reference exists to be replaced. To point O-Cloud at the customer's secrets:

1. **Re-point `cluster-secret-store.yaml`** `spec.provider.vault.server` (+ CA /
   auth) at the customer Vault — **or** replace the whole `provider:` block with
   `aws:` / `gcpsm:` / `azurekv:` (ESO docs). This is the only file that changes.
2. **Keep the `ExternalSecret` files** as-is (they reference the store by name);
   adjust each `remoteRef.key` to the customer's path layout if it differs.
3. **Never commit a real secret** to this repo (root `CLAUDE.md` §6) — Vault (or
   the customer manager) is the source of truth; the cluster only ever holds the
   ESO-projected Secret, reconciled on `refreshInterval`.

[eso]: https://external-secrets.io/
