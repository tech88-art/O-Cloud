import { useEffect, useMemo } from 'react';
import { Skeleton, Tree, Typography } from 'antd';
import type { DataNode } from 'antd/es/tree';
import { useTranslation } from 'react-i18next';
import { EmptyState } from '@/components/EmptyState';
import { ErrorState } from '@/components/ErrorState';
import { useClusters, useClusterTopology, type Topology } from '@/services/cluster';
import { useTopologyStore } from '@/store/topologyStore';
import { TopologyView } from './TopologyView';
import styles from './styles.module.css';

const { Title } = Typography;

/**
 * Overview page (P1-T-108a — topology skeleton).
 *
 * Layout: three-column grid (see `styles.module.css`):
 *   - Left:    AntD `<Tree>` driven by the topology DTO.
 *   - Center:  `<TopologyView>` — ReactFlow graph.
 *   - Right:   reserved 320px slot for T-108b's `<DetailPanel>`.
 *
 * Selection contract: tree click and graph click both write the same key
 * (`selectedNodeId`) into the Zustand topology store. The center pane
 * reads it back and passes it to `<TopologyGraph selectedNodeId>` so the
 * highlight follows. This is the AC: "tree click → topology node
 * highlights".
 *
 * Cluster: T-108a auto-picks the first cluster returned by `/api/v1/clusters`
 * so the skeleton is reachable without a cluster switcher UI. A future
 * cluster picker will go in this same shell.
 */
export default function OverviewPage() {
  const { t } = useTranslation();
  const clustersQuery = useClusters();
  const selectedClusterId = useTopologyStore((s) => s.selectedClusterId);
  const setSelectedCluster = useTopologyStore((s) => s.setSelectedCluster);
  const selectedNodeId = useTopologyStore((s) => s.selectedNodeId);
  const setSelectedNode = useTopologyStore((s) => s.setSelectedNode);

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

  const topologyQuery = useClusterTopology(selectedClusterId);

  const treeData = useMemo<DataNode[]>(
    () => (topologyQuery.data ? buildTreeData(topologyQuery.data) : []),
    [topologyQuery.data],
  );

  return (
    <div className={styles.page} data-testid="overview-page">
      <aside className={styles.treePane} data-testid="overview-tree-pane">
        <div className={styles.treePaneHeader}>
          <Title level={5} style={{ margin: 0 }}>
            {t('overview.tree.title')}
          </Title>
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
      <main className={styles.centerPane} data-testid="overview-center-pane">
        <TopologyView clusterId={selectedClusterId} />
      </main>
      <aside className={styles.detailPane} data-testid="overview-detail-pane">
        {/*
         * T-108b will mount <DetailPanel /> here. Keeping the slot present
         * (not display:none) so the layout grid is stable across stages.
         */}
        <div className={styles.detailPanePlaceholder}>
          {t('overview.detailPanePending')}
        </div>
      </aside>
    </div>
  );
}

/**
 * Build the left tree from a topology DTO. We rebuild parent→children
 * relations from the `contains` edges so the tree is a strict superset of
 * what the graph shows — every graph node appears in the tree exactly
 * once. Nodes that don't appear as a target end up as roots (typically
 * the cluster).
 */
function buildTreeData(topology: Topology): DataNode[] {
  const byId = new Map<string, DataNode>();
  for (const n of topology.nodes) {
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
