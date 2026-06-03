import { expect, test } from '@playwright/test';

/**
 * P3-T-104 — three API-level smoke cases against a real kind cluster
 * provisioned by `tests/e2e/kind/install.sh` + `seed-resources.sh`.
 *
 * Chain under test
 * ----------------
 *   pool-operator Reconcile loops (T002 NPUSlicePool, T003 NPUPool,
 *   T004 NodePool)  -->  NPUSlicePool.status.totalSlices > 0  (asserted
 *   by the workflow's `wait for NPUSlicePool reconcile` step BEFORE we
 *   reach this spec, so it's already established when these cases run)
 *
 *   exporter-plus DaemonSet + simulator JSON  -->  raw Prometheus
 *   exposition format on :9100/metrics  (asserted by `exporter-plus
 *   /metrics`)
 *
 *   k8s.Source on demo-backend  -->  /api/v1/workloads returns the
 *   seeded smoke-workload Pod (asserted by `workloads list`)
 *
 *   mock-backed clusters source on demo-backend  -->  /api/v1/clusters
 *   returns a non-empty list and the routing harness is healthy
 *   (asserted by `clusters list smoke`)
 *
 * Why API smokes (and not browser)
 * --------------------------------
 *   Rendering the React frontend inside kind requires either a
 *   node+nginx image build pipeline or a serve-static sidecar. Neither
 *   adds end-to-end coverage we don't already get from the Phase 1
 *   mock-backed Playwright suite in `playwright.config.ts`. The unique
 *   value of the kind smoke is proving the *server-side* chain, which
 *   the JSON shape assertions below already cover.
 *
 * Why direct exporter scrape (and not /api/v1/metrics/query)
 * ----------------------------------------------------------
 *   The backend's prometheus datasource POSTs to
 *   /api/v1/query_range against a Prometheus *server* (not a raw
 *   exposition endpoint). Standing up Prometheus on top of kind would
 *   add a kube-prometheus-stack install — useful but materially out
 *   of scope for the chart-and-operator smoke that lives here. The
 *   honest probe of "exporter-plus emits real values" is a single
 *   GET against the exporter's NodePort, so that's what we do.
 *
 * Failure surface
 * ---------------
 *   On any assertion failure the workflow step
 *   `dump cluster on failure` (in .github/workflows/e2e-kind.yml)
 *   `kubectl describe`s the relevant CRDs and dumps the
 *   pool-operator-controller-manager logs. Playwright traces are
 *   collected on first retry.
 */

test.describe('kind smoke (real cluster + reconcile + exporter + aggregator)', () => {
  test('clusters list smoke — backend boots and routes are mounted', async ({ request }) => {
    // /api/v1/clusters is served by the mock datasource per the config
    // ConfigMap (set-a-small). Asserting >= 1 entry proves the binary
    // started, parsed config, loaded the mock fixture, and the gin
    // router mounted the v1 group. Anything earlier in the chain (image
    // pull, kind load, RBAC) would have failed the rollout-status wait
    // upstream of this test.
    const resp = await request.get('/api/v1/clusters');
    expect(resp.ok(), `clusters list non-OK: ${resp.status()}`).toBeTruthy();
    const body = await resp.json();
    expect(Array.isArray(body), `expected array, got ${typeof body}`).toBe(true);
    expect(body.length).toBeGreaterThanOrEqual(1);
    expect(body[0]).toHaveProperty('id');
    expect(typeof body[0].id).toBe('string');
  });

  test('workloads list — seeded smoke-workload Pod surfaces through k8s.Source', async ({ request }) => {
    // The seeded Pod has metadata.name=smoke-workload, requests
    // huawei.com/Ascend910B=1 (so it landed on a worker), and is in
    // namespace ocloud-system. k8s.Source materialises that as a
    // model.Workload with .name=smoke-workload .namespace=ocloud-system.
    // We poll for ~20s because Pod scheduling + status propagation
    // can lag past the initial seed-resources.sh return.
    await expect.poll(async () => {
      const resp = await request.get('/api/v1/workloads');
      if (!resp.ok()) return null;
      const list = await resp.json();
      if (!Array.isArray(list)) return null;
      return list.find(
        (w: { name?: string; namespace?: string }) =>
          w.name === 'smoke-workload' && w.namespace === 'ocloud-system',
      );
    }, {
      timeout: 30_000,
      intervals: [500, 1_000, 2_000],
      message: 'smoke-workload not surfaced by /api/v1/workloads within 30s',
    }).toBeTruthy();
  });

  test('exporter-plus emits ascend_npu_utilization_percent > 0 from the simulator', async ({ request }) => {
    // Direct scrape against the NodePort Service that install.sh
    // attaches to the exporter's DaemonSet pods. The simulator JSON
    // (exporters/ascend-npu-exporter-plus/testdata/simulator-set-a-
    // small.json) seeds 24 NPUs with non-zero utilisation, so any
    // sample > 0 confirms the simulator -> collector -> registry ->
    // /metrics chain is alive.
    const exporterBase =
      process.env.OCEDGE_KIND_EXPORTER_BASE ?? 'http://localhost:30090';
    const resp = await request.get(`${exporterBase}/metrics`);
    expect(resp.ok(), `exporter /metrics non-OK: ${resp.status()}`).toBeTruthy();
    const body = await resp.text();

    // Two-layer assertion:
    //   1. the metric family exists (catches mis-built images that
    //      don't register the collector)
    //   2. at least one sample line has a numeric value > 0 (catches
    //      simulator-disabled or empty-seed regressions)
    expect(body).toContain('ascend_npu_utilization_percent');

    const sampleLines = body
      .split('\n')
      .filter((l) => l.startsWith('ascend_npu_utilization_percent{'));
    expect(sampleLines.length, 'expected at least one ascend_npu_utilization_percent sample').toBeGreaterThanOrEqual(1);

    // Prometheus exposition format: <metric>{labels} <value> [timestamp]
    // Pluck the trailing value column from each line and assert at least
    // one is strictly positive.
    const positiveValues = sampleLines
      .map((line) => {
        const parts = line.trim().split(/\s+/);
        return Number(parts[parts.length - 1]);
      })
      .filter((v) => Number.isFinite(v) && v > 0);
    expect(positiveValues.length, `no positive samples in:\n${sampleLines.join('\n')}`).toBeGreaterThanOrEqual(1);
  });
});
