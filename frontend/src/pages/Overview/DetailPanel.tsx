import { Skeleton, Space, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { EmptyState } from '@/components/EmptyState';
import { ErrorState } from '@/components/ErrorState';
import { MetricChip } from '@/components/MetricChip';
import { ResourceCard } from '@/components/ResourceCard';
import { StatusTag } from '@/components/StatusTag';
import { useClusters, useClusterTopology, type TopologyNode } from '@/services/cluster';
import { useNode, useNodeNPUs } from '@/services/node';
import { useTopologyStore } from '@/store/topologyStore';

const { Text } = Typography;

/**
 * Right-column detail panel for the Overview page (P1-T-108b).
 *
 * The panel is a dispatcher keyed on the topology-node type of
 * `selectedNodeId` (Zustand store):
 *
 *   - null        — empty state ("select a node")
 *   - cluster     — cluster metadata from the already-loaded `useClusters()`
 *                   list (no extra fetch)
 *   - node        — calls `/api/v1/nodes/:name` and `/nodes/:name/npus`
 *   - npu / slice — derived from the topology DTO (no extra fetch — the
 *                   topology returned by `useClusterTopology(depth=slice)`
 *                   already carries npu / slice nodes with attributes)
 *
 * Loading / error / empty states per frontend/CLAUDE.md §4.8.
 *
 * i18n: every visible label flows through `useTranslation`; keys live
 * under `detailPanel.*` in `i18n/{zh-CN,en-US}.json`.
 */
export function DetailPanel() {
  const { t } = useTranslation();
  const selectedClusterId = useTopologyStore((s) => s.selectedClusterId);
  const selectedNodeId = useTopologyStore((s) => s.selectedNodeId);

  // Cluster list — same hook as the page shell uses; cached so this is
  // free unless the panel mounts before the page.
  const clustersQuery = useClusters();
  // Topology — same key/depth as the page shell, so this hits the cache.
  const topologyQuery = useClusterTopology(selectedClusterId);

  // Locate the selected node within the loaded topology. We need this to
  // dispatch on `.type` even before we know whether to issue a node-detail
  // fetch.
  const selectedTopoNode: TopologyNode | undefined =
    selectedNodeId && topologyQuery.data
      ? topologyQuery.data.nodes.find((n) => n.id === selectedNodeId)
      : undefined;

  // Empty state — nothing selected yet.
  if (!selectedNodeId) {
    return (
      <div data-testid="detail-panel-empty">
        <EmptyState description={t('detailPanel.selectNode')} />
      </div>
    );
  }

  // Topology hasn't landed yet — show a skeleton; we can't dispatch on type
  // until we know what was clicked.
  if (topologyQuery.isLoading) {
    return (
      <div data-testid="detail-panel-loading">
        <Skeleton active paragraph={{ rows: 4 }} />
      </div>
    );
  }

  // Selected id is stale (was deleted / topology refetched without it).
  if (!selectedTopoNode) {
    return (
      <div data-testid="detail-panel-stale">
        <EmptyState description={t('detailPanel.staleSelection')} />
      </div>
    );
  }

  switch (selectedTopoNode.type) {
    case 'cluster':
      return (
        <ClusterDetail
          cluster={
            clustersQuery.data?.find((c) => c.id === selectedTopoNode.id) ??
            null
          }
          isLoading={clustersQuery.isLoading}
          error={(clustersQuery.error as Error | null) ?? null}
          onRetry={() => void clustersQuery.refetch()}
          topoNode={selectedTopoNode}
        />
      );
    case 'node':
    case 'nodepool':
      return <NodeDetailView name={selectedTopoNode.label} topoNode={selectedTopoNode} />;
    case 'npu':
      return <NpuDetailView topoNode={selectedTopoNode} />;
    case 'slice':
      return <SliceDetailView topoNode={selectedTopoNode} />;
    case 'network':
    default:
      return (
        <ResourceCard
          title={selectedTopoNode.label}
          description={t(`detailPanel.type.${selectedTopoNode.type}`)}
          status={selectedTopoNode.status}
          testId="detail-panel-generic"
        >
          <Text type="secondary">{t('detailPanel.noExtra')}</Text>
        </ResourceCard>
      );
  }
}

interface ClusterDetailProps {
  cluster: import('@/services/cluster').Cluster | null;
  isLoading: boolean;
  error: Error | null;
  onRetry: () => void;
  topoNode: TopologyNode;
}

function ClusterDetail({
  cluster,
  isLoading,
  error,
  onRetry,
  topoNode,
}: ClusterDetailProps) {
  const { t } = useTranslation();
  if (isLoading) {
    return (
      <div data-testid="detail-panel-cluster-loading">
        <Skeleton active paragraph={{ rows: 4 }} />
      </div>
    );
  }
  if (error) {
    return (
      <ErrorState
        error={error}
        title={t('detailPanel.errorTitle')}
        retryLabel={t('common.retry')}
        onRetry={onRetry}
      />
    );
  }
  // Cluster wasn't in the list (rare; treat as topology-only render).
  const status = cluster?.status ?? topoNode.status;
  return (
    <ResourceCard
      title={cluster?.name ?? topoNode.label}
      description={t('detailPanel.type.cluster')}
      status={status}
      testId="detail-panel-cluster"
    >
      <Space direction="vertical" size={8} style={{ width: '100%' }}>
        {cluster?.role && (
          <MetricChip
            label={t('detailPanel.cluster.role')}
            value={cluster.role}
          />
        )}
        {cluster?.location && (
          <MetricChip
            label={t('detailPanel.cluster.location')}
            value={cluster.location}
          />
        )}
        {typeof cluster?.nodeCount === 'number' && (
          <MetricChip
            label={t('detailPanel.cluster.nodeCount')}
            value={cluster.nodeCount}
          />
        )}
        {typeof cluster?.npuCount === 'number' && (
          <MetricChip
            label={t('detailPanel.cluster.npuCount')}
            value={cluster.npuCount}
          />
        )}
        {cluster?.kubernetesVersion && (
          <MetricChip
            label={t('detailPanel.cluster.k8sVersion')}
            value={cluster.kubernetesVersion}
          />
        )}
      </Space>
    </ResourceCard>
  );
}

interface NodeDetailViewProps {
  name: string;
  topoNode: TopologyNode;
}

function NodeDetailView({ name, topoNode }: NodeDetailViewProps) {
  const { t } = useTranslation();
  const nodeQuery = useNode(name);
  const npusQuery = useNodeNPUs(name);

  if (nodeQuery.isLoading || npusQuery.isLoading) {
    return (
      <div data-testid="detail-panel-node-loading">
        <Skeleton active paragraph={{ rows: 6 }} />
      </div>
    );
  }
  if (nodeQuery.error) {
    return (
      <ErrorState
        error={nodeQuery.error as Error}
        title={t('detailPanel.errorTitle')}
        retryLabel={t('common.retry')}
        onRetry={() => {
          void nodeQuery.refetch();
          void npusQuery.refetch();
        }}
      />
    );
  }

  const detail = nodeQuery.data;
  const npus = npusQuery.data ?? [];

  return (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      <ResourceCard
        title={detail?.name ?? name}
        description={t('detailPanel.type.node')}
        status={detail?.status ?? topoNode.status}
        testId="detail-panel-node"
      >
        <Space direction="vertical" size={8} style={{ width: '100%' }}>
          {detail?.cpu?.raw && (
            <MetricChip
              label={t('detailPanel.node.cpu')}
              value={detail.cpu.raw}
            />
          )}
          {detail?.memory?.raw && (
            <MetricChip
              label={t('detailPanel.node.memory')}
              value={detail.memory.raw}
            />
          )}
          {detail?.arch && (
            <MetricChip
              label={t('detailPanel.node.arch')}
              value={detail.arch}
            />
          )}
          {detail?.os && (
            <MetricChip
              label={t('detailPanel.node.os')}
              value={detail.os}
            />
          )}
          {detail?.kubeletVersion && (
            <MetricChip
              label={t('detailPanel.node.kubelet')}
              value={detail.kubeletVersion}
            />
          )}
          {typeof detail?.npuCount === 'number' && (
            <MetricChip
              label={t('detailPanel.node.npuCount')}
              value={detail.npuCount}
            />
          )}
        </Space>
      </ResourceCard>

      {detail?.numa && detail.numa.length > 0 && (
        <ResourceCard
          title={t('detailPanel.node.numaSection')}
          testId="detail-panel-node-numa"
        >
          <Space direction="vertical" size={4} style={{ width: '100%' }}>
            {detail.numa.map((n, i) => (
              <Text key={n.id ?? i} type="secondary">
                {t('detailPanel.node.numaRow', {
                  id: n.id ?? i,
                  cpus: n.cpus?.length ?? 0,
                  memory: n.memory?.raw ?? '-',
                  npus: n.npus?.length ?? 0,
                })}
              </Text>
            ))}
          </Space>
        </ResourceCard>
      )}

      {detail?.networkInterfaces && detail.networkInterfaces.length > 0 && (
        <ResourceCard
          title={t('detailPanel.node.networkSection')}
          testId="detail-panel-node-network"
        >
          <Space direction="vertical" size={4} style={{ width: '100%' }}>
            {detail.networkInterfaces.map((nic, i) => (
              <Text key={nic.name ?? i} type="secondary">
                {nic.name ?? '-'} · {nic.speed ?? '-'} ·{' '}
                {(nic.ips ?? []).join(', ') || '-'}
              </Text>
            ))}
          </Space>
        </ResourceCard>
      )}

      {detail?.storage && detail.storage.length > 0 && (
        <ResourceCard
          title={t('detailPanel.node.storageSection')}
          testId="detail-panel-node-storage"
        >
          <Space direction="vertical" size={4} style={{ width: '100%' }}>
            {detail.storage.map((s, i) => (
              <Text key={s.device ?? i} type="secondary">
                {s.device ?? '-'} · {s.type ?? '-'} · {s.size?.raw ?? '-'}
              </Text>
            ))}
          </Space>
        </ResourceCard>
      )}

      <ResourceCard
        title={t('detailPanel.node.npuListSection', { count: npus.length })}
        testId="detail-panel-node-npus"
      >
        {npus.length === 0 ? (
          <EmptyState description={t('detailPanel.node.noNpus')} />
        ) : (
          <Space size={[4, 4]} wrap>
            {npus.map((n) => (
              <StatusTag
                key={n.id}
                status={n.status}
                label={`${n.id} · ${n.status}`}
              />
            ))}
          </Space>
        )}
      </ResourceCard>
    </Space>
  );
}

interface NpuDetailViewProps {
  topoNode: TopologyNode;
}

function NpuDetailView({ topoNode }: NpuDetailViewProps) {
  const { t } = useTranslation();
  const attrs = (topoNode.attributes ?? {}) as Record<string, unknown>;
  const vramMiB = numberOrUndefined(attrs.vramMiB ?? attrs.vram);
  const aiCoreTotal = numberOrUndefined(attrs.aiCoreTotal);
  const hccsGroup = stringOrUndefined(attrs.hccsGroup);
  const model = stringOrUndefined(attrs.model);
  const sliceMode = stringOrUndefined(attrs.sliceMode);
  const slices = Array.isArray(attrs.slices)
    ? (attrs.slices as Array<{ id?: string; status?: string }>)
    : [];

  return (
    <ResourceCard
      title={topoNode.label}
      description={t('detailPanel.type.npu')}
      status={topoNode.status}
      testId="detail-panel-npu"
    >
      <Space direction="vertical" size={8} style={{ width: '100%' }}>
        {model && (
          <MetricChip label={t('detailPanel.npu.model')} value={model} />
        )}
        {typeof vramMiB === 'number' && (
          <MetricChip
            label={t('detailPanel.npu.vram')}
            value={vramMiB}
            unit="MiB"
          />
        )}
        {typeof aiCoreTotal === 'number' && (
          <MetricChip
            label={t('detailPanel.npu.aiCore')}
            value={aiCoreTotal}
          />
        )}
        {hccsGroup && (
          <MetricChip label={t('detailPanel.npu.hccs')} value={hccsGroup} />
        )}
        {sliceMode && (
          <MetricChip
            label={t('detailPanel.npu.sliceMode')}
            value={sliceMode}
          />
        )}
        <div data-testid="detail-panel-npu-slices">
          <Text strong>{t('detailPanel.npu.slicesHeader')}</Text>{' '}
          {slices.length === 0 ? (
            <Text type="secondary">{t('detailPanel.npu.noSlices')}</Text>
          ) : (
            <Space size={[4, 4]} wrap>
              {slices.map((s, i) => (
                <StatusTag
                  key={s.id ?? i}
                  status={s.status ?? 'unknown'}
                  label={`${s.id ?? `slice-${i}`} · ${s.status ?? '?'}`}
                />
              ))}
            </Space>
          )}
        </div>
      </Space>
    </ResourceCard>
  );
}

interface SliceDetailViewProps {
  topoNode: TopologyNode;
}

function SliceDetailView({ topoNode }: SliceDetailViewProps) {
  const { t } = useTranslation();
  const attrs = (topoNode.attributes ?? {}) as Record<string, unknown>;
  const parentNPU = stringOrUndefined(attrs.parentNPU);
  const template = stringOrUndefined(attrs.template);
  const allocatedTo = attrs.allocatedTo as
    | { namespace?: string; podName?: string; containerName?: string }
    | undefined
    | null;

  return (
    <ResourceCard
      title={topoNode.label}
      description={t('detailPanel.type.slice')}
      status={topoNode.status}
      testId="detail-panel-slice"
    >
      <Space direction="vertical" size={8} style={{ width: '100%' }}>
        {parentNPU && (
          <MetricChip
            label={t('detailPanel.slice.parentNPU')}
            value={parentNPU}
          />
        )}
        {template && (
          <MetricChip
            label={t('detailPanel.slice.template')}
            value={template}
          />
        )}
        {allocatedTo && allocatedTo.namespace && allocatedTo.podName && (
          <MetricChip
            label={t('detailPanel.slice.allocatedTo')}
            value={`${allocatedTo.namespace}/${allocatedTo.podName}`}
          />
        )}
      </Space>
    </ResourceCard>
  );
}

/**
 * `attributes` is `unknown` per the contract; coerce conservatively.
 * Returns undefined when the value isn't the expected primitive type.
 */
function numberOrUndefined(v: unknown): number | undefined {
  return typeof v === 'number' && Number.isFinite(v) ? v : undefined;
}

function stringOrUndefined(v: unknown): string | undefined {
  return typeof v === 'string' && v.length > 0 ? v : undefined;
}

export default DetailPanel;
