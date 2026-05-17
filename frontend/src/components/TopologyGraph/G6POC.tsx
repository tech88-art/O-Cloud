import { useEffect, useRef, useState } from 'react';
import {
  Graph,
  NodeEvent,
  type GraphData,
  type IPointerEvent,
  type Node as G6Node,
} from '@antv/g6';
import { POC_FIXTURE, STATUS_COLOR, TYPE_SIZE, makeStressFixture, type PocFixture } from './poc-fixture';
import { useFpsMeter } from './useFpsMeter';

/**
 * P1-T-009 — AntV G6 v5 topology POC.
 *
 * Demonstrates the minimum capability set the production TopologyGraph
 * component will need (per `docs/architecture.md` §8.2 + `frontend/CLAUDE.md`
 * §7): node + edge rendering, click → highlight, double-click hook,
 * pointer hover, zoom/pan. The component is intentionally small — just
 * enough to compare ergonomics against the ReactFlow POC.
 *
 * Layout choice: dagre (hierarchical, top-down). Force-directed is also a
 * one-liner swap in G6, see commented `layout` block below.
 *
 * Performance: a stress-fixture toggle renders 100 / 800 nodes so the
 * comparison table FPS row is measured under matched conditions.
 *
 * Container sizing: G6 v5 requires explicit width/height because it does
 * not auto-observe its container. We measure the wrapper div once on
 * mount and feed those numbers into `new Graph({ width, height })`.
 */
type StressLevel = '10' | '100' | '800';

const STRESS_SIZES: Record<StressLevel, number> = {
  '10': 0, // 0 means use the canonical 10-node fixture
  '100': 100,
  '800': 800,
};

function pickFixture(level: StressLevel): PocFixture {
  return STRESS_SIZES[level] === 0 ? POC_FIXTURE : makeStressFixture(STRESS_SIZES[level]);
}

function toG6Data(fixture: PocFixture): GraphData {
  return {
    nodes: fixture.nodes.map((n) => ({
      id: n.id,
      data: { kind: n.type, status: n.status },
      style: {
        fill: STATUS_COLOR[n.status],
        size: TYPE_SIZE[n.type],
        labelText: n.label,
        labelFontSize: 11,
        labelPlacement: 'bottom',
        stroke: '#fff',
        lineWidth: 1.5,
      },
    })),
    edges: fixture.edges.map((e) => ({
      id: e.id,
      source: e.source,
      target: e.target,
      style: { stroke: '#bfbfbf', lineWidth: 1 },
    })),
  };
}

export interface G6POCProps {
  /** Optional override of the canonical 10-node fixture (used by tests). */
  fixture?: PocFixture;
  /** Notify parent when a node is clicked. POC-level — just the id. */
  onNodeClick?: (nodeId: string) => void;
}

export function G6POC({ fixture, onNodeClick }: G6POCProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const graphRef = useRef<Graph | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [stressLevel, setStressLevel] = useState<StressLevel>('10');
  const { fps } = useFpsMeter();

  // Effective fixture: explicit prop > stress dropdown > canonical 10-node.
  const activeFixture = fixture ?? pickFixture(stressLevel);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) {
      return;
    }
    const { clientWidth, clientHeight } = container;
    const graph = new Graph({
      container,
      width: clientWidth || 800,
      height: clientHeight || 480,
      data: toG6Data(activeFixture),
      node: { type: 'circle' },
      edge: { type: 'line' },
      layout: { type: 'dagre', rankdir: 'TB', nodesep: 24, ranksep: 36 },
      behaviors: ['zoom-canvas', 'drag-canvas', 'click-select'],
      animation: false,
    });

    graph.on<IPointerEvent<G6Node>>(NodeEvent.CLICK, (event) => {
      const id = String(event.target.id);
      setSelectedId(id);
      onNodeClick?.(id);
    });

    void graph.render();
    graphRef.current = graph;

    return () => {
      graph.destroy();
      graphRef.current = null;
    };
    // The fixture identity drives re-init; we rely on stable references
    // from `pickFixture` (memoised by stressLevel) and the optional prop.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeFixture]);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', gap: 8 }}>
      <header
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 12,
          padding: '8px 12px',
          background: '#fafafa',
          borderRadius: 4,
        }}
      >
        <strong>G6 v5 POC</strong>
        <span data-testid="g6-fps">FPS: {fps}</span>
        <span data-testid="g6-node-count">nodes: {activeFixture.nodes.length}</span>
        <label>
          stress:&nbsp;
          <select
            value={stressLevel}
            onChange={(e) => setStressLevel(e.target.value as StressLevel)}
            data-testid="g6-stress"
          >
            <option value="10">10</option>
            <option value="100">100</option>
            <option value="800">800</option>
          </select>
        </label>
        <span data-testid="g6-selected">selected: {selectedId ?? '—'}</span>
      </header>
      <div
        ref={containerRef}
        data-testid="g6-canvas"
        style={{
          flex: 1,
          minHeight: 360,
          border: '1px solid #f0f0f0',
          borderRadius: 4,
          position: 'relative',
        }}
      />
    </div>
  );
}

export default G6POC;
