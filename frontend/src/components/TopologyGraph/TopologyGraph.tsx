import { memo, useCallback, useMemo } from 'react';
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  type Edge,
  type Node,
  type NodeMouseHandler,
  type NodeProps,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import dagre from 'dagre';
import { StatusTag, type StatusTone } from '@/components/StatusTag';
import type { Topology, TopologyEdge, TopologyNode } from '@/services/cluster';

/**
 * ADR-0004 / RFC-003 fabric extension.
 *
 * The backend now emits two extra wire values that the auto-generated
 * `services/types.ts` doesn't yet know about (the OpenAPI spec is
 * regenerated separately — see RFC-003 §schema):
 *   - TopologyNode.type === 'switch'       → an inter-node network switch
 *   - TopologyEdge.type === 'fabric-link'  → a node↔switch / switch↔switch
 *                                            link in the inter-node fabric
 *
 * Until the contract regen lands we treat them as runtime-only string
 * variants. We narrow with `typeof n.type === 'string'` + literal compare
 * at the render boundary; the TopologyGraph never trusts the generated
 * union alone, so the component is forward-compatible with the contract
 * regen (no churn when the union gets the extra members).
 */
const NODE_TYPE_SWITCH = 'switch' as const;
const EDGE_TYPE_FABRIC_LINK = 'fabric-link' as const;

/** Widened topology-node `type` admitting the runtime-only `switch` literal. */
type TopologyNodeType = TopologyNode['type'] | typeof NODE_TYPE_SWITCH;
/** Widened topology-edge `type` admitting the runtime-only `fabric-link`. */
type TopologyEdgeType = TopologyEdge['type'] | typeof EDGE_TYPE_FABRIC_LINK;

/**
 * Production topology graph wrapper around ReactFlow. Replaces the
 * P1-T-009 POCs (`G6POC` / `ReactFlowPOC`) that compared library candidates.
 *
 * Why ReactFlow won:
 *   - Pure React; G6 v5 wants imperative DOM ownership.
 *   - Easier to drop custom AntD-styled node renderers (see `TopoNode` below).
 *   - Better TS types out of the box.
 *
 * Layout: ReactFlow ships no built-in layouter; we use `dagre` (top-down,
 * `rankdir: 'TB'`) so the cluster → node → npu → slice hierarchy reads
 * naturally for the demo storyline. Pure function — call once per topology
 * payload, memoised in the parent.
 *
 * Selection: this component is **controlled** — pass `selectedNodeId` from
 * the parent (driven by the Zustand `topologyStore`) and we render a
 * highlight ring on the matching node. We do NOT use ReactFlow's internal
 * selection so we keep the data model library-agnostic and the Zustand
 * store remains the single source of truth (T-108b WS updates will write
 * here too).
 *
 * Events: `onNodeClick` fires on single-click; `onNodeDoubleClick` is
 * surfaced for T-108b's slice-expand hook (T-108a does not implement
 * expansion, just keeps the event plumbed through).
 */

const NODE_WIDTH = 160;
const NODE_HEIGHT = 64;
/** dagre uses these to space nodes within a rank (`nodesep`) and between
 *  ranks (`ranksep`). Tuned so a 24-NPU cluster reads cleanly at 1280px. */
const NODE_SEPARATION = 24;
const RANK_SEPARATION = 60;

/**
 * Type-keyed accent colors. AntD design tokens, kept here as constants so
 * the colors match the AntD theme without pulling the full theme object.
 *
 * Cluster: blue-6 / Node: geekblue-6 / NPU: status-driven via `<StatusTag>` /
 * Slice: status-driven via `<StatusTag>` /
 * Switch (ADR-0004): AntD blue-6, same family as cluster so it reads as a
 * "platform-level" element rather than an in-node resource. Status colour
 * (up/degraded/down) is conveyed through the node border ring — see
 * `switchBorderForStatus` below.
 */
const TYPE_ACCENT: Record<TopologyNodeType, string> = {
  cluster: '#1677ff',
  nodepool: '#722ed1',
  node: '#2f54eb',
  npu: '#13c2c2',
  slice: '#52c41a',
  network: '#8c8c8c',
  switch: '#1677ff',
};

/**
 * ADR-0004 switch status → border colour. Backend emits `up | degraded |
 * down`; everything else falls through to the neutral `#d9d9d9` border
 * also used by non-switch nodes (so the renderer is forgiving to an unknown
 * status string without crashing).
 *
 * Colours match AntD success / warning / error so the visual language
 * lines up with `<StatusTag>` even though we're not rendering one inside
 * the switch node (switch nodes are compact — name + status border ring).
 */
const SWITCH_STATUS_BORDER: Record<string, string> = {
  up: '#52c41a',
  degraded: '#faad14',
  down: '#ff4d4f',
};

function switchBorderForStatus(status: string | undefined, selected: boolean): string {
  if (selected) return '#1677ff';
  if (status && SWITCH_STATUS_BORDER[status]) return SWITCH_STATUS_BORDER[status];
  return '#d9d9d9';
}

