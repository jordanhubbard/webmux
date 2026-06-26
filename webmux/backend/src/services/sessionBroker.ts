import { EventEmitter } from 'events';
import { v4 as uuidv4 } from 'uuid';
import * as pty from 'node-pty';
import { Session, CreateSessionRequest } from '../types';
import { transportLauncher } from './transportLauncher';
import { presenceService } from './presenceService';
import { persistence } from './persistenceManager';
import { compactPositions } from './gridLayout';
import { assertTerminalGridPosition, nextTerminalGridPosition } from './terminalGridLimits';
import { agentService } from './agentService';
import { AgentAccessError, getAgentAccess } from './agentAccess';
import type { AgentSessionRole, WorkspaceName } from '../types';

export function isAgentSession(session?: Session): boolean {
  return !!session?.agent_id && !!session.agent_role;
}

function argvEqual(left: string[] | undefined, right: string[] | undefined): boolean {
  if (left === right) return true;
  if (!left || !right || left.length !== right.length) return false;
  return left.every((value, index) => value === right[index]);
}

function latestIso(left: string | undefined, right: string | undefined): string | undefined {
  if (!left) return right;
  if (!right) return left;
  return Date.parse(right) >= Date.parse(left) ? right : left;
}

interface AgentStatusUpdate {
  status: 'working';
  source: 'webmux';
  last_input_at?: string;
  last_output_at?: string;
  last_output_source?: 'live';
}

interface PendingAgentStatusUpdate {
  agentId: string;
  name: string;
  update: AgentStatusUpdate;
}

function agentStatusKey(agentId: string, name: string): string {
  return `${agentId}:${name}`;
}

function mergeAgentStatusUpdates(current: AgentStatusUpdate, next: AgentStatusUpdate): AgentStatusUpdate {
  const lastOutputAt = latestIso(current.last_output_at, next.last_output_at);
  const nextOutputIsLatest = !!next.last_output_at && lastOutputAt === next.last_output_at;
  return {
    status: next.status,
    source: next.source,
    last_input_at: latestIso(current.last_input_at, next.last_input_at),
    last_output_at: lastOutputAt,
    last_output_source: nextOutputIsLatest ? next.last_output_source : current.last_output_source,
  };
}

interface InternalCreateSessionOptions {
  title?: string;
  persistent?: boolean;
  execArgv?: string[];
  execCwd?: string;
  workspace?: WorkspaceName;
  agentId?: string;
  agentRole?: AgentSessionRole;
  agentSessionName?: string;
}

interface DeleteSessionOptions {
  closeCode?: number;
  closeReason?: string;
}

export class SessionBroker extends EventEmitter {
  private sessions = new Map<string, Session>();
  private scrollback = new Map<string, string>();
  private launchGenerations = new Map<string, number>();
  private pendingAgentStatusUpdates = new Map<string, PendingAgentStatusUpdate>();
  private agentStatusFlushTimers = new Map<string, ReturnType<typeof setTimeout>>();
  private agentStatusWrites = new Map<string, Promise<void>>();
  private static readonly SCROLLBACK_SIZE = 64 * 1024;
  private static AGENT_ATTACH_REPLAY_SUPPRESS_MS = 1500;
  private static AGENT_STATUS_FLUSH_DEBOUNCE_MS = 200;

  constructor() {
    super();
  }

  async initialize(): Promise<void> {
    const saved = persistence.loadSessions();
    saved.forEach(s => {
      this.sessions.set(s.id, s);
    });
    console.log(`Loaded ${saved.length} sessions from persistence`);
    await this.enforceAgentAccessPolicy();

    // Auto-reconnect persistent sessions that were previously active
    const reconnectable = saved.filter(s => s.persistent && s.hostname && !isAgentSession(s));
    if (reconnectable.length > 0) {
      console.log(`Auto-reconnecting ${reconnectable.length} persistent sessions...`);
      for (const session of reconnectable) {
        try {
          session.state = 'connecting';
          session.updated_at = new Date().toISOString();
          this.scrollback.delete(session.id);
          const generation = this.bumpLaunchGeneration(session.id);
          const ptyProcess = transportLauncher.launch(session, undefined, session.key_id || undefined);
          this.wireEvents(session, ptyProcess, undefined, generation);
          console.log(`  reconnected: ${session.title} (${session.id})`);
        } catch (err) {
          session.state = 'error';
          session.updated_at = new Date().toISOString();
          console.error(`  failed to reconnect ${session.title}: ${(err as Error).message}`);
        }
      }
      this.persistSessions();
    }
  }

