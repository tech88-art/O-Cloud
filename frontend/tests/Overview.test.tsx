import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  render,
  screen,
  waitFor,
  act,
  within,
  renderHook,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { MemoryRouter } from 'react-router-dom';
import { ConfigProvider } from 'antd';
import type { ReactNode } from 'react';
import { createElement } from 'react';
import i18n from '@/i18n';
import { useTopologyStore } from '@/store/topologyStore';
import { loadRuntimeConfig } from '@/config/runtime';
import type { Cluster, Topology, TopologyNode } from '@/services/cluster';
import type { NodeDetail, NPU } from '@/services/node';

/**
 * P1-T-108b Overview page tests.
 *
 * Extends the 7 cases from P1-T-108a with:
 *   - DetailPanel renders for cluster / node / npu / slice / null selections
 *   - Dbl-click NPU → store toggle → graph re-renders with slice children
 *   - useTopologyWS hook: handshake, message dispatch, auto-reconnect
 *
 * jsdom can't render the ReactFlow canvas (no ResizeObserver, no full
 * SVG layout) and we don't care here — the production code is exercised
 * up to the renderer; library rendering is the library's job. Mock
 * `@xyflow/react` at the module boundary so the component imports
 * resolve without hitting real DOM measurement code. The stub also
 * surfaces a button per node so user-event can click them and we can
 * count them after a dbl-click to assert slice nodes appeared.
 */

// -------- Mock @xyflow/react --------

vi.mock('@xyflow/react', () => {
  type StubNodeProps = {
    id: string;
    data?: { label?: string };
  };
  type StubProps = {
    nodes: StubNodeProps[];
    onNodeClick?: (e: unknown, n: { id: string }) => void;
    onNodeDoubleClick?: (e: unknown, n: { id: string }) => void;
  };
  const ReactFlow = ({ nodes, onNodeClick, onNodeDoubleClick }: StubProps) => (
    <div data-testid="rf-stub" data-node-count={nodes.length}>
      {nodes.map((n) => (
        <button
          key={n.id}
          data-testid={`rf-node-${n.id}`}
          onClick={() => onNodeClick?.(null, { id: n.id })}
          onDoubleClick={() => onNodeDoubleClick?.(null, { id: n.id })}
        >
          {n.data?.label ?? n.id}
        </button>
      ))}
    </div>
  );
  return {
    ReactFlow,
    ReactFlowProvider: ({ children }: { children: ReactNode }) => (
      <>{children}</>
    ),
    Background: () => null,
    Controls: () => null,
    MiniMap: () => null,
    // ADR-0005 (P1-T-214): the production TopologyGraph imports
    // `MarkerType` to stamp an arrowhead on pd-pair edges. Mirror the
    // enum shape so the import resolves to defined values in jsdom.
    MarkerType: { Arrow: 'arrow', ArrowClosed: 'arrowclosed' },
  };
});
vi.mock('@xyflow/react/dist/style.css', () => ({}));

// -------- Mock api module --------

const mockGet = vi.fn<(url: string, config?: unknown) => Promise<unknown>>();
vi.mock('@/services/api', () => ({
  api: {
    get: (url: string, config?: unknown) => mockGet(url, config),
  },
}));

// Imports of the SUT must come AFTER the mocks are registered.
const { default: OverviewPage } = await import('@/pages/Overview');
const { DetailPanel } = await import('@/pages/Overview/DetailPanel');
const { useTopologyWS } = await import('@/hooks/useTopologyWS');

// -------- Mock WebSocket --------

interface MockWebSocketInstance {
  url: string;
  readyState: number;
  onopen: ((ev: Event) => void) | null;
  onmessage: ((ev: MessageEvent) => void) | null;
  onclose: ((ev: CloseEvent) => void) | null;
  onerror: ((ev: Event) => void) | null;
  close: () => void;
  send: (data: string) => void;
  __triggerOpen: () => void;
  __triggerMessage: (data: unknown) => void;
  __triggerClose: () => void;
}

const wsInstances: MockWebSocketInstance[] = [];

class MockWebSocket implements Partial<WebSocket> {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  url: string;
  readyState = 0;
  onopen: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;

