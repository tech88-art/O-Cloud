/**
 * Runtime configuration loaded from /config.json at app start.
 *
 * Deployment injects /config.json via ConfigMap so that the same build
 * artifact can target different environments (dev / staging / prod) without
 * rebuilding. See frontend/CLAUDE.md §4.7.
 */

export interface RuntimeConfig {
  apiBaseURL: string;
  wsBaseURL: string;
  grafanaBaseURL: string;
}

const DEFAULTS: RuntimeConfig = {
  apiBaseURL: 'http://localhost:8080',
  wsBaseURL: 'ws://localhost:8080',
  grafanaBaseURL: 'http://localhost:3001',
};

let cached: RuntimeConfig | null = null;

/**
 * Load /config.json once and memoise. Failures fall back to DEFAULTS so dev
 * never blocks on a missing config file.
 */
export async function loadRuntimeConfig(): Promise<RuntimeConfig> {
  if (cached) return cached;
  try {
    const res = await fetch('/config.json', { cache: 'no-store' });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = (await res.json()) as Partial<RuntimeConfig>;
    cached = { ...DEFAULTS, ...data };
  } catch (err) {
    console.warn('runtime config load failed, using defaults', err);
    cached = { ...DEFAULTS };
  }
  return cached;
}

/**
 * Synchronous accessor for code paths that run after loadRuntimeConfig() has
 * resolved (i.e. anything mounted under <App />). Throws if called too early.
 */
export function getRuntimeConfig(): RuntimeConfig {
  if (!cached) {
    throw new Error('runtime config not loaded yet — call loadRuntimeConfig() first');
  }
  return cached;
}