  shutdown(): void {
    console.log('Persisting session state before shutdown...');
    for (const session of this.sessions.values()) {
      if (session.state === 'connected' || session.state === 'connecting') {
        session.state = 'disconnected';
        session.updated_at = new Date().toISOString();
      }
      this.bumpLaunchGeneration(session.id);
      transportLauncher.kill(session.id);
    }
    this.persistSessions();
  }

  async create(req: CreateSessionRequest, owner: string = 'anonymous', internal: InternalCreateSessionOptions = {}): Promise<Session> {
    const id = uuidv4();

    // Determine hostname
    let hostname = req.hostname || '';
    let port = req.port || 22;
    if (req.host_id) {
      try {
        const hostsConfig = persistence.loadHosts();
        const hostEntry = hostsConfig.hosts.find(h => h.id === req.host_id);
        if (hostEntry) {
          hostname = hostEntry.hostname;
          port = hostEntry.port;
        }
      } catch {
        // hosts.yaml not available
      }
    }

    // Determine layout position (scoped to this owner's sessions)
    const ownerSessions = Array.from(this.sessions.values()).filter(s => s.owner === owner && !isAgentSession(s));
    const { row, col } = internal.agentId
      ? { row: req.row ?? 0, col: req.col ?? 0 }
      : nextTerminalGridPosition(ownerSessions, req.row, req.col);

    // Determine transport: use mosh if host allows it and config prefers it
    let transport = req.transport || 'ssh';
    if (transport === 'ssh' && req.host_id) {
      try {
        const appConfig = persistence.loadApp();
        const hostsConfig = persistence.loadHosts();
        const hostEntry = hostsConfig.hosts.find(h => h.id === req.host_id);
        if (appConfig.app.transport.prefer_mosh && hostEntry?.mosh_allowed) {
          transport = 'mosh';
        }
      } catch {
        // config not available, stick with ssh
      }
    }

    const session: Session = {
      id,
      kind: 'terminal',
      owner,
      transport,
      host_id: req.host_id || '',
      hostname,
      port,
      username: req.username,
      key_id: req.key_id || '',
      exec_command: req.exec_command,
      exec_argv: internal.execArgv,
      exec_cwd: internal.execCwd,
      cols: req.cols || 80,
      rows: req.rows || 24,
      row,
      col,
      state: 'connecting',
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
      title: internal.title ?? (transport === 'exec' ? `${hostname}:${port}` : `${req.username}@${hostname}`),
      persistent: internal.persistent ?? true,
      minimized: false,
      workspace: internal.workspace,
      agent_id: internal.agentId,
      agent_role: internal.agentRole,
      agent_session_name: internal.agentSessionName,
    };

    this.sessions.set(id, session);

    // Launch the PTY process (state stays 'connecting' until first data arrives)
    try {
      const generation = this.bumpLaunchGeneration(session.id);
      const ptyProcess = transportLauncher.launch(session, req.password, req.key_id);
      // Resolve initial command: explicit > template lookup
      let initialCmd = req.initial_cmd;
      if (!initialCmd && req.template_id) {
        // Lazy import to avoid circular dep
        const { TEMPLATES } = await import('../api/templates.js');
        const tpl = TEMPLATES.find(t => t.id === req.template_id);
        if (tpl?.initialCmd) initialCmd = tpl.initialCmd;
      }
      this.wireEvents(session, ptyProcess, initialCmd, generation);
      this.markAgentAttachReady(session);
    } catch (err) {
      session.state = 'error';
      session.updated_at = new Date().toISOString();
      console.error(`Failed to launch session ${id}:`, err);
    }

    this.persistSessions();
    await persistence.appendEvent({ type: 'session_created', session_id: id, hostname, username: req.username });
    this.emit('session_created', session);
    return session;
  }

