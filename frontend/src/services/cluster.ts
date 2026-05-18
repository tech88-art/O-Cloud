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
 */
export function useClusterTopology(
  clusterId: string | null | undefined,
  depth: TopologyDepth = 'slice',
) {
  return useQuery({
    queryKey: ['cluster-topology', clusterId, depth],
    queryFn: async (): Promise<Topology> => {
      const { data } = await api.get<Topology>(
        `/api/v1/clusters/${clusterId}/topology`,
        { params: { depth } },
      );
      return data;
    },
    enabled: Boolean(clusterId),
    staleTime: 30_000,
  });
}
