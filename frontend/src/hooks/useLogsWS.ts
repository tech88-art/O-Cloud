import { useEffect, useRef, useState } from 'react';
import { getRuntimeConfig } from '@/config/runtime';
import type { components } from '@/services/types';
import type { LogLine } from '@/services/logs';

/**
 * P1-T-302 — workload logs WebSocket subscription hook.
 *
 * Connects to `/ws/logs/:namespace/:name` (NOT under `/api/v1/` — mounted at
 * the root, see backend/pkg/api/router.go). Parses incoming WSMessage frames
 * with type === "log" and forwards each decoded LogLine to the caller-supplied
 * `onLine` callback.
 *
 * Why callback (not react-query): logs are a forward-only push stream; storing
 * them through react-query's cache would either pile up indefinitely or
 * require manual eviction. The page owns the line buffer and decides what to
 * cap / scroll / colour-code. Hook = pure transport.
 *
 * Lifecycle mirrors `useTopologyWS`:
 *   - exponential reconnect backoff capped at `reconnectDelayMs` (default 3s),
 *     starting at 250ms.
 *   - `enabled === false` keeps the socket closed (used by the "Live stream"
 *     toggle on the Logs page).
 *   - Cleanup detaches handlers BEFORE close so the close handler doesn't
 *     schedule another reconnect on the way out.
 */

export type WSStatus = 'idle' | 'connecting' | 'open' | 'closed';

export interface UseLogsWSResult {
  status: WSStatus;
  lastError?: Error;
}

export interface UseLogsWSOptions {
  /** Cap for exponential reconnect backoff (default 3000ms). */
  reconnectDelayMs?: number;
  /** Optional container filter — forwarded as `?container=` to the WS URL. */
  container?: string;
}

type WSMessage = components['schemas']['WSMessage'];

const INITIAL_RECONNECT_DELAY_MS = 250;

export function useLogsWS(
  namespace: string | null | undefined,
  name: string | null | undefined,
  enabled: boolean,
  onLine: (line: LogLine) => void,
  opts: UseLogsWSOptions = {},
): UseLogsWSResult {
  const reconnectCap = opts.reconnectDelayMs ?? 3_000;
  const container = opts.container ?? '';

  const [status, setStatus] = useState<WSStatus>('idle');
  const [lastError, setLastError] = useState<Error | undefined>(undefined);

  // The `onLine` reference may change every render (caller often inlines a
  // closure). We capture it in a ref so the connect-effect doesn't churn the
  // socket every render — and we always invoke the latest callback when a
  // frame arrives.
  const onLineRef = useRef(onLine);
  onLineRef.current = onLine;

  const wsRef = useRef<WebSocket | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectDelayRef = useRef<number>(INITIAL_RECONNECT_DELAY_MS);
  const cancelledRef = useRef<boolean>(false);

  useEffect(() => {
    if (!enabled || !namespace || !name) {
      setStatus('idle');
      return;
    }
    cancelledRef.current = false;

    let url: string;
    try {
      url = buildLogsWsUrl(namespace, name, container);
    } catch (err) {
      console.warn('useLogsWS: runtime config unavailable, staying idle', err);
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
        setLastError(coerceError(err));
        scheduleReconnect(reconnectCap);
        return;
      }
      wsRef.current = ws;

      ws.onopen = () => {
        if (cancelledRef.current) return;
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
          console.warn('useLogsWS: drop unparseable WS frame', err);
          return;
        }
        if (parsed.type !== 'log.line') {
          // The /ws/logs endpoint only emits type=log.line envelopes
          // (canonical name per docs/api-contract.yaml WSMessage enum),
          // but be defensive — silently drop anything else so a future
          // contract addition doesn't crash the client.
          return;
        }
        const payload = parsed.payload as unknown;
        if (!payload || typeof payload !== 'object') return;
        // The backend wraps the LogLine in WSMessage.payload (json.RawMessage
        // on the wire). On the JS side it's already a parsed object.
        onLineRef.current(payload as LogLine);
      };

      ws.onerror = () => {
        if (cancelledRef.current) return;
        setLastError(new Error('websocket error'));
      };

      ws.onclose = () => {
        if (cancelledRef.current) return;
        setStatus('closed');
        scheduleReconnect(reconnectDelayRef.current);
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
    // `onLine` lives in onLineRef so we don't want it as a dep (it changes
    // every render); same reasoning as the useTopologyWS pattern.
  }, [namespace, name, enabled, container, reconnectCap]);

  return { status, lastError };
}

function buildLogsWsUrl(namespace: string, name: string, container: string): string {
  const base = getRuntimeConfig().wsBaseURL;
  const trimmed = base.endsWith('/') ? base.slice(0, -1) : base;
  const path = `/ws/logs/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`;
  if (container && container.length > 0) {
    return `${trimmed}${path}?container=${encodeURIComponent(container)}`;
  }
  return `${trimmed}${path}`;
}

function coerceError(err: unknown): Error {
  if (err instanceof Error) return err;
  return new Error(typeof err === 'string' ? err : 'websocket error');
}