/**
 * Slice-specific status → StatusTag tone mapping. Per T-108b AC:
 *   idle / available → success (AntD green)
 *   allocated / busy → processing (AntD blue)
 *   error / failed / faulty → error (AntD red)
 *   unknown / anything else → default (gray, picked up by StatusTag's fallback)
 *
 * The default StatusTag map covers most of these already, but it routes
 * `allocated` / `busy` to the default tone (gray); slices in the graph
 * read more clearly when the busy/allocated state is highlighted in
 * blue (matches the AntD primary), so we override that here.
 */
const SLICE_STATUS_MAP: Record<string, StatusTone> = {
  available: 'success',
  idle: 'success',
  allocated: 'processing',
  busy: 'processing',
  error: 'error',
  failed: 'error',
  faulty: 'error',
};

interface TopoNodeData {
  /** Original topology node — passed straight through so consumers can read
   *  attributes / status without re-deriving anything. Indexed by `[key:string]`
   *  to satisfy ReactFlow's `Record<string, unknown>` data constraint. */
  label: string;
  topoType: TopologyNodeType;
  status?: TopologyNode['status'];
  selected: boolean;
  [key: string]: unknown;
}

type TopoFlowNode = Node<TopoNodeData, 'topo'>;

/**
 * Custom node renderer. AntD-card-style layout: type accent stripe on top,
 * label in the middle, status tag at the bottom. Selection highlight is
 * a 3px ring matching AntD's primary blue.
 *
 * Switch nodes (ADR-0004) use a status-coloured border ring instead of
 * the StatusTag underneath — they're typically compact in the inter-node
 * fabric layer and the up/degraded/down state is the only thing that
 * matters at a glance, so a colour ring reads faster than a tag.
 *
 * The whole node is a single `<div>` so ReactFlow's click handlers still
 * fire on any sub-element.
 */
function TopoNode({ data }: NodeProps<TopoFlowNode>) {
  const accent = TYPE_ACCENT[data.topoType];
  const isSwitch = data.topoType === NODE_TYPE_SWITCH;
  const border = isSwitch
    ? switchBorderForStatus(data.status, data.selected)
    : data.selected
    ? '#1677ff'
    : '#d9d9d9';
  return (
    <div
      data-testid={`topo-node-${data.topoType}`}
      style={{
        width: NODE_WIDTH,
        height: NODE_HEIGHT,
        background: '#ffffff',
        border: `1.5px solid ${border}`,
        borderRadius: 6,
        boxShadow: data.selected
          ? '0 0 0 3px rgba(22, 119, 255, 0.25)'
          : '0 1px 2px rgba(0, 0, 0, 0.05)',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        fontSize: 12,
      }}
    >
      <div
        style={{
          height: 4,
          background: accent,
          flexShrink: 0,
        }}
      />
      <div
        style={{
          padding: '6px 10px',
          display: 'flex',
          flexDirection: 'column',
          gap: 4,
          flex: 1,
          minWidth: 0,
        }}
      >
        <div
          style={{
            fontWeight: 600,
            color: '#1f1f1f',
            whiteSpace: 'nowrap',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
          }}
          title={data.label}
        >
          {isSwitch ? `⚡ ${data.label}` : data.label}
        </div>
        {data.status && !isSwitch ? (
          <StatusTag
            status={data.status}
            mapping={data.topoType === 'slice' ? SLICE_STATUS_MAP : undefined}
          />
        ) : null}
      </div>
    </div>
  );
}

const NODE_TYPES = { topo: memo(TopoNode) };

/**
 * Filter a topology so that slice nodes only appear under NPUs that are
 * currently in `expandedNPUs`. Other node types (cluster / node / npu /
 * network) always render. Edges that reference filtered-out nodes are
 * dropped so dagre doesn't lay out dangling endpoints.
 *
 * Pure: same input → same output. Returns a fresh object; the input
 * topology is untouched.
 *
 * P1-T-108b dbl-click UX: a slice subtree appears under an NPU only
 * after the user dbl-clicks it. Default = collapsed so a 24-NPU /
 * 96-slice cluster doesn't drown the canvas.
 */
function filterTopologyForExpandedNPUs(
  topology: Topology,
  expandedNPUs: Set<string>,
): Topology {
  // Cheap path: no slice nodes → nothing to filter.
  const hasSlice = topology.nodes.some((n) => n.type === 'slice');
  if (!hasSlice) return topology;

  // 1. Index NPUs and their direct slice children via `contains` edges.
  const npuOfSlice = new Map<string, string>();
  for (const e of topology.edges) {
    if (e.type !== 'contains') continue;
    const child = topology.nodes.find((n) => n.id === e.target);
    if (!child || child.type !== 'slice') continue;
    npuOfSlice.set(e.target, e.source);
  }

  // 2. Drop slice nodes whose parent NPU is not expanded.
  const visibleNodeIds = new Set<string>();
  const nodes = topology.nodes.filter((n) => {
    if (n.type !== 'slice') {
      visibleNodeIds.add(n.id);
      return true;
    }
    const parent = npuOfSlice.get(n.id);
    const visible = !!parent && expandedNPUs.has(parent);
    if (visible) visibleNodeIds.add(n.id);
    return visible;
  });

  // 3. Drop edges that touch a filtered-out node.
  const edges = topology.edges.filter(
    (e) => visibleNodeIds.has(e.source) && visibleNodeIds.has(e.target),
  );

  return { ...topology, nodes, edges };
}

