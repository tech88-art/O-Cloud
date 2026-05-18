import { useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Form,
  Input,
  InputNumber,
  Modal,
  Radio,
  Select,
  Space,
  Steps,
  Typography,
} from 'antd';
import { useTranslation } from 'react-i18next';
import type {
  DeployMutationError,
  DeployRequest,
  DeployResponse,
  Preset,
} from '@/services/preset';
import {
  useDeployMutation,
  useNodeNpusList,
  useNodesList,
} from '@/services/preset';
import styles from './styles.module.css';

const { Text } = Typography;

export type DeployMode = 'auto' | 'manual';

const DEFAULT_NAMESPACE = 'ai-inference';
const DEFAULT_REPLICAS = 1;
const MIN_REPLICAS = 1;
const MAX_REPLICAS = 3;

export interface DeployWizardProps {
  /** The preset the user selected on the grid. `null` keeps the modal closed. */
  preset: Preset | null;
  /** Open / close. When `false` the wizard unmounts (form resets). */
  open: boolean;
  /** Close handler (cancel button + modal "X" + after-success default). */
  onClose: () => void;
  /** Optional success callback — parent surfaces toast + may navigate. */
  onSuccess?: (response: DeployResponse) => void;
  /**
   * Optional error callback — invoked for non-409 failures so the parent
   * can surface a global toast. 409 (resource conflict) is handled inline
   * by the wizard, so the parent typically ignores it.
   */
  onError?: (error: DeployMutationError) => void;
}

/**
 * Two-step deploy wizard:
 *   - Step 1: mode selection (auto | manual)
 *   - Step 2: form fields driven by mode
 *
 * Submit fires `POST /api/v1/deploy` via `useDeployMutation`. 409
 * (resource conflict) keeps the wizard open + surfaces an inline alert
 * so the user can adjust manual placement; other errors propagate to
 * the parent via the mutation hook for global message handling.
 *
 * Drag-to-resource (architecture.md §8.3) is a Should-Have per
 * phase1-plan.md §9 DoD — deferred to W4. Manual placement is captured
 * via AntD `Select` here.
 */
