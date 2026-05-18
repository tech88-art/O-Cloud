import { memo, useCallback, useMemo } from 'react';
import {
  Background,
  Controls,
  MarkerType,
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
 * Topology node + edge type literals used by the renderer.
 *
 * Historically (T211 / T213 / T214) these were runtime-only strings the
 * generated `services/types.ts` union didn't name, and this file widened
 * the union via cast at the render boundary. ADR-0006 (2026-05-18)
 * regenerated the OpenAPI contract so the generated `TopologyNode['type']`
 * and `TopologyEdge['type']` unions now cover them natively; the
 * `as const` constants here remain as named anchors for switch/branch
 * logic (they're cheaper to grep than literal strings sprinkled inline).
 */
const NODE_TYPE_SWITCH = 'switch' as const;
const NODE_TYPE_WORKLOAD = 'workload' as const;
const NODE_TYPE_POD = 'pod' as const;
const EDGE_TYPE_FABRIC_LINK = 'fabric-link' as const;
const EDGE_TYPE_BINDS_TO = 'binds-to' as const;
const EDGE_TYPE_PD_PAIR = 'pd-pair' as const;

/** Topology-node `type`, sourced directly from the auto-gen contract. */
type TopologyNodeType = TopologyNode['type'];
/** Topology-edge `type`, sourced directly from the auto-gen contract. */
type TopologyEdgeType = TopologyEdge['type'];

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
/** Compact dimensions for pod nodes (ADR-0005). Pod nodes are the leaves
 *  of the workload-fusion subtree, often 2-3 per workload — keeping them a
 *  bit smaller than the default lets a 21-pod set-a-small fixture lay out
 *  cleanly alongside 24 NPU + 66 slice nodes without overflowing the
 *  canvas. Width chosen so a 50-char "qwen-8b-pd-decode-0" pod name
 *  truncates cleanly via the existing ellipsis. */
const NODE_WIDTH_POD = 140;
const NODE_HEIGHT_POD = 52;
/** Compact dimensions for NPU nodes. With 24 NPUs per cluster sitting in
 *  one dagre rank, the full-width 160px node would push the rank out to
 *  ~4400px and force `fitView` to ~0.3× zoom — labels turn unreadable.
 *  NPU labels are short ("Ascend910B#23") and don't carry a StatusTag
 *  string longer than "healthy", so 110×56 keeps the visual density at a
 *  readable default while leaving room for the accent stripe + tag. */
const NODE_WIDTH_NPU = 110;
const NODE_HEIGHT_NPU = 56;
/** dagre uses these to space nodes within a rank (`nodesep`) and between
 *  ranks (`ranksep`). Tuned so a 24-NPU cluster reads cleanly at 1280px. */
const NODE_SEPARATION = 24;
const RANK_SEPARATION = 60;

/**
 * Per-type render dimensions. Pod nodes use the compact size; NPU nodes
 * use a narrower variant so the 24-per-cluster rank fits at a readable
 * zoom; everything else gets the standard 160×64. The dagre layout and
 * the `<TopoNode>` renderer both consult this helper so they never
 * disagree on a node's footprint (which would produce overlap or
 * misaligned edges).
 */
function dimensionsForNodeType(type: TopologyNodeType): {
  width: number;
  height: number;
} {
  if (type === NODE_TYPE_POD) {
    return { width: NODE_WIDTH_POD, height: NODE_HEIGHT_POD };
  }
  if (type === 'npu') {
    return { width: NODE_WIDTH_NPU, height: NODE_HEIGHT_NPU };
  }
  return { width: NODE_WIDTH, height: NODE_HEIGHT };
}

/**
 * Type-keyed accent colors. AntD design tokens, kept here as constants so
 * the colors match the AntD theme without pulling the full theme object.
 *
 * Cluster: blue-6 / Node: geekblue-6 / NPU: cyan-6 / Slice: green-6 /
 * Network: grey-6 /
 * Switch (ADR-0004): AntD blue-6, same family as cluster so it reads as a
 * "platform-level" element rather than an in-node resource. Status colour
 * (up/degraded/down) is conveyed through the node border ring — see
 * `switchBorderForStatus` below.
 * Workload (ADR-0005): AntD grey-6, intentionally muted so the workload
 * aggregate doesn't visually compete with the infrastructure stripe colours.
 * Pod (ADR-0005): AntD grey-5, one step lighter than workload so the
 * parent/child relation reads even when the dagre layout puts them in
 * unrelated ranks (backend doesn't emit a workload→pod contains edge —
 * see ADR-0005 §Schema).
 */
const TYPE_ACCENT: Record<TopologyNodeType, string> = {
  cluster: '#1677ff',
  nodepool: '#722ed1',
  node: '#2f54eb',
  npu: '#13c2c2',
  slice: '#52c41a',
  network: '#8c8c8c',
  switch: '#1677ff',
  workload: '#8c8c8c',
  pod: '#bfbfbf',
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
 * Workload nodes (ADR-0005) get a ⚙ prefix and a regular StatusTag.
 * Pod nodes (ADR-0005) use the compact dimensions from
 * `dimensionsForNodeType` plus tighter padding so the smaller card still
 * fits the label + StatusTag. The accent stripe still encodes the
 * workload/pod distinction by colour.
 *
 * The whole node is a single `<div>` so ReactFlow's click handlers still
 * fire on any sub-element.
 */
function TopoNode({ data }: NodeProps<TopoFlowNode>) {
  const accent = TYPE_ACCENT[data.topoType];
  const isSwitch = data.topoType === NODE_TYPE_SWITCH;
  const isWorkload = data.topoType === NODE_TYPE_WORKLOAD;
  const isPod = data.topoType === NODE_TYPE_POD;
  const dims = dimensionsForNodeType(data.topoType);
  const border = isSwitch
    ? switchBorderForStatus(data.status, data.selected)
    : data.selected
    ? '#1677ff'
    : '#d9d9d9';
  // Pod cards are visibly smaller; squeeze padding + font so the label +
  // StatusTag still fit inside NODE_HEIGHT_POD (52px). Other node types
  // keep the standard 6px/10px padding + 12px font.
  const innerPadding = isPod ? '4px 8px' : '6px 10px';
  const labelFontSize = isPod ? 11 : 12;
  return (
    <div
      data-testid={`topo-node-${data.topoType}`}
      style={{
        width: dims.width,
        height: dims.height,
        background: '#ffffff',
        border: `1.5px solid ${border}`,
        borderRadius: 6,
        boxShadow: data.selected
          ? '0 0 0 3px rgba(22, 119, 255, 0.25)'
          : '0 1px 2px rgba(0, 0, 0, 0.05)',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        fontSize: labelFontSize,
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
          padding: innerPadding,
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
          {isSwitch
            ? `⚡ ${data.label}`
            : isWorkload
            ? `⚙ ${data.label}`
            : data.label}
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
 *
 * Edge styling table (kept here so the visual contract is greppable):
 *   - contains      → grey 1px solid       (default infrastructure tree)
 *   - fabric-link   → blue 1.5px solid     (ADR-0004 platform fabric)
 *   - binds-to      → cyan 1px solid       (ADR-0005 pod→slice resource bind)
 *   - pd-pair       → orange dashed + arrow + "PD" label (ADR-0005 P↔D)
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
    // Per-type dimensions — pod nodes are smaller (ADR-0005). The dagre
    // layout and the `<TopoNode>` renderer must consult the same helper
    // or the layout misaligns with the rendered card.
    g.setNode(n.id, dimensionsForNodeType(n.type));
  }
  for (const e of topology.edges) {
    g.setEdge(e.source, e.target);
  }

  dagre.layout(g);

  const flowNodes: TopoFlowNode[] = topology.nodes.map((n) => {
    const pos = g.node(n.id);
    const dims = dimensionsForNodeType(n.type);
    return {
      id: n.id,
      type: 'topo',
      // dagre returns the CENTER; ReactFlow expects top-left.
      position: pos
        ? { x: pos.x - dims.width / 2, y: pos.y - dims.height / 2 }
        : { x: 0, y: 0 },
      data: {
        label: n.label,
        topoType: n.type,
        status: n.status,
        selected: n.id === selectedNodeId,
      },
    };
  });

  const flowEdges: Edge[] = topology.edges.map((e, i) => {
    return {
      // Contract edges have no id; synthesise a stable one from endpoints.
      id: `${e.source}->${e.target}-${e.type}-${i}`,
      source: e.source,
      target: e.target,
      ...edgeRenderingFor(e.type),
      data: { topoEdgeType: e.type },
    };
  });

  return { nodes: flowNodes, edges: flowEdges };
}

/**
 * Per-edge-type ReactFlow styling. Pulled out of `layoutWithDagre` so the
 * styling decisions live next to each other (and so future edge types can
 * be added with one new branch each, not by editing inline ternaries).
 *
 * Returns only ReactFlow `Edge` fields (style / label / labelStyle /
 * markerEnd) — the caller stamps the id / source / target / data.
 *
 * `markerEnd` uses the literal `'arrowclosed'` string instead of the
 * `MarkerType` enum so the test mock for `@xyflow/react` doesn't have to
 * re-export the enum — the wire-level value is what ReactFlow consumes
 * either way.
 */
function edgeRenderingFor(
  edgeType: TopologyEdgeType,
): Pick<Edge, 'style' | 'label' | 'labelStyle' | 'markerEnd'> {
  switch (edgeType) {
    case EDGE_TYPE_FABRIC_LINK:
      // Fabric links render in a blue-grey solid line (1.5px) so they read
      // as "platform-level" wiring distinct from the in-cluster contains
      // edges. Solid — fabric is not a "weak" relation.
      return { style: { stroke: '#69b1ff', strokeWidth: 1.5 } };
    case EDGE_TYPE_BINDS_TO:
      // Pod → slice resource binding. Cyan matches the NPU/slice accent
      // family so the eye reads "this pod is consuming an NPU resource".
      // Thin so a workload with 4 pods × 4 slices doesn't dominate.
      return { style: { stroke: '#13c2c2', strokeWidth: 1 } };
    case EDGE_TYPE_PD_PAIR:
      // Prefill ↔ Decode relation. Dashed + AntD warning-orange + a "PD"
      // label so demo viewers instantly spot the disaggregation pair, with
      // an arrowhead at the target end so the directionality of the
      // backend's `from`/`to` survives the render.
      return {
        style: {
          stroke: '#fa8c16',
          strokeWidth: 1.5,
          strokeDasharray: '6 4',
        },
        label: 'PD',
        labelStyle: { fontSize: 10, fill: '#fa8c16', fontWeight: 600 },
        markerEnd: { type: MarkerType.ArrowClosed, color: '#fa8c16' },
      };
    default:
      // contains / hccs / network / allocated and anything unknown.
      return { style: { stroke: '#bfbfbf', strokeWidth: 1 } };
  }
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
        // Cap the initial fitView zoom — at set-a-small (24 NPUs in one
        // rank) the natural fit shrinks labels to ~0.3× and they become
        // unreadable. `minZoom: 0.55` keeps the default zoom legible at
        // the cost of clipping the edges of the widest rank; the user
        // can pan, or click "fit view" in the Controls widget to see
        // everything. `padding: 0.05` trims excess whitespace.
        fitViewOptions={{ minZoom: 0.55, padding: 0.05 }}
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
