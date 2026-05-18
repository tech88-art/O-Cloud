import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { MemoryRouter } from 'react-router-dom';
import { ConfigProvider } from 'antd';
import i18n from '@/i18n';
import type { Node, NPU, Preset } from '@/services/preset';

/**
 * P1-T-207 Deploy page tests.
 *
 * Coverage:
 *   1. Loading state — PresetGrid renders a skeleton while presets are pending.
 *   2. Error state  — PresetGrid renders ErrorState when presets fail.
 *   3. 4 preset cards rendered from `GET /api/v1/presets`.
 *   4. Wizard step transitions — click card → step 0 → next → step 1.
 *   5. Auto-mode submit — POST /api/v1/deploy with `scheduling.mode=auto`.
 *   6. Manual-mode submit — node + slice picked, POST with manualPlacement.
 *   7. 409 — wizard stays open + warning Alert appears.
 *
 * Mocks the axios `api` module at the boundary; doesn't try to render
 * the AntD `message` host so we avoid jsdom DOM-portal noise.
 */

const mockGet =
  vi.fn<(url: string, config?: unknown) => Promise<{ data: unknown }>>();
const mockPost = vi.fn<
  (url: string, body?: unknown) => Promise<{ data: unknown }>
>();

vi.mock('@/services/api', () => ({
  api: {
    get: (url: string, config?: unknown) => mockGet(url, config),
    post: (url: string, body?: unknown) => mockPost(url, body),
  },
  // The Deploy page never imports queryClient, but tsc + the module graph
  // pull this in transitively via `services/preset.ts → services/api.ts`.
  // Provide a stub so the mock module shape matches the original.
  queryClient: undefined,
}));

const { default: DeployPage } = await import('@/pages/Deploy');

// -------- Fixtures --------

const PRESETS: Preset[] = [
  {
    id: 'pi-3b',
    name: 'Pi 3B Inference',
    kind: 'inference',
    modelSize: '3B',
    runtime: 'vllm',
    requirements: { npuCount: 1, vramMiBPerNPU: 16384 },
    description: 'Small inference baseline.',
    tags: ['small', 'inference'],
  },
  {
    id: 'qwen-8b-pd',
    name: 'Qwen 8B PD',
    kind: 'inference-pd',
    modelSize: '8B',
    runtime: 'vllm',
    requirements: { npuCount: 2, vramMiBPerNPU: 32768 },
    description: 'Prefill-decode disaggregated.',
    tags: ['medium', 'pd'],
  },
  {
    id: 'deepseek-20b',
    name: 'DeepSeek 20B',
    kind: 'inference',
    modelSize: '20B',
    runtime: 'mindie',
    requirements: { npuCount: 4, vramMiBPerNPU: 65536 },
    description: 'Large inference.',
    tags: ['large'],
  },
  {
    id: 'benchmark',
    name: 'Benchmark Tool',
    kind: 'benchmark',
    runtime: 'vllm',
    description: 'Synthetic load test.',
    tags: ['tool'],
  },
];

const NODES: Node[] = [
  { name: 'node-1', status: 'Ready', arch: 'amd64', npuCount: 4 },
  { name: 'node-2', status: 'Ready', arch: 'amd64', npuCount: 4 },
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
  {
    id: 'npu-1-1',
    model: 'Ascend910B',
    status: 'healthy',
    vramMiB: 65536,
    sliceMode: 'whole',
  },
];

// -------- Helpers --------

function renderDeploy() {
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
          <MemoryRouter initialEntries={['/deploy']}>
            <DeployPage />
          </MemoryRouter>
        </QueryClientProvider>
      </I18nextProvider>
    </ConfigProvider>,
  );
}

function mockListPresets(presets: Preset[] | Promise<{ data: Preset[] }>) {
  mockGet.mockImplementation((url: string) => {
    if (url === '/api/v1/presets') {
      if (presets instanceof Promise) return presets;
      return Promise.resolve({ data: presets });
    }
    if (url === '/api/v1/nodes') {
      return Promise.resolve({ data: NODES });
    }
    if (url.startsWith('/api/v1/nodes/') && url.endsWith('/npus')) {
      return Promise.resolve({ data: NPUS });
    }
    return Promise.reject(new Error(`unexpected URL: ${url}`));
  });
}

