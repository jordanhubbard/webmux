import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
  seatCount: number; runningCount: number; detail?: string; contexts?: string[]; attentionCount?: number;
}
export interface CoordinationSeat {
  provider: string; id: string; projectId: string; projectName: string; name: string; host: string;
  workspace?: string; runtime: string; state: string; openable: boolean; managed: boolean;
  nativeSessionId?: string; context?: string; parentId?: string; parentName?: string;
  role?: string; substate?: string; lastActivityAt?: string; attention?: boolean; canReceive?: boolean; exitCode?: number;
}
export interface CoordinationMessage {
  provider?: string; from: string; to: string; body: string; time: string; state: string;
  kind?: string; level?: string; sourceRemote?: string;
}
export interface CoordinationSendResult {
  target: string; sendId?: string; state?: string; verdict?: string; delivery?: string;
  confirmation?: string; reason?: string; queued: boolean; submitted: boolean;
}
export interface CoordinationDrainResult {
  remote: string; target: string; fetched: number; written: number; duplicates: number; unknown: number;
  woke: boolean; writerRunning?: boolean; writerDetail?: string; messages: CoordinationMessage[];
}
export interface WorkspaceListing {
  host: string; path: string; parent: string | null; repository: boolean;
  entries: { name: string; path: string; repository: boolean }[];
}
export interface CoordinationStatus {
  contract: string; defaultProvider: 'agent-deck' | 'rig' | 'disabled' | 'choose'; providers: CoordinationProvider[]; projects: CoordinationProject[]; seats: CoordinationSeat[];
  messages: CoordinationMessage[]; errors: Record<string, string>; attentionCount?: number;
}

