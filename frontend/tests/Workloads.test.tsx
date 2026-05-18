import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { ConfigProvider } from 'antd';
import i18n from '@/i18n';
import type {
  Workload,
  WorkloadDetail,
} from '@/services/workload';

/**
 * P1-T-206 Workloads page tests.
 *
 * 8 cases:
 *   1. loading skeleton while the list fetch is in-flight
 *   2. ErrorState when the list fetch fails (with retry)
 *   3. EmptyState when the list returns []
 *   4. list renders all rows from the fixture
 *   5. status filter is forwarded as `?status=` server-side
 *   6. row click opens the Drawer + fetches detail
 *   7. Drawer renders pod info + container command/args/resources
 *   8. Drawer renders PD-pair relation as a Tag + the metrics href
 *
 * Pattern mirrors `tests/Overview.test.tsx` — mock `@/services/api` at the
 * module boundary so we can drive URL → response mapping per test.
 */

const mockGet = vi.fn<(url: string, config?: unknown) => Promise<unknown>>();
vi.mock('@/services/api', () => ({
  api: {
    get: (url: string, config?: unknown) => mockGet(url, config),
  },
}));

// Imports of the SUT must come AFTER the mocks are registered.
const { default: WorkloadsPage } = await import('@/pages/Workloads');

// -------- Fixtures --------

function workloadList(): Workload[] {
  return [
    {
      name: 'qwen-8b-pd',
      namespace: 'ai-inference',
      type: 'inference',
      kind: 'InferenceService',
      status: 'running',
      replicas: { desired: 2, ready: 2 },
      nodeNames: ['worker-site-a-01', 'worker-site-a-02'],
      createdAt: '2026-05-17T08:00:00Z',
    },
    {
      name: 'oom-test-model',
      namespace: 'ai-inference',
      type: 'inference',
      kind: 'Deployment',
      status: 'failed',
      replicas: { desired: 1, ready: 0 },
      nodeNames: ['worker-site-a-03'],
      createdAt: '2026-05-17T09:00:00Z',
    },
    {
      name: 'llama2-7b-finetune',
      namespace: 'training',
      type: 'training',
      kind: 'PyTorchJob',
      status: 'pending',
      replicas: { desired: 1, ready: 0 },
      nodeNames: [],
      createdAt: '2026-05-17T10:00:00Z',
    },
  ];
}

function workloadDetailFixture(): WorkloadDetail {
  return {
    name: 'qwen-8b-pd',
    namespace: 'ai-inference',
    type: 'inference',
    kind: 'InferenceService',
    status: 'running',
    replicas: { desired: 2, ready: 2 },
    nodeNames: ['worker-site-a-01', 'worker-site-a-02'],
    pods: [
      {
        name: 'qwen-8b-pd-prefill-0',
        namespace: 'ai-inference',
        nodeName: 'worker-site-a-01',
        status: 'Running',
        containers: [
          {
            name: 'prefill',
            image: 'mindie/vllm-ascend:0.11.0',
            command: ['/bin/sh', '-c'],
            args: [
              'vllm serve qwen/Qwen2.5-8B-Instruct --role prefill --kv-transfer-config mooncake',
            ],
            resources: {
              cpu: '16',
              memory: '64Gi',
              npuSlices: ['worker-site-a-01-npu-1'],
            },
          },
        ],
      },
      {
        name: 'qwen-8b-pd-decode-0',
        namespace: 'ai-inference',
        nodeName: 'worker-site-a-02',
        status: 'Running',
        containers: [
          {
            name: 'decode',
            image: 'mindie/vllm-ascend:0.11.0',
            command: ['/bin/sh', '-c'],
            args: [
              'vllm serve qwen/Qwen2.5-8B-Instruct --role decode --kv-transfer-config mooncake',
            ],
            resources: {
              cpu: '16',
              memory: '64Gi',
              npuSlices: ['worker-site-a-02-npu-1'],
            },
          },
        ],
      },
    ],
    relations: [
      {
        from: 'qwen-8b-pd-prefill-0',
        to: 'qwen-8b-pd-decode-0',
        type: 'pd-pair',
      },
    ],
  };
}

// -------- Render helper --------

function renderPage() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: 0 },
    },
  });
  return render(
    <ConfigProvider>
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={qc}>
          <WorkloadsPage />
        </QueryClientProvider>
      </I18nextProvider>
    </ConfigProvider>,
  );
}

// -------- Setup --------

beforeEach(() => {
  mockGet.mockReset();
});

// -------- Tests --------

