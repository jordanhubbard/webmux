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
  const [checking, setChecking] = useState(true);
  const [refresh, setRefresh] = useState(0);
  const [attachment, setAttachment] = useState(0);
  const connected = session?.state === 'connected';
  const reconnectable = session && (session.state === 'disconnected' || session.state === 'error');
  const connect = () => {
    setBusy(true); setError('');
    void api.attachOpenRig().then(value => { setSession(value); setAttachment(value => value + 1); })
      .catch(err => setError(String(err))).finally(() => setBusy(false));
  };
  useEffect(() => {
    let active = true;
    setChecking(true);
    void api.getOpenRig().then(status => {
      if (active) { setAvailable(status.available); setSession(status.session); setError(status.message); }
    }).catch(err => { if (active) { setAvailable(false); setError(String(err)); } })
      .finally(() => { if (active) setChecking(false); });
    return () => { active = false; };
  }, [refresh]);
  return <section className="openrig-workspace" aria-label="OpenRig dashboard">
    <header><strong>Rig diagnostics</strong>
      {session && <span role="status">{connected ? 'Dashboard connected' : session.state === 'connecting' ? 'Dashboard connecting…' : 'Dashboard disconnected'}</span>}
      {reconnectable && <button disabled={busy || !available || checking} onClick={connect}>{busy ? 'Reconnecting dashboard…' : 'Reconnect dashboard'}</button>}
      {session && <button disabled={busy} onClick={() => {
        setBusy(true);
        void api.deleteSession(session.id).then(() => setSession(null)).catch(err => setError(String(err))).finally(() => setBusy(false));
      }}>Detach dashboard</button>}
      {poppedOut && <button onClick={() => window.close()}>Close view</button>}
    </header>
    <p>Provider: <strong>Rig</strong>. This dashboard shows Rig status. For Agent Deck sessions, select Agent Deck in Coordination in the main WebMux window.</p>
    {error && <p role="alert">{error}</p>}
    {!session && <div className="openrig-guidance">
      <h2>Open Rig diagnostics</h2>
      <p>This view connects the live Rig text dashboard. It does not create coding agents.</p>
      <button disabled={!available || busy || checking} onClick={connect}>{busy ? 'Connecting dashboard…' : 'Connect dashboard'}</button>
      <button disabled={busy || checking} onClick={() => setRefresh(value => value + 1)}>Refresh connection</button>
      {checking && <p role="status">Checking the Rig coordinator…</p>}
      {!checking && !available && !error && <p>Rig is unavailable. Check the coordinator, then refresh the connection.</p>}
      <p>For projects, workers and their terminal tiles, open <strong>Coordination</strong> in the main WebMux window.</p>
    </div>}
    {session && <div className="openrig-terminal" data-testid="openrig-terminal" data-session-id={session.id}>
      <InputBroadcastProvider><Terminal key={`${session.id}:${attachment}`} sessionId={session.id} fontSize={fontSize} fontFamily={fontFamily} state={session.state}
        autoScroll onStateChange={state => setSession(value => value ? { ...value, state } : value)} onViewerUpdate={() => {}} onFocusGained={() => {}} /></InputBroadcastProvider>
    </div>}
    <footer>Detaching this dashboard leaves the coordinator and agents running.</footer>
  </section>;
}
