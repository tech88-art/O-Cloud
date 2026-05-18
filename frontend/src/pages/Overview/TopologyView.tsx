import { Skeleton } from 'antd';
import { useTranslation } from 'react-i18next';
import { EmptyState } from '@/components/EmptyState';
import { ErrorState } from '@/components/ErrorState';
import { TopologyGraph } from '@/components/TopologyGraph';
import { useClusterTopology } from '@/services/cluster';
import { useTopologyStore } from '@/store/topologyStore';
import styles from './styles.module.css';

/**
 * Center pane of the Overview page. Owns:
 *   - topology data fetch (react-query)
 *   - loading / error / empty state branching (frontend/CLAUDE.md §4.8)
 *   - wiring ReactFlow node clicks into the Zustand store so the left
 *     tree and the graph stay in sync
 *
 * Selection state is read from `useTopologyStore` — the tree (in
 * `index.tsx`) writes the same store on tree click, which is how the
 * "click tree node → topology highlights" AC is satisfied.
 *
 * T-108b will add: detail-panel data fetch on selection; WS subscription;
 * dblclick → slice expansion.
 */
export interface TopologyViewProps {
  clusterId: string | null;
}

export function TopologyView({ clusterId }: TopologyViewProps) {
  const { t } = useTranslation();
  const selectedNodeId = useTopologyStore((s) => s.selectedNodeId);
  const setSelectedNode = useTopologyStore((s) => s.setSelectedNode);

  const { data, isLoading, error, refetch } = useClusterTopology(clusterId);

  if (!clusterId) {
    return (
      <div className={styles.centerStateOverlay}>
        <EmptyState description={t('overview.selectCluster')} />
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className={styles.centerStateOverlay} data-testid="topology-loading">
        <Skeleton active paragraph={{ rows: 6 }} title={false} />
      </div>
    );
  }

  if (error) {
    return (
      <div className={styles.centerStateOverlay}>
        <ErrorState
          error={error as Error}
          title={t('overview.errorTitle')}
          retryLabel={t('common.retry')}
          onRetry={() => {
            void refetch();
          }}
        />
      </div>
    );
  }

  if (!data || data.nodes.length === 0) {
    return (
      <div className={styles.centerStateOverlay}>
        <EmptyState description={t('overview.noData')} />
      </div>
    );
  }

  return (
    <TopologyGraph
      topology={data}
      selectedNodeId={selectedNodeId}
      onNodeClick={(id) => setSelectedNode(id)}
      // Hook reserved for T-108b slice expansion. Setting the same store
      // value on dblclick keeps the event observable to tests.
      onNodeDoubleClick={(id) => setSelectedNode(id)}
    />
  );
}

export default TopologyView;
