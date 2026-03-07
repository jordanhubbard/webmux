import * as pty from 'node-pty';
import { Session } from '../types';
import { persistence } from './persistenceManager';

export interface PtyHandle {
  pty: pty.IPty;
  sessionId: string;
}

export class TransportLauncher {
  private handles = new Map<string, pty.IPty>();

  launch(session: Session, password?: string, keyId?: string): pty.IPty {
    if (session.transport === 'mosh') {
      if (!this.findBinary('mosh')) {
        throw new Error('mosh is not installed on this system');
      }
      return this.launchMosh(session, keyId);
    }
    return this.launchSsh(session, password, keyId);
  }

  private launchSsh(session: Session, password?: string, keyId?: string): pty.IPty {
    const args = this.buildSshArgs(session, keyId);
    const env: NodeJS.ProcessEnv = {
      ...process.env,
      TERM: 'xterm-256color',
    };

    if (password) {
      // Use sshpass for password auth if available
      const sshpass = this.findBinary('sshpass');
      if (sshpass) {
        env['SSHPASS'] = password;
        const ptyProcess = pty.spawn('sshpass', ['-e', 'ssh', ...args], {
          name: 'xterm-256color',
          cols: session.cols,
          rows: session.rows,
          cwd: process.env.HOME || '/',
          env,
        });
        this.handles.set(session.id, ptyProcess);
        return ptyProcess;
      }
      // Fall back to SSH_ASKPASS
      env['SSH_ASKPASS'] = 'echo';
      env['SSH_ASKPASS_REQUIRE'] = 'force';
    }

    const ptyProcess = pty.spawn('ssh', args, {
      name: 'xterm-256color',
      cols: session.cols,
      rows: session.rows,
      cwd: process.env.HOME || '/',
      env,
    });

    this.handles.set(session.id, ptyProcess);
    return ptyProcess;
  }

  private launchMosh(session: Session, keyId?: string): pty.IPty {
    const args = this.buildMoshArgs(session, keyId);
    const env: NodeJS.ProcessEnv = {
      ...process.env,
      TERM: 'xterm-256color',
    };

    const ptyProcess = pty.spawn('mosh', args, {
      name: 'xterm-256color',
      cols: session.cols,
      rows: session.rows,
      cwd: process.env.HOME || '/',
      env,
    });

    this.handles.set(session.id, ptyProcess);
    return ptyProcess;
  }

  private buildSshArgs(session: Session, keyId?: string): string[] {
    const args = [
      '-tt',
      '-o', 'ServerAliveInterval=15',
      '-o', 'ServerAliveCountMax=3',
      '-o', 'ConnectTimeout=10',
      '-o', 'TCPKeepAlive=yes',
      '-o', 'StrictHostKeyChecking=accept-new',
      '-p', String(session.port || 22),
    ];

    const keyPath = this.resolveKeyPath(keyId);
    if (keyPath) {
      args.push('-i', keyPath);
    }

    if (session.username) {
      args.push('-l', session.username);
    }

    args.push(session.hostname);
    return args;
  }

  private buildMoshArgs(session: Session, keyId?: string): string[] {
    const args: string[] = [];

    // Pass SSH options via --ssh flag
    const sshParts = ['ssh'];
    sshParts.push('-o', 'StrictHostKeyChecking=accept-new');
    sshParts.push('-p', String(session.port || 22));
    const keyPath = this.resolveKeyPath(keyId);
    if (keyPath) {
      sshParts.push('-i', keyPath);
    }
    args.push('--ssh=' + sshParts.join(' '));

    if (session.username) {
      args.push(session.username + '@' + session.hostname);
    } else {
      args.push(session.hostname);
    }

    return args;
  }

  private resolveKeyPath(keyId?: string): string | null {
    if (!keyId) return null;
    try {
      const keysConfig = persistence.loadKeys();
      const keyEntry = keysConfig.keys.find(k => k.id === keyId);
      if (keyEntry) {
        return keyEntry.private_key_path;
      }
    } catch {
      // keys.yaml not available
    }
    return null;
  }

  private findBinary(name: string): string | null {
    if (!/^[a-zA-Z0-9_-]+$/.test(name)) return null;
    try {
      const { execSync } = require('child_process');
      execSync(`which ${name}`, { stdio: 'ignore' });
      return name;
    } catch {
      return null;
    }
  }

  resize(sessionId: string, cols: number, rows: number): void {
    const handle = this.handles.get(sessionId);
    if (handle) {
      handle.resize(cols, rows);
    }
  }

  kill(sessionId: string): void {
    const handle = this.handles.get(sessionId);
    if (handle) {
      handle.kill();
      this.handles.delete(sessionId);
    }
  }

  isAlive(sessionId: string): boolean {
    return this.handles.has(sessionId);
  }

  getHandle(sessionId: string): pty.IPty | undefined {
    return this.handles.get(sessionId);
  }
}

export const transportLauncher = new TransportLauncher();
