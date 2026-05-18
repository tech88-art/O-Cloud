import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
import GrafanaPanel from './index';
import { buildPocUrl } from './urlBuilder';
import { loadRuntimeConfig } from '@/config/runtime';

describe('buildPocUrl', () => {
  const base = 'http://localhost:3001';

  it('builds a URL for a known dashboard with default kiosk=tv', () => {
    const url = buildPocUrl(base, 'cluster_overview', undefined, 'tv');
    expect(url).toBe(
      'http://localhost:3001/d/cluster-overview/cluster-overview?orgId=1&kiosk=tv',
    );
  });

  it('returns null for an unknown dashboard', () => {
    expect(buildPocUrl(base, 'no_such_dashboard', undefined, 'tv')).toBeNull();
  });

  it('omits kiosk param when kiosk=false', () => {
    const url = buildPocUrl(base, 'node_detail', undefined, false);
    expect(url).toContain('?orgId=1');
    expect(url).not.toContain('kiosk');
  });

  it('injects template variables as var-<name> params', () => {
    const url = buildPocUrl(
      base,
      'workload_business',
      { cluster: 'cluster-a', namespace: 'ocloud-system' },
      'tv',
    );
    expect(url).toContain('var-cluster=cluster-a');
    expect(url).toContain('var-namespace=ocloud-system');
  });

  it('strips trailing slash from base', () => {
    const url = buildPocUrl(
      'http://localhost:3001/',
      'cluster_overview',
      undefined,
      'tv',
    );
    expect(url?.startsWith('http://localhost:3001/d/')).toBe(true);
  });
});

describe('GrafanaPanel', () => {
  beforeEach(async () => {
    // Force-reload runtime config so getRuntimeConfig() returns DEFAULTS.
    // (Test environment has no /config.json server; loadRuntimeConfig falls
    // back gracefully.)
    await loadRuntimeConfig();
  });

  it('renders an iframe with the computed URL for a known dashboard', async () => {
    render(<GrafanaPanel dashboard="cluster_overview" />);
    const iframe = await waitFor(() =>
      screen.getByTestId('grafana-panel-iframe'),
    );
    expect(iframe).toBeInTheDocument();
    expect(iframe.getAttribute('src')).toContain(
      '/d/cluster-overview/cluster-overview',
    );
    expect(iframe.getAttribute('src')).toContain('kiosk=tv');
  });

  it('renders an error Alert for an unknown dashboard key', async () => {
    render(<GrafanaPanel dashboard="not_real" />);
    const error = await waitFor(() =>
      screen.getByTestId('grafana-panel-error'),
    );
    expect(error).toBeInTheDocument();
    expect(error.textContent).toContain('not_real');
  });
});