beforeEach(() => {
  mockGet.mockReset();
  mockPost.mockReset();
  // Ensure i18n is in zh-CN so labels are predictable across runs (the
  // tests target i18n keys via translated text).
  void i18n.changeLanguage('zh-CN');
});

// -------- Tests --------

describe('DeployPage — preset grid', () => {
  it('renders a loading skeleton while presets are pending', () => {
    let resolvePresets: (v: { data: Preset[] }) => void = () => {};
    mockListPresets(
      new Promise<{ data: Preset[] }>((res) => {
        resolvePresets = res;
      }),
    );

    renderDeploy();

    expect(screen.getByTestId('preset-grid-loading')).toBeInTheDocument();

    // Cleanup: resolve the in-flight promise so react-query doesn't
    // log an "unhandled promise" warning after the test ends.
    resolvePresets({ data: PRESETS });
  });

  it('renders ErrorState when the preset fetch fails', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/presets') {
        return Promise.reject(new Error('preset boom'));
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    renderDeploy();

    expect(await screen.findByTestId('error-state')).toBeInTheDocument();
    expect(screen.getByTestId('error-state-retry')).toBeInTheDocument();
  });

  it('renders one card per preset (4 cards from the fixture)', async () => {
    mockListPresets(PRESETS);

    renderDeploy();

    expect(await screen.findByTestId('preset-grid')).toBeInTheDocument();
    for (const preset of PRESETS) {
      expect(screen.getByTestId(`preset-card-${preset.id}`)).toBeInTheDocument();
    }
    // Smoke-check that the requirement row renders the NPU count for at
    // least one card.
    expect(screen.getAllByTestId('preset-npu-count').length).toBeGreaterThan(0);
  });
});

describe('DeployPage — wizard step transitions', () => {
  it(
    'opens the wizard at step 0 when a preset card is clicked, and advances to step 1 via Next',
    async () => {
      mockListPresets(PRESETS);

      const user = userEvent.setup();
      renderDeploy();

      const card = await screen.findByTestId('preset-card-pi-3b');
      await user.click(card);

      // Step 0 mode form is rendered. AntD Modal + Steps mounting is slow
      // in jsdom (Steps measures with getComputedStyle), so we extend the
      // default findBy timeout.
      expect(
        await screen.findByTestId('deploy-wizard', undefined, {
          timeout: 10_000,
        }),
      ).toBeInTheDocument();
      expect(
        await screen.findByTestId('deploy-wizard-step-mode', undefined, {
          timeout: 10_000,
        }),
      ).toBeInTheDocument();
      expect(
        screen.queryByTestId('deploy-wizard-step-config'),
      ).not.toBeInTheDocument();

      // Advance to step 1.
      await user.click(screen.getByTestId('deploy-wizard-next'));

      expect(
        await screen.findByTestId('deploy-wizard-step-config', undefined, {
          timeout: 10_000,
        }),
      ).toBeInTheDocument();
      expect(screen.getByTestId('deploy-wizard-replicas')).toBeInTheDocument();
      expect(screen.getByTestId('deploy-wizard-namespace')).toBeInTheDocument();
      // Manual fields should NOT be present (default mode is auto).
      expect(
        screen.queryByTestId('deploy-wizard-node-select'),
      ).not.toBeInTheDocument();
    },
    20_000,
  );
});

