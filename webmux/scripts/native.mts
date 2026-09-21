import { spawn, spawnSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { constants } from 'node:os';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '..');
const binary = path.join(root, 'bin', process.platform === 'win32' ? 'webmux.exe' : 'webmux');
const [action, ...args] = process.argv.slice(2);
if (action === 'build') {
  if (args.length) throw new Error('Native build takes no arguments');
  mkdirSync(path.dirname(binary), { recursive: true });
  const result = spawnSync('go', ['build', '-trimpath', '-o', binary, './cmd/webmux'], {
    cwd: path.join(root, 'server'), stdio: 'inherit',
  });
  if (result.error) throw result.error;
  process.exitCode = result.status ?? 1;
} else if (action === 'start') {
  const child = spawn(binary, args, {
    cwd: root, stdio: 'inherit', env: { ...process.env, WEBMUX_ROOT: process.env.WEBMUX_ROOT || root },
  });
  for (const signal of ['SIGINT', 'SIGTERM'] as const) process.on(signal, () => child.kill(signal));
  child.on('error', error => {
    console.error(`Unable to start native WebMux; run npm run build first: ${error.message}`);
    process.exitCode = 1;
  });
  child.on('exit', (code, signal) => { process.exitCode = code ?? (signal ? 128 + constants.signals[signal] : 1); });
} else {
  throw new Error('Expected build or start');
}
