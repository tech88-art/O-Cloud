import { expect, test } from '@playwright/test';

/**
 * P1-T-305 — Deploy page (P1-T-207) end-to-end smoke.
 *
 * Exercises against the live mock backend (set-a-small):
 *   1. /deploy renders the preset grid with 4 cards
 *   2. clicking a preset opens the 2-step wizard (mode → config)
 *   3. auto-mode submit POSTs /api/v1/deploy and the wizard closes on 201
 *
 * Mock data assumptions (configs/mock-data/set-a-small/presets.json):
 *   - 4 presets: pi-3b, qwen-8b-pd, deepseek-20b, qwen-14b
 */

test.describe('Deploy page — preset grid + wizard submission', () => {
  test.beforeEach(({ page }) => {
    page.setDefaultTimeout(30_000);
  });

  test('preset grid renders 4 cards', async ({ page }) => {
    await page.goto('/deploy');
    await expect(page.getByTestId('preset-grid')).toBeVisible({ timeout: 20_000 });

    // Card test-ids are `preset-card-${preset.id}` — the 4 ids are
    // documented in mock-data/set-a-small/presets.json.
    await expect(page.getByTestId('preset-card-pi-3b')).toBeVisible();
    await expect(page.getByTestId('preset-card-qwen-8b-pd')).toBeVisible();
    await expect(page.getByTestId('preset-card-deepseek-20b')).toBeVisible();
    await expect(page.getByTestId('preset-card-qwen-14b')).toBeVisible();
  });

  test('clicking a preset opens the 2-step wizard', async ({ page }) => {
    await page.goto('/deploy');
    await expect(page.getByTestId('preset-grid')).toBeVisible({ timeout: 20_000 });

    await page.getByTestId('preset-card-pi-3b').click();

    // AntD Modal root (`data-testid="deploy-wizard"`) carries visibility
    // toggles asymmetrically (the root wrapper stays in the DOM with
    // display:none when closed and AntD's open animation flips that
    // asynchronously). Asserting on a child step element that's only
    // mounted while the wizard is open is the more reliable check.
    await expect(page.getByTestId('deploy-wizard-step-mode')).toBeVisible({ timeout: 10_000 });
    await expect(page.getByTestId('deploy-wizard-mode-auto')).toBeVisible();
    await expect(page.getByTestId('deploy-wizard-mode-manual')).toBeVisible();

    // Advance to step 1: replicas + namespace form.
    await page.getByTestId('deploy-wizard-next').click();
    await expect(page.getByTestId('deploy-wizard-step-config')).toBeVisible({ timeout: 5_000 });
    await expect(page.getByTestId('deploy-wizard-replicas')).toBeVisible();
    await expect(page.getByTestId('deploy-wizard-namespace')).toBeVisible();
  });

  test('auto-mode submit POSTs /deploy and closes the wizard', async ({ page }) => {
    // Wait for the deploy POST to fire — Playwright's request interception
    // lets us inspect what the wizard sent without re-asserting on UI.
    let postSent = false;
    page.on('request', (req) => {
      if (req.method() === 'POST' && req.url().includes('/api/v1/deploy')) {
        postSent = true;
      }
    });

    await page.goto('/deploy');
    await expect(page.getByTestId('preset-grid')).toBeVisible({ timeout: 20_000 });
    await page.getByTestId('preset-card-pi-3b').click();
    // Auto mode is the default; just click Next then Submit.
    await page.getByTestId('deploy-wizard-next').click();
    await expect(page.getByTestId('deploy-wizard-step-config')).toBeVisible({ timeout: 5_000 });
    await page.getByTestId('deploy-wizard-submit').click();

    // The wizard closes on 201 (mock returns 201 immediately) and the
    // success flow navigates to /workloads. Asserting URL change is more
    // robust than asserting Modal visibility because AntD leaves the
    // closed modal in the DOM with display:none and the testid stays
    // attached.
    await page.waitForURL('**/workloads', { timeout: 10_000 });
    expect(postSent).toBe(true);
  });
});
