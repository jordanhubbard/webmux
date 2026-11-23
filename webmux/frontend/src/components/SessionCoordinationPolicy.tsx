import { useCallback, useEffect, useRef, useState } from 'react';
import { api, type SessionCoordinationPolicy as Policy } from '../utils/api';

export function SessionCoordinationPolicy() {
  const [policy, setPolicy] = useState<Policy | null>(null);
  const [phase, setPhase] = useState<'loading' | 'saving' | null>('loading');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const active = useRef<AbortController | null>(null);
  const reconcile = useCallback(async (enabled?: boolean) => {
    active.current?.abort();
    const controller = new AbortController();
    active.current = controller;
    setPhase(enabled === undefined ? 'loading' : 'saving');
    setError(''); setNotice('');
    try {
      if (enabled !== undefined) await api.setSessionCoordinationPolicy(enabled, controller.signal);
      const current = await api.getSessionCoordinationPolicy(controller.signal);
      if (controller.signal.aborted) return;
      setPolicy(current);
      if (enabled !== undefined) setNotice(current.enabled === enabled
        ? `Automatic coordination is ${current.enabled ? 'enabled' : 'disabled'} on this server.`
        : 'The current server setting differs from the requested change.');
    } catch {
      if (controller.signal.aborted) return;
      setPolicy(null);
      setError(enabled === undefined
        ? 'Cannot load the coordination policy. Reload to check its current setting.'
        : 'The coordination change could not be confirmed. Reload to check the current server setting.');
    } finally {
      if (!controller.signal.aborted) setPhase(null);
    }
  }, []);
  useEffect(() => {
    void reconcile();
    return () => active.current?.abort();
  }, [reconcile]);

  return <section aria-label="Automatic session coordination">
    <h3>Automatic session coordination</h3>
    <p>Share verified project context, task claims, handoffs and worker reservations across authorized sessions. This policy applies to the whole server.</p>
    <p>Turning coordination off leaves terminal sessions and their agents running.</p>
    {policy && <label className="servers-check"><input type="checkbox" checked={policy.enabled} disabled={phase !== null}
      onChange={event => { void reconcile(event.target.checked); }} />Enable automatic session coordination</label>}
    {error && <p role="alert">{error}</p>}
    {phase && <p role="status">{phase === 'saving' ? 'Saving coordination policy…' : 'Loading coordination policy…'}</p>}
    {notice && <p role="status">{notice}</p>}
    <button type="button" disabled={phase !== null} onClick={() => { void reconcile(); }}>Reload coordination policy</button>
  </section>;
}