describe('WorkloadsPage — list state', () => {
  it('renders the loading skeleton while the list fetch is in-flight', async () => {
    let resolveList: (v: Workload[]) => void = () => {};
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/workloads') {
        return new Promise<{ data: Workload[] }>((res) => {
          resolveList = (data) => res({ data });
        });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    renderPage();

    expect(screen.getByTestId('workloads-loading')).toBeInTheDocument();
    expect(screen.queryByTestId('workload-table')).not.toBeInTheDocument();
    resolveList(workloadList());
  });

  it('renders ErrorState when the list fetch fails', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/workloads') {
        return Promise.reject(new Error('boom'));
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    renderPage();

    expect(await screen.findByTestId('error-state')).toBeInTheDocument();
    expect(screen.getByTestId('error-state-retry')).toBeInTheDocument();
  });

  it('renders EmptyState when the list returns []', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/workloads') {
        return Promise.resolve({ data: [] as Workload[] });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    renderPage();

    expect(await screen.findByTestId('workloads-empty')).toBeInTheDocument();
    expect(screen.getByTestId('empty-state')).toBeInTheDocument();
  });

  it('renders all rows from the fixture once data lands', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/workloads') {
        return Promise.resolve({ data: workloadList() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    renderPage();

    expect(await screen.findByTestId('workload-table')).toBeInTheDocument();
    expect(
      screen.getByTestId('workload-row-ai-inference/qwen-8b-pd'),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId('workload-row-ai-inference/oom-test-model'),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId('workload-row-training/llama2-7b-finetune'),
    ).toBeInTheDocument();
    // Replicas rendered as "ready/desired"
    expect(
      screen.getByTestId('workload-replicas-ai-inference/qwen-8b-pd'),
    ).toHaveTextContent('2/2');
    expect(
      screen.getByTestId('workload-replicas-ai-inference/oom-test-model'),
    ).toHaveTextContent('0/1');
  });
});

describe('WorkloadsPage — filter passthrough', () => {
  it('passes the chosen status to the server as ?status=', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/workloads') {
        return Promise.resolve({ data: workloadList() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderPage();

    // First call has no status filter.
    await waitFor(() => {
      expect(mockGet).toHaveBeenCalledWith('/api/v1/workloads', {
        params: {},
      });
    });

    // AntD `Select` opens an option list when clicked. The status select
    // lives inside the filter bar; pick "running".
    const statusSelect = within(screen.getByTestId('workloads-filter-status'))
      .getByRole('combobox');
    await user.click(statusSelect);
    // Use AntD's option text — `running` appears in the dropdown popup
    // (rendered to document.body, so use a global query).
    const runningOption = await screen.findByText('running', {
      selector: '.ant-select-item-option-content',
    });
    await user.click(runningOption);

    await waitFor(() => {
      expect(mockGet).toHaveBeenCalledWith('/api/v1/workloads', {
        params: { status: 'running' },
      });
    });
  });
});

describe('WorkloadsPage — drawer detail', () => {
  it('opens the drawer + fetches detail on row click', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/workloads') {
        return Promise.resolve({ data: workloadList() });
      }
      if (url === '/api/v1/workloads/ai-inference/qwen-8b-pd') {
        return Promise.resolve({ data: workloadDetailFixture() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderPage();

    await screen.findByTestId('workload-table');
    await user.click(
      screen.getByTestId('workload-row-ai-inference/qwen-8b-pd'),
    );

    await waitFor(() => {
      expect(
        mockGet.mock.calls.some(
          ([url]) => url === '/api/v1/workloads/ai-inference/qwen-8b-pd',
        ),
      ).toBe(true);
    });
    expect(await screen.findByTestId('workload-detail-body')).toBeInTheDocument();
  });

  it('renders pod info + container command/args/resources in the drawer', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/workloads') {
        return Promise.resolve({ data: workloadList() });
      }
      if (url === '/api/v1/workloads/ai-inference/qwen-8b-pd') {
        return Promise.resolve({ data: workloadDetailFixture() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderPage();

    await screen.findByTestId('workload-table');
    await user.click(
      screen.getByTestId('workload-row-ai-inference/qwen-8b-pd'),
    );

    expect(
      await screen.findByTestId('workload-pod-qwen-8b-pd-prefill-0'),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId('workload-pod-qwen-8b-pd-decode-0'),
    ).toBeInTheDocument();
    // Scope assertions to the prefill container so the matching decode rows
    // don't make the "multiple elements" queries ambiguous.
    const prefillContainer = screen.getByTestId('workload-container-prefill');
    expect(prefillContainer).toBeInTheDocument();
    expect(within(prefillContainer).getByText('/bin/sh -c')).toBeInTheDocument();
    expect(
      within(prefillContainer).getByText(
        /vllm serve qwen\/Qwen2\.5-8B-Instruct --role prefill/,
      ),
    ).toBeInTheDocument();
    // Resources block surfaces cpu / memory / npu slice id
    expect(
      within(prefillContainer).getByText(
        /cpu: 16 · memory: 64Gi · npu: worker-site-a-01-npu-1/,
      ),
    ).toBeInTheDocument();
  });

  it('renders the PD-pair relation tag + a metrics link href', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/workloads') {
        return Promise.resolve({ data: workloadList() });
      }
      if (url === '/api/v1/workloads/ai-inference/qwen-8b-pd') {
        return Promise.resolve({ data: workloadDetailFixture() });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderPage();

    await screen.findByTestId('workload-table');
    await user.click(
      screen.getByTestId('workload-row-ai-inference/qwen-8b-pd'),
    );

    const relationTag = await screen.findByTestId('workload-relation-0');
    expect(relationTag).toHaveTextContent('qwen-8b-pd-prefill-0');
    expect(relationTag).toHaveTextContent('qwen-8b-pd-decode-0');
    expect(relationTag).toHaveTextContent('pd-pair');

    const metricsLink = screen.getByTestId('workload-detail-metrics-href');
    expect(metricsLink).toHaveAttribute(
      'href',
      '/metrics?workload=ai-inference/qwen-8b-pd',
    );
  });
});
