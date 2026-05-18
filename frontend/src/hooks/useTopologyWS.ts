import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { getRuntimeConfig } from '@/config/runtime';
import { useTopologyStore } from '@/store/topologyStore';
import type { components } from '@/services/types';

/**
 * P1-T-108b — topology WebSocket subscription hook.
 *
 * Connects to `/ws/topology` (NOTE: NOT under `/api/v1/` — the WS endpoint
 * is mounted at the root by the backend), parses incoming WSMessages, and
 * either:
 *
 *   - invalidates the relevant react-query cache (so the next render
 *     refetches via REST and the page re-paints with fresh data), or
 *   - patches the Zustand `lastEventAt` so a live-status indicator can
 *     show "feed is alive".
 *
 * Both effects happen on every event so the page is correct regardless
 * of which strategy the consumer relies on. Refetch-on-invalidate is
 * the load-bearing behaviour; `lastEventAt` is observability only.
 *
 * **Reconnect**: on any close / error, schedule a reconnect with
 * exponential backoff capped at `reconnectDelayMs` (default 3000ms).
 * The first retry fires at 250ms, then doubles up to the cap, so a
 * transient disconnect comes back fast and a persistent one doesn't
 * hammer the server. The hook never gives up — operators rely on the
 * indicator to know the link is down.
 *
 * **Cleanup**: on unmount or `clusterId` change we close the socket and
 * cancel any pending reconnect timer.
 */

export type WSStatus = 'idle' | 'connecting' | 'open' | 'closed';

export interface UseTopologyWSResult {
  status: WSStatus;
  lastError?: Error;
}

export interface UseTopologyWSOptions {
  /** Cap for exponential reconnect backoff (default 3000ms). */
  reconnectDelayMs?: number;
}

/** Types we care about on `/ws/topology`. Other types are accepted but
 *  trigger only a generic topology refetch (safe superset).
 *
 *  Note: `slice.allocated` / `slice.released` / `npu.statusChanged` are
 *  NOT in the OpenAPI WSMessage enum at time of writing (T-105 covers
 *  `topology.update` / `workload.statusChanged`). We accept them defensively
 *  because the task spec calls them out — the backend may emit them; if
 *  not, no harm done. */
type WSMessage = components['schemas']['WSMessage'];
type RelevantType =
  | WSMessage['type']
  | 'slice.allocated'
  | 'slice.released'
  | 'npu.statusChanged';
const TOPOLOGY_AFFECTING: ReadonlySet<RelevantType> = new Set<RelevantType>([
  'topology.update',
  'topology.delta',
  'slice.allocated',
  'slice.released',
  'npu.statusChanged',
  'workload.statusChanged',
]);

/** Initial reconnect delay; doubles until it reaches `reconnectDelayMs`. */
const INITIAL_RECONNECT_DELAY_MS = 250;

