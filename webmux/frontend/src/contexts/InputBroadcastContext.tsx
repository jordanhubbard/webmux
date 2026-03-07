import { createContext, useContext, useCallback, useRef, useState } from 'react';
import type { ReactNode } from 'react';

type SendFn = (data: string) => void;

interface InputBroadcastState {
  broadcastMode: boolean;
  setBroadcastMode: (on: boolean) => void;
  focusedSessionId: string | null;
  setFocusedSessionId: (id: string) => void;
  registerSend: (sessionId: string, send: SendFn) => void;
  unregisterSend: (sessionId: string) => void;
  /** Route input: if broadcast mode, send to all; otherwise send to focused only. */
  routeInput: (fromSessionId: string, data: string) => void;
}

const InputBroadcastContext = createContext<InputBroadcastState | null>(null);

export function InputBroadcastProvider({ children }: { children: ReactNode }) {
  const [broadcastMode, setBroadcastMode] = useState(false);
  const [focusedSessionId, setFocusedSessionIdState] = useState<string | null>(null);
  const sendFns = useRef(new Map<string, SendFn>());

  const registerSend = useCallback((sessionId: string, send: SendFn) => {
    sendFns.current.set(sessionId, send);
    // Auto-focus first registered session if nothing focused
  }, []);

  const unregisterSend = useCallback((sessionId: string) => {
    sendFns.current.delete(sessionId);
  }, []);

  const setFocusedSessionId = useCallback((id: string) => {
    setFocusedSessionIdState(id);
  }, []);

  const routeInput = useCallback((fromSessionId: string, data: string) => {
    if (broadcastMode) {
      // Send to ALL registered sessions
      sendFns.current.forEach(send => send(data));
    } else {
      // Send only to the originating session's WebSocket
      const send = sendFns.current.get(fromSessionId);
      if (send) send(data);
    }
  }, [broadcastMode]);

  return (
    <InputBroadcastContext.Provider value={{
      broadcastMode,
      setBroadcastMode,
      focusedSessionId,
      setFocusedSessionId,
      registerSend,
      unregisterSend,
      routeInput,
    }}>
      {children}
    </InputBroadcastContext.Provider>
  );
}

export function useInputBroadcast(): InputBroadcastState {
  const ctx = useContext(InputBroadcastContext);
  if (!ctx) throw new Error('useInputBroadcast must be used within InputBroadcastProvider');
  return ctx;
}