  constructor(url: string) {
    this.url = url;
    // eslint-disable-next-line @typescript-eslint/no-this-alias
    const self = this;
    const instance: MockWebSocketInstance = {
      url,
      get readyState() {
        return self.readyState;
      },
      set readyState(v: number) {
        self.readyState = v;
      },
      get onopen() {
        return self.onopen;
      },
      set onopen(v) {
        self.onopen = v;
      },
      get onmessage() {
        return self.onmessage;
      },
      set onmessage(v) {
        self.onmessage = v;
      },
      get onclose() {
        return self.onclose;
      },
      set onclose(v) {
        self.onclose = v;
      },
      get onerror() {
        return self.onerror;
      },
      set onerror(v) {
        self.onerror = v;
      },
      close: () => self.close(),
      send: () => {
        /* no-op */
      },
      __triggerOpen: () => {
        self.readyState = MockWebSocket.OPEN;
        self.onopen?.(new Event('open'));
      },
      __triggerMessage: (data: unknown) => {
        self.onmessage?.(
          new MessageEvent('message', { data: JSON.stringify(data) }),
        );
      },
      __triggerClose: () => {
        self.readyState = MockWebSocket.CLOSED;
        self.onclose?.(new CloseEvent('close'));
      },
    };
    wsInstances.push(instance);
  }

  close() {
    this.readyState = MockWebSocket.CLOSED;
  }

  send() {
    /* no-op */
  }
}

// -------- Fixtures --------

const CLUSTER_ID = 'cluster-prod-a-01';

function makeClusters(): Cluster[] {
  return [
    {
      id: CLUSTER_ID,
      name: CLUSTER_ID,
      status: 'healthy',
      role: 'edge-single',
      location: 'Site-A',
      nodeCount: 2,
      npuCount: 8,
      kubernetesVersion: 'v1.30.0',
    },
  ];
}

function makeTopology(): Topology {
  return {
    nodes: [
      { id: CLUSTER_ID, type: 'cluster', label: CLUSTER_ID, status: 'healthy' },
      { id: 'node-1', type: 'node', label: 'node-1', status: 'healthy' },
      { id: 'npu-1-0', type: 'npu', label: 'npu-1-0', status: 'idle' },
    ],
    edges: [
      { source: CLUSTER_ID, target: 'node-1', type: 'contains' },
      { source: 'node-1', target: 'npu-1-0', type: 'contains' },
    ],
  };
}

/**
 * Topology fixture extended with a `switch` node and a `fabric-link`
 * edge (ADR-0004). Post-ADR-0006 the generated `TopologyNode['type']`
 * union names `switch` natively, so the fixture is fully type-safe
 * without `as unknown as` widening (history: that cast pattern lived
 * here from T212 until the contract regen).
 */
function makeTopologyWithFabric(): Topology {
  return {
    nodes: [
      { id: CLUSTER_ID, type: 'cluster', label: CLUSTER_ID, status: 'healthy' },
      { id: 'node-1', type: 'node', label: 'node-1', status: 'healthy' },
      { id: 'npu-1-0', type: 'npu', label: 'npu-1-0', status: 'idle' },
      { id: 'switch-tor-01', type: 'switch', label: 'tor-01', status: 'up' },
    ],
    edges: [
      { source: CLUSTER_ID, target: 'node-1', type: 'contains' },
      { source: 'node-1', target: 'npu-1-0', type: 'contains' },
      { source: 'node-1', target: 'switch-tor-01', type: 'fabric-link' },
    ],
  };
}

/**
 * Topology fixture extended with a `workload` node, two `pod` nodes,
 * one `binds-to` (pod→slice) edge and one `pd-pair` (pod↔pod) edge
 * (ADR-0005). Post-ADR-0006 all three new node literals and both new
 * edge literals are in the auto-gen `TopologyNode/Edge['type']` union,
 * so no widening cast is needed.
 *
 * Mirrors what the backend (P1-T-213, `aggregator/topology.go:485-613`)
 * would serve when `?includeWorkloads=true` is set against the
 * set-a-small fixture — minimal subset (1 workload + 2 pods + 1 of each
 * edge type) so test assertions can pin specific test-ids without
 * the test getting brittle.
 */
