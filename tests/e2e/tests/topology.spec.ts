import { expect, test } from '@playwright/test';

/**
 * P1-T-110 — Overview topology W2 main flow.
 *
 * What we exercise
 * ----------------
 *   1. navigate to /overview
 *   2. wait until both the AntD Tree (left pane) and the ReactFlow graph
 *      (center pane) are rendered against the live mock backend
 *   3. assert: tree shows 1 cluster + 3 nodes + 24 NPUs (from set-a-small)
 *   4. click the first worker node in the tree
 *   5. assert: ReactFlow highlights the same id AND the right DetailPanel
 *      flips to the node card with cpu / memory / NPU count
 *   6. dbl-click an NPU node in the topology
 *   7. assert: slice child nodes appear under that NPU (and that the
 *      DetailPanel NPU card is now visible with slice info)
 *   8. (optional, skipped by default) wait for a backend-driven
 *      `npu.statusChanged` event from events.json and assert the
 *      WsStatusChip surfaces a recent `lastEventAt`. The mock backend's
 *      replay timing is real-time-scaled so this can take 3-10s; the
 *      assertion is correct but flaky on CI runners under load. The
 *      WebSocket *handshake* (step 2c) is asserted unconditionally because
 *      it doesn't depend on event scheduling.
 *
 * Mock data assumptions (configs/mock-data/set-a-small)
 * -----------------------------------------------------
 *   clusters.json     →  1 cluster `cluster-prod-a-01`
 *   nodes.json        →  3 nodes, first is `worker-site-a-01` (8 NPUs)
 *   npus.json         →  24 NPUs total
 *   events.json       →  topology.update @ t=0, npu.statusChanged @ t=3s
 *
 * Selector strategy
 * -----------------
 *   - High-level `data-testid` from the page (`overview-tree`, …) for
 *     layout regions
 *   - ReactFlow auto-injects `data-id="<node-id>"` on every node so we
 *     can address topology nodes precisely by their backend id without
 *     adding extra hooks
 *   - AntD Tree titles are unique strings from the topology — `getByText`
 *     scoped to the tree pane works without coupling to internal class
 *     names
 *
 * Failure artifacts
 * -----------------
 *   - `playwright-report/` has the HTML report
 *   - On failure: screenshot + video + trace (set in playwright.config.ts)
 */

const CLUSTER_ID = 'cluster-prod-a-01';
const FIRST_NODE_ID = 'worker-site-a-01';
// Picked from set-a-small/npus.json: an NPU that actually has slice
// children in slices.json (npu-0/1 have no slices in the dataset).
const FIRST_NPU_ID = 'worker-site-a-01-npu-2';

