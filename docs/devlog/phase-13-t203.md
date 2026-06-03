# P13-T-203 · [A2] Vault / external-secrets — replace static secrets

- **Commit**: this commit (main agent)
- **Date**: 2026-06-03
- **Duration**: plan 2d vs actual ~0.5d (thin real secret surface; offline layer only — live Vault sync is lab-gated T301).

## Intent

Close build-doc §5 item 2 (Secret 管理) per ADR-0025 §2 Decision B + §3 right-sizing: route every
real-profile secret through the External Secrets Operator (ESO) + a Vault backend instead of
inline/static YAML, with one documented customer swap point. NOT a production secret backend
(reference-grade).

## Verify-before-propose (P3)

Grepped the whole real-path secret surface BEFORE designing, rather than assuming the plan's path
list was exhaustive. Findings drove a *smaller, more honest* delivery than "edit config.real.yaml +
every profile":
- **`config.real.yaml` is already secret-free** — Prometheus/Grafana fields are Service DNS, and the
  backend authenticates with its projected SA token (P13-T-201), not a static bearer. So there is NO
  inline token to externalise there → documented the posture in its header, did NOT fabricate a
  secret-ref change.
- **No component chart consumes a Vault secret via values** — the secret *consumers* are the raw Dex
  manifest (`deploy/idp/dex.yaml`) and the BMC `CredentialsRef` CRs, neither of which is a chart-values
  surface → NO `deploy/profiles/real/*.values.yaml` change needed (would have been padding · M4).
- **P4 horizontal scan caught a 2nd static credential** the plan didn't name: Grafana
  `adminPassword: admin` in `deploy/single-node/values-kps.yaml` → added a reference ExternalSecret +
  documented swap (see Key decisions).

## Path adaptations

- **`deploy/idp/` is P13-T-202's path, not T203's.** To externalise `dex-client-secrets` without
  editing the demo manifest, the production override lives in `deploy/secrets/` (T203 territory): an
  `ExternalSecret` owns the same-named Secret; the inline demo block stays as the offline/kind fallback,
  documented as mutually exclusive (apply one OR the other, not both — name collision). Mirrors how
  T202 documented the env-key drift instead of editing out-of-path files.
- **`deploy/single-node/values-kps.yaml` (Grafana admin) is also out of T203 paths** + it's an upstream
  chart's demo values → documented swap (`grafana.admin.existingSecret=grafana-admin`) + reference
  ExternalSecret in `deploy/secrets/`, demo default left intact for the demo profile.
- **No `helm` on the dev box** → ESO resources are CRs (not chart templates) so no helm render is
  needed; validated by `yaml.safe_load_all` + `bash -n` (install.sh). Live Vault sync = T301 lab.

## Key decisions

- **One cluster-scoped `ClusterSecretStore/ocloud-vault`** (Vault KV v2, kubernetes auth) — the single
  file a customer re-points (or swaps `provider.vault` → `aws`/`gcpsm`/`azurekv`). Right-sized: NOT a
  multi-region/HA/auto-unseal Vault (ADR-0025 §3).
- **4 reference ExternalSecrets**: `dex-client-secrets` (P13-T-202 closure), `grafana-admin` (P4 scan
  finding), `bmc-node-01-creds` (closes ADR-0018 §4(c) forward note). Each `creationPolicy: Owner`,
  `refreshInterval: 1h`, keyed at `secret/ocloud/<...>`.
- **ESO API `external-secrets.io/v1beta1`** — stable across ESO 0.5→0.10; field-identical to the `v1`
  GA alias in ESO ≥0.14. Chart pinned `0.10.5` (ships both). Documented in README.
- **`install.sh --with-secrets`** — installs ESO (idempotent helm, mirrors `--with-prometheus`/
  `--with-dra-driver` preflight) + applies the store + ExternalSecrets best-effort; warns that Vault
  itself is a prereq (store stays NotReady until a reachable Vault answers). Wired into usage + parse +
  dispatch + `--uninstall`.

## Verification (offline · plan §8 layer 1)

- `yaml.safe_load_all` on all 4 new `deploy/secrets/*.yaml` → parse clean; apiVersion/kind/name correct.
- `bash -n scripts/install.sh` → clean; `--with-secrets` present in function/usage/parse/dispatch/uninstall (grep).
- **P4 horizontal re-scan** of `deploy/ backend/configs/config.real.yaml configs/` for `stringData:`/
  hardcoded `password:`/`bearer <tok>` → only the 2 *documented demo-only* defaults remain (dex.yaml
  `DO-NOT-USE` placeholder + values-kps.yaml `admin/admin`), both with a real-path ExternalSecret + swap note.

**Real-machine (LAB-GATED · T301)**: live Vault → ESO → Secret materialisation + `kubectl get
externalsecret` SecretSynced=True is the T301 integration stamp; offline proves manifests are
schema-shaped and the wiring is internally consistent.

## Carry-forward

- **T301**: stand up a reference Vault in the lab (or point at platform Vault) + `vault kv put` the
  reference paths; verify `dex-client-secrets`/`grafana-admin` materialise and Dex/Grafana consume them.
- **T204/T205 unaffected** — no secret coupling.
- Demo profile unchanged: `--profile demo` never installs ESO (no Vault dependency for the validation台).