function makeTopologyWithWorkloads(): Topology {
  return {
    nodes: [
      { id: CLUSTER_ID, type: 'cluster', label: CLUSTER_ID, status: 'healthy' },
      { id: 'node-1', type: 'node', label: 'node-1', status: 'healthy' },
      { id: 'npu-1-0', type: 'npu', label: 'npu-1-0', status: 'idle' },
      {
        id: 'npu-1-0-slice-0',
        type: 'slice',
        label: 'npu-1-0-slice-0',
        status: 'busy',
      },
      {
        id: 'workload/ai-inference/qwen-8b',
        type: 'workload',
        label: 'ai-inference/qwen-8b',
        status: 'running',
      },
      {
        id: 'pod/ai-inference/qwen-8b-prefill-0',
        type: 'pod',
        label: 'qwen-8b-prefill-0',
        status: 'running',
      },
      {
        id: 'pod/ai-inference/qwen-8b-decode-0',
        type: 'pod',
        label: 'qwen-8b-decode-0',
        status: 'running',
      },
    ],
    edges: [
      { source: CLUSTER_ID, target: 'node-1', type: 'contains' },
      { source: 'node-1', target: 'npu-1-0', type: 'contains' },
      { source: 'npu-1-0', target: 'npu-1-0-slice-0', type: 'contains' },
      {
        source: 'pod/ai-inference/qwen-8b-prefill-0',
        target: 'npu-1-0-slice-0',
        type: 'binds-to',
      },
      {
        source: 'pod/ai-inference/qwen-8b-prefill-0',
        target: 'pod/ai-inference/qwen-8b-decode-0',
        type: 'pd-pair',
      },
    ],
  };
}

function makeTopologyWithSlice(): Topology {
  const nodes: TopologyNode[] = [
    { id: CLUSTER_ID, type: 'cluster', label: CLUSTER_ID, status: 'healthy' },
    { id: 'node-1', type: 'node', label: 'node-1', status: 'healthy' },
    {
      id: 'npu-1-0',
      type: 'npu',
      label: 'npu-1-0',
      status: 'healthy',
      attributes: {
        model: 'Ascend910B',
        vramMiB: 65536,
        aiCoreTotal: 32,
        hccsGroup: 'hccs-0',
        sliceMode: 'fixed-template',
        slices: [
          { id: 'npu-1-0-slice-0', status: 'available' },
          { id: 'npu-1-0-slice-1', status: 'allocated' },
        ],
      },
    },
    {
      id: 'npu-1-0-slice-0',
      type: 'slice',
      label: 'npu-1-0-slice-0',
      status: 'idle',
      attributes: {
        parentNPU: 'npu-1-0',
        template: 'vir04',
      },
    },
    {
      id: 'npu-1-0-slice-1',
      type: 'slice',
      label: 'npu-1-0-slice-1',
      status: 'busy',
      attributes: {
        parentNPU: 'npu-1-0',
        template: 'vir04',
        allocatedTo: { namespace: 'default', podName: 'pod-1' },
      },
    },
  ];
  return {
    nodes,
    edges: [
      { source: CLUSTER_ID, target: 'node-1', type: 'contains' },
      { source: 'node-1', target: 'npu-1-0', type: 'contains' },
      { source: 'npu-1-0', target: 'npu-1-0-slice-0', type: 'contains' },
      { source: 'npu-1-0', target: 'npu-1-0-slice-1', type: 'contains' },
    ],
  };
}

function nodeDetailFixture(): NodeDetail {
  return {
    name: 'node-1',
    status: 'Ready',
    cpu: { raw: '32', bytes: null },
    memory: { raw: '128Gi', bytes: null },
    arch: 'amd64',
    os: 'Linux',
    kubeletVersion: 'v1.30.0',
    npuCount: 4,
  };
}

function npusFixture(): NPU[] {
  return [
    {
      id: 'npu-1-0',
      model: 'Ascend910B',
      status: 'healthy',
      vramMiB: 65536,
      aiCoreTotal: 32,
    },
    {
      id: 'npu-1-1',
      model: 'Ascend910B',
      status: 'degraded',
      vramMiB: 65536,
      aiCoreTotal: 32,
    },
  ];
}

// -------- Render helpers --------

function renderOverview() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: 0 },
    },
  });
  return render(
    <ConfigProvider>
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={qc}>
          <MemoryRouter initialEntries={['/overview']}>
            <OverviewPage />
          </MemoryRouter>
        </QueryClientProvider>
      </I18nextProvider>
    </ConfigProvider>,
  );
}

