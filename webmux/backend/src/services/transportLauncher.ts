import * as pty from 'node-pty';
import { Session } from '../types';

export interface PtyHandle {
  pty: pty.IPty;
  sessionId: string;
}

export class TransportLauncher {
  private handles = new Map<string, pty.IPty>();

  launch(session: Session, password?: string): pty.IPty {
    if (session.transport === 'ssh') {
      return this.launchSsh(session, password);
    }
    throw new Error(`Unsupported transport: ${session.transport}`);
  }

  private launchSsh(session: Session, password?: string): pty.IPty {
    const args = this.buildSshArgs(session);
    const env: NodeJS.ProcessEnv = {
      ...process.env,
      TERM: 'xterm-256color',
    };

    if (password) {
      // Use sshpass for password auth if available
      const sshpass = this.findSshpass();
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

  private buildSshArgs(session: Session): string[] {
    const args = [
      '-tt',
      '-o', 'ServerAliveInterval=15',
      '-o', 'ServerAliveCountMax=3',
      '-o', 'ConnectTimeout=10',
      '-o', 'TCPKeepAlive=yes',
      '-o', 'StrictHostKeyChecking=accept-new',
      '-p', String(session.port || 22),
    ];

    if (session.username) {
      args.push('-l', session.username);
    }

    args.push(session.hostname);
    return args;
  }

  private findSshpass(): string | null {
    try {
      const { execSync } = require('child_process');
      execSync('which sshpass', { stdio: 'ignore' });
      return 'sshpass';
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
