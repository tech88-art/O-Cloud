import { Alert, Spin } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
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
 * Reachability (known-issues #6): pre-probe the Grafana host before
 * mounting the iframe. The probe loads Grafana's stock favicon via an
 * Image() — succeeds when reachable, fails on connection refused / 404
 * / opaque cross-origin block. Replaces the "blank iframe + cryptic
 * browser error page" experience when Grafana isn't running.
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
  const { t } = useTranslation();
  const [url, setUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reachable, setReachable] = useState<'probing' | 'reachable' | 'unreachable'>(
    'probing',
  );
  const [grafanaBase, setGrafanaBase] = useState<string | null>(null);

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
    setGrafanaBase(cfg.grafanaBaseURL);
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

  // Reachability probe (known-issues #6). Image-based so we don't need
  // CORS headers on Grafana; the stock /public/img/fav32.png ships with
  // every Grafana install. onload → reachable; onerror / 3s timeout →
  // unreachable. Re-runs whenever the base URL changes (rare).
  useEffect(() => {
    if (!grafanaBase) return;
    setReachable('probing');
    const trimmed = grafanaBase.endsWith('/') ? grafanaBase.slice(0, -1) : grafanaBase;
    const probe = new Image();
    let settled = false;
    const finish = (ok: boolean) => {
      if (settled) return;
      settled = true;
      probe.onload = null;
      probe.onerror = null;
      setReachable(ok ? 'reachable' : 'unreachable');
    };
    const timer = window.setTimeout(() => finish(false), 3_000);
    probe.onload = () => {
      window.clearTimeout(timer);
      finish(true);
    };
    probe.onerror = () => {
      window.clearTimeout(timer);
      finish(false);
    };
    // Cache-bust so a previously-failed probe doesn't pin the negative
    // result in the browser cache when Grafana comes up later.
    probe.src = `${trimmed}/public/img/fav32.png?_=${Date.now()}`;
    return () => {
      window.clearTimeout(timer);
      probe.onload = null;
      probe.onerror = null;
    };
  }, [grafanaBase]);

  if (error) {
    return (
      <Alert
        type="error"
        message={t('grafanaPanel.errorTitle')}
        description={error}
        showIcon
        data-testid="grafana-panel-error"
      />
    );
  }
  if (!url || reachable === 'probing') {
    return <Spin data-testid="grafana-panel-loading" />;
  }
  if (reachable === 'unreachable') {
    return (
      <Alert
        type="warning"
        message={t('grafanaPanel.unreachableTitle')}
        description={t('grafanaPanel.unreachableDescription', {
          base: grafanaBase ?? '',
        })}
        showIcon
        data-testid="grafana-panel-unreachable"
      />
    );
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
