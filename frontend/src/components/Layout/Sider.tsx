import { Menu, Layout } from 'antd';
import {
  AppstoreOutlined,
  CloudServerOutlined,
  DeploymentUnitOutlined,
  LineChartOutlined,
  FileTextOutlined,
} from '@ant-design/icons';
import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useAppStore } from '@/store';

const { Sider: AntSider } = Layout;

/**
 * Left navigation. 5 pages (frontend/CLAUDE.md §1). Selection follows the URL
 * so deep links light up the right menu item.
 */
export function Sider() {
  const navigate = useNavigate();
  const location = useLocation();
  const collapsed = useAppStore((s) => s.siderCollapsed);
  const { t } = useTranslation();

  const selectedKey = location.pathname.split('/')[1] || 'overview';

  const items = [
    { key: 'overview', icon: <AppstoreOutlined />, label: t('menu.overview') },
    { key: 'workloads', icon: <CloudServerOutlined />, label: t('menu.workloads') },
    { key: 'deploy', icon: <DeploymentUnitOutlined />, label: t('menu.deploy') },
    { key: 'metrics', icon: <LineChartOutlined />, label: t('menu.metrics') },
    { key: 'logs', icon: <FileTextOutlined />, label: t('menu.logs') },
  ];

  return (
    <AntSider collapsible collapsed={collapsed} trigger={null} width={220}>
      <Menu
        theme="dark"
        mode="inline"
        selectedKeys={[selectedKey]}
        items={items}
        onClick={({ key }) => navigate(`/${key}`)}
      />
    </AntSider>
  );
}
