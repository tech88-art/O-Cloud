import { useMemo } from 'react';
import { Cascader, Select, Space } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import type { components } from '@/services/types';
import type { DashboardKey } from './DashboardTabs';
import styles from './styles.module.css';

/**
 * Per-tab variable selectors for the Metrics page.
 *
 * Layout decisions:
 *   - `cluster-overview` has no variables → nothing renders (callers should
 *     still mount the component; it returns `null`).
 *   - `node-detail` → AntD `<Select>` for node name. Variable: `node`.
 *   - `npu-detail` → AntD `<Cascader>` for node → NPU → slice. Variables:
 *     `node`, `npu`, optionally `slice` once the third level is picked.
 *   - `workload-*` → AntD `<Select>` for `namespace/name`. Variable:
 *     `workload`.
 *
 * The chosen values are reported back via `onChange` so the parent can
 * recompute the `variables` prop passed to `<GrafanaPanel>`. Disable rules
 * matched to the T209 dashboard template variables in
 * `deploy/dev/grafana/dashboards/<key>.json` (see §templating list).
 *
 * Per frontend/CLAUDE.md §4.2 — API calls go through react-query. Because
 * Allowed Paths for this task scope writes to `pages/Metrics/**` only, the
 * hooks live here rather than in `services/*` (P1-T-208 spec).
 */

export type NPU = components['schemas']['NPU'];
export type Workload = components['schemas']['Workload'];

interface NodeListItem {
  name: string;
}

function useNodeList() {
  return useQuery({
    queryKey: ['metrics-page', 'nodes'],
    queryFn: async (): Promise<NodeListItem[]> => {
      const { data } = await api.get<NodeListItem[]>('/api/v1/nodes');
      return data;
    },
    staleTime: 30_000,
  });
}

function useNodeNPUsForCascader(nodeName: string | null) {
  return useQuery({
    queryKey: ['metrics-page', 'node-npus', nodeName],
    queryFn: async (): Promise<NPU[]> => {
      const { data } = await api.get<NPU[]>(
        `/api/v1/nodes/${encodeURIComponent(String(nodeName))}/npus`,
      );
      return data;
    },
    enabled: Boolean(nodeName),
    staleTime: 30_000,
  });
}

function useWorkloadList() {
  return useQuery({
    queryKey: ['metrics-page', 'workloads'],
    queryFn: async (): Promise<Workload[]> => {
      const { data } = await api.get<Workload[]>('/api/v1/workloads');
      return data;
    },
    staleTime: 30_000,
  });
}

/**
 * Variable selection emitted by `<VarSelectors>`.
 *
 * Keys correspond to Grafana template variable names — they become
 * `var-<key>=<value>` URL params in the iframe source (see
 * `components/GrafanaPanel/buildPocUrl`).
 */
export interface SelectedVars {
  node?: string;
  npu?: string;
  slice?: string;
  workload?: string;
}

interface VarSelectorsProps {
  tab: DashboardKey;
  value: SelectedVars;
  onChange: (next: SelectedVars) => void;
}

export function VarSelectors({ tab, value, onChange }: VarSelectorsProps) {
  const { t } = useTranslation();

  if (tab === 'cluster-overview') {
    return null;
  }

  if (tab === 'node-detail') {
    return (
      <NodeSelect
        labelText={t('metrics.selectNode')}
        value={value.node}
        onChange={(node) => onChange({ ...value, node })}
      />
    );
  }

  if (tab === 'npu-detail') {
    return (
      <NodeNpuSliceCascader
        labelText={t('metrics.selectNPU')}
        nodeLabel={t('metrics.selectNode')}
        npuLabel={t('metrics.selectNPU')}
        sliceLabel={t('metrics.selectSlice')}
        value={value}
        onChange={(node, npu, slice) =>
          onChange({ ...value, node, npu, slice })
        }
      />
    );
  }

  // workload-business / workload-resource
  return (
    <WorkloadSelect
      labelText={t('metrics.selectWorkload')}
      value={value.workload}
      onChange={(workload) => onChange({ ...value, workload })}
    />
  );
}

interface NodeSelectProps {
  labelText: string;
  value: string | undefined;
  onChange: (next: string | undefined) => void;
}

