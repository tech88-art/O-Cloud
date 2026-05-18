import { expect, test } from '@playwright/test';

/**
 * P1-T-305 — Logs page (P1-T-302) end-to-end smoke.
 *
 * Exercises against the live mock backend (set-a-small):
 *   1. /logs renders the toolbar + empty state when no workload is picked
 *   2. picking a workload triggers a REST tail fetch and renders log rows
 *   3. flipping the Live toggle ON opens a WS and rows continue to grow
 *
 * Mock backend behaviour (configs/mock-data/set-a-small/workloads.json):
 *   - GET /workloads returns 10 workloads
 *   - GET /workloads/.../logs?tail=N returns N synthesized lines
 *   - GET /ws/logs/.../... streams a new WSMessage{type:"log.line"} every 1s
 */

test.describe('Logs page — REST tail + WS stream', () => {
  test.beforeEach(({ page }) => {
    page.setDefaultTimeout(30_000);
  });

  test('toolbar renders + empty state when no workload selected', async ({ page }) => {
    await page.goto('/logs');
    await expect(page.getByTestId('logs-page')).toBeVisible({ timeout: 20_000 });
    await expect(page.getByTestId('logs-workload-select')).toBeVisible();
    await expect(page.getByTestId('logs-live-switch')).toBeVisible();

    // Before workload pick, the body is the EmptyState (data-testid="empty-state"
    // from the shared component).
    await expect(page.getByTestId('empty-state')).toBeVisible();
  });

  test('picking a workload fetches the REST tail and renders rows', async ({ page }) => {
    await page.goto('/logs');
    await expect(page.getByTestId('logs-workload-select')).toBeVisible({ timeout: 20_000 });

    // Open the AntD Select and pick the qwen-8b-pd workload via the
    // dropdown portal pattern.
    const select = page.getByTestId('logs-workload-select');
    await select.locator('.ant-select-selector').click();
    await page
      .locator('.ant-select-item-option-content', { hasText: 'ai-inference/qwen-8b-pd' })
      .first()
      .click();

    // LogViewer should fill with the default 200 lines from /logs.
    await expect(page.getByTestId('log-viewer')).toBeVisible({ timeout: 15_000 });
    const rows = page.locator('[data-testid="log-viewer-row"]');
    await expect.poll(async () => rows.count(), { timeout: 15_000 }).toBeGreaterThanOrEqual(50);
  });

  test('flipping the Live toggle opens a WS and rows continue to grow', async ({ page }) => {
    await page.goto('/logs');
    await expect(page.getByTestId('logs-workload-select')).toBeVisible({ timeout: 20_000 });

    // Pick a workload first.
    await page.getByTestId('logs-workload-select').locator('.ant-select-selector').click();
    await page
      .locator('.ant-select-item-option-content', { hasText: 'ai-inference/qwen-8b-pd' })
      .first()
      .click();
    await expect(page.getByTestId('log-viewer')).toBeVisible({ timeout: 15_000 });

    const rows = page.locator('[data-testid="log-viewer-row"]');
    const before = await rows.count();
    expect(before).toBeGreaterThan(0);

    // Flip Live ON.
    await page.getByTestId('logs-live-switch').click();

    // WS status pill flips to "open" (text varies by locale; assert on the
    // data-testid presence + that the row count grows).
    await expect(page.getByTestId('logs-ws-status')).toBeVisible();

    // Wait up to 5s for the row count to grow by at least 1 (cadence is
    // 1s per backend logs.go).
    await expect.poll(
      async () => rows.count(),
      {
        timeout: 5_000,
        intervals: [500, 1_000, 2_000],
      },
    ).toBeGreaterThan(before);
  });
});
