import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';
import { ConfigProvider } from 'antd';
import { AppLayout } from '@/components/Layout';
import OverviewPage from '@/pages/Overview';
import i18n from '@/i18n';

/**
 * Smoke test: AppLayout renders the app title, menu, and the nested
 * Overview page when navigated to /overview.
 */
describe('AppLayout', () => {
  it('renders header title, sider menu, and the overview placeholder', () => {
    render(
      <ConfigProvider>
        <I18nextProvider i18n={i18n}>
          <MemoryRouter initialEntries={['/overview']}>
            <Routes>
              <Route path="/" element={<AppLayout />}>
                <Route path="overview" element={<OverviewPage />} />
              </Route>
            </Routes>
          </MemoryRouter>
        </I18nextProvider>
      </ConfigProvider>,
    );

    // Header title (zh-CN default)
    expect(screen.getByText('O-Cloud 边缘云平台')).toBeInTheDocument();

    // Sider menu items — "概览" appears twice (menu + page heading), use getAllByText
    expect(screen.getAllByText('概览').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('工作负载')).toBeInTheDocument();
    expect(screen.getByText('应用部署')).toBeInTheDocument();
    expect(screen.getByText('性能指标')).toBeInTheDocument();
    expect(screen.getByText('日志')).toBeInTheDocument();

    // Page heading rendered as <h2>
    expect(screen.getByRole('heading', { level: 2, name: '概览' })).toBeInTheDocument();

    // Overview placeholder rendered
    expect(screen.getByText('概览页面 — W2 即将上线')).toBeInTheDocument();
  });
});
