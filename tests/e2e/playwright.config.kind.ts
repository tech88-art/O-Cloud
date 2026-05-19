import { defineConfig } from '@playwright/test';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

/**
 * Phase 3 P3-T-104 — kind smoke Playwright config.
 *
 * Strategy
 * --------
 * This config is intentionally separate from `playwright.config.ts` so the
 * Phase 1 mock-backed suite under `./tests/` keeps running unchanged. The
 * kind smoke specs live under `./specs/` and target a running kind cluster
 * provisioned by `tests/e2e/kind/install.sh up`.
 *
 * No `webServer` block — the kind cluster's NodePort 30080 (demo-backend)
 * and 30090 (ascend-npu-exporter-plus) are already host-mapped via
 * `extraPortMappings` in `tests/e2e/kind/kind-config.yaml`, so by the time
 * the workflow reaches `playwright test --config=playwright.config.kind.ts`,
 * `http://localhost:30080` and `http://localhost:30090` are reachable from
 * the runner.
 *
 * Why API-level smokes (no browser)
 * --------------------------------
 * The frontend image (React + Vite + nginx serve pipeline) is not built in
 * this task — its end-to-end coverage is already provided by the Phase 1
 * mock-backed Playwright suite in `playwright.config.ts`. The kind smoke's
 * unique value is proving the *server-side* chain:
 *   pool-operator Reconcile -> NPUSlicePool.status -> exporter-plus metrics
 *   -> demo-backend k8s+crd datasources -> JSON over /api/v1.
 * API-level assertions are the cheapest probe for that chain.
 */

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  testDir: path.join(__dirname, 'specs'),
  fullyParallel: false,
  // Real cluster + image pulls have a wider variance than mock; one retry
  // absorbs transient image-pull/network blips without masking real bugs.
  retries: 1,
  workers: 1,
  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
  ],
  outputDir: 'test-results',
  use: {
    baseURL: process.env.OCEDGE_KIND_BASE ?? 'http://localhost:30080',
    trace: 'on-first-retry',
    // Real-cluster reconcile loops can take a beat; give each action room.
    actionTimeout: 15_000,
    navigationTimeout: 30_000,
  },
  projects: [
    {
      name: 'kind-smoke',
      use: {},
    },
  ],
});
