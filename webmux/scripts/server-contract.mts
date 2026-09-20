import assert from 'node:assert/strict';
import { spawn, spawnSync, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import { cp, mkdir, mkdtemp, readFile, rename, rm, stat, utimes, writeFile } from 'node:fs/promises';
import { createServer, type Socket } from 'node:net';
import { createServer as createHTTPServer } from 'node:http';
import os from 'node:os';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import * as argon2 from 'argon2';
import jwt from 'jsonwebtoken';
import yaml from 'js-yaml';
import WebSocket from 'ws';

const root = path.resolve(import.meta.dirname, '..');
const temporary = await mkdtemp(path.join(os.tmpdir(), 'webmux-auth-contract-'));
const binary = path.join(temporary, process.platform === 'win32' ? 'webmux.exe' : 'webmux');
const agentFixtureDir = path.join(temporary, 'agent-bin');
const agentFixture = path.join(agentFixtureDir, process.platform === 'win32' ? 'tmux.exe' : 'tmux');
const secret = 'isolated-contract-fixture-not-a-production-secret';
type Backend = 'node' | 'go';
type Mode = 'local' | 'none';
type JSONRecord = Record<string, unknown>;

function record(value: unknown): JSONRecord {
  assert.ok(value !== null && typeof value === 'object' && !Array.isArray(value));
  return value as JSONRecord;
}

function stringField(value: unknown, field: string): string {
  const result = record(value)[field];
  assert.equal(typeof result, 'string', `missing string field ${field}`);
  return result as string;
}

async function reservePort(): Promise<number> {
  const server = createServer();
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const address = server.address();
  assert.ok(address && typeof address === 'object');
  await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
  return address.port;
}

async function stop(child: ChildProcess): Promise<void> {
  if (!child.pid || child.exitCode !== null || child.signalCode !== null) return;
  const exited = once(child, 'exit');
  child.kill('SIGTERM');
  const force = setTimeout(() => child.kill('SIGKILL'), 5_000);
  try { await exited; } finally { clearTimeout(force); }
}

interface RunningServer {
  home: string;
  socket: (route: string) => WebSocket;
  raw: (route: string, headers?: Record<string, string>, method?: string) => Promise<Response>;
  upload: (data: Uint8Array, name?: string, token?: string, contentType?: string) => Promise<Response>;
  request: (method: string, route: string, body?: JSONRecord, token?: string) => Promise<{ status: number; body: unknown }>;
  close: () => Promise<void>;
}

async function start(backend: Backend, mode: Mode, existingHome?: string, environment: NodeJS.ProcessEnv = {}, installationRoot = root): Promise<RunningServer> {
  const home = existingHome ?? await mkdtemp(path.join(temporary, `${backend}-${mode}-`));
  if (!existingHome) {
    await cp(path.join(root, 'config.defaults'), path.join(home, 'config'), { recursive: true });
    await writeFile(path.join(home, 'config', 'auth.yaml'), yaml.dump({ auth: { mode, users: [], jwt_secret: secret } }));
    const appPath = path.join(home, 'config', 'app.yaml');
    const app = await readFile(appPath, 'utf8');
    await writeFile(appPath, app.replace('listen_host: 0.0.0.0', 'listen_host: 127.0.0.1'));
  }
  const port = await reservePort();
  const child = spawn(backend === 'go' ? binary : process.execPath,
    backend === 'go' ? ['--root', installationRoot, '--home', home, '--listen', `127.0.0.1:${port}`] : [path.join(root, 'backend', 'dist', 'index.js')],
    {
      cwd: root,
      env: {
        ...process.env,
        WEBMUX_ROOT: installationRoot,
        WEBMUX_HOME: home,
        HTTP_PORT: String(port),
        HTTPS_PORT: '0',
        JWT_SECRET: '',
        WEBMUX_SLAVE_HOST: '',
        WEBMUX_SLAVE_PORT: '',
        WEBMUX_RCC_URL: '', WEBMUX_RCC_TOKEN: '', LOOM_RCC_BRAIN_URL: '', LOOM_RCC_AGENT_TOKEN: '',
        NVIDIA_API_KEY: '', OPENAI_API_KEY: '', WEBMUX_MODEL: '',
        WEBMUX_EXEC_COMMAND: undefined,
        WEBMUX_TERMINAL_GRID_MAX_COLS: undefined,
        WEBMUX_TERMINAL_GRID_MAX_ROWS: undefined,
        NODE_ENV: 'contract',
        ...environment,
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    });
  let logs = '';
  let startupError: Error | undefined;
  child.stdout?.on('data', (chunk: Buffer) => { logs = (logs + chunk.toString()).slice(-8_000); });
  child.stderr?.on('data', (chunk: Buffer) => { logs = (logs + chunk.toString()).slice(-8_000); });
  child.on('error', error => { startupError = error; });
  const base = `http://127.0.0.1:${port}`;
  try {
    let ready = false;
    for (let attempt = 0; attempt < 100; attempt++) {
      if (startupError) throw startupError;
      if (child.exitCode !== null || child.signalCode !== null) throw new Error(`${backend} exited during startup:\n${logs}`);
      try {
        const response = await fetch(`${base}/api/health`, { signal: AbortSignal.timeout(500) });
        ready = response.ok;
        await response.body?.cancel();
        if (ready) break;
      } catch { /* Server has not bound its socket yet. */ }
      await delay(100);
    }
    assert.ok(ready, `${backend} did not become healthy:\n${logs}`);
  } catch (error) {
    await stop(child);
    throw error;
  }
  return {
    home,
    socket: route => new WebSocket(base.replace('http:', 'ws:') + route),
    raw: (route, headers, method = 'GET') => fetch(base + route, { headers, method, redirect: 'manual', signal: AbortSignal.timeout(10_000) }),
    upload: (data, name, token, contentType = 'application/octet-stream') => fetch(base + '/api/upload', {
      method: 'POST', body: new Uint8Array(data).buffer,
      headers: { 'Content-Type': contentType, ...(name === undefined ? {} : { 'X-Filename': name }), ...(token ? { Authorization: `Bearer ${token}` } : {}) },
      signal: AbortSignal.timeout(15_000),
    }),
    async request(method, route, body, token) {
      const response = await fetch(base + route, {
        method,
        headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: AbortSignal.timeout(10_000),
      });
      return { status: response.status, body: response.status === 204 ? null : await response.json() as unknown };
    },
    close: () => stop(child),
  };
}

function checkToken(body: unknown, username: string, mode: Mode): string {
  const token = stringField(body, 'token');
  assert.equal(record(body).mode, mode);
  const claims = jwt.verify(token, secret, { algorithms: ['HS256'] });
  assert.ok(typeof claims === 'object');
  assert.equal(claims.sub, username);
  assert.equal(typeof claims.iat, 'number');
  assert.equal(typeof claims.exp, 'number');
  assert.equal(claims.exp! - claims.iat!, 8 * 60 * 60);
  return token;
}

let aiBaseline: unknown;
async function aiContract(backend: Backend): Promise<void> {
  const unavailable = { error: 'AI assistant unavailable', detail: 'No LLM API key configured', hint: 'Set WEBMUX_RCC_URL+WEBMUX_RCC_TOKEN or NVIDIA_API_KEY/OPENAI_API_KEY' };
  const plain = await start(backend, 'none');
  try {
    assert.deepEqual(await plain.request('GET', '/api/ai/status'), { status: 200, body: { available: false, providers: { rcc: false, nvidia: false, openai: false }, model: 'gpt-4o-mini' } });
    for (const message of [undefined, '', '  ', 42]) assert.deepEqual(await plain.request('POST', '/api/ai/chat', { message }), { status: 400, body: { error: 'message required' } });
    assert.deepEqual(await plain.request('POST', '/api/ai/chat', { message: 'help' }), { status: 503, body: unavailable });
  } finally { await plain.close(); }
  const requests: { url: string | undefined; authorization: string | undefined; body: JSONRecord }[] = [];
  let fail = false;
  let fixtureError: unknown;
  const provider = createHTTPServer((request, response) => {
    void (async () => {
      const chunks: Buffer[] = [];
      for await (const chunk of request) { assert.ok(Buffer.isBuffer(chunk)); chunks.push(chunk); }
      requests.push({ url: request.url, authorization: request.headers.authorization, body: record(JSON.parse(Buffer.concat(chunks).toString()) as unknown) });
      response.writeHead(fail ? 503 : 200, { 'Content-Type': 'application/json' });
      response.end(JSON.stringify(fail ? { error: 'fixture failure' } : { status: 'completed', result: '  fixture reply  ' }));
    })().catch((error: unknown) => { fixtureError = error; response.writeHead(500); response.end(); });
  });
  provider.listen(0, '127.0.0.1'); await once(provider, 'listening');
  const address = provider.address(); assert.ok(address && typeof address !== 'string');
  try {
    for (const primary of [false, true]) {
      fail = false;
      const environment: NodeJS.ProcessEnv = { LOOM_RCC_BRAIN_URL: `http://127.0.0.1:${address.port}/`, LOOM_RCC_AGENT_TOKEN: 'alias-fixture', WEBMUX_MODEL: 'fixture-model' };
      if (primary) { environment.WEBMUX_RCC_URL = `http://127.0.0.1:${address.port}/primary/`; environment.WEBMUX_RCC_TOKEN = 'primary-fixture'; }
      const server = await start(backend, 'local', undefined, environment);
      try {
        assert.equal((await server.request('GET', '/api/ai/status')).status, 401);
        assert.equal((await server.request('POST', '/api/ai/chat', { message: 'help' })).status, 401);
        const owner = stringField((await server.request('POST', '/api/auth/bootstrap', { username: 'owner', password: 'password' })).body, 'token');
        assert.deepEqual(await server.request('GET', '/api/ai/status', undefined, owner), { status: 200, body: { available: true, providers: { rcc: true, nvidia: false, openai: false }, model: 'fixture-model' } });
        const history = Array.from({ length: 11 }, (_, index) => ({ role: index === 5 ? 'system' : index % 2 ? 'user' : 'assistant', content: `history-${index}` }));
        const context = '😀' + 'x'.repeat(2999);
        const before = Date.now();
        const result = await server.request('POST', '/api/ai/chat', { message: ' \uFEFFhelp ', context, history, sessionId: 'client-context-only' }, owner);
        assert.equal(result.status, 200);
        const value = record(result.body);
        assert.equal(value.reply, '  fixture reply  '); assert.equal(value.source, 'rcc'); assert.equal(value.model, 'rcc-brain');
        // Node and Go use different wall-clock implementations on Windows;
        // millisecond samples need not be strictly ordered across processes.
        const after = Date.now();
        assert.ok(typeof value.ts === 'number' && Number.isSafeInteger(value.ts) && value.ts >= before - 1000 && value.ts <= after + 1000,
          `AI timestamp outside request window: ${JSON.stringify({ before, timestamp: value.ts, after })}`);
        const captured = requests.at(-1); assert.ok(captured);
        assert.equal(captured.url, primary ? '/primary/api/brain/request' : '/api/brain/request');
        assert.equal(captured.authorization, `Bearer ${primary ? 'primary' : 'alias'}-fixture`);
        const messages = captured.body.messages; assert.ok(Array.isArray(messages));
        assert.equal(messages.length, 11); assert.equal(record(messages[0]).role, 'system');
        assert.deepEqual(messages.slice(1, -1), history.slice(-10).filter(message => message.role !== 'system'));
        assert.deepEqual(messages.at(-1), { role: 'user', content: `<terminal_context>\n${context.slice(-3000)}\n</terminal_context>\n\nhelp` });
        if (backend === 'node') aiBaseline = captured.body;
        else assert.deepEqual(captured.body, aiBaseline, 'provider request parity');
        fail = true;
        assert.deepEqual(await server.request('POST', '/api/ai/chat', { message: 'help' }, owner), { status: 503, body: unavailable });
        assert.equal(fixtureError, undefined);
      } finally { await server.close(); }
    }
  } finally { await new Promise<void>((resolve, reject) => provider.close(error => error ? reject(error) : resolve())); }
}

async function uploadContract(backend: Backend): Promise<void> {
  const server = await start(backend, 'local');
  let referenced = '';
  let stale = '';
  try {
    const unauthorized = await server.upload(Buffer.from('no'));
    assert.equal(unauthorized.status, 401); await unauthorized.body?.cancel();
    const owner = stringField((await server.request('POST', '/api/auth/bootstrap', { username: 'owner', password: 'password' })).body, 'token');
    const invalid = await server.upload(Buffer.from('invalid'), 'file', owner, 'text/plain');
    assert.equal(invalid.status, 400); assert.deepEqual(await invalid.json(), { error: 'Content-Type must be application/octet-stream' });
    for (const [name, suffix, size] of [['file.pem', '-file.pem', 256], ['../../file.pem', '-file.pem', 0], ['unsafe file.pem', '.pem', 37], [undefined, '.bin', 10*1024*1024]] as const) {
      const data = Buffer.alloc(size, 255);
      const response = await server.upload(data, name, owner);
      assert.equal(response.status, 201);
      const value = record(await response.json());
      const file = stringField(value, 'path'); const storedName = stringField(value, 'name');
      assert.match(storedName.slice(0, 12), /^[0-9a-f]{12}$/); assert.ok(storedName.endsWith(suffix));
      assert.equal(file, path.join(server.home, 'uploads', storedName)); assert.equal(value.size, size);
      assert.deepEqual(await readFile(file), data);
      if (!referenced) referenced = file; else stale = file;
    }
    const oversized = await server.upload(Buffer.alloc(10*1024*1024+1), 'large.bin', owner);
    assert.equal(oversized.status, 413); assert.deepEqual(await oversized.json(), { error: 'File too large (max 10 MB)' });
    assert.equal((await server.request('POST', '/api/keys', { id: 'uploaded-key', private_key_path: referenced }, owner)).status, 201);
    const old = new Date(Date.now() - 31*24*60*60*1000);
    await utimes(referenced, old, old); await utimes(stale, old, old);
  } finally { await server.close(); }
  const restored = await start(backend === 'node' ? 'go' : 'node', 'local', server.home);
  try {
    const deadline = Date.now() + 5_000;
    while (await stat(stale).then(() => true, (error: NodeJS.ErrnoException) => { if (error.code !== 'ENOENT') throw error; return false; })) {
      assert.ok(Date.now() < deadline, 'startup did not purge stale upload'); await delay(20);
    }
    assert.deepEqual(await readFile(referenced), Buffer.alloc(256, 255));
  } finally { await restored.close(); }
}

let staticBaseline: unknown;
async function staticContract(backend: Backend): Promise<void> {
  const installation = await mkdtemp(path.join(temporary, `${backend}-static-`));
  await cp(path.join(root, 'config.defaults'), path.join(installation, 'config.defaults'), { recursive: true });
  await mkdir(path.join(installation, 'web', 'assets'), { recursive: true });
  const files = { 'index.html': '<!doctype html><title>WebMux</title>', 'assets/app.js': 'console.log("fixture");', 'assets/app.css': 'body{color:red}' };
  for (const [name, text] of Object.entries(files)) {
    const file = path.join(installation, 'web', name);
    await writeFile(file, text);
    await utimes(file, new Date('2025-01-01T00:00:00Z'), new Date('2025-01-01T00:00:00Z'));
  }
  const server = await start(backend, 'local', undefined, {}, installation);
  try {
    const responses: unknown[] = [];
    for (const route of ['/', '/index.html', '/workspace/terminals', '/assets/app.js', '/assets/app.css']) {
      const response = await server.raw(route);
      assert.equal(response.status, 200);
      responses.push({ route, body: await response.text(), headers: Object.fromEntries(['content-type', 'cache-control', 'etag', 'last-modified', 'accept-ranges'].map(name => [name, response.headers.get(name)])) });
    }
    const asset = await server.raw('/assets/app.js');
    const etag = asset.headers.get('etag'); assert.ok(etag); await asset.body?.cancel();
    const cached = await server.raw('/assets/app.js', { 'If-None-Match': etag, 'Cache-Control': 'max-age=0' });
    assert.equal(cached.status, 304);
    const range = await server.raw('/assets/app.js', { Range: 'bytes=0-6' });
    assert.equal(range.status, 206); assert.equal(await range.text(), 'console');
    const head = await server.raw('/assets/app.js', {}, 'HEAD');
    assert.equal(head.status, 200); assert.equal(await head.text(), '');
    assert.equal(Number(head.headers.get('content-length')), files['assets/app.js'].length);
    const redirect = await server.raw('/assets?version=1');
    assert.equal(redirect.status, 301); assert.equal(redirect.headers.get('location'), '/assets/?version=1'); await redirect.body?.cancel();
    assert.equal((await server.request('GET', '/api/sessions')).status, 401);
    if (backend === 'node') staticBaseline = responses;
    else assert.deepEqual(responses, staticBaseline, 'static frontend response parity');
  } finally { await server.close(); }
}

async function localContract(backend: Backend): Promise<void> {
  const server = await start(backend, 'local');
  let ownerToken: string;
  try {
    const call = server.request;
    assert.deepEqual(await call('GET', '/api/auth/status'), { status: 200, body: { mode: 'local', bootstrap_required: true } });
    assert.deepEqual(await call('GET', '/api/auth/me'), { status: 401, body: { error: 'Unauthorized' } });
    assert.deepEqual(await call('GET', '/api/hosts'), { status: 401, body: { error: 'Unauthorized' } });
    assert.deepEqual(await call('GET', '/api/keys'), { status: 401, body: { error: 'Unauthorized' } });
    assert.deepEqual(await call('GET', '/api/config'), { status: 401, body: { error: 'Unauthorized' } });
    assert.deepEqual(await call('POST', '/api/auth/login', {}), { status: 400, body: { error: 'Username and password required' } });
    const bootstrap = await call('POST', '/api/auth/bootstrap', { username: 'owner', password: 'owner-🔐-password' });
    assert.equal(bootstrap.status, 200);
    ownerToken = checkToken(bootstrap.body, 'owner', 'local');
    assert.deepEqual(await call('GET', '/api/auth/status'), { status: 200, body: { mode: 'local', bootstrap_required: false } });
    assert.deepEqual(await call('GET', '/api/auth/me', undefined, ownerToken), { status: 200, body: { username: 'owner', admin: true } });
    assert.deepEqual(await call('POST', '/api/auth/bootstrap', { username: 'other', password: 'password' }), { status: 403, body: { error: 'Bootstrap not available — accounts already exist' } });
    assert.deepEqual(await call('POST', '/api/auth/login', { username: 'owner', password: 'wrong' }), { status: 401, body: { error: 'Invalid credentials' } });
    const login = await call('POST', '/api/auth/login', { username: 'owner', password: 'owner-🔐-password' });
    assert.equal(login.status, 200);
    checkToken(login.body, 'owner', 'local');
    assert.deepEqual(await call('POST', '/api/auth/register', { username: 'member', password: 'member-password' }, ownerToken), { status: 201, body: { username: 'member', admin: false } });
    assert.deepEqual(await call('POST', '/api/auth/register', { username: 'member', password: 'member-password' }, ownerToken), { status: 409, body: { error: 'Username already exists' } });
    const memberLogin = await call('POST', '/api/auth/login', { username: 'member', password: 'member-password' });
    assert.equal(memberLogin.status, 200);
    const memberToken = checkToken(memberLogin.body, 'member', 'local');
    assert.deepEqual(await call('GET', '/api/auth/users', undefined, memberToken), { status: 403, body: { error: 'Admin privileges required' } });
    assert.equal((await call('GET', '/api/config', undefined, memberToken)).status, 200);
    assert.deepEqual(await call('PUT', '/api/config', { app: { name: 'not-allowed' } }, memberToken), { status: 403, body: { error: 'Admin privileges required' } });
    assert.equal((await call('PUT', '/api/config', { app: { name: 'owner-settings' } }, ownerToken)).status, 200);
    assert.equal((await call('PUT', '/api/config/layout', { layout: { font_size: 17, tiles: [] } }, memberToken)).status, 200);
    assert.deepEqual(await call('GET', '/api/auth/users', undefined, ownerToken), { status: 200, body: [{ username: 'owner', admin: true }, { username: 'member', admin: false }] });
    const refresh = await call('POST', '/api/auth/refresh', undefined, ownerToken);
    assert.equal(refresh.status, 200);
    checkToken(refresh.body, 'owner', 'local');
    const ticket = await call('POST', '/api/auth/ticket', undefined, ownerToken);
    assert.equal(ticket.status, 200);
    assert.match(stringField(ticket.body, 'ticket'), /^[a-f0-9]{48}$/);
    assert.equal(record(ticket.body).expires_in, 60);
    assert.deepEqual(await call('DELETE', '/api/auth/users/owner', undefined, ownerToken), { status: 400, body: { error: 'You cannot remove your own account' } });
    assert.deepEqual(await call('DELETE', '/api/auth/users/member', undefined, ownerToken), { status: 204, body: null });

    // Use the actual persisted hash with node-argon2, not a Go round-trip alone.
    const persisted = record(record(yaml.load(await readFile(path.join(server.home, 'config', 'auth.yaml'), 'utf8'))).auth);
    assert.equal(persisted.jwt_secret, secret);
    assert.ok(Array.isArray(persisted.users));
    assert.equal(persisted.users.length, 1);
    assert.ok(await argon2.verify(stringField(persisted.users[0], 'password_hash'), 'owner-🔐-password'));
  } finally { await server.close(); }

  // Restart the opposite implementation on the same files. The old token and
  // password must still work, in both directions, without a data conversion.
  const other = await start(backend === 'go' ? 'node' : 'go', 'local', server.home);
  try {
    assert.deepEqual(await other.request('GET', '/api/auth/me', undefined, ownerToken), { status: 200, body: { username: 'owner', admin: true } });
    const login = await other.request('POST', '/api/auth/login', { username: 'owner', password: 'owner-🔐-password' });
    assert.equal(login.status, 200);
    checkToken(login.body, 'owner', 'local');
  } finally { await other.close(); }
}

async function trustedContract(backend: Backend): Promise<void> {
  const server = await start(backend, 'none');
  try {
    assert.deepEqual(await server.request('GET', '/api/auth/status'), { status: 200, body: { mode: 'none', bootstrap_required: true } });
    assert.deepEqual(await server.request('GET', '/api/auth/me'), { status: 200, body: { username: 'anonymous', admin: false } });
    assert.deepEqual(await server.request('GET', '/api/auth/users'), { status: 401, body: { error: 'Unauthorized' } });
    const refresh = await server.request('POST', '/api/auth/refresh');
    assert.equal(refresh.status, 200);
    checkToken(refresh.body, 'anonymous', 'none');
    const login = await server.request('POST', '/api/auth/login', { username: 'operator', password: 'unused' });
    assert.equal(login.status, 200);
    checkToken(login.body, 'operator', 'none');
  } finally { await server.close(); }
}

async function catalogContract(backend: Backend): Promise<void> {
  const server = await start(backend, 'none');
  const hostID = 'fixture/with slash/';
  const hostRoute = `/api/hosts/${encodeURIComponent(hostID)}`;
  let savedHost: unknown;
  try {
    const call = server.request;
    assert.deepEqual(await call('GET', '/api/hosts/'), { status: 200, body: [] });
    assert.deepEqual(await call('GET', '/api/keys/'), { status: 200, body: [] });
    assert.deepEqual(await call('POST', '/api/hosts', {}), { status: 400, body: { error: 'hostname is required' } });
    assert.deepEqual(await call('POST', '/api/keys', {}), { status: 400, body: { error: 'private_key_path is required' } });
    const created = await call('POST', '/api/hosts/', { id: hostID, hostname: 'example.invalid', port: 0, vnc_port: 0, metadata: 'ignored on creation' });
    const expected = {
      id: hostID, hostname: 'example.invalid', port: 22, username: '', transport: 'ssh',
      key_id: '', tags: [], mosh_allowed: false, vnc_enabled: false, vnc_port: 0,
      rdp_enabled: false, rdp_port: 3389,
    };
    assert.deepEqual(created, { status: 201, body: expected });
    savedHost = { ...expected, username: 'operator', vnc_port: 5900, rdp_port: null, metadata: { purpose: 'contract' } };
    assert.deepEqual(await call('PUT', hostRoute, { id: 'cannot-replace-id', username: 'operator', vnc_port: null, rdp_port: null, metadata: { purpose: 'contract' } }), { status: 200, body: savedHost });
    assert.deepEqual(await call('GET', '/api/hosts'), { status: 200, body: [savedHost] });
    assert.deepEqual(await call('PUT', '/api/hosts/missing', {}), { status: 404, body: { error: 'Host not found' } });

    const key = await call('POST', '/api/keys/', { id: 'key-fixture', private_key_path: '/fixture/private/key', description: 'fixture' });
    const publicKey = { id: 'key-fixture', type: 'rsa', encrypted: false, description: 'fixture' };
    assert.deepEqual(key, { status: 201, body: publicKey });
    assert.deepEqual(await call('GET', '/api/keys'), { status: 200, body: [publicKey] });
    const persisted = record(yaml.load(await readFile(path.join(server.home, 'config', 'keys.yaml'), 'utf8')));
    assert.ok(Array.isArray(persisted.keys));
    assert.equal(record(persisted.keys[0]).private_key_path, '/fixture/private/key');
    assert.deepEqual(await call('DELETE', '/api/keys/missing'), { status: 404, body: { error: 'Key not found' } });
    // Exercise IDs generated by the implementation, not just supplied IDs.
    const generated = await call('POST', '/api/keys', { private_key_path: '/fixture/generated' });
    assert.equal(generated.status, 201);
    const generatedID = stringField(generated.body, 'id');
    assert.match(generatedID, /^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/);
    assert.deepEqual(await call('DELETE', `/api/keys/${generatedID}`), { status: 204, body: null });
  } finally { await server.close(); }

  const other = await start(backend === 'go' ? 'node' : 'go', 'none', server.home);
  try {
    assert.deepEqual(await other.request('GET', '/api/hosts'), { status: 200, body: [savedHost] });
    assert.deepEqual(await other.request('GET', '/api/keys'), { status: 200, body: [{ id: 'key-fixture', type: 'rsa', encrypted: false, description: 'fixture' }] });
    assert.deepEqual(await other.request('DELETE', hostRoute), { status: 204, body: null });
    assert.deepEqual(await other.request('DELETE', '/api/keys/key-fixture'), { status: 204, body: null });
    assert.deepEqual(await other.request('GET', '/api/hosts'), { status: 200, body: [] });
    assert.deepEqual(await other.request('GET', '/api/keys'), { status: 200, body: [] });
    assert.deepEqual(await other.request('DELETE', hostRoute), { status: 404, body: { error: 'Host not found' } });

    // Old hand-written configurations may omit defaults and contain metadata.
    // Preserve that exact representation when reading and changing records.
    const legacy = { id: 'legacy', hostname: 'old.invalid', metadata: { owner: 'operator' } };
    await writeFile(path.join(server.home, 'config', 'hosts.yaml'), yaml.dump({ hosts: [legacy], extension: 'preserved' }));
    assert.deepEqual(await other.request('GET', '/api/hosts'), { status: 200, body: [legacy] });
    assert.deepEqual(await other.request('PUT', '/api/hosts/legacy/', { tags: ['old'] }), { status: 200, body: { ...legacy, tags: ['old'], vnc_enabled: false, vnc_port: 5900 } });
    const updated = record(yaml.load(await readFile(path.join(server.home, 'config', 'hosts.yaml'), 'utf8')));
    assert.equal(updated.extension, 'preserved');
    await writeFile(path.join(server.home, 'config', 'keys.yaml'), yaml.dump({ keys: [{ id: 'old-key', private_key_path: '/fixture/hidden', private_note: 'never expose' }] }));
    assert.deepEqual(await other.request('GET', '/api/keys'), { status: 200, body: [{ id: 'old-key' }] });
  } finally { await other.close(); }
}

async function settingsContract(backend: Backend): Promise<void> {
  const environment = {
    WEBMUX_TERMINAL_GRID_MAX_COLS: '3',
    WEBMUX_TERMINAL_GRID_MAX_ROWS: 'unlimited',
    WEBMUX_EXEC_COMMAND: 'fixture-exec {host}',
  };
  const server = await start(backend, 'none', undefined, environment);
  const appPath = path.join(server.home, 'config', 'app.yaml');
  let savedResponse: unknown;
  const layout = { layout: { font_size: 18, tiles: [{ session_id: 'fixture', row: 0, col: 1 }] }, metadata: 'preserved' };
  try {
    const initial = await server.request('GET', '/api/config/');
    assert.equal(initial.status, 200);
    const initialApp = record(record(initial.body).app);
    assert.deepEqual(initialApp.terminal_grid, { max_cols: 3, max_rows: null });
    assert.equal(initialApp.exec_command, 'fixture-exec {host}');
    assert.deepEqual(initialApp.agents, { enabled: false, combined_pane: true, disable_in_multi_user_mode: true, definitions: [] });
    assert.deepEqual(await server.request('PUT', '/api/config', {}), { status: 400, body: { error: 'Request body must contain an app object' } });
    const before = await readFile(appPath, 'utf8');
    const invalidUpdates: [JSONRecord, string][] = [
      [{ listen_host: '127.0.0.2' }, "Field 'listen_host' cannot be changed at runtime"],
      [{ secure_mode: true }, "Field 'secure_mode' cannot be changed at runtime"],
      [{ agents: { enabled: true } }, "Field 'agents' cannot be changed at runtime"],
      [{ default_term: { font_family: 'bad; color: red' } }, 'Invalid app.default_term.font_family'],
      [{ terminal_grid: { max_cols: -1 } }, 'app.terminal_grid.max_cols must be a positive integer, null, 0, or unlimited'],
      [{ font_faces: [{ family: 'Outside', source: '../outside.ttf' }] }, 'Invalid app.font_faces'],
      [{ transport: { mosh_server_path: 'relative' } }, 'Invalid mosh_server_path: must be an absolute path'],
    ];
    for (const [app, message] of invalidUpdates) {
      assert.deepEqual(await server.request('PUT', '/api/config', { app }), { status: 400, body: { error: message } });
      assert.equal(await readFile(appPath, 'utf8'), before, 'invalid update changed configuration');
    }
    await mkdir(path.join(server.home, 'config', 'fonts'));
    await writeFile(path.join(server.home, 'config', 'fonts', 'fixture.woff2'), 'fixture-font-bytes');
    const update = await server.request('PUT', '/api/config', { app: {
      name: 'contract-settings', default_term: { font_family: 'Fixture Font,monospace', font_size: 17 },
      terminal_grid: { max_cols: 4, max_rows: 'unlimited' }, session_logging: { enabled: true },
      font_faces: [{ family: 'Fixture Font', source: 'fonts/fixture.woff2', weight: 400, style: 'ITALIC', display: 'SWAP' }],
      transport: { mosh_server_path: '/usr/local/bin/mosh-server' },
      ui: { default_pane: ' desktops ', host_switcher: { enabled: true, suffixes: ['.example', 2, ' '], hosts: [{ id: 'local', hostname: 'localhost', label: 'Local' }, { id: 7 }] } },
    } });
    assert.equal(update.status, 200);
    const updatedApp = record(record(update.body).app);
    assert.deepEqual(updatedApp.default_term, { ...record(initialApp.default_term), font_family: '"Fixture Font", monospace', font_size: 17 });
    assert.deepEqual(updatedApp.terminal_grid, { max_cols: 3, max_rows: null });
    assert.deepEqual(updatedApp.session_logging, { enabled: true });
    assert.deepEqual(updatedApp.font_faces, [{ family: 'Fixture Font', source: 'fonts/fixture.woff2', weight: '400', style: 'italic', display: 'swap', url: '/api/config/fonts/0' }]);
    assert.deepEqual(updatedApp.ui, { default_pane: 'desktops', host_switcher: { enabled: true, suffixes: ['.example'], hosts: [{ id: 'local', hostname: 'localhost', label: 'Local' }] } });
    assert.deepEqual(updatedApp.transport, { ...record(initialApp.transport), mosh_server_path: '/usr/local/bin/mosh-server' });
    const persisted = record(record(yaml.load(await readFile(appPath, 'utf8'))).app);
    assert.deepEqual(persisted.terminal_grid, { max_cols: 4, max_rows: 'unlimited' });
    assert.equal(persisted.exec_command, undefined);
    assert.ok(Array.isArray(persisted.font_faces));
    assert.equal(record(persisted.font_faces[0]).url, undefined);
    const font = await server.raw('/api/config/fonts/0');
    assert.equal(font.status, 200);
    assert.equal(font.headers.get('content-type'), 'font/woff2');
    assert.equal(font.headers.get('cache-control'), 'private, max-age=3600');
    assert.equal(await font.text(), 'fixture-font-bytes');
    const etag = font.headers.get('etag');
    assert.ok(etag);
    // Node fetch otherwise injects Cache-Control: no-cache for conditional
    // requests, which deliberately forces Express to return a fresh 200 body.
    const cached = await server.raw('/api/config/fonts/0', { 'If-None-Match': etag, 'Cache-Control': 'max-age=0' });
    assert.equal(cached.status, 304);
    await cached.body?.cancel();
    const uncached = await server.raw('/api/config/fonts/0', { 'If-None-Match': etag, 'Cache-Control': 'no-cache' });
    assert.equal(uncached.status, 200);
    assert.equal(await uncached.text(), 'fixture-font-bytes');
    const range = await server.raw('/api/config/fonts/0', { Range: 'bytes=0-6' });
    assert.equal(range.status, 206);
    assert.equal(await range.text(), 'fixture');
    assert.deepEqual(await server.request('GET', '/api/config/fonts/-1'), { status: 404, body: { error: 'Font not found' } });
    assert.deepEqual(await server.request('PUT', '/api/config/layout', layout), { status: 200, body: layout });
    assert.deepEqual(await server.request('GET', '/api/config/layout'), { status: 200, body: layout });
    // This fixture creates no real sessions. The Node broker reconciles tiles
    // with sessions at shutdown/startup, so leave an empty tile list for the
    // settings-only restart check. Real session/layout recovery is a separate
    // transport contract; the nonempty layout API round-trip is checked above.
    layout.layout.tiles = [];
    assert.deepEqual(await server.request('PUT', '/api/config/layout', layout), { status: 200, body: layout });
    savedResponse = (await server.request('GET', '/api/config')).body;
  } finally { await server.close(); }
  const other = await start(backend === 'go' ? 'node' : 'go', 'none', server.home, environment);
  try {
    assert.deepEqual(await other.request('GET', '/api/config'), { status: 200, body: savedResponse });
    assert.deepEqual(await other.request('GET', '/api/config/layout'), { status: 200, body: layout });
    const font = await other.raw('/api/config/fonts/0');
    assert.equal(font.status, 200);
    assert.equal(await font.text(), 'fixture-font-bytes');
  } finally { await other.close(); }
}

async function sessionContract(backend: Backend): Promise<void> {
  const server = await start(backend, 'local', undefined, { WEBMUX_TERMINAL_GRID_MAX_COLS: '2', WEBMUX_TERMINAL_GRID_MAX_ROWS: '2' });
  let id = '';
  let owner = '';
  try {
    owner = stringField((await server.request('POST', '/api/auth/bootstrap', { username: 'owner', password: 'password' })).body, 'token');
    assert.equal((await server.request('POST', '/api/auth/register', { username: 'member', password: 'password' }, owner)).status, 201);
    const member = stringField((await server.request('POST', '/api/auth/login', { username: 'member', password: 'password' })).body, 'token');
    const templateCatalog: unknown = JSON.parse(await readFile(path.join(root, 'server', 'internal', 'templates', 'builtin.json'), 'utf8'));
    assert.ok(Array.isArray(templateCatalog));
    assert.equal((await server.request('GET', '/api/sessions/templates')).status, 401);
    assert.equal((await server.request('GET', '/api/sessions/templates/htop')).status, 401);
    assert.deepEqual(await server.request('GET', '/api/sessions/templates/', undefined, member), {
      status: 200, body: { templates: templateCatalog, count: templateCatalog.length },
    });
    for (const template of templateCatalog) {
      assert.deepEqual(await server.request('GET', `/api/sessions/templates/${stringField(template, 'id')}`, undefined, owner), { status: 200, body: template });
    }
    assert.deepEqual(await server.request('GET', '/api/sessions/templates/unknown', undefined, owner), { status: 404, body: { error: "Template 'unknown' not found" } });
    assert.equal((await server.request('GET', '/api/sessions')).status, 401);
    assert.equal((await server.request('POST', '/api/sessions', { hostname: 'localhost' }, owner)).status, 400);
    // Missing exec commands fail locally without opening a remote connection.
    const created = await server.request('POST', '/api/sessions/', { hostname: 'localhost', username: 'alice', transport: 'exec', password: 'transient-only' }, owner);
    assert.equal(created.status, 201);
    const session = record(created.body);
    id = stringField(session, 'id');
    assert.deepEqual(Object.fromEntries(['kind', 'owner', 'hostname', 'username', 'transport', 'port', 'cols', 'rows', 'row', 'col', 'state', 'title', 'persistent', 'minimized', 'host_id', 'key_id'].map(key => [key, session[key]])), {
      kind: 'terminal', owner: 'owner', hostname: 'localhost', username: 'alice', transport: 'exec', port: 22, cols: 80, rows: 24, row: 0, col: 0, state: 'error', title: 'localhost:22', persistent: true, minimized: false, host_id: '', key_id: '',
    });
    assert.equal(session.password, undefined);
    const route = `/api/sessions/${id}`;
    for (const method of ['GET', 'PATCH', 'DELETE']) assert.equal((await server.request(method, route, method === 'PATCH' ? {} : undefined, member)).status, 404);
    assert.equal((await server.request('POST', `${route}/reconnect`, {}, member)).status, 404);
    assert.deepEqual((await server.request('GET', '/api/sessions', undefined, member)).body, []);
    let changed = await server.request('PATCH', route, { minimized: true, title: 'ignored', row: 1, col: 1 }, owner);
    assert.equal(changed.status, 200);
    assert.equal(record(changed.body).title, 'localhost:22');
    assert.equal(record(changed.body).row, 0);
    assert.equal(record(changed.body).minimized, true);
    for (const body of [{ title: ' ' }, { row: -1, col: 0 }, { row: 2, col: 0 }, { row: 0, col: 2 }, { row: 1.5, col: 0 }]) assert.equal((await server.request('PATCH', route, body, owner)).status, 400);
    changed = await server.request('PATCH', route, { title: ' renamed ' }, owner);
    assert.equal(record(changed.body).title, 'renamed');
    assert.equal((await server.request('PATCH', route, { row: 1, col: 1 }, owner)).status, 200);
    assert.equal((await server.request('POST', `${route}/reconnect`, {}, owner)).status, 500);
    const layout = record(record((await server.request('GET', '/api/config/layout', undefined, owner)).body).layout);
    assert.deepEqual(layout.tiles, [{ session_id: id, row: 1, col: 1 }]);
  } finally { await server.close(); }
  const saved = await readFile(path.join(server.home, 'data', 'sessions', 'sessions.yaml'), 'utf8');
  assert.ok(!saved.includes('transient-only') && !saved.includes('password'));
  const other = await start(backend === 'go' ? 'node' : 'go', 'local', server.home);
  try {
    const restored = await other.request('GET', `/api/sessions/${id}`, undefined, owner);
    assert.equal(restored.status, 200);
    const value = record(restored.body);
    assert.equal(value.title, 'renamed');
    assert.equal(value.minimized, true);
    assert.equal(value.row, 1);
    assert.equal(value.col, 1);
    assert.equal(value.state, 'error');
    assert.equal((await other.request('DELETE', `/api/sessions/${id}`, undefined, owner)).status, 204);
    assert.deepEqual((await other.request('GET', '/api/sessions', undefined, owner)).body, []);
    assert.deepEqual(record(record((await other.request('GET', '/api/config/layout', undefined, owner)).body).layout).tiles, []);
  } finally { await other.close(); }
}

interface SocketProbe {
  socket: WebSocket;
  events: JSONRecord[];
  output: string;
  closed?: number;
  error?: Error;
}

function probe(socket: WebSocket): SocketProbe {
  const result: SocketProbe = { socket, events: [], output: '' };
  socket.on('message', raw => {
    try {
      const data = Array.isArray(raw) ? Buffer.concat(raw) : Buffer.isBuffer(raw) ? raw : Buffer.from(raw);
      const event = record(JSON.parse(data.toString()) as unknown);
      result.events.push(event);
      if (event.type === 'output' && typeof event.data === 'string') result.output += event.data;
    } catch (error) { result.error = error instanceof Error ? error : new Error(String(error)); }
  });
  socket.on('close', code => { result.closed = code; });
  socket.on('error', error => { result.error = error; });
  return result;
}

async function waitSocket(client: SocketProbe, predicate: () => boolean, label: string): Promise<void> {
  const deadline = Date.now() + 10_000;
  while (!predicate()) {
    if (client.error) throw client.error;
    assert.ok(Date.now() < deadline, `${label}: ${JSON.stringify(client.events)} (close ${client.closed})`);
    await delay(10);
  }
}

async function slaveContract(backend: Backend): Promise<void> {
  const initial = await start(backend, 'local');
  let system = '', other = '';
  const oldIDs: string[] = [];
  const desktops: { kind: string; id: string }[] = [];
  try {
    system = stringField((await initial.request('POST', '/api/auth/bootstrap', { username: 'system', password: 'password' })).body, 'token');
    assert.equal((await initial.request('POST', '/api/auth/register', { username: 'other', password: 'password' }, system)).status, 201);
    other = stringField((await initial.request('POST', '/api/auth/login', { username: 'other', password: 'password' })).body, 'token');
    for (const owner of [system, other]) {
      const created = await initial.request('POST', '/api/sessions', { hostname: 'localhost', username: 'old', transport: 'exec' }, owner);
      assert.equal(created.status, 201); oldIDs.push(stringField(created.body, 'id'));
    }
    for (const kind of ['vnc', 'rdp']) {
      const created = await initial.request('POST', `/api/${kind}/sessions`, { hostname: '192.0.2.1' }, system);
      assert.equal(created.status, 201); desktops.push({ kind, id: stringField(created.body, 'id') });
    }
  } finally { await initial.close(); }
  const quote = (value: string): string => process.platform === 'win32' ? `"${value}"` : `'${value.replaceAll("'", `'"'"'`)}'`;
  const command = `${quote(process.execPath)} ${quote(path.join(root, 'scripts', 'terminal-fixture.mts'))}`;
  for (const [index, runtime] of [backend, backend === 'node' ? 'go' : 'node'].entries()) {
    const server = await start(runtime as Backend, 'local', initial.home, { WEBMUX_SLAVE_HOST: 'console.local', WEBMUX_SLAVE_PORT: index === 0 ? '1234' : '', WEBMUX_EXEC_COMMAND: command });
    let client: SocketProbe | undefined;
    try {
      const listed = await server.request('GET', '/api/sessions', undefined, system);
      assert.equal(listed.status, 200); assert.ok(Array.isArray(listed.body)); assert.equal(listed.body.length, 1);
      const value = record(listed.body[0]); const id = stringField(value, 'id');
      assert.ok(!oldIDs.includes(id)); oldIDs.push(id);
      assert.deepEqual(Object.fromEntries(['owner', 'username', 'hostname', 'transport', 'port', 'row', 'col', 'title'].map(key => [key, value[key]])), {
        owner: 'system', username: 'console', hostname: 'console.local', transport: 'exec', port: index === 0 ? 1234 : 22, row: 0, col: 0, title: `console.local:${index === 0 ? 1234 : 22}`,
      });
      assert.deepEqual((await server.request('GET', '/api/sessions', undefined, other)).body, []);
      assert.equal((await server.request('GET', `/api/sessions/${id}`, undefined, other)).status, 404);
      for (const desktop of desktops) assert.equal(stringField((await server.request('GET', `/api/${desktop.kind}/sessions/${desktop.id}`, undefined, system)).body, 'id'), desktop.id);
      client = probe(server.socket(`/api/term/${id}?token=${system}`));
      const active = client;
      await waitSocket(active, () => active.output.includes('fixture-ready'), 'slave console launch');
      active.socket.send(JSON.stringify({ type: 'input', data: 'slave-input\r' }));
      await waitSocket(active, () => active.output.includes('fixture-reply:slave-input:λ😀'), 'slave console input');
    } finally { client?.socket.terminate(); await server.close(); }
    const saved = record(yaml.load(await readFile(path.join(initial.home, 'data', 'sessions', 'sessions.yaml'), 'utf8')));
    assert.ok(Array.isArray(saved.sessions)); assert.equal(saved.sessions.length, 1);
    assert.equal(record(saved.sessions[0]).owner, 'system');
  }
}

async function terminalContract(backend: Backend): Promise<void> {
  const server = await start(backend, 'local');
  const clients: SocketProbe[] = [];
  const open = (route: string): SocketProbe => { const client = probe(server.socket(route)); clients.push(client); return client; };
  try {
    const owner = stringField((await server.request('POST', '/api/auth/bootstrap', { username: 'owner', password: 'password' })).body, 'token');
    assert.equal((await server.request('PUT', '/api/config', { app: { session_logging: { enabled: true } } }, owner)).status, 200);
    const quote = (value: string): string => process.platform === 'win32' ? `"${value}"` : `'${value.replaceAll("'", `'"'"'`)}'`;
    const command = `${quote(process.execPath)} ${quote(path.join(root, 'scripts', 'terminal-fixture.mts'))}`;
    const created = await server.request('POST', '/api/sessions', { hostname: 'localhost', username: 'fixture', transport: 'exec', exec_command: command }, owner);
    assert.equal(created.status, 201);
    const id = stringField(created.body, 'id');
    const pathName = `/api/term/${id}`;
    const unauthorized = open(pathName);
    await waitSocket(unauthorized, () => unauthorized.closed !== undefined, 'unauthenticated socket close');
    assert.equal(unauthorized.closed, 1008);
    assert.ok(unauthorized.events.some(event => event.type === 'error' && event.message === 'Unauthorized'));
    const ticket = stringField((await server.request('POST', '/api/auth/ticket', {}, owner)).body, 'ticket');
    const first = open(`${pathName}?ticket=${ticket}`);
    await waitSocket(first, () => first.output.includes('fixture-ready'), 'initial terminal output');
    const firstStatus = first.events.find(event => event.type === 'status' && typeof event.viewer_id === 'string');
    assert.ok(firstStatus);
    assert.equal(firstStatus.transcript_enabled, true);
    const firstID = stringField(firstStatus, 'viewer_id');
    const reuse = open(`${pathName}?ticket=${ticket}`);
    await waitSocket(reuse, () => reuse.closed !== undefined, 'ticket reuse close');
    assert.equal(reuse.closed, 1008);
    first.socket.send(JSON.stringify({ type: 'input', data: 'hello\r' }));
    await waitSocket(first, () => first.output.includes('fixture-reply:hello:λ😀'), 'interactive output');
    first.socket.send(JSON.stringify({ type: 'resize', cols: 99, rows: 31 }));
    // ConPTY delivery and Node's SIGWINCH-driven stdout size cache update
    // asynchronously. Query until the child observes the requested dimensions;
    // the protocol has no resize acknowledgement to await before sending input.
    const resizeDeadline = Date.now() + 10_000;
    while (!first.output.includes('fixture-size:99x31')) {
      if (first.error) throw first.error;
      assert.ok(Date.now() < resizeDeadline, `terminal resize: ${JSON.stringify(first.events)}`);
      first.socket.send(JSON.stringify({ type: 'input', data: 'size\r' }));
      await delay(50);
    }
    first.socket.send(JSON.stringify({ type: 'transcript_toggle' }));
    await waitSocket(first, () => first.events.some(event => event.type === 'transcript_status' && event.transcript_enabled === false), 'transcript pause');
    const paused = first.events.find(event => event.type === 'transcript_status' && event.transcript_enabled === false);
    assert.ok(paused);
    const transcript = stringField(paused, 'transcript_file');
    const beforePause = await readFile(transcript, 'utf8');
    assert.ok(beforePause.includes('[webmux transcript started ') && beforePause.includes('fixture-reply:hello:λ😀'));
    assert.ok(beforePause.includes('reason=manual_pause'));
    first.socket.send(JSON.stringify({ type: 'input', data: 'paused-output\r' }));
    await waitSocket(first, () => first.output.includes('fixture-reply:paused-output:λ😀'), 'output while transcript paused');
    assert.ok(!(await readFile(transcript, 'utf8')).includes('paused-output'));
    const resumeIndex = first.events.length;
    first.socket.send(JSON.stringify({ type: 'transcript_toggle' }));
    await waitSocket(first, () => first.events.slice(resumeIndex).some(event => event.type === 'transcript_status' && event.transcript_enabled === true), 'transcript resume');
    const resumed = first.events.slice(resumeIndex).find(event => event.type === 'transcript_status' && event.transcript_enabled === true);
    assert.equal(resumed?.transcript_file, transcript);
    first.socket.send(JSON.stringify({ type: 'input', data: 'recorded-output\r' }));
    await waitSocket(first, () => first.output.includes('fixture-reply:recorded-output:λ😀'), 'resumed transcript output');
    await waitFile(transcript, 'fixture-reply:recorded-output:λ😀');
    if (process.platform !== 'win32') {
      assert.equal((await stat(transcript)).mode & 0o777, 0o600);
      assert.equal((await stat(path.dirname(transcript))).mode & 0o777, 0o700);
    }
    const second = open(`${pathName}?token=${owner}`);
    await waitSocket(second, () => second.output.includes('fixture-reply:hello:λ😀'), 'scrollback replay');
    const secondJoin = second.events.find(event => event.type === 'viewer_join');
    assert.ok(secondJoin);
    const secondID = stringField(secondJoin, 'viewer_id');
    assert.equal(secondJoin.focus_owner, firstID);
    assert.equal(secondJoin.viewer_count, 2);
    second.socket.send('malformed JSON');
    second.socket.send(JSON.stringify({ type: 'resize', cols: -1, rows: 24 }));
    second.socket.send(JSON.stringify({ type: 'focus' }));
    await waitSocket(first, () => first.events.some(event => event.type === 'focus' && event.focus_owner === secondID), 'viewer focus');
    second.socket.close();
    await waitSocket(first, () => first.events.some(event => event.type === 'viewer_leave' && event.viewer_id === secondID), 'viewer leave');
    const leave = first.events.find(event => event.type === 'viewer_leave' && event.viewer_id === secondID);
    assert.equal(leave?.focus_owner, undefined);
    assert.equal(leave?.viewer_count, 1);
    const reconnectIndex = first.events.length;
    const outputIndex = first.output.length;
    assert.equal((await server.request('POST', `/api/sessions/${id}/reconnect`, {}, owner)).status, 200);
    await waitSocket(first, () => first.events.slice(reconnectIndex).some(event => event.type === 'transcript_status' && event.transcript_enabled === true), 'new launch transcript');
    const restarted = first.events.slice(reconnectIndex).find(event => event.type === 'transcript_status' && event.transcript_enabled === true);
    assert.ok(restarted);
    const nextTranscript = stringField(restarted, 'transcript_file');
    assert.notEqual(nextTranscript, transcript);
    await waitSocket(first, () => first.output.slice(outputIndex).includes('fixture-ready'), 'reconnected terminal output');
    await waitFile(transcript, 'reason=reconnect');
    assert.ok((await readFile(transcript, 'utf8')).includes('[webmux transcript resumed '));
    assert.ok(!(await readFile(transcript, 'utf8')).includes('paused-output'));
    assert.equal((await server.request('DELETE', `/api/sessions/${id}`, undefined, owner)).status, 204);
    await waitSocket(first, () => first.closed !== undefined, 'session deletion close');
    assert.equal(first.closed, 1000);
    await waitFile(nextTranscript, 'reason=deleted');
  } finally {
    for (const client of clients) client.socket.terminate();
    await server.close();
  }
}

async function vncContract(backend: Backend): Promise<void> {
  const upstreams = new Set<Socket>();
  const echo = createServer(socket => {
    upstreams.add(socket);
    socket.on('close', () => upstreams.delete(socket));
    socket.on('error', () => {});
    socket.write(Buffer.from('RFB 003.008\n'));
    socket.on('data', data => socket.write(data));
  });
  echo.listen(0, '127.0.0.1'); await once(echo, 'listening');
  const address = echo.address(); assert.ok(address && typeof address !== 'string');
  const server = await start(backend, 'local', undefined, { WEBMUX_ALLOW_LOCAL_TARGETS: '1' }).catch((error: unknown) => { echo.close(); throw error; });
  const clients: WebSocket[] = [];
  const open = (route: string): WebSocket => { const socket = server.socket(route); clients.push(socket); socket.on('error', () => {}); return socket; };
  try {
    const owner = stringField((await server.request('POST', '/api/auth/bootstrap', { username: 'owner', password: 'password' })).body, 'token');
    const created = await server.request('POST', '/api/vnc/sessions', { hostname: '127.0.0.1', vnc_port: address.port }, owner);
    assert.equal(created.status, 201); const id = stringField(created.body, 'id'); const route = `/api/vnc/ws/${id}`;
    const denied = open(route); assert.equal((await once(denied, 'close'))[0], 1008);
    const ticket = stringField((await server.request('POST', '/api/auth/ticket', {}, owner)).body, 'ticket');
    const client = open(`${route}?ticket=${ticket}`);
    let received = Buffer.alloc(0);
    client.on('message', (data: WebSocket.RawData, binary: boolean) => {
      assert.equal(binary, true);
      received = Buffer.concat([received, Array.isArray(data) ? Buffer.concat(data) : Buffer.isBuffer(data) ? data : Buffer.from(data)]);
    });
    const waitBytes = async (length: number): Promise<void> => { const deadline = Date.now() + 10_000; while (received.length < length) { assert.ok(Date.now() < deadline, 'VNC fixture data timeout'); await delay(10); } };
    await waitBytes(12); assert.equal(received.toString(), 'RFB 003.008\n');
    const reused = open(`${route}?ticket=${ticket}`); assert.equal((await once(reused, 'close'))[0], 1008);
    const payload = Buffer.from([0, 255, 128, 1, 13, 10]); client.send(payload);
    await waitBytes(12 + payload.length); assert.deepEqual(received.subarray(12), payload);
    assert.equal(record((await server.request('GET', `/api/vnc/sessions/${id}`, undefined, owner)).body).state, 'connected');
    const close = once(client, 'close'); for (const upstream of upstreams) upstream.end();
    assert.equal((await close)[0], 1001);
  } finally {
    for (const client of clients) client.terminate();
    await server.close();
    for (const upstream of upstreams) upstream.destroy();
    await new Promise<void>((resolve, reject) => echo.close(error => error ? reject(error) : resolve()));
  }
}

async function rdpContract(backend: Backend): Promise<void> {
  // ASCII fixture instructions isolate shared legacy behavior. Fragmented
  // Unicode and embedded separators have dedicated Go protocol tests.
  const instruction = (...values: string[]): string => values.map(value => `${value.length}.${value}`).join(',') + ';';
  const upstreams = new Set<Socket>();
  const requests: string[] = [];
  const ready = instruction('ready', 'fixture-connection');
  const output = instruction('sync', '123');
  const input = instruction('key', '65293', '1');
  const daemon = createServer(socket => {
    upstreams.add(socket);
    socket.on('close', () => upstreams.delete(socket));
    socket.on('error', () => {});
    let buffered = '';
    socket.on('data', data => {
      buffered += data.toString('utf8');
      let boundary: number;
      while ((boundary = buffered.indexOf(';')) >= 0) {
        const message = buffered.slice(0, boundary + 1);
        buffered = buffered.slice(boundary + 1);
        requests.push(message);
        if (requests.length === 1) socket.write(instruction('args', 'hostname', 'port', 'username', 'password', 'domain', 'resize-method', 'unknown'));
        else if (requests.length === 2) socket.write(ready + output);
        else socket.write(message);
      }
    });
  });
  daemon.listen(0, '127.0.0.1'); await once(daemon, 'listening');
  const address = daemon.address(); assert.ok(address && typeof address !== 'string');
  const server = await start(backend, 'local').catch((error: unknown) => { daemon.close(); throw error; });
  const clients: WebSocket[] = [];
  const open = (route: string): WebSocket => { const socket = server.socket(route); clients.push(socket); socket.on('error', () => {}); return socket; };
  try {
    const appPath = path.join(server.home, 'config', 'app.yaml');
    const config = record(yaml.load(await readFile(appPath, 'utf8')));
    record(config.app).guacd = { host: '127.0.0.1', port: address.port };
    await writeFile(appPath, yaml.dump(config));
    const owner = stringField((await server.request('POST', '/api/auth/bootstrap', { username: 'owner', password: 'password' })).body, 'token');
    const created = await server.request('POST', '/api/rdp/sessions', { hostname: '192.0.2.1', rdp_port: 3390, rdp_username: 'desktop-user', rdp_password: 'password', rdp_domain: 'domain' }, owner);
    assert.equal(created.status, 201);
    const id = stringField(created.body, 'id'); const route = `/api/rdp/ws/${id}`;
    const denied = open(route); assert.equal((await once(denied, 'close'))[0], 1008);
    const ticket = stringField((await server.request('POST', '/api/auth/ticket', {}, owner)).body, 'ticket');
    const client = open(`${route}?ticket=${ticket}`);
    let received = '';
    client.on('message', (data: WebSocket.RawData, binary: boolean) => {
      assert.equal(binary, false);
      received += (Array.isArray(data) ? Buffer.concat(data) : Buffer.isBuffer(data) ? data : Buffer.from(data)).toString('utf8');
    });
    const waitText = async (text: string): Promise<void> => {
      const deadline = Date.now() + 10_000;
      while (received.length < text.length) { assert.ok(Date.now() < deadline, 'RDP fixture data timeout'); await delay(10); }
      assert.equal(received, text);
    };
    await waitText(ready + output);
    assert.deepEqual(requests, [instruction('select', 'rdp'), instruction('connect', '192.0.2.1', '3390', 'desktop-user', 'password', 'domain', 'display-update', '')]);
    const reused = open(`${route}?ticket=${ticket}`); assert.equal((await once(reused, 'close'))[0], 1008);
    client.send(input);
    await waitText(ready + output + input);
    assert.equal(requests[2], input);
    assert.equal(record((await server.request('GET', `/api/rdp/sessions/${id}`, undefined, owner)).body).state, 'connected');
    const close = once(client, 'close'); for (const upstream of upstreams) upstream.end();
    assert.equal((await close)[0], 1001);
  } finally {
    for (const client of clients) client.terminate();
    await server.close();
    for (const upstream of upstreams) upstream.destroy();
    await new Promise<void>((resolve, reject) => daemon.close(error => error ? reject(error) : resolve()));
  }
}

async function desktopContract(backend: Backend): Promise<void> {
  const server = await start(backend, 'local');
  let owner = '';
  const saved: { kind: 'vnc' | 'rdp'; value: JSONRecord }[] = [];
  try {
    owner = stringField((await server.request('POST', '/api/auth/bootstrap', { username: 'owner', password: 'password' })).body, 'token');
    assert.equal((await server.request('POST', '/api/auth/register', { username: 'member', password: 'password' }, owner)).status, 201);
    const member = stringField((await server.request('POST', '/api/auth/login', { username: 'member', password: 'password' })).body, 'token');
    assert.equal((await server.request('POST', '/api/hosts', { id: 'desktop-host', hostname: 'desktop.invalid', vnc_port: 5907, rdp_port: 3397 }, owner)).status, 201);
    for (const kind of ['vnc', 'rdp'] as const) {
      const base = `/api/${kind}/sessions`;
      const call = (method: string, route: string, body?: JSONRecord) => server.request(method, route, body, owner);
      assert.equal((await server.request('GET', base)).status, 401);
      assert.deepEqual(await call('POST', base, {}), { status: 400, body: { error: 'hostname or host_id is required' } });
      const first = await call('POST', base, { hostname: 'first.invalid', [`${kind}_password`]: 'transient-only' });
      assert.equal(first.status, 201);
      const firstID = stringField(first.body, 'id');
      assert.equal(record(first.body)[`${kind}_port`], kind === 'vnc' ? 5900 : 3389);
      assert.equal(record(first.body).state, 'connecting'); assert.equal(record(first.body).persistent, true);
      assert.equal(record(first.body).row, 0); assert.equal(record(first.body).col, 0);
      assert.equal(record(first.body)[`${kind}_password`], undefined);
      const foreign = await server.request('POST', base, { hostname: 'foreign.invalid' }, member);
      assert.equal(foreign.status, 201); assert.equal(record(foreign.body).col, 0);
      for (const method of ['GET', 'PATCH', 'DELETE']) assert.equal((await server.request(method, `${base}/${firstID}`, method === 'PATCH' ? {} : undefined, member)).status, 404);
      const second = await call('POST', base, { host_id: 'desktop-host', hostname: 'ignored.invalid', [`${kind}_port`]: 6000, row: 2, col: 3, rdp_username: 'desktop-user', rdp_domain: 'fixture-domain' });
      assert.equal(second.status, 201);
      const secondID = stringField(second.body, 'id');
      assert.equal(record(second.body).hostname, 'desktop.invalid');
      assert.equal(record(second.body)[`${kind}_port`], kind === 'vnc' ? 5907 : 3397);
      if (kind === 'rdp') { assert.equal(record(second.body).rdp_username, 'desktop-user'); assert.equal(record(second.body).rdp_domain, 'fixture-domain'); }
      else assert.equal(record(second.body).rdp_username, undefined);
      const third = await call('POST', base, { hostname: 'third.invalid' });
      assert.equal(record(third.body).row, 2); assert.equal(record(third.body).col, 4);
      assert.equal((await call('DELETE', `${base}/${firstID}`)).status, 204);
      const compacted = record((await call('GET', `${base}/${secondID}`)).body);
      assert.equal(compacted.row, 0); assert.equal(compacted.col, 0);
      assert.deepEqual(await call('PATCH', `${base}/${secondID}`, { row: 1 }), { status: 400, body: { error: 'row and col are required' } });
      assert.deepEqual(await call('PATCH', `${base}/${secondID}`, { row: null, col: 0 }), { status: 400, body: { error: 'row and col must be non-negative numbers' } });
      assert.deepEqual(await call('PATCH', `${base}/${secondID}`, { row: -1, col: 0 }), { status: 400, body: { error: 'row and col must be non-negative numbers' } });
      const moved = await call('PATCH', `${base}/${secondID}`, { row: 3.5, col: 5.5 });
      assert.equal(moved.status, 200); assert.equal(record(moved.body).row, 3.5); assert.equal(record(moved.body).col, 5.5);
      const reconnect = await call('POST', `${base}/${secondID}/reconnect`, {});
      assert.equal(reconnect.status, 200); assert.equal(record(reconnect.body).state, 'connecting');
      saved.push({ kind, value: record(reconnect.body) });
      const data = await readFile(path.join(server.home, 'data', 'sessions', `${kind}-sessions.yaml`), 'utf8');
      assert.ok(!data.includes('password') && !data.includes('transient-only'));
    }
    assert.deepEqual(await server.request('GET', '/api/sessions', undefined, owner), { status: 200, body: [] });
  } finally { await server.close(); }
  const other = await start(backend === 'go' ? 'node' : 'go', 'local', server.home);
  try {
    for (const { kind, value } of saved) {
      const restored = await other.request('GET', `/api/${kind}/sessions/${stringField(value, 'id')}`, undefined, owner);
      assert.equal(restored.status, 200);
      assert.deepEqual({ ...record(restored.body), updated_at: undefined }, { ...value, state: 'disconnected', updated_at: undefined });
    }
  } finally { await other.close(); }
}

async function agentContract(backend: Backend): Promise<void> {
  const cwd = await mkdtemp(path.join(temporary, 'agent-cwd-'));
  const server = await start(backend, 'local', undefined, {
    PATH: `${agentFixtureDir}${path.delimiter}${process.env.PATH ?? ''}`,
    SHELL: agentFixture, COMSPEC: agentFixture, WEBMUX_AGENT_FIXTURE_CWD: cwd,
  });
  const clients: SocketProbe[] = [];
  try {
    const owner = stringField((await server.request('POST', '/api/auth/bootstrap', { username: 'owner', password: 'password' })).body, 'token');
    const call = (method: string, route: string, body?: JSONRecord) => server.request(method, route, body, owner);
    assert.equal((await server.request('GET', '/api/agents/config')).status, 401);
    assert.deepEqual(await call('GET', '/api/agents/sessions'), { status: 404, body: { error: 'Agent sessions are not enabled' } });
    const appPath = path.join(server.home, 'config', 'app.yaml');
    const app = record(yaml.load(await readFile(appPath, 'utf8')));
    record(app.app).agents = { enabled: true, combined_pane: true, disable_in_multi_user_mode: true, definitions: [{ id: 'alpha', label: 'Alpha', plural_label: 'Alphas', badge: 'A', tmux_socket: 'fixture-only', workspace: 'agents', enabled: true }] };
    await writeFile(`${appPath}.contract`, yaml.dump(app));
    await rename(`${appPath}.contract`, appPath);
    assert.deepEqual(await call('GET', '/api/agents/config'), { status: 200, body: record(app.app).agents });
    assert.deepEqual(await call('GET', '/api/agents/missing/sessions'), { status: 404, body: { error: 'Agent definition not found' } });
    const listed = await call('GET', '/api/agents/alpha/sessions');
    assert.equal(listed.status, 200);
    assert.ok(Array.isArray(listed.body));
    assert.deepEqual(listed.body.map(item => record(item).name), ['alpha-task-a', 'alpha-task-b']);
    assert.deepEqual((await call('GET', '/api/agents/sessions')).body, listed.body);
    assert.deepEqual(await call('POST', '/api/agents/alpha/attach', {}), { status: 400, body: { error: 'name is required' } });
    assert.deepEqual(await call('POST', '/api/agents/alpha/attach', { name: 'missing' }), { status: 404, body: { error: 'Alpha session not found' } });
    assert.deepEqual(await call('POST', '/api/agents/alpha/scratch', { selectedName: null }), { status: 400, body: { error: 'selectedName must be a string' } });
    const statusDir = path.join(server.home, 'data', 'agent-status', 'alpha');
    await mkdir(statusDir, { recursive: true });
    const statusFile = path.join(statusDir, `${Buffer.from('alpha-task-a').toString('base64url')}.json`);
    await writeFile(statusFile, JSON.stringify({ agent_id: 'alpha', name: 'alpha-task-a', status: 'waiting', source: 'hook', updated_at: new Date().toISOString(), extension: 'preserved' }));
    const attached = await call('POST', '/api/agents/alpha/attach', { name: 'alpha-task-a', cols: 999, rows: 1 });
    assert.equal(attached.status, 201);
    const id = stringField(attached.body, 'id');
    const value = record(attached.body);
    assert.equal(value.cols, 240); assert.equal(value.rows, 10); assert.equal(value.state, 'connected');
    assert.equal(value.agent_role, 'attach'); assert.equal(value.persistent, false);
    assert.equal(value.workspace, 'agents'); assert.deepEqual(value.exec_argv, ['tmux', '-L', 'fixture-only', 'attach-session', '-t', 'alpha-task-a']);
    const client = probe(server.socket(`/api/term/${id}?token=${encodeURIComponent(owner)}`)); clients.push(client);
    await waitSocket(client, () => client.output.includes('agent-fixture-ready'), 'agent attachment output');
    client.socket.send(JSON.stringify({ type: 'input', data: 'hello\r' }));
    await waitSocket(client, () => client.output.includes('agent-fixture-reply:hello'), 'agent interactive output');
    await waitFile(statusFile, 'last_input_at');
    const activity = record(JSON.parse(await readFile(statusFile, 'utf8')) as unknown);
    assert.equal(activity.status, 'working'); assert.equal(activity.source, 'webmux'); assert.equal(activity.extension, 'preserved');
    await delay(1600); // Pass the intentional replay suppression window.
    client.socket.send(JSON.stringify({ type: 'input', data: 'live-output\r' }));
    await waitSocket(client, () => client.output.includes('agent-fixture-reply:live-output'), 'live agent output');
    await waitFile(statusFile, 'last_output_source');
    assert.equal(record(JSON.parse(await readFile(statusFile, 'utf8')) as unknown).last_output_source, 'live');
    const reused = await call('POST', '/api/agents/alpha/attach', { name: 'alpha-task-a' });
    assert.equal(reused.status, 200); assert.equal(stringField(reused.body, 'id'), id);
    const outputIndex = client.output.length;
    const replaced = await call('POST', '/api/agents/alpha/attach', { name: 'alpha-task-b', cols: 100.9, rows: 30.9 });
    assert.equal(replaced.status, 200); assert.equal(stringField(replaced.body, 'id'), id);
    assert.equal(record(replaced.body).cols, 100); assert.equal(record(replaced.body).rows, 30);
    await waitSocket(client, () => client.output.slice(outputIndex).includes('agent-fixture-ready'), 'agent replacement output');
    const scratch = await call('POST', '/api/agents/alpha/scratch', { selectedName: 'alpha-task-a' });
    assert.equal(scratch.status, 201); assert.equal(record(scratch.body).exec_cwd, cwd);
    assert.equal(record(scratch.body).agent_role, 'scratch'); assert.equal(record(scratch.body).col, 1);
    const scratchAgain = await call('POST', '/api/agents/alpha/scratch', {});
    assert.equal(scratchAgain.status, 200); assert.equal(stringField(scratchAgain.body, 'id'), stringField(scratch.body, 'id'));
    assert.deepEqual(await call('GET', '/api/sessions'), { status: 200, body: [] });
    assert.equal((await call('POST', '/api/auth/register', { username: 'member', password: 'password' })).status, 201);
    assert.deepEqual(await call('GET', '/api/agents/sessions'), { status: 403, body: { error: 'Agent sessions are disabled in multi-user mode' } });
    await waitSocket(client, () => client.closed !== undefined, 'agent policy revocation');
    assert.equal(client.closed, 1008);
    assert.equal((await call('GET', `/api/sessions/${id}`)).status, 404);
    assert.equal((await call('GET', '/api/agents/config')).status, 200);
  } finally { for (const client of clients) client.socket.terminate(); await server.close(); }
}

async function waitFile(file: string, content: string): Promise<void> {
  const deadline = Date.now() + 10_000;
  for (;;) {
    if ((await readFile(file, 'utf8')).includes(content)) return;
    assert.ok(Date.now() < deadline, `transcript missing ${content}`);
    await delay(10);
  }
}

try {
  const build = spawnSync('go', ['build', '-o', binary, './cmd/webmux'], { cwd: path.join(root, 'server'), stdio: 'inherit' });
  if (build.error) throw build.error;
  assert.equal(build.status, 0, 'Go build failed');
  await mkdir(agentFixtureDir);
  const fixtureBuild = spawnSync('go', ['build', '-o', agentFixture, './internal/agent/testdata/tmux'], { cwd: path.join(root, 'server'), stdio: 'inherit' });
  if (fixtureBuild.error) throw fixtureBuild.error;
  assert.equal(fixtureBuild.status, 0, 'Agent fixture build failed');
  for (const backend of ['node', 'go'] as const) {
    await staticContract(backend);
    await uploadContract(backend);
    await aiContract(backend);
    await localContract(backend);
    await trustedContract(backend);
    await catalogContract(backend);
    await settingsContract(backend);
    await sessionContract(backend);
    await terminalContract(backend);
    await slaveContract(backend);
    await agentContract(backend);
    await desktopContract(backend);
    await vncContract(backend);
    await rdpContract(backend);
    console.log(`${backend}: HTTP, templates, terminal WebSocket, agent, VNC/RDP and cross-backend restart contracts passed`);
  }
} finally {
  await rm(temporary, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
}
