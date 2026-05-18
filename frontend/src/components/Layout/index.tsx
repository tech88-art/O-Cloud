import { Layout as AntLayout, Button, Dropdown, Space, Tag, Tooltip, Typography } from 'antd';
import type { MenuProps } from 'antd';
import {
  GlobalOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { Outlet } from 'react-router-dom';
import { Sider } from './Sider';
import { useAppStore } from '@/store';
import { SUPPORTED_LOCALES, persistLocale, type Locale } from '@/i18n';

const { Header, Content } = AntLayout;
const { Title, Text } = Typography;

/**
 * Vite injects the package.json version via __APP_VERSION__ only if the
 * config defines it; we don't depend on that. Fall back to a literal so the
 * brand area always shows something. Updating this in lock-step with
 * package.json is acceptable for a demo prototype.
 */
const APP_VERSION = '0.1.0';

/** Flag emoji per locale — keeps the dropdown visually distinct without an extra icon dep. */
const LOCALE_META: Record<Locale, { flag: string; labelKey: string }> = {
  'zh-CN': { flag: '🇨🇳', labelKey: 'header.languageOption.zhCN' },
  'en-US': { flag: '🇺🇸', labelKey: 'header.languageOption.enUS' },
};

/**
 * AntD Sider + Header + Content shell. The Header carries the app brand
 * (title + version), sider collapse toggle, and a language dropdown
 * (zh-CN/en-US, flag-prefixed, persisted to localStorage). Content hosts
 * the route outlet.
 *
 * Locale change flow:
 *   user picks item → setLocale(store) → i18n.changeLanguage → persistLocale(storage)
 * Reload then rehydrates via i18n/index.ts loadPersistedLocale().
 */
export function AppLayout() {
  const { t, i18n } = useTranslation();
  const collapsed = useAppStore((s) => s.siderCollapsed);
  const toggleSider = useAppStore((s) => s.toggleSider);
  const setLocale = useAppStore((s) => s.setLocale);

  const currentLocale: Locale = (i18n.language as Locale) in LOCALE_META
    ? (i18n.language as Locale)
    : 'zh-CN';

  const handleLocaleSelect = (next: Locale) => {
    setLocale(next);
    void i18n.changeLanguage(next);
    persistLocale(next);
  };

  const localeMenuItems: MenuProps['items'] = SUPPORTED_LOCALES.map((l) => ({
    key: l,
    label: (
      <Space>
        <span aria-hidden="true">{LOCALE_META[l].flag}</span>
        <span>{t(LOCALE_META[l].labelKey)}</span>
      </Space>
    ),
  }));

  const onLocaleMenuClick: MenuProps['onClick'] = (info) => {
    if ((SUPPORTED_LOCALES as readonly string[]).includes(info.key)) {
      handleLocaleSelect(info.key as Locale);
    }
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
          <Space size="middle">
            <Tooltip title={t('header.toggleSider')}>
              <Button
                type="text"
                aria-label={t('header.toggleSider')}
                icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
                onClick={toggleSider}
                style={{ color: '#fff' }}
              />
            </Tooltip>
            <Space size={8} align="baseline">
              <Title level={4} style={{ color: '#fff', margin: 0 }}>
                {t('app.title')}
              </Title>
              <Tag color="blue" style={{ marginInlineEnd: 0 }} aria-label={t('app.version')}>
                v{APP_VERSION}
              </Tag>
            </Space>
          </Space>
          <Dropdown
            menu={{
              items: localeMenuItems,
              onClick: onLocaleMenuClick,
              selectedKeys: [currentLocale],
            }}
            trigger={['click']}
            placement="bottomRight"
          >
            <Button
              type="text"
              aria-label={t('header.language')}
              icon={<GlobalOutlined />}
              style={{ color: '#fff' }}
            >
              <Space size={4}>
                <span aria-hidden="true">{LOCALE_META[currentLocale].flag}</span>
                <Text style={{ color: '#fff' }}>{t(LOCALE_META[currentLocale].labelKey)}</Text>
              </Space>
            </Button>
          </Dropdown>
        </Header>
        <Content style={{ margin: 16, padding: 24, background: '#fff', borderRadius: 4 }}>
          <Outlet />
        </Content>
      </AntLayout>
    </AntLayout>
  );
}
