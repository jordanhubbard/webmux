import { EventEmitter } from 'events';
import { v4 as uuidv4 } from 'uuid';
import * as pty from 'node-pty';
import { Session, CreateSessionRequest } from '../types';
import { transportLauncher } from './transportLauncher';
import { presenceService } from './presenceService';
import { persistence } from './persistenceManager';

export class SessionBroker extends EventEmitter {
  private sessions = new Map<string, Session>();
  private scrollback = new Map<string, string>();
  private static readonly SCROLLBACK_SIZE = 64 * 1024;

  constructor() {
    super();
  }

  async initialize(): Promise<void> {
    const saved = persistence.loadSessions();
    saved.forEach(s => {
      this.sessions.set(s.id, s);
    });
    console.log(`Loaded ${saved.length} sessions from persistence`);

    // Auto-reconnect persistent sessions that were previously active
    const reconnectable = saved.filter(s => s.persistent && s.hostname);
    if (reconnectable.length > 0) {
      console.log(`Auto-reconnecting ${reconnectable.length} persistent sessions...`);
      for (const session of reconnectable) {
        try {
          session.state = 'connecting';
          session.updated_at = new Date().toISOString();
          this.scrollback.delete(session.id);
          const ptyProcess = transportLauncher.launch(session, undefined, session.key_id || undefined);
          this.wireEvents(session, ptyProcess);
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
      transportLauncher.kill(session.id);
    }
    this.persistSessions();
  }

  async create(req: CreateSessionRequest, owner: string = 'anonymous'): Promise<Session> {
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
    const ownerSessions = Array.from(this.sessions.values()).filter(s => s.owner === owner);
    const { row, col } = this.nextPositionFor(ownerSessions, req.row, req.col);

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
      owner,
      transport,
      host_id: req.host_id || '',
      hostname,
      port,
      username: req.username,
      key_id: req.key_id || '',
      cols: req.cols || 80,
      rows: req.rows || 24,
      row,
      col,
      state: 'connecting',
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
      title: `${req.username}@${hostname}`,
      persistent: true,
    };

    this.sessions.set(id, session);

    // Launch the PTY process (state stays 'connecting' until first data arrives)
    try {
      const ptyProcess = transportLauncher.launch(session, req.password, req.key_id);
      this.wireEvents(session, ptyProcess);
    } catch (err) {
      session.state = 'error';
      session.updated_at = new Date().toISOString();
      console.error(`Failed to launch session ${id}:`, err);
    }

    this.persistSessions();
    persistence.appendEvent({ type: 'session_created', session_id: id, hostname, username: req.username });
    this.emit('session_created', session);
    return session;
  }

  private wireEvents(session: Session, ptyProcess: pty.IPty): void {
    let firstData = true;

    ptyProcess.onData((data: string) => {
      if (firstData) {
        firstData = false;
        session.state = 'connected';
        session.updated_at = new Date().toISOString();
        presenceService.broadcastToSession(session.id, {
          type: 'status',
          session_id: session.id,
          state: 'connected',
        });
        this.persistSessions();
      }

      // Accumulate scrollback for late-joining viewers
      let buf = (this.scrollback.get(session.id) || '') + data;
      if (buf.length > SessionBroker.SCROLLBACK_SIZE) {
        buf = buf.slice(buf.length - SessionBroker.SCROLLBACK_SIZE);
      }
      this.scrollback.set(session.id, buf);

      presenceService.broadcastToSession(session.id, {
        type: 'output',
        session_id: session.id,
        data,
      });
    });

    ptyProcess.onExit(({ exitCode }: { exitCode: number }) => {
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

    if (transportLauncher.isAlive(sessionId)) {
      transportLauncher.kill(sessionId);
    }

    session.state = 'connecting';
    session.updated_at = new Date().toISOString();

    try {
      this.scrollback.delete(session.id);
      const ptyProcess = transportLauncher.launch(session, password, session.key_id || undefined);
      this.wireEvents(session, ptyProcess);
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

  async delete(sessionId: string): Promise<void> {
    transportLauncher.kill(sessionId);
    this.sessions.delete(sessionId);
    this.scrollback.delete(sessionId);
    this.persistSessions();
    this.updateLayout(sessionId);
    persistence.appendEvent({ type: 'session_deleted', session_id: sessionId });
    this.emit('session_deleted', sessionId);
  }

  get(sessionId: string): Session | undefined {
    return this.sessions.get(sessionId);
  }

  list(): Session[] {
    return Array.from(this.sessions.values());
  }

  listByOwner(owner: string): Session[] {
    return Array.from(this.sessions.values()).filter(s => s.owner === owner);
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
    }
  }

  splitRight(sessionId: string): { row: number; col: number } {
    const session = this.sessions.get(sessionId);
    if (!session) throw new Error(`Session ${sessionId} not found`);
    return { row: session.row, col: session.col + 1 };
  }

  splitBelow(sessionId: string): { row: number; col: number } {
    const session = this.sessions.get(sessionId);
    if (!session) throw new Error(`Session ${sessionId} not found`);
    return { row: session.row + 1, col: session.col };
  }

  private nextPositionFor(sessions: Session[], requestedRow?: number, requestedCol?: number): { row: number; col: number } {
    if (requestedRow !== undefined && requestedCol !== undefined) {
      return { row: requestedRow, col: requestedCol };
    }

    if (sessions.length === 0) return { row: 0, col: 0 };

    const maxRow = Math.max(...sessions.map(s => s.row));
    const rowSessions = sessions.filter(s => s.row === maxRow);
    const maxCol = Math.max(...rowSessions.map(s => s.col));
    return { row: maxRow, col: maxCol + 1 };
  }

  private persistSessions(): void {
    const sessions = Array.from(this.sessions.values());
    persistence.saveSessions(sessions);

    try {
      const layout = persistence.loadLayout();
      layout.layout.tiles = sessions.map(s => ({
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