export function CoordinationDialog({ onClose, position, inline = false }: { onClose: () => void; position?: { row: number; col: number }; inline?: boolean }) {
  const [status, setStatus] = useState<CoordinationStatus>({ contract: '', defaultProvider: 'choose', providers: [], projects: [], seats: [], messages: [], errors: {} });
  const [provider, setProvider] = useState('');
  const [changingProvider, setChangingProvider] = useState(false);
  const providerInitialized = useRef(false);
  const [statusError, setStatusError] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [mode, setMode] = useState<'existing' | 'new'>('new');
  const [projectID, setProjectID] = useState('');
  const [projectName, setProjectName] = useState<string | null>(null);
  const [context, setContext] = useState('');
  const [name, setName] = useState<string | null>(null);
  const [purpose, setPurpose] = useState('builder');
  const [host, setHost] = useState('localhost');
  const [workspace, setWorkspace] = useState('');
  const [runtime, setRuntime] = useState('codex');
  const [browser, setBrowser] = useState<WorkspaceListing | null>(null);
  const [composerTarget, setComposerTarget] = useState('');
  const [instruction, setInstruction] = useState('');
  const [queueInstruction, setQueueInstruction] = useState(true);
  const [deliveries, setDeliveries] = useState<Record<string, CoordinationSendResult>>({});
  const [drainRemote, setDrainRemote] = useState('');
  const [drainTarget, setDrainTarget] = useState('');
  const [drainResult, setDrainResult] = useState<CoordinationDrainResult | null>(null);
  const { setActivePane } = useWorkspacePane();

  const refresh = useCallback(async () => {
    try {
      const value = await api.getCoordination();
      setStatus(value); setStatusError('');
      if (!providerInitialized.current) {
        providerInitialized.current = true;
        if (value.defaultProvider === 'agent-deck' || value.defaultProvider === 'rig' || value.defaultProvider === 'disabled') setProvider(value.defaultProvider);
        else setChangingProvider(true);
      }
    } catch (err) { setStatusError(String(err)); }
  }, []);
  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => { void refresh(); }, 5000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  const selectedProvider = status.providers.find(value => value.id === provider);
  const defaultProvider = status.providers.find(value => value.id === status.defaultProvider);
  const projects = status.projects.filter(value => value.provider === provider);
  const seats = status.seats.filter(value => value.provider === provider);
  const providerMessages = status.messages.filter(value => !value.provider || value.provider === provider);
  const selectedProject = projects.find(value => value.id === projectID);
  const contexts = selectedProject?.contexts || [];
  const hostOptions = useMemo(() => [...new Set(['localhost', ...projects.map(value => value.host), ...seats.map(value => value.host)])].filter(Boolean), [projects, seats]);
  const drainRemotes = useMemo(() => [...new Set(projects.map(value => value.host).filter(value => value && value !== 'localhost'))], [projects]);
  const drainTargets = seats.filter(value => value.canReceive && value.id.startsWith('local:'));

  // Names identify sessions, not special commands or agent instructions.
  const folderName = workspace.replace(/\\/g, '/').split('/').filter(Boolean).pop() || 'project';
  let suggestedProject = folderName.toLowerCase().replace(/[^a-z0-9-]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 42) || 'project';
  if (!/^[a-z]/.test(suggestedProject)) suggestedProject = `project-${suggestedProject}`.slice(0, 42);
  const projectBase = suggestedProject;
  for (let suffix = 2; projects.some(value => value.host === host && value.name === suggestedProject); suffix++) suggestedProject = `${projectBase}-${suffix}`;
  const resolvedProjectName = projectName ?? suggestedProject;
  const nameBase = `${runtime === 'codex' ? 'codex' : 'claude'}-${purpose}`;
  let suggestedName = nameBase;
  for (let suffix = 2; seats.some(value => value.host === host && value.name === suggestedName && (mode === 'new' ? value.projectName === resolvedProjectName : value.projectId === projectID)); suffix++) suggestedName = `${nameBase}-${suffix}`;
  const resolvedName = name ?? suggestedName;

  useEffect(() => {
    setProjectID(current => projects.some(value => value.id === current) ? current : projects.find(value => !value.historical)?.id || projects[0]?.id || '');
  }, [provider, status.projects]);
  useEffect(() => {
    setContext(current => contexts.includes(current) ? current : contexts[0] || '');
    if (mode === 'existing' && selectedProject && provider === 'agent-deck' && host !== selectedProject.host) {
      setHost(selectedProject.host); setWorkspace(''); setBrowser(null);
    }
  }, [mode, projectID, provider, selectedProject?.host, contexts.join('\0'), host]);
  useEffect(() => {
    setDrainRemote(current => drainRemotes.includes(current) ? current : drainRemotes[0] || '');
    setDrainTarget(current => drainTargets.some(value => value.id === current) ? current : drainTargets[0]?.id || '');
  }, [provider, drainRemotes.join('\0'), drainTargets.map(value => value.id).join('\0')]);
  useEffect(() => {
    const pending = Object.entries(deliveries).filter(([, value]) => value.sendId && !['landed', 'delivered', 'failed', 'settled'].includes(value.state || value.verdict || value.delivery || ''));
    if (pending.length === 0) return;
    const timer = window.setInterval(() => {
      for (const [target, delivery] of pending) {
        void api.getCoordinationSendStatus('agent-deck', target, delivery.sendId!).then(value => {
          const text = (key: string) => typeof value[key] === 'string' ? value[key] as string : undefined;
          setDeliveries(current => ({ ...current, [target]: { ...(current[target] ?? delivery), target, sendId: delivery.sendId, state: text('state'), verdict: text('verdict'), delivery: text('delivery'), confirmation: text('confirmation'), reason: text('reason'), queued: text('state') === 'queued', submitted: value.submitted === true } }));
        }).catch(() => {});
      }
    }, 2500);
    return () => window.clearInterval(timer);
  }, [deliveries]);

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
    if (browser) setWorkspace(browser.path);
  };
  const chooseProvider = (value: string) => {
    setProvider(value); setProjectID(''); setBrowser(null); setWorkspace('');
  };
  const makeDefault = async () => {
    if (provider !== 'agent-deck' && provider !== 'rig' && provider !== 'disabled') return;
    setBusy(true); setError('');
    try {
      await api.updateConfig({ app: { coordination: { default_provider: provider } } });
      setStatus(current => ({ ...current, defaultProvider: provider }));
      setChangingProvider(false);
      window.dispatchEvent(new CustomEvent('webmux-sessions-changed'));
    } catch (err) { setError(String(err)); }
    finally { setBusy(false); }
  };
  const sendInstruction = async (seat: CoordinationSeat) => {
    const message = instruction.trim();
    if (!message) return;
    setBusy(true); setError('');
    try {
      const result = await api.sendCoordinationMessage(seat.provider, seat.id, message, queueInstruction);
      setDeliveries(current => ({ ...current, [seat.id]: result }));
      setInstruction('');
    } catch (err) { setError(String(err)); }
    finally { setBusy(false); }
  };
  const drainRemoteCompletions = async () => {
    if (!drainRemote || !drainTarget) return;
    setBusy(true); setError(''); setDrainResult(null);
    try {
      setDrainResult(await api.drainCoordinationRemote('agent-deck', drainRemote, drainTarget));
      await refresh();
    } catch (err) { setError(String(err)); }
    finally { setBusy(false); }
  };

  const content = <>
    <div className="coordination-dialog">
      <p>Start a coding agent in a folder, or open a session you already have. A project keeps agents working on the same code together. Opening a session puts its terminal in the Terminals grid.</p>
      {statusError && <p role="alert" className="coordination-error">{statusError}</p>}
      {error && <p role="alert" className="coordination-error">{error}</p>}
      <div className="coordination-toolbar">
        {provider && <div className="coordination-current"><span>Using <strong>{provider === 'disabled' ? 'None' : selectedProvider?.label ?? provider}</strong>{status.defaultProvider !== provider && <> · default: {status.defaultProvider === 'disabled' ? 'None' : defaultProvider?.label ?? status.defaultProvider}</>}</span></div>}
          <div className="coordination-provider-choice">
              <label>Coordinator<select aria-label="Coordinator" disabled={busy} value={provider} onChange={event => { chooseProvider(event.target.value); setChangingProvider(true); }}>
                <option value="">Choose an experience…</option>
                {['agent-deck', 'rig'].map(id => {
                  const value = status.providers.find(value => value.id === id);
                  return <option key={id} value={id}>{id === 'agent-deck' ? 'Agent Deck' : 'Rig'}{value?.available ? '' : ' · unavailable'}</option>;
                })}
                <option value="disabled">None</option>
              </select></label>
              {provider && changingProvider && <button type="button" disabled={busy} onClick={() => setChangingProvider(false)}>Use this time</button>}
              {provider && status.defaultProvider !== provider && <button type="button" disabled={busy} onClick={() => void makeDefault()}>Make default</button>}
            </div>
        <button type="button" disabled={busy} onClick={() => void refresh()}>Refresh projects and sessions</button>
      </div>
      {status.defaultProvider === 'choose' && !provider && <p role="note">Choose Agent Deck or Rig to begin. Making it the default completes the one-time upgrade choice.</p>}
      {status.defaultProvider === 'disabled' && !provider && <p role="note">Coordination is disabled. Choose an experience above to enable it now or make it the default.</p>}
      {provider === 'disabled' && <p role="status">No coordinator selected. Existing sessions keep running. Choose Agent Deck or Rig to manage coordinated sessions.</p>}
      {provider && provider !== 'disabled' && <>
      {selectedProvider?.message && !selectedProvider.available && <p role="note">{selectedProvider.message}</p>}
      {status.errors[provider] && <p role="alert" className="coordination-error">{status.errors[provider]}</p>}

      <form id="coordination-create-seat" onSubmit={event => {
        event.preventDefault(); setBusy(true); setError('');
        const input = { provider, mode, projectId: mode === 'existing' ? projectID : undefined, projectName: mode === 'existing' ? selectedProject?.name || '' : resolvedProjectName, context: mode === 'existing' ? context : undefined, name: resolvedName, host, workspace, runtime, ...position };
        void api.createCoordinatedAgent(input).then(value => revealSession(value.session)).catch(err => setError(String(err))).finally(() => setBusy(false));
      }}>
        <h3>Start a coding agent</h3>
        <p>Choose your code folder and an agent. WebMux opens its terminal there so you can describe the task. Localhost is the computer running WebMux.</p>
        <div className="coordination-fields">
          <label>Run on host<input required list="coordination-hosts" value={host} onChange={event => { setHost(event.target.value); setBrowser(null); setWorkspace(''); }} placeholder={provider === 'agent-deck' ? 'Agent Deck remote name' : 'localhost or SSH host'} /><datalist id="coordination-hosts">{hostOptions.map(value => <option value={value} key={value} />)}</datalist></label>
          <div><label htmlFor="coordination-workspace">Code folder</label><span className="coordination-input-action"><input id="coordination-workspace" required value={workspace} onChange={event => setWorkspace(event.target.value)} placeholder="Choose the folder containing your code" /><button type="button" disabled={busy || !host.trim()} onClick={() => void browse(workspace || undefined)}>Browse</button></span></div>
          <div><label htmlFor="coordination-runtime">Coding agent</label><select id="coordination-runtime" value={runtime} onChange={event => setRuntime(event.target.value)}><option value="codex">Codex</option><option value="claude-code">Claude Code</option></select></div>
          <label>Session purpose<select value={purpose} onChange={event => setPurpose(event.target.value)}><option value="builder">Build a feature</option><option value="reviewer">Review changes</option><option value="investigator">Investigate a problem</option></select></label>
        </div>
        <fieldset className="coordination-choice"><legend>Where should this agent belong?</legend>
          <label><input type="radio" name="coordination-mode" checked={mode === 'new'} onChange={() => setMode('new')} /> Start a new project</label>
          <label><input type="radio" name="coordination-mode" checked={mode === 'existing'} onChange={() => setMode('existing')} disabled={projects.filter(value => !value.historical).length === 0} /> Add an agent to an existing project</label>
        </fieldset>
        {mode === 'existing' && <div className="coordination-fields">
          <label>Project<select required value={projectID} onChange={event => setProjectID(event.target.value)}><option value="">Choose a project</option>{projects.filter(value => !value.historical).map(value => <option key={value.id} value={value.id}>{value.name} · {value.host} · {value.state}</option>)}</select></label>
          {provider === 'rig' && <label>Rig pod<select required value={context} onChange={event => setContext(event.target.value)}><option value="">Choose a pod</option>{contexts.map(value => <option key={value} value={value}>{value}</option>)}</select></label>}
        </div>}
        <div className="coordination-fields coordination-names">
          {mode === 'new' && <label>Project name<input required pattern="[a-z]([a-z0-9]|-){0,47}" value={resolvedProjectName} onChange={event => setProjectName(event.target.value)} /></label>}
          <label>Session name<input required pattern="[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}" value={resolvedName} onChange={event => setName(event.target.value)} /></label>
        </div>
        <p className="coordination-hint">Names are editable labels, not commands. We suggest a project from your folder and a session from the agent and purpose. The purpose names the session; give the agent its instructions after it opens.</p>
        {browser && <section className="coordination-browser" aria-label={`Workspaces on ${host}`}><div><strong>{host}</strong> · <code>{browser.path}</code> {browser.repository && <span className="coordination-repository">Git repository</span>}</div><div className="coordination-browser-actions"><button type="button" onClick={chooseFolder}>Choose this folder</button>{browser.parent && <button type="button" onClick={() => void browse(browser.parent!)}>Parent folder</button>}</div><ul>{browser.entries.map(entry => <li key={entry.path}><button type="button" onClick={() => void browse(entry.path)}>{entry.name}/</button>{entry.repository && <span>Git repository</span>}</li>)}</ul></section>}
        <p><button type="submit" disabled={busy || !selectedProvider?.available || (mode === 'existing' && (!selectedProject || (provider === 'rig' && !context)))}>{busy ? 'Connecting…' : 'Start agent and open terminal'}</button></p>
      </form>
      <section aria-labelledby="coordination-projects-heading">
        <h3 id="coordination-projects-heading">Your projects and sessions</h3>
        {projects.length === 0 && selectedProvider?.available && !status.errors[provider] && <p>No projects yet. Choose a workspace above to start your first agent.</p>}
        <div className="coordination-rig-list">
          {projects.map(project => {
            const projectSeats = seats.filter(seat => seat.projectId === project.id);
            return <details className="coordination-project" key={`${project.provider}:${project.id}`} open={!project.historical}>
              <summary><strong>{project.name}</strong> <span className={`coordination-state coordination-state-${project.state}`}>{project.state}</span> · {project.runningCount}/{project.seatCount} active{project.attentionCount ? ` · ${project.attentionCount} need attention` : ''} · {project.host}</summary>
              <details className="coordination-identity"><summary>Project details</summary><code>{project.id}</code>{project.detail && <p>{project.detail}</p>}</details>
              {projectSeats.length === 0 ? <p>No sessions in this project.</p> : <ul aria-label={`${project.name} sessions`} className="coordination-workers">
                {projectSeats.map(seat => <li key={`${seat.provider}:${seat.id}`}><article aria-label={`${seat.name} in ${seat.projectName}`} className={seat.attention ? 'coordination-worker-attention' : undefined}>
                  <strong>{seat.name}</strong> {seat.managed && <span className="coordination-rig">{provider === 'rig' ? 'Rig-managed' : 'Agent Deck-managed'}</span>}{seat.role && seat.role !== 'session' && <span className="coordination-role">{seat.role}</span>}{seat.attention && <span className="coordination-attention">Needs attention</span>}
                  <dl className="coordination-worker-facts">
                    <div><dt>Host</dt><dd>{seat.host}</dd></div><div><dt>Runtime</dt><dd>{seat.runtime}</dd></div>
                    <div><dt>State</dt><dd>{seat.state}{seat.substate ? ` · ${seat.substate}` : ''}{seat.exitCode !== undefined ? ` · exit ${seat.exitCode}` : ''}</dd></div>{seat.context && <div><dt>Context</dt><dd>{seat.context}</dd></div>}
                    {seat.parentId && <div><dt>Parent</dt><dd>{seat.parentName || seat.parentId}</dd></div>}
                    {seat.workspace && <div><dt>Workspace</dt><dd><code>{seat.workspace}</code></dd></div>}
                    {seat.nativeSessionId && <div><dt>Native session</dt><dd><code>{seat.nativeSessionId}</code></dd></div>}
                    {seat.lastActivityAt && <div><dt>Last activity</dt><dd>{new Date(seat.lastActivityAt).toLocaleString()}</dd></div>}
                  </dl>
                  {seat.openable ? <button type="button" disabled={busy} onClick={() => void openTerminal(seat)}>Open session</button>
                    : seat.managed && provider === 'rig' && seat.workspace ? <button type="button" disabled={busy} onClick={() => void restartSeat(seat)}>Restart provider and open</button>
                      : <span className="coordination-unavailable">No attach transport reported by this provider</span>}
                  {provider === 'agent-deck' && seat.openable && <button type="button" disabled={busy} onClick={() => { setComposerTarget(current => current === seat.id ? '' : seat.id); setInstruction(''); }}>{composerTarget === seat.id ? 'Cancel instruction' : 'Send instruction'}</button>}
                  {composerTarget === seat.id && <form className="coordination-composer" onSubmit={event => { event.preventDefault(); void sendInstruction(seat); }}>
                    <label>Instruction for {seat.name}<textarea required value={instruction} onChange={event => setInstruction(event.target.value)} placeholder="Describe the bounded task or question" /></label>
                    <label className="coordination-inline"><input type="checkbox" checked={queueInstruction} onChange={event => setQueueInstruction(event.target.checked)} /> Queue safely until the target can accept input</label>
                    <button type="submit" disabled={busy || !instruction.trim()}>{queueInstruction ? 'Queue instruction' : 'Send now'}</button>
                  </form>}
                  {deliveries[seat.id] && <p role="status" className="coordination-delivery">Delivery: {deliveries[seat.id].state || deliveries[seat.id].verdict || deliveries[seat.id].delivery || 'accepted'}{deliveries[seat.id].confirmation ? ` · ${deliveries[seat.id].confirmation}` : ''}{deliveries[seat.id].reason ? ` · ${deliveries[seat.id].reason}` : ''}</p>}
                </article></li>)}
              </ul>}
              {!project.historical && <button type="button" disabled={busy || (provider === 'rig' && (project.contexts?.length ?? 0) === 0)} onClick={() => { setMode('existing'); setProjectID(project.id); setHost(project.host); document.getElementById('coordination-create-seat')?.scrollIntoView({ block: 'nearest' }); }}>Create session in this project</button>}
            </details>;
          })}
        </div>
      </section>

      {providerMessages.length > 0 && <details><summary>{provider === 'rig' ? 'Recent Rig coordination' : `Agent Deck activity (${providerMessages.length})`}</summary><div className="coordination-messages">{providerMessages.map((message, index) => <div key={`${message.time}-${message.from}-${index}`} className={message.level === 'urgent' ? 'coordination-message-urgent' : undefined}><small>{new Date(message.time).toLocaleTimeString()} · {message.from} → {message.to} · {message.kind || 'message'} ({message.state || 'recorded'})</small><pre>{(message.body.split('\n---\n')[1] ?? message.body) || 'Status changed without a text result.'}</pre></div>)}</div></details>}

      {provider === 'agent-deck' && drainRemotes.length > 0 && <section className="coordination-drain" aria-labelledby="coordination-drain-heading">
        <h3 id="coordination-drain-heading">Collect remote completions</h3>
        {drainTargets.length === 0 ? <p>Start or identify a local Agent Deck conductor before collecting remote completion records.</p> : <>
          <p>Pull durable completion and transition records from a registered remote into a local conductor. The remote records are not consumed.</p>
          <div className="coordination-fields"><label>Remote Agent Deck<select value={drainRemote} onChange={event => setDrainRemote(event.target.value)}>{drainRemotes.map(value => <option key={value} value={value}>{value}</option>)}</select></label><label>Receiving conductor<select value={drainTarget} onChange={event => setDrainTarget(event.target.value)}>{drainTargets.map(value => <option key={value.id} value={value.id}>{value.name} · {value.projectName}</option>)}</select></label></div>
          <button type="button" disabled={busy || !drainRemote || !drainTarget} onClick={() => void drainRemoteCompletions()}>Collect completions</button>
        </>}
        {drainResult && <p role="status">Collected {drainResult.written} new of {drainResult.fetched} records from {drainResult.remote}; {drainResult.duplicates} already present.{drainResult.writerRunning === false ? ` Warning: ${drainResult.writerDetail || 'the remote transition writer is not running.'}` : ''}</p>}
      </section>}


      </>}
      <details><summary>Provider contracts</summary><ul>{status.providers.map(value => <li key={value.id}><strong>{value.label}</strong> · <code>{value.contract}</code> · {value.capabilities.join(', ')}</li>)}</ul></details>
      {provider === 'rig' && <details><summary>Rig diagnostics</summary><p><a href="?panel=openrig" target="_blank" rel="noreferrer">Open Rig text dashboard (Rig provider only)</a></p></details>}
    </div>
  </>;
  return inline
    ? <section className="coordination-page" aria-label="Coordination"><header><h1>Coordination</h1><p>Projects and coding agents, when you need them</p></header>{content}</section>
    : <Dialog title="Coordination" subtitle={position ? `Start an agent at row ${position.row + 1}, column ${position.col + 1}` : 'Projects and coding agents'} onClose={onClose} width={880} dismissible={!busy}>{content}</Dialog>;
}