export function DeployWizard({
  preset,
  open,
  onClose,
  onSuccess,
  onError,
}: DeployWizardProps) {
  const { t } = useTranslation();

  const [step, setStep] = useState(0);
  const [mode, setMode] = useState<DeployMode>('auto');
  const [replicas, setReplicas] = useState<number>(DEFAULT_REPLICAS);
  const [namespace, setNamespace] = useState<string>(DEFAULT_NAMESPACE);
  const [nodeName, setNodeName] = useState<string | null>(null);
  const [npuSliceIds, setNpuSliceIds] = useState<string[]>([]);

  const mutation = useDeployMutation();

  // Reset wizard state every time it (re)opens, so picking a different
  // preset doesn't bleed previous selections through.
  useEffect(() => {
    if (open) {
      setStep(0);
      setMode('auto');
      setReplicas(DEFAULT_REPLICAS);
      setNamespace(DEFAULT_NAMESPACE);
      setNodeName(null);
      setNpuSliceIds([]);
      mutation.reset();
    }
    // We intentionally exclude `mutation` from deps — keeping the hook
    // reference identity-stable across renders. `open` and `preset.id`
    // are the actual triggers.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, preset?.id]);

  const nodesQuery = useNodesList({ enabled: open && mode === 'manual' });
  const npusQuery = useNodeNpusList(nodeName, {
    enabled: open && mode === 'manual' && Boolean(nodeName),
  });

  const npuOptions = useMemo(() => {
    if (!npusQuery.data) return [];
    return npusQuery.data.flatMap((npu) => {
      if (npu.slices && npu.slices.length > 0) {
        return npu.slices.map((slice) => ({
          value: slice.id,
          label: `${npu.id} / ${slice.id} · ${slice.status}`,
          disabled: slice.status !== 'available',
        }));
      }
      return [
        {
          value: npu.id,
          label: `${npu.id} · ${npu.status}`,
          disabled: npu.status !== 'healthy',
        },
      ];
    });
  }, [npusQuery.data]);

  const conflictError =
    mutation.error && (mutation.error as DeployMutationError).status === 409
      ? (mutation.error as DeployMutationError)
      : null;

  const handleSubmit = () => {
    if (!preset) return;

    const body: DeployRequest = {
      presetId: preset.id,
      namespace,
      replicas,
      scheduling: {
        mode,
      },
    };

    if (mode === 'manual' && nodeName) {
      body.scheduling = {
        mode: 'manual',
        manualPlacement: [
          {
            nodeName,
            npuSliceIds,
          },
        ],
      };
    }

    mutation.mutate(body, {
      onSuccess: (response) => {
        onSuccess?.(response);
      },
      onError: (err) => {
        // Inline 409 stays in the wizard (Alert is rendered from
        // `conflictError` below). Everything else hands off to the parent
        // for a global toast — we don't message.error() in-component so the
        // wizard stays purely presentational.
        onError?.(err);
      },
    });
  };

  // For non-409 errors we still want the wizard to know not to hang on
  // "loading" — react-query's `isError` flips automatically, and the
  // submit button respects `isPending`. No extra wiring needed.

  const canAdvanceFromStep1 = mode === 'auto' || mode === 'manual';
  const canSubmit =
    step === 1 &&
    namespace.trim().length > 0 &&
    replicas >= MIN_REPLICAS &&
    replicas <= MAX_REPLICAS &&
    (mode === 'auto' || Boolean(nodeName));

  return (
    <Modal
      title={preset ? `${t('deploy.title')} · ${preset.name}` : t('deploy.title')}
      open={open}
      footer={null}
      onCancel={onClose}
      width={640}
      destroyOnClose
      data-testid="deploy-wizard"
    >
      <Steps
        size="small"
        current={step}
        className={styles.wizardSteps}
        items={[
          { title: t('deploy.steps.mode') },
          { title: t('deploy.steps.config') },
        ]}
      />

      <div className={styles.wizardStepBody}>
        {step === 0 && (
          <Form layout="vertical" data-testid="deploy-wizard-step-mode">
            <Form.Item label={t('deploy.modeLabel')}>
              <Radio.Group
                value={mode}
                onChange={(e) => setMode(e.target.value as DeployMode)}
                data-testid="deploy-wizard-mode-radio"
              >
                <Space direction="vertical">
                  <Radio value="auto" data-testid="deploy-wizard-mode-auto">
                    <Text strong>{t('deploy.autoMode')}</Text>
                    <br />
                    <Text type="secondary">
                      {t('deploy.autoModeDescription')}
                    </Text>
                  </Radio>
                  <Radio value="manual" data-testid="deploy-wizard-mode-manual">
                    <Text strong>{t('deploy.manualMode')}</Text>
                    <br />
                    <Text type="secondary">
                      {t('deploy.manualModeDescription')}
                    </Text>
                  </Radio>
                </Space>
              </Radio.Group>
            </Form.Item>
          </Form>
        )}

        {step === 1 && (
          <Form layout="vertical" data-testid="deploy-wizard-step-config">
            <Form.Item
              label={t('deploy.replicas')}
              required
              className={styles.wizardFormRow}
            >
              <InputNumber
                min={MIN_REPLICAS}
                max={MAX_REPLICAS}
                value={replicas}
                onChange={(v) =>
                  setReplicas(typeof v === 'number' ? v : DEFAULT_REPLICAS)
                }
                data-testid="deploy-wizard-replicas"
              />
            </Form.Item>
            <Form.Item
              label={t('deploy.namespace')}
              required
              className={styles.wizardFormRow}
            >
              <Input
                value={namespace}
                onChange={(e) => setNamespace(e.target.value)}
                data-testid="deploy-wizard-namespace"
              />
            </Form.Item>

            {mode === 'manual' && (
              <>
                <Form.Item
                  label={t('deploy.node')}
                  required
                  className={styles.wizardFormRow}
                >
                  <Select
                    placeholder={t('deploy.nodePlaceholder')}
                    value={nodeName ?? undefined}
                    onChange={(v) => {
                      setNodeName(v);
                      setNpuSliceIds([]);
                    }}
                    loading={nodesQuery.isLoading}
                    options={(nodesQuery.data ?? []).map((n) => ({
                      value: n.name,
                      label: `${n.name} · ${n.status}`,
                    }))}
                    data-testid="deploy-wizard-node-select"
                  />
                </Form.Item>
                <Form.Item
                  label={t('deploy.npu')}
                  className={styles.wizardFormRow}
                >
                  <Select
                    mode="multiple"
                    placeholder={t('deploy.npuPlaceholder')}
                    value={npuSliceIds}
                    onChange={(v) => setNpuSliceIds(v)}
                    loading={npusQuery.isLoading}
                    options={npuOptions}
                    disabled={!nodeName}
                    data-testid="deploy-wizard-npu-select"
                  />
                </Form.Item>
                <Text type="secondary">{t('deploy.manualDragNote')}</Text>
              </>
            )}

            {conflictError && (
              <Alert
                type="warning"
                showIcon
                style={{ marginTop: 12 }}
                message={t('deployMessages.conflict')}
                description={conflictError.message}
                data-testid="deploy-wizard-conflict-alert"
              />
            )}
          </Form>
        )}
      </div>

      <div className={styles.wizardActions}>
        <Button onClick={onClose} data-testid="deploy-wizard-cancel">
          {t('deploy.cancel')}
        </Button>
        {step === 1 && (
          <Button
            onClick={() => setStep(0)}
            data-testid="deploy-wizard-back"
          >
            {t('deploy.back')}
          </Button>
        )}
        {step === 0 && (
          <Button
            type="primary"
            disabled={!canAdvanceFromStep1}
            onClick={() => setStep(1)}
            data-testid="deploy-wizard-next"
          >
            {t('deploy.next')}
          </Button>
        )}
        {step === 1 && (
          <Button
            type="primary"
            disabled={!canSubmit}
            loading={mutation.isPending}
            onClick={handleSubmit}
            data-testid="deploy-wizard-submit"
          >
            {t('deploy.submit')}
          </Button>
        )}
      </div>
    </Modal>
  );
}

export default DeployWizard;
