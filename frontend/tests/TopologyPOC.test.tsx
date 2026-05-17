import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { POC_FIXTURE, makeStressFixture } from '@/components/TopologyGraph/poc-fixture';

/**
 * P1-T-009 — Smoke tests for both topology POCs.
 *
 * jsdom does NOT implement HTMLCanvasElement.getContext (G6 needs it for
 * Canvas2D rendering) nor ResizeObserver (ReactFlow uses it to track its
 * container). Installing `canvas` as a peer-dep would balloon the test
 * env and only test the third-party library, not our POC code.
 *
 * Strategy: mock the two libraries at the module boundary BEFORE
 * importing the POC components. The mocks render a stub <div> with the
 * test-ids the production component would expose, so we verify:
 *   - POC components import without error (catches missing peer deps,
 *     CSS-side imports, version-skew issues)
 *   - The shared fixture contract holds (cluster / node / npu / slice
 *     plus the stress generator)
 *   - The POC page toggles between libs
 *
 * For pixel-level rendering verification we rely on `pnpm dev` + manual
 * walkthrough at `/poc/topology` (documented in the PR description).
 */

vi.mock('@antv/g6', () => {
  class FakeGraph {
    constructor(public opts: unknown) {}
    on() {
      return this;
    }
    render() {
      return Promise.resolve();
    }
    destroy() {}
  }
  return {
    Graph: FakeGraph,
    NodeEvent: { CLICK: 'node:click', DBLCLICK: 'node:dblclick' },
  };
});

vi.mock('@xyflow/react', () => {
  // Stub a ReactFlowProvider passthrough + a ReactFlow div so the POC
  // renders its header but doesn't reach into the real renderer.
  return {
    ReactFlow: ({ children }: { children?: React.ReactNode }) => (
      <div data-testid="rf-stub">{children}</div>
    ),
    ReactFlowProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
    Background: () => null,
    Controls: () => null,
    MiniMap: () => null,
  };
});

// CSS side-effect import is harmless under vitest+jsdom but vite's css
// transform isn't loaded here; stub to keep the import resolvable.
vi.mock('@xyflow/react/dist/style.css', () => ({}));

// Now safe to import the POC components.
const { default: G6POC } = await import('@/components/TopologyGraph/G6POC');
const { default: ReactFlowPOC } = await import('@/components/TopologyGraph/ReactFlowPOC');
const { default: POCPage } = await import('@/components/TopologyGraph/POCPage');

describe('POC fixture contract', () => {
  it('has exactly 10 nodes covering all 4 architecture.md §8.2 types', () => {
    expect(POC_FIXTURE.nodes).toHaveLength(10);
    const types = new Set(POC_FIXTURE.nodes.map((n) => n.type));
    expect(types).toEqual(new Set(['cluster', 'node', 'npu', 'slice']));
  });

  it('has 9 edges (10-node tree)', () => {
    expect(POC_FIXTURE.edges).toHaveLength(9);
  });

  it('every edge endpoint refers to a real node', () => {
    const ids = new Set(POC_FIXTURE.nodes.map((n) => n.id));
    for (const e of POC_FIXTURE.edges) {
      expect(ids.has(e.source)).toBe(true);
      expect(ids.has(e.target)).toBe(true);
    }
  });

  it('makeStressFixture produces requested node count', () => {
    const f = makeStressFixture(100);
    expect(f.nodes).toHaveLength(100);
    expect(f.edges).toHaveLength(99);
  });
});

describe('G6POC', () => {
  it('renders the POC header with FPS, node count, stress, and selection chrome', () => {
    render(<G6POC />);
    expect(screen.getByText(/G6 v5 POC/)).toBeInTheDocument();
    expect(screen.getByTestId('g6-fps')).toBeInTheDocument();
    expect(screen.getByTestId('g6-node-count')).toHaveTextContent('nodes: 10');
    expect(screen.getByTestId('g6-stress')).toBeInTheDocument();
    expect(screen.getByTestId('g6-selected')).toHaveTextContent('selected: —');
  });

  it('updates node count when stress level changes', async () => {
    const user = userEvent.setup();
    render(<G6POC />);
    await user.selectOptions(screen.getByTestId('g6-stress'), '100');
    expect(screen.getByTestId('g6-node-count')).toHaveTextContent('nodes: 100');
  });
});

describe('ReactFlowPOC', () => {
  it('renders the POC header with FPS, node count, stress, and selection chrome', () => {
    render(<ReactFlowPOC />);
    expect(screen.getByText(/ReactFlow v12 POC/)).toBeInTheDocument();
    expect(screen.getByTestId('rf-fps')).toBeInTheDocument();
    expect(screen.getByTestId('rf-node-count')).toHaveTextContent('nodes: 10');
    expect(screen.getByTestId('rf-stress')).toBeInTheDocument();
    expect(screen.getByTestId('rf-selected')).toHaveTextContent('selected: —');
  });

  it('updates node count when stress level changes', async () => {
    const user = userEvent.setup();
    render(<ReactFlowPOC />);
    await user.selectOptions(screen.getByTestId('rf-stress'), '100');
    expect(screen.getByTestId('rf-node-count')).toHaveTextContent('nodes: 100');
  });
});

describe('POCPage', () => {
  it('starts on G6 tab and toggles to ReactFlow when clicked', async () => {
    const user = userEvent.setup();
    render(<POCPage />);
    expect(screen.getByText(/G6 v5 POC/)).toBeInTheDocument();
    await user.click(screen.getByTestId('tab-reactflow'));
    expect(screen.getByText(/ReactFlow v12 POC/)).toBeInTheDocument();
  });
});
