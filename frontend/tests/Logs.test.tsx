import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { ConfigProvider } from 'antd';
import i18n from '@/i18n';
import { loadRuntimeConfig } from '@/config/runtime';
import type { Workload, WorkloadDetail } from '@/services/workload';
import type { LogPage } from '@/services/logs';

/**
 * P1-T-302 Logs page tests.
 *
 * Pattern mirrors Workloads.test.tsx / Overview.test.tsx:
 *   - api.get mocked at the module boundary, dispatched by URL
 *   - WebSocket class stubbed so the live-stream toggle can be exercised
 *     without a real server
 *
 * 6 cases:
 *   1. empty state when no workload is selected
 *   2. picking a workload fetches the REST tail and renders lines
 *   3. switching workloads triggers a fresh tail fetch + clears stale lines
 *   4. container filter is forwarded as ?container=X
 *   5. flipping the Live toggle ON opens a /ws/logs/:ns/:name WebSocket and
 *      newly-pushed lines append to the viewer
 *   6. flipping Live OFF closes the socket
 */

const mockGet = vi.fn<(url: string, config?: unknown) => Promise<unknown>>();
vi.mock('@/services/api', () => ({
  api: {
    get: (url: string, config?: unknown) => mockGet(url, config),
  },
}));

const { default: LogsPage } = await import('@/pages/Logs');

// -------- Mock WebSocket --------

interface MockWS {
  url: string;
  readyState: number;
  onopen: ((ev: Event) => void) | null;
  onmessage: ((ev: MessageEvent) => void) | null;
  onclose: ((ev: CloseEvent) => void) | null;
  onerror: ((ev: Event) => void) | null;
  close: () => void;
  push: (data: unknown) => void;
}

const wsInstances: MockWS[] = [];

class MockWebSocket {
  url: string;
  readyState = 0;
  onopen: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;

  constructor(url: string) {
    this.url = url;
    wsInstances.push(this as unknown as MockWS);
    // Open async so the consumer can register onopen first.
    queueMicrotask(() => {
      this.readyState = 1;
      this.onopen?.(new Event('open'));
    });
    (this as unknown as MockWS).push = (data: unknown) => {
      this.onmessage?.(new MessageEvent('message', { data: JSON.stringify(data) }));
    };
  }

  close() {
    this.readyState = 3;
    this.onclose?.(new CloseEvent('close'));
  }
}

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
    },
    {
      name: 'oom-test-model',
      namespace: 'training',
      type: 'training',
      kind: 'Deployment',
      status: 'failed',
      replicas: { desired: 1, ready: 0 },
    },
  ];
}

function workloadDetailFixture(): WorkloadDetail {
  return {
    name: 'qwen-8b-pd',
    namespace: 'ai-inference',
    type: 'inference',
    status: 'running',
    pods: [
      {
        name: 'qwen-8b-pd-prefill-0',
        namespace: 'ai-inference',
        containers: [
          { name: 'prefill', image: 'mindie/vllm-ascend:0.11.0' },
          { name: 'decode', image: 'mindie/vllm-ascend:0.11.0' },
        ],
      },
    ],
  };
}

function logPage(n: number, container = 'prefill'): LogPage {
  const lines = Array.from({ length: n }).map((_, i) => ({
    timestamp: new Date(Date.UTC(2026, 4, 18, 9, 0, i)).toISOString(),
    level: i % 7 === 0 ? 'WARN' : 'INFO',
    container,
    message: `line ${i}`,
  }));
  return { lines, hasMore: false };
}

// -------- Helpers --------

/**
 * Open an AntD Select identified by data-testid and pick the option matching
 * `optionLabel`. Mirrors the pattern from tests/Metrics.test.tsx: the trigger
 * is the inner `.ant-select-selector`, options render into a portal as
 * `.ant-select-item-option-content` (querying by class avoids the
 * duplicate-match the selected-value display would otherwise produce).
 */
async function pickSelectOption(
  user: ReturnType<typeof userEvent.setup>,
  selectTestId: string,
  optionLabel: string,
): Promise<void> {
  const select = screen.getByTestId(selectTestId);
  const trigger = select.querySelector('.ant-select-selector');
  if (!trigger) throw new Error(`no .ant-select-selector inside ${selectTestId}`);
  await user.click(trigger);
  const option = await waitFor(() => {
    const items = document.querySelectorAll('.ant-select-item-option-content');
    const match = Array.from(items).find((el) => el.textContent === optionLabel);
    if (!match) throw new Error(`no option "${optionLabel}" yet`);
    return match;
  });
  await user.click(option);
}

