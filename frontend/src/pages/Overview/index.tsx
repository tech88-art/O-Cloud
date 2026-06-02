import { useCallback, useEffect, useMemo } from 'react';
import { Skeleton, Splitter, Tree, Typography } from 'antd';
import type { DataNode } from 'antd/es/tree';
import { useTranslation } from 'react-i18next';
import { EmptyState } from '@/components/EmptyState';
import { ErrorState } from '@/components/ErrorState';
import { useClusters, useClusterTopology, type Topology, type TopologyNode } from '@/services/cluster';
import { useAppStore } from '@/store';
import { useTopologyStore } from '@/store/topologyStore';
import { useTopologyWS } from '@/hooks/useTopologyWS';
import { DetailPanel } from './DetailPanel';
import { TopologyView } from './TopologyView';
import styles from './styles.module.css';

const { Title, Text } = Typography;

/**
 * Overview workspace — the single-page operator console (P12-T-201 / ADR-0022).
 *
 * Layout: AntD `<Splitter>` (replaces the old fixed CSS grid):
 *   - Left:    AntD `<Tree>` driven by the topology DTO — collapsible + resizable.
 *   - Center:  `<TopologyView>` — ReactFlow graph (the main stage, never collapsed).
 *   - Right:   `<DetailPanel>` — selection-driven detail — collapsible + resizable.
 *
 * Pane visibility (left/right) is toggled from the Header buttons; pane
 * widths + collapsed state persist to localStorage via the app store
 * (`useAppStore`). Hidden panes are unmounted (conditional `<Splitter.Panel>`),
 * which keeps the resize-size mapping unambiguous and frees their subtrees.
 *
 * Selection contract: tree click and graph click both write the same key
 * (`selectedNodeId`) into the Zustand topology store. The center pane
 * reads it back and passes it to `<TopologyGraph selectedNodeId>` so the
 * highlight follows. DetailPanel reads the same key. T-108b adds:
 *   - DetailPanel mounted in the right slot
 *   - useTopologyWS subscription for live updates
 *   - dbl-click NPU toggles slice subtree expansion via the store
 *
 * Cluster: auto-picks the first cluster returned by `/api/v1/clusters`
 * so the page is reachable without a cluster switcher UI.
 */
