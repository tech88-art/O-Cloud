import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { ConfigProvider } from 'antd';
import i18n from '@/i18n';
import type { Node, NPU, Preset } from '@/services/preset';

/**
 * P12-T-205 DeployWizard tests.
 *
 * Migrated from the retired tests/Deploy.test.tsx: the /deploy page +
 * PresetGrid were retired (logic folded into the top PresetBar), but the
 * DeployWizard lives on (reused by PresetBar). These tests render the wizard
 * directly (open=true) instead of opening it via a preset card, preserving
 * the wizard coverage:
 *   1. Step transitions — mode → config.
 *   2. Auto-mode submit — POST /api/v1/deploy with scheduling.mode=auto.
 *   3. Manual-mode submit — node + slice picked, POST with manualPlacement.
 *   4. 409 — wizard stays open + warning Alert appears.
 */

const mockGet = vi.fn<(url: string, config?: unknown) => Promise<{ data: unknown }>>();
const mockPost = vi.fn<(url: string, body?: unknown) => Promise<{ data: unknown }>>();

vi.mock('@/services/api', () => ({
  api: {
    get: (url: string, config?: unknown) => mockGet(url, config),
    post: (url: string, body?: unknown) => mockPost(url, body),
  },
  queryClient: undefined,
}));

const { DeployWizard } = await import('@/pages/Deploy/DeployWizard');

// -------- Fixtures --------

const PRESET_PI: Preset = {
  id: 'pi-3b',
  name: 'Pi 3B Inference',
  kind: 'inference',
  modelSize: '3B',
  runtime: 'vllm',
  requirements: { npuCount: 1, vramMiBPerNPU: 16384 },
  description: 'Small inference baseline.',
  tags: ['small', 'inference'],
};

const PRESET_QWEN: Preset = {
  id: 'qwen-8b-pd',
  name: 'Qwen 8B PD',
  kind: 'inference-pd',
  modelSize: '8B',
  runtime: 'vllm',
  requirements: { npuCount: 2, vramMiBPerNPU: 32768 },
  description: 'Prefill-decode disaggregated.',
  tags: ['medium', 'pd'],
};

const PRESET_DEEPSEEK: Preset = {
  id: 'deepseek-20b',
  name: 'DeepSeek 20B',
  kind: 'inference',
  modelSize: '20B',
  runtime: 'mindie',
  requirements: { npuCount: 4, vramMiBPerNPU: 65536 },
  description: 'Large inference.',
  tags: ['large'],
};

const NODES: Node[] = [
  { name: 'node-1', status: 'Ready', arch: 'arm64', npuCount: 4 },
  { name: 'node-2', status: 'Ready', arch: 'arm64', npuCount: 4 },
];

const NPUS: NPU[] = [
  {
    id: 'npu-1-0',
    model: 'Ascend910B',
    status: 'healthy',
    vramMiB: 65536,
    sliceMode: 'fixed-template',
    slices: [
      { id: 'npu-1-0-slice-0', status: 'available' },
      { id: 'npu-1-0-slice-1', status: 'allocated' },
    ],
  },
  { id: 'npu-1-1', model: 'Ascend910B', status: 'healthy', vramMiB: 65536, sliceMode: 'whole' },
];

// -------- Helpers --------

function renderWizard(preset: Preset) {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: 0 },
      mutations: { retry: 0 },
    },
  });
  return render(
    <ConfigProvider>
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={qc}>
          <DeployWizard preset={preset} open onClose={() => {}} />
        </QueryClientProvider>
      </I18nextProvider>
    </ConfigProvider>,
  );
}

function mockNodesNpus() {
  mockGet.mockImplementation((url: string) => {
    if (url === '/api/v1/nodes') return Promise.resolve({ data: NODES });
    if (url.startsWith('/api/v1/nodes/') && url.endsWith('/npus')) {
      return Promise.resolve({ data: NPUS });
    }
    return Promise.reject(new Error(`unexpected URL: ${url}`));
  });
}

beforeEach(() => {
  mockGet.mockReset();
  mockPost.mockReset();
  void i18n.changeLanguage('zh-CN');
});

// -------- Tests --------

describe('DeployWizard — step transitions', () => {
  it(
    'opens at the mode step and advances to the config step via Next',
    async () => {
      mockNodesNpus();
      const user = userEvent.setup();
      renderWizard(PRESET_PI);

      expect(
        await screen.findByTestId('deploy-wizard', undefined, { timeout: 10_000 }),
      ).toBeInTheDocument();
      expect(
        await screen.findByTestId('deploy-wizard-step-mode', undefined, { timeout: 10_000 }),
      ).toBeInTheDocument();
      expect(screen.queryByTestId('deploy-wizard-step-config')).not.toBeInTheDocument();

      await user.click(screen.getByTestId('deploy-wizard-next'));

      expect(
        await screen.findByTestId('deploy-wizard-step-config', undefined, { timeout: 10_000 }),
      ).toBeInTheDocument();
      expect(screen.getByTestId('deploy-wizard-replicas')).toBeInTheDocument();
      expect(screen.getByTestId('deploy-wizard-namespace')).toBeInTheDocument();
      // Default mode is auto → no manual node select.
      expect(screen.queryByTestId('deploy-wizard-node-select')).not.toBeInTheDocument();
    },
    20_000,
  );
});

