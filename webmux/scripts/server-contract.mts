import assert from 'node:assert/strict';
import { spawn, spawnSync, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import { cp, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
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
  raw: (route: string, headers?: Record<string, string>) => Promise<Response>;
  request: (method: string, route: string, body?: JSONRecord, token?: string) => Promise<{ status: number; body: unknown }>;
  close: () => Promise<void>;
}

async function start(backend: Backend, mode: Mode, existingHome?: string, environment: NodeJS.ProcessEnv = {}): Promise<RunningServer> {
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
    backend === 'go' ? ['--root', root, '--home', home, '--listen', `127.0.0.1:${port}`] : [path.join(root, 'backend', 'dist', 'index.js')],
    {
      cwd: root,
      env: {
        ...process.env,
        WEBMUX_ROOT: root,
        WEBMUX_HOME: home,
        HTTP_PORT: String(port),
        HTTPS_PORT: '0',
        JWT_SECRET: '',
        WEBMUX_SLAVE_HOST: '',
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
    raw: (route, headers) => fetch(base + route, { headers, signal: AbortSignal.timeout(10_000) }),
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

async function terminalContract(backend: Backend): Promise<void> {
  const server = await start(backend, 'local');
  const clients: SocketProbe[] = [];
  const open = (route: string): SocketProbe => { const client = probe(server.socket(route)); clients.push(client); return client; };
  try {
    const owner = stringField((await server.request('POST', '/api/auth/bootstrap', { username: 'owner', password: 'password' })).body, 'token');
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
    const firstID = stringField(firstStatus, 'viewer_id');
    const reuse = open(`${pathName}?ticket=${ticket}`);
    await waitSocket(reuse, () => reuse.closed !== undefined, 'ticket reuse close');
    assert.equal(reuse.closed, 1008);
    first.socket.send(JSON.stringify({ type: 'input', data: 'hello\r' }));
    await waitSocket(first, () => first.output.includes('fixture-reply:hello:λ😀'), 'interactive output');
    first.socket.send(JSON.stringify({ type: 'resize', cols: 99, rows: 31 }));
    first.socket.send(JSON.stringify({ type: 'input', data: 'size\r' }));
    await waitSocket(first, () => first.output.includes('fixture-size:99x31'), 'terminal resize');
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
    assert.equal((await server.request('DELETE', `/api/sessions/${id}`, undefined, owner)).status, 204);
    await waitSocket(first, () => first.closed !== undefined, 'session deletion close');
    assert.equal(first.closed, 1000);
  } finally {
    for (const client of clients) client.socket.terminate();
    await server.close();
  }
}

try {
  const build = spawnSync('go', ['build', '-o', binary, './cmd/webmux'], { cwd: path.join(root, 'server'), stdio: 'inherit' });
  if (build.error) throw build.error;
  assert.equal(build.status, 0, 'Go build failed');
  for (const backend of ['node', 'go'] as const) {
    await localContract(backend);
    await trustedContract(backend);
    await catalogContract(backend);
    await settingsContract(backend);
    await sessionContract(backend);
    await terminalContract(backend);
    console.log(`${backend}: HTTP, terminal WebSocket and cross-backend restart contracts passed`);
  }
} finally {
  await rm(temporary, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
}
