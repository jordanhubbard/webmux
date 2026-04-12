import { EventEmitter } from 'events';
import { v4 as uuidv4 } from 'uuid';
import { VncSession, CreateVncSessionRequest, ConnectionState } from '../types';
import { nextPositionFor, compactPositions } from './gridLayout';
import { persistence } from './persistenceManager';

export class VncBroker extends EventEmitter {
  private sessions = new Map<string, VncSession>();
  private passwords = new Map<string, string>(); // sessionId → password, never persisted

  async initialize(): Promise<void> {
    const saved = persistence.loadVncSessions();
    saved.forEach(s => {
      // All persisted sessions are set to disconnected — reconnect requires user action
      s.state = 'disconnected';
      s.updated_at = new Date().toISOString();
      this.sessions.set(s.id, s);
    });
    console.log(`Loaded ${saved.length} VNC sessions from persistence`);
    // Persist the state reset so the file reflects 'disconnected' on next load.
    if (saved.length > 0) this.persistSessions();
  }

  async create(req: CreateVncSessionRequest, owner: string = 'anonymous'): Promise<VncSession> {
    const id = uuidv4();

    // Resolve hostname and vnc_port from host_id if provided
    let hostname = req.hostname || '';
    let vnc_port = req.vnc_port || 5900;
    if (req.host_id) {
      try {
        const hostsConfig = persistence.loadHosts();
        const hostEntry = hostsConfig.hosts.find(h => h.id === req.host_id);
        if (hostEntry) {
          hostname = hostEntry.hostname;
          if (hostEntry.vnc_port) {
            vnc_port = hostEntry.vnc_port;
          }
        }
      } catch {
        // hosts.yaml not available; use provided values
      }
    }

    // Determine grid position scoped to this owner's sessions
    const ownerSessions = Array.from(this.sessions.values()).filter(s => s.owner === owner);
    const { row, col } = nextPositionFor(ownerSessions, req.row, req.col);

    const session: VncSession = {
      id,
      kind: 'vnc',
      owner,
      host_id: req.host_id || '',
      hostname,
      vnc_port,
      row,
      col,
      state: 'connecting',
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
      title: `vnc://${hostname}:${vnc_port}`,
      persistent: true,
    };

    this.sessions.set(id, session);
    if (req.vnc_password) {
      this.passwords.set(id, req.vnc_password);
    }
    this.persistSessions();
    this.emit('vnc_session_created', session);
    return session;
  }

  get(id: string): VncSession | undefined {
    return this.sessions.get(id);
  }

  list(): VncSession[] {
    return Array.from(this.sessions.values());
  }

  listByOwner(owner: string): VncSession[] {
    return this.list().filter(s => s.owner === owner);
  }

  move(id: string, row: number, col: number): VncSession {
    const session = this.sessions.get(id);
    if (!session) throw new Error(`VncSession ${id} not found`);
    session.row = row;
    session.col = col;
    session.updated_at = new Date().toISOString();
    this.persistSessions();
    return session;
  }

  async delete(id: string): Promise<void> {
    const session = this.sessions.get(id);
    const owner = session?.owner;
    this.sessions.delete(id);
    this.passwords.delete(id);
    // Compact positions for remaining sessions belonging to the same owner
    if (owner) {
      const ownerSessions = Array.from(this.sessions.values()).filter(s => s.owner === owner);
      compactPositions(ownerSessions);
    }
    this.persistSessions();
    this.emit('vnc_session_deleted', id);
  }

  getPassword(id: string): string | undefined {
    return this.passwords.get(id);
  }

  setState(id: string, state: ConnectionState): void {
    const session = this.sessions.get(id);
    if (session) {
      session.state = state;
      session.updated_at = new Date().toISOString();
    }
  }

  shutdown(): void {
    this.persistSessions();
  }

  private persistSessions(): void {
    persistence.saveVncSessions(Array.from(this.sessions.values()));
  }
}

export const vncBroker = new VncBroker();
