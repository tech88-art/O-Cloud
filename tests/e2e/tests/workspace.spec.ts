import { expect, test } from '@playwright/test';

/**
 * P12-T-301 — one-page workspace main flow (replaces the 5 per-page specs:
 * topology / workloads / deploy / metrics / logs).
 *
 * Phase 12 (ADR-0022) collapsed the 5 routes into a single workspace at
 * `/overview`: a top preset bar, a left resource tree, a center ReactFlow
 * topology, and a right DetailPanel that absorbs workload info + metrics +
 * logs sections. This spec exercises that flow end-to-end against the live
 * mock backend (set-a-small):
 *
 *   1. Shell + topology render (tree + graph + preset bar + WS handshake)
 *   2. Tree select → DetailPanel sync (resource card)
 *   3. NPU select → NPU card (PCIe) + hardware metrics section, NO logs
 *   4. dbl-click NPU → slice children appear in the graph
 *   5. Fabric toggle → bandwidth (network/hccs) edges render (BandwidthEdge)
 *   6. Focus → select a resource + focus button isolates its subtree
 *   7. Preset bar → click a preset chip → deploy wizard opens
 *   8. Deep-link → a retired route (/workloads) redirects to the workspace
 *   9. WS fastforward replay drives a lastEventAt update
 *
 * Mock data (configs/mock-data/set-a-small): 1 cluster, 3 nodes, 24 NPUs.
 * Selectors: page `data-testid` for regions; ReactFlow `data-id` for nodes.
 */

const CLUSTER_ID = 'cluster-prod-a-01';
const FIRST_NODE_ID = 'worker-site-a-01';
// An NPU that has slice children in set-a-small/slices.json (npu-0/1 don't).
const FIRST_NPU_ID = 'worker-site-a-01-npu-2';

