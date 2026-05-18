import { useState } from 'react';
import { message, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { usePresets, type Preset } from '@/services/preset';
import { DeployWizard } from './DeployWizard';
import { PresetGrid } from './PresetGrid';
import styles from './styles.module.css';

const { Title, Text } = Typography;

/**
 * `/deploy` — preset application catalog + 2-step deploy wizard
 * (P1-T-207).
 *
 * Layout:
 *   - Header: title + brief subtitle.
 *   - PresetGrid: cards fetched from `GET /api/v1/presets`. Clicking a
 *     card opens the wizard.
 *   - DeployWizard: AntD Modal hosting a 2-step Steps form (mode → config).
 *     Submission fires `POST /api/v1/deploy` via the mutation hook in
 *     `services/preset.ts`. 409 keeps the wizard open with an inline
 *     warning; success surfaces a toast and routes to `/workloads`.
 *
 * Architecture references:
 *   - docs/architecture.md §8.3 — application deploy UX expectations.
 *   - docs/api-contract.yaml — `/api/v1/presets*` + `/api/v1/deploy` POST.
 *
 * Drag-to-resource is a Should-Have per phase1-plan.md §9 DoD; deferred
 * to W4. The manual mode uses an AntD `Select` to pick node + slice ids.
 */
export default function DeployPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const presetsQuery = usePresets();

  const [wizardPreset, setWizardPreset] = useState<Preset | null>(null);
  const [wizardOpen, setWizardOpen] = useState(false);

  const handleSelectPreset = (preset: Preset) => {
    setWizardPreset(preset);
    setWizardOpen(true);
  };

  const handleClose = () => {
    setWizardOpen(false);
  };

  return (
    <div className={styles.page} data-testid="deploy-page">
      <div className={styles.headerRow}>
        <Title level={2} style={{ margin: 0 }}>
          {t('deploy.title')}
        </Title>
        <Text type="secondary">{t('deploy.subtitle')}</Text>
      </div>

      <PresetGrid
        presets={presetsQuery.data}
        isLoading={presetsQuery.isLoading}
        error={(presetsQuery.error as Error | null) ?? null}
        onRetry={() => void presetsQuery.refetch()}
        onSelect={handleSelectPreset}
        selectedId={wizardOpen ? wizardPreset?.id ?? null : null}
      />

      <DeployWizard
        preset={wizardPreset}
        open={wizardOpen}
        onClose={handleClose}
        onSuccess={(response) => {
          message.success(
            t('deployMessages.successId', { id: response.deployId }),
          );
          setWizardOpen(false);
          // Navigate to workloads so the user can watch the new deploy
          // progress. We only navigate on success; failures keep the
          // wizard open so the user can adjust.
          navigate('/workloads');
        }}
        onError={(err) => {
          // 409 is surfaced inline by the wizard (the wizard stays open).
          // 4xx / 5xx / network errors get a global toast here so the user
          // sees what failed even if the wizard isn't the right surface.
          if (err.status === 409) return;
          message.error(t('deployMessages.error', { message: err.message }));
        }}
      />
    </div>
  );
}
