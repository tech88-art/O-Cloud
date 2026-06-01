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
 * T-108b additions:
 *   - reads `expandedNPUs` from the store and passes to `<TopologyGraph>`
 *   - dbl-clicking an NPU node toggles its id in `expandedNPUs` so the
 *     slice subtree appears (or disappears) on demand
 */
export interface TopologyViewProps {
  clusterId: string | null;
}

export function TopologyView({ clusterId }: TopologyViewProps) {
  const { t } = useTranslation();
  const selectedNodeId = useTopologyStore((s) => s.selectedNodeId);
  const setSelectedNode = useTopologyStore((s) => s.setSelectedNode);
  const expandedNPUs = useTopologyStore((s) => s.expandedNPUs);
  const toggleExpandedNPU = useTopologyStore((s) => s.toggleExpandedNPU);
  // P12-T-202 / ADR-0022 §4(b): focus/isolate anchor + setter. The graph's
  // focus toolbar button writes this; TopologyGraph filters the graph to the
  // anchor's subtree + placed workloads.
  const focusedNodeId = useTopologyStore((s) => s.focusedNodeId);
  const setFocusedNode = useTopologyStore((s) => s.setFocusedNode);
  // ADR-0004 / ADR-0005 / RFC-003. Reads the same flags the OverviewPage
  // header writes so react-query de-dupes the tree's and the graph's
  // topology fetches into one network request rather than diverging on
  // cache keys. Both flags default OFF in the store; the toggles in the
  // header flip them.
  const showFabric = useTopologyStore((s) => s.showFabric);
  const showWorkloads = useTopologyStore((s) => s.showWorkloads);

  const { data, isLoading, error, refetch } = useClusterTopology(
    clusterId,
    'slice',
    showFabric,
    showWorkloads,
  );

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
      expandedNPUs={expandedNPUs}
      focusedNodeId={focusedNodeId}
      onFocusNode={setFocusedNode}
      onNodeClick={(id) => setSelectedNode(id)}
      onNodeDoubleClick={(id) => {
        // Dbl-click an NPU → toggle its slice subtree. Other node types
        // dbl-click to no-op (the click handler already selected them).
        const node = data.nodes.find((n) => n.id === id);
        if (node?.type === 'npu') {
          toggleExpandedNPU(id);
        }
      }}
    />
  );
}

export default TopologyView;
