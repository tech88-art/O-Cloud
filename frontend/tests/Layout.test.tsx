import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ConfigProvider } from 'antd';
import { AppLayout } from '@/components/Layout';
import i18n from '@/i18n';

/**
 * Smoke test: AppLayout renders the app title, menu, and the nested
 * Overview page when navigated to /overview.
 *
 * P1-T-108a note: the production OverviewPage now fires react-query
 * requests on mount, which would either need an axios mock or pull in
 * runtime config. The Layout test is about the Layout shell, not the
 * Overview page contents — we mock the page module to a stub component
 * so Layout-level assertions (header title, menu items) stay isolated.
 * Overview's own behaviour is covered by `tests/Overview.test.tsx`.
 */
vi.mock('@/pages/Overview', () => ({
  default: () => <h2>概览</h2>,
}));

import OverviewPage from '@/pages/Overview';

describe('AppLayout', () => {
  it('renders header title and sider menu around the overview route', () => {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    });
    render(
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

    // Header title (zh-CN default)
    expect(screen.getByText('O-Cloud 边缘云平台')).toBeInTheDocument();

    // Sider menu items — "概览" appears in both menu and page stub, use getAllByText
    expect(screen.getAllByText('概览').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('工作负载')).toBeInTheDocument();
    expect(screen.getByText('应用部署')).toBeInTheDocument();
    expect(screen.getByText('性能指标')).toBeInTheDocument();
    expect(screen.getByText('日志')).toBeInTheDocument();

    // Page heading rendered as <h2> (from the mocked Overview stub)
    expect(screen.getByRole('heading', { level: 2, name: '概览' })).toBeInTheDocument();
  });
});
