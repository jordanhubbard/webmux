import { EventEmitter } from 'events';
import { v4 as uuidv4 } from 'uuid';
import { RdpSession, CreateRdpSessionRequest, ConnectionState } from '../types';
import { nextPositionFor, compactPositions } from './gridLayout';
import { persistence } from './persistenceManager';

export class RdpBroker extends EventEmitter {
  private sessions = new Map<string, RdpSession>();
  private passwords = new Map<string, string>(); // never persisted

  async initialize(): Promise<void> {
    const saved = persistence.loadRdpSessions();
    saved.forEach(s => {
      s.state = 'disconnected';
      s.updated_at = new Date().toISOString();
      this.sessions.set(s.id, s);
    });
    console.log(`Loaded ${saved.length} RDP sessions from persistence`);
    if (saved.length > 0) this.persistSessions();
  }

  async create(req: CreateRdpSessionRequest, owner: string = 'anonymous'): Promise<RdpSession> {
    const id = uuidv4();
    let hostname = req.hostname || '';
    let rdp_port = req.rdp_port || 3389;
    const rdp_username = req.rdp_username || '';
    const rdp_domain = req.rdp_domain || '';

    if (req.host_id) {
      try {
        const hostsConfig = persistence.loadHosts();
        const hostEntry = hostsConfig.hosts.find(h => h.id === req.host_id);
        if (hostEntry) {
          hostname = hostEntry.hostname;
          if (hostEntry.rdp_port) rdp_port = hostEntry.rdp_port;
        }
      } catch {
        // hosts.yaml not available; use provided values
      }
    }

    const ownerSessions = Array.from(this.sessions.values()).filter(s => s.owner === owner);
    const { row, col } = nextPositionFor(ownerSessions, req.row, req.col);

    const session: RdpSession = {
      id,
      kind: 'rdp',
      owner,
      host_id: req.host_id || '',
      hostname,
      rdp_port,
      rdp_username,
      rdp_domain,
      row,
      col,
      state: 'connecting',
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
      title: `rdp://${hostname}:${rdp_port}`,
      persistent: true,
    };

    this.sessions.set(id, session);
    if (req.rdp_password) this.passwords.set(id, req.rdp_password);
    this.persistSessions();
    this.emit('rdp_session_created', session);
    return session;
  }

  get(id: string): RdpSession | undefined {
    return this.sessions.get(id);
  }

  list(): RdpSession[] {
    return Array.from(this.sessions.values());
  }

  listByOwner(owner: string): RdpSession[] {
    return this.list().filter(s => s.owner === owner);
  }

  move(id: string, row: number, col: number): RdpSession {
    const session = this.sessions.get(id);
    if (!session) throw new Error(`RdpSession ${id} not found`);
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
    if (owner) {
      const ownerSessions = Array.from(this.sessions.values()).filter(s => s.owner === owner);
      compactPositions(ownerSessions);
    }
    this.persistSessions();
    this.emit('rdp_session_deleted', id);
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
    persistence.saveRdpSessions(Array.from(this.sessions.values()));
  }
}

export const rdpBroker = new RdpBroker();
