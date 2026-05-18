import { useMemo, useState } from 'react';
import { Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { GrafanaPanel } from '@/components/GrafanaPanel';
import {
  DashboardTabs,
  toGrafanaPanelKey,
  type DashboardKey,
} from './DashboardTabs';
import { VarSelectors, type SelectedVars } from './VarSelectors';
import styles from './styles.module.css';

const { Title } = Typography;

/**
 * Metrics page (P1-T-208).
 *
 * Five tabs, each rendering the same `<GrafanaPanel>` with a different
 * dashboard key and the matching variable set. Tab switches and variable
 * picks both recompose the iframe `src` via GrafanaPanel's internal
 * effect (it diffs `dashboard` + serialized `variables` and re-runs the
 * URL builder; see `components/GrafanaPanel/buildPocUrl`).
 *
 * Variable contract for each tab maps to the `templating.list` block in
 * the T209 dashboard JSON (`deploy/dev/grafana/dashboards/<key>.json`):
 *
 *   cluster-overview   → none
 *   node-detail        → var-node
 *   npu-detail         → var-node, var-npu, var-slice (slice is forward-
 *                        looking; T209 currently exposes node + npu only,
 *                        but the URL includes var-slice when set so the
 *                        slice panel can be wired up without a frontend
 *                        change later)
 *   workload-business  → var-workload
 *   workload-resource  → var-workload
 *
 * Per frontend/CLAUDE.md §1 + §4.4 — text goes through i18n; no hardcoded
 * Chinese / English strings in JSX.
 */
export default function MetricsPage() {
  const { t } = useTranslation();
  const [tab, setTab] = useState<DashboardKey>('cluster-overview');
  const [vars, setVars] = useState<SelectedVars>({});

  /**
   * Variables actually sent to GrafanaPanel for the current tab. We do
   * NOT carry stale picks across unrelated tabs (e.g. a node picked under
   * node-detail must not leak into workload-business as `var-node=...`).
   * `node` does carry into `npu-detail` because the NPU cascader builds on
   * it.
   */
  const panelVariables = useMemo<Record<string, string>>(() => {
    const out: Record<string, string> = {};
    if (tab === 'node-detail' && vars.node) {
      out.node = vars.node;
    }
    if (tab === 'npu-detail') {
      if (vars.node) out.node = vars.node;
      if (vars.npu) out.npu = vars.npu;
      if (vars.slice) out.slice = vars.slice;
    }
    if (
      (tab === 'workload-business' || tab === 'workload-resource') &&
      vars.workload
    ) {
      out.workload = vars.workload;
    }
    return out;
  }, [tab, vars]);

  return (
    <div className={styles.page} data-testid="metrics-page">
      <div className={styles.toolbar} data-testid="metrics-toolbar">
        <div className={styles.tabsRow}>
          <Title level={4} style={{ margin: 0 }}>
            {t('metrics.title')}
          </Title>
        </div>
        <DashboardTabs
          active={tab}
          onChange={(next) => {
            setTab(next);
          }}
        />
        <div className={styles.selectorsRow} data-testid="metrics-selectors-row">
          <VarSelectors
            tab={tab}
            value={vars}
            onChange={(next) => setVars(next)}
          />
        </div>
      </div>
      <div className={styles.panel} data-testid="metrics-panel">
        <GrafanaPanel
          dashboard={toGrafanaPanelKey(tab)}
          variables={panelVariables}
          height={600}
        />
      </div>
    </div>
  );
}