export function useTopologyWS(
  clusterId: string | null,
  opts: UseTopologyWSOptions = {},
): UseTopologyWSResult {
  const reconnectCap = opts.reconnectDelayMs ?? 3_000;
  const queryClient = useQueryClient();
  const setLastEventAt = useTopologyStore((s) => s.setLastEventAt);

  const [status, setStatus] = useState<WSStatus>('idle');
  const [lastError, setLastError] = useState<Error | undefined>(undefined);

  // Mutable refs survive renders without re-firing the connect effect.
  const wsRef = useRef<WebSocket | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectDelayRef = useRef<number>(INITIAL_RECONNECT_DELAY_MS);
  // Guard so an in-flight reconnect from a previous mount doesn't race a
  // new mount's connection attempt.
  const cancelledRef = useRef<boolean>(false);

  useEffect(() => {
    if (!clusterId) {
      setStatus('idle');
      return;
    }
    cancelledRef.current = false;

    // Resolve runtime config; if it isn't loaded yet (e.g. in a test
    // environment that mounts <Overview> without calling
    // `loadRuntimeConfig()` first) stay idle rather than crash. The page
    // can re-mount once config lands; we don't try to poll.
    let url: string;
    try {
      url = buildWsUrl();
    } catch (err) {
      console.warn('useTopologyWS: runtime config unavailable, staying idle', err);
      setStatus('idle');
      setLastError(coerceError(err));
      return;
    }

    const connect = () => {
      if (cancelledRef.current) return;
      setStatus('connecting');
      let ws: WebSocket;
      try {
        ws = new WebSocket(url);
      } catch (err) {
        // URL parse failure / browser blocks WS schema → record + schedule
        // a reconnect at the cap (no point retrying fast on a malformed URL).
        setLastError(coerceError(err));
        scheduleReconnect(reconnectCap);
        return;
      }
      wsRef.current = ws;

      ws.onopen = () => {
        if (cancelledRef.current) return;
        // Reset backoff on a successful handshake — next disconnect retries
        // fast again.
        reconnectDelayRef.current = INITIAL_RECONNECT_DELAY_MS;
        setStatus('open');
        setLastError(undefined);
      };

      ws.onmessage = (ev) => {
        if (cancelledRef.current) return;
        let parsed: WSMessage;
        try {
          parsed = JSON.parse(String(ev.data)) as WSMessage;
        } catch (err) {
          // Bad JSON from server — log + ignore. Don't tear down the socket.
          console.warn('useTopologyWS: drop unparseable WS frame', err);
          return;
        }
        handleMessage(parsed);
      };

      ws.onerror = () => {
        if (cancelledRef.current) return;
        // The Event payload from a WebSocket onerror is opaque; record a
        // synthetic Error so consumers have a non-empty diagnostic.
        setLastError(new Error('websocket error'));
      };

      ws.onclose = () => {
        if (cancelledRef.current) return;
        setStatus('closed');
        scheduleReconnect(reconnectDelayRef.current);
        // Backoff for next attempt; capped at the configured ceiling.
        reconnectDelayRef.current = Math.min(
          reconnectDelayRef.current * 2,
          reconnectCap,
        );
      };
    };

    const scheduleReconnect = (delayMs: number) => {
      if (cancelledRef.current) return;
      if (timerRef.current) {
        clearTimeout(timerRef.current);
      }
      timerRef.current = setTimeout(() => {
        timerRef.current = null;
        connect();
      }, delayMs);
    };

    const handleMessage = (msg: WSMessage) => {
      const ts = msg.timestamp ?? new Date().toISOString();
      setLastEventAt(ts);
      const type = msg.type as RelevantType;
      if (TOPOLOGY_AFFECTING.has(type)) {
        // Single invalidation covers all depth variants of the cluster
        // topology query because the key is `['cluster-topology', clusterId, depth]`.
        void queryClient.invalidateQueries({
          queryKey: ['cluster-topology', clusterId],
        });
        // The detail panel separately depends on node REST data; if a node
        // status changed the easiest thing is to invalidate both.
        if (type === 'npu.statusChanged' || type === 'slice.allocated' || type === 'slice.released') {
          void queryClient.invalidateQueries({ queryKey: ['node-npus'] });
          void queryClient.invalidateQueries({ queryKey: ['node'] });
        }
      }
    };

    connect();

    return () => {
      cancelledRef.current = true;
      if (timerRef.current) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
      const ws = wsRef.current;
      wsRef.current = null;
      if (ws) {
        // Detach handlers BEFORE close so the close handler doesn't
        // schedule another reconnect on the way out.
        ws.onopen = null;
        ws.onmessage = null;
        ws.onerror = null;
        ws.onclose = null;
        try {
          ws.close();
        } catch {
          /* ignore — already closed */
        }
      }
      reconnectDelayRef.current = INITIAL_RECONNECT_DELAY_MS;
      setStatus('idle');
    };
    // We deliberately leave `queryClient` / `setLastEventAt` out of the dep
    // list — both are stable references inside the providers and adding them
    // would re-fire the connect effect on every render. Same reasoning as
    // the standard react-query pattern for socket effects.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clusterId, reconnectCap]);

  return { status, lastError };
}

/**
 * Build the WS URL. We use the runtime-config `wsBaseURL` (set by
 * `/config.json` on deploy) so the same build can target dev / staging /
 * prod without rebuild.
 *
 * Note: backend mounts `/ws/topology` at the ROOT, not under `/api/v1`.
 */
function buildWsUrl(): string {
  const base = getRuntimeConfig().wsBaseURL;
  // Tolerate trailing slash in config without producing `//`.
  const trimmed = base.endsWith('/') ? base.slice(0, -1) : base;
  return `${trimmed}/ws/topology`;
}

function coerceError(err: unknown): Error {
  if (err instanceof Error) return err;
  return new Error(typeof err === 'string' ? err : 'websocket error');
}