function renderDetailPanel() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: 0 },
    },
  });
  return render(
    <ConfigProvider>
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={qc}>
          <DetailPanel />
        </QueryClientProvider>
      </I18nextProvider>
    </ConfigProvider>,
  );
}

function wsWrapper(client?: QueryClient) {
  const qc =
    client ??
    new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
    });
  return ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children);
}

// -------- Setup --------

beforeEach(async () => {
  mockGet.mockReset();
  wsInstances.length = 0;
  vi.stubGlobal('WebSocket', MockWebSocket);
  // Runtime config has to be loaded before the WS hook can build a URL.
  // In jsdom there's no /config.json to fetch so it falls back to DEFAULTS;
  // that's fine for our purposes — we just need cached !== null.
  await loadRuntimeConfig();
  act(() => {
    useTopologyStore.setState({
      selectedClusterId: null,
      selectedNodeId: null,
      expandedNPUs: new Set<string>(),
      lastEventAt: null,
      // P1-T-212 / ADR-0004: reset the fabric toggle each test so cases
      // don't leak the previous test's flip.
      showFabric: false,
      // P1-T-214 / ADR-0005: same reset story for the workloads toggle.
      showWorkloads: false,
    });
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

// -------- Tests --------

describe('OverviewPage — state branches', () => {
  it('renders the loading skeleton while the topology fetch is in-flight', async () => {
    let resolveClusters: (v: Cluster[]) => void = () => {};
    let resolveTopology: (v: Topology) => void = () => {};
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return new Promise<{ data: Cluster[] }>((res) => {
          resolveClusters = (data) => res({ data });
        });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return new Promise<{ data: Topology }>((res) => {
          resolveTopology = (data) => res({ data });
        });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    renderOverview();

    expect(screen.getByTestId('overview-tree-pane')).toBeInTheDocument();
    expect(screen.getByTestId('overview-center-pane')).toBeInTheDocument();
    expect(screen.queryByTestId('overview-tree')).not.toBeInTheDocument();
    expect(screen.queryByTestId('rf-stub')).not.toBeInTheDocument();

    resolveClusters(makeClusters());
    resolveTopology(makeTopology());
  });

  it('renders ErrorState when the topology fetch fails', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.reject(new Error('boom'));
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    renderOverview();

    const errorState = await screen.findAllByTestId('error-state');
    expect(errorState.length).toBeGreaterThan(0);
    expect(screen.getAllByTestId('error-state-retry').length).toBeGreaterThan(0);
  });

  it('renders EmptyState when the topology has zero nodes', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: { nodes: [], edges: [] } satisfies Topology });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    renderOverview();

    const empty = await screen.findAllByTestId('empty-state');
    expect(empty.length).toBeGreaterThanOrEqual(1);
  });

  it('renders the tree + ReactFlow stub once topology data lands', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: makeTopology() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    renderOverview();

    expect(await screen.findByTestId('overview-tree')).toBeInTheDocument();
    expect(await screen.findByTestId('rf-stub')).toBeInTheDocument();
    expect(screen.getByTestId('rf-node-cluster-prod-a-01')).toBeInTheDocument();
    expect(screen.getByTestId('rf-node-node-1')).toBeInTheDocument();
    expect(screen.getByTestId('rf-node-npu-1-0')).toBeInTheDocument();
  });
});

describe('OverviewPage — interactions write the topology store', () => {
  it('clicking a tree node updates selectedNodeId in the Zustand store', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: makeTopology() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderOverview();

    await screen.findByTestId('overview-tree');
    const treePane = screen.getByTestId('overview-tree-pane');
    await user.click(within(treePane).getByText('npu-1-0'));

    await waitFor(() => {
      expect(useTopologyStore.getState().selectedNodeId).toBe('npu-1-0');
    });
  });

  it('clicking a ReactFlow node updates selectedNodeId in the Zustand store', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: makeTopology() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderOverview();

    await screen.findByTestId('rf-stub');
    await user.click(screen.getByTestId('rf-node-node-1'));

    await waitFor(() => {
      expect(useTopologyStore.getState().selectedNodeId).toBe('node-1');
    });
  });

  it('double-clicking an NPU toggles expandedNPUs and reveals slice children in the graph', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: makeTopologyWithSlice() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderOverview();

    await screen.findByTestId('rf-stub');

    // Default state: NPU not expanded → no slice nodes rendered in the graph.
    expect(screen.queryByTestId('rf-node-npu-1-0-slice-0')).not.toBeInTheDocument();
    expect(screen.queryByTestId('rf-node-npu-1-0-slice-1')).not.toBeInTheDocument();
    expect(useTopologyStore.getState().expandedNPUs.size).toBe(0);

    await user.dblClick(screen.getByTestId('rf-node-npu-1-0'));

    // After dbl-click the store should hold the NPU id and the graph
    // should now render the two slice buttons.
    await waitFor(() => {
      expect(useTopologyStore.getState().expandedNPUs.has('npu-1-0')).toBe(true);
    });
    await waitFor(() => {
      expect(screen.getByTestId('rf-node-npu-1-0-slice-0')).toBeInTheDocument();
      expect(screen.getByTestId('rf-node-npu-1-0-slice-1')).toBeInTheDocument();
    });
  });
});

