import { Empty } from 'antd';
import type { ReactNode } from 'react';

export interface EmptyStateProps {
  /**
   * Description text. Defaults to AntD's built-in "No data" (locale-aware,
   * supplied by AntD's ConfigProvider — see `App.tsx` for the active locale).
   *
   * Pass `t('common.emptyState')` to override with a project-specific
   * i18n message. The component performs no translation lookups itself
   * (i18n files are outside T-107 Allowed Paths; consumers supply text).
   */
  description?: ReactNode;
  /** Optional CTA / action button rendered under the description. */
  action?: ReactNode;
  /** Optional className for the empty root. */
  className?: string;
}

/**
 * Thin wrapper around AntD `Empty`. Use as the empty-data state in any
 * list / table / panel per `frontend/CLAUDE.md §4.8`.
 *
 * Usage:
 *   {data.length === 0 && (
 *     <EmptyState
 *       description={t('workloads.empty')}
 *       action={<Button onClick={openDeploy}>Deploy</Button>}
 *     />
 *   )}
 */
export function EmptyState({ description, action, className }: EmptyStateProps) {
  return (
    <Empty
      className={className}
      data-testid="empty-state"
      description={description}
    >
      {action}
    </Empty>
  );
}

export default EmptyState;
