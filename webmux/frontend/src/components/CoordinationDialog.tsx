import { useCallback, useEffect, useMemo, useState } from 'react';
import { Dialog } from './Dialog';
import './CoordinationDialog.css';
import { api } from '../utils/api';
import { useWorkspacePane } from '../contexts/WorkspacePaneContext';
import type { Session } from '../types';

export interface CoordinationProvider {
  id: string; label: string; available: boolean; message?: string; capabilities: string[]; contract: string;
}
export interface CoordinationProject {
  provider: string; id: string; name: string; host: string; state: string; historical: boolean;
  seatCount: number; runningCount: number; detail?: string; contexts?: string[];
}
export interface CoordinationSeat {
  provider: string; id: string; projectId: string; projectName: string; name: string; host: string;
  workspace?: string; runtime: string; state: string; openable: boolean; managed: boolean;
  nativeSessionId?: string; context?: string;
}
export interface WorkspaceListing {
  host: string; path: string; parent: string | null; repository: boolean;
  entries: { name: string; path: string; repository: boolean }[];
}
export interface CoordinationStatus {
  contract: string; providers: CoordinationProvider[]; projects: CoordinationProject[]; seats: CoordinationSeat[];
  messages: { from: string; to: string; body: string; time: string; state: string }[]; errors: Record<string, string>;
}

