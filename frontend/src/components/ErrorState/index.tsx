import { Alert, Button, Space } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import type { ReactNode } from 'react';

export interface ErrorStateProps {
  /**
   * The error to display. Accepts either an `Error` instance (its `message`
   * is rendered) or a raw string.
   */
  error: Error | string;
  /**
   * Optional retry callback. When provided, a "Retry" button is rendered
   * alongside the error alert.
   */
  onRetry?: () => void;
  /**
   * Optional label for the retry button. Defaults to "Retry".
   * Pass `t('common.retry')` for i18n. The component performs no
   * translation lookups itself (i18n files are outside T-107
   * Allowed Paths).
   */
  retryLabel?: ReactNode;
  /**
   * Optional title above the error message. Defaults to "Error".
   * Pass `t('common.error')` for i18n.
   */
  title?: ReactNode;
  /** Optional className for the alert root. */
  className?: string;
}

/**
 * Wrapper around AntD `Alert` (type=error) for displaying recoverable
 * errors. Use as the error state in any data-driven view per
 * `frontend/CLAUDE.md §4.8`.
 *
 * Usage:
 *   if (error) return <ErrorState error={error} onRetry={refetch} />;
 */
export function ErrorState({
  error,
  onRetry,
  retryLabel = 'Retry',
  title = 'Error',
  className,
}: ErrorStateProps) {
  const message = error instanceof Error ? error.message : error;
  const action = onRetry ? (
    <Space>
      <Button
        size="small"
        type="primary"
        icon={<ReloadOutlined />}
        onClick={onRetry}
        data-testid="error-state-retry"
      >
        {retryLabel}
      </Button>
    </Space>
  ) : undefined;
  return (
    <Alert
      className={className}
      type="error"
      showIcon
      message={title}
      description={message}
      action={action}
      data-testid="error-state"
    />
  );
}

export default ErrorState;
