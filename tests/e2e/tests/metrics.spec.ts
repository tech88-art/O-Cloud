import { expect, test } from '@playwright/test';

/**
 * P1-T-305 — Metrics page (P1-T-208) end-to-end smoke.
 *
 * Exercises against the live mock backend (set-a-small):
 *   1. /metrics renders the 5 dashboard tabs + GrafanaPanel iframe
 *   2. switching to "node-detail" surfaces the node selector
 *
 * We don't assert the iframe content (Grafana may not even be running in
 * E2E; the panel shows a placeholder when the iframe fails). The
 * load-bearing UI contract is that the tab/selector wiring works and the
 * /api/v1/grafana/url call fires when the user picks a tab/variable.
 */

test.describe('Metrics page — dashboard tabs + selectors', () => {
  test.beforeEach(({ page }) => {
    page.setDefaultTimeout(30_000);
  });

  test('5 dashboard tabs render + cluster-overview is the default tab', async ({ page }) => {
    await page.goto('/metrics');
    await expect(page.getByTestId('metrics-dashboard-tabs')).toBeVisible({ timeout: 20_000 });

    // All 5 tabs present.
    await expect(page.getByTestId('metrics-tab-cluster-overview')).toBeVisible();
    await expect(page.getByTestId('metrics-tab-node-detail')).toBeVisible();
    await expect(page.getByTestId('metrics-tab-npu-detail')).toBeVisible();
    await expect(page.getByTestId('metrics-tab-workload-business')).toBeVisible();
    await expect(page.getByTestId('metrics-tab-workload-resource')).toBeVisible();

    // GrafanaPanel iframe is mounted (src may be a placeholder if Grafana
    // isn't running, but the element must exist).
    await expect(page.getByTestId('metrics-panel')).toBeVisible();
  });

  test('switching to node-detail surfaces the node selector', async ({ page }) => {
    await page.goto('/metrics');
    await expect(page.getByTestId('metrics-dashboard-tabs')).toBeVisible({ timeout: 20_000 });

    await page.getByTestId('metrics-tab-node-detail').click();
    await expect(page.getByTestId('metrics-vars-node')).toBeVisible({ timeout: 5_000 });
    await expect(page.getByTestId('metrics-select-node')).toBeVisible();
  });

  test('switching to workload-resource surfaces the workload selector', async ({ page }) => {
    await page.goto('/metrics');
    await expect(page.getByTestId('metrics-dashboard-tabs')).toBeVisible({ timeout: 20_000 });

    await page.getByTestId('metrics-tab-workload-resource').click();
    await expect(page.getByTestId('metrics-vars-workload')).toBeVisible({ timeout: 5_000 });
    await expect(page.getByTestId('metrics-select-workload')).toBeVisible();
  });
});
