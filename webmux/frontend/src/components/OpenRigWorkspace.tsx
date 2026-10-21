import { useEffect, useRef, useState } from 'react';
import { InputBroadcastProvider } from '../contexts/InputBroadcastContext';
import { api } from '../utils/api';
import type { Session } from '../types';
import { Terminal } from './Terminal';
import './OpenRigWorkspace.css';

export function OpenRigWorkspace({ fontSize, fontFamily, poppedOut = false }: {
  fontSize: number; fontFamily?: string; poppedOut?: boolean;
}) {
  const [status, setStatus] = useState<Awaited<ReturnType<typeof api.getOpenRig>> | null>(null);
  const [session, setSession] = useState<Session | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [refreshVersion, setRefreshVersion] = useState(0);
  const operation = useRef(0);

  useEffect(() => {
    let active = true;
    const refresh = () => {
      const version = operation.current;
      api.getOpenRig().then(value => {
        if (!active || version !== operation.current) return;
        setStatus(value);
        setSession(value.session);
        setError('');
      }).catch(err => { if (active && version === operation.current) setError((err as Error).message); });
    };
    refresh();
    window.addEventListener('focus', refresh);
    return () => { active = false; window.removeEventListener('focus', refresh); };
  }, [refreshVersion]);

  const connect = async () => {
    operation.current++;
    setBusy(true);
    setError('');
    try { setSession(await api.attachOpenRig()); }
    catch (err) { setError((err as Error).message); }
    finally { setBusy(false); }
  };

  const disconnect = async () => {
    if (!session) return;
    operation.current++;
    setBusy(true);
    try { await api.deleteSession(session.id); setSession(null); setError(''); }
    catch (err) { setError((err as Error).message); }
    finally { setBusy(false); }
  };

  const popOut = () => {
    const url = new URL(window.location.href);
    url.searchParams.set('panel', 'openrig');
    const popup = window.open(url.toString(), 'webmux-openrig', 'popup,width=1200,height=800');
    if (popup) popup.opener = null;
    else setError('Allow pop-ups for WebMux to open the OpenRig dashboard in a separate window.');
  };

  return (
    <section className="openrig-workspace" aria-label="OpenRig dashboard">
      <header>
        <strong>OpenRig <small>Prototype · this WebMux host</small></strong>
        <button onClick={() => setRefreshVersion(value => value + 1)} disabled={busy}>Check again</button>
        <button onClick={connect} disabled={busy || !status?.available || session?.state === 'connected' || session?.state === 'connecting'}>
          {busy ? 'Please wait…' : session?.state === 'connected' ? 'Dashboard connected' : session ? 'Reconnect dashboard' : 'Connect dashboard'}
        </button>
        {session && <button onClick={disconnect} disabled={busy}>Detach dashboard</button>}
        {!poppedOut && <button onClick={popOut}>Pop out</button>}
        {poppedOut && <button onClick={() => window.close()}>Close view</button>}
      </header>
      {error && <p role="alert">{error}</p>}
      {status && !status.available && <p role="status">{status.message} <a href="https://github.com/mvschwarz/openrig" target="_blank" rel="noreferrer">OpenRig setup guide</a></p>}
      {!status && !error && <p role="status">Checking OpenRig availability…</p>}
      {!session && <div className="openrig-guidance">
        <h2>Agent coordination alongside your terminals</h2>
        <p>Connect to an existing OpenRig shared dashboard running as the WebMux service user on this host. Set up OpenRig and start its dashboard there first.</p>
        <p>This panel attaches with <code>rig tui --shared</code>. The dashboard shows team status and navigation; agent conversations remain in their own terminals.</p>
        <p>Agents need OpenRig integration to coordinate. Existing SSH/tmux sessions remain independently accessible.</p>
      </div>}
      {session && <div className="openrig-terminal" data-testid="openrig-terminal" data-session-id={session.id}>
        <InputBroadcastProvider>
          <Terminal key={session.id} sessionId={session.id} fontSize={fontSize} fontFamily={fontFamily}
            state={session.state} autoScroll onStateChange={state => setSession(prev => prev ? { ...prev, state } : prev)}
            onViewerUpdate={() => {}} onFocusGained={() => {}} />
        </InputBroadcastProvider>
      </div>}
      <footer>Switching panels or closing a view leaves OpenRig running. Detach dashboard closes only WebMux’s viewer client. This dashboard is separate from Type to All.</footer>
    </section>
  );
}
