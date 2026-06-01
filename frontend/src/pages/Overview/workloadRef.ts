import type { TopologyNode } from '@/services/cluster';

/**
 * Helpers for resolving the namespace/name of workload + pod topology nodes
 * (P12-T-203). The backend emits these nodes with id `<type>/<namespace>/<name>`
 * (e.g. `workload/ai-inference/qwen-8b`, `pod/ai-inference/qwen-8b-prefill-0`)
 * and an `attributes` bag (`namespace`, and for pods `workload` = parent
 * workload name). The right-panel metrics + logs sections need ns/name to
 * build Grafana vars and the logs endpoint path.
 *
 * Kept in a sibling `.ts` module (not the component) so MetricsSection,
 * LogsSection and DetailPanel share ONE parser — single source of truth.
 */
export interface WorkloadRef {
  namespace: string;
  /** workload name, or pod name (depending on the node type). */
  name: string;
}

function attrString(node: TopologyNode, key: string): string | undefined {
  const v = (node.attributes ?? {})[key];
  return typeof v === 'string' && v.length > 0 ? v : undefined;
}

/**
 * Parse a workload/pod node's `{ namespace, name }`. `name` is the node's own
 * name (the workload name for a workload node, the pod name for a pod node).
 * Prefers the explicit `namespace` attribute, falling back to the id, then
 * the `<namespace>/<name>` label. Returns null when nothing parses.
 */
export function parseWorkloadRef(node: TopologyNode): WorkloadRef | null {
  const idParts = node.id.split('/');
  if (idParts.length >= 3) {
    const namespace = attrString(node, 'namespace') ?? idParts[1];
    const name = idParts.slice(2).join('/');
    if (namespace && name) return { namespace, name };
  }
  const labelParts = node.label.split('/');
  if (labelParts.length >= 2) {
    const namespace = attrString(node, 'namespace') ?? labelParts[0];
    return { namespace, name: labelParts.slice(1).join('/') };
  }
  return null;
}

/**
 * Resolve the workload whose log stream covers this node. Logs are served
 * per-workload (`/api/v1/workloads/:ns/:name/logs`), so a pod node resolves
 * to its PARENT workload (the `workload` attribute), while a workload node
 * resolves to itself. Returns null when ns/name can't be determined.
 */
export function logsWorkloadRef(node: TopologyNode): WorkloadRef | null {
  if (node.type === 'pod') {
    const ref = parseWorkloadRef(node);
    const parent = attrString(node, 'workload');
    const namespace = ref?.namespace ?? attrString(node, 'namespace');
    if (namespace && parent) return { namespace, name: parent };
    return null;
  }
  if (node.type === 'workload') {
    return parseWorkloadRef(node);
  }
  return null;
}
