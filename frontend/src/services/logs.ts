import { useQuery } from '@tanstack/react-query';
import { api } from './api';
import type { components } from './types';

/**
 * Workload logs react-query hook (P1-T-302).
 *
 * REST tail: `GET /api/v1/workloads/:namespace/:name/logs?tail=N&container=X`
 * returns LogPage{lines[], hasMore, nextCursor}. We surface the LogPage as-is
 * so the Logs page can decide whether to render hasMore / nextCursor cues.
 *
 * Disabled while either path segment is missing — mirrors useWorkloadDetail
 * so the page doesn't fire a request with empty `/api/v1/workloads//logs`.
 *
 * The WS streaming hook lives in `hooks/useLogsWS.ts` (page state ownership
 * sits there, not here — react-query is a poor fit for a forward-only push
 * stream).
 */
export type LogPage = components['schemas']['LogPage'];
export type LogLine = NonNullable<LogPage['lines']>[number];

export interface LogQueryOptions {
  /** Container name filter (case-sensitive exact match). Empty → no filter. */
  container?: string;
  /** Number of lines to return. Default 200, cap 2000 enforced by backend. */
  tail?: number;
  /** ISO 8601 timestamp. Currently advisory — mock ignores it; PHASE-2 honors. */
  since?: string;
}

/**
 * `useWorkloadLogs(namespace, name, opts)` — fetch a REST tail of log lines.
 *
 * The queryKey includes opts so flipping the container filter / tail size
 * triggers a fresh fetch rather than serving stale lines. staleTime is 0 —
 * unlike the structural queries (cluster / topology / workloads list) the
 * log tail is intrinsically time-sensitive; even a 5-second cache would
 * surface "old" lines on remount and confuse the operator.
 */
export function useWorkloadLogs(
  namespace: string | null | undefined,
  name: string | null | undefined,
  opts: LogQueryOptions = {},
) {
  const params: Record<string, string | number> = {};
  if (opts.container && opts.container.trim().length > 0) {
    params.container = opts.container.trim();
  }
  if (typeof opts.tail === 'number' && opts.tail > 0) {
    params.tail = opts.tail;
  }
  if (opts.since && opts.since.trim().length > 0) {
    params.since = opts.since.trim();
  }
  return useQuery({
    queryKey: ['workload-logs', namespace, name, params],
    queryFn: async (): Promise<LogPage> => {
      const { data } = await api.get<LogPage>(
        `/api/v1/workloads/${encodeURIComponent(String(namespace))}/${encodeURIComponent(String(name))}/logs`,
        { params },
      );
      return data;
    },
    enabled: Boolean(namespace) && Boolean(name),
    staleTime: 0,
    // Logs are time-sensitive; don't keep stale tail visible while a refetch
    // is in flight — show the loading skeleton instead.
    placeholderData: undefined,
  });
}
