import { Drawer, Skeleton, Space, Tag, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { EmptyState } from '@/components/EmptyState';
import { ErrorState } from '@/components/ErrorState';
import { MetricChip } from '@/components/MetricChip';
import { StatusTag } from '@/components/StatusTag';
import {
  useWorkloadDetail,
  type Pod,
  type WorkloadDetail,
} from '@/services/workload';
import styles from './styles.module.css';

const { Text } = Typography;

export interface WorkloadDetailDrawerProps {
  /** Drawer open/close state owned by the page. */
  open: boolean;
  /** Selected workload identity. When either is null the drawer renders empty. */
  namespace: string | null;
  name: string | null;
  onClose: () => void;
}

/**
 * Workload detail Drawer (P1-T-206).
 *
 * Sections:
 *   - Basics (status / kind / replicas / nodeNames)
 *   - Pods   (per pod: name + nodeName + status + containers)
 *   - Relations (PD pairs etc. as AntD `Tag`s)
 *   - "View metrics" link to /metrics?workload=ns/name (T208 wires the page;
 *     we only provide the href)
 *
 * Loading / error / empty states per frontend/CLAUDE.md §4.8.
 */
export function WorkloadDetailDrawer({
  open,
  namespace,
  name,
  onClose,
}: WorkloadDetailDrawerProps) {
  const { t } = useTranslation();
  const detailQuery = useWorkloadDetail(namespace, name);

  const titleText =
    namespace && name
      ? `${namespace} / ${name}`
      : t('workloadDetail.titleFallback');

  return (
    <Drawer
      data-testid="workload-detail-drawer"
      title={titleText}
      open={open}
      onClose={onClose}
      width={640}
      destroyOnClose
    >
      {!namespace || !name ? (
        <EmptyState description={t('workloadDetail.empty')} />
      ) : detailQuery.isLoading ? (
        <div data-testid="workload-detail-loading">
          <Skeleton active paragraph={{ rows: 8 }} />
        </div>
      ) : detailQuery.error ? (
        <ErrorState
          error={detailQuery.error as Error}
          title={t('workloadDetail.errorTitle')}
          retryLabel={t('common.retry')}
          onRetry={() => void detailQuery.refetch()}
        />
      ) : detailQuery.data ? (
        <DetailBody
          detail={detailQuery.data}
          metricsHref={`/metrics?workload=${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`}
        />
      ) : (
        <EmptyState description={t('workloadDetail.empty')} />
      )}
    </Drawer>
  );
}

interface DetailBodyProps {
  detail: WorkloadDetail;
  metricsHref: string;
}

function DetailBody({ detail, metricsHref }: DetailBodyProps) {
  const { t } = useTranslation();
  const ready = detail.replicas?.ready ?? 0;
  const desired = detail.replicas?.desired ?? 0;
  const pods = detail.pods ?? [];
  const relations = detail.relations ?? [];
  const nodeNames = detail.nodeNames ?? [];

  return (
    <div className={styles.detailSections} data-testid="workload-detail-body">
      <section
        className={styles.detailSection}
        data-testid="workload-detail-basics"
      >
        <h4 className={styles.detailHeading}>{t('workloadDetail.basics')}</h4>
        <Space size={[8, 8]} wrap>
          <StatusTag status={detail.status} />
          {detail.kind && (
            <MetricChip
              label={t('workloadDetail.kind')}
              value={detail.kind}
            />
          )}
          {detail.type && (
            <MetricChip
              label={t('workloads.type')}
              value={detail.type}
            />
          )}
          <MetricChip
            label={t('workloads.ready')}
            value={`${ready}/${desired}`}
          />
          {nodeNames.length > 0 && (
            <MetricChip
              label={t('workloads.nodeNames')}
              value={nodeNames.join(', ')}
            />
          )}
        </Space>
      </section>

      <section
        className={styles.detailSection}
        data-testid="workload-detail-pods"
      >
        <h4 className={styles.detailHeading}>
          {t('workloadDetail.pods')} ({pods.length})
        </h4>
        {pods.length === 0 ? (
          <EmptyState description={t('workloadDetail.noPods')} />
        ) : (
          pods.map((pod, i) => (
            <PodCard key={pod.name ?? `pod-${i}`} pod={pod} />
          ))
        )}
      </section>

      <section
        className={styles.detailSection}
        data-testid="workload-detail-relations"
      >
        <h4 className={styles.detailHeading}>
          {t('workloadDetail.relations')}
        </h4>
        {relations.length === 0 ? (
          <Text type="secondary">{t('workloadDetail.noRelations')}</Text>
        ) : (
          <div className={styles.relationsRow}>
            {relations.map((r, i) => (
              <Tag
                key={`${r.from ?? ''}-${r.to ?? ''}-${i}`}
                color={relationTone(r.type)}
                data-testid={`workload-relation-${i}`}
              >
                {r.from ?? '?'} → {r.to ?? '?'}
                {r.type ? ` (${r.type})` : ''}
              </Tag>
            ))}
          </div>
        )}
      </section>

      <section
        className={styles.detailSection}
        data-testid="workload-detail-metrics-link"
      >
        <a href={metricsHref} data-testid="workload-detail-metrics-href">
          {t('workloadDetail.viewMetrics')}
        </a>
      </section>
    </div>
  );
}

interface PodCardProps {
  pod: Pod;
}

function PodCard({ pod }: PodCardProps) {
  const { t } = useTranslation();
  const containers = pod.containers ?? [];
  return (
    <div
      className={styles.podCard}
      data-testid={`workload-pod-${pod.name ?? 'unknown'}`}
    >
      <div className={styles.podHeader}>
        <Space size={8}>
          <span className={styles.podName}>{pod.name ?? '-'}</span>
          {pod.status && <StatusTag status={pod.status} />}
        </Space>
        <span className={styles.podMeta}>
          {t('workloads.nodeNames')}: {pod.nodeName ?? '-'}
        </span>
      </div>
      {containers.length === 0 ? (
        <Text type="secondary">{t('workloadDetail.noContainers')}</Text>
      ) : (
        containers.map((c, i) => (
          <div
            key={c.name ?? `container-${i}`}
            className={styles.containerBlock}
            data-testid={`workload-container-${c.name ?? `idx-${i}`}`}
          >
            <span className={styles.containerName}>
              {t('workloadDetail.containers')}: {c.name ?? '-'}
            </span>
            {c.image && (
              <div className={styles.kv}>
                <span className={styles.kvLabel}>image</span>
                <span>{c.image}</span>
              </div>
            )}
            {c.command && c.command.length > 0 && (
              <div className={styles.kv}>
                <span className={styles.kvLabel}>
                  {t('workloadDetail.command')}
                </span>
                <pre className={styles.code}>{c.command.join(' ')}</pre>
              </div>
            )}
            {c.args && c.args.length > 0 && (
              <div className={styles.kv}>
                <span className={styles.kvLabel}>
                  {t('workloadDetail.args')}
                </span>
                <pre className={styles.code}>{c.args.join(' ')}</pre>
              </div>
            )}
            {c.resources && (
              <div className={styles.kv}>
                <span className={styles.kvLabel}>
                  {t('workloadDetail.resources')}
                </span>
                <span>
                  cpu: {c.resources.cpu ?? '-'} · memory:{' '}
                  {c.resources.memory ?? '-'}
                  {c.resources.npuSlices && c.resources.npuSlices.length > 0
                    ? ` · npu: ${c.resources.npuSlices.join(', ')}`
                    : ''}
                </span>
              </div>
            )}
          </div>
        ))
      )}
    </div>
  );
}

type RelationType = NonNullable<
  NonNullable<WorkloadDetail['relations']>[number]['type']
>;

function relationTone(type: RelationType | undefined): string {
  switch (type) {
    case 'pd-pair':
      return 'magenta';
    case 'sidecar':
      return 'blue';
    case 'init':
      return 'cyan';
    case 'peer':
      return 'geekblue';
    default:
      return 'default';
  }
}

export default WorkloadDetailDrawer;