/**
 * Lay out a topology with dagre. Returns ReactFlow-ready nodes + edges.
 * Pure: same input → same output, no DOM access.
 */
function layoutWithDagre(topology: Topology, selectedNodeId: string | null): {
  nodes: TopoFlowNode[];
  edges: Edge[];
} {
  const g = new dagre.graphlib.Graph();
  g.setGraph({
    rankdir: 'TB',
    nodesep: NODE_SEPARATION,
    ranksep: RANK_SEPARATION,
    marginx: 16,
    marginy: 16,
  });
  g.setDefaultEdgeLabel(() => ({}));

  for (const n of topology.nodes) {
    g.setNode(n.id, { width: NODE_WIDTH, height: NODE_HEIGHT });
  }
  for (const e of topology.edges) {
    g.setEdge(e.source, e.target);
  }

  dagre.layout(g);

  const flowNodes: TopoFlowNode[] = topology.nodes.map((n) => {
    const pos = g.node(n.id);
    return {
      id: n.id,
      type: 'topo',
      // dagre returns the CENTER; ReactFlow expects top-left.
      position: pos
        ? { x: pos.x - NODE_WIDTH / 2, y: pos.y - NODE_HEIGHT / 2 }
        : { x: 0, y: 0 },
      data: {
        label: n.label,
        // Cast widens to TopologyNodeType; runtime values may include
        // 'switch' which the generated union doesn't yet name (see top
        // of file for the ADR-0004 contract-regen note).
        topoType: n.type as TopologyNodeType,
        status: n.status,
        selected: n.id === selectedNodeId,
      },
    };
  });

  const flowEdges: Edge[] = topology.edges.map((e, i) => {
    // Cast widens to TopologyEdgeType — see node `as TopologyNodeType`
    // comment for the rationale.
    const edgeType = e.type as TopologyEdgeType;
    const isFabricLink = edgeType === EDGE_TYPE_FABRIC_LINK;
    return {
      // Contract edges have no id; synthesise a stable one from endpoints.
      id: `${e.source}->${e.target}-${e.type}-${i}`,
      source: e.source,
      target: e.target,
      // Fabric links render in a blue-grey solid line (1.5px) so they read
      // as "platform-level" wiring distinct from the in-cluster contains
      // edges (light grey 1px). Both are solid — fabric is not a "weak"
      // relation, so a dashed style would mislead.
      style: isFabricLink
        ? { stroke: '#69b1ff', strokeWidth: 1.5 }
        : { stroke: '#bfbfbf', strokeWidth: 1 },
      data: { topoEdgeType: edgeType },
    };
  });

  return { nodes: flowNodes, edges: flowEdges };
}

export interface TopologyGraphProps {
  topology: Topology;
  selectedNodeId?: string | null;
  /**
   * Set of NPU ids whose slice subtree should be visible. Slice nodes whose
   * parent NPU is NOT in this set are filtered out before layout, so the
   * dbl-click UX in T-108b is just "toggle the parent id". Defaults to an
   * empty set (collapsed).
   */
  expandedNPUs?: ReadonlySet<string>;
  onNodeClick?: (id: string) => void;
  onNodeDoubleClick?: (id: string) => void;
}

// Module-level frozen empty set so the default never creates a new
// reference (which would defeat the `useMemo` shallow comparison).
const EMPTY_EXPANDED: ReadonlySet<string> = new Set<string>();

function TopologyGraphInner({
  topology,
  selectedNodeId = null,
  expandedNPUs = EMPTY_EXPANDED,
  onNodeClick,
  onNodeDoubleClick,
}: TopologyGraphProps) {
  const visibleTopology = useMemo(
    () => filterTopologyForExpandedNPUs(topology, new Set(expandedNPUs)),
    [topology, expandedNPUs],
  );

  const { nodes, edges } = useMemo(
    () => layoutWithDagre(visibleTopology, selectedNodeId),
    [visibleTopology, selectedNodeId],
  );

  const handleNodeClick = useCallback<NodeMouseHandler>(
    (_event, node) => {
      onNodeClick?.(node.id);
    },
    [onNodeClick],
  );

  const handleNodeDoubleClick = useCallback<NodeMouseHandler>(
    (_event, node) => {
      onNodeDoubleClick?.(node.id);
    },
    [onNodeDoubleClick],
  );

  return (
    <div data-testid="topology-graph" style={{ width: '100%', height: '100%' }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={NODE_TYPES}
        onNodeClick={handleNodeClick}
        onNodeDoubleClick={handleNodeDoubleClick}
        fitView
        minZoom={0.2}
        maxZoom={2}
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <Controls showInteractive={false} />
        <MiniMap pannable zoomable />
      </ReactFlow>
    </div>
  );
}

export function TopologyGraph(props: TopologyGraphProps) {
  return (
    <ReactFlowProvider>
      <TopologyGraphInner {...props} />
    </ReactFlowProvider>
  );
}

export default TopologyGraph;