// -------- Render helpers --------

function renderLogs() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  });
  return render(
    <ConfigProvider>
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={qc}>
          <LogsPage />
        </QueryClientProvider>
      </I18nextProvider>
    </ConfigProvider>,
  );
}

// -------- Setup --------

beforeEach(async () => {
  mockGet.mockReset();
  wsInstances.length = 0;
  vi.stubGlobal('WebSocket', MockWebSocket);
  await loadRuntimeConfig();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

// -------- Tests --------

describe('LogsPage — initial state', () => {
  it('renders the empty placeholder when no workload is selected', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/workloads') return Promise.resolve({ data: workloadList() });
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    renderLogs();

    // Workload picker is rendered…
    expect(await screen.findByTestId('logs-workload-select')).toBeInTheDocument();
    // …and the viewer shows the "pick a workload" CTA, not the empty-logs state.
    const empty = await screen.findByTestId('empty-state');
    expect(empty).toBeInTheDocument();
  });
});

describe('LogsPage — REST tail fetch', () => {
  it('fetches the REST tail when a workload is picked and renders lines', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/workloads') return Promise.resolve({ data: workloadList() });
      if (url === '/api/v1/workloads/ai-inference/qwen-8b-pd') {
        return Promise.resolve({ data: workloadDetailFixture() });
      }
      if (url === '/api/v1/workloads/ai-inference/qwen-8b-pd/logs') {
        return Promise.resolve({ data: logPage(5) });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderLogs();

    // Open the workload select and pick qwen-8b-pd via the AntD-portal helper.
    await pickSelectOption(user, 'logs-workload-select', 'ai-inference/qwen-8b-pd');

    await waitFor(() => {
      const rows = screen.queryAllByTestId('log-viewer-row');
      expect(rows.length).toBe(5);
    });
    expect(screen.getAllByTestId('log-viewer-row')[0]).toHaveTextContent('line 0');
  });

  it('forwards container filter as a query param', async () => {
    mockGet.mockImplementation((url: string, config?: unknown) => {
      const params = (config as { params?: Record<string, unknown> } | undefined)?.params;
      if (url === '/api/v1/workloads') return Promise.resolve({ data: workloadList() });
      if (url === '/api/v1/workloads/ai-inference/qwen-8b-pd') {
        return Promise.resolve({ data: workloadDetailFixture() });
      }
      if (url === '/api/v1/workloads/ai-inference/qwen-8b-pd/logs') {
        return Promise.resolve({ data: logPage(3, (params?.container as string) || 'all') });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderLogs();

    await pickSelectOption(user, 'logs-workload-select', 'ai-inference/qwen-8b-pd');

    // Wait for the initial unfiltered fetch to land so the detail query
    // resolves before we touch the container dropdown.
    await waitFor(() => {
      expect(screen.queryAllByTestId('log-viewer-row').length).toBe(3);
    });

    // Open the container dropdown and pick "decode" via the portal helper.
    await pickSelectOption(user, 'logs-container-select', 'decode');

    await waitFor(() => {
      const filteredCall = mockGet.mock.calls.find(([url, cfg]) => {
        if (!String(url).endsWith('/logs')) return false;
        const p = (cfg as { params?: Record<string, unknown> } | undefined)?.params;
        return p?.container === 'decode';
      });
      expect(filteredCall).toBeTruthy();
    });
  });

  it('clears the buffer when the user switches workloads', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/workloads') return Promise.resolve({ data: workloadList() });
      if (url === '/api/v1/workloads/ai-inference/qwen-8b-pd') {
        return Promise.resolve({ data: workloadDetailFixture() });
      }
      if (url === '/api/v1/workloads/training/oom-test-model') {
        return Promise.resolve({
          data: {
            ...workloadDetailFixture(),
            name: 'oom-test-model',
            namespace: 'training',
            pods: [],
          },
        });
      }
      if (url.endsWith('/logs')) {
        return Promise.resolve({ data: logPage(4) });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderLogs();

    await pickSelectOption(user, 'logs-workload-select', 'ai-inference/qwen-8b-pd');

    await waitFor(() => {
      expect(screen.queryAllByTestId('log-viewer-row').length).toBe(4);
    });

    // Switch to the other workload via the portal helper.
    await pickSelectOption(user, 'logs-workload-select', 'training/oom-test-model');

    // The buffer is cleared on workload change; the new tail fetch lands
    // shortly after — the final state shows the new lines.
    await waitFor(() => {
      const rows = screen.queryAllByTestId('log-viewer-row');
      expect(rows.length).toBe(4);
    });
  });
});

describe('LogsPage — live stream', () => {
  it('opens a /ws/logs/:ns/:name socket when the live toggle flips ON and appends pushed lines', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/workloads') return Promise.resolve({ data: workloadList() });
      if (url === '/api/v1/workloads/ai-inference/qwen-8b-pd') {
        return Promise.resolve({ data: workloadDetailFixture() });
      }
      if (url.endsWith('/logs')) {
        return Promise.resolve({ data: logPage(2) });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderLogs();

    await pickSelectOption(user, 'logs-workload-select', 'ai-inference/qwen-8b-pd');

    await waitFor(() => {
      expect(screen.queryAllByTestId('log-viewer-row').length).toBe(2);
    });

    // Flip the live toggle ON.
    await user.click(screen.getByTestId('logs-live-switch'));

    // A WS instance should be created against /ws/logs/ai-inference/qwen-8b-pd.
    await waitFor(() => {
      expect(wsInstances.length).toBe(1);
    });
    expect(wsInstances[0].url).toContain('/ws/logs/ai-inference/qwen-8b-pd');

    // Push two WSMessage envelopes (canonical type "log.line"); the viewer
    // should grow from 2 → 4 rows.
    act(() => {
      wsInstances[0].push({
        type: 'log.line',
        timestamp: '2026-05-18T09:01:00Z',
        payload: {
          timestamp: '2026-05-18T09:01:00Z',
          level: 'INFO',
          container: 'prefill',
          message: 'pushed-1',
        },
      });
      wsInstances[0].push({
        type: 'log.line',
        timestamp: '2026-05-18T09:01:01Z',
        payload: {
          timestamp: '2026-05-18T09:01:01Z',
          level: 'ERROR',
          container: 'prefill',
          message: 'pushed-2',
        },
      });
    });

    await waitFor(() => {
      const rows = screen.queryAllByTestId('log-viewer-row');
      expect(rows.length).toBe(4);
    });
    expect(screen.getAllByTestId('log-viewer-row').slice(-1)[0]).toHaveTextContent(
      'pushed-2',
    );
  });

  it('closes the socket when the live toggle flips OFF', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/workloads') return Promise.resolve({ data: workloadList() });
      if (url === '/api/v1/workloads/ai-inference/qwen-8b-pd') {
        return Promise.resolve({ data: workloadDetailFixture() });
      }
      if (url.endsWith('/logs')) {
        return Promise.resolve({ data: logPage(1) });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderLogs();

    await pickSelectOption(user, 'logs-workload-select', 'ai-inference/qwen-8b-pd');
    await waitFor(() => {
      expect(screen.queryAllByTestId('log-viewer-row').length).toBe(1);
    });

    await user.click(screen.getByTestId('logs-live-switch'));
    await waitFor(() => {
      expect(wsInstances.length).toBe(1);
      expect(wsInstances[0].readyState).toBe(1);
    });

    await user.click(screen.getByTestId('logs-live-switch'));
    // Closing is synchronous in the cleanup effect.
    await waitFor(() => {
      expect(wsInstances[0].readyState).toBe(3);
    });
  });

  it('ignores non-log.line WS frames', async () => {
    mockGet.mockImplementation((url: string) => {
      if (url === '/api/v1/workloads') return Promise.resolve({ data: workloadList() });
      if (url === '/api/v1/workloads/ai-inference/qwen-8b-pd') {
        return Promise.resolve({ data: workloadDetailFixture() });
      }
      if (url.endsWith('/logs')) {
        return Promise.resolve({ data: logPage(1) });
      }
      return Promise.reject(new Error(`unexpected URL: ${url}`));
    });

    const user = userEvent.setup();
    renderLogs();

    await pickSelectOption(user, 'logs-workload-select', 'ai-inference/qwen-8b-pd');
    await waitFor(() => {
      expect(screen.queryAllByTestId('log-viewer-row').length).toBe(1);
    });
    await user.click(screen.getByTestId('logs-live-switch'));
    await waitFor(() => {
      expect(wsInstances.length).toBe(1);
    });

    act(() => {
      // Push an off-type frame — must NOT grow the buffer.
      wsInstances[0].push({
        type: 'topology.update',
        timestamp: '2026-05-18T09:01:00Z',
        payload: { ignored: true },
      });
    });

    // Brief settle window — row count must stay at 1.
    await new Promise((res) => setTimeout(res, 50));
    expect(screen.queryAllByTestId('log-viewer-row').length).toBe(1);
  });
});