  private bumpLaunchGeneration(sessionId: string): number {
    const next = (this.launchGenerations.get(sessionId) ?? 0) + 1;
    this.launchGenerations.set(sessionId, next);
    return next;
  }

  private isCurrentLaunch(sessionId: string, generation: number): boolean {
    return this.launchGenerations.get(sessionId) === generation && this.sessions.has(sessionId);
  }

  private markConnected(session: Session, broadcast = false): boolean {
    const changed = session.state !== 'connected';
    if (changed) {
      session.state = 'connected';
      session.updated_at = new Date().toISOString();
    }
    if (broadcast) {
      presenceService.broadcastToSession(session.id, {
        type: 'status',
        session_id: session.id,
        state: 'connected',
      });
    }
    return changed;
  }

  private markAgentAttachReady(session: Session, broadcast = false): boolean {
    if (session.agent_role !== 'attach') return false;
    return this.markConnected(session, broadcast);
  }

  private wireEvents(session: Session, ptyProcess: pty.IPty, initialCmd: string | undefined, generation: number): void {
    let firstData = true;
    let cmdInjected = false;
    const suppressAgentOutputUntil = session.agent_role === 'attach'
      ? Date.now() + SessionBroker.AGENT_ATTACH_REPLAY_SUPPRESS_MS
      : 0;

    ptyProcess.onData((data: string) => {
      if (!this.isCurrentLaunch(session.id, generation)) return;
      if (firstData) {
        firstData = false;
        this.markConnected(session, true);
        // Inject initial command after a brief delay so the shell prompt is ready
        if (initialCmd && !cmdInjected) {
          cmdInjected = true;
          setTimeout(() => {
            if (!this.isCurrentLaunch(session.id, generation)) return;
            try { ptyProcess.write(`${initialCmd}\r`); }
            catch { /* session may have closed */ }
          }, 800);
        }
        this.persistSessions();
      }

      // Accumulate scrollback for late-joining viewers. When trimming, advance
      // past the next newline so a replay doesn't begin in the middle of an ANSI
      // escape sequence or a multi-byte UTF-8 codepoint.
      let buf = (this.scrollback.get(session.id) || '') + data;
      if (buf.length > SessionBroker.SCROLLBACK_SIZE) {
        buf = buf.slice(buf.length - SessionBroker.SCROLLBACK_SIZE);
        const nl = buf.indexOf('\n');
        if (nl !== -1 && nl < 4096) {
          buf = buf.slice(nl + 1);
        }
      }
      this.scrollback.set(session.id, buf);

      presenceService.broadcastToSession(session.id, {
        type: 'output',
        session_id: session.id,
        data,
      });
      this.recordAgentActivity(session, 'output', Date.now() < suppressAgentOutputUntil);
    });

    ptyProcess.onExit(({ exitCode }: { exitCode: number }) => {
      if (!this.isCurrentLaunch(session.id, generation)) return;
      session.state = 'disconnected';
      session.updated_at = new Date().toISOString();
      presenceService.broadcastToSession(session.id, {
        type: 'status',
        session_id: session.id,
        state: 'disconnected',
        message: `Process exited with code ${exitCode}`,
      });
      this.persistSessions();
      persistence.appendEvent({ type: 'session_exited', session_id: session.id, exit_code: exitCode });
    });
  }

  async reconnect(sessionId: string, password?: string): Promise<Session> {
    const session = this.sessions.get(sessionId);
    if (!session) throw new Error(`Session ${sessionId} not found`);
    if (isAgentSession(session)) {
      const access = getAgentAccess(session.agent_id);
      if (!access.allowed) {
        if (access.status !== 500) {
          await this.delete(sessionId, { closeCode: 1008, closeReason: access.error });
        }
        throw new AgentAccessError(access);
      }
    }

    if (transportLauncher.isAlive(sessionId)) {
      this.bumpLaunchGeneration(sessionId);
      transportLauncher.kill(sessionId);
    }

    session.state = 'connecting';
    session.updated_at = new Date().toISOString();

    try {
      this.scrollback.delete(session.id);
      const generation = this.bumpLaunchGeneration(session.id);
      const ptyProcess = transportLauncher.launch(session, password, session.key_id || undefined);
      this.wireEvents(session, ptyProcess, undefined, generation);
    } catch (err) {
      session.state = 'error';
      session.updated_at = new Date().toISOString();
      throw err;
    }

    this.persistSessions();
    return session;
  }

