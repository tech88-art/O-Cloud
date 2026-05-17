import type { ThemeConfig } from 'antd';

/**
 * AntD theme tokens. Keep this lean — extend only when there's a clear
 * design-system reason. See frontend/CLAUDE.md §6 (style conventions).
 */
export const theme: ThemeConfig = {
  token: {
    colorPrimary: '#1677ff',
    borderRadius: 4,
    fontFamily:
      "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
  },
  components: {
    Layout: {
      headerBg: '#001529',
      siderBg: '#001529',
      bodyBg: '#f0f2f5',
    },
  },
};
