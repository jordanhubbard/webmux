import { useEffect, useRef, useState } from 'react';
import { InputBroadcastProvider } from '../contexts/InputBroadcastContext';
import { api } from '../utils/api';
import type { Session } from '../types';
import { Terminal } from './Terminal';
import './OpenRigWorkspace.css';

const views = { dashboard: 'Dashboard', proof: 'Live test console', alpha: 'Claude', beta: 'Codex' };
type View = keyof typeof views | `agent:${string}`;
const label = (view: View) => view.startsWith('agent:') ? view.slice(6) : views[view as keyof typeof views];
function initialView(): View {
  const value = new URL(window.location.href).searchParams.get('view');
  return value && (Object.prototype.hasOwnProperty.call(views, value) || value.startsWith('agent:')) ? value as View : 'dashboard';
}

export function OpenRigWorkspace({ fontSize, fontFamily, poppedOut = false }: {
  fontSize: number; fontFamily?: string; poppedOut?: boolean;
}) {
  const [status, setStatus] = useState<Awaited<ReturnType<typeof api.getOpenRig>> | null>(null);
  const [sessions, setSessions] = useState<Record<string, Session>>({});
  const [agents, setAgents] = useState<Awaited<ReturnType<typeof api.getOpenRigAgents>>>([]);
  const [agentError, setAgentError] = useState('');
  const [view, setView] = useState<View>(initialView);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [refreshVersion, setRefreshVersion] = useState(0);
  const operation = useRef(0);
  const session = sessions[view];

  useEffect(() => {
    let active = true;
    const refresh = () => {
      const version = operation.current;
      api.getOpenRig().then(value => {
        if (!active || version !== operation.current) return;
        setStatus(value);
        setSessions(value.views ?? (value.session ? { dashboard: value.session } : {}));
      }).catch(err => { if (active && version === operation.current) setError((err as Error).message); });
    };
    refresh();
    const timer = window.setInterval(refresh, 2500);
    window.addEventListener('focus', refresh);
    return () => { active = false; clearInterval(timer); window.removeEventListener('focus', refresh); };
  }, [refreshVersion]);

  useEffect(() => {
    if (!status?.available) return;
    let active = true;
    api.getOpenRigAgents().then(value => { if (active) { setAgents(value); setAgentError(''); } })
      .catch(err => { if (active) setAgentError((err as Error).message); });
    return () => { active = false; };
  }, [status?.available, status?.proof?.phase, refreshVersion]);

  const connect = async (target: View = view) => {
    operation.current++;
    setBusy(true);
    setError('');
    setView(target);
    if (target === 'proof') setStatus(previous => previous ? { ...previous, proof: { phase: 'preparing', run_id: '', message: 'Starting a fresh challenge…' } } : previous);
    try {
      const value = target.startsWith('agent:') ? await api.attachOpenRig('agent', target.slice(6)) : await api.attachOpenRig(target);
      setSessions(previous => ({ ...previous, [target]: value }));
    } catch (err) { setError((err as Error).message); }
    finally { setBusy(false); setRefreshVersion(value => value + 1); }
  };

  const disconnect = async () => {
    if (!session) return;
    operation.current++;
    setBusy(true);
    try {
      await api.deleteSession(session.id);
      setSessions(previous => { const next = { ...previous }; delete next[view]; return next; });
      setError('');
    } catch (err) { setError((err as Error).message); }
    finally { setBusy(false); }
  };

  const stop = async () => {
    operation.current++;
    setBusy(true); setError('');
    try { await api.stopOpenRigProof(); }
    catch (err) { setError((err as Error).message); }
    finally { setBusy(false); setRefreshVersion(value => value + 1); }
  };

  const popOut = () => {
    const url = new URL(window.location.href);
    url.searchParams.set('panel', 'openrig');
    url.searchParams.set('view', view);
    const popup = window.open(url.toString(), `webmux-openrig-${view}`, 'popup,width=1200,height=800');
    if (popup) popup.opener = null;
    else setError('Allow pop-ups for WebMux to open the OpenRig dashboard in a separate window.');
  };
  const active = session?.state === 'connected' || session?.state === 'connecting';
  const running = (sessions.proof?.state === 'connected' || sessions.proof?.state === 'connecting') && (status?.proof?.phase === 'preparing' || status?.proof?.phase === 'waiting');

  return (
    <section className="openrig-workspace" aria-label="OpenRig dashboard">
      <header>
        <strong>OpenRig <small>This WebMux host · interactive text UI</small></strong>
        <button onClick={() => setRefreshVersion(value => value + 1)} disabled={busy}>Check again</button>
        <button onClick={() => void connect()} disabled={busy || !status?.available || active || view === 'proof'}>
          {busy ? 'Please wait…' : active ? `${label(view)} connected` : session ? `Reconnect ${label(view).toLowerCase()}` : `Connect ${label(view).toLowerCase()}`}
        </button>
        {session && <button onClick={disconnect} disabled={busy || (view === 'proof' && running)}>Detach {label(view).toLowerCase()}</button>}
        {!poppedOut && <button onClick={popOut}>Pop out</button>}
        {poppedOut && <button onClick={() => window.close()}>Close view</button>}
      </header>
      <nav aria-label="OpenRig views">
        {(Object.keys(views) as (keyof typeof views)[]).map(key => <button key={key} aria-pressed={view === key} onClick={() => setView(key)}>{views[key]}</button>)}
      {agents.map(agent => <button key={agent.target} aria-pressed={view === `agent:${agent.target}`} onClick={() => setView(`agent:${agent.target}`)} title={`${agent.runtime} · ${agent.status}`}>{agent.target}</button>)}
      </nav>
      {agentError && <p className="openrig-agent-error">{agentError}</p>}
      <div className="openrig-proof-controls">
        <button onClick={() => void connect('proof')} disabled={busy || !status?.available || running}>Run {status?.proof ? 'fresh challenge' : 'live test'}</button>
        <button onClick={() => void stop()} disabled={busy || !status?.proof || status.proof.phase === 'stopped'}>Stop test agents</button>
        <span>Starts a named Claude/Codex pair using the host’s configured accounts.</span>
      </div>
      {status?.proof && <details className="openrig-proof-result" open={status.proof.phase === 'passed'}>
        <summary>{status.proof.phase.toUpperCase()}: {status.proof.message}</summary>
        <dl>{(['run_id', 'nonce_a', 'nonce_b', 'digest'] as const).map(key => status.proof?.[key] && <div key={key}><dt>{key}</dt><dd>{status.proof[key]}</dd></div>)}</dl>
      </details>}
      {error && <p role="alert">{error}</p>}
      {status && !status.available && <p role="status">{status.message} <a href="https://github.com/mvschwarz/openrig" target="_blank" rel="noreferrer">OpenRig setup guide</a></p>}
      {!status && !error && <p role="status">Checking OpenRig availability…</p>}
      {!session && <div className="openrig-guidance">
        <h2>Agent coordination alongside your terminals</h2>
        {view === 'dashboard' ? <><p>Connect to the actual OpenRig text dashboard with <code>rig tui</code>. The host’s daemon and provider integration must be configured first.</p>
          <p>Pop out the dashboard, then open Claude and Codex here to watch and interact with the test pair. The dashboard shows real team status and navigation; conversations stay in the agent terminals.</p></> :
          <p>{view === 'proof' ? 'Run live test to start the pair and send a fresh challenge through OpenRig.' : view.startsWith('agent:') ? `Connect ${label(view)} to attach to its existing OpenRig CLI terminal.` : `Start the live test, then connect ${label(view)} to interact with its real CLI session. Pop out this view to watch it beside the dashboard.`}</p>}
        <p>Existing OpenRig agents appear as additional terminal tabs. Check again refreshes the list. The test pair remains available after PASS. You can ask either agent to send a message to its peer using rig send. Use Stop test agents when finished.</p>
      </div>}
      {session && <div className="openrig-terminal" data-testid="openrig-terminal" data-session-id={session.id}>
        <InputBroadcastProvider>
          <Terminal key={session.id} sessionId={session.id} fontSize={fontSize} fontFamily={fontFamily}
            state={session.state} autoScroll onStateChange={state => setSessions(previous => previous[view] ? { ...previous, [view]: { ...previous[view], state } } : previous)}
            onViewerUpdate={() => {}} onFocusGained={() => {}} />
        </InputBroadcastProvider>
      </div>}
      <footer>Closing a view leaves OpenRig and its agents running. Detach closes this viewer client. These views are separate from Type to All.</footer>
    </section>
  );
}
