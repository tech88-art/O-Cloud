import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { ConfigProvider } from 'antd';
import i18n from '@/i18n';

/**
 * P12-T-204 PresetBar tests. The bar lists preset apps (from usePresets) as
 * chips and opens the reused DeployWizard on click. We mock the api module so
 * usePresets resolves a fixture; the wizard's own behaviour (steps, submit,
 * 409) is covered by tests/Deploy.test.tsx — here we just assert the bar
 * renders chips and clicking one opens the wizard.
 */
const mockGet = vi.fn<(url: string, config?: unknown) => Promise<unknown>>();
const mockPost = vi.fn();
vi.mock('@/services/api', () => ({
  api: {
    get: (url: string, config?: unknown) => mockGet(url, config),
    post: (url: string, body?: unknown) => mockPost(url, body),
  },
}));

const { PresetBar } = await import('./index');

function presetsFixture() {
  return [
    {
      id: 'qwen-8b',
      name: 'Qwen 8B',
      kind: 'inference',
      runtime: 'vllm-ascend',
      modelSize: '8B',
      requirements: { npuCount: 1, vramMiBPerNPU: 32768 },
      tags: ['llm'],
      description: 'Qwen2 8B inference preset',
    },
  ];
}

function renderBar() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  });
  return render(
    <ConfigProvider>
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={qc}>
          <PresetBar />
        </QueryClientProvider>
      </I18nextProvider>
    </ConfigProvider>,
  );
}

describe('PresetBar (P12-T-204)', () => {
  beforeEach(() => {
    mockGet.mockReset();
    mockPost.mockReset();
  });

  it('renders a chip per preset with the NPU count', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/presets') return Promise.resolve({ data: presetsFixture() });
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    renderBar();

    const chip = await screen.findByTestId('preset-bar-item-qwen-8b');
    expect(chip).toBeInTheDocument();
    expect(chip).toHaveTextContent('Qwen 8B');
    expect(chip).toHaveTextContent('1 NPU');
  });

  it('opens the deploy wizard when a preset chip is clicked', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/presets') return Promise.resolve({ data: presetsFixture() });
      if (url === '/api/v1/nodes') return Promise.resolve({ data: [] });
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderBar();

    await user.click(await screen.findByTestId('preset-bar-item-qwen-8b'));

    // The wizard's step 1 is mode selection — assert the auto-mode label
    // (zh-CN default) appears once the modal opens.
    await waitFor(() => {
      expect(screen.getByText('自动调度')).toBeInTheDocument();
    });
  });
});
