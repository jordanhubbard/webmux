import assert from 'node:assert/strict';
import { spawn, spawnSync, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import { cp, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import * as argon2 from 'argon2';
import jwt from 'jsonwebtoken';
import yaml from 'js-yaml';

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
  request: (method: string, route: string, body?: JSONRecord, token?: string) => Promise<{ status: number; body: unknown }>;
  close: () => Promise<void>;
}

async function start(backend: Backend, mode: Mode, existingHome?: string): Promise<RunningServer> {
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
        NODE_ENV: 'contract',
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

try {
  const build = spawnSync('go', ['build', '-o', binary, './cmd/webmux'], { cwd: path.join(root, 'server'), stdio: 'inherit' });
  if (build.error) throw build.error;
  assert.equal(build.status, 0, 'Go build failed');
  for (const backend of ['node', 'go'] as const) {
    await localContract(backend);
    await trustedContract(backend);
    await catalogContract(backend);
    console.log(`${backend}: authentication/catalog contracts and cross-backend restarts passed`);
  }
} finally {
  await rm(temporary, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
}
