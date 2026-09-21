#!/usr/bin/env node
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import net from 'node:net';
import { execFile, spawn, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import { randomBytes } from 'node:crypto';
import { promisify, stripVTControlCharacters } from 'node:util';

function record(value: unknown): Record<string, unknown> {
  assert(value && typeof value === 'object' && !Array.isArray(value));
  return value as Record<string, unknown>;
}
function stringField(value: unknown, key: string): string {
  const field = record(value)[key]; assert.equal(typeof field, 'string'); return field as string;
}
async function until(check: () => boolean | Promise<boolean>, label: string | (() => string)): Promise<void> {
  const deadline = Date.now() + 15000;
  do {
    if (await check()) return;
    await new Promise(resolve => setTimeout(resolve, 50));
  } while (Date.now() < deadline);
  throw new Error(`Timed out: ${typeof label === 'function' ? label() : label}`);
}
async function stop(child: ChildProcess): Promise<void> {
  if (child.exitCode !== null || child.signalCode !== null) return;
  const closed = once(child, 'close');
  // Node cannot deliver a graceful SIGTERM on Windows. Kill the isolated
  // fixture tree so a failed PTY assertion cannot leave a shell holding cwd.
  const termination = process.platform === 'win32' && child.pid !== undefined
    ? promisify(execFile)('taskkill.exe', ['/PID', String(child.pid), '/T', '/F'], { timeout: 10000 })
    : Promise.resolve(child.kill('SIGTERM'));
  const timer = setTimeout(() => child.kill('SIGKILL'), 10000);
  try {
    await termination;
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
for (const excluded of ['node_modules', 'backend', 'config', 'data', 'package.json', 'web', 'config.defaults']) {
  assert(!fs.existsSync(path.join(root, excluded)), `Unexpected bundle entry: ${excluded}`);
}
const listener = net.createServer();
listener.listen(0, '127.0.0.1');
await once(listener, 'listening');
const address = listener.address(); assert(address && typeof address === 'object');
const port = address.port;
await new Promise<void>((resolve, reject) => listener.close(error => error ? reject(error) : resolve()));
const home = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-native-smoke-'));
const program = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-standalone-'));
const executable = path.join(program, process.platform === 'win32' ? 'webmux.exe' : 'webmux');
fs.copyFileSync(path.join(root, 'bin', path.basename(executable)), executable);
fs.chmodSync(executable, 0o700);
let app = '';
let previousToken: string | undefined;
const password = randomBytes(24).toString('hex');
let child: ChildProcess | undefined;
let socket: WebSocket | undefined;
let logs = '';
const failures: unknown[] = [];
try {
  for (let attempt = 0; attempt < 2; attempt++) {
    let launchError: Error | undefined;
    child = spawn(executable, ['--listen', `127.0.0.1:${port}`], {
      cwd: home, stdio: ['ignore', 'pipe', 'pipe'],
      env: { ...process.env, WEBMUX_HOME: home, WEBMUX_ROOT: '', HTTP_PORT: String(port), HTTPS_PORT: '0',
        JWT_SECRET: '', WEBMUX_SLAVE_HOST: '', WEBMUX_SLAVE_PORT: '', SHELL: '/bin/sh', WEBMUX_SMOKE_WORD: 'ok',
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
        return response.ok && value.status === 'ok' && value.name === (attempt === 0 ? 'webmux' : 'native-package-test');
      } catch { return false; }
    }, `HTTP startup: ${logs}`);
    const frontend = await fetch(base);
    assert.equal(frontend.status, 200);
    const html = await frontend.text(); assert.match(html, /<div id="root">/);
    const assets = [...html.matchAll(/(?:src|href)="(\/assets\/[^"?#]+)"/g)].map(match => match[1]);
    assert(assets.some(asset => asset.endsWith('.js')) && assets.some(asset => asset.endsWith('.css')), 'Missing bundled JS/CSS');
    for (const asset of assets) {
      const response = await fetch(base + asset);
      assert.equal(response.status, 200); assert((await response.arrayBuffer()).byteLength > 0);
      const etag = response.headers.get('etag'); assert(etag);
      const cached = await fetch(base + asset, { headers: { 'If-None-Match': etag, 'Cache-Control': 'max-age=0' } });
      assert.equal(cached.status, 304);
    }
    assert.match(await (await fetch(base + '/workspace/terminals')).text(), /<div id="root">/);
    for (const file of ['app.yaml', 'auth.yaml', 'hosts.yaml', 'keys.yaml', 'layout.yaml']) assert(fs.existsSync(path.join(home, 'config', file)));
    if (attempt === 0) app = fs.readFileSync(path.join(home, 'config/app.yaml'), 'utf8');
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
    if (previousToken) await request('GET', '/api/auth/me', undefined, previousToken);
    const token = stringField(await request('POST', '/api/auth/login', { username: 'package-user', password }), 'token');
    previousToken = token;
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
    // The expected marker is absent from the input, so terminal input echo
    // cannot pass this assertion. ConPTY may encode newlines as cursor moves.
    const command = process.platform === 'win32' ? 'echo native-pty-%WEBMUX_SMOKE_WORD%\r'
      : 'printf "%s%s\\n" native-pty- "$WEBMUX_SMOKE_WORD"\r';
    socket.send(JSON.stringify({ type: 'input', data: command }));
    await until(() => stripVTControlCharacters(output).includes('native-pty-ok'),
      () => `PTY command output: ${JSON.stringify(output)}`);
    await request('DELETE', `/api/sessions/${id}`, undefined, token);
    await until(() => socket?.readyState === WebSocket.CLOSED, 'Deleted terminal socket close');
    socket = undefined;
    assert.equal(fs.readFileSync(path.join(home, 'config/app.yaml'), 'utf8'), app);
    assert(fs.existsSync(path.join(home, 'config/auth.yaml')));
    await stop(child); child = undefined;
    if (attempt === 0) {
      app = app.replace('name: webmux', 'name: native-package-test');
      fs.writeFileSync(path.join(home, 'config/app.yaml'), app);
    }
  }
  for (const entry of ['config', 'data']) assert(!fs.existsSync(path.join(root, entry)), `Runtime wrote bundle/${entry}`);
  assert.deepEqual(fs.readdirSync(program), [path.basename(executable)], 'Runtime wrote beside the executable');
  console.log('Relocated standalone executable passed: HTTP, UI, persisted login, PTY/WebSocket, deletion, restart, config preservation.');
} catch (error) { console.error(logs); failures.push(error); }
finally {
  socket?.close();
  try { if (child && !child.killed) await stop(child); } catch (error) { failures.push(error); }
  try { fs.rmSync(program, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }); fs.rmSync(home, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }); }
  catch (error) { failures.push(error); }
}
if (failures.length > 0) throw new AggregateError(failures, 'Native bundle smoke test failed');
