import { Alert, Spin } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { getRuntimeConfig } from '@/config/runtime';

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

/**
 * POC fallback: dashboard key -> Grafana URL path.
 *
 * Production: P1-T-205 implements `GET /api/v1/grafana/url?dashboard=...` which
 * returns a signed URL. This component will swap to that API when T205 lands.
 *
 * Keys + slugs come from `deploy/CLAUDE.md §3.6` (file name = key with `_` -> `-`).
 */
const POC_DASHBOARD_MAP: Record<string, string> = {
  cluster_overview: '/d/cluster-overview/cluster-overview',
  node_detail: '/d/node-detail/node-detail',
  npu_detail: '/d/npu-detail/npu-detail',
  workload_business: '/d/workload-business/workload-business',
  workload_resource: '/d/workload-resource/workload-resource',
};

/** @internal exported for unit tests only. */
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

/**
 * Embeds a Grafana dashboard via iframe. POC for P1-T-010.
 *
 * Cross-origin / auth model (Phase 1 dev stack):
 * - Frontend on http://localhost:3000, Grafana on http://localhost:3001
 * - Grafana sets GF_SECURITY_ALLOW_EMBEDDING=true (deploy/dev/docker-compose.yaml)
 * - Grafana anonymous Viewer enabled so iframe needs no login (POC only)
 *
 * Production path (Phase 2+):
 * - Backend signs short-lived URLs (P1-T-205, /api/v1/grafana/url)
 * - This component calls that endpoint and uses the signed URL
 * - Grafana behind proxy with shared SSO (OIDC) — see ADR-NNNN when added
 */
export function GrafanaPanel({
  dashboard,
  variables,
  height = 600,
  kiosk = 'tv',
}: GrafanaPanelProps) {
  const [url, setUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Stable serialization so the effect doesn't re-run on identical objects.
  const varsKey = useMemo(
    () => (variables ? JSON.stringify(variables) : ''),
    [variables],
  );

  useEffect(() => {
    setError(null);
    setUrl(null);
    let cfg;
    try {
      cfg = getRuntimeConfig();
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'runtime config not loaded',
      );
      return;
    }
    const computed = buildPocUrl(
      cfg.grafanaBaseURL,
      dashboard,
      variables,
      kiosk,
    );
    if (!computed) {
      setError(`Unknown dashboard key: ${dashboard}`);
      return;
    }
    setUrl(computed);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dashboard, varsKey, kiosk]);

  if (error) {
    return (
      <Alert
        type="error"
        message="Grafana panel error"
        description={error}
        showIcon
        data-testid="grafana-panel-error"
      />
    );
  }
  if (!url) {
    return <Spin data-testid="grafana-panel-loading" />;
  }
  return (
    <iframe
      src={url}
      style={{ width: '100%', height, border: 'none' }}
      title={`Grafana: ${dashboard}`}
      sandbox="allow-scripts allow-same-origin allow-popups allow-forms"
      loading="lazy"
      data-testid="grafana-panel-iframe"
    />
  );
}

export default GrafanaPanel;
