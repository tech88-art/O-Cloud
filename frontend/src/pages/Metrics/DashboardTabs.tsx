/* eslint-disable react-refresh/only-export-components */
import { Tabs } from 'antd';
import { useTranslation } from 'react-i18next';

/**
 * Dashboard keys used by this page.
 *
 * Hyphenated form matches the T209 dashboard UIDs at
 * `deploy/dev/grafana/dashboards/<key>.json` (cluster-overview, node-detail,
 * npu-detail, workload-business, workload-resource).
 *
 * GrafanaPanel (T010) accepts the underscored form for its `dashboard` prop;
 * conversion happens at the call site in `<MetricsPage>`.
 *
 * The eslint `react-refresh/only-export-components` warning is suppressed
 * here because the two exported helpers (`DASHBOARD_KEYS`, `toGrafanaPanelKey`)
 * are typed constants and a pure mapping function — they have no runtime
 * state and are co-located with the tabs component that uses them. Same
 * pattern as `components/GrafanaPanel/index.tsx` which also co-locates a
 * `buildPocUrl` helper next to its component (see file header there).
 */
export type DashboardKey =
  | 'cluster-overview'
  | 'node-detail'
  | 'npu-detail'
  | 'workload-business'
  | 'workload-resource';

export const DASHBOARD_KEYS: readonly DashboardKey[] = [
  'cluster-overview',
  'node-detail',
  'npu-detail',
  'workload-business',
  'workload-resource',
] as const;

/**
 * Map the hyphenated dashboard key (UI / URL friendly) to the underscored
 * form accepted by `<GrafanaPanel dashboard>` (POC_DASHBOARD_MAP keys, see
 * `components/GrafanaPanel/index.tsx`).
 */
export function toGrafanaPanelKey(key: DashboardKey): string {
  return key.replaceAll('-', '_');
}

interface DashboardTabsProps {
  active: DashboardKey;
  onChange: (k: DashboardKey) => void;
}

/**
 * Five-tab dashboard selector. The label set comes from i18n (zh / en
 * parity required); the keys themselves are stable string literals so
 * deep-linking (`?tab=npu-detail`) remains predictable.
 */
export function DashboardTabs({ active, onChange }: DashboardTabsProps) {
  const { t } = useTranslation();
  return (
    <Tabs
      data-testid="metrics-dashboard-tabs"
      activeKey={active}
      onChange={(k) => onChange(k as DashboardKey)}
      items={[
        {
          key: 'cluster-overview',
          label: (
            <span data-testid="metrics-tab-cluster-overview">
              {t('metrics.clusterOverview')}
            </span>
          ),
        },
        {
          key: 'node-detail',
          label: (
            <span data-testid="metrics-tab-node-detail">
              {t('metrics.nodeDetail')}
            </span>
          ),
        },
        {
          key: 'npu-detail',
          label: (
            <span data-testid="metrics-tab-npu-detail">
              {t('metrics.npuDetail')}
            </span>
          ),
        },
        {
          key: 'workload-business',
          label: (
            <span data-testid="metrics-tab-workload-business">
              {t('metrics.workloadBusiness')}
            </span>
          ),
        },
        {
          key: 'workload-resource',
          label: (
            <span data-testid="metrics-tab-workload-resource">
              {t('metrics.workloadResource')}
            </span>
          ),
        },
      ]}
    />
  );
}

export default DashboardTabs;
