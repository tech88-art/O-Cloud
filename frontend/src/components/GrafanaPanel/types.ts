/**
 * GrafanaPanel public types.
 *
 * Lives in a sibling module so both `index.tsx` (component) and
 * `urlBuilder.ts` (helpers) can depend on it without bouncing back
 * through the component file — keeps `index.tsx` free of non-component
 * exports for the React-refresh HMR boundary.
 */

export interface GrafanaPanelProps {
  /** Dashboard key as defined in `deploy/CLAUDE.md §3.6` (e.g. `cluster_overview`). */
  dashboard: string;
  /** Grafana dashboard variables to inject as `var-<name>=<value>` URL params. */
  variables?: Record<string, string>;
  /** Iframe height in CSS pixels. Defaults to 600. */
  height?: number;
  /** Kiosk mode hides Grafana nav/sidebar. Defaults to 'tv'. Pass false to disable. */
  kiosk?: 'tv' | 'full' | false;
}
