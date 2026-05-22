import { useState } from 'react';
import { Input, Select, Skeleton, Space, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { EmptyState } from '@/components/EmptyState';
import { ErrorState } from '@/components/ErrorState';
import {
  useWorkloads,
  type Workload,
  type WorkloadFilter,
  type WorkloadStatus,
  type WorkloadType,
} from '@/services/workload';
import { WorkloadDetailDrawer } from './WorkloadDetailDrawer';
import { WorkloadTable } from './WorkloadTable';
import styles from './styles.module.css';

const { Title } = Typography;

const STATUS_OPTIONS: WorkloadStatus[] = [
  'running',
  'pending',
  'succeeded',
  'failed',
  'unknown',
];

const TYPE_OPTIONS: WorkloadType[] = [
  'inference',
  'benchmark',
  'training',
  'other',
];

/**
 * Workloads page (P1-T-206).
 *
 * Layout: header (title + filters) + AntD `<Table>`. Row click opens an
 * AntD `<Drawer>` rendering the workload detail (pods + relations + container
 * command/args/resources).
 *
 * Filter contract: status / type / namespace are forwarded to the backend
 * `GET /api/v1/workloads` query string by `useWorkloads`. The page itself
 * owns the filter state and re-issues the query on each change.
 *
 * Loading / error / empty per frontend/CLAUDE.md §4.8.
 */
export default function WorkloadsPage() {
  const { t } = useTranslation();
  const [filter, setFilter] = useState<WorkloadFilter>({});
  const [selected, setSelected] = useState<{
    namespace: string;
    name: string;
  } | null>(null);

  // P6-T-103: always opt-in to includeSliceBindings on the list endpoint
  // so the Slice Bindings column appears in the table when at least one
  // workload has bindings. Backend default is opt-out so the on-the-wire
  // payload is unchanged for non-Phase-6 callers; this page explicitly
  // asks for it. WorkloadTable hides the column when no workload has
  // bindings, so the table stays compact in pre-Phase-6 environments.
  // P11-T-105: opt in to the 3 Phase 11 indicator fields(o2DMSExposed
  // badge + quotaUsage progress bar + scaleHistory mini count)by
  // default. Backend cost is small(annotation check + Quota CR join +
  // scaleHistory ring-buffer fetch). WorkloadTable hides each column
  // when no row has the data, keeping the table compact in pre-Phase 11
  // environments.
  const workloadsQuery = useWorkloads({
    ...filter,
    includeSliceBindings: true,
    includeO2DMSExposed: true,
    includeQuotaUsage: true,
    includeScaleHistory: true,
  });

  const handleRowClick = (workload: Workload) => {
    setSelected({ namespace: workload.namespace, name: workload.name });
  };

  const handleClose = () => {
    setSelected(null);
  };

  return (
    <div className={styles.page} data-testid="workloads-page">
      <div className={styles.header}>
        <Title level={2} className={styles.title}>
          {t('workloads.title')}
        </Title>
        <div className={styles.filterBar} data-testid="workloads-filter-bar">
          <Space size="small">
            <span>{t('workloads.filter')}:</span>
            <Select<WorkloadStatus | undefined>
              data-testid="workloads-filter-status"
              className={styles.filterControl}
              placeholder={t('workloads.status')}
              allowClear
              value={filter.status}
              onChange={(value) =>
                setFilter((prev) => ({ ...prev, status: value }))
              }
              options={STATUS_OPTIONS.map((s) => ({ label: s, value: s }))}
            />
            <Select<WorkloadType | undefined>
              data-testid="workloads-filter-type"
              className={styles.filterControl}
              placeholder={t('workloads.type')}
              allowClear
              value={filter.type}
              onChange={(value) =>
                setFilter((prev) => ({ ...prev, type: value }))
              }
              options={TYPE_OPTIONS.map((s) => ({ label: s, value: s }))}
            />
            <Input
              data-testid="workloads-filter-namespace"
              className={styles.namespaceInput}
              placeholder={t('workloads.namespace')}
              allowClear
              value={filter.namespace ?? ''}
              onChange={(e) =>
                setFilter((prev) => ({
                  ...prev,
                  namespace: e.target.value || undefined,
                }))
              }
            />
          </Space>
        </div>
      </div>

      <div className={styles.tableWrapper}>
        <TableBody
          isLoading={workloadsQuery.isLoading}
          error={(workloadsQuery.error as Error | null) ?? null}
          data={workloadsQuery.data ?? []}
          onRowClick={handleRowClick}
          selectedRowKey={
            selected ? `${selected.namespace}/${selected.name}` : null
          }
          onRetry={() => void workloadsQuery.refetch()}
        />
      </div>

      <WorkloadDetailDrawer
        open={selected !== null}
        namespace={selected?.namespace ?? null}
        name={selected?.name ?? null}
        onClose={handleClose}
      />
    </div>
  );
}

interface TableBodyProps {
  isLoading: boolean;
  error: Error | null;
  data: Workload[];
  onRowClick: (w: Workload) => void;
  selectedRowKey: string | null;
  onRetry: () => void;
}

function TableBody({
  isLoading,
  error,
  data,
  onRowClick,
  selectedRowKey,
  onRetry,
}: TableBodyProps) {
  const { t } = useTranslation();
  if (isLoading) {
    return (
      <div data-testid="workloads-loading">
        <Skeleton active paragraph={{ rows: 6 }} />
      </div>
    );
  }
  if (error) {
    return (
      <ErrorState
        error={error}
        title={t('workloads.errorTitle')}
        retryLabel={t('common.retry')}
        onRetry={onRetry}
      />
    );
  }
  if (data.length === 0) {
    return (
      <div data-testid="workloads-empty">
        <EmptyState description={t('workloads.empty')} />
      </div>
    );
  }
  return (
    <WorkloadTable
      data={data}
      onRowClick={onRowClick}
      selectedRowKey={selectedRowKey ?? undefined}
    />
  );
}