  getScrollback(sessionId: string): string {
    return this.scrollback.get(sessionId) || '';
  }

  async delete(sessionId: string, options: DeleteSessionOptions = {}): Promise<void> {
    const session = this.sessions.get(sessionId);
    const owner = session?.owner;
    const wasAgentSession = isAgentSession(session);
    this.bumpLaunchGeneration(sessionId);
    presenceService.closeSession(sessionId, options.closeCode ?? 1000, options.closeReason ?? 'Session deleted');
    transportLauncher.kill(sessionId);
    this.sessions.delete(sessionId);
    this.launchGenerations.delete(sessionId);
    this.scrollback.delete(sessionId);
    if (owner && !wasAgentSession) {
      const ownerSessions = Array.from(this.sessions.values()).filter(s => s.owner === owner && !isAgentSession(s));
      compactPositions(ownerSessions);
    }
    this.persistSessions();
    this.updateLayout(sessionId);
    await persistence.appendEvent({ type: 'session_deleted', session_id: sessionId });
    this.emit('session_deleted', sessionId);
  }

  get(sessionId: string): Session | undefined {
    return this.sessions.get(sessionId);
  }

  list(): Session[] {
    return Array.from(this.sessions.values());
  }

  listByOwner(owner: string): Session[] {
    return Array.from(this.sessions.values()).filter(s => s.owner === owner && !isAgentSession(s));
  }

  listAgentByOwner(owner: string, agentId?: string): Session[] {
    return Array.from(this.sessions.values()).filter(s => {
      if (s.owner !== owner || !isAgentSession(s)) return false;
      return !agentId || s.agent_id === agentId;
    });
  }

  async deleteAgentSessions(reason = 'Agent sessions are no longer allowed'): Promise<void> {
    const sessions = Array.from(this.sessions.values()).filter(isAgentSession);
    for (const session of sessions) {
      await this.delete(session.id, { closeCode: 1008, closeReason: reason });
    }
  }

  async enforceAgentAccessPolicy(): Promise<void> {
    const sessions = Array.from(this.sessions.values()).filter(isAgentSession);
    for (const session of sessions) {
      const access = getAgentAccess(session.agent_id);
      if (!access.allowed && access.status !== 500) {
        await this.delete(session.id, { closeCode: 1008, closeReason: access.error });
      }
    }
  }

  findAgentAttach(owner: string, agentId: string, name?: string): Session | undefined {
    const attachSessions = this.listAgentByOwner(owner, agentId).filter(s => s.agent_role === 'attach');
    if (name) {
      return attachSessions.find(s => s.agent_session_name === name) ?? attachSessions[0];
    }
    return attachSessions[0];
  }

  findAgentScratch(owner: string, agentId: string): Session | undefined {
    return this.listAgentByOwner(owner, agentId).find(s => s.agent_role === 'scratch');
  }

