import { useCallback, useEffect, useRef, useState } from 'react';
import { Terminal } from './Terminal';
import { Dialog } from './Dialog';
import { api, type ConnectedServer } from '../utils/api';
import { sessionIdentity } from '../utils/sessionIdentity';
import type { ConnectionState, NamedTheme, Session } from '../types';
import './ServerWorkspace.css';

type Snapshot = { sessions: Session[]; error?: string };
interface Props { servers: ConnectedServer[]; selected: string; fontSize: number; fontFamily?: string; themes: NamedTheme[]; globalTheme: string | null }

function ServerTerminal({ server, session, fontSize, fontFamily, theme, onHide, onError }: {
  server: ConnectedServer; session: Session; fontSize: number; fontFamily?: string; theme?: NamedTheme['theme']; onHide: () => void; onError: (error: string) => void;
}) {
  const [connection, setConnection] = useState<ConnectionState>('connecting');
  const [providerState, setProviderState] = useState(session.state);
  useEffect(() => { setProviderState(session.state); }, [session.state]);
  const [generation, setGeneration] = useState(0);
  const [message, setMessage] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const [notice, setNotice] = useState('');
  const identity = sessionIdentity(session);
  const title = `${server.name} · ${identity.label}`;
  return <section className="server-terminal" aria-label={title} data-testid={`server-terminal-${server.id}-${session.id}`}>
    <header>
      <strong title={`${title} · ${identity.source}`}>{title}</strong>
      <span title="Provider state reported by the owning server">{providerState}</span>
      <span title="Browser viewer connection">Viewer: {connection}</span>
      <button onClick={() => { setConnection('connecting'); setGeneration(value => value + 1); }} title="Reconnect this viewer without restarting its provider">Reconnect viewer</button>
      {providerState !== 'connected' && providerState !== 'connecting' && <button onClick={() => {
        if (window.confirm(`Restart the provider for ${title}? This starts a new terminal process on ${server.name}.`)) void api.reconnectServerSession(server.id, session.id).catch(error => onError(String(error)));
      }}>Restart provider</button>}
      <button onClick={() => { setMessage(''); setNotice(''); }}>Message</button>
      <button onClick={onHide} title="Hide this viewer; keep the session running on its server">Hide</button>
    </header>
    <div className="server-terminal-body"><Terminal key={generation} sessionId={`@${server.id}/${session.id}`} fontSize={fontSize} fontFamily={fontFamily} state={providerState} autoScroll onStateChange={setProviderState} onTransportChange={connected => setConnection(connected ? 'connected' : 'disconnected')} onViewerUpdate={() => {}} onFocusGained={() => { document.title = `${title} — WebMux`; }} theme={theme} /></div>
    {message !== null && <Dialog title={`Message ${identity.name}`} subtitle={`Send one line as terminal input to ${server.name}. Enter is submitted; use only with an agent ready to receive a prompt.`} onClose={() => { if (!sending) setMessage(null); }}>
      <form onSubmit={event => {
        event.preventDefault(); setSending(true); setNotice('');
        void api.sendServerMessage(server.id, session.id, message).then(() => { setNotice('Queued as terminal input. Agent processing is not confirmed.'); setMessage(''); }).catch(error => setNotice(String(error))).finally(() => setSending(false));
      }}>
        <label>Message<input aria-label="Message" data-dialog-autofocus value={message} disabled={sending} maxLength={16384} onChange={event => setMessage(event.target.value)} /></label>
        <button type="submit" disabled={sending || !message.trim()}>Send to this session</button>
        {notice && <p role="status">{notice}</p>}
      </form>
    </Dialog>}
  </section>;
}

export function ServerWorkspace({ servers, selected, fontSize, fontFamily, themes, globalTheme }: Props) {
  const [snapshots, setSnapshots] = useState<Record<string, Snapshot>>({});
  const [hidden, setHidden] = useState<Set<string>>(() => new Set());
  const [error, setError] = useState('');
  const [creating, setCreating] = useState<string | null>(null);
  const mounted = useRef(true);
  const targets = servers.filter(server => selected === 'all' || server.id === selected);
  const targetKey = targets.map(server => server.id).join(',');
  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);
  const refresh = useCallback(async (ids: string[], valid: () => boolean) => {
    await Promise.all(ids.map(async id => {
      try {
        const sessions = await api.getServerSessions(id);
        if (valid()) setSnapshots(previous => ({ ...previous, [id]: { sessions } }));
      } catch (error) {
        if (valid()) setSnapshots(previous => ({ ...previous, [id]: { sessions: previous[id]?.sessions || [], error: String(error) } }));
      }
    }));
  }, []);
  useEffect(() => {
    let canceled = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      await refresh(targetKey.split(',').filter(Boolean), () => !canceled);
      if (!canceled) timer = setTimeout(() => { void poll(); }, 5000);
    };
    void poll();
    return () => { canceled = true; clearTimeout(timer); };
  }, [targetKey, refresh]);
  const theme = themes.find(theme => theme.name === globalTheme)?.theme;
  return <div className="server-workspace">
    {error && <p role="alert">{error} <button onClick={() => setError('')}>Dismiss</button></p>}
    {targets.map(server => {
      const snapshot = snapshots[server.id];
      return <section key={server.id} aria-label={`${server.name} sessions`}>
        <div className="server-heading"><strong>{server.name}</strong>
          {snapshot?.error ? <span role="alert">Unavailable · {snapshot.error}. Session status may be stale.</span> : <span>{snapshot ? `${snapshot.sessions.length} sessions` : 'Connecting…'}</span>}
          <button disabled={creating !== null || !!snapshot?.error} onClick={() => {
            setCreating(server.id); setError('');
            void api.createServerSession(server.id).then(() => refresh([server.id], () => mounted.current)).catch(error => { if (mounted.current) setError(String(error)); }).finally(() => { if (mounted.current) setCreating(null); });
          }}>New terminal on {server.name}</button>
          {[...hidden].some(key => key.startsWith(`${server.id}/`)) && <button onClick={() => setHidden(previous => new Set([...previous].filter(key => !key.startsWith(`${server.id}/`))))}>Show hidden viewers</button>}
        </div>
        {snapshot && !snapshot.error && snapshot.sessions.length === 0 && <p>No terminal sessions on this server.</p>}
        <div className="server-terminal-grid">{snapshot?.sessions.filter(session => !hidden.has(`${server.id}/${session.id}`)).map(session => <ServerTerminal key={session.id} server={server} session={session} fontSize={fontSize} fontFamily={fontFamily} theme={theme} onError={setError} onHide={() => setHidden(previous => new Set(previous).add(`${server.id}/${session.id}`))} />)}</div>
      </section>;
    })}
  </div>;
}
