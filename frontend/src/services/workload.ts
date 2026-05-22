import { useQuery } from '@tanstack/react-query';
import { api } from './api';
import type { components } from './types';

/**
 * Workload list + detail react-query hooks (P1-T-206).
 *
 * Mirrors the pattern used by `services/cluster.ts` and `services/node.ts`:
 *   - one hook per endpoint
 *   - types pulled from auto-generated `components['schemas'][...]`
 *   - `staleTime` matches sibling hooks (30s) so re-renders during filter
 *     edits don't hammer the backend
 *   - `enabled` gate on the detail hook so it doesn't fire with empty path
 *     segments while the Drawer is closed
 *
 * Filter contract:
 * `?namespace=` / `?type=` / `?status=` are server-side filters (backend
 * P1-T-201). We forward exactly the values the OpenAPI enum admits, and pass
 * `undefined` (axios drops it) when the caller hasn't picked one.
 */
export type Workload = components['schemas']['Workload'];
export type WorkloadDetail = components['schemas']['WorkloadDetail'];
export type Pod = components['schemas']['Pod'];
// P6-T-102 — SliceBinding generated from the new SliceBinding component.
export type SliceBinding = components['schemas']['SliceBinding'];

export type WorkloadType = NonNullable<Workload['type']>;
export type WorkloadStatus = NonNullable<Workload['status']>;

export interface WorkloadFilter {
  namespace?: string;
  type?: WorkloadType;
  status?: WorkloadStatus;
  /**
   * P6-T-103 opt-in: when true, the list endpoint populates
   * `Workload.sliceBindings` from NPUSliceAllocation data joined
   * against the workload's pods. Default false — keeps response
   * shape unchanged for callers that don't ask for the data. The
   * detail endpoint always populates when source has data.
   */
  includeSliceBindings?: boolean;
  /**
   * P11-T-105 opt-in: when true, list endpoint populates
   * `Workload.o2DMSExposed`. Per ADR-0013 §6 forward note.
   */
  includeO2DMSExposed?: boolean;
  /**
   * P11-T-105 opt-in: when true, list endpoint populates
   * `Workload.quotaUsage`. Per ADR-0014 §7 forward note.
   */
  includeQuotaUsage?: boolean;
  /**
   * P11-T-105 opt-in: when true, list endpoint populates
   * `Workload.scaleHistory`. Per ADR-0012 §5 forward note.
   */
  includeScaleHistory?: boolean;
}

export type QuotaUsageSummary = NonNullable<Workload['quotaUsage']>;
export type ScaleEvent = NonNullable<Workload['scaleHistory']>[number];

/**
 * `useWorkloads(filter)` — list workloads, optionally filtered server-side.
 *
 * Empty-string values are treated as "no filter" so the page can pass an
 * uncontrolled input's value directly without trimming. Anything else flows
 * through to the backend query string.
 *
 * P6-T-103: passing `includeSliceBindings: true` adds the
 * `?includeSliceBindings=true` query param; backend response then carries
 * the `sliceBindings[]` field per workload (per-pod NPU device + AI core
 * allocations).
 */
export function useWorkloads(filter: WorkloadFilter = {}) {
  const params: Record<string, string | boolean> = {};
  if (filter.namespace && filter.namespace.trim().length > 0) {
    params.namespace = filter.namespace.trim();
  }
  if (filter.type) {
    params.type = filter.type;
  }
  if (filter.status) {
    params.status = filter.status;
  }
  if (filter.includeSliceBindings) {
    params.includeSliceBindings = true;
  }
  if (filter.includeO2DMSExposed) {
    params.includeO2DMSExposed = true;
  }
  if (filter.includeQuotaUsage) {
    params.includeQuotaUsage = true;
  }
  if (filter.includeScaleHistory) {
    params.includeScaleHistory = true;
  }
  return useQuery({
    queryKey: ['workloads', params],
    queryFn: async (): Promise<Workload[]> => {
      const { data } = await api.get<Workload[]>('/api/v1/workloads', {
        params,
      });
      return data;
    },
    staleTime: 30_000,
  });
}

/**
 * `useWorkloadDetail(namespace, name)` — fetch one workload's detail
 * (pods + relations + spec). Disabled while either segment is missing.
 */
export function useWorkloadDetail(
  namespace: string | null | undefined,
  name: string | null | undefined,
) {
  return useQuery({
    queryKey: ['workload-detail', namespace, name],
    queryFn: async (): Promise<WorkloadDetail> => {
      const { data } = await api.get<WorkloadDetail>(
        `/api/v1/workloads/${encodeURIComponent(String(namespace))}/${encodeURIComponent(String(name))}`,
      );
      return data;
    },
    enabled: Boolean(namespace) && Boolean(name),
    staleTime: 30_000,
  });
}
