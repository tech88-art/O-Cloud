// Phase 12 render baseline — standalone Playwright screenshot script.
//
// WHY standalone (not a *.spec.ts): this is a one-off capture utility, not a
// test. Keeping it out of tests/ avoids `playwright test` picking it up on
// every CI run. It drives the *already-running* servers (backend :8080 +
// frontend :3000) directly — no webServer spawn — and uses deterministic
// waits (domcontentloaded + selector + fixed settle) instead of networkidle,
// so it screenshots fine even though the Overview page holds an open topology
// WebSocket (which makes Preview MCP's networkidle-based screenshot time out).
//
// Usage (servers must be up):
//   node tests/e2e/baseline-screenshots.mjs            # before-*.png
//   PHASE=after node tests/e2e/baseline-screenshots.mjs # after-*.png
//   BASE_URL=http://localhost:3000 node tests/e2e/baseline-screenshots.mjs
//
// Output: docs/screenshots/phase12/<phase>-<page>.png  (1440x900 viewport)

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

// [name, route, settle-ms] — settle covers react-query fetch + render.
// Overview gets longer: ReactFlow + dagre layout of ~100 nodes.
const PAGES = [
  ['overview', '/overview', 3500],
  ['workloads', '/workloads', 2500],
  ['deploy', '/deploy', 2000],
  ['metrics', '/metrics', 2500],
  ['logs', '/logs', 2500],
];

const browser = await chromium.launch();
const ctx = await browser.newContext({
  viewport: { width: 1440, height: 900 },
  deviceScaleFactor: 1,
});
const page = await ctx.newPage();

const results = [];
for (const [name, route, settle] of PAGES) {
  const url = BASE + route;
  try {
    await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 30_000 });
    // AntD layout shell always renders a <main>; wait for it then let data land.
    await page.waitForSelector('main', { timeout: 15_000 }).catch(() => {});
    await page.waitForTimeout(settle);
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
