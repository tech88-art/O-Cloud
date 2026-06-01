import { Switch, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { GrafanaPanel } from '@/components/GrafanaPanel';
import type { TopologyNode } from '@/services/cluster';
import { useTopologyStore } from '@/store/topologyStore';
import { parseWorkloadRef } from './workloadRef';
import sectionStyles from './sections.module.css';

const { Text } = Typography;

interface DashboardTarget {
  dashboard: string;
  variables?: Record<string, string>;
  /** true = hardware dashboard (resource), false = business (workload/pod). */
  hardware: boolean;
}

/**
 * Resolve the Grafana dashboard + variables for a selected node
 * (ADR-0022 §2.3 · one-page-workspace DESIGN §6.2):
 *   resource (cluster/nodepool/node/npu/slice) → hardware dashboards
 *   workload → workload_business · pod → workload_resource
 * Returns null for types with no dashboard (network/switch) so the section
 * hides itself.
 */
function dashboardForNode(node: TopologyNode, clusterId: string | null): DashboardTarget | null {
  switch (node.type) {
    case 'cluster':
    case 'nodepool':
      return {
        dashboard: 'cluster_overview',
        variables: clusterId ? { cluster: clusterId } : undefined,
        hardware: true,
      };
    case 'node':
      return { dashboard: 'node_detail', variables: { node: node.label }, hardware: true };
    case 'npu':
      return { dashboard: 'npu_detail', variables: { npu: node.label }, hardware: true };
    case 'slice': {
      const parent =
        typeof node.attributes?.parentNPU === 'string'
          ? (node.attributes.parentNPU as string)
          : node.label;
      return { dashboard: 'npu_detail', variables: { npu: parent }, hardware: true };
    }
    case 'workload': {
      const ref = parseWorkloadRef(node);
      return ref
        ? {
            dashboard: 'workload_business',
            variables: { namespace: ref.namespace, workload: ref.name },
            hardware: false,
          }
        : null;
    }
    case 'pod': {
      const ref = parseWorkloadRef(node);
      return ref
        ? {
            dashboard: 'workload_resource',
            variables: { namespace: ref.namespace, pod: ref.name },
            hardware: false,
          }
        : null;
    }
    default:
      return null;
  }
}

export interface MetricsSectionProps {
  node: TopologyNode;
  clusterId: string | null;
}

/**
 * Right-panel metrics section (P12-T-203). A collapsible card whose Grafana
 * iframe dashboard is dispatched by the selected node's type. The section
 * show/hide state lives in the topology store so it persists across
 * selections (ADR-0022 §2.3 "显示开关").
 */
export function MetricsSection({ node, clusterId }: MetricsSectionProps) {
  const { t } = useTranslation();
  const open = useTopologyStore((s) => s.metricsSectionOpen);
  const setOpen = useTopologyStore((s) => s.setMetricsSectionOpen);

  const target = dashboardForNode(node, clusterId);
  if (!target) return null;

  const title = target.hardware
    ? t('overview.section.metricsHardware')
    : t('overview.section.metricsBusiness');

  return (
    <section data-testid="metrics-section" className={sectionStyles.section}>
      <header className={sectionStyles.sectionHeader}>
        <Text strong>{title}</Text>
        <Switch
          size="small"
          checked={open}
          onChange={setOpen}
          data-testid="metrics-section-toggle"
          aria-label={title}
        />
      </header>
      {open && (
        <div className={sectionStyles.sectionBody} data-testid="metrics-section-body">
          <GrafanaPanel dashboard={target.dashboard} variables={target.variables} height={300} />
        </div>
      )}
    </section>
  );
}

export default MetricsSection;