describe('OverviewPage — fabric toggle (P1-T-212 / ADR-0004)', () => {
  it('defaults the fabric toggle to OFF and omits includeFabric from the topology URL', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: makeTopology() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    renderOverview();

    // Toggle exists and starts unchecked.
    const toggle = await screen.findByTestId('fabric-toggle-switch');
    expect(toggle).toHaveAttribute('aria-checked', 'false');
    expect(useTopologyStore.getState().showFabric).toBe(false);

    // Wait for the topology fetch to land, then assert no call carried
    // includeFabric=true (zero-regression check vs T-108a/T-108b).
    await screen.findByTestId('rf-stub');
    const calls = mockGet.mock.calls.filter(([url]) =>
      String(url).endsWith('/topology'),
    );
    expect(calls.length).toBeGreaterThan(0);
    for (const [, config] of calls) {
      const params = (config as { params?: Record<string, unknown> } | undefined)?.params;
      expect(params?.includeFabric).toBeUndefined();
    }
  });

  it('flipping the toggle ON triggers a topology fetch with includeFabric=true', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: makeTopologyWithFabric() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderOverview();

    // Wait for first (off) render to settle.
    await screen.findByTestId('rf-stub');

    // Flip the toggle on.
    await user.click(screen.getByTestId('fabric-toggle-switch'));

    await waitFor(() => {
      expect(useTopologyStore.getState().showFabric).toBe(true);
    });

    // A new fetch should fire with the includeFabric param.
    await waitFor(() => {
      const withFabric = mockGet.mock.calls.filter(([url, config]) => {
        if (!String(url).endsWith('/topology')) return false;
        const params = (config as { params?: Record<string, unknown> } | undefined)
          ?.params;
        return params?.includeFabric === true;
      });
      expect(withFabric.length).toBeGreaterThan(0);
    });
  });

  it('renders switch nodes when fabric data lands and the toggle is ON', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: makeTopologyWithFabric() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    // Pre-flip the store so the very first fetch already carries
    // includeFabric=true (avoids a second flip-driven render in the
    // assertion path).
    act(() => {
      useTopologyStore.setState({ showFabric: true });
    });

    renderOverview();

    // The switch node should be among the rendered ReactFlow stubs.
    await waitFor(() => {
      expect(screen.getByTestId('rf-node-switch-tor-01')).toBeInTheDocument();
    });

    // And the regular cluster / node / npu nodes should still render —
    // fabric is additive, not a replacement.
    expect(screen.getByTestId('rf-node-cluster-prod-a-01')).toBeInTheDocument();
    expect(screen.getByTestId('rf-node-node-1')).toBeInTheDocument();
    expect(screen.getByTestId('rf-node-npu-1-0')).toBeInTheDocument();
  });
});

