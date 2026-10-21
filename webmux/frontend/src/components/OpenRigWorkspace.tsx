import { useEffect, useState } from 'react';
import { InputBroadcastProvider } from '../contexts/InputBroadcastContext';
import { api } from '../utils/api';
import type { Session } from '../types';
import { Terminal } from './Terminal';
import './OpenRigWorkspace.css';

// Optional raw dashboard for diagnostics. Everyday agents use terminal tiles.
export function OpenRigWorkspace({ fontSize, fontFamily, poppedOut = false }: {
  fontSize: number; fontFamily?: string; poppedOut?: boolean;
}) {
  const [session, setSession] = useState<Session | null>(null);
  const [available, setAvailable] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let active = true;
    void api.getOpenRig().then(status => { if (active) { setAvailable(status.available); setSession(status.session); setError(status.message); } }).catch(err => { if (active) setError(String(err)); });
    return () => { active = false; };
  }, []);
  return <section className="openrig-workspace" aria-label="OpenRig dashboard">
    <header><strong>OpenRig diagnostics</strong>
      <button disabled={!available || busy || !!session} onClick={() => {
        setBusy(true);
        void api.attachOpenRig().then(setSession).catch(err => setError(String(err))).finally(() => setBusy(false));
      }}>{session ? 'Dashboard connected' : 'Connect dashboard'}</button>
      {session && <button disabled={busy} onClick={() => {
        setBusy(true);
        void api.deleteSession(session.id).then(() => setSession(null)).catch(err => setError(String(err))).finally(() => setBusy(false));
      }}>Detach dashboard</button>}
      {poppedOut && <button onClick={() => window.close()}>Close view</button>}
    </header>
    {error && <p role="alert">{error}</p>}
    {!session && <p>Connect the raw Rig text dashboard for diagnostics. Use Coordination in the main WebMux window to create agents and open their terminal tiles.</p>}
    {session && <div className="openrig-terminal" data-testid="openrig-terminal" data-session-id={session.id}>
      <InputBroadcastProvider><Terminal sessionId={session.id} fontSize={fontSize} fontFamily={fontFamily} state={session.state}
        autoScroll onStateChange={state => setSession(value => value ? { ...value, state } : value)} onViewerUpdate={() => {}} onFocusGained={() => {}} /></InputBroadcastProvider>
    </div>}
    <footer>Detaching this dashboard leaves the coordinator and agents running.</footer>
  </section>;
}
