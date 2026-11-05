import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { Terminal } from './Terminal';
import { Dialog } from './Dialog';
import { WorkspaceOverview, type WorkspaceOverviewTile } from './WorkspaceMinimap';
import { api, type ConnectedServer } from '../utils/api';
import { sessionIdentity } from '../utils/sessionIdentity';
import type { ConnectionState, NamedTheme, Session } from '../types';
import './ServerWorkspace.css';

type Snapshot = { sessions: Session[]; error?: string };
interface Props { servers: ConnectedServer[]; selected: string; fontSize: number; fontFamily?: string; themes: NamedTheme[]; globalTheme: string | null; onManageServers?: () => void; onRefreshDirectory?: () => Promise<void>; directoryRevision?: number }

function ServerTerminal({ server, session, fontSize, fontFamily, theme, onHide, onError, reconnectVersion }: {
  reconnectVersion: string; server: ConnectedServer; session: Session; fontSize: number; fontFamily?: string; theme?: NamedTheme['theme']; onHide: () => void; onError: (error: string) => void;
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
  return <section className="server-terminal" aria-label={title} data-overview-id={`${server.id}/${session.id}`} data-overview-state={session.state} data-testid={`server-terminal-${server.id}-${session.id}`}>
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
    <div className="server-terminal-body"><Terminal key={`${reconnectVersion}/${generation}`} sessionId={`@${server.id}/${session.id}`} fontSize={fontSize} fontFamily={fontFamily} state={providerState} autoScroll onStateChange={setProviderState} onTransportChange={connected => setConnection(connected ? 'connected' : 'disconnected')} onViewerUpdate={() => {}} onFocusGained={() => { document.title = `${title} — WebMux`; }} theme={theme} /></div>
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

export function ServerWorkspace({ servers, selected, fontSize, fontFamily, themes, globalTheme, onManageServers, onRefreshDirectory, directoryRevision = 0 }: Props) {
  const [reconnectVersions, setReconnectVersions] = useState<Record<string, number>>({});
  const [refreshing, setRefreshing] = useState(false);
  const refreshRequests = useRef<Record<string, number>>({});
  const [snapshots, setSnapshots] = useState<Record<string, Snapshot>>({});
  const [hidden, setHidden] = useState<Set<string>>(() => new Set());
  const [error, setError] = useState('');
  const [packed, setPacked] = useState(() => sessionStorage.getItem('webmux-server-packed') === 'true');
  const [creating, setCreating] = useState<string | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const [overview, setOverview] = useState({ tiles: [] as WorkspaceOverviewTile[], width: 0, height: 0 });
  const mounted = useRef(true);
  const targets = servers.filter(server => selected === 'all' || server.id === selected);
  const targetKey = targets.map(server => server.id).join(',');
  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);
  const refresh = useCallback(async (ids: string[], valid: () => boolean) => {
    await Promise.all(ids.map(async id => {
      const request = (refreshRequests.current[id] || 0) + 1;
      refreshRequests.current[id] = request;
      const current = () => valid() && refreshRequests.current[id] === request;
      try {
        const sessions = await api.getServerSessions(id);
        if (current()) setSnapshots(previous => ({ ...previous, [id]: { sessions } }));
      } catch (error) {
        if (current()) setSnapshots(previous => ({ ...previous, [id]: { sessions: previous[id]?.sessions || [], error: String(error) } }));
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
  }, [targetKey, refresh, directoryRevision]);
  // Read actual CSS grid geometry for both responsive grouped and packed views.
  useLayoutEffect(() => {
    const container = scrollRef.current;
    if (!container) return;
    const measure = () => {
      const origin = container.getBoundingClientRect();
      const tiles = Array.from(container.querySelectorAll<HTMLElement>('.server-terminal')).map(element => {
        const rect = element.getBoundingClientRect();
        return {
          id: element.dataset.overviewId!,
          title: element.getAttribute('aria-label') || '',
          state: element.dataset.overviewState as ConnectionState,
          x: rect.left - origin.left + container.scrollLeft,
          y: rect.top - origin.top + container.scrollTop,
          width: rect.width,
          height: rect.height,
        };
      });
      setOverview({ tiles, width: container.scrollWidth, height: container.scrollHeight });
    };
    measure();
    const observer = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(measure) : null;
    observer?.observe(container);
    // Content can grow without changing the scroll container's own size.
    container.querySelectorAll('.server-terminal-grid, .server-heading').forEach(element => observer?.observe(element));
    window.addEventListener('resize', measure);
    return () => { observer?.disconnect(); window.removeEventListener('resize', measure); };
  }, [snapshots, hidden, targetKey, packed]);
  const refreshAndReconnect = async (ids: string[], reloadDirectory = false) => {
    setRefreshing(true); setError('');
    try {
      if (reloadDirectory) await onRefreshDirectory?.();
      await refresh(ids, () => mounted.current);
      if (mounted.current) setReconnectVersions(previous => {
        const next = { ...previous };
        for (const id of ids) next[id] = (next[id] || 0) + 1;
        return next;
      });
    } catch (error) { if (mounted.current) setError(String(error)); }
    finally { if (mounted.current) setRefreshing(false); }
  };
  const theme = themes.find(theme => theme.name === globalTheme)?.theme;
  const terminals = (server: ConnectedServer) => snapshots[server.id]?.sessions
    .filter(session => !hidden.has(`${server.id}/${session.id}`))
    .sort((a, b) => a.row - b.row || a.col - b.col || a.id.localeCompare(b.id))
    .map(session => <ServerTerminal reconnectVersion={`${directoryRevision}/${reconnectVersions[server.id] || 0}`} key={`${server.id}/${session.id}`} server={server} session={session} fontSize={fontSize} fontFamily={fontFamily} theme={theme} onError={setError} onHide={() => setHidden(previous => new Set(previous).add(`${server.id}/${session.id}`))} />);
  return <div className="server-workspace-container">
    <div ref={scrollRef} className={`server-workspace${packed ? ' server-workspace-packed' : ''}`}>
      <div className="server-layout-controls" role="toolbar" aria-label="Terminal layout">
        <button disabled={refreshing} onClick={() => { void refreshAndReconnect(targets.map(server => server.id), true); }}>Refresh servers</button>
        <button disabled={packed || !targets.some(server => snapshots[server.id]?.sessions.length)} title="Pack visible terminals from all selected servers into one matrix" onClick={() => { setPacked(true); sessionStorage.setItem('webmux-server-packed', 'true'); }}>Pack</button>
      </div>
      {error && <p role="alert">{error} <button onClick={() => setError('')}>Dismiss</button></p>}
      {targets.map(server => {
        const snapshot = snapshots[server.id];
        return <section key={server.id} aria-label={`${server.name} sessions`}>
          <div className="server-heading"><strong>{server.name}</strong>
            {snapshot?.error ? <span role="alert">Unavailable · {snapshot.error}. Session status may be stale. {onManageServers && <button onClick={onManageServers}>Manage connection to {server.name}</button>}</span> : <span>{snapshot ? `${snapshot.sessions.length} sessions` : 'Connecting…'}</span>}
            <button disabled={refreshing} title="Reload sessions and reconnect viewers; keep terminal processes running" onClick={() => { void refreshAndReconnect([server.id]); }}>Refresh {server.name}</button>
            <button disabled={creating !== null || !!snapshot?.error} onClick={() => {
              setCreating(server.id); setError('');
              void api.createServerSession(server.id).then(() => refresh([server.id], () => mounted.current)).catch(error => { if (mounted.current) setError(String(error)); }).finally(() => { if (mounted.current) setCreating(null); });
            }}>New terminal on {server.name}</button>
            {[...hidden].some(key => key.startsWith(`${server.id}/`)) && <button onClick={() => setHidden(previous => new Set([...previous].filter(key => !key.startsWith(`${server.id}/`))))}>Show hidden viewers</button>}
          </div>
          {snapshot && !snapshot.error && snapshot.sessions.length === 0 && <p>No terminal sessions on this server.</p>}
          {!packed && <div className="server-terminal-grid">{terminals(server)}</div>}
        </section>;
      })}
      {packed && <div className="server-terminal-grid" aria-label="Server terminal matrix">{targets.flatMap(server => terminals(server) || [])}</div>}
    </div>
    <WorkspaceOverview scrollRef={scrollRef} tiles={overview.tiles} gridW={overview.width} gridH={overview.height} />
  </div>;
}