describe('DeployWizard — submission', () => {
  it(
    'submits an auto-mode deploy with the default replicas + namespace',
    async () => {
      mockNodesNpus();
      mockPost.mockResolvedValue({
        data: { deployId: 'dep-001', status: 'accepted', namespace: 'ai-inference' },
      });
      const user = userEvent.setup();
      renderWizard(PRESET_QWEN);

      await screen.findByTestId('deploy-wizard-next', undefined, { timeout: 10_000 });
      await user.click(screen.getByTestId('deploy-wizard-next'));
      await screen.findByTestId('deploy-wizard-submit', undefined, { timeout: 10_000 });
      await user.click(screen.getByTestId('deploy-wizard-submit'));

      await waitFor(() => {
        expect(mockPost).toHaveBeenCalledTimes(1);
      });
      const [url, body] = mockPost.mock.calls[0]!;
      expect(url).toBe('/api/v1/deploy');
      expect(body).toMatchObject({
        presetId: 'qwen-8b-pd',
        namespace: 'ai-inference',
        replicas: 1,
        scheduling: { mode: 'auto' },
      });
      const sched = (body as { scheduling: { manualPlacement?: unknown } }).scheduling;
      expect(sched.manualPlacement).toBeUndefined();
    },
    20_000,
  );

  it(
    'submits a manual-mode deploy with node + slice selections',
    async () => {
      mockNodesNpus();
      mockPost.mockResolvedValue({
        data: { deployId: 'dep-002', status: 'accepted', namespace: 'ai-inference' },
      });
      const user = userEvent.setup();
      renderWizard(PRESET_PI);

      const radioContainer = screen.getByTestId('deploy-wizard-mode-radio');
      const manualRadio = within(radioContainer).getByRole('radio', { name: /手动指定/ });
      await user.click(manualRadio);
      await user.click(screen.getByTestId('deploy-wizard-next'));

      await screen.findByTestId('deploy-wizard-node-select');
      await waitFor(() => {
        expect(mockGet.mock.calls.some(([url]) => url === '/api/v1/nodes')).toBe(true);
      });

      const nodeSelector = within(screen.getByTestId('deploy-wizard-node-select')).getByRole('combobox');
      fireEvent.mouseDown(nodeSelector);
      fireEvent.click(await screen.findByText(/node-1 · Ready/));

      await waitFor(() => {
        expect(
          mockGet.mock.calls.some(([url]) => String(url).endsWith('/api/v1/nodes/node-1/npus')),
        ).toBe(true);
      });
      const npuSelector = within(
        await screen.findByTestId('deploy-wizard-npu-select'),
      ).getByRole('combobox');
      fireEvent.mouseDown(npuSelector);
      fireEvent.click(await screen.findByText(/npu-1-0-slice-0 · available/));

      await user.click(screen.getByTestId('deploy-wizard-submit'));

      await waitFor(() => {
        expect(mockPost).toHaveBeenCalledTimes(1);
      });
      const [url, body] = mockPost.mock.calls[0]!;
      expect(url).toBe('/api/v1/deploy');
      expect(body).toMatchObject({
        presetId: 'pi-3b',
        namespace: 'ai-inference',
        replicas: 1,
        scheduling: {
          mode: 'manual',
          manualPlacement: [{ nodeName: 'node-1', npuSliceIds: ['npu-1-0-slice-0'] }],
        },
      });
    },
    20_000,
  );

  it(
    'keeps the wizard open and shows a warning Alert on a 409 conflict',
    async () => {
      mockNodesNpus();
      const axios409 = Object.assign(new Error('slice in use'), {
        isAxiosError: true,
        response: { status: 409, data: { message: 'slice in use' } },
      });
      mockPost.mockRejectedValue(axios409);
      const user = userEvent.setup();
      renderWizard(PRESET_DEEPSEEK);

      await screen.findByTestId('deploy-wizard-next', undefined, { timeout: 10_000 });
      await user.click(screen.getByTestId('deploy-wizard-next'));
      await screen.findByTestId('deploy-wizard-submit', undefined, { timeout: 10_000 });
      await user.click(screen.getByTestId('deploy-wizard-submit'));

      expect(
        await screen.findByTestId('deploy-wizard-conflict-alert', undefined, { timeout: 10_000 }),
      ).toBeInTheDocument();
      expect(screen.getByTestId('deploy-wizard')).toBeInTheDocument();
    },
    20_000,
  );
});
