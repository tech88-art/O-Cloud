import { Layout as AntLayout, Button, Select, Space, Typography } from 'antd';
import { MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { Outlet } from 'react-router-dom';
import { Sider } from './Sider';
import { useAppStore } from '@/store';
import { SUPPORTED_LOCALES, type Locale } from '@/i18n';

const { Header, Content } = AntLayout;
const { Title } = Typography;

/**
 * AntD Sider + Header + Content shell. The Header carries the app title,
 * collapse toggle, and language switch; the Content hosts the route outlet.
 */
export function AppLayout() {
  const { t, i18n } = useTranslation();
  const collapsed = useAppStore((s) => s.siderCollapsed);
  const toggleSider = useAppStore((s) => s.toggleSider);
  const setLocale = useAppStore((s) => s.setLocale);

  const onLocaleChange = (next: Locale) => {
    setLocale(next);
    void i18n.changeLanguage(next);
  };

  return (
    <AntLayout style={{ minHeight: '100vh' }}>
      <Sider />
      <AntLayout>
        <Header
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            paddingInline: 16,
          }}
        >
          <Space>
            <Button
              type="text"
              icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
              onClick={toggleSider}
              style={{ color: '#fff' }}
            />
            <Title level={4} style={{ color: '#fff', margin: 0 }}>
              {t('app.title')}
            </Title>
          </Space>
          <Select<Locale>
            value={(i18n.language as Locale) || 'zh-CN'}
            onChange={onLocaleChange}
            options={SUPPORTED_LOCALES.map((l) => ({ value: l, label: l }))}
            style={{ width: 110 }}
          />
        </Header>
        <Content style={{ margin: 16, padding: 24, background: '#fff', borderRadius: 4 }}>
          <Outlet />
        </Content>
      </AntLayout>
    </AntLayout>
  );
}
