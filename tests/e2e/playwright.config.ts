import { defineConfig, devices } from '@playwright/test';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

/**
 * Playwright config for O-Cloud Edge — P1-T-110.
 *
 * Strategy
 * --------
 * Goal: `pnpm exec playwright test` should work both when (a) the developer
 * already has `make dev-up` running, AND (b) when no servers are running at
 * all. We rely on `reuseExistingServer: true` so the same command is safe in
 * either case — if :8080 / :3000 already respond, Playwright skips spawn;
 * otherwise it starts them.
 *
 * On Windows the backend binary lives at `backend/bin/demo-backend.exe`,
 * built via `cd backend && make build`. The webServer commands below assume
 * the binary exists. If you see "command not found" in the webServer log,
 * run `make build` in `backend/` once before retrying.
 *
 * Browsers
 * --------
 * Phase 1 chromium only. firefox / webkit can be added in a follow-up; the
 * topology canvas (ReactFlow + dagre) is the same DOM contract in all three
 * but CI runner setup cost is highest for webkit, so we defer.
 *
 * Reports
 * -------
 * HTML report under `playwright-report/`. Failure screenshots + traces are
 * attached.
 */

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '..', '..');

const BACKEND_PORT = process.env.E2E_BACKEND_PORT ?? '8080';
const FRONTEND_PORT = process.env.E2E_FRONTEND_PORT ?? '3000';
const FRONTEND_URL = `http://localhost:${FRONTEND_PORT}`;
const BACKEND_HEALTHZ = `http://localhost:${BACKEND_PORT}/api/v1/healthz`;

// The backend binary is built in the worktree under
// `backend/bin/demo-backend{.exe,}`. We pick the platform-specific suffix
// so the same config works on Linux CI and Windows dev.
const backendCwd = path.join(repoRoot, 'backend');
const frontendCwd = path.join(repoRoot, 'frontend');
const isWindows = process.platform === 'win32';
const backendBinaryName = isWindows ? 'demo-backend.exe' : 'demo-backend';
// Absolute path avoids the `./bin/...` vs `.\bin\...` cmd-vs-bash split.
const backendBinaryPath = path.join(backendCwd, 'bin', backendBinaryName);

export default defineConfig({
  testDir: './tests',
  fullyParallel: false, // single backend instance, keep tests serial for now
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: [
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
    ['list'],
  ],
  outputDir: 'test-results',

  use: {
    baseURL: FRONTEND_URL,
    screenshot: 'only-on-failure',
    trace: 'on-first-retry',
    video: 'retain-on-failure',
    // Be patient with the dev server's first paint (Vite cold start).
    actionTimeout: 15_000,
    navigationTimeout: 30_000,
  },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],

  /**
   * Auto-start backend + frontend if they aren't already running.
   *
   * - `reuseExistingServer: true` makes both safe to invoke repeatedly.
   * - Backend health check hits `/api/v1/healthz` (200 OK from
   *   `pkg/api/system.go`).
   * - Frontend `pnpm dev --host 0.0.0.0` so the spawned Vite is reachable
   *   on localhost.
   *
   * Working directories use absolute paths so we don't depend on the
   * shell's cwd when Playwright forks subprocesses.
   */
  webServer: [
    {
      // Absolute path + bare program name → cmd.exe and bash both accept.
      // Quoting is tolerated by both since the path has no spaces.
      command: `"${backendBinaryPath}" -c configs/config.dev.yaml`,
      cwd: backendCwd,
      url: BACKEND_HEALTHZ,
      timeout: 60_000,
      reuseExistingServer: true,
      stdout: 'pipe',
      stderr: 'pipe',
    },
    {
      command: `pnpm dev --host 0.0.0.0 --port ${FRONTEND_PORT}`,
      cwd: frontendCwd,
      url: FRONTEND_URL,
      timeout: 120_000,
      reuseExistingServer: true,
      stdout: 'pipe',
      stderr: 'pipe',
    },
  ],
});
