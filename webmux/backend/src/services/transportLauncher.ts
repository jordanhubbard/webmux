import * as pty from 'node-pty';
import * as path from 'path';
import { Session } from '../types';
import { persistence } from './persistenceManager';
import { commandShell, homeDirectory, resolveExecutable } from './platform';

export interface PtyHandle {
  pty: pty.IPty;
  sessionId: string;
}

const HOSTNAME_RE = /^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9.])?$|^\[[0-9a-fA-F:]+\]$/;
const USERNAME_RE = /^[a-zA-Z0-9._-]+$/;

export class TransportLauncher {
  private handles = new Map<string, pty.IPty>();

  constructor(
    private readonly platform: NodeJS.Platform = process.platform,
    private readonly executableResolver: typeof resolveExecutable = resolveExecutable,
  ) {}

  static validateHostname(hostname: string): void {
    if (!hostname) {
      throw new Error('Hostname is required');
    }
    if (hostname.length > 255) {
      throw new Error('Hostname too long');
    }
    if (!HOSTNAME_RE.test(hostname)) {
      throw new Error(`Invalid hostname: ${hostname}`);
    }
  }

  static validateUsername(username: string): void {
    if (username.length > 64) {
      throw new Error('Username too long');
    }
    if (!USERNAME_RE.test(username)) {
      throw new Error(`Invalid username: ${username}`);
    }
  }

  static validateMoshServerPath(p: string): void {
    if (!p.startsWith('/')) {
      throw new Error('Invalid mosh_server_path: must be an absolute path');
    }
    if (p.length > 4096) {
      throw new Error('Invalid mosh_server_path: too long');
    }
    if (!/^[a-zA-Z0-9/_.-]+$/.test(p)) {
      throw new Error(`Invalid mosh_server_path: ${p}`);
    }
    // Reject ".." as a path component to block traversal (e.g. /usr/bin/../../etc/passwd)
    if (p.split('/').some(seg => seg === '..')) {
      throw new Error(`Invalid mosh_server_path: ${p}`);
    }
  }

  launch(session: Session, password?: string, keyId?: string): pty.IPty {
    // Validate inputs that flow into shell/command lines for every transport,
    // including exec (where {host}/{user} are substituted into a shell template).
    TransportLauncher.validateHostname(session.hostname);
    if (session.username) {
      TransportLauncher.validateUsername(session.username);
    }

    if (session.transport === 'exec') {
      return this.launchExec(session);
    }

    if (session.transport === 'mosh') {
      if (!this.executableResolver('mosh', process.env, this.platform)) {
        throw new Error('mosh is not installed on this system');
      }
      return this.launchMosh(session, keyId);
    }
    return this.launchSsh(session, password, keyId);
  }

  private launchExec(session: Session): pty.IPty {
    if (session.exec_argv && session.exec_argv.length > 0) {
      const [command, ...args] = session.exec_argv;
      if (!command || command.includes('\0')) {
        throw new Error('Invalid exec argv command');
      }
      if (args.some(arg => arg.includes('\0'))) {
        throw new Error('Invalid exec argv argument');
      }

      // ConPTY needs absolute executable paths. Resolve bare commands when they
      // are installed, but let node-pty report the launch error for optional
      // commands (for example a configured tmux binary) that are unavailable.
      const executable = this.resolvePtyExecutable(command);

      const ptyProcess = pty.spawn(executable, args, {
        name: 'xterm-256color',
        cols: session.cols,
        rows: session.rows,
        cwd: session.exec_cwd || homeDirectory(),
        env: { ...process.env, TERM: 'xterm-256color' },
      });

      this.handles.set(session.id, ptyProcess);
      return ptyProcess;
    }

    const template = session.exec_command || process.env.WEBMUX_EXEC_COMMAND || '';
    if (!template) {
      throw new Error('exec transport requires exec_command or WEBMUX_EXEC_COMMAND env var');
    }
    const cmd = template
      .replace(/\{host\}/g, session.hostname)
      .replace(/\{port\}/g, String(session.port))
      .replace(/\{user\}/g, session.username);

    const shell = commandShell(this.platform, process.env, this.executableResolver);
    const ptyProcess = pty.spawn(shell.command, [...shell.args, cmd], {
      name: 'xterm-256color',
      cols: session.cols,
      rows: session.rows,
      cwd: session.exec_cwd || homeDirectory(),
      env: { ...process.env, TERM: 'xterm-256color' },
    });

    this.handles.set(session.id, ptyProcess);
    return ptyProcess;
  }

  private launchSsh(session: Session, password?: string, keyId?: string): pty.IPty {
    const args = this.buildSshArgs(session, keyId);
    const ssh = this.requireExecutable('ssh');
    const env: NodeJS.ProcessEnv = {
      ...process.env,
      TERM: 'xterm-256color',
    };

    if (password) {
      const sshpass = this.executableResolver('sshpass', process.env, this.platform);
      if (!sshpass) {
        throw new Error('Password authentication requires sshpass to be installed on the jump box');
      }
      env['SSHPASS'] = password;
      const ptyProcess = pty.spawn(sshpass, ['-e', ssh, ...args], {
        name: 'xterm-256color',
        cols: session.cols,
        rows: session.rows,
        cwd: homeDirectory(),
        env,
      });
      this.handles.set(session.id, ptyProcess);
      return ptyProcess;
    }

    const ptyProcess = pty.spawn(ssh, args, {
      name: 'xterm-256color',
      cols: session.cols,
      rows: session.rows,
      cwd: homeDirectory(),
      env,
    });

    this.handles.set(session.id, ptyProcess);
    return ptyProcess;
  }

  private launchMosh(session: Session, keyId?: string): pty.IPty {
    const args = this.buildMoshArgs(session, keyId);
    const mosh = this.requireExecutable('mosh');
    const env: NodeJS.ProcessEnv = {
      ...process.env,
      TERM: 'xterm-256color',
    };

    const ptyProcess = pty.spawn(mosh, args, {
      name: 'xterm-256color',
      cols: session.cols,
      rows: session.rows,
      cwd: homeDirectory(),
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
      sshParts.push('-i', '"' + keyPath + '"');
    }
    args.push('--ssh=' + sshParts.join(' '));

    // Use configured mosh-server path if set (for hosts where it's not in PATH)
    try {
      const appConfig = persistence.loadApp();
      const serverPath = appConfig.app.transport.mosh_server_path;
      if (serverPath) {
        TransportLauncher.validateMoshServerPath(serverPath);
        args.push('--server=' + serverPath);
      }
    } catch (err) {
      if (err instanceof Error && err.message.startsWith('Invalid mosh_server_path')) throw err;
      // config not available
    }

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

  private requireExecutable(name: string): string {
    const pathImpl = this.platform === 'win32' ? path.win32 : path.posix;
    if (pathImpl.isAbsolute(name)) return name;
    const executable = this.executableResolver(name, process.env, this.platform);
    if (executable) return executable;
    throw new Error(`${name} is not installed or is not available on PATH`);
  }

  private resolvePtyExecutable(name: string): string {
    const pathImpl = this.platform === 'win32' ? path.win32 : path.posix;
    if (pathImpl.isAbsolute(name)) return name;
    return this.executableResolver(name, process.env, this.platform) || name;
  }

  resize(sessionId: string, cols: number, rows: number): void {
    const handle = this.handles.get(sessionId);
    if (handle) {
      try {
        handle.resize(cols, rows);
      } catch {
        // PTY may have already exited; remove stale handle
        this.handles.delete(sessionId);
      }
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
