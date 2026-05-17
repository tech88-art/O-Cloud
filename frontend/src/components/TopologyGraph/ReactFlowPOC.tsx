import { useCallback, useMemo, useState } from 'react';
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  type Edge,
  type Node,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { POC_FIXTURE, STATUS_COLOR, TYPE_SIZE, makeStressFixture, type PocFixture } from './poc-fixture';
import { useFpsMeter } from './useFpsMeter';

/**
 * P1-T-009 — ReactFlow (xyflow) v12 topology POC.
 *
 * Demonstrates the same capability set as `G6POC.tsx`: nodes + edges,
 * click-to-highlight (we drive `selected` state ourselves so the data
 * model stays library-agnostic), pointer-driven zoom/pan via the
 * built-in <Controls> + viewport plumbing.
 *
 * Layout choice: ReactFlow does not ship a built-in layout engine — you
 * either compute positions yourself or pull in `dagre`/`elk`. For the POC
 * we hand-compute a tidy radial-ish layout from depth so the comparison
 * is fair (we are not measuring layout quality, just rendering).
 *
 * Performance: same stress-level toggle as the G6 POC (10 / 100 / 800).
 */
type StressLevel = '10' | '100' | '800';

const STRESS_SIZES: Record<StressLevel, number> = {
  '10': 0,
  '100': 100,
  '800': 800,
};

function pickFixture(level: StressLevel): PocFixture {
  return STRESS_SIZES[level] === 0 ? POC_FIXTURE : makeStressFixture(STRESS_SIZES[level]);
}

/**
 * Compute (x, y) per node from parent-child depth so ReactFlow has
 * positions to render. Naive but produces a readable tree at 10 nodes
 * and an interpretable cloud at 100+ nodes.
 */
function layout(fixture: PocFixture): Map<string, { x: number; y: number }> {
  const depthOf = new Map<string, number>();
  const childrenOf = new Map<string, string[]>();
  for (const e of fixture.edges) {
    const arr = childrenOf.get(e.source) ?? [];
    arr.push(e.target);
    childrenOf.set(e.source, arr);
  }
  const isChild = new Set(fixture.edges.map((e) => e.target));
  const roots = fixture.nodes.filter((n) => !isChild.has(n.id));
  for (const r of roots) {
    depthOf.set(r.id, 0);
  }
  // BFS to assign depth (cycles unexpected for the POC fixture).
  const queue = roots.map((r) => r.id);
  while (queue.length > 0) {
    const id = queue.shift();
    if (id === undefined) {
      break;
    }
    const d = depthOf.get(id) ?? 0;
    for (const c of childrenOf.get(id) ?? []) {
      if (!depthOf.has(c)) {
        depthOf.set(c, d + 1);
        queue.push(c);
      }
    }
  }
  // Group by depth, then assign x by index within the depth bucket.
  const byDepth = new Map<number, string[]>();
  for (const n of fixture.nodes) {
    const d = depthOf.get(n.id) ?? 0;
    const arr = byDepth.get(d) ?? [];
    arr.push(n.id);
    byDepth.set(d, arr);
  }
  const positions = new Map<string, { x: number; y: number }>();
  const xSpacing = 120;
  const ySpacing = 110;
  for (const [d, ids] of byDepth.entries()) {
    const xOffset = -((ids.length - 1) * xSpacing) / 2;
    ids.forEach((id, i) => {
      positions.set(id, { x: xOffset + i * xSpacing, y: d * ySpacing });
    });
  }
  return positions;
}

function toRFData(fixture: PocFixture): { nodes: Node[]; edges: Edge[] } {
  const positions = layout(fixture);
  const nodes: Node[] = fixture.nodes.map((n) => {
    const pos = positions.get(n.id) ?? { x: 0, y: 0 };
    const size = TYPE_SIZE[n.type];
    return {
      id: n.id,
      position: pos,
      data: { label: n.label, kind: n.type, status: n.status },
      style: {
        background: STATUS_COLOR[n.status],
        color: '#fff',
        border: '1.5px solid #fff',
        borderRadius: '50%',
        width: size,
        height: size,
        fontSize: 10,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        textAlign: 'center',
      },
    };
  });
  const edges: Edge[] = fixture.edges.map((e) => ({
    id: e.id,
    source: e.source,
    target: e.target,
    style: { stroke: '#bfbfbf', strokeWidth: 1 },
  }));
  return { nodes, edges };
}

export interface ReactFlowPOCProps {
  fixture?: PocFixture;
  onNodeClick?: (nodeId: string) => void;
}

function ReactFlowPOCInner({ fixture, onNodeClick }: ReactFlowPOCProps) {
  const [stressLevel, setStressLevel] = useState<StressLevel>('10');
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const { fps } = useFpsMeter();

  const activeFixture = fixture ?? pickFixture(stressLevel);
  const { nodes: baseNodes, edges } = useMemo(() => toRFData(activeFixture), [activeFixture]);

  // Apply selection-driven outline so click → visible highlight without
  // hooking ReactFlow's internal selection store (keeps POC honest about
  // event ergonomics).
  const nodes = useMemo(
    () =>
      baseNodes.map((n) =>
        n.id === selectedId
          ? { ...n, style: { ...n.style, boxShadow: '0 0 0 3px #1d39c4' } }
          : n,
      ),
    [baseNodes, selectedId],
  );

  const handleNodeClick = useCallback(
    (_event: React.MouseEvent, node: Node) => {
      setSelectedId(node.id);
      onNodeClick?.(node.id);
    },
    [onNodeClick],
  );

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
        <strong>ReactFlow v12 POC</strong>
        <span data-testid="rf-fps">FPS: {fps}</span>
        <span data-testid="rf-node-count">nodes: {nodes.length}</span>
        <label>
          stress:&nbsp;
          <select
            value={stressLevel}
            onChange={(e) => setStressLevel(e.target.value as StressLevel)}
            data-testid="rf-stress"
          >
            <option value="10">10</option>
            <option value="100">100</option>
            <option value="800">800</option>
          </select>
        </label>
        <span data-testid="rf-selected">selected: {selectedId ?? '—'}</span>
      </header>
      <div
        data-testid="rf-canvas"
        style={{ flex: 1, minHeight: 360, border: '1px solid #f0f0f0', borderRadius: 4 }}
      >
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodeClick={handleNodeClick}
          fitView
          minZoom={0.1}
          maxZoom={2}
        >
          <Background />
          <Controls />
          <MiniMap pannable zoomable />
        </ReactFlow>
      </div>
    </div>
  );
}

export function ReactFlowPOC(props: ReactFlowPOCProps) {
  // ReactFlowProvider is required for the hooks to work; wrap once so
  // the POC is drop-in-mountable at the route.
  return (
    <ReactFlowProvider>
      <ReactFlowPOCInner {...props} />
    </ReactFlowProvider>
  );
}

export default ReactFlowPOC;
