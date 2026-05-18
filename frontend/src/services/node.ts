import { useQuery } from '@tanstack/react-query';
import { api } from './api';
import type { components } from './types';

/**
 * Node detail + NPU list react-query hooks (P1-T-108b).
 *
 * Mirrors the pattern in `services/cluster.ts`:
 *   - one hook per endpoint
 *   - typed off the auto-generated `components['schemas'][...]` types
 *   - `enabled` gate so that the query doesn't fire while the name is null
 *   - `staleTime` matched to the cluster topology (30s) so the panel
 *     doesn't refetch on every selection blink
 *
 * The Overview page's `<DetailPanel>` consumes these — they are not used
 * anywhere else yet but live in `services/` because react-query hooks
 * always do (frontend/CLAUDE.md §4.2).
 */
export type NodeDetail = components['schemas']['NodeDetail'];
export type NPU = components['schemas']['NPU'];

/**
 * Fetch one node's detail (`GET /api/v1/nodes/:name`).
 *
 * Disabled while `name` is null / empty so the request URL never collapses
 * to `/api/v1/nodes/`.
 */
export function useNode(name: string | null | undefined) {
  return useQuery({
    queryKey: ['node', name],
    queryFn: async (): Promise<NodeDetail> => {
      const { data } = await api.get<NodeDetail>(
        `/api/v1/nodes/${encodeURIComponent(String(name))}`,
      );
      return data;
    },
    enabled: Boolean(name),
    staleTime: 30_000,
  });
}

/**
 * Fetch the list of NPUs on one node (`GET /api/v1/nodes/:name/npus`).
 *
 * Returned as an array per the OpenAPI schema. Slices are nested per-NPU
 * (see `NPU.slices` in `services/types.ts`).
 */
export function useNodeNPUs(name: string | null | undefined) {
  return useQuery({
    queryKey: ['node-npus', name],
    queryFn: async (): Promise<NPU[]> => {
      const { data } = await api.get<NPU[]>(
        `/api/v1/nodes/${encodeURIComponent(String(name))}/npus`,
      );
      return data;
    },
    enabled: Boolean(name),
    staleTime: 30_000,
  });
}
