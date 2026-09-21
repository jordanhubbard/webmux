// Seed with the installed Node backend, then verify the same home after upgrade.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import net from 'node:net';
import { once } from 'node:events';
import { randomBytes } from 'node:crypto';
import { spawn, execFile } from 'node:child_process';
import { promisify } from 'node:util';

const [backend, phase, root, home] = process.argv.slice(2);
assert((backend === 'node' && phase === 'seed') || (backend === 'go' && phase === 'verify'));
assert(root && path.isAbsolute(root) && home && path.isAbsolute(home));
function record(value: unknown): Record<string, unknown> {
  assert(value && typeof value === 'object' && !Array.isArray(value));
  return value as Record<string, unknown>;
}
function textField(value: unknown, field: string): string {
  const result = record(value)[field]; assert(typeof result === 'string'); return result;
}
const snapshot = path.join(home, '.upgrade-fixture.json');
let saved: Record<string, unknown> = {};
if (phase === 'seed') {
  // Exclusive home creation prevents accidentally seeding operator state.
  await fs.mkdir(home, { mode: 0o700 });
  await fs.mkdir(path.join(home, 'config'));
  const app = (await fs.readFile(path.join(root, 'config.defaults/app.yaml'), 'utf8'))
    .replace('name: webmux', 'name: installer-state-fixture')
    .replace('listen_host: 0.0.0.0', 'listen_host: 127.0.0.1');
  await fs.writeFile(path.join(home, 'config/app.yaml'), app);
} else {
  saved = record(JSON.parse(await fs.readFile(snapshot, 'utf8')));
  for (const name of ['app', 'auth']) assert.equal(await fs.readFile(path.join(home, `config/${name}.yaml`), 'utf8'), textField(saved, name));
}
const listener = net.createServer(); listener.listen(0, '127.0.0.1'); await once(listener, 'listening');
const address = listener.address(); assert(address && typeof address === 'object');
await new Promise<void>((resolve, reject) => listener.close(error => error ? reject(error) : resolve()));
const command = backend === 'node' ? process.execPath : path.join(root, 'bin', process.platform === 'win32' ? 'webmux.exe' : 'webmux');
const args = backend === 'node' ? [path.join(root, 'backend/dist/index.js')] : [];
const child = spawn(command, args, { cwd: home, stdio: ['ignore', 'pipe', 'pipe'],
  env: { ...process.env, WEBMUX_ROOT: root, WEBMUX_HOME: home, HTTP_PORT: String(address.port), HTTPS_PORT: '0',
    JWT_SECRET: '', WEBMUX_SLAVE_HOST: '', WEBMUX_SLAVE_PORT: '' },
});
let launchError: Error | undefined;
child.on('error', error => { launchError = error; });
let logs = '';
child.stdout?.on('data', (data: Buffer) => { logs = (logs + data.toString()).slice(-4000); });
child.stderr?.on('data', (data: Buffer) => { logs = (logs + data.toString()).slice(-4000); });
const base = `http://127.0.0.1:${address.port}`;
async function request(method: string, route: string, body?: unknown, token?: string): Promise<unknown> {
  const response = await fetch(`${base}${route}`, { method, signal: AbortSignal.timeout(5000),
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  assert(response.ok, `${method} ${route}: ${response.status}`);
  return response.status === 204 ? undefined : response.json();
}
try {
  const deadline = Date.now() + 15000;
  for (;;) {
    if (launchError) throw launchError;
    assert(child.exitCode === null && child.signalCode === null, `Server exited: ${logs}`);
    let healthy = false;
    try { healthy = record(await request('GET', '/api/health')).name === 'installer-state-fixture'; } catch { /* starting */ }
    if (healthy) break;
    assert(Date.now() < deadline, `Server startup timed out: ${logs}`);
    await new Promise(resolve => setTimeout(resolve, 50));
  }
  const password = phase === 'seed' ? randomBytes(24).toString('hex') : textField(saved, 'password');
  if (phase === 'seed') await request('POST', '/api/auth/bootstrap', { username: 'upgrade-user', password });
  const token = textField(await request('POST', '/api/auth/login', { username: 'upgrade-user', password }), 'token');
  if (phase === 'seed') {
    await request('POST', '/api/hosts', { id: 'upgrade-host', hostname: 'upgrade.invalid', username: 'fixture', port: 2222 }, token);
    saved = { password, token, hosts: await request('GET', '/api/hosts', undefined, token) };
    for (const name of ['app', 'auth']) saved[name] = await fs.readFile(path.join(home, `config/${name}.yaml`), 'utf8');
    await fs.writeFile(snapshot, JSON.stringify(saved), { mode: 0o600, flag: 'wx' });
  } else {
    assert.deepEqual(await request('GET', '/api/hosts', undefined, token), saved.hosts, 'Saved hosts changed');
    assert.deepEqual(await request('GET', '/api/hosts', undefined, textField(saved, 'token')), saved.hosts, 'Pre-upgrade JWT stopped working');
    for (const name of ['app', 'auth']) assert.equal(await fs.readFile(path.join(home, `config/${name}.yaml`), 'utf8'), textField(saved, name));
  }
  console.log(`${backend}: upgrade ${phase} passed account login, saved hosts, signing-secret and configuration checks`);
} finally {
  if (child.exitCode === null && child.signalCode === null && child.pid !== undefined) {
    const closed = once(child, 'close');
    const timeout = setTimeout(() => child.kill('SIGKILL'), 10000);
    try {
      if (process.platform === 'win32') await promisify(execFile)('taskkill.exe', ['/PID', String(child.pid), '/T', '/F'], { timeout: 10000 });
      else child.kill('SIGTERM');
      const [code] = await closed;
      if (process.platform !== 'win32') assert.equal(code, 0);
    } finally { clearTimeout(timeout); }
  }
}
