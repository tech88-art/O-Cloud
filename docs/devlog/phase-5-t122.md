# Phase 5 · T-122 · inference-operator chart: bundle ModelService CRD

**Date**: 2026-05-20
**Trigger**: kind smoke CI iteration after T121

## Failure observed

After T121 push (RBAC for resourceslices), `kind smoke (real cluster)` advanced
past every prior failure point:

```
✓ seed pool CRDs + mock workload
✓ wait for NPUSlicePool reconcile (totalSlices > 0)     15s   ← T121 fix worked
✓ Phase 5 — install cert-manager (P5-T-106)              3s
✓ Phase 5 — build + load inference-operator image       1m11s
✓ Phase 5 — install inference-operator chart            15s
✗ Phase 5 — apply ModelService + assert PD pair          1s
```

Failure body:

```
== apply ModelService fixture ==
error: resource mapping not found for name: "smoke-ms"
  namespace: "ocloud-system" from ".../modelservice-sample.yaml":
  no matches for kind "ModelService" in version
  "inference.ocloud.edge.example.com/v1alpha1"
ensure CRDs are installed first
```

## Root cause

`deploy/helm-charts/inference-operator/` shipped without a `crds/` directory:

```
deploy/helm-charts/inference-operator/
├── Chart.yaml
├── README.md
├── templates/
│   ├── _helpers.tpl
│   ├── certificate.yaml
│   ├── deployment.yaml
│   ├── issuer.yaml
│   ├── mutatingwebhookconfiguration.yaml
│   ├── rbac.yaml
│   ├── service.yaml
│   └── serviceaccount.yaml
└── values.yaml
```

The Helm convention for v3 is: any YAML inside `<chart>/crds/` is applied
automatically by `helm install` before templates are rendered. With no
`crds/` directory, the chart installed the controller Deployment +
webhook plumbing but never registered the `ModelService` CRD.

Apply step then failed instantly (1s) because the K8s API server's
discovery cache had no mapping for `inference.ocloud.edge.example.com/v1alpha1`.

Same shape as T118 (NPUSliceAllocation missing from npu-dra-driver chart)
— both were Phase 5 W1 scaffold-task oversights where the chart
maintainer forgot the CRD bundling step.

## Fix

Mirror the T118 pattern exactly:

```
mkdir -p deploy/helm-charts/inference-operator/crds
cp operators/inference-operator/config/crd/bases/\
   inference.ocloud.edge.example.com_modelservices.yaml \
   deploy/helm-charts/inference-operator/crds/modelservices.yaml
```

The source CRD is the controller-gen v0.20.1 output (matches the pin in
`operators/inference-operator/Makefile`); no manual editing.

## Verify

- `ls deploy/helm-charts/inference-operator/crds/` → `modelservices.yaml`
- File header matches generated CRD (apiVersion / kind / group / kind: ModelService)
- 237-line file copied intact (10458 bytes)

CI verification (next run): the assert step should advance past
`kubectl apply -f .../modelservice-sample.yaml` and start checking the
PD-pair status assertion.

## Notes

- No code/controller change needed — the controller body (T006-T008) is
  already correct; this is purely a packaging defect in the chart.
- Sibling oversight: also re-verified npu-dra-driver/crds/ is in place
  (it is, from T118). No other charts in `deploy/helm-charts/` need CRDs.
- Devlog count after T122: Phase 5 task series now at 22 sequential
  devlogs (T001-T107 plan tasks + T108-T122 CI iteration fixes).
