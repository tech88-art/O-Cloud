import { useMutation, useQuery } from '@tanstack/react-query';
import type { AxiosError } from 'axios';
import { api } from './api';
import type { components } from './types';

/**
 * Preset + deploy react-query hooks (P1-T-207).
 *
 * Mirrors `services/cluster.ts` / `services/node.ts` style:
 *   - one hook per endpoint
 *   - types pulled from the auto-generated `components['schemas'][...]`
 *   - no hand-written API shapes (frontend/CLAUDE.md §4.1, §4.2)
 *
 * `useNodesList` lives here (not in `services/node.ts`) because manual-mode
 * placement is a Deploy-page concern and the existing `services/node.ts`
 * exports per-node hooks only. The Allowed Paths for T-207 cover this file
 * but not `node.ts`; adding the list query here keeps both modules tight.
 */

export type Preset = components['schemas']['Preset'];
export type PresetDetail = components['schemas']['PresetDetail'];
export type DeployRequest = components['schemas']['DeployRequest'];
export type DeployResponse = components['schemas']['DeployResponse'];
export type Node = components['schemas']['Node'];
export type NPU = components['schemas']['NPU'];

/**
 * `usePresets` — list available preset applications from
 * `GET /api/v1/presets`. The Deploy page renders these as cards.
 */
export function usePresets() {
  return useQuery({
    queryKey: ['presets'],
    queryFn: async (): Promise<Preset[]> => {
      const { data } = await api.get<Preset[]>('/api/v1/presets');
      return data;
    },
    staleTime: 60_000,
  });
}

/**
 * `useNodesList` — `GET /api/v1/nodes` for the manual-mode Node `Select`.
 *
 * `enabled` controls whether the query fires; the wizard mounts this only
 * on the manual step so we avoid an idle fetch during auto-mode usage.
 */
export function useNodesList(options: { enabled?: boolean } = {}) {
  const { enabled = true } = options;
  return useQuery({
    queryKey: ['nodes-list'],
    queryFn: async (): Promise<Node[]> => {
      const { data } = await api.get<Node[]>('/api/v1/nodes');
      return data;
    },
    enabled,
    staleTime: 30_000,
  });
}

/**
 * `useNodeNpusList` — `GET /api/v1/nodes/:name/npus` re-exported under the
 * preset namespace for symmetry with `useNodesList`. Same payload shape as
 * `services/node.ts#useNodeNPUs`. We duplicate the wrapper rather than
 * cross-import to keep this file's Allowed-Path footprint minimal and to
 * avoid edits to `services/node.ts`.
 */
export function useNodeNpusList(
  nodeName: string | null | undefined,
  options: { enabled?: boolean } = {},
) {
  const { enabled = true } = options;
  return useQuery({
    queryKey: ['preset-deploy-node-npus', nodeName],
    queryFn: async (): Promise<NPU[]> => {
      const { data } = await api.get<NPU[]>(
        `/api/v1/nodes/${encodeURIComponent(String(nodeName))}/npus`,
      );
      return data;
    },
    enabled: enabled && Boolean(nodeName),
    staleTime: 30_000,
  });
}

/**
 * `DeployMutationError` — narrows the error returned by the deploy
 * mutation so callers can branch on the HTTP status without juggling
 * `AxiosError` generics. `status` is `null` for offline / non-HTTP errors.
 */
export interface DeployMutationError {
  status: number | null;
  message: string;
}

function normalizeDeployError(err: unknown): DeployMutationError {
  const ax = err as AxiosError<{ message?: string }> | undefined;
  if (ax?.isAxiosError) {
    const status = ax.response?.status ?? null;
    const data = ax.response?.data;
    const message =
      (data && typeof data === 'object' && 'message' in data
        ? (data as { message?: string }).message
        : undefined) ??
      ax.message ??
      '';
    return { status, message };
  }
  if (err instanceof Error) {
    return { status: null, message: err.message };
  }
  return { status: null, message: String(err ?? '') };
}

/**
 * `useDeployMutation` — `POST /api/v1/deploy`. Returns a mutation whose
 * `error` is shaped as `DeployMutationError` so callers can distinguish
 * 409 (resource conflict, leave the wizard open) from other failures
 * without re-parsing axios errors.
 *
 * The contract returns 201 on success and 400 / 409 / 5xx on failure.
 * react-query will surface the rejection as `mutation.error`; the
 * `mutationFn` normalises it via `normalizeDeployError` before throwing.
 */
export function useDeployMutation() {
  return useMutation<DeployResponse, DeployMutationError, DeployRequest>({
    mutationFn: async (req: DeployRequest): Promise<DeployResponse> => {
      try {
        const { data } = await api.post<DeployResponse>(
          '/api/v1/deploy',
          req,
        );
        return data;
      } catch (err) {
        throw normalizeDeployError(err);
      }
    },
  });
}
