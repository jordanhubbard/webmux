import { createContext, useContext, useCallback, useRef, useState, useMemo } from 'react';
import type { ReactNode } from 'react';

type SendFn = (data: string) => void;
// null retains default broadcast behavior until the owner chooses recipients.
type Recipients = ReadonlySet<string> | null;
const includesRecipient = (recipients: Recipients, id: string) => recipients === null || recipients.has(id);

interface InputBroadcastState {
  broadcastMode: boolean;
  setBroadcastMode: (on: boolean) => void;
  focusedSessionId: string | null;
  setFocusedSessionId: (id: string) => void;
  registerSend: (sessionId: string, send: SendFn) => void;
  unregisterSend: (sessionId: string) => void;
  /** Route broadcast input to its selected viewers; ordinary input stays at its source. */
  routeInput: (fromSessionId: string, data: string) => void;
  broadcastScope: 'all' | 'selected';
  isBroadcastRecipient: (sessionId: string) => boolean;
  toggleBroadcastExclude: (sessionId: string) => void;
  setBroadcastRecipients: (allIds: string[], selectedIds: string[]) => void;
}

const InputBroadcastContext = createContext<InputBroadcastState | null>(null);

export function InputBroadcastProvider({ children }: { children: ReactNode }) {
  const [broadcastMode, setBroadcastModeState] = useState(false);
  const [focusedSessionId, setFocusedSessionIdState] = useState<string | null>(null);
  const [recipients, setRecipients] = useState<Recipients>(null);
  const sendFns = useRef(new Map<string, SendFn>());
  const broadcastModeRef = useRef(false);
  const recipientsRef = useRef<Recipients>(null);

  const setBroadcastMode = useCallback((on: boolean) => {
    broadcastModeRef.current = on;
    setBroadcastModeState(on);
  }, []);

  const registerSend = useCallback((sessionId: string, send: SendFn) => {
    sendFns.current.set(sessionId, send);
  }, []);

  const unregisterSend = useCallback((sessionId: string) => {
    sendFns.current.delete(sessionId);
  }, []);

  const setFocusedSessionId = useCallback((id: string) => {
    setFocusedSessionIdState(id);
  }, []);

  const commitRecipients = useCallback((next: ReadonlySet<string>) => {
    // Input may be routed in this same event, before React commits a render.
    recipientsRef.current = next;
    setRecipients(next);
  }, []);

  const toggleBroadcastExclude = useCallback((sessionId: string) => {
    const current = recipientsRef.current;
    const next = new Set(current ?? sendFns.current.keys());
    if (includesRecipient(current, sessionId)) next.delete(sessionId);
    else next.add(sessionId);
    commitRecipients(next);
  }, [commitRecipients]);

  const setBroadcastRecipients = useCallback((allIds: string[], selectedIds: string[]) => {
    const known = new Set(allIds);
    commitRecipients(new Set(selectedIds.filter(id => known.has(id))));
  }, [commitRecipients]);

  const isBroadcastRecipient = useCallback((sessionId: string) => includesRecipient(recipients, sessionId), [recipients]);

  const routeInput = useCallback((fromSessionId: string, data: string) => {
    if (broadcastModeRef.current) {
      const selected = recipientsRef.current;
      sendFns.current.forEach((send, id) => {
        if (includesRecipient(selected, id)) send(data);
      });
    } else {
      sendFns.current.get(fromSessionId)?.(data);
    }
  }, []);

  const value = useMemo(() => ({
    broadcastMode,
    setBroadcastMode,
    focusedSessionId,
    setFocusedSessionId,
    registerSend,
    unregisterSend,
    routeInput,
    broadcastScope: recipients === null ? 'all' as const : 'selected' as const,
    isBroadcastRecipient,
    toggleBroadcastExclude,
    setBroadcastRecipients,
  }), [broadcastMode, setBroadcastMode, focusedSessionId, setFocusedSessionId, registerSend, unregisterSend, routeInput, recipients, isBroadcastRecipient, toggleBroadcastExclude, setBroadcastRecipients]);

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
