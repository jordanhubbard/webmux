import { useCallback, useEffect, useState } from 'react';
import { Dialog } from './Dialog';
import './CoordinationDialog.css';
import { api } from '../utils/api';
import { useWorkspacePane } from '../contexts/WorkspacePaneContext';

export interface CoordinatedAgent {
  target: string; project: string; name: string; host: string; cwd: string;
  runtime: string; connected: boolean; sessionStatus?: string;
  activityState?: string;
}

export function CoordinationDialog({ onClose }: { onClose: () => void }) {
  const [agents, setAgents] = useState<CoordinatedAgent[]>([]);
  const [messages, setMessages] = useState<Awaited<ReturnType<typeof api.getCoordination>>['messages']>([]);
  const [statusError, setStatusError] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [project, setProject] = useState('');
  const [name, setName] = useState('');
  const [host, setHost] = useState('localhost');
  const [cwd, setCwd] = useState('');
  const [runtime, setRuntime] = useState('codex');
  const { setActivePane } = useWorkspacePane();
  const refresh = useCallback(async () => {
    try { const status = await api.getCoordination(); setAgents(status.agents); setMessages(status.messages); setStatusError(''); }
    catch (err) { setStatusError(String(err)); }
  }, []);
  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => { void refresh(); }, 5000);
    return () => window.clearInterval(timer);
  }, [refresh]);
  const revealTerminal = async (target: string) => {
    const session = await api.attachCoordinatedAgent(target);
    window.dispatchEvent(new CustomEvent('webmux-sessions-changed', { detail: { sessionId: session.id } }));
    setActivePane('terminals');
    onClose();
  };
  const openTerminal = async (target: string) => {
    setBusy(true);
    setError('');
    try {
      const agent = agents.find(a => a.target === target);
      if (agent && !agent.connected) await api.createCoordinatedAgent(agent);
      await revealTerminal(target);
    } catch (err) { setError(String(err)); }
    finally { setBusy(false); }
  };
  return <Dialog title="Rig" subtitle="One coordinator for your agents across hosts" onClose={onClose} width={800} dismissible={!busy}>
    <div className="coordination-dialog">
    <p>Agents run in your terminal grid. OpenRig shares their project identity and messages; project files stay on each host.</p>
    {statusError && <p role="alert" style={{ color: '#ff9b9b' }}>{statusError}</p>}
    {error && <p role="alert" style={{ color: '#ff9b9b', whiteSpace: 'pre-wrap' }}>{error}</p>}
    <button type="button" disabled={busy} onClick={() => void refresh()}>Refresh status</button>
    <div style={{ maxHeight: 280, overflow: 'auto', margin: '12px 0' }}>
      {agents.length === 0 && <p>No managed agents yet. Create an agent below to connect it automatically.</p>}
      {[...new Set(agents.map(agent => agent.project))].sort().map(projectId => <details className="coordination-project" key={projectId} open>
        <summary><strong>{projectId}</strong> · {agents.filter(agent => agent.project === projectId).length} workers</summary>
        <ul aria-label={`${projectId} workers`} className="coordination-workers">
          {agents.filter(agent => agent.project === projectId).map(agent => <li key={agent.target}>
            <article aria-label={`${agent.name} on ${agent.host}`}>
              <strong>{agent.name}</strong> <span className="coordination-rig">Rig-managed</span>
              <dl className="coordination-worker-facts">
                <div><dt>Execution host</dt><dd>{agent.host}</dd></div>
                <div><dt>Runtime</dt><dd>{agent.runtime}</dd></div>
                <div><dt>State</dt><dd>{agent.connected ? (agent.activityState || agent.sessionStatus || 'Connected; activity unknown') : 'Terminal stopped'}</dd></div>
              </dl>
              <div><code>{agent.cwd}</code></div>
              <details><summary>Messaging address</summary><code>{agent.target}</code></details>
              <button type="button" disabled={busy} onClick={() => void openTerminal(agent.target)}>{agent.connected ? 'Open terminal' : 'Reconnect agent'}</button>
            </article>
          </li>)}
        </ul>
      </details>)}
    </div>
    {messages.length > 0 && <details open><summary>Recent coordination</summary><div style={{ maxHeight: 180, overflow: 'auto' }}>{messages.map((message, index) => <div key={`${message.time}-${index}`} style={{ padding: '6px 0' }}><small>{new Date(message.time).toLocaleTimeString()} · {agents.find(a => a.target === message.from)?.name || message.from} → {agents.find(a => a.target === message.to)?.name || message.to} · recorded ({message.state})</small><pre style={{ whiteSpace: 'pre-wrap', fontSize: 12 }}>{message.body.split('\n---\n')[1] ?? message.body}</pre></div>)}</div></details>}
    <form onSubmit={event => {
      event.preventDefault(); setBusy(true); setError('');
      void api.createCoordinatedAgent({ project, name, host, cwd, runtime }).then(agent => revealTerminal(agent.target))
        .catch(err => setError(String(err))).finally(() => setBusy(false));
    }}>
      <h3>Create agent</h3>
      <p>Use the same project ID on every host. Remote hosts use your server’s OpenSSH configuration and need the agent CLI and Rig client installed; no remote Rig daemon is needed.</p>
      <div className="coordination-fields">
        <label>Project ID<input required pattern="[a-z]([a-z0-9]|-){0,47}" value={project} onChange={e => setProject(e.target.value)} placeholder="webmux" /></label>
        <label>Agent name<input required pattern="[a-z]([a-z0-9]|-){0,47}" value={name} onChange={e => setName(e.target.value)} placeholder="reviewer" /></label>
        <label>Execution host<input required value={host} onChange={e => setHost(e.target.value)} placeholder="localhost or SSH host" /></label>
        <label>Project directory on this host<input required value={cwd} onChange={e => setCwd(e.target.value)} placeholder="/absolute/project/path" /></label>
        <div><label htmlFor="coordination-runtime">Agent</label><select id="coordination-runtime" value={runtime} onChange={e => setRuntime(e.target.value)}><option value="codex">Codex</option><option value="claude-code">Claude Code</option></select></div>
      </div>
      <p><button type="submit" disabled={busy}>{busy ? 'Connecting…' : 'Create and open terminal'}</button></p>
    </form>
    <details><summary>Advanced diagnostics</summary><p><a href="?panel=openrig" target="_blank" rel="noreferrer">Open Rig text dashboard</a></p></details>
    </div>
  </Dialog>;
}