export default function OverviewPage() {
  const { t } = useTranslation();
  const leftPaneHidden = useAppStore((s) => s.leftPaneHidden);
  const rightPaneHidden = useAppStore((s) => s.rightPaneHidden);
  const leftPaneSize = useAppStore((s) => s.leftPaneSize);
  const rightPaneSize = useAppStore((s) => s.rightPaneSize);
  const setPaneSizes = useAppStore((s) => s.setPaneSizes);
  const clustersQuery = useClusters();
  const selectedClusterId = useTopologyStore((s) => s.selectedClusterId);
  const setSelectedCluster = useTopologyStore((s) => s.setSelectedCluster);
  const selectedNodeId = useTopologyStore((s) => s.selectedNodeId);
  const setSelectedNode = useTopologyStore((s) => s.setSelectedNode);
  const lastEventAt = useTopologyStore((s) => s.lastEventAt);
  // ADR-0004 / ADR-0005 / RFC-003: fabric + workloads flags. The TOGGLES now
  // live in the right DetailPanel (P12 polish #2); here we only READ the flags
  // and pass them to `useClusterTopology` so the left-tree query stays in
  // lock-step with the graph + panel queries (one shared react-query cache key).
  const showFabric = useTopologyStore((s) => s.showFabric);
  const showWorkloads = useTopologyStore((s) => s.showWorkloads);

  // Auto-pick first cluster once the list lands. Idempotent: only runs if
  // nothing is selected yet.
  useEffect(() => {
    if (!selectedClusterId && clustersQuery.data && clustersQuery.data.length > 0) {
      const first = clustersQuery.data[0];
      if (first) {
        setSelectedCluster(first.id);
      }
    }
  }, [clustersQuery.data, selectedClusterId, setSelectedCluster]);

  // Passing showFabric + showWorkloads here keeps the left-tree query
  // (this hook) in lock-step with the center-pane query inside
  // <TopologyView> — both call useClusterTopology with the same args, so
  // react-query de-dupes them into a single network request.
  const topologyQuery = useClusterTopology(
    selectedClusterId,
    'slice',
    showFabric,
    showWorkloads,
  );

  // Topology WS — mount once the cluster is known. The hook handles
  // reconnects + cache invalidation; we just surface its status to the
  // header chip.
  //
  // known-issues #5: an opt-in `?ffwd=<N>` page query param flips the
  // mock-event replayer to fast-forward mode. Production users don't
  // pass it; the E2E suite uses ?ffwd=100 to deterministically observe
  // a backend event landing in ~1.8s instead of 180s real-time.
  const ws = useTopologyWS(selectedClusterId, { fastforward: readFastforwardFromUrl() });

  const treeData = useMemo<DataNode[]>(
    () => (topologyQuery.data ? buildTreeData(topologyQuery.data) : []),
    [topologyQuery.data],
  );

  // Persist pane widths when the user finishes a drag. `sizes` lists only the
  // currently-mounted panels in order [left?, center, right?], so we walk the
  // same visibility gates used to render them — a hidden pane keeps its
  // stored width rather than being clobbered by the center pane's size.
  const handleResizeEnd = useCallback(
    (sizes: number[]) => {
      let i = 0;
      let nextLeft = leftPaneSize;
      let nextRight = rightPaneSize;
      if (!leftPaneHidden) {
        nextLeft = sizes[i] ?? nextLeft;
        i += 1;
      }
      i += 1; // center pane (flexible, not persisted)
      if (!rightPaneHidden) {
        nextRight = sizes[i] ?? nextRight;
      }
      setPaneSizes(nextLeft, nextRight);
    },
    [leftPaneHidden, rightPaneHidden, leftPaneSize, rightPaneSize, setPaneSizes],
  );

  return (
    <div className={styles.workspace} data-testid="overview-page">
      <Splitter style={{ height: '100%' }} onResizeEnd={handleResizeEnd}>
        {!leftPaneHidden && (
          <Splitter.Panel key="left" defaultSize={leftPaneSize} min={200} max="55%">
            <aside className={styles.treePane} data-testid="overview-tree-pane">
              <div className={styles.treePaneHeader}>
                <Title level={5} style={{ margin: 0 }}>
                  {t('overview.tree.title')}
                </Title>
                <WsStatusChip
                  status={ws.status}
                  lastEventAt={lastEventAt}
                  connectingLabel={t('ws.connecting')}
                  openLabel={t('ws.open')}
                  closedLabel={t('ws.closed')}
                  idleLabel={t('ws.idle')}
                  lastEventLabel={t('ws.lastEventAt')}
                />
                {/*
                 * P12 polish #2: the Fabric / Workloads view toggles moved out of
                 * this tree header into the right DetailPanel (decluttered core
                 * overview · controls sit with the detail you inspect). The store
                 * flags they write are still read here (below) so the tree query
                 * stays in lock-step with the graph + panel queries.
                 */}
              </div>
              <LeftTree
                treeData={treeData}
                selectedNodeId={selectedNodeId}
                onSelect={(id) => setSelectedNode(id)}
                isLoading={topologyQuery.isLoading || clustersQuery.isLoading}
                error={(topologyQuery.error ?? clustersQuery.error) as Error | null}
                onRetry={() => {
                  void topologyQuery.refetch();
                  void clustersQuery.refetch();
                }}
                emptyText={t('overview.noData')}
                errorTitle={t('overview.errorTitle')}
                retryLabel={t('common.retry')}
              />
            </aside>
          </Splitter.Panel>
        )}
        <Splitter.Panel key="center" min={360}>
          <main className={styles.centerPane} data-testid="overview-center-pane">
            <TopologyView clusterId={selectedClusterId} />
          </main>
        </Splitter.Panel>
        {!rightPaneHidden && (
          <Splitter.Panel key="right" defaultSize={rightPaneSize} min={260} max="55%">
            <aside className={styles.detailPane} data-testid="overview-detail-pane">
              <DetailPanel />
            </aside>
          </Splitter.Panel>
        )}
      </Splitter>
    </div>
  );
}

/**
 * Build the left tree from a topology DTO. We rebuild parent→children
 * relations from the `contains` edges so the tree is a strict superset of
 * what the graph shows — every graph node appears in the tree exactly
 * once. Nodes that don't appear as a target end up as roots (typically
 * the cluster).
 *
 * ADR-0005 caveat: workload + pod nodes are intentionally filtered out
 * of the left tree. The backend does NOT emit cluster→workload or
 * workload→pod `contains` edges (only `binds-to` / `pd-pair`), so they
 * would otherwise appear as orphan tree roots and clutter the resource
 * hierarchy. Workload/pod stay graph-only in Phase 1; surfacing them in
 * the tree (e.g. under a synthesized "Workloads" parent) is a possible
 * Phase 2 follow-up.
 */
