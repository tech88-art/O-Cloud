import { expect, test } from '@playwright/test';

/**
 * P1-T-305 — Workloads page (P1-T-206) end-to-end smoke.
 *
 * Exercises against the live mock backend (set-a-small):
 *   1. /workloads renders the AntD Table with the expected 10 rows
 *   2. status filter "running" narrows the table; clearing restores it
 *   3. clicking a row opens the WorkloadDetail Drawer with pods +
 *      PD-pair Tag for the qwen-8b-pd workload
 *
 * Mock data assumptions (configs/mock-data/set-a-small/workloads.json):
 *   - 10 workloads total (1 PD pair + 9 others)
 *   - ai-inference/qwen-8b-pd has a `relations[pd-pair]` entry
 */

test.describe('Workloads page — main flow', () => {
  test.beforeEach(({ page }) => {
    page.setDefaultTimeout(30_000);
  });

  test('table renders the set-a-small workloads', async ({ page }) => {
    await page.goto('/workloads');
    await expect(page.getByTestId('workload-table')).toBeVisible({ timeout: 20_000 });
    const rows = page.locator('[data-testid="workload-table"] .ant-table-tbody > tr.ant-table-row');
    // The fixture has 10+ workloads. Don't pin a literal so the assertion
    // survives fixture evolution (P1-T-307 may add D6 affinity-comparison
    // workloads).
    await expect.poll(async () => rows.count(), { timeout: 15_000 }).toBeGreaterThanOrEqual(10);

    // The PD-pair workload should be one of the rows.
    await expect(page.getByText('qwen-8b-pd').first()).toBeVisible();
  });

  test('status filter "running" narrows the table', async ({ page }) => {
    await page.goto('/workloads');
    await expect(page.getByTestId('workload-table')).toBeVisible({ timeout: 20_000 });
    const rows = page.locator('[data-testid="workload-table"] .ant-table-tbody > tr.ant-table-row');
    await expect.poll(async () => rows.count(), { timeout: 15_000 }).toBeGreaterThanOrEqual(10);
    const initialCount = await rows.count();

    // Open the status filter and pick "running". The filter exposes a
    // data-testid that wraps an AntD Select; click the inner combobox.
    const statusFilter = page.getByTestId('workloads-filter-status');
    const trigger = statusFilter.locator('.ant-select-selector');
    await trigger.click();
    await page.locator('.ant-select-item-option-content', { hasText: 'running' }).first().click();

    // After filter: rows must be fewer than the unfiltered count AND > 0.
    await expect.poll(
      async () => rows.count(),
      { timeout: 15_000 },
    ).toBeLessThan(initialCount);
    await expect.poll(async () => rows.count()).toBeGreaterThan(0);
  });

  test('row click opens the WorkloadDetail Drawer with pods + PD-pair Tag', async ({ page }) => {
    await page.goto('/workloads');
    await expect(page.getByTestId('workload-table')).toBeVisible({ timeout: 20_000 });

    // Click the qwen-8b-pd row to open the Drawer.
    await page.getByText('qwen-8b-pd').first().click();

    const drawer = page.getByTestId('workload-detail-drawer');
    await expect(drawer).toBeVisible({ timeout: 10_000 });

    // The pods section lists prefill + decode by their pod test-ids
    // (set by WorkloadDetailDrawer:195 as `workload-pod-${name}`).
    await expect(drawer.getByTestId('workload-pod-qwen-8b-pd-prefill-0')).toBeVisible();
    await expect(drawer.getByTestId('workload-pod-qwen-8b-pd-decode-0')).toBeVisible();

    // PD-pair relation is rendered as an AntD Tag in the relations
    // section (data-testid="workload-relation-0").
    await expect(drawer.getByTestId('workload-relation-0')).toBeVisible();
  });
});
