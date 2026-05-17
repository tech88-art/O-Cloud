import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClientProvider } from '@tanstack/react-query';
import { ConfigProvider } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import { I18nextProvider } from 'react-i18next';
import App from './App';
import i18n from './i18n';
import { queryClient } from './services/api';
import { theme } from './styles/theme';
import { loadRuntimeConfig } from './config/runtime';
import './styles/global.css';

/**
 * Boot order:
 *  1. Load /config.json (so api.ts has a baseURL before any query fires).
 *  2. Mount providers: ConfigProvider (AntD theme/locale) → QueryClientProvider
 *     → I18nextProvider → <App />.
 */
async function bootstrap() {
  await loadRuntimeConfig();

  const rootEl = document.getElementById('root');
  if (!rootEl) {
    throw new Error('root element not found');
  }

  createRoot(rootEl).render(
    <StrictMode>
      <ConfigProvider theme={theme} locale={zhCN}>
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={i18n}>
            <App />
          </I18nextProvider>
        </QueryClientProvider>
      </ConfigProvider>
    </StrictMode>,
  );
}

void bootstrap();
