import { Alert, Spin } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { getRuntimeConfig } from '@/config/runtime';
import { buildPocUrl } from './urlBuilder';
import type { GrafanaPanelProps } from './types';

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
 *
 * URL builder + props type live in sibling modules (`urlBuilder.ts`,
 * `types.ts`) so this file exports components only — keeps the React-
 * refresh HMR boundary clean (see known-issues.md #1).
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
    // varsKey carries the variables identity; pulling `variables` itself in
    // would re-fire the effect on every parent re-render even when the
    // serialized payload is unchanged.
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
