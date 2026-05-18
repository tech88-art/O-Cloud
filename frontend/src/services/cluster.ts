import { useQuery } from '@tanstack/react-query';
import { api } from './api';
import type { components } from './types';

/**
 * Cluster + topology react-query hooks.
 *
 * Per frontend/CLAUDE.md §4.2 — all API calls go through react-query, no
 * direct axios access from components. Types are pulled from the
 * auto-generated `./types.ts` (do not hand-write API shapes).
 */
export type Cluster = components['schemas']['Cluster'];
export type Topology = components['schemas']['Topology'];
export type TopologyNode = components['schemas']['TopologyNode'];
export type TopologyEdge = components['schemas']['TopologyEdge'];

/**
 * Topology depth query parameter. Matches the OpenAPI enum at
 * `/api/v1/clusters/{clusterId}/topology` (depth):
 *
 *   - `node`  — stop at node level (no NPU / slice)
 *   - `npu`   — expand to NPU but not slices
 *   - `slice` — full depth (default; matches architecture.md §8.2 expectation)
 */
export type TopologyDepth = 'node' | 'npu' | 'slice';

/**
 * `useClusters` — list available clusters. The left-side tree on the
 * Overview page picks the cluster to render; demo backend currently
 * returns a single fixture cluster but the API allows N.
 */
export function useClusters() {
  return useQuery({
    queryKey: ['clusters'],
    queryFn: async (): Promise<Cluster[]> => {
      const { data } = await api.get<Cluster[]>('/api/v1/clusters');
      return data;
    },
    staleTime: 30_000,
  });
}

/**
 * `useClusterTopology` — fetch the topology graph for one cluster.
 *
 * Disabled while `clusterId` is falsy so that we don't fire a request with
 * an empty path segment.
 *
 * `includeFabric` (default false) → ADR-0004 / RFC-003. When truthy the
 * URL gains `?includeFabric=true`, causing the backend to emit
 * `type=switch` nodes and `type=fabric-link` edges in addition to the
 * regular cluster/node/npu/slice tree. Default false keeps the wire
 * payload byte-equivalent to the T-108a/T-108b expectation.
 *
 * `includeWorkloads` (default false) → ADR-0005 / RFC-003. Mirror of
 * `includeFabric`: when truthy the URL gains `?includeWorkloads=true`,
 * causing the backend (P1-T-213) to emit `type=workload` / `type=pod`
 * nodes plus `type=binds-to` (pod↔slice) and `type=pd-pair` (pod↔pod)
 * edges. Default false preserves byte-equivalence with the pre-T-214
 * topology DTO.
 *
 * Both flags participate in the queryKey so toggling either one triggers
 * a fresh fetch rather than serving a cached payload from the other
 * branch.
 */
export function useClusterTopology(
  clusterId: string | null | undefined,
  depth: TopologyDepth = 'slice',
  includeFabric = false,
  includeWorkloads = false,
) {
  return useQuery({
    queryKey: ['cluster-topology', clusterId, depth, includeFabric, includeWorkloads],
    queryFn: async (): Promise<Topology> => {
      // Build the params object conditionally — when either flag is false
      // we drop its key entirely so the request URL is identical to the
      // pre-T-212/T-214 shape (zero regression for snapshot-style tests
      // and for backend handlers that key on URL alone).
      const params: Record<string, string | boolean> = { depth };
      if (includeFabric) {
        params.includeFabric = true;
      }
      if (includeWorkloads) {
        params.includeWorkloads = true;
      }
      const { data } = await api.get<Topology>(
        `/api/v1/clusters/${clusterId}/topology`,
        { params },
      );
      return data;
    },
    enabled: Boolean(clusterId),
    staleTime: 30_000,
  });
}
