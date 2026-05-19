# P4-T-101 · npu-dra-driver Dockerfile + Helm chart skeleton

- **Commit**: f10a1dd
- **Date**: 2026-05-19
- **Duration**: plan 1d · actual ~1.5h

## Intent

Replace the T003 Dockerfile "scaffold stub" header with a "T101 multi-stage" header (build logic was already correct; only labels + comments change) and create the full Helm chart at `deploy/helm-charts/npu-dra-driver/` — Chart.yaml + values.yaml + 5 templates + files/npus.json + .helmignore + README.md.

## Path adaptations

None — paths followed plan literal.

## Debugging trail

- **`.Files.Get` only reads files inside the chart directory**. Tried `.Files.Get "../../../configs/mock-data/set-a-small/npus.json"` first — Helm refused (Files.Get is bounded to chart-relative). Resolution: created `deploy/helm-charts/npu-dra-driver/files/` directory, copied `npus.json` into it at chart-build time. Documented the convention in chart README: chart-time snapshot, not runtime symlink. Phase 5 if the mock evolves, `cp configs/mock-data/set-a-small/npus.json deploy/helm-charts/npu-dra-driver/files/npus.json` is the resync command (could become a `Makefile chart-package` target).
- **helm lint --strict INFO-level "icon recommended"**. Non-blocking — the lint output says "0 chart(s) failed" even with the strict flag. helm-lint CI workflow (P3-T-103) treats failure exit code = job failure; INFO-level doesn't fail. Confirmed by running the chart against `--strict` and matching the existing ascend-npu-exporter-plus chart's posture (which also has the same INFO).
- **kubectl apply --dry-run=client without live cluster**. Plan acceptance bullet: `helm template ... | kubectl apply --dry-run=client -f -` succeeds. Locally fails because kubectl can't reach openapi schemas without a live cluster context — error: "couldn't get current server API group list". Tried `--validate=false` → still errors out (kubectl needs the live API for `unable to recognize "STDIN"`). Documented in commit footer: this is expected outside kind, P4-T-104 e2e-kind validates end-to-end.

## Key decisions

- **OCI labels on Dockerfile** (Phase 4 hygiene): `org.opencontainers.image.title/description/source/licenses`. Mirrors what good open-source images carry; kubectl describe / docker inspect surfaces image provenance.
- **Pod-level securityContext hardening**: `runAsNonRoot: true` + `runAsUser/Group: 65532` + `seccompProfile: RuntimeDefault` + container `allowPrivilegeEscalation: false` + `drop: [ALL]` + `readOnlyRootFilesystem: true`. Phase 4 simulator has no need for any escalated privilege; setting the floor here means Phase 5 has to consciously relax (not consciously tighten).
- **ConfigMap mount path = `/etc/npu-dra-driver/mock/npus.json`** matching the manager's `--mock-data-path` flag default. The values.yaml's `publisher.mockDataPath` is the single source of truth — Deployment template references `.Values.publisher.mockDataPath`.
- **ClusterRole pre-grants resourceclaims/status verbs + deviceclasses get/list/watch**. Phase 4 doesn't use them (annotations-only path + no DeviceClass registered yet), but pre-granting avoids a chart bump when Phase 5 lifts to status writes + registers the DeviceClass. Documented inline.

## Verification

P3 三维度:
- Existence: `git ls-files deploy/helm-charts/npu-dra-driver/` → 10 files
- Completeness: `helm lint --strict deploy/helm-charts/npu-dra-driver/` → 1 linted, 0 failed
- Correctness: `helm template test deploy/helm-charts/npu-dra-driver/` renders 5 K8s objects (SA + ConfigMap with embedded 544-line npus.json + ClusterRole + ClusterRoleBinding + Deployment); rendered Deployment args include `--enable-publisher --mock-data-path=/etc/npu-dra-driver/mock/npus.json --enable-claim-controller` (T005+T006 flags wired correctly)

## Carry-forward

- T104 wires `tests/e2e/kind/install.sh up` to `helm upgrade --install npu-dra-driver` against this chart. Image tag override via `--set image.tag=$(echo "${NPU_DRA_IMG}" | cut -d: -f2)`.
- helm-lint CI workflow (P3-T-103) auto-covers this chart on every PR — no workflow change needed.
- Phase 5 chart bump: add `templates/deviceclass.yaml` for DeviceClass registration; update values.yaml with sub-class names. Major-version bump of chart.appVersion when claim controller lifts to status writes.