function buildTreeData(topology: Topology): DataNode[] {
  // ADR-0005: workload + pod nodes live in the graph only; the backend
  // emits no `contains` edges into them, so leaving them in the tree
  // would produce orphan roots. ADR-0006 promoted both literals into
  // `TopologyNode['type']` so this comparison is fully type-safe now
  // (was a runtime-only string cast pre-regen).
  const isGraphOnlyType = (t: TopologyNode['type']): boolean =>
    t === 'workload' || t === 'pod';

  const byId = new Map<string, DataNode>();
  for (const n of topology.nodes) {
    if (isGraphOnlyType(n.type)) continue;
    byId.set(n.id, {
      key: n.id,
      title: n.label,
      children: [],
    });
  }

  const targets = new Set<string>();
  for (const e of topology.edges) {
    if (e.type !== 'contains') continue;
    const parent = byId.get(e.source);
    const child = byId.get(e.target);
    if (!parent || !child) continue;
    parent.children!.push(child);
    targets.add(e.target);
  }

  // Roots = anything that is not a `contains` target. For a well-formed
  // topology this is the cluster node.
  return topology.nodes
    .filter((n) => !isGraphOnlyType(n.type))
    .filter((n) => !targets.has(n.id))
    .map((n) => byId.get(n.id)!)
    .filter(Boolean);
}

interface LeftTreeProps {
  treeData: DataNode[];
  selectedNodeId: string | null;
  onSelect: (id: string) => void;
  isLoading: boolean;
  error: Error | null;
  onRetry: () => void;
  emptyText: string;
  errorTitle: string;
  retryLabel: string;
}

function LeftTree({
  treeData,
  selectedNodeId,
  onSelect,
  isLoading,
  error,
  onRetry,
  emptyText,
  errorTitle,
  retryLabel,
}: LeftTreeProps) {
  if (isLoading) {
    return <Skeleton active title={false} paragraph={{ rows: 6 }} />;
  }
  if (error) {
    return (
      <ErrorState
        error={error}
        title={errorTitle}
        retryLabel={retryLabel}
        onRetry={onRetry}
      />
    );
  }
  if (treeData.length === 0) {
    return <EmptyState description={emptyText} />;
  }
  return (
    <Tree
      data-testid="overview-tree"
      treeData={treeData}
      defaultExpandAll
      blockNode
      selectedKeys={selectedNodeId ? [selectedNodeId] : []}
      onSelect={(keys) => {
        const first = keys[0];
        if (typeof first === 'string') {
          onSelect(first);
        }
      }}
    />
  );
}

/**
 * Small chip showing the WS connection state. Lives next to the tree
 * header so operators can see whether the live feed is up at a glance.
 * Text-only — color is conveyed via AntD `Text type`.
 */
interface WsStatusChipProps {
  status: 'idle' | 'connecting' | 'open' | 'closed';
  lastEventAt: string | null;
  connectingLabel: string;
  openLabel: string;
  closedLabel: string;
  idleLabel: string;
  lastEventLabel: string;
}

function WsStatusChip({
  status,
  lastEventAt,
  connectingLabel,
  openLabel,
  closedLabel,
  idleLabel,
  lastEventLabel,
}: WsStatusChipProps) {
  const labelMap: Record<WsStatusChipProps['status'], string> = {
    idle: idleLabel,
    connecting: connectingLabel,
    open: openLabel,
    closed: closedLabel,
  };
  const toneMap: Record<WsStatusChipProps['status'], 'secondary' | 'success' | 'warning' | 'danger'> = {
    idle: 'secondary',
    connecting: 'warning',
    open: 'success',
    closed: 'danger',
  };
  return (
    <div
      data-testid="ws-status-chip"
      data-status={status}
      // known-issues #5: surface lastEventAt as a stable attribute so
      // the E2E suite can poll for "any event has landed" without
      // depending on rendered text format.
      data-last-event-at={lastEventAt ?? ''}
    >
      <Text type={toneMap[status]}>● {labelMap[status]}</Text>
      {lastEventAt && (
        <Text type="secondary">
          {' '}
          · {lastEventLabel}: {formatTimestamp(lastEventAt)}
        </Text>
      )}
    </div>
  );
}

/**
 * Read `?ffwd=<N>` from the current page URL. Returns 0 when absent /
 * invalid / non-positive (which the useTopologyWS hook treats as
 * "real-time replay"). E2E-only knob; production users don't set it.
 */
function readFastforwardFromUrl(): number {
  if (typeof window === 'undefined') return 0;
  const params = new URLSearchParams(window.location.search);
  const raw = params.get('ffwd');
  if (!raw) return 0;
  const n = Number(raw);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

function formatTimestamp(iso: string): string {
  try {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleTimeString();
  } catch {
    return iso;
  }
}

// P12 polish #2: `FabricToggle` / `WorkloadsToggle` moved to `DetailPanel.tsx`
// (as `ViewOptions`). The overview header now carries only the title + WS chip.