describe('OverviewPage — workloads toggle (P1-T-214 / ADR-0005)', () => {
  it('defaults the workloads toggle to OFF and omits includeWorkloads from the topology URL', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: makeTopology() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    renderOverview();

    // Toggle exists and starts unchecked.
    const toggle = await screen.findByTestId('workloads-toggle-switch');
    expect(toggle).toHaveAttribute('aria-checked', 'false');
    expect(useTopologyStore.getState().showWorkloads).toBe(false);

    // Wait for the topology fetch to land, then assert no call carried
    // includeWorkloads=true (zero-regression check vs T-108a/T-108b/T-212).
    await screen.findByTestId('rf-stub');
    const calls = mockGet.mock.calls.filter(([url]) =>
      String(url).endsWith('/topology'),
    );
    expect(calls.length).toBeGreaterThan(0);
    for (const [, config] of calls) {
      const params = (config as { params?: Record<string, unknown> } | undefined)?.params;
      expect(params?.includeWorkloads).toBeUndefined();
    }
  });

  it('flipping the toggle ON triggers a topology fetch with includeWorkloads=true', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: makeTopologyWithWorkloads() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderOverview();

    // Wait for first (off) render to settle.
    await screen.findByTestId('rf-stub');

    // Flip the toggle on.
    await user.click(screen.getByTestId('workloads-toggle-switch'));

    await waitFor(() => {
      expect(useTopologyStore.getState().showWorkloads).toBe(true);
    });

    // A new fetch should fire with the includeWorkloads param.
    await waitFor(() => {
      const withWorkloads = mockGet.mock.calls.filter(([url, config]) => {
        if (!String(url).endsWith('/topology')) return false;
        const params = (config as { params?: Record<string, unknown> } | undefined)
          ?.params;
        return params?.includeWorkloads === true;
      });
      expect(withWorkloads.length).toBeGreaterThan(0);
    });
  });

  it('renders workload + pod nodes when workloads data lands and the toggle is ON', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: makeTopologyWithWorkloads() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    // Pre-flip the store so the very first fetch already carries
    // includeWorkloads=true (avoids a second flip-driven render in the
    // assertion path; mirrors the T-212 fabric pattern).
    act(() => {
      useTopologyStore.setState({ showWorkloads: true });
    });

    renderOverview();

    // The workload + pod nodes should be among the rendered ReactFlow stubs.
    await waitFor(() => {
      expect(
        screen.getByTestId('rf-node-workload/ai-inference/qwen-8b'),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByTestId('rf-node-pod/ai-inference/qwen-8b-prefill-0'),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId('rf-node-pod/ai-inference/qwen-8b-decode-0'),
    ).toBeInTheDocument();

    // And the regular cluster / node / npu nodes should still render —
    // workload fusion is additive. (Slice nodes are gated by the
    // `expandedNPUs` set so they don't render here even though they're
    // in the fixture; that's T-108b behaviour, orthogonal to T-214.)
    expect(screen.getByTestId('rf-node-cluster-prod-a-01')).toBeInTheDocument();
    expect(screen.getByTestId('rf-node-node-1')).toBeInTheDocument();
    expect(screen.getByTestId('rf-node-npu-1-0')).toBeInTheDocument();
  });
});

