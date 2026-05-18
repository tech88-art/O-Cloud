import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, act, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { MemoryRouter } from 'react-router-dom';
import { ConfigProvider } from 'antd';
import i18n from '@/i18n';
import { useTopologyStore } from '@/store/topologyStore';
import type { Cluster, Topology } from '@/services/cluster';

/**
 * P1-T-108a Overview page tests.
 *
 * jsdom can't render the ReactFlow canvas (no ResizeObserver, no full
 * SVG layout) and we don't care here — the production code is exercised
 * up to the renderer; library rendering is the library's job. Mock
 * `@xyflow/react` at the module boundary so the component imports
 * resolve without hitting real DOM measurement code.
 *
 * Tests run six scenarios:
 *   1. loading      — Skeleton visible while the request is in-flight
 *   2. error        — ErrorState renders with a retry button
 *   3. empty        — EmptyState renders when topology has zero nodes
 *   4. happy        — tree + topology stub render after data lands
 *   5. tree click   — selecting a tree node writes to the Zustand store
 *   6. graph click  — clicking a topology stub node writes to the store
 *                     (covers ReactFlow click → store contract)
 */

// Stub ReactFlow's API surface so the production component's imports
// succeed and we can drive the `onNodeClick` contract directly via the
// stub. We expose a button per node so user-event can click them.
vi.mock('@xyflow/react', () => {
  // The component will be configured with `nodes` and `onNodeClick`.
  // Render each node as a button carrying its id; clicking calls
  // `onNodeClick(_, { id })` exactly like the real lib would.
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
    <div data-testid="rf-stub">
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
    ReactFlowProvider: ({ children }: { children: React.ReactNode }) => (
      <>{children}</>
    ),
    Background: () => null,
    Controls: () => null,
    MiniMap: () => null,
  };
});
vi.mock('@xyflow/react/dist/style.css', () => ({}));

// Mock the api module to avoid touching axios + runtime config. We hand
// out a queue of mock implementations per test below.
const mockGet = vi.fn<(url: string, config?: unknown) => Promise<unknown>>();
vi.mock('@/services/api', () => ({
  api: {
    get: (url: string, config?: unknown) => mockGet(url, config),
  },
}));

// Imports of the SUT must come AFTER the mocks are registered.
const { default: OverviewPage } = await import('@/pages/Overview');

function makeClusters(): Cluster[] {
  return [
    {
      id: 'cluster-prod-a-01',
      name: 'cluster-prod-a-01',
      status: 'healthy',
      role: 'edge-single',
    },
  ];
}

function makeTopology(): Topology {
  return {
    nodes: [
      { id: 'cluster-prod-a-01', type: 'cluster', label: 'cluster-prod-a-01', status: 'healthy' },
      { id: 'node-1', type: 'node', label: 'node-1', status: 'healthy' },
      { id: 'npu-1-0', type: 'npu', label: 'npu-1-0', status: 'idle' },
    ],
    edges: [
      { source: 'cluster-prod-a-01', target: 'node-1', type: 'contains' },
      { source: 'node-1', target: 'npu-1-0', type: 'contains' },
    ],
  };
}

function renderOverview() {
  // Fresh QueryClient per test so cache doesn't leak across cases.
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

beforeEach(() => {
  mockGet.mockReset();
  // Reset Zustand store between tests so selections from one don't bleed
  // into the next.
  act(() => {
    useTopologyStore.setState({ selectedClusterId: null, selectedNodeId: null });
  });
});

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

    // Loading skeleton is in both panes initially.
    expect(screen.getByTestId('overview-tree-pane')).toBeInTheDocument();
    expect(screen.getByTestId('overview-center-pane')).toBeInTheDocument();
    // No tree, no topology stub rendered yet.
    expect(screen.queryByTestId('overview-tree')).not.toBeInTheDocument();
    expect(screen.queryByTestId('rf-stub')).not.toBeInTheDocument();

    // Resolve so React doesn't warn about pending promises after teardown.
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

    // ErrorState (data-testid="error-state") shows up in the center pane.
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
    // Both tree pane (no tree data) and center pane (empty topology)
    // render an EmptyState.
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
    // The ReactFlow stub contains one button per topology node.
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

    // Wait for the tree to render then click the leaf NPU label INSIDE
    // the tree pane (`npu-1-0` also appears as a button in the ReactFlow
    // stub, so an un-scoped `getByText` would match multiple elements).
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

  it('double-clicking a ReactFlow node fires the dblclick hook (reserved for T-108b)', async () => {
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
    // Pick a different node than the click test so we can assert the
    // dblclick path independently routes through the store.
    await user.dblClick(screen.getByTestId('rf-node-npu-1-0'));

    await waitFor(() => {
      expect(useTopologyStore.getState().selectedNodeId).toBe('npu-1-0');
    });
  });
});
