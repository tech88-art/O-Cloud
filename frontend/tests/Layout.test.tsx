import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ConfigProvider } from 'antd';
import { AppLayout } from '@/components/Layout';
import { useAppStore } from '@/store';
import i18n from '@/i18n';

/**
 * P12-T-201 / ADR-0022: the AppLayout is now the one-page workspace shell.
 * The 5-item AntSider nav menu is gone; the Header carries the brand plus
 * two pane-toggle buttons (left resource tree / right detail pane) wired to
 * the app store. We mock the Overview page to a stub so these Layout-shell
 * assertions stay isolated from Overview's react-query data fetching
 * (Overview's own behaviour is covered by `tests/Overview.test.tsx`).
 */
vi.mock('@/pages/Overview', () => ({
  default: () => <h2>概览</h2>,
}));

import OverviewPage from '@/pages/Overview';

function renderLayout() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <ConfigProvider>
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={qc}>
          <MemoryRouter initialEntries={['/overview']}>
            <Routes>
              <Route path="/" element={<AppLayout />}>
                <Route path="overview" element={<OverviewPage />} />
              </Route>
            </Routes>
          </MemoryRouter>
        </QueryClientProvider>
      </I18nextProvider>
    </ConfigProvider>,
  );
}

describe('AppLayout (one-page workspace shell)', () => {
  beforeEach(() => {
    localStorage.clear();
    useAppStore.setState({ leftPaneHidden: false, rightPaneHidden: false });
  });

  it('renders the brand + pane-toggle controls and no nav menu', () => {
    renderLayout();

    // Brand (zh-CN default).
    expect(screen.getByText('O-Cloud 边缘云平台')).toBeInTheDocument();

    // Two pane toggles instead of nav items.
    expect(screen.getByTestId('toggle-left-pane')).toBeInTheDocument();
    expect(screen.getByTestId('toggle-right-pane')).toBeInTheDocument();

    // The old nav menu labels must be gone (workloads/deploy/metrics/logs
    // were menu-only strings; with the menu removed they no longer render).
    expect(screen.queryByText('工作负载')).not.toBeInTheDocument();
    expect(screen.queryByText('应用部署')).not.toBeInTheDocument();
    expect(screen.queryByText('性能指标')).not.toBeInTheDocument();

    // The workspace (mocked Overview stub) renders inside the Content outlet.
    expect(screen.getByRole('heading', { level: 2, name: '概览' })).toBeInTheDocument();
  });

  it('toggles left pane visibility through the app store', async () => {
    const user = userEvent.setup();
    renderLayout();

    expect(useAppStore.getState().leftPaneHidden).toBe(false);
    await user.click(screen.getByTestId('toggle-left-pane'));
    expect(useAppStore.getState().leftPaneHidden).toBe(true);
  });

  it('toggles right pane visibility through the app store', async () => {
    const user = userEvent.setup();
    renderLayout();

    expect(useAppStore.getState().rightPaneHidden).toBe(false);
    await user.click(screen.getByTestId('toggle-right-pane'));
    expect(useAppStore.getState().rightPaneHidden).toBe(true);
  });
});
