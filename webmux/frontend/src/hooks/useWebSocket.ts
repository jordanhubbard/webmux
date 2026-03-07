import { useEffect, useRef, useCallback } from 'react';
import { buildWsUrl } from '../utils/api';
import type { WebSocketMessage } from '../types';

interface UseWebSocketOptions {
  sessionId: string;
  onMessage: (msg: WebSocketMessage) => void;
  onOpen?: () => void;
  onClose?: () => void;
}

export interface WebSocketHandle {
  send: (msg: WebSocketMessage) => void;
  close: () => void;
}

export function useWebSocket(options: UseWebSocketOptions): WebSocketHandle {
  const wsRef = useRef<WebSocket | null>(null);
  const { sessionId, onMessage, onOpen, onClose } = options;

  // Store callbacks in refs so the WebSocket handlers always call the latest
  // version without needing to recreate the socket when they change.
  const onMessageRef = useRef(onMessage);
  const onOpenRef = useRef(onOpen);
  const onCloseRef = useRef(onClose);
  useEffect(() => { onMessageRef.current = onMessage; }, [onMessage]);
  useEffect(() => { onOpenRef.current = onOpen; }, [onOpen]);
  useEffect(() => { onCloseRef.current = onClose; }, [onClose]);

  useEffect(() => {
    const url = buildWsUrl(sessionId);
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => onOpenRef.current?.();
    ws.onclose = () => onCloseRef.current?.();
    ws.onerror = (e) => console.error('WebSocket error', e);
    ws.onmessage = (event: MessageEvent<string>) => {
      try {
        const msg = JSON.parse(event.data) as WebSocketMessage;
        onMessageRef.current(msg);
      } catch {
        // ignore malformed messages
      }
    };

    return () => {
      ws.close();
      wsRef.current = null;
    };
  }, [sessionId]);

  const send = useCallback((msg: WebSocketMessage) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg));
    }
  }, []);

  const close = useCallback(() => {
    wsRef.current?.close();
  }, []);

  return { send, close };
}