function NodeSelect({ labelText, value, onChange }: NodeSelectProps) {
  const { data, isLoading } = useNodeList();
  const options = useMemo(
    () => (data ?? []).map((n) => ({ value: n.name, label: n.name })),
    [data],
  );
  return (
    <Space size={8} data-testid="metrics-vars-node">
      <span className={styles.selectorLabel}>{labelText}</span>
      <Select
        data-testid="metrics-select-node"
        style={{ minWidth: 220 }}
        loading={isLoading}
        showSearch
        allowClear
        options={options}
        value={value}
        onChange={(v) => onChange(v ?? undefined)}
        placeholder={labelText}
      />
    </Space>
  );
}

interface NodeNpuSliceCascaderProps {
  labelText: string;
  nodeLabel: string;
  npuLabel: string;
  sliceLabel: string;
  value: SelectedVars;
  onChange: (
    node: string | undefined,
    npu: string | undefined,
    slice: string | undefined,
  ) => void;
}

interface CascadeNode {
  value: string;
  label: string;
  children?: CascadeNode[];
  isLeaf?: boolean;
}

function NodeNpuSliceCascader({
  labelText,
  nodeLabel,
  npuLabel,
  sliceLabel,
  value,
  onChange,
}: NodeNpuSliceCascaderProps) {
  const nodesQuery = useNodeList();
  const npusQuery = useNodeNPUsForCascader(value.node ?? null);

  const options = useMemo<CascadeNode[]>(() => {
    const nodes = nodesQuery.data ?? [];
    return nodes.map((n) => {
      const node: CascadeNode = {
        value: n.name,
        label: n.name,
      };
      if (n.name === value.node && npusQuery.data) {
        node.children = npusQuery.data.map((npu) => {
          const npuOpt: CascadeNode = { value: npu.id, label: npu.id };
          if (npu.slices && npu.slices.length > 0) {
            npuOpt.children = npu.slices.map((s) => ({
              value: s.id,
              label: s.id,
              isLeaf: true,
            }));
          }
          return npuOpt;
        });
      }
      return node;
    });
  }, [nodesQuery.data, npusQuery.data, value.node]);

  const cascaderValue = useMemo(() => {
    const out: string[] = [];
    if (value.node) out.push(value.node);
    if (value.npu) out.push(value.npu);
    if (value.slice) out.push(value.slice);
    return out;
  }, [value]);

  return (
    <Space size={8} wrap data-testid="metrics-vars-cascader">
      <span className={styles.selectorLabel}>{labelText}</span>
      <Cascader
        data-testid="metrics-cascader-node-npu-slice"
        style={{ minWidth: 360 }}
        options={options}
        value={cascaderValue}
        loadData={(selectedOptions) => {
          // For deeper levels: when the user opens the node level the
          // companion query (`useNodeNPUsForCascader`) will refresh based
          // on `value.node`. We surface the partial selection here so that
          // the parent reacts immediately to mid-cascade picks.
          if (selectedOptions.length === 1) {
            const node = selectedOptions[0]!.value as string;
            onChange(node, undefined, undefined);
          }
        }}
        changeOnSelect
        onChange={(path) => {
          const node = (path?.[0] as string | undefined) ?? undefined;
          const npu = (path?.[1] as string | undefined) ?? undefined;
          const slice = (path?.[2] as string | undefined) ?? undefined;
          onChange(node, npu, slice);
        }}
        placeholder={`${nodeLabel} / ${npuLabel} / ${sliceLabel}`}
        expandTrigger="hover"
        allowClear
      />
    </Space>
  );
}

interface WorkloadSelectProps {
  labelText: string;
  value: string | undefined;
  onChange: (next: string | undefined) => void;
}

function WorkloadSelect({ labelText, value, onChange }: WorkloadSelectProps) {
  const { data, isLoading } = useWorkloadList();
  const options = useMemo(
    () =>
      (data ?? []).map((w) => {
        const v = `${w.namespace}/${w.name}`;
        return { value: v, label: v };
      }),
    [data],
  );
  return (
    <Space size={8} data-testid="metrics-vars-workload">
      <span className={styles.selectorLabel}>{labelText}</span>
      <Select
        data-testid="metrics-select-workload"
        style={{ minWidth: 260 }}
        loading={isLoading}
        showSearch
        allowClear
        options={options}
        value={value}
        onChange={(v) => onChange(v ?? undefined)}
        placeholder={labelText}
      />
    </Space>
  );
}

export default VarSelectors;
