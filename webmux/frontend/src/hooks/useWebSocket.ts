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
  onSendError?: (message: string) => void;
}

export interface WebSocketHandle {
  send: (msg: WebSocketMessage) => void;
  close: () => void;
}

const MAX_PENDING_BYTES = 512 * 1024;
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
  const pendingRef = useRef<string[]>([]);
  const pendingBytesRef = useRef(0);
  const connectedOnceRef = useRef(false);
  const onSendErrorRef = useRef(options.onSendError);
  onSendErrorRef.current = options.onSendError;

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
    connectedOnceRef.current = false;

    function connect() {
      if (closedRef.current) return;

      const url = buildWsUrl(sessionId);
      const ws = new WebSocket(url);
      wsRef.current = ws;

      ws.onopen = () => {
        if (closedRef.current || wsRef.current !== ws) return;
        connectedOnceRef.current = true;
        attemptRef.current = 0;
        const queued = pendingRef.current;
        pendingRef.current = [];
        pendingBytesRef.current = 0;
        for (const msg of queued) {
          ws.send(msg);
        }
        onOpenRef.current?.();
      };

      ws.onclose = (event) => {
        if (closedRef.current || wsRef.current !== ws) return;
        wsRef.current = null;
        if (pendingRef.current.length) onSendErrorRef.current?.('Connection closed before queued input was sent. Please retry.');
        pendingRef.current = [];
        pendingBytesRef.current = 0;
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
      pendingBytesRef.current = 0;
    };
  }, [sessionId]);

  const send = useCallback((msg: WebSocketMessage) => {
    const serialized = JSON.stringify(msg);
    const size = new TextEncoder().encode(serialized).length;
    const ws = wsRef.current;
    if (closedRef.current || !ws || (ws.readyState !== WebSocket.OPEN &&
        (ws.readyState !== WebSocket.CONNECTING || connectedOnceRef.current))) {
      onSendErrorRef.current?.('Input was not sent: terminal disconnected. Reconnect and retry.');
      return;
    }
    if (size + pendingBytesRef.current + (ws.bufferedAmount || 0) > MAX_PENDING_BYTES) {
      onSendErrorRef.current?.('Input was not sent: terminal is busy or paste is too large. Wait and send smaller chunks.');
      return;
    }
    if (ws.readyState === WebSocket.OPEN) {
      try { ws.send(serialized); }
      catch { onSendErrorRef.current?.('Input was not sent. Reconnect and retry.'); }
    } else {
      pendingRef.current.push(serialized);
      pendingBytesRef.current += size;
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
