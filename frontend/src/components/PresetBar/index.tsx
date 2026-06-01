import { useState } from 'react';
import { Button, message, Popover, Spin, Typography } from 'antd';
import { AppstoreAddOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { usePresets, type Preset } from '@/services/preset';
import { DeployWizard } from '@/pages/Deploy/DeployWizard';
import styles from './styles.module.css';

const { Text } = Typography;

/**
 * Top preset application bar (P12-T-204 / ADR-0022 §2.1). Folds the Deploy
 * page's catalog into a thin strip below the Header: each preset is a chip
 * showing basic info, a hover Popover with the full requirements, and a click
 * that opens the (reused) `<DeployWizard>`. On success a toast confirms the
 * deploy id; we stay on the workspace (no navigation — the user can flip the
 * workloads toggle to watch it land).
 *
 * Reuses `usePresets` + `<DeployWizard>` directly (not a fork) so the preset
 * catalog + 2-step deploy flow stay single-sourced with the /deploy page.
 */
export function PresetBar() {
  const { t } = useTranslation();
  const presetsQuery = usePresets();
  const [wizardPreset, setWizardPreset] = useState<Preset | null>(null);
  const [wizardOpen, setWizardOpen] = useState(false);

  const openWizard = (preset: Preset) => {
    setWizardPreset(preset);
    setWizardOpen(true);
  };

  const presets = presetsQuery.data ?? [];

  return (
    <div className={styles.bar} data-testid="preset-bar">
      <Text type="secondary" className={styles.barLabel}>
        <AppstoreAddOutlined /> {t('presetBar.title')}
      </Text>
      {presetsQuery.isLoading ? (
        <Spin size="small" data-testid="preset-bar-loading" />
      ) : presets.length === 0 ? (
        <Text type="secondary" style={{ fontSize: 12 }}>
          {t('presetBar.empty')}
        </Text>
      ) : (
        <div className={styles.barItems}>
          {presets.map((preset) => (
            <Popover
              key={preset.id}
              title={preset.name}
              trigger="hover"
              mouseEnterDelay={0.15}
              content={<PresetDetail preset={preset} />}
            >
              <Button
                size="small"
                data-testid={`preset-bar-item-${preset.id}`}
                onClick={() => openWizard(preset)}
              >
                {preset.name}
                {preset.requirements?.npuCount !== undefined
                  ? ` · ${preset.requirements.npuCount} NPU`
                  : ''}
              </Button>
            </Popover>
          ))}
        </div>
      )}

      <DeployWizard
        preset={wizardPreset}
        open={wizardOpen}
        onClose={() => setWizardOpen(false)}
        onSuccess={(response) => {
          message.success(t('deployMessages.successId', { id: response.deployId }));
          setWizardOpen(false);
        }}
        onError={(err) => {
          // 409 (resource conflict) is surfaced inline by the wizard.
          if (err.status === 409) return;
          message.error(t('deployMessages.error', { message: err.message }));
        }}
      />
    </div>
  );
}

/** Hover detail — same preset fields the /deploy grid card shows. */
function PresetDetail({ preset }: { preset: Preset }) {
  const { t } = useTranslation();
  return (
    <div className={styles.detail} data-testid="preset-bar-detail">
      {preset.kind && (
        <div className={styles.detailRow}>
          <Text type="secondary">Kind</Text>
          <Text>{preset.kind}</Text>
        </div>
      )}
      {preset.modelSize && (
        <div className={styles.detailRow}>
          <Text type="secondary">{t('deploy.preset.modelSize')}</Text>
          <Text>{preset.modelSize}</Text>
        </div>
      )}
      {preset.runtime && (
        <div className={styles.detailRow}>
          <Text type="secondary">{t('deploy.preset.runtime')}</Text>
          <Text>{preset.runtime}</Text>
        </div>
      )}
      {preset.requirements?.npuCount !== undefined && (
        <div className={styles.detailRow}>
          <Text type="secondary">{t('deploy.preset.npuCount')}</Text>
          <Text>{preset.requirements.npuCount}</Text>
        </div>
      )}
      {preset.requirements?.vramMiBPerNPU !== undefined && (
        <div className={styles.detailRow}>
          <Text type="secondary">{t('deploy.preset.vramPerNpu')}</Text>
          <Text>{preset.requirements.vramMiBPerNPU} MiB</Text>
        </div>
      )}
      {preset.description && (
        <div className={styles.detailDesc}>{preset.description}</div>
      )}
    </div>
  );
}

export default PresetBar;
