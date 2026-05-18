import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { ConfigProvider } from 'antd';
import i18n from '@/i18n';
import { loadRuntimeConfig } from '@/config/runtime';
import type { components } from '@/services/types';

/**
 * Stub the global Image so GrafanaPanel's reachability probe
 * (known-issues #6) resolves immediately in jsdom — every probe
 * "succeeds" by default, which matches the dev compose flow where
 * Grafana IS up. Without this stub the iframe never mounts and the
 * URL assertions below time out.
 */
class FakeImage {
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  private _src = '';
  set src(_value: string) {
    this._src = _value;
    queueMicrotask(() => this.onload?.());
  }
  get src() {
    return this._src;
  }
}

/**
 * P1-T-208 Metrics page tests.
 *
 * Assertion focus is on URL composition — the spec calls out tab switches
 * and variable picks both recomputing the GrafanaPanel iframe `src`.
 * We pierce GrafanaPanel via `data-testid="grafana-panel-iframe"` (set in
 * `components/GrafanaPanel/index.tsx`) and read its `src` attribute. Real
 * Grafana isn't reachable from jsdom, but URL building is pure and runs
 * against the runtime config DEFAULTS (`http://localhost:3001`).
 *
 * api.get is mocked at the module boundary so the per-tab variable
 * selectors can hand the page a stable set of options to pick from.
 */

const mockGet = vi.fn<(url: string, config?: unknown) => Promise<unknown>>();
vi.mock('@/services/api', () => ({
  api: {
    get: (url: string, config?: unknown) => mockGet(url, config),
  },
}));

const { default: MetricsPage } = await import('@/pages/Metrics');

type Node = { name: string };
type NPU = components['schemas']['NPU'];
type Workload = components['schemas']['Workload'];

function makeNodes(): Node[] {
  return [{ name: 'node-1' }, { name: 'node-2' }];
}

function makeNPUs(): NPU[] {
  return [
    {
      id: 'npu-1-0',
      model: 'Ascend910B',
      status: 'healthy',
      slices: [
        { id: 'npu-1-0-slice-0', status: 'available' },
        { id: 'npu-1-0-slice-1', status: 'allocated' },
      ],
    },
    {
      id: 'npu-1-1',
      model: 'Ascend910B',
      status: 'healthy',
      slices: [],
    },
  ];
}

function makeWorkloads(): Workload[] {
  return [
    { name: 'qwen-8b-pd-prefill', namespace: 'default', type: 'inference', status: 'running' },
    { name: 'qwen-8b-pd-decode', namespace: 'default', type: 'inference', status: 'running' },
  ];
}

function defaultMockImpl(url: string): Promise<unknown> {
  if (url === '/api/v1/nodes') {
    return Promise.resolve({ data: makeNodes() });
  }
  if (url === '/api/v1/nodes/node-1/npus') {
    return Promise.resolve({ data: makeNPUs() });
  }
  if (url === '/api/v1/nodes/node-2/npus') {
    return Promise.resolve({ data: [] satisfies NPU[] });
  }
  if (url === '/api/v1/workloads') {
    return Promise.resolve({ data: makeWorkloads() });
  }
  return Promise.reject(new Error(`unexpected URL: ${url}`));
}

function renderMetrics() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: 0 },
    },
  });
  return render(
    <ConfigProvider>
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={qc}>
          <MetricsPage />
        </QueryClientProvider>
      </I18nextProvider>
    </ConfigProvider>,
  );
}

async function getIframeSrc(): Promise<string> {
  const iframe = await waitFor(() => screen.getByTestId('grafana-panel-iframe'));
  const src = iframe.getAttribute('src');
  if (!src) throw new Error('iframe has no src');
  return src;
}

const originalImage = globalThis.Image;

beforeEach(async () => {
  mockGet.mockReset();
  mockGet.mockImplementation(defaultMockImpl);
  await loadRuntimeConfig();
  await act(async () => {
    await i18n.changeLanguage('zh-CN');
  });
  // @ts-expect-error — partial Image stub for jsdom (Vitest test env).
  globalThis.Image = FakeImage;
});

afterEach(() => {
  globalThis.Image = originalImage;
});

describe('MetricsPage — initial render', () => {
  it('renders all 5 dashboard tabs', async () => {
    renderMetrics();

    expect(await screen.findByTestId('metrics-tab-cluster-overview')).toBeInTheDocument();
    expect(screen.getByTestId('metrics-tab-node-detail')).toBeInTheDocument();
    expect(screen.getByTestId('metrics-tab-npu-detail')).toBeInTheDocument();
    expect(screen.getByTestId('metrics-tab-workload-business')).toBeInTheDocument();
    expect(screen.getByTestId('metrics-tab-workload-resource')).toBeInTheDocument();
  });

  it('defaults to cluster-overview tab with no variables in the iframe src', async () => {
    renderMetrics();
    const src = await getIframeSrc();
    expect(src).toContain('/d/cluster-overview/cluster-overview');
    expect(src).not.toContain('var-node');
    expect(src).not.toContain('var-npu');
    expect(src).not.toContain('var-workload');
  });

  it('does not render variable selectors on the cluster-overview tab', async () => {
    renderMetrics();
    await screen.findByTestId('grafana-panel-iframe');
    expect(screen.queryByTestId('metrics-vars-node')).not.toBeInTheDocument();
    expect(screen.queryByTestId('metrics-vars-cascader')).not.toBeInTheDocument();
    expect(screen.queryByTestId('metrics-vars-workload')).not.toBeInTheDocument();
  });
});

