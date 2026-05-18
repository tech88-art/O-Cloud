/**
 * Grafana iframe URL helpers for the demo (P1-T-010 POC).
 *
 * Kept in a sibling module to `index.tsx` so the component file exports
 * components only — React-refresh's `only-export-components` rule
 * disables HMR for files that mix components and helpers. Splitting
 * is mechanical: the production component imports the helpers from
 * here, the test file does the same.
 *
 * Production (Phase 2+): replace this whole module with a call to
 * `GET /api/v1/grafana/url?dashboard=...` (signed URL handler, P1-T-205).
 */

import type { GrafanaPanelProps } from './types';

/**
 * POC fallback: dashboard key → Grafana URL path.
 *
 * Keys + slugs come from `deploy/CLAUDE.md §3.6` (file name = key with
 * `_` → `-`).
 */
export const POC_DASHBOARD_MAP: Record<string, string> = {
  cluster_overview: '/d/cluster-overview/cluster-overview',
  node_detail: '/d/node-detail/node-detail',
  npu_detail: '/d/npu-detail/npu-detail',
  workload_business: '/d/workload-business/workload-business',
  workload_resource: '/d/workload-resource/workload-resource',
};

/**
 * Build a Grafana iframe URL from the POC dashboard map. Returns null
 * when the dashboard key isn't recognized — caller should surface an
 * error state to the operator.
 */
export function buildPocUrl(
  base: string,
  dashboard: string,
  variables: Record<string, string> | undefined,
  kiosk: GrafanaPanelProps['kiosk'],
): string | null {
  const path = POC_DASHBOARD_MAP[dashboard];
  if (!path) return null;
  const params = new URLSearchParams({ orgId: '1' });
  if (kiosk) params.set('kiosk', kiosk);
  if (variables) {
    for (const [k, v] of Object.entries(variables)) {
      params.set(`var-${k}`, v);
    }
  }
  return `${base.replace(/\/$/, '')}${path}?${params.toString()}`;
}
