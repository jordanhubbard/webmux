import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawn, spawnSync } from 'node:child_process';

// Own the socket outside Playwright's webServer process group: teardown can kill
// that whole group before start-server's child-exit callback gets a turn.
export async function runWithPrivateTmux(command: string, args: string[], options: {
  cwd: string;
  env: NodeJS.ProcessEnv;
}): Promise<number> {
  const directory = process.platform === 'win32' ? null : fs.mkdtempSync(path.join(os.tmpdir(), 'wm-tmux-'));
  const env = { ...options.env };
  if (directory) {
    env.TMUX_TMPDIR = directory;
    delete env.TMUX;
  }
  try {
    const child = spawn(command, args, { ...options, env, stdio: 'inherit' });
    let interrupted: number | undefined;
    let deadline: NodeJS.Timeout | undefined;
    const forward = (signal: 'SIGINT' | 'SIGTERM') => {
      interrupted = signal === 'SIGINT' ? 130 : 143;
      child.kill(signal);
      deadline ??= setTimeout(() => child.kill('SIGKILL'), 10_000).unref();
    };
    const interrupt = () => forward('SIGINT');
    const terminate = () => forward('SIGTERM');
    process.on('SIGINT', interrupt);
    process.on('SIGTERM', terminate);
    try {
      return await new Promise<number>((resolve, reject) => {
        child.once('error', reject);
        child.once('exit', (code, signal) => resolve(interrupted ?? code ?? (signal === 'SIGINT' ? 130 : signal === 'SIGTERM' ? 143 : 1)));
      });
    } finally {
      process.off('SIGINT', interrupt);
      process.off('SIGTERM', terminate);
      clearTimeout(deadline);
    }
  } finally {
    if (directory) {
      // An explicit socket avoids the operator's server even with inherited tmux
      // environment. Never hide a cleanup failure by unlinking a live socket.
      const socket = path.join(directory, `tmux-${process.getuid!()}`, 'default');
      const result = spawnSync('tmux', ['-S', socket, 'kill-server'], { env, encoding: 'utf8', timeout: 5000 });
      if ((result.error || result.status !== 0) && fs.existsSync(socket)) {
        throw new Error(`Cannot stop private test tmux at ${socket}: ${result.error?.message ?? result.stderr}`);
      }
      fs.rmSync(directory, { recursive: true, force: true });
    }
  }
}