describe('MetricsPage — tab switch recomputes iframe src', () => {
  it('switching to node-detail loads node-detail dashboard URL', async () => {
    const user = userEvent.setup();
    renderMetrics();

    await screen.findByTestId('grafana-panel-iframe');
    await user.click(screen.getByTestId('metrics-tab-node-detail'));

    await waitFor(async () => {
      const src = await getIframeSrc();
      expect(src).toContain('/d/node-detail/node-detail');
    });
  });

  it('switching to workload-business loads workload-business dashboard URL', async () => {
    const user = userEvent.setup();
    renderMetrics();

    await screen.findByTestId('grafana-panel-iframe');
    await user.click(screen.getByTestId('metrics-tab-workload-business'));

    await waitFor(async () => {
      const src = await getIframeSrc();
      expect(src).toContain('/d/workload-business/workload-business');
    });
  });
});

describe('MetricsPage — node-detail variable selection', () => {
  it('picking a node injects var-node=<name> into the iframe src', async () => {
    const user = userEvent.setup();
    renderMetrics();

    await screen.findByTestId('grafana-panel-iframe');
    await user.click(screen.getByTestId('metrics-tab-node-detail'));

    // The node Select fetches /api/v1/nodes; wait for the loaded options.
    await waitFor(() => {
      expect(
        mockGet.mock.calls.some(([url]) => url === '/api/v1/nodes'),
      ).toBe(true);
    });

    // Open the AntD Select and pick "node-1". AntD renders options in a
    // portal as `.ant-select-item-option-content`; querying by that class
    // avoids the duplicate match against the selected-value display node.
    const nodeSelect = screen.getByTestId('metrics-select-node');
    const combobox = nodeSelect.querySelector('.ant-select-selector');
    if (!combobox) throw new Error('node select combobox not found');
    await user.click(combobox);

    const option = await waitFor(() => {
      const items = document.querySelectorAll(
        '.ant-select-item-option-content',
      );
      const match = Array.from(items).find((el) => el.textContent === 'node-1');
      if (!match) throw new Error('no option "node-1" yet');
      return match;
    });
    await user.click(option);

    await waitFor(async () => {
      const src = await getIframeSrc();
      expect(src).toContain('var-node=node-1');
    });
  });
});

describe('MetricsPage — npu-detail cascader composition', () => {
  it('switching to npu-detail without selection has no var-node / var-npu / var-slice', async () => {
    const user = userEvent.setup();
    renderMetrics();

    await screen.findByTestId('grafana-panel-iframe');
    await user.click(screen.getByTestId('metrics-tab-npu-detail'));

    await waitFor(async () => {
      const src = await getIframeSrc();
      expect(src).toContain('/d/npu-detail/npu-detail');
    });

    const src = await getIframeSrc();
    expect(src).not.toContain('var-node=');
    expect(src).not.toContain('var-npu=');
    expect(src).not.toContain('var-slice=');
  });
});

describe('MetricsPage — workload tab variable selection', () => {
  it('picking a workload injects var-workload=<namespace/name>', async () => {
    const user = userEvent.setup();
    renderMetrics();

    await screen.findByTestId('grafana-panel-iframe');
    await user.click(screen.getByTestId('metrics-tab-workload-resource'));

    await waitFor(() => {
      expect(
        mockGet.mock.calls.some(([url]) => url === '/api/v1/workloads'),
      ).toBe(true);
    });

    const wlSelect = screen.getByTestId('metrics-select-workload');
    const combobox = wlSelect.querySelector('.ant-select-selector');
    if (!combobox) throw new Error('workload select combobox not found');
    await user.click(combobox);

    const option = await waitFor(() => {
      const items = document.querySelectorAll(
        '.ant-select-item-option-content',
      );
      const match = Array.from(items).find(
        (el) => el.textContent === 'default/qwen-8b-pd-prefill',
      );
      if (!match) throw new Error('no option for prefill yet');
      return match;
    });
    await user.click(option);

    await waitFor(async () => {
      const src = await getIframeSrc();
      expect(src).toContain('var-workload=default%2Fqwen-8b-pd-prefill');
      expect(src).toContain('/d/workload-resource/workload-resource');
    });
  });

  it('switching back to cluster-overview drops the workload variable', async () => {
    const user = userEvent.setup();
    renderMetrics();

    await screen.findByTestId('grafana-panel-iframe');
    await user.click(screen.getByTestId('metrics-tab-workload-business'));

    await waitFor(() => {
      expect(
        mockGet.mock.calls.some(([url]) => url === '/api/v1/workloads'),
      ).toBe(true);
    });

    const wlSelect = screen.getByTestId('metrics-select-workload');
    const combobox = wlSelect.querySelector('.ant-select-selector');
    if (!combobox) throw new Error('workload select combobox not found');
    await user.click(combobox);
    const option = await waitFor(() => {
      const items = document.querySelectorAll(
        '.ant-select-item-option-content',
      );
      const match = Array.from(items).find(
        (el) => el.textContent === 'default/qwen-8b-pd-decode',
      );
      if (!match) throw new Error('no option for decode yet');
      return match;
    });
    await user.click(option);

    await waitFor(async () => {
      const src = await getIframeSrc();
      expect(src).toContain('var-workload=default%2Fqwen-8b-pd-decode');
    });

    // Now flip back to cluster-overview — variables should drop.
    await user.click(screen.getByTestId('metrics-tab-cluster-overview'));

    await waitFor(async () => {
      const src = await getIframeSrc();
      expect(src).toContain('/d/cluster-overview/cluster-overview');
      expect(src).not.toContain('var-workload');
    });
  });
});
