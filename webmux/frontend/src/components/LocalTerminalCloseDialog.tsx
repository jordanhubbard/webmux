import { useEffect, useState } from 'react';
import { Dialog } from './Dialog';
import { api } from '../utils/api';
import type { Session } from '../types';

function stopID() {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 15) | 64; bytes[8] = (bytes[8] & 63) | 128;
  const hex = Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

export function LocalTerminalCloseDialog({ session, onClose, onChanged }: {
  session: Session; onClose: () => void; onChanged: () => void;
}) {
  const [reviewed, setReviewed] = useState<Session | null>(null);
  const [action, setAction] = useState<{ id: string; selection: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [attempted, setAttempted] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  useEffect(() => {
    let disposed = false;
    api.getSession(session.id).then(value => {
      if (disposed) return;
      setReviewed(value);
      const lifecycle = value.local_lifecycle;
      if (lifecycle?.contract !== 'webmux-local-provider-lifecycle/v1') {
        setError('The terminal lifecycle changed. Keep this window and review its current owner.');
        return;
      }
      if (lifecycle.selection) setAction({ id: lifecycle.operation_id || stopID(), selection: lifecycle.selection });
      setAttempted(!!lifecycle.operation_id);
    }).catch(err => { if (!disposed) setError(String(err)); });
    return () => { disposed = true; };
  }, [session.id]);
  const hide = async () => {
    setBusy(true); setError('');
    try { await api.setMinimized(session.id, true); onChanged(); onClose(); }
    catch (err) { setError(String(err)); }
    finally { setBusy(false); }
  };
  const closeStopped = async () => {
    await api.deleteSession(session.id); onChanged(); onClose();
  };
  const stop = async (mode: 'stop' | 'inspect' | 'continue') => {
    if (!action || busy) return;
    setBusy(true); setError(''); setAttempted(true);
    try {
      const result = await api.stopLocalSession(session.id, { ...action, mode, confirm: mode !== 'inspect' });
      if (result.session.id !== session.id || result.session.local_lifecycle?.operation_id !== action.id || result.session.local_lifecycle.selection !== action.selection) {
        throw new Error('The stop response does not identify the original request. Keep this window and inspect it.');
      }
      setReviewed(result.session);
      if (result.outcome === 'stopped') await closeStopped();
      else { setMessage(result.message || 'Stop is not confirmed. Inspect the original stop before continuing it.'); onChanged(); }
    } catch (err) { setError(`Stop is not confirmed. ${String(err)}`); onChanged(); }
    finally { setBusy(false); }
  };
  return <Dialog title="Close local terminal" subtitle={session.title} onClose={onClose} dismissible={!busy} width={520}>
    <p>{attempted ? 'Hiding keeps this session available in the window list. It does not cancel a stop already requested.' : 'Hide this window to keep its work running. Restore it from the window list whenever you need it.'}</p>
    <p>Stop ends the work in this terminal and disconnects all its viewers. Other terminals keep running.</p>
    {message && <p role="status">{message}</p>}
    {error && <p role="alert">{error}</p>}
    {reviewed?.local_lifecycle?.state === 'unavailable' && <p>The original provider cannot be verified. Keep this window and reconnect to review it before stopping.</p>}
    <p><button disabled={busy} onClick={() => void hide()}>{attempted ? 'Hide this window' : 'Hide and keep running'}</button></p>
    {!attempted && <p><button disabled={busy || !action} onClick={() => void stop('stop')}>Stop terminal and close</button></p>}
    {attempted && reviewed?.local_lifecycle?.state !== 'stopped' && <>
      <p>The original request is retained. Checking its outcome does not send another Stop.</p>
      <p><button disabled={busy || !action} onClick={() => void stop('inspect')}>Inspect original stop</button></p>
      {reviewed?.local_lifecycle?.state === 'uncertain' && <p><button disabled={busy || !action} onClick={() => void stop('continue')}>Continue original stop</button></p>}
    </>}
    {reviewed?.local_lifecycle?.state === 'stopped' && <p><button disabled={busy} onClick={() => {
      setBusy(true); void closeStopped().catch(err => setError(String(err))).finally(() => setBusy(false));
    }}>Close stopped terminal</button></p>}
    <p><button disabled={busy} onClick={onClose}>Cancel</button></p>
  </Dialog>;
}
