import { createContext, useContext, useCallback, useRef, useState, useMemo, useEffect } from 'react';
import type { ReactNode } from 'react';

type SendFn = (data: string) => void;
type ScrollbackFn = (lines: number) => string;

interface InputBroadcastState {
  broadcastMode: boolean;
  setBroadcastMode: (on: boolean) => void;
  focusedSessionId: string | null;
  setFocusedSessionId: (id: string) => void;
  registerSend: (sessionId: string, send: SendFn) => void;
  unregisterSend: (sessionId: string) => void;
  /** Route input: if broadcast mode, send to all; otherwise send to focused only. */
  routeInput: (fromSessionId: string, data: string) => void;
  /** Sessions excluded from broadcast */
  broadcastExcluded: Set<string>;
  toggleBroadcastExclude: (sessionId: string) => void;
  /** Scrollback access for AI context */
  registerScrollback: (sessionId: string, getter: ScrollbackFn) => void;
  unregisterScrollback: (sessionId: string) => void;
  getScrollbackForSession: (sessionId: string, lines: number) => string;
  /** Send raw input to a specific session (bypasses broadcast routing) */
  sendToSession: (sessionId: string, data: string) => void;
}

const InputBroadcastContext = createContext<InputBroadcastState | null>(null);

export function InputBroadcastProvider({ children }: { children: ReactNode }) {
  const [broadcastMode, setBroadcastMode] = useState(false);
  const [focusedSessionId, setFocusedSessionIdState] = useState<string | null>(null);
  const [broadcastExcluded, setBroadcastExcluded] = useState<Set<string>>(new Set());
  const sendFns = useRef(new Map<string, SendFn>());
  const scrollbackFns = useRef(new Map<string, ScrollbackFn>());
  const broadcastModeRef = useRef(broadcastMode);
  const broadcastExcludedRef = useRef(broadcastExcluded);

  useEffect(() => { broadcastModeRef.current = broadcastMode; }, [broadcastMode]);
  useEffect(() => { broadcastExcludedRef.current = broadcastExcluded; }, [broadcastExcluded]);

  const registerSend = useCallback((sessionId: string, send: SendFn) => {
    sendFns.current.set(sessionId, send);
  }, []);

  const unregisterSend = useCallback((sessionId: string) => {
    sendFns.current.delete(sessionId);
  }, []);

  const setFocusedSessionId = useCallback((id: string) => {
    setFocusedSessionIdState(id);
  }, []);

  const toggleBroadcastExclude = useCallback((sessionId: string) => {
    setBroadcastExcluded(prev => {
      const next = new Set(prev);
      if (next.has(sessionId)) {
        next.delete(sessionId);
      } else {
        next.add(sessionId);
      }
      return next;
    });
  }, []);

  const routeInput = useCallback((fromSessionId: string, data: string) => {
    if (broadcastModeRef.current) {
      const excluded = broadcastExcludedRef.current;
      sendFns.current.forEach((send, id) => {
        if (!excluded.has(id)) send(data);
      });
    } else {
      const send = sendFns.current.get(fromSessionId);
      if (send) send(data);
    }
  }, []);

  const registerScrollback = useCallback((sessionId: string, getter: ScrollbackFn) => {
    scrollbackFns.current.set(sessionId, getter);
  }, []);

  const unregisterScrollback = useCallback((sessionId: string) => {
    scrollbackFns.current.delete(sessionId);
  }, []);

  const getScrollbackForSession = useCallback((sessionId: string, lines: number): string => {
    const getter = scrollbackFns.current.get(sessionId);
    return getter ? getter(lines) : '';
  }, []);

  const sendToSession = useCallback((sessionId: string, data: string) => {
    const send = sendFns.current.get(sessionId);
    if (send) send(data);
  }, []);

  const value = useMemo(() => ({
    broadcastMode,
    setBroadcastMode,
    focusedSessionId,
    setFocusedSessionId,
    registerSend,
    unregisterSend,
    routeInput,
    broadcastExcluded,
    toggleBroadcastExclude,
    registerScrollback,
    unregisterScrollback,
    getScrollbackForSession,
    sendToSession,
  }), [broadcastMode, focusedSessionId, setFocusedSessionId, registerSend, unregisterSend, routeInput, broadcastExcluded, toggleBroadcastExclude, registerScrollback, unregisterScrollback, getScrollbackForSession, sendToSession]);

  return (
    <InputBroadcastContext.Provider value={value}>
      {children}
    </InputBroadcastContext.Provider>
  );
}

export function useInputBroadcast(): InputBroadcastState {
  const ctx = useContext(InputBroadcastContext);
  if (!ctx) throw new Error('useInputBroadcast must be used within InputBroadcastProvider');
  return ctx;
}
