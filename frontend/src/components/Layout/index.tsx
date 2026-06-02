import { Layout as AntLayout, Button, Dropdown, Space, Tag, Tooltip, Typography } from 'antd';
import type { MenuProps } from 'antd';
import { GlobalOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { Outlet } from 'react-router-dom';
import { PresetBar } from '@/components/PresetBar';
import { useAppStore } from '@/store';
import { SUPPORTED_LOCALES, persistLocale, type Locale } from '@/i18n';

const { Header, Content } = AntLayout;
const { Title, Text } = Typography;

/**
 * Obsidian-style sidebar-toggle icons (lucide `panel-left` / `panel-right`):
 * a rounded rect with a divider near the left / right edge. Rendered at the
 * two ENDS of the header so each toggle sits on the side of the pane it
 * controls (P12 polish — replaces the two chevrons bunched at top-left).
 */
function PanelLeftIcon() {
  return (
    <svg
      width="1em"
      height="1em"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <rect x="3" y="3" width="18" height="18" rx="2" />
      <path d="M9 3v18" />
    </svg>
  );
}

function PanelRightIcon() {
  return (
    <svg
      width="1em"
      height="1em"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <rect x="3" y="3" width="18" height="18" rx="2" />
      <path d="M15 3v18" />
    </svg>
  );
}

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
 * One-page workspace shell (P12-T-201 / ADR-0022). The 5-route AntSider nav
 * is gone — the app is a single workspace, so the Header no longer drives
 * navigation. Instead it carries:
 *   - app brand (title + version)
 *   - two pane-toggle buttons that show/hide the workspace's left resource
 *     tree and right detail pane (state in the app store, persisted)
 *   - a language dropdown (zh-CN/en-US, flag-prefixed, persisted)
 * Content hosts the route outlet (the workspace). Preset bar (T204) mounts
 * between Header and Content in a later task.
 *
 * Locale change flow:
 *   user picks item → setLocale(store) → i18n.changeLanguage → persistLocale(storage)
 * Reload then rehydrates via i18n/index.ts loadPersistedLocale().
 */
export function AppLayout() {
  const { t, i18n } = useTranslation();
  const leftPaneHidden = useAppStore((s) => s.leftPaneHidden);
  const rightPaneHidden = useAppStore((s) => s.rightPaneHidden);
  const toggleLeftPane = useAppStore((s) => s.toggleLeftPane);
  const toggleRightPane = useAppStore((s) => s.toggleRightPane);
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
    // Fixed 100vh + overflow:hidden makes this a proper full-height flex
    // column: Header + PresetBar take their natural height, Content flex-fills
    // the rest, and the workspace (Splitter) sizes off that — no magic-number
    // height calc, and the preset bar's height is accounted for automatically.
    <AntLayout style={{ height: '100vh', overflow: 'hidden' }}>
      <Header
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          paddingInline: 16,
        }}
      >
        {/* Left end: left-pane toggle (Obsidian-style, on the side it controls) + brand. */}
        <Space size="middle">
          <Tooltip title={t('header.toggleLeftPane')}>
            <Button
              type="text"
              aria-label={t('header.toggleLeftPane')}
              aria-pressed={!leftPaneHidden}
              icon={<PanelLeftIcon />}
              onClick={toggleLeftPane}
              data-testid="toggle-left-pane"
              style={{ color: leftPaneHidden ? '#8c8c8c' : '#fff' }}
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
        {/* Right end: language dropdown + right-pane toggle (far right, on its side). */}
        <Space size="middle">
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
          <Tooltip title={t('header.toggleRightPane')}>
            <Button
              type="text"
              aria-label={t('header.toggleRightPane')}
              aria-pressed={!rightPaneHidden}
              icon={<PanelRightIcon />}
              onClick={toggleRightPane}
              data-testid="toggle-right-pane"
              style={{ color: rightPaneHidden ? '#8c8c8c' : '#fff' }}
            />
          </Tooltip>
        </Space>
      </Header>
      <PresetBar />
      <Content
        style={{
          display: 'flex',
          flexDirection: 'column',
          margin: 12,
          minHeight: 0,
          overflow: 'hidden',
        }}
      >
        <Outlet />
      </Content>
    </AntLayout>
  );
}