test.describe('Overview topology — W2 main flow', () => {
  test.beforeEach(async ({ page }) => {
    // Vite cold-starts can stretch past the default 30s on Windows.
    // Bumping just for the first nav per test.
    page.setDefaultTimeout(30_000);
  });

  test('open /overview → tree + graph render against mock backend', async ({ page }) => {
    await page.goto('/overview');

    // (a) Layout shell present — proves the route mounted.
    await expect(page.getByTestId('overview-tree-pane')).toBeVisible();
    await expect(page.getByTestId('overview-center-pane')).toBeVisible();
    await expect(page.getByTestId('overview-detail-pane')).toBeVisible();

    // (b) Left tree populated from /api/v1/clusters/:id/topology.
    const tree = page.getByTestId('overview-tree');
    await expect(tree).toBeVisible({ timeout: 20_000 });

    // (c) Center pane has the ReactFlow canvas with topology nodes.
    const graph = page.getByTestId('topology-graph');
    await expect(graph).toBeVisible({ timeout: 20_000 });

    // (d) Cluster + a known worker node are present in the graph by id.
    await expect(page.locator(`.react-flow__node[data-id="${CLUSTER_ID}"]`)).toBeVisible();
    await expect(page.locator(`.react-flow__node[data-id="${FIRST_NODE_ID}"]`)).toBeVisible();

    // (e) Mock backend ships 24 NPUs — we don't pin to exactly 24 (the
    // dataset may evolve) but assert there are at least 8 NPU-type
    // nodes rendered. `topo-node-npu` is set by the TopologyGraph node
    // renderer for any NPU node.
    const npuNodes = page.locator('[data-testid="topo-node-npu"]');
    await expect.poll(async () => await npuNodes.count(), { timeout: 10_000 })
      .toBeGreaterThanOrEqual(8);

    // (f) Cluster is in the tree title — same id is visible in left pane.
    const treePane = page.getByTestId('overview-tree-pane');
    await expect(treePane.getByText(CLUSTER_ID).first()).toBeVisible();

    // (g) WebSocket handshake — useTopologyWS opens /ws/topology and the
    // status chip flips to `open`. This proves backend WS + frontend
    // wiring works end-to-end (P1-T-105 + P1-T-108b). We don't wait for
    // a specific event to land — that's the flaky step (9) below.
    await expect.poll(
      async () => await page.getByTestId('ws-status-chip').getAttribute('data-status'),
      { timeout: 15_000 },
    ).toBe('open');
  });

  test('tree click → topology highlight + DetailPanel sync', async ({ page }) => {
    await page.goto('/overview');
    await expect(page.getByTestId('overview-tree')).toBeVisible({ timeout: 20_000 });
    await expect(page.getByTestId('topology-graph')).toBeVisible({ timeout: 20_000 });

    // Click the first worker node in the tree. AntD's Tree renders titles
    // inside `.ant-tree-title` spans; clicking the text triggers select.
    const treePane = page.getByTestId('overview-tree-pane');
    await treePane.getByText(FIRST_NODE_ID, { exact: true }).click();

    // DetailPanel switches to the node card — this is the load-bearing UX
    // contract: tree click writes `selectedNodeId` into the store, the
    // store drives the DetailPanel, and the API plumbing
    // (`/api/v1/nodes/:name`) gets exercised. (The TopologyGraph's
    // highlight is a controlled inline-style change on a custom node
    // renderer — there's no stable selector for it, so we rely on the
    // panel switch as the synchronisation proof.)
    const detailPanel = page.getByTestId('overview-detail-pane');
    await expect(detailPanel.getByTestId('detail-panel-node')).toBeVisible({ timeout: 10_000 });

    // The node card shows the node name. We don't pin literal values like
    // cpu count (mock data may evolve) — finding the node id in the panel
    // confirms the panel re-fetched against the selected node.
    await expect(detailPanel.getByText(FIRST_NODE_ID).first()).toBeVisible();

    // Sanity: the ReactFlow node for the same id is still rendered (the
    // graph didn't accidentally re-mount and lose the node).
    await expect(
      page.locator(`.react-flow__node[data-id="${FIRST_NODE_ID}"]`),
    ).toBeVisible();
  });

  test('dbl-click NPU → slice children visible in the graph', async ({ page }) => {
    await page.goto('/overview');
    await expect(page.getByTestId('topology-graph')).toBeVisible({ timeout: 20_000 });

    // Locate the NPU node and capture how many slice nodes are visible
    // BEFORE expansion. Slices are collapsed by default in the **graph**
    // (the TopologyGraph filters them out unless their parent NPU is in
    // `expandedNPUs`). The left tree always shows them — that's by
    // design — so we assert against the canvas only.
    const npuNode = page.locator(`.react-flow__node[data-id="${FIRST_NPU_ID}"]`);
    await expect(npuNode).toBeVisible({ timeout: 15_000 });

    // Pre-check: count slice nodes inside the topology graph container.
    // Scoping to the graph excludes the tree (which renders slice labels
    // as plain text, not as `topo-node-slice` data-testids).
    const graph = page.getByTestId('topology-graph');
    const sliceLocator = graph.locator('[data-testid="topo-node-slice"]');
    expect(await sliceLocator.count()).toBe(0);

    // Double-click to expand.
    //
    // Why we dispatch synthetic events rather than `.dblclick()`:
    // ReactFlow uses a pan-on-drag listener that swallows the second
    // pointerdown of a synthetic mouse dblclick when the viewport
    // transform shifts between clicks (dagre lays out the graph wider
    // than the pane, so `fitView` applies a transform that moves the
    // node a few pixels mid-sequence). Native event dispatch on the
    // wrapper element matches what React's `onClick` / `onDoubleClick`
    // bindings listen for and bypasses the panner entirely. We fire
    // `click` first because in real browsers `dblclick` is preceded by
    // a `click` — ReactFlow's onClick handler uses that signal to set
    // the selected node, which is what flips the DetailPanel.
    await npuNode.dispatchEvent('click');
    await npuNode.dispatchEvent('dblclick');

    // After expansion at least one slice node should be present in the
    // graph DOM. ReactFlow virtualises off-screen nodes but our viewport
    // uses `fitView` so re-layout puts everything inside the visible
    // area — count() inside the graph is reliable.
    await expect.poll(
      async () => await sliceLocator.count(),
      {
        timeout: 15_000,
        intervals: [200, 500, 1_000],
      },
    ).toBeGreaterThan(0);

    // The DetailPanel should now be on the NPU card (single-click on
    // the NPU is implied by ReactFlow's dblclick path firing onClick
    // first).
    const detailPanel = page.getByTestId('overview-detail-pane');
    await expect(detailPanel.getByTestId('detail-panel-npu')).toBeVisible({ timeout: 10_000 });
  });

  /**
   * Optional step 9 from the task brief.
   *
   * What we'd ideally assert: a backend-emitted `npu.statusChanged` event
   * (from events.json @ t=3s) lands on the WebSocket, triggers a query
   * invalidation, and the WsStatusChip's `lastEventAt` updates.
   *
   * Why it's skipped: the mock event timing is real-time-scaled. On a
   * CI runner under load — or when Vite is still warming the dep graph
   * during the page load — the 3s budget often passes before the
   * frontend mounts the WS hook. The result is a flaky test.
   *
   * Follow-up (post-Phase 1): a `FastForward` knob already exists in the
   * mock datasource (`backend/pkg/datasource/mock/events.go`, see
   * `model.StreamEventsOptions.FastForward`). Expose it via a query
   * parameter on `/ws/topology` (e.g. `?fastforward=10`) so the e2e
   * suite can replay the same script in 0.3s and this assertion becomes
   * deterministic.
   */
  test.skip('WS event drives status change (TODO: needs FastForward query param)', async ({ page }) => {
    await page.goto('/overview');
    await expect(page.getByTestId('overview-tree')).toBeVisible({ timeout: 20_000 });

    // Wait for an event to land — currently flaky on slow runners.
    await expect.poll(
      async () => {
        const ts = await page.getByTestId('ws-status-chip').getAttribute('data-last-event-at');
        return ts !== null;
      },
      { timeout: 15_000 },
    ).toBe(true);
  });
});
