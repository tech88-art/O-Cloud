/**
 * P1-T-009 — Shared POC fixture for G6 vs ReactFlow comparison.
 *
 * Single source of truth for the 10-node tree + edges used by both
 * `G6POC.tsx` and `ReactFlowPOC.tsx`. Matches the 4-level hierarchy from
 * `docs/architecture.md` §8.2: cluster → node → npu → slice. Node-type
 * choices and status enums mirror the future production data model so
 * the winning library's POC can be lifted into P1-T-108a with minimal
 * reshaping.
 *
 * The library-specific adapters live in the POC components themselves —
 * this file stays library-agnostic on purpose so we can swap one POC
 * without touching the dataset.
 */
export type NodeType = 'cluster' | 'node' | 'npu' | 'slice';
export type NodeStatus = 'idle' | 'allocated' | 'error' | 'healthy';

export interface PocNode {
  /** Stable id used as React/G6 node key + edge endpoint. */
  id: string;
  /** Human label shown on the node. Latin only — POC is English-first. */
  label: string;
  /** One of the 4 architecture.md §8.2 node types. */
  type: NodeType;
  /** Status drives node colour. */
  status: NodeStatus;
}

export interface PocEdge {
  /** Edge id; both libs require unique edge ids. */
  id: string;
  /** Parent node id (cluster → node → npu → slice). */
  source: string;
  /** Child node id. */
  target: string;
}

export interface PocFixture {
  nodes: PocNode[];
  edges: PocEdge[];
}

/**
 * 10-node tree: 1 cluster + 2 nodes + 3 NPUs + 4 slices.
 * Mix covers every node type from §8.2 plus all 3 status colours.
 */
export const POC_FIXTURE: PocFixture = {
  nodes: [
    { id: 'cluster-a', label: 'cluster-a', type: 'cluster', status: 'healthy' },
    { id: 'node-1', label: 'node-1', type: 'node', status: 'healthy' },
    { id: 'node-2', label: 'node-2', type: 'node', status: 'healthy' },
    { id: 'npu-1-0', label: 'npu-1-0', type: 'npu', status: 'allocated' },
    { id: 'npu-1-1', label: 'npu-1-1', type: 'npu', status: 'idle' },
    { id: 'npu-2-0', label: 'npu-2-0', type: 'npu', status: 'error' },
    { id: 'slice-1-0-a', label: 'slice-a', type: 'slice', status: 'allocated' },
    { id: 'slice-1-0-b', label: 'slice-b', type: 'slice', status: 'idle' },
    { id: 'slice-1-1-a', label: 'slice-a', type: 'slice', status: 'idle' },
    { id: 'slice-2-0-a', label: 'slice-a', type: 'slice', status: 'error' },
  ],
  edges: [
    { id: 'e1', source: 'cluster-a', target: 'node-1' },
    { id: 'e2', source: 'cluster-a', target: 'node-2' },
    { id: 'e3', source: 'node-1', target: 'npu-1-0' },
    { id: 'e4', source: 'node-1', target: 'npu-1-1' },
    { id: 'e5', source: 'node-2', target: 'npu-2-0' },
    { id: 'e6', source: 'npu-1-0', target: 'slice-1-0-a' },
    { id: 'e7', source: 'npu-1-0', target: 'slice-1-0-b' },
    { id: 'e8', source: 'npu-1-1', target: 'slice-1-1-a' },
    { id: 'e9', source: 'npu-2-0', target: 'slice-2-0-a' },
  ],
};

/** Status → fill colour. Same palette both libs use, no surprises in compare. */
export const STATUS_COLOR: Record<NodeStatus, string> = {
  healthy: '#52c41a',
  idle: '#1677ff',
  allocated: '#faad14',
  error: '#ff4d4f',
};

/** Type → node size (radius in px). Larger upstream, smaller leaves. */
export const TYPE_SIZE: Record<NodeType, number> = {
  cluster: 48,
  node: 40,
  npu: 32,
  slice: 24,
};

/**
 * Generate an N-node stress fixture (cluster + N-1 ring of nodes).
 * Used for the 100/1000-node FPS test row in the comparison table.
 * Tree-ish but flat enough that force-layout produces a clear ring.
 */
export function makeStressFixture(nodeCount: number): PocFixture {
  if (nodeCount < 1) {
    return { nodes: [], edges: [] };
  }
  const nodes: PocNode[] = [
    { id: 'stress-cluster', label: 'cluster', type: 'cluster', status: 'healthy' },
  ];
  const edges: PocEdge[] = [];
  for (let i = 1; i < nodeCount; i++) {
    const status: NodeStatus = i % 3 === 0 ? 'error' : i % 2 === 0 ? 'idle' : 'allocated';
    const type: NodeType = i % 7 === 0 ? 'node' : i % 3 === 0 ? 'npu' : 'slice';
    nodes.push({ id: `stress-${i}`, label: `n${i}`, type, status });
    edges.push({ id: `stress-e${i}`, source: 'stress-cluster', target: `stress-${i}` });
  }
  return { nodes, edges };
}
