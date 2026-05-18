import { Table } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useTranslation } from 'react-i18next';
import { StatusTag } from '@/components/StatusTag';
import type { Workload } from '@/services/workload';
import styles from './styles.module.css';

export interface WorkloadTableProps {
  data: Workload[];
  /** Fired when a row is clicked. The Drawer reads the values out of the row. */
  onRowClick: (workload: Workload) => void;
  /** Optional row that should appear visually selected (matches Drawer state). */
  selectedRowKey?: string | null;
}

/**
 * Row id = `<namespace>/<name>`. Workload names are not globally unique, only
 * unique within a namespace; the namespace must be in the key so AntD's row
 * de-duplication doesn't collapse same-named workloads in different namespaces.
 *
 * Exported as a sibling helper alongside the component (same pattern as
 * `StatusTag` / `GrafanaPanel` in this repo). The react-refresh hint is
 * intentionally suppressed — the helper is tiny and tightly coupled to the
 * row rendering; splitting it into its own module would only add indirection.
 */
// eslint-disable-next-line react-refresh/only-export-components
export function workloadRowKey(w: Workload): string {
  return `${w.namespace}/${w.name}`;
}

/**
 * Workload list table (P1-T-206).
 *
 * Columns (per task spec): name, namespace, type, status, replicas(ready/desired),
 * nodeNames, createdAt.
 *
 * The table itself is dumb: data + click handler in, rendered rows out. Filter
 * state is owned by the page (`index.tsx`) and flows through `useWorkloads`.
 */
export function WorkloadTable({
  data,
  onRowClick,
  selectedRowKey,
}: WorkloadTableProps) {
  const { t } = useTranslation();

  const columns: ColumnsType<Workload> = [
    {
      title: t('workloads.name'),
      dataIndex: 'name',
      key: 'name',
      ellipsis: true,
    },
    {
      title: t('workloads.namespace'),
      dataIndex: 'namespace',
      key: 'namespace',
      ellipsis: true,
    },
    {
      title: t('workloads.type'),
      dataIndex: 'type',
      key: 'type',
      width: 120,
    },
    {
      title: t('workloads.status'),
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (status: Workload['status']) => <StatusTag status={status} />,
    },
    {
      title: t('workloads.ready'),
      key: 'replicas',
      width: 100,
      render: (_: unknown, row: Workload) => {
        const ready = row.replicas?.ready ?? 0;
        const desired = row.replicas?.desired ?? 0;
        return (
          <span data-testid={`workload-replicas-${workloadRowKey(row)}`}>
            {ready}/{desired}
          </span>
        );
      },
    },
    {
      title: t('workloads.nodeNames'),
      dataIndex: 'nodeNames',
      key: 'nodeNames',
      ellipsis: true,
      render: (nodeNames: string[] | undefined) =>
        nodeNames && nodeNames.length > 0 ? nodeNames.join(', ') : '-',
    },
    {
      title: t('workloads.createdAt'),
      dataIndex: 'createdAt',
      key: 'createdAt',
      width: 180,
      render: (iso: string | undefined) => formatTimestamp(iso),
    },
  ];

  return (
    <Table<Workload>
      data-testid="workload-table"
      rowKey={workloadRowKey}
      columns={columns}
      dataSource={data}
      pagination={{ pageSize: 20, showSizeChanger: false }}
      size="middle"
      rowClassName={(row) => {
        const base = styles.clickableRow;
        return selectedRowKey && workloadRowKey(row) === selectedRowKey
          ? `${base} ant-table-row-selected`
          : base;
      }}
      onRow={(row) => ({
        'data-testid': `workload-row-${workloadRowKey(row)}`,
        onClick: () => onRowClick(row),
      })}
    />
  );
}

function formatTimestamp(iso: string | undefined): string {
  if (!iso) return '-';
  try {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleString();
  } catch {
    return iso;
  }
}

export default WorkloadTable;
