import { useCallback, useEffect, useRef, useState } from 'react';
import { api, checkedCoordinationSendOutcome, type CoordinationSendInput } from '../utils/api';
import type { CoordinationSendResult } from '../components/CoordinationDialog';

type Pending = { id: string; scope: string; input?: CoordinationSendInput; outcome?: CoordinationSendResult };
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const storageKey = (scope: string) => `webmux-coordination-send:${scope}`;
function requestID() {
  // getRandomValues also works on HTTP origins without randomUUID.
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 15) | 64; bytes[8] = (bytes[8] & 63) | 128;
  const hex = Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('');
  return [hex.slice(0, 8), hex.slice(8, 12), hex.slice(12, 16), hex.slice(16, 20), hex.slice(20)].join('-');
}

// Store only an opaque ID, partitioned by the server's home/owner scope. The
// frozen instruction lives in the private server ledger, never browser storage.
export function useCoordinationSendIntent(enabled: boolean, returned: (target: string, result: CoordinationSendResult) => void) {
  const [scope, setScope] = useState('');
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [pending, setPending] = useState<Pending | null>(null);
  const current = useRef<Pending | null>(null);
  const inFlight = useRef(false);
  const save = useCallback((next: Pending | null) => { current.current = next; setPending(next); }, []);
  const accept = useCallback((original: Pending, value: CoordinationSendResult) => {
    const outcome = checkedCoordinationSendOutcome(value, original.id, original.scope);
    const input = original.input || outcome.request;
    if (input && outcome.target && outcome.target !== input.target) throw new Error('The saved send target does not match its receipt. Inspect the original request.');
    save({ ...original, input, outcome });
    if (input) returned(input.target, { ...outcome, target: input.target });
  }, [returned, save]);

  useEffect(() => {
    if (!enabled) { setReady(false); return; }
    let active = true;
    setReady(false);
    void (async () => {
      try {
        const value = await api.getCoordinationSendScope();
        if (!active) return;
        if (!/^[a-f0-9]{64}$/.test(value.scope)) throw new Error('Cannot verify the instruction owner scope. Nothing was sent.');
        setScope(value.scope);
        const id = sessionStorage.getItem(storageKey(value.scope));
        if (id) {
          if (!uuid.test(id)) throw new Error('The saved instruction identity is unreadable. Nothing new was sent.');
          const original = { id, scope: value.scope };
          save(original);
          const result = await api.inspectCoordinationSend(id, value.scope);
          if (!active) return;
          accept(original, result);
        }
        if (active) { setReady(true); setError(''); }
      } catch (reason) { if (active) setError(String(reason)); }
    })();
    return () => { active = false; };
  }, [enabled, accept, save]);

  const invoke = useCallback(async (original: Pending) => {
    if (!original.input) return;
    try { accept(original, await api.sendCoordinationMessage(original.input)); }
    catch (reason) {
      setError(String(reason));
      returned(original.input.target, { requestId: original.id, requestState: 'uncertain', target: original.input.target, queued: false, submitted: false });
    }
  }, [accept, returned]);
  const send = async (input: Omit<CoordinationSendInput, 'requestId' | 'scope'>) => {
    if (!ready || !scope || inFlight.current || current.current) return;
    inFlight.current = true; setBusy(true); setError('');
    try {
      const key = storageKey(scope);
      const saved = sessionStorage.getItem(key);
      if (saved) {
        if (!uuid.test(saved)) throw new Error('The saved instruction identity is unreadable. Nothing new was sent.');
        const original = { id: saved, scope };
        save(original);
        accept(original, await api.inspectCoordinationSend(saved, scope));
        return;
      }
      const id = requestID();
      sessionStorage.setItem(key, id);
      if (sessionStorage.getItem(key) !== id) throw new Error('Cannot retain this instruction identity in the browser. Nothing was sent.');
      const original = { id, scope, input: { ...input, requestId: id, scope } };
      save(original);
      returned(input.target, { requestId: id, requestState: 'pending', requestCode: 'awaiting_response', target: input.target, queued: false, submitted: false });
      await invoke(original);
    } catch (reason) { setError(String(reason)); }
    finally { inFlight.current = false; setBusy(false); }
  };
  const inspect = async () => {
    const original = current.current;
    if (!original || inFlight.current) return;
    inFlight.current = true; setBusy(true); setError('');
    try { accept(original, await api.inspectCoordinationSend(original.id, original.scope)); setReady(true); }
    catch (reason) { setError(String(reason)); }
    finally { inFlight.current = false; setBusy(false); }
  };
  const continueOriginal = async () => {
    const original = current.current;
    if (!original?.input || inFlight.current || (original.outcome && !['pending', 'unobserved'].includes(original.outcome.requestState || ''))) return;
    inFlight.current = true; setBusy(true); setError('');
    try { await invoke(original); }
    finally { inFlight.current = false; setBusy(false); }
  };
  const dismiss = async () => {
    const original = current.current;
    if (!original || !scope || inFlight.current) return false;
    inFlight.current = true; setBusy(true); setError('');
    try {
      // Even a not-yet-observed request must be fenced against delayed arrival.
      const result = checkedCoordinationSendOutcome(await api.dismissCoordinationSend(original.id, original.scope), original.id, original.scope);
      if (!['completed', 'refused', 'uncertain', 'cancelled'].includes(result.requestState || '')) throw new Error('The original request is not fenced. Keep its saved identity.');
      if (original.input) returned(original.input.target, { ...result, target: original.input.target });
      sessionStorage.removeItem(storageKey(scope));
      if (sessionStorage.getItem(storageKey(scope))) throw new Error('Cannot clear the browser instruction identity. Nothing new was sent.');
      save(null); setReady(true); return true;
    } catch (reason) { setError(String(reason)); return false; }
    finally { inFlight.current = false; setBusy(false); }
  };
  return { ready, busy, error, pending, send, inspect, continueOriginal, dismiss,
    canContinue: !!pending?.input && (!pending.outcome || ['pending', 'unobserved'].includes(pending.outcome.requestState || '')) };
}
