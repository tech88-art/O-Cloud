import { Card, Space, Typography } from 'antd';
import type { ReactNode } from 'react';
import { StatusTag } from '../StatusTag';

const { Text } = Typography;

export interface ResourceCardProps {
  /** Card heading (e.g. cluster name, pool name). Required. */
  title: ReactNode;
  /** Optional sub-text under the title (e.g. resource type / description). */
  description?: ReactNode;
  /**
   * Optional status string. When provided, a `StatusTag` is rendered in the
   * card extra slot. Pass `mapping` via `statusMapping` to override colors.
   */
  status?: string;
  /** Optional custom mapping forwarded to the embedded `StatusTag`. */
  statusMapping?: Parameters<typeof StatusTag>[0]['mapping'];
  /**
   * Optional action node(s) shown in the card extra slot (right side of
   * the header). Sits next to the `StatusTag` if both are provided.
   */
  actions?: ReactNode;
  /** Card body content. */
  children?: ReactNode;
  /** Optional className for the AntD Card root. */
  className?: string;
  /**
   * Optional test id forwarded onto the card root. Defaults to `resource-card`
   * so tests can locate it by default.
   */
  testId?: string;
}

/**
 * Generic AntD Card wrapper for displaying a resource summary
 * (cluster, node, NPU, workload, etc.).
 *
 * Usage:
 *   <ResourceCard title="cluster-a" status="healthy">
 *     <p>4 nodes, 16 NPUs</p>
 *   </ResourceCard>
 *
 * i18n: `title` / `description` accept any `ReactNode` — pass `t('key')`
 * results in. The component itself performs no translation lookups
 * (i18n files are outside T-107 Allowed Paths).
 */
export function ResourceCard({
  title,
  description,
  status,
  statusMapping,
  actions,
  children,
  className,
  testId = 'resource-card',
}: ResourceCardProps) {
  const extra = (status || actions) && (
    <Space size="small">
      {status && <StatusTag status={status} mapping={statusMapping} />}
      {actions}
    </Space>
  );
  return (
    <Card
      className={className}
      data-testid={testId}
      size="small"
      title={
        <Space direction="vertical" size={0}>
          <Text strong>{title}</Text>
          {description && (
            <Text type="secondary" data-testid="resource-card-description">
              {description}
            </Text>
          )}
        </Space>
      }
      extra={extra}
    >
      {children}
    </Card>
  );
}

export default ResourceCard;