describe('DeployPage — submission', () => {
  it(
    'submits an auto-mode deploy with the default replicas + namespace',
    async () => {
      mockListPresets(PRESETS);
      mockPost.mockResolvedValue({
        data: {
          deployId: 'dep-001',
          status: 'accepted',
          namespace: 'ai-inference',
        },
      });

      const user = userEvent.setup();
      renderDeploy();

      const card = await screen.findByTestId('preset-card-qwen-8b-pd');
      await user.click(card);
      // Wait for the modal + Steps to mount before clicking Next; AntD
      // measures the DOM on mount which is slow in jsdom.
      await screen.findByTestId('deploy-wizard-next', undefined, {
        timeout: 10_000,
      });
      await user.click(screen.getByTestId('deploy-wizard-next'));
      await screen.findByTestId('deploy-wizard-submit', undefined, {
        timeout: 10_000,
      });
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
      // Auto mode never sends manualPlacement.
      const sched = (body as { scheduling: { manualPlacement?: unknown } })
        .scheduling;
      expect(sched.manualPlacement).toBeUndefined();
    },
    20_000,
  );

  it(
    'submits a manual-mode deploy with node + slice selections',
    async () => {
      mockListPresets(PRESETS);
      mockPost.mockResolvedValue({
        data: {
          deployId: 'dep-002',
          status: 'accepted',
          namespace: 'ai-inference',
        },
      });

      const user = userEvent.setup();
      renderDeploy();

      const card = await screen.findByTestId('preset-card-pi-3b');
      await user.click(card);

      // Pick manual mode.
      const radioContainer = screen.getByTestId('deploy-wizard-mode-radio');
      const manualRadio = within(radioContainer).getByRole('radio', {
        name: /手动指定/,
      });
      await user.click(manualRadio);

      await user.click(screen.getByTestId('deploy-wizard-next'));

      // Wait for the manual-mode form (node + npu Selects) to mount.
      await screen.findByTestId('deploy-wizard-node-select');

      // Wait for the node list to land so the AntD Select has options to
      // render. The Select itself fires no request — useNodesList does.
      await waitFor(() => {
        expect(
          mockGet.mock.calls.some(([url]) => url === '/api/v1/nodes'),
        ).toBe(true);
      });

      // AntD Select dropdown popup uses `getComputedStyle` for scrollbar
      // measurement, which jsdom only stubs partially. Driving the Select
      // via `fireEvent.mouseDown` on the selector opens the listbox using
      // the same code path AntD's own tests rely on. We then `click` the
      // option from the body-portaled listbox.
      const nodeSelector = within(
        screen.getByTestId('deploy-wizard-node-select'),
      ).getByRole('combobox');
      fireEvent.mouseDown(nodeSelector);
      const nodeOption = await screen.findByText(/node-1 · Ready/);
      fireEvent.click(nodeOption);

      // Wait for the NPU query to fire on the selected node.
      await waitFor(() => {
        expect(
          mockGet.mock.calls.some(([url]) =>
            String(url).endsWith('/api/v1/nodes/node-1/npus'),
          ),
        ).toBe(true);
      });
      const npuSelector = within(
        await screen.findByTestId('deploy-wizard-npu-select'),
      ).getByRole('combobox');
      fireEvent.mouseDown(npuSelector);
      const sliceOption = await screen.findByText(
        /npu-1-0-slice-0 · available/,
      );
      fireEvent.click(sliceOption);

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
          manualPlacement: [
            {
              nodeName: 'node-1',
              npuSliceIds: ['npu-1-0-slice-0'],
            },
          ],
        },
      });
    },
    20_000,
  );

  it(
    'keeps the wizard open and shows a warning Alert on a 409 conflict',
    async () => {
      mockListPresets(PRESETS);
      // Simulate an axios 409 error shape.
      const axios409 = Object.assign(new Error('slice in use'), {
        isAxiosError: true,
        response: {
          status: 409,
          data: { message: 'slice in use' },
        },
      });
      mockPost.mockRejectedValue(axios409);

      const user = userEvent.setup();
      renderDeploy();

      const card = await screen.findByTestId('preset-card-deepseek-20b');
      await user.click(card);
      await screen.findByTestId('deploy-wizard-next', undefined, {
        timeout: 10_000,
      });
      await user.click(screen.getByTestId('deploy-wizard-next'));
      await screen.findByTestId('deploy-wizard-submit', undefined, {
        timeout: 10_000,
      });
      await user.click(screen.getByTestId('deploy-wizard-submit'));

      // Wizard stays mounted + inline conflict alert appears.
      expect(
        await screen.findByTestId('deploy-wizard-conflict-alert', undefined, {
          timeout: 10_000,
        }),
      ).toBeInTheDocument();
      expect(screen.getByTestId('deploy-wizard')).toBeInTheDocument();
    },
    20_000,
  );
});
