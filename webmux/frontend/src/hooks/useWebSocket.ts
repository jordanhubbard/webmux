import { useEffect, useRef, useCallback } from 'react';
import { buildWsUrl } from '../utils/api';
import type { WebSocketMessage } from '../types';
import { signalAuthExpired } from '../utils/authSession';

interface UseWebSocketOptions {
  sessionId: string;
  onMessage: (msg: WebSocketMessage) => void;
  onOpen?: () => void;
  onClose?: () => void;
  onUnauthorized?: () => void;
}

export interface WebSocketHandle {
  send: (msg: WebSocketMessage) => void;
  close: () => void;
}

const INITIAL_DELAY_MS = 1000;
const MAX_DELAY_MS = 30_000;

export function useWebSocket(options: UseWebSocketOptions): WebSocketHandle {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const attemptRef = useRef(0);
  const closedRef = useRef(false);
  // Messages sent before the socket reaches OPEN (e.g. the initial terminal
  // fit/resize, which fires synchronously on mount while the handshake is
  // still in flight) are queued here and flushed in order once onopen fires,
  // instead of being silently dropped.
  const pendingRef = useRef<WebSocketMessage[]>([]);

  const { sessionId, onMessage, onOpen, onClose, onUnauthorized } = options;

  const onMessageRef = useRef(onMessage);
  const onOpenRef = useRef(onOpen);
  const onCloseRef = useRef(onClose);
  const onUnauthorizedRef = useRef(onUnauthorized);
  useEffect(() => { onMessageRef.current = onMessage; }, [onMessage]);
  useEffect(() => { onOpenRef.current = onOpen; }, [onOpen]);
  useEffect(() => { onCloseRef.current = onClose; }, [onClose]);
  useEffect(() => { onUnauthorizedRef.current = onUnauthorized; }, [onUnauthorized]);

  useEffect(() => {
    const closeExpiredConnection = () => {
      closedRef.current = true;
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
      wsRef.current?.close(1000);
    };
    window.addEventListener('webmux:auth-expired', closeExpiredConnection);
    return () => window.removeEventListener('webmux:auth-expired', closeExpiredConnection);
  }, []);

  useEffect(() => {
    closedRef.current = false;
    attemptRef.current = 0;

    function connect() {
      if (closedRef.current) return;

      const url = buildWsUrl(sessionId);
      const ws = new WebSocket(url);
      wsRef.current = ws;

      ws.onopen = () => {
        attemptRef.current = 0;
        const queued = pendingRef.current;
        pendingRef.current = [];
        for (const msg of queued) {
          ws.send(JSON.stringify(msg));
        }
        onOpenRef.current?.();
      };

      ws.onclose = (event) => {
        if (closedRef.current) return;
        onCloseRef.current?.();
        if (event.code === 1008 && event.reason === 'Unauthorized') {
          closedRef.current = true;
          onUnauthorizedRef.current?.();
          signalAuthExpired();
          return;
        }
        // 1000 = intentional close (component unmount, session deleted)
        if (event.code === 1000) return;
        const delay = Math.min(INITIAL_DELAY_MS * Math.pow(2, attemptRef.current), MAX_DELAY_MS);
        attemptRef.current++;
        reconnectTimerRef.current = setTimeout(connect, delay);
      };

      ws.onerror = (e) => console.error('WebSocket error', e);

      ws.onmessage = (event: MessageEvent<string>) => {
        try {
          const msg = JSON.parse(event.data) as WebSocketMessage;
          onMessageRef.current(msg);
        } catch {
          // ignore malformed messages
        }
      };
    }

    connect();

    return () => {
      closedRef.current = true;
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      wsRef.current?.close(1000);
      wsRef.current = null;
      // Drop anything still queued so a torn-down component can't leak
      // sends into a future socket (new session, or a remount).
      pendingRef.current = [];
    };
  }, [sessionId]);

  const send = useCallback((msg: WebSocketMessage) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg));
    } else if (wsRef.current?.readyState === WebSocket.CONNECTING) {
      pendingRef.current.push(msg);
    }
  }, []);

  const close = useCallback(() => {
    closedRef.current = true;
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    wsRef.current?.close(1000);
  }, []);

  return { send, close };
}