test.describe('One-page workspace — main flow', () => {
  test.beforeEach(async ({ page }) => {
    page.setDefaultTimeout(30_000);
  });

  test('workspace shell renders: preset bar + tree + graph + WS handshake', async ({ page }) => {
    await page.goto('/overview');

    // Single-page workspace regions (ADR-0022 §2.1).
    await expect(page.getByTestId('preset-bar')).toBeVisible({ timeout: 20_000 });
    await expect(page.getByTestId('overview-tree-pane')).toBeVisible();
    await expect(page.getByTestId('overview-center-pane')).toBeVisible();
    await expect(page.getByTestId('overview-detail-pane')).toBeVisible();

    // Tree + graph populated from /api/v1/clusters/:id/topology.
    await expect(page.getByTestId('overview-tree')).toBeVisible({ timeout: 20_000 });
    await expect(page.getByTestId('topology-graph')).toBeVisible({ timeout: 20_000 });
    await expect(page.locator(`.react-flow__node[data-id="${CLUSTER_ID}"]`)).toBeVisible();
    await expect(page.locator(`.react-flow__node[data-id="${FIRST_NODE_ID}"]`)).toBeVisible();

    // Topology EDGES render (P12-T-202 handle fix — pre-T202 zero edges drew).
    await expect.poll(
      async () => await page.locator('.react-flow__edge-path').count(),
      { timeout: 10_000 },
    ).toBeGreaterThan(0);

    const npuNodes = page.locator('[data-testid="topo-node-npu"]');
    await expect.poll(async () => await npuNodes.count(), { timeout: 10_000 })
      .toBeGreaterThanOrEqual(8);

    // At least one preset chip in the top bar.
    await expect.poll(
      async () => await page.locator('[data-testid^="preset-bar-item-"]').count(),
      { timeout: 10_000 },
    ).toBeGreaterThan(0);

    // WebSocket handshake — status chip flips to `open`.
    await expect.poll(
      async () => await page.getByTestId('ws-status-chip').getAttribute('data-status'),
      { timeout: 15_000 },
    ).toBe('open');
  });

  test('tree select → DetailPanel node card sync', async ({ page }) => {
    await page.goto('/overview');
    await expect(page.getByTestId('overview-tree')).toBeVisible({ timeout: 20_000 });
    await expect(page.getByTestId('topology-graph')).toBeVisible({ timeout: 20_000 });

    const treePane = page.getByTestId('overview-tree-pane');
    await treePane.getByText(FIRST_NODE_ID, { exact: true }).click();

    const detailPanel = page.getByTestId('overview-detail-pane');
    await expect(detailPanel.getByTestId('detail-panel-node')).toBeVisible({ timeout: 10_000 });
    await expect(detailPanel.getByText(FIRST_NODE_ID).first()).toBeVisible();
  });

  test('NPU select → NPU card with PCIe + hardware metrics section, no logs', async ({ page }) => {
    await page.goto('/overview');
    await expect(page.getByTestId('topology-graph')).toBeVisible({ timeout: 20_000 });

    // Select the NPU via its graph node. Native event dispatch — ReactFlow's
    // pan-on-drag listener swallows a synthetic `.click()` on a node.
    await page.locator(`.react-flow__node[data-id="${FIRST_NPU_ID}"]`).dispatchEvent('click');

    const detailPanel = page.getByTestId('overview-detail-pane');
    await expect(detailPanel.getByTestId('detail-panel-npu')).toBeVisible({ timeout: 10_000 });
    // PCIe surfaced in the NPU card (ADR-0021).
    await expect(detailPanel.getByText(/PCIe/).first()).toBeVisible();
    // Resource → metrics section present, logs section absent (ADR-0022 §2.3).
    await expect(detailPanel.getByTestId('metrics-section')).toBeVisible();
    await expect(detailPanel.getByTestId('logs-section')).toHaveCount(0);
  });

  test('dbl-click NPU → slice children appear in the graph', async ({ page }) => {
    await page.goto('/overview');
    await expect(page.getByTestId('topology-graph')).toBeVisible({ timeout: 20_000 });

    const npuNode = page.locator(`.react-flow__node[data-id="${FIRST_NPU_ID}"]`);
    await expect(npuNode).toBeVisible({ timeout: 15_000 });

    const graph = page.getByTestId('topology-graph');
    const sliceLocator = graph.locator('[data-testid="topo-node-slice"]');
    expect(await sliceLocator.count()).toBe(0);

    // Native event dispatch (ReactFlow's panner swallows synthetic dblclick).
    await npuNode.dispatchEvent('click');
    await npuNode.dispatchEvent('dblclick');

    await expect.poll(async () => await sliceLocator.count(), {
      timeout: 15_000,
      intervals: [200, 500, 1_000],
    }).toBeGreaterThan(0);
  });

  test('fabric toggle → bandwidth (network/hccs) edges render', async ({ page }) => {
    await page.goto('/overview');
    await expect(page.getByTestId('topology-graph')).toBeVisible({ timeout: 20_000 });

    await page.getByTestId('fabric-toggle-switch').click();

    // network + hccs + fabric-link edges render via the custom BandwidthEdge
    // (hover hit-path carries data-testid `bandwidth-edge-hit-*`).
    await expect.poll(
      async () => await page.locator('[data-testid^="bandwidth-edge-hit-"]').count(),
      { timeout: 15_000 },
    ).toBeGreaterThan(0);
  });

  test('focus button isolates the selected resource subtree', async ({ page }) => {
    await page.goto('/overview');
    await expect(page.getByTestId('topology-graph')).toBeVisible({ timeout: 20_000 });

    const nodeCountBefore = await page.locator('.react-flow__node').count();
    expect(nodeCountBefore).toBeGreaterThan(1);

    // Select a worker node (native dispatch — panner swallows .click()), then
    // focus it via the toolbar button once the selection has registered.
    await page.locator(`.react-flow__node[data-id="${FIRST_NODE_ID}"]`).dispatchEvent('click');
    await expect(page.getByTestId('topology-focus-selected')).toBeEnabled({ timeout: 10_000 });
    await page.getByTestId('topology-focus-selected').click();

    // Clear-focus button appears (focus is active) and the graph shrinks.
    await expect(page.getByTestId('topology-clear-focus')).toBeVisible({ timeout: 10_000 });
    await expect.poll(
      async () => await page.locator('.react-flow__node').count(),
      { timeout: 10_000 },
    ).toBeLessThan(nodeCountBefore);
  });

  test('preset bar chip opens the deploy wizard', async ({ page }) => {
    await page.goto('/overview');
    await expect(page.getByTestId('preset-bar')).toBeVisible({ timeout: 20_000 });

    const chip = page.locator('[data-testid^="preset-bar-item-"]').first();
    await expect(chip).toBeVisible({ timeout: 15_000 });
    await chip.click();

    // The wizard's step-mode content is the reliable "open" signal — the
    // `deploy-wizard` modal-root wrapper stays display-collapsed per AntD.
    await expect(page.getByTestId('deploy-wizard-step-mode')).toBeVisible({ timeout: 10_000 });
  });

  test('retired deep-link /workloads redirects to the workspace', async ({ page }) => {
    await page.goto('/workloads');
    // Catch-all → /overview (ADR-0022 §2.4 · no 404).
    await expect(page).toHaveURL(/\/overview$/, { timeout: 20_000 });
    await expect(page.getByTestId('preset-bar')).toBeVisible({ timeout: 20_000 });
    await expect(page.getByTestId('topology-graph')).toBeVisible({ timeout: 20_000 });
  });

  test('WS event drives status change (fastforward replay)', async ({ page }) => {
    await page.goto('/overview?ffwd=100');
    await expect(page.getByTestId('overview-tree')).toBeVisible({ timeout: 20_000 });

    await expect.poll(
      async () => {
        const ts = await page.getByTestId('ws-status-chip').getAttribute('data-last-event-at');
        return ts ?? '';
      },
      { timeout: 15_000 },
    ).not.toBe('');
  });
});
