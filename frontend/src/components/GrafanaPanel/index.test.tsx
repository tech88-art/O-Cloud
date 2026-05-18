import type { ReactElement } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import i18n from '@/i18n';
import GrafanaPanel from './index';
import { buildPocUrl } from './urlBuilder';
import { loadRuntimeConfig } from '@/config/runtime';

/**
 * Stub the global Image so the GrafanaPanel reachability probe
 * (known-issues #6) resolves deterministically in jsdom. Default
 * behavior: every probe succeeds (onload fires). Tests that want
 * the unreachable path call `stubImageProbe('error')` to override.
 */
type ProbeOutcome = 'load' | 'error';
let probeOutcome: ProbeOutcome = 'load';
function stubImageProbe(outcome: ProbeOutcome) {
  probeOutcome = outcome;
}

class FakeImage {
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  private _src = '';
  set src(_value: string) {
    this._src = _value;
    // Resolve on next microtask so the consumer can attach handlers
    // after constructing the object.
    queueMicrotask(() => {
      if (probeOutcome === 'load') this.onload?.();
      else this.onerror?.();
    });
  }
  get src() {
    return this._src;
  }
}

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
  const originalImage = globalThis.Image;

  beforeEach(async () => {
    // Force-reload runtime config so getRuntimeConfig() returns DEFAULTS.
    // (Test environment has no /config.json server; loadRuntimeConfig falls
    // back gracefully.)
    await loadRuntimeConfig();
    // Default probe outcome: reachable (matches prod docker-compose).
    probeOutcome = 'load';
    // @ts-expect-error — assigning a partial Image stub is fine for tests.
    globalThis.Image = FakeImage;
  });

  afterEach(() => {
    globalThis.Image = originalImage;
  });

  const renderWithI18n = (node: ReactElement) =>
    render(<I18nextProvider i18n={i18n}>{node}</I18nextProvider>);

  it('renders an iframe with the computed URL for a known dashboard', async () => {
    renderWithI18n(<GrafanaPanel dashboard="cluster_overview" />);
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
    renderWithI18n(<GrafanaPanel dashboard="not_real" />);
    const error = await waitFor(() =>
      screen.getByTestId('grafana-panel-error'),
    );
    expect(error).toBeInTheDocument();
    expect(error.textContent).toContain('not_real');
  });

  it('renders the unreachable Alert when the probe fails (known-issues #6)', async () => {
    stubImageProbe('error');
    renderWithI18n(<GrafanaPanel dashboard="cluster_overview" />);
    const unreachable = await waitFor(() =>
      screen.getByTestId('grafana-panel-unreachable'),
    );
    expect(unreachable).toBeInTheDocument();
    // The iframe must NOT render alongside the unreachable banner.
    expect(screen.queryByTestId('grafana-panel-iframe')).toBeNull();
  });
});