describe('DetailPanel — selection branches', () => {
  it('shows the empty placeholder when nothing is selected', () => {
    renderDetailPanel();
    expect(screen.getByTestId('detail-panel-empty')).toBeInTheDocument();
  });

  it('renders the cluster card when a cluster is selected', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: makeTopologyWithSlice() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    act(() => {
      useTopologyStore.setState({
        selectedClusterId: CLUSTER_ID,
        selectedNodeId: CLUSTER_ID,
      });
    });

    renderDetailPanel();

    expect(await screen.findByTestId('detail-panel-cluster')).toBeInTheDocument();
    expect(screen.getAllByText(CLUSTER_ID).length).toBeGreaterThan(0);
    expect(screen.getByText('edge-single')).toBeInTheDocument();
    expect(screen.getByText('Site-A')).toBeInTheDocument();
  });

  it('fetches /nodes/:name + /nodes/:name/npus when a node is selected', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: makeTopologyWithSlice() });
      }
      if (url === '/api/v1/nodes/node-1') {
        return Promise.resolve({ data: nodeDetailFixture() });
      }
      if (url === '/api/v1/nodes/node-1/npus') {
        return Promise.resolve({ data: npusFixture() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    act(() => {
      useTopologyStore.setState({
        selectedClusterId: CLUSTER_ID,
        selectedNodeId: 'node-1',
      });
    });

    renderDetailPanel();

    expect(await screen.findByTestId('detail-panel-node')).toBeInTheDocument();
    await waitFor(() => {
      expect(
        mockGet.mock.calls.some(([url]) => url === '/api/v1/nodes/node-1'),
      ).toBe(true);
      expect(
        mockGet.mock.calls.some(([url]) => url === '/api/v1/nodes/node-1/npus'),
      ).toBe(true);
    });
    await waitFor(() => {
      expect(screen.getByText(/npu-1-0 · healthy/)).toBeInTheDocument();
      expect(screen.getByText(/npu-1-1 · degraded/)).toBeInTheDocument();
    });
  });

  it('renders the NPU card from topology attributes (no extra fetch)', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: makeTopologyWithSlice() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    act(() => {
      useTopologyStore.setState({
        selectedClusterId: CLUSTER_ID,
        selectedNodeId: 'npu-1-0',
      });
    });

    renderDetailPanel();

    expect(await screen.findByTestId('detail-panel-npu')).toBeInTheDocument();
    expect(screen.getByText('Ascend910B')).toBeInTheDocument();
    expect(screen.getByText('fixed-template')).toBeInTheDocument();
    expect(screen.getByText(/npu-1-0-slice-0 · available/)).toBeInTheDocument();
    expect(screen.getByText(/npu-1-0-slice-1 · allocated/)).toBeInTheDocument();

    expect(
      mockGet.mock.calls.find(([url]) =>
        String(url).startsWith('/api/v1/nodes/'),
      ),
    ).toBeUndefined();
  });

  it('renders the slice card when a slice is selected (no extra fetch)', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/clusters') {
        return Promise.resolve({ data: makeClusters() });
      }
      if (url.startsWith('/api/v1/clusters/') && url.endsWith('/topology')) {
        return Promise.resolve({ data: makeTopologyWithSlice() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    act(() => {
      useTopologyStore.setState({
        selectedClusterId: CLUSTER_ID,
        selectedNodeId: 'npu-1-0-slice-1',
      });
    });

    renderDetailPanel();

    expect(await screen.findByTestId('detail-panel-slice')).toBeInTheDocument();
    // parentNPU + template come from the slice's attributes.
    expect(screen.getByText('npu-1-0')).toBeInTheDocument();
    expect(screen.getByText('vir04')).toBeInTheDocument();
    expect(screen.getByText('default/pod-1')).toBeInTheDocument();

    expect(
      mockGet.mock.calls.find(([url]) =>
        String(url).startsWith('/api/v1/nodes/'),
      ),
    ).toBeUndefined();
  });
});

describe('useTopologyWS', () => {
  it('opens a socket and reaches "open" state after handshake', async () => {
    const { result } = renderHook(() => useTopologyWS('cluster-1'), {
      wrapper: wsWrapper(),
    });

    await waitFor(() => {
      expect(wsInstances.length).toBeGreaterThan(0);
    });
    expect(wsInstances[0]!.url).toMatch(/\/ws\/topology$/);

    act(() => {
      wsInstances[0]!.__triggerOpen();
    });

    await waitFor(() => {
      expect(result.current.status).toBe('open');
    });
  });

  it('dispatches topology.update — invalidates cache + updates lastEventAt', async () => {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
    });
    const invalidate = vi.spyOn(qc, 'invalidateQueries');

    renderHook(() => useTopologyWS('cluster-1'), { wrapper: wsWrapper(qc) });

    await waitFor(() => expect(wsInstances.length).toBe(1));
    act(() => wsInstances[0]!.__triggerOpen());

    act(() => {
      wsInstances[0]!.__triggerMessage({
        type: 'topology.update',
        timestamp: '2026-05-17T12:00:00Z',
        payload: { nodes: [], edges: [] },
      });
    });

    await waitFor(() => {
      expect(invalidate).toHaveBeenCalledWith({
        queryKey: ['cluster-topology', 'cluster-1'],
      });
      expect(useTopologyStore.getState().lastEventAt).toBe(
        '2026-05-17T12:00:00Z',
      );
    });
  });

  it('auto-reconnects within 3s after close', async () => {
    vi.useFakeTimers();
    try {
      renderHook(
        () => useTopologyWS('cluster-1', { reconnectDelayMs: 3_000 }),
        { wrapper: wsWrapper() },
      );

      expect(wsInstances.length).toBe(1);
      act(() => wsInstances[0]!.__triggerOpen());
      act(() => wsInstances[0]!.__triggerClose());

      await act(async () => {
        vi.advanceTimersByTime(3_000);
      });

      expect(wsInstances.length).toBeGreaterThan(1);
      expect(wsInstances[1]!.url).toMatch(/\/ws\/topology$/);
    } finally {
      vi.useRealTimers();
    }
  });
});