  async ensureAgentAttach(
    owner: string,
    agentId: string,
    workspace: WorkspaceName,
    name: string,
    cols: number,
    rows: number,
    execArgv: string[],
  ): Promise<{ session: Session; created: boolean }> {
    const attachSessions = this.listAgentByOwner(owner, agentId).filter(s => s.agent_role === 'attach');
    const existing = attachSessions.find(s => s.agent_session_name === name) ?? attachSessions[0];
    if (existing) {
      const shouldRelaunch = existing.agent_session_name !== name;
      const shouldResize = existing.cols !== cols || existing.rows !== rows;
      const commandChanged = !argvEqual(existing.exec_argv, execArgv);
      const metadataChanged =
        existing.title !== name ||
        existing.workspace !== workspace ||
        commandChanged ||
        existing.agent_id !== agentId ||
        existing.agent_role !== 'attach' ||
        existing.agent_session_name !== name;
      const needsRelaunch = shouldRelaunch || commandChanged || !transportLauncher.isAlive(existing.id) ||
        existing.state === 'disconnected' || existing.state === 'error';
      for (const stale of attachSessions) {
        if (stale.id !== existing.id) {
          await this.delete(stale.id);
        }
      }
      if (!needsRelaunch && !shouldResize && !metadataChanged) {
        if (this.markAgentAttachReady(existing, true)) {
          this.persistSessions();
        }
        return { session: existing, created: false };
      }
      existing.cols = cols;
      existing.rows = rows;
      existing.title = name;
      existing.workspace = workspace;
      existing.exec_argv = execArgv;
      existing.agent_id = agentId;
      existing.agent_role = 'attach';
      existing.agent_session_name = name;
      existing.updated_at = new Date().toISOString();
      if (needsRelaunch) {
        this.relaunch(existing);
      } else if (shouldResize) {
        transportLauncher.resize(existing.id, cols, rows);
        this.markAgentAttachReady(existing, true);
      } else {
        this.markAgentAttachReady(existing, true);
      }
      this.persistSessions();
      return { session: existing, created: false };
    }

    const session = await this.create({
      username: agentId,
      hostname: `${agentId}.local`,
      port: 0,
      transport: 'exec',
      cols,
      rows,
      row: 0,
      col: 0,
    }, owner, {
      title: name,
      persistent: false,
      execArgv,
      workspace,
      agentId,
      agentRole: 'attach',
      agentSessionName: name,
    });
    return { session, created: true };
  }

  async ensureAgentScratch(
    owner: string,
    agentId: string,
    workspace: WorkspaceName,
    cols: number,
    rows: number,
    cwd?: string,
  ): Promise<{ session: Session; created: boolean }> {
    const shell = process.env.SHELL?.trim() || '/bin/sh';
    const execArgv = [shell, '-l'];
    const existing = this.findAgentScratch(owner, agentId);
    if (existing) {
      existing.cols = cols;
      existing.rows = rows;
      existing.workspace = workspace;
      existing.exec_argv = execArgv;
      existing.exec_cwd = cwd;
      existing.agent_id = agentId;
      existing.agent_role = 'scratch';
      existing.agent_session_name = undefined;
      existing.updated_at = new Date().toISOString();
      if (!transportLauncher.isAlive(existing.id) || existing.state === 'disconnected' || existing.state === 'error') {
        this.relaunch(existing);
      } else {
        transportLauncher.resize(existing.id, cols, rows);
      }
      this.persistSessions();
      return { session: existing, created: false };
    }

    const session = await this.create({
      username: 'shell',
      hostname: 'local.shell',
      port: 0,
      transport: 'exec',
      cols,
      rows,
      row: 0,
      col: 1,
    }, owner, {
      title: 'Scratch shell',
      persistent: false,
      execArgv,
      execCwd: cwd,
      workspace,
      agentId,
      agentRole: 'scratch',
    });
    return { session, created: true };
  }

  private relaunch(session: Session): void {
    const generation = this.bumpLaunchGeneration(session.id);
    transportLauncher.kill(session.id);
    session.state = 'connecting';
    session.updated_at = new Date().toISOString();
    this.scrollback.delete(session.id);
    try {
      const ptyProcess = transportLauncher.launch(session, undefined, session.key_id || undefined);
      this.wireEvents(session, ptyProcess, undefined, generation);
      this.markAgentAttachReady(session, true);
    } catch (err) {
      session.state = 'error';
      session.updated_at = new Date().toISOString();
      throw err;
    }
  }

  move(sessionId: string, row: number, col: number): Session {
    const session = this.sessions.get(sessionId);
    if (!session) throw new Error(`Session ${sessionId} not found`);
    if (isAgentSession(session)) throw new Error('Agent workspace sessions cannot be moved');
    assertTerminalGridPosition(row, col);
    session.row = row;
    session.col = col;
    session.updated_at = new Date().toISOString();
    this.persistSessions();
    return session;
  }

