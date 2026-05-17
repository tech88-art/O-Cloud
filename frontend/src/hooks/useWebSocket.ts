import { useEffect, useRef, useState } from 'react';
import { getRuntimeConfig } from '@/config/runtime';

/**
 * Stub WebSocket hook — will be fleshed out in P1-T-105 (WS topology) and
 * surrounding tasks. For now it exposes the shape so downstream code can
 * compile against it.
 *
 * See frontend/CLAUDE.md §4.6.
 */
export type WSStatus = 'idle' | 'connecting' | 'open' | 'closed' | 'error';

export interface UseWebSocketResult<T> {
  status: WSStatus;
  lastMessage: T | null;
}

export function useWebSocket<T = unknown>(path: string | null): UseWebSocketResult<T> {
  const [status, setStatus] = useState<WSStatus>('idle');
  const [lastMessage, setLastMessage] = useState<T | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    if (!path) {
      setStatus('idle');
      return;
    }
    const base = getRuntimeConfig().wsBaseURL;
    const url = `${base}${path.startsWith('/') ? path : `/${path}`}`;
    setStatus('connecting');
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => setStatus('open');
    ws.onclose = () => setStatus('closed');
    ws.onerror = () => setStatus('error');
    ws.onmessage = (ev) => {
      try {
        const parsed = JSON.parse(ev.data as string) as T;
        setLastMessage(parsed);
      } catch (err) {
        console.warn('ws message parse failed', err);
      }
    };

    return () => {
      ws.close();
      wsRef.current = null;
    };
  }, [path]);

  return { status, lastMessage };
}