export function CoordinationDialog({ onClose, position }: { onClose: () => void; position?: { row: number; col: number } }) {
  const [status, setStatus] = useState<CoordinationStatus>({ contract: '', providers: [], projects: [], seats: [], messages: [], errors: {} });
  const [provider, setProvider] = useState('rig');
  const [statusError, setStatusError] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [mode, setMode] = useState<'existing' | 'new'>('existing');
  const [projectID, setProjectID] = useState('');
  const [projectName, setProjectName] = useState('');
  const [context, setContext] = useState('');
  const [name, setName] = useState('');
  const [host, setHost] = useState('localhost');
  const [workspace, setWorkspace] = useState('');
  const [runtime, setRuntime] = useState('codex');
  const [browser, setBrowser] = useState<WorkspaceListing | null>(null);
  const { setActivePane } = useWorkspacePane();

  const refresh = useCallback(async () => {
    try {
      const value = await api.getCoordination();
      setStatus(value); setStatusError('');
      setProvider(current => value.providers.some(item => item.id === current && item.available) ? current : value.providers.find(item => item.available)?.id || current);
    } catch (err) { setStatusError(String(err)); }
  }, []);
  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => { void refresh(); }, 5000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  const selectedProvider = status.providers.find(value => value.id === provider);
  const projects = status.projects.filter(value => value.provider === provider);
  const seats = status.seats.filter(value => value.provider === provider);
  const selectedProject = projects.find(value => value.id === projectID);
  const contexts = selectedProject?.contexts || [];
  const hostOptions = useMemo(() => [...new Set(['localhost', ...projects.map(value => value.host), ...seats.map(value => value.host)])].filter(Boolean), [projects, seats]);

  useEffect(() => {
    setProjectID(current => projects.some(value => value.id === current) ? current : projects.find(value => !value.historical)?.id || projects[0]?.id || '');
  }, [provider, status.projects]);
  useEffect(() => {
    setContext(current => contexts.includes(current) ? current : contexts[0] || '');
    if (selectedProject && provider === 'agent-deck') setHost(selectedProject.host);
  }, [projectID, provider, selectedProject?.host, contexts.join('\0')]);

  const revealSession = (session: Session) => {
    window.dispatchEvent(new CustomEvent('webmux-sessions-changed', { detail: { sessionId: session.id } }));
    setActivePane('terminals'); onClose();
  };
  const revealTerminal = async (providerID: string, target: string) => {
    revealSession(await api.attachCoordinatedAgent(providerID, target, position));
  };
  const openTerminal = async (seat: CoordinationSeat) => {
    setBusy(true); setError('');
    try { await revealTerminal(seat.provider, seat.id); }
    catch (err) { setError(String(err)); }
    finally { setBusy(false); }
  };
  const restartSeat = async (seat: CoordinationSeat) => {
    setBusy(true); setError('');
    try {
      const launched = await api.createCoordinatedAgent({
        provider: seat.provider, mode: 'existing', projectId: seat.projectId, projectName: seat.projectName,
        context: seat.context, name: seat.name, host: seat.host, workspace: seat.workspace || '', runtime: seat.runtime, ...position,
      });
      revealSession(launched.session);
    } catch (err) { setError(String(err)); }
    finally { setBusy(false); }
  };
  const browse = async (path?: string) => {
    setBusy(true); setError('');
    try { setBrowser(await api.browseCoordinationWorkspaces(provider, host, path)); }
    catch (err) { setError(String(err)); }
    finally { setBusy(false); }
  };
  const chooseFolder = () => {
    if (!browser) return;
    setWorkspace(browser.path);
    if (mode === 'new' && !projectName) {
      const parts = browser.path.split('/').filter(Boolean);
      setProjectName(parts[parts.length - 1]?.toLowerCase().replace(/[^a-z0-9-]+/g, '-').replace(/^[^a-z]+/, '') || 'project');
    }
  };

  return <Dialog title="Coordination" subtitle={position ? `Open a session at row ${position.row + 1}, column ${position.col + 1}` : 'Projects and coding sessions across hosts'} onClose={onClose} width={880} dismissible={!busy}>
    <div className="coordination-dialog">
      <p>Browse coordinator-owned projects and sessions, then launch Codex or Claude in a validated workspace on the selected host.</p>
      {statusError && <p role="alert" className="coordination-error">{statusError}</p>}
      {error && <p role="alert" className="coordination-error">{error}</p>}
      <div className="coordination-toolbar">
        <label>Coordinator<select aria-label="Coordinator" value={provider} onChange={event => { setProvider(event.target.value); setProjectID(''); setBrowser(null); setWorkspace(''); }}>
          {status.providers.map(value => <option key={value.id} value={value.id} disabled={!value.available}>{value.label}{value.available ? '' : ' · unavailable'}</option>)}
        </select></label>
        <button type="button" disabled={busy} onClick={() => void refresh()}>Refresh projects and sessions</button>
      </div>
      {selectedProvider?.message && !selectedProvider.available && <p role="note">{selectedProvider.message}</p>}
      {status.errors[provider] && <p role="alert" className="coordination-error">{status.errors[provider]}</p>}

      <section aria-labelledby="coordination-projects-heading">
        <h3 id="coordination-projects-heading">{provider === 'rig' ? 'Canonical Rig projects' : 'Agent Deck groups'}</h3>
        {projects.length === 0 && selectedProvider?.available && !status.errors[provider] && <p>No projects are registered with this coordinator. Create one below.</p>}
        <div className="coordination-rig-list">
          {projects.map(project => {
            const projectSeats = seats.filter(seat => seat.projectId === project.id);
            return <details className="coordination-project" key={`${project.provider}:${project.id}`} open={!project.historical}>
              <summary><strong>{project.name}</strong> <span className={`coordination-state coordination-state-${project.state}`}>{project.state}</span> · {project.runningCount}/{project.seatCount} active · {project.host}</summary>
              <p className="coordination-identity"><span>Provider ID</span> <code>{project.id}</code>{project.detail && <> · {project.detail}</>}</p>
              {projectSeats.length === 0 ? <p>No sessions in this project.</p> : <ul aria-label={`${project.name} sessions`} className="coordination-workers">
                {projectSeats.map(seat => <li key={`${seat.provider}:${seat.id}`}><article aria-label={`${seat.name} in ${seat.projectName}`}>
                  <strong>{seat.name}</strong> {seat.managed && <span className="coordination-rig">{provider === 'rig' ? 'Rig-managed' : 'Agent Deck-managed'}</span>}
                  <dl className="coordination-worker-facts">
                    <div><dt>Host</dt><dd>{seat.host}</dd></div><div><dt>Runtime</dt><dd>{seat.runtime}</dd></div>
                    <div><dt>State</dt><dd>{seat.state}</dd></div>{seat.context && <div><dt>Context</dt><dd>{seat.context}</dd></div>}
                    {seat.workspace && <div><dt>Workspace</dt><dd><code>{seat.workspace}</code></dd></div>}
                    {seat.nativeSessionId && <div><dt>Native session</dt><dd><code>{seat.nativeSessionId}</code></dd></div>}
                  </dl>
                  {seat.openable ? <button type="button" disabled={busy} onClick={() => void openTerminal(seat)}>Open session</button>
                    : seat.managed && provider === 'rig' && seat.workspace ? <button type="button" disabled={busy} onClick={() => void restartSeat(seat)}>Restart provider and open</button>
                      : <span className="coordination-unavailable">No attach transport reported by this provider</span>}
                </article></li>)}
              </ul>}
              {!project.historical && <button type="button" disabled={busy || (provider === 'rig' && (project.contexts?.length ?? 0) === 0)} onClick={() => { setMode('existing'); setProjectID(project.id); setHost(project.host); document.getElementById('coordination-create-seat')?.scrollIntoView({ block: 'nearest' }); }}>Create session in this project</button>}
            </details>;
          })}
        </div>
      </section>

      {provider === 'rig' && status.messages.length > 0 && <details><summary>Recent Rig coordination</summary><div className="coordination-messages">{status.messages.map((message, index) => <div key={`${message.time}-${index}`}><small>{new Date(message.time).toLocaleTimeString()} · {message.from} → {message.to} · recorded ({message.state})</small><pre>{message.body.split('\n---\n')[1] ?? message.body}</pre></div>)}</div></details>}

      <form id="coordination-create-seat" onSubmit={event => {
        event.preventDefault(); setBusy(true); setError('');
        const input = { provider, mode, projectId: mode === 'existing' ? projectID : undefined, projectName: mode === 'existing' ? selectedProject?.name || '' : projectName, context: mode === 'existing' ? context : undefined, name, host, workspace, runtime, ...position };
        void api.createCoordinatedAgent(input).then(value => revealSession(value.session)).catch(err => setError(String(err))).finally(() => setBusy(false));
      }}>
        <h3>Create coordinated session</h3>
        <fieldset className="coordination-choice"><legend>Action</legend>
          <label><input type="radio" name="coordination-mode" checked={mode === 'existing'} onChange={() => setMode('existing')} /> New session in an existing project</label>
          <label><input type="radio" name="coordination-mode" checked={mode === 'new'} onChange={() => setMode('new')} /> New coordinated project</label>
        </fieldset>
        <div className="coordination-fields">
          {mode === 'existing' ? <>
            <label>{provider === 'rig' ? 'Rig topology' : 'Agent Deck group'}<select required value={projectID} onChange={event => setProjectID(event.target.value)}><option value="">Choose a project</option>{projects.filter(value => !value.historical).map(value => <option key={value.id} value={value.id}>{value.name} · {value.host} · {value.state}</option>)}</select></label>
            {provider === 'rig' && <label>Rig pod<select required value={context} onChange={event => setContext(event.target.value)}><option value="">Choose a pod</option>{contexts.map(value => <option key={value} value={value}>{value}</option>)}</select></label>}
          </> : <label>{provider === 'rig' ? 'New Rig name' : 'New group name'}<input required pattern="[a-z]([a-z0-9]|-){0,47}" value={projectName} onChange={event => setProjectName(event.target.value)} placeholder="project-coordination" /></label>}
          <label>Session name<input required pattern="[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}" value={name} onChange={event => setName(event.target.value)} placeholder="reviewer" /></label>
          <label>Execution host<input required list="coordination-hosts" value={host} onChange={event => { setHost(event.target.value); setBrowser(null); setWorkspace(''); }} placeholder={provider === 'agent-deck' ? 'Agent Deck remote name' : 'localhost or SSH host'} /><datalist id="coordination-hosts">{hostOptions.map(value => <option value={value} key={value} />)}</datalist></label>
          <div><label htmlFor="coordination-workspace">Workspace on execution host</label><span className="coordination-input-action"><input id="coordination-workspace" required value={workspace} onChange={event => setWorkspace(event.target.value)} placeholder="Browse or enter an absolute path" /><button type="button" disabled={busy || !host.trim()} onClick={() => void browse(workspace || undefined)}>Browse</button></span></div>
          <div><label htmlFor="coordination-runtime">Coding agent</label><select id="coordination-runtime" value={runtime} onChange={event => setRuntime(event.target.value)}><option value="codex">Codex</option><option value="claude-code">Claude Code</option></select></div>
        </div>
        {browser && <section className="coordination-browser" aria-label={`Workspaces on ${host}`}><div><strong>{host}</strong> · <code>{browser.path}</code> {browser.repository && <span className="coordination-repository">Git repository</span>}</div><div className="coordination-browser-actions"><button type="button" onClick={chooseFolder}>Choose this folder</button>{browser.parent && <button type="button" onClick={() => void browse(browser.parent!)}>Parent folder</button>}</div><ul>{browser.entries.map(entry => <li key={entry.path}><button type="button" onClick={() => void browse(entry.path)}>{entry.name}/</button>{entry.repository && <span>Git repository</span>}</li>)}</ul></section>}
        <p><button type="submit" disabled={busy || !selectedProvider?.available || (mode === 'existing' && (!selectedProject || (provider === 'rig' && !context)))}>{busy ? 'Connecting…' : 'Create session and open terminal'}</button></p>
      </form>
      <details><summary>Provider contracts</summary><ul>{status.providers.map(value => <li key={value.id}><strong>{value.label}</strong> · <code>{value.contract}</code> · {value.capabilities.join(', ')}</li>)}</ul></details>
      <details><summary>Advanced diagnostics</summary><p><a href="?panel=openrig" target="_blank" rel="noreferrer">Open Rig text dashboard</a></p></details>
    </div>
  </Dialog>;
}