  rename(sessionId: string, title: string): Session {
    const session = this.sessions.get(sessionId);
    if (!session) throw new Error(`Session ${sessionId} not found`);
    session.title = title;
    session.updated_at = new Date().toISOString();
    this.persistSessions();
    return session;
  }

  setMinimized(sessionId: string, minimized: boolean): Session {
    const session = this.sessions.get(sessionId);
    if (!session) throw new Error(`Session ${sessionId} not found`);
    session.minimized = minimized;
    session.updated_at = new Date().toISOString();
    this.persistSessions();
    return session;
  }

  resize(sessionId: string, cols: number, rows: number): void {
    const session = this.sessions.get(sessionId);
    if (!session) return;
    session.cols = cols;
    session.rows = rows;
    session.updated_at = new Date().toISOString();
    transportLauncher.resize(sessionId, cols, rows);
  }

  sendInput(sessionId: string, data: string): void {
    const handle = transportLauncher.getHandle(sessionId);
    if (handle) {
      handle.write(data);
      const session = this.sessions.get(sessionId);
      if (session) this.recordAgentActivity(session, 'input');
    }
  }

  private recordAgentActivity(session: Session, activity: 'input' | 'output', replayOutput = false): void {
    const agentId = session.agent_id;
    const name = session.agent_session_name;
    if (!agentId || session.agent_role !== 'attach' || !name) return;
    if (activity === 'output' && replayOutput) return;

    const now = new Date().toISOString();
    const update = activity === 'input'
      ? { status: 'working' as const, source: 'webmux' as const, last_input_at: now }
      : { status: 'working' as const, source: 'webmux' as const, last_output_at: now, last_output_source: 'live' as const };
    this.queueAgentStatusUpdate(agentId, name, update);
  }

  private queueAgentStatusUpdate(agentId: string, name: string, update: AgentStatusUpdate): void {
    const key = agentStatusKey(agentId, name);
    const pending = this.pendingAgentStatusUpdates.get(key);
    this.pendingAgentStatusUpdates.set(key, pending
      ? { ...pending, update: mergeAgentStatusUpdates(pending.update, update) }
      : { agentId, name, update });

    const existingTimer = this.agentStatusFlushTimers.get(key);
    if (existingTimer) clearTimeout(existingTimer);

    const timer = setTimeout(() => this.flushAgentStatusUpdate(key), SessionBroker.AGENT_STATUS_FLUSH_DEBOUNCE_MS);
    this.agentStatusFlushTimers.set(key, timer);
  }

  private flushAgentStatusUpdate(key: string): void {
    const timer = this.agentStatusFlushTimers.get(key);
    if (timer) {
      clearTimeout(timer);
      this.agentStatusFlushTimers.delete(key);
    }

    const pending = this.pendingAgentStatusUpdates.get(key);
    if (!pending) return;
    this.pendingAgentStatusUpdates.delete(key);

    const previousWrite = this.agentStatusWrites.get(key) ?? Promise.resolve();
    const write = previousWrite
      .catch(() => undefined)
      .then(() => agentService.recordStatus(pending.agentId, pending.name, pending.update))
      .catch(err => console.error(`Failed to record ${pending.agentId} agent status:`, err))
      .finally(() => {
        if (this.agentStatusWrites.get(key) === write) {
          this.agentStatusWrites.delete(key);
        }
      });
    this.agentStatusWrites.set(key, write);
  }

  private persistSessions(): void {
    const sessions = Array.from(this.sessions.values());
    persistence.saveSessions(sessions);

    try {
      const layout = persistence.loadLayout();
      layout.layout.tiles = sessions.filter(s => !isAgentSession(s)).map(s => ({
        session_id: s.id,
        row: s.row,
        col: s.col,
      }));
      persistence.saveLayout(layout);
    } catch {
      // Layout not yet initialized
    }
  }

  private updateLayout(removedSessionId: string): void {
    try {
      const layout = persistence.loadLayout();
      layout.layout.tiles = layout.layout.tiles.filter(t => t.session_id !== removedSessionId);
      persistence.saveLayout(layout);
    } catch {
      // ignore
    }
  }
}

export const sessionBroker = new SessionBroker();
