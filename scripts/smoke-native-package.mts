#!/usr/bin/env node
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import net from 'node:net';
import { spawn, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import { randomBytes } from 'node:crypto';
import { stripVTControlCharacters } from 'node:util';

function record(value: unknown): Record<string, unknown> {
  assert(value && typeof value === 'object' && !Array.isArray(value));
  return value as Record<string, unknown>;
}
function stringField(value: unknown, key: string): string {
  const field = record(value)[key]; assert.equal(typeof field, 'string'); return field as string;
}
async function until(check: () => boolean | Promise<boolean>, label: string): Promise<void> {
  const deadline = Date.now() + 15000;
  do {
    if (await check()) return;
    await new Promise(resolve => setTimeout(resolve, 50));
  } while (Date.now() < deadline);
  throw new Error(`Timed out: ${label}`);
}
async function stop(child: ChildProcess): Promise<void> {
  if (child.exitCode !== null || child.signalCode !== null) return;
  const closed = once(child, 'close');
  child.kill('SIGTERM');
  const timer = setTimeout(() => child.kill('SIGKILL'), 10000);
  try {
    const [code, signal] = await closed;
    if (process.platform !== 'win32') assert.equal(code, 0, `Shutdown: ${String(signal)}`);
    else assert(code !== null || signal);
  } finally { clearTimeout(timer); }
}

const argument = process.argv[2];
if (!argument) throw new Error('Usage: node scripts/smoke-native-package.mts <extracted-directory>');
const root = path.resolve(argument);
const manifest = record(JSON.parse(fs.readFileSync(path.join(root, 'bundle.json'), 'utf8')));
assert.equal(manifest.backend, 'go');
assert.equal(manifest.platform, process.platform); assert.equal(manifest.arch, process.arch);
for (const excluded of ['node_modules', 'backend', 'config', 'data', 'package.json']) {
  assert(!fs.existsSync(path.join(root, excluded)), `Unexpected bundle entry: ${excluded}`);
}
const listener = net.createServer();
listener.listen(0, '127.0.0.1');
await once(listener, 'listening');
const address = listener.address(); assert(address && typeof address === 'object');
const port = address.port;
await new Promise<void>((resolve, reject) => listener.close(error => error ? reject(error) : resolve()));
const home = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-native-smoke-'));
fs.mkdirSync(path.join(home, 'config'));
const app = fs.readFileSync(path.join(root, 'config.defaults/app.yaml'), 'utf8')
  .replace('name: webmux', 'name: native-package-test').replace('listen_host: 0.0.0.0', 'listen_host: 127.0.0.1');
fs.writeFileSync(path.join(home, 'config/app.yaml'), app);
const password = randomBytes(24).toString('hex');
let child: ChildProcess | undefined;
let socket: WebSocket | undefined;
let logs = '';
try {
  for (let attempt = 0; attempt < 2; attempt++) {
    let launchError: Error | undefined;
    child = spawn(path.join(root, 'bin', process.platform === 'win32' ? 'webmux.exe' : 'webmux'), [], {
      cwd: home, stdio: ['ignore', 'pipe', 'pipe'],
      env: { ...process.env, WEBMUX_HOME: home, WEBMUX_ROOT: '', HTTP_PORT: String(port), HTTPS_PORT: '0',
        JWT_SECRET: '', WEBMUX_SLAVE_HOST: '', WEBMUX_SLAVE_PORT: '', SHELL: '/bin/sh',
        WEBMUX_EXEC_COMMAND: process.platform === 'win32' ? 'cmd.exe /d /q' : '/bin/sh' },
    });
    child.on('error', error => { launchError = error; });
    child.stdout?.on('data', (data: Buffer) => { logs += data.toString(); });
    child.stderr?.on('data', (data: Buffer) => { logs += data.toString(); });
    const base = `http://127.0.0.1:${port}`;
    await until(async () => {
      if (launchError) throw launchError;
      if (child?.exitCode !== null || child?.signalCode !== null) throw new Error(`Server exited: ${logs}`);
      try {
        const response = await fetch(`${base}/api/health`, { signal: AbortSignal.timeout(1000) });
        const value = record(await response.json());
        return response.ok && value.status === 'ok' && value.name === 'native-package-test';
      } catch { return false; }
    }, `HTTP startup: ${logs}`);
    const frontend = await fetch(base);
    assert.equal(frontend.status, 200); assert.match(await frontend.text(), /<div id="root">/);
    assert.equal((await fetch(`${base}/api/sessions`)).status, 401);
    async function request(method: string, route: string, body?: unknown, token?: string): Promise<unknown> {
      const response = await fetch(`${base}${route}`, { method, signal: AbortSignal.timeout(5000),
        headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
      assert(response.ok, `${method} ${route}: ${response.status}`);
      return response.status === 204 ? undefined : response.json();
    }
    if (attempt === 0) await request('POST', '/api/auth/bootstrap', { username: 'package-user', password });
    const token = stringField(await request('POST', '/api/auth/login', { username: 'package-user', password }), 'token');
    const id = stringField(await request('POST', '/api/sessions', { hostname: 'localhost', username: 'fixture', transport: 'exec' }, token), 'id');
    let output = '';
    let wsError = false;
    socket = new WebSocket(`ws://127.0.0.1:${port}/api/term/${id}?token=${encodeURIComponent(token)}`);
    socket.addEventListener('error', () => { wsError = true; });
    socket.addEventListener('message', event => {
      const value = record(JSON.parse(String(event.data)));
      if (value.type === 'output' && typeof value.data === 'string') output += value.data;
    });
    await until(() => { assert(!wsError, 'WebSocket failed'); return socket?.readyState === WebSocket.OPEN; }, 'WebSocket open');
    socket.send(JSON.stringify({ type: 'input', data: 'echo native-pty-ok\r' }));
    await until(() => /(?:^|\n)native-pty-ok\r?\n/.test(stripVTControlCharacters(output)), 'PTY command output');
    await request('DELETE', `/api/sessions/${id}`, undefined, token);
    await until(() => socket?.readyState === WebSocket.CLOSED, 'Deleted terminal socket close');
    socket = undefined;
    assert.equal(fs.readFileSync(path.join(home, 'config/app.yaml'), 'utf8'), app);
    assert(fs.existsSync(path.join(home, 'config/auth.yaml')));
    await stop(child); child = undefined;
  }
  for (const entry of ['config', 'data']) assert(!fs.existsSync(path.join(root, entry)), `Runtime wrote bundle/${entry}`);
  console.log('Native bundle passed: HTTP, UI, persisted login, PTY/WebSocket, deletion, restart, config preservation.');
} catch (error) { console.error(logs); throw error; }
finally {
  socket?.close();
  try { if (child && !child.killed) await stop(child); }
  finally { fs.rmSync(home, { recursive: true, force: true }); }
}
