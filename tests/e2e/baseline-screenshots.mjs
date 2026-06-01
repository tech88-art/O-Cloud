// Phase 12 render baseline — standalone Playwright screenshot script.
//
// WHY standalone (not a *.spec.ts): this is a one-off capture utility, not a
// test. It drives the *already-running* servers (backend :8080 + frontend
// :3000) directly and uses deterministic waits (domcontentloaded + selector +
// fixed settle) instead of networkidle — it screenshots fine even though the
// workspace holds an open topology WebSocket (which makes Preview MCP's
// networkidle-based screenshot time out).
//
// P12-T-301: the 5 separate pages (overview/workloads/deploy/metrics/logs)
// were collapsed into ONE workspace at /overview (ADR-0022). The retired
// routes now redirect there, so we capture the workspace in its KEY STATES
// instead of per-page. The phase-11 `before-*.png` (5 pages) stay as the
// historical baseline; the T302 checkpoint pairs them against these
// `after-*` workspace states to show the 5→1 collapse.
//
// Usage (servers must be up):
//   node tests/e2e/baseline-screenshots.mjs            # before-*.png
//   PHASE=after node tests/e2e/baseline-screenshots.mjs # after-*.png
//
// Output: docs/screenshots/phase12/<phase>-<state>.png  (1440x900 viewport)

import { chromium } from '@playwright/test';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import fs from 'node:fs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '..', '..');
const outDir = path.join(repoRoot, 'docs', 'screenshots', 'phase12');
fs.mkdirSync(outDir, { recursive: true });

const BASE = process.env.BASE_URL ?? 'http://localhost:3000';
const PHASE = process.env.PHASE ?? 'before';

// Workspace key states. Each: [output-name, async setup(page)]. The default
// state is the bare workspace; the others toggle fabric / drive a selection
// so the after-* set documents the one-page console's main affordances.
const STATES = [
  ['overview', async () => {}],
  [
    'overview-fabric',
    async (page) => {
      // network (green) + hccs (purple) + fabric-link (blue) edges via BandwidthEdge.
      await page.getByTestId('fabric-toggle-switch').click().catch(() => {});
      await page.waitForTimeout(2500);
    },
  ],
];

const browser = await chromium.launch();
const ctx = await browser.newContext({
  viewport: { width: 1440, height: 900 },
  deviceScaleFactor: 1,
});
const page = await ctx.newPage();

const results = [];
for (const [name, setup] of STATES) {
  const url = `${BASE}/overview`;
  try {
    await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 30_000 });
    // The workspace always renders the topology canvas; wait then let data land.
    await page.waitForSelector('[data-testid="topology-graph"]', { timeout: 20_000 }).catch(() => {});
    await page.waitForTimeout(3500);
    await setup(page);
    const file = path.join(outDir, `${PHASE}-${name}.png`);
    await page.screenshot({ path: file, fullPage: false });
    results.push(`OK  ${PHASE}-${name}.png  <- ${url}`);
  } catch (err) {
    results.push(`ERR ${PHASE}-${name}  <- ${url}  :: ${err.message}`);
  }
}

await browser.close();
console.log(results.join('\n'));
console.log(`\nDONE -> ${outDir}`);
