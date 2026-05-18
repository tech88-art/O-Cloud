import { Skeleton, Space, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { EmptyState } from '@/components/EmptyState';
import { ErrorState } from '@/components/ErrorState';
import { ResourceCard } from '@/components/ResourceCard';
import type { Preset } from '@/services/preset';
import styles from './styles.module.css';

const { Text } = Typography;

export interface PresetGridProps {
  presets: Preset[] | undefined;
  isLoading: boolean;
  error: Error | null;
  onRetry: () => void;
  onSelect: (preset: Preset) => void;
  selectedId?: string | null;
}

/**
 * Grid of preset application cards. Each card surfaces a preset's name,
 * runtime + NPU requirements, and tags; clicking the card hands the
 * preset back via `onSelect` so the parent page can open the wizard.
 *
 * The component wraps the shared `ResourceCard` (P1-T-107) rather than
 * rolling a new card type — keeps a single visual vocabulary for "a
 * thing you can pick" across Overview, Workloads, and Deploy.
 *
 * Loading / error / empty states follow frontend/CLAUDE.md §4.8.
 */
export function PresetGrid({
  presets,
  isLoading,
  error,
  onRetry,
  onSelect,
  selectedId,
}: PresetGridProps) {
  const { t } = useTranslation();

  if (isLoading) {
    return (
      <div className={styles.gridStateOverlay} data-testid="preset-grid-loading">
        <Skeleton active title={false} paragraph={{ rows: 6 }} />
      </div>
    );
  }

  if (error) {
    return (
      <div className={styles.gridStateOverlay}>
        <ErrorState
          error={error}
          title={t('common.error')}
          retryLabel={t('common.retry')}
          onRetry={onRetry}
        />
      </div>
    );
  }

  if (!presets || presets.length === 0) {
    return (
      <div className={styles.gridStateOverlay}>
        <EmptyState description={t('common.empty')} />
      </div>
    );
  }

  return (
    <div className={styles.grid} data-testid="preset-grid">
      {presets.map((preset) => (
        <PresetGridCard
          key={preset.id}
          preset={preset}
          onSelect={onSelect}
          selected={preset.id === selectedId}
        />
      ))}
    </div>
  );
}

interface PresetGridCardProps {
  preset: Preset;
  onSelect: (preset: Preset) => void;
  selected: boolean;
}

function PresetGridCard({ preset, onSelect, selected }: PresetGridCardProps) {
  const { t } = useTranslation();
  const className = [
    styles.gridCardClickable,
    selected ? styles.gridCardSelected : '',
  ]
    .filter(Boolean)
    .join(' ');

  // The card itself is the click target so the entire surface feels
  // selectable. AntD's `Card` doesn't forward `onClick` to its root by
  // default — we wrap in a div with role=button to make it accessible
  // and testable without nesting an actual `<button>` inside `<button>`.
  return (
    <div
      role="button"
      tabIndex={0}
      data-testid={`preset-card-${preset.id}`}
      className={className}
      onClick={() => onSelect(preset)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onSelect(preset);
        }
      }}
    >
      <ResourceCard
        title={preset.name}
        description={preset.kind}
        actions={preset.tags && preset.tags.length > 0 ? (
          <Space size={4} wrap>
            {preset.tags.slice(0, 2).map((tag) => (
              <Text key={tag} type="secondary" data-testid="preset-tag">
                #{tag}
              </Text>
            ))}
          </Space>
        ) : undefined}
      >
        {preset.modelSize && (
          <div className={styles.requirementRow}>
            <Text type="secondary">{t('deploy.preset.modelSize')}</Text>
            <Text>{preset.modelSize}</Text>
          </div>
        )}
        {preset.runtime && (
          <div className={styles.requirementRow}>
            <Text type="secondary">{t('deploy.preset.runtime')}</Text>
            <Text>{preset.runtime}</Text>
          </div>
        )}
        {preset.requirements?.npuCount !== undefined && (
          <div className={styles.requirementRow}>
            <Text type="secondary">{t('deploy.preset.npuCount')}</Text>
            <Text data-testid="preset-npu-count">
              {preset.requirements.npuCount}
            </Text>
          </div>
        )}
        {preset.requirements?.vramMiBPerNPU !== undefined && (
          <div className={styles.requirementRow}>
            <Text type="secondary">{t('deploy.preset.vramPerNpu')}</Text>
            <Text>{preset.requirements.vramMiBPerNPU} MiB</Text>
          </div>
        )}
        {preset.description && (
          <div className={styles.description}>{preset.description}</div>
        )}
      </ResourceCard>
    </div>
  );
}

export default PresetGrid;
