// A single launch handoff over an owner-private Unix socket. Arbitrary owner
// environment values live only in memory, never in files or Rig metadata.
import fs from 'node:fs';
import net from 'node:net';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { validateCodexLaunchEnvironment, type CodexLaunchEnvironment } from './codex-launch-environment.mts';

const failure = () => new Error('codex_environment_handoff_unavailable');
const hash = (text: string) => createHash('sha256').update(text).digest('hex');
function privateParent(file: string) {
  const parent = path.dirname(file), stat = fs.lstatSync(parent);
  if (!path.isAbsolute(file) || Buffer.byteLength(file) > 100 || fs.realpathSync(parent) !== parent
    || !stat.isDirectory() || stat.uid !== process.geteuid?.() || (stat.mode & 0o777) !== 0o700) throw failure();
}
function ownedSocket(file: string) {
  const stat = fs.lstatSync(file);
  if (!stat.isSocket() || stat.uid !== process.geteuid?.() || (stat.mode & 0o077) !== 0) throw failure();
  return stat;
}
async function removeStale(file: string) {
  let before: fs.Stats;
  try { before = ownedSocket(file); } catch (error) { if ((error as NodeJS.ErrnoException).code === 'ENOENT') return; throw error; }
  const stale = await new Promise<boolean>(resolve => {
    const socket = net.createConnection(file); socket.setTimeout(1000);
    socket.once('connect', () => { socket.destroy(); resolve(false); });
    socket.once('timeout', () => { socket.destroy(); resolve(false); });
    socket.once('error', error => { socket.destroy(); resolve((error as NodeJS.ErrnoException).code === 'ECONNREFUSED'); });
  });
  const after = ownedSocket(file);
  if (!stale || after.ino !== before.ino || after.dev !== before.dev || after.ctimeMs !== before.ctimeMs) throw failure();
  fs.unlinkSync(file);
}
function lines(socket: net.Socket, maximum: number, handle: (value: unknown) => void) {
  let buffer = '', total = 0;
  socket.setEncoding('utf8');
  socket.on('data', (chunk: string) => {
    total += Buffer.byteLength(chunk); buffer += chunk;
    if (total > maximum) { socket.destroy(failure()); return; }
    for (;;) {
      const at = buffer.indexOf('\n'); if (at < 0) break;
      const line = buffer.slice(0, at); buffer = buffer.slice(at + 1);
      try { handle(JSON.parse(line)); } catch { socket.destroy(failure()); return; }
    }
  });
}
const row = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);

export async function openCodexEnvironmentChannel(file: string, snapshot: CodexLaunchEnvironment, env: NodeJS.ProcessEnv,
  delivered: () => void, lifetime = 45000) {
  privateParent(file); validateCodexLaunchEnvironment(snapshot, snapshot.values.CODEX_HOME);
  const values = Object.fromEntries(Object.entries(snapshot.inheritedHashes).map(([name, expected]) => {
    const value = env[name]; if (typeof value !== 'string' || hash(value) !== expected) throw failure(); return [name, value];
  }));
  const payload = JSON.stringify({ fingerprint: snapshot.fingerprint, values }) + '\n';
  if (Buffer.byteLength(payload) > 2 * 1024 * 1024) throw failure();
  await removeStale(file);
  const clients = new Set<net.Socket>(); let claimed = false, done = false, interrupted = false;
  let finish!: () => void;
  const waited = new Promise<void>(resolve => { finish = resolve; });
  const server = net.createServer(socket => {
    if (claimed || clients.size >= 4) { socket.destroy(); return; }
    clients.add(socket); socket.setTimeout(5000, () => socket.destroy());
    socket.on('error', () => {}); socket.on('close', () => clients.delete(socket));
    let phase = 0;
    lines(socket, 1024, value => {
      if (!row(value) || value.fingerprint !== snapshot.fingerprint) throw failure();
      if (phase === 0 && !claimed && Object.keys(value).length === 1) {
        claimed = true; phase = 1; socket.write(payload);
      } else if (phase === 1 && value.received === true && Object.keys(value).length === 2) {
        delivered(); done = true; socket.once('close', finish); socket.end('{"ready":true}\n');
      } else throw failure();
    });
  });
  await new Promise<void>((resolve, reject) => {
    server.once('error', reject); server.listen(file, () => { server.removeListener('error', reject); resolve(); });
  });
  fs.chmodSync(file, 0o600); const bound = ownedSocket(file);
  server.on('error', () => { finish(); });
  const timer = setTimeout(finish, lifetime);
  const cancel = () => { interrupted = true; for (const socket of clients) socket.destroy(); server.close(); finish(); };
  process.once('SIGTERM', cancel); process.once('SIGINT', cancel);
  return {
    wait: async () => { await waited; if (interrupted) throw failure(); return done; },
    close: async () => {
      clearTimeout(timer); for (const socket of clients) socket.destroy();
      await new Promise<void>(resolve => { if (!server.listening) resolve(); else server.close(() => resolve()); });
      process.removeListener('SIGTERM', cancel); process.removeListener('SIGINT', cancel);
      try { const current = ownedSocket(file); if (current.ino === bound.ino && current.dev === bound.dev) fs.unlinkSync(file); }
      catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
    },
  };
}

async function receiveOnce(file: string, snapshot: CodexLaunchEnvironment, timeout: number): Promise<NodeJS.ProcessEnv> {
  privateParent(file); ownedSocket(file); validateCodexLaunchEnvironment(snapshot, snapshot.values.CODEX_HOME);
  return new Promise((resolve, reject) => {
    const socket = net.createConnection(file); let values: NodeJS.ProcessEnv | undefined, settled = false;
    const fail = (error?: Error) => {
      if (!settled) {
        settled = true;
        const code = (error as NodeJS.ErrnoException | undefined)?.code;
        reject(Object.assign(failure(), { code: ['ENOENT', 'ECONNREFUSED'].includes(code ?? '') ? code : undefined }));
      }
      socket.destroy();
    };
    socket.setTimeout(timeout, fail); socket.on('error', fail); socket.on('close', () => { if (!settled) fail(); });
    socket.once('connect', () => socket.write(JSON.stringify({ fingerprint: snapshot.fingerprint }) + '\n'));
    lines(socket, 2 * 1024 * 1024, value => {
      if (!values) {
        if (!row(value) || value.fingerprint !== snapshot.fingerprint || !row(value.values)
          || Object.keys(value).length !== 2 || Object.keys(value.values).length !== Object.keys(snapshot.inheritedHashes).length) throw failure();
        for (const [name, expected] of Object.entries(snapshot.inheritedHashes)) {
          const entry = value.values[name]; if (typeof entry !== 'string' || entry.includes('\0') || hash(entry) !== expected) throw failure();
        }
        values = value.values as NodeJS.ProcessEnv;
        socket.write(JSON.stringify({ fingerprint: snapshot.fingerprint, received: true }) + '\n');
      } else {
        if (!row(value) || value.ready !== true || Object.keys(value).length !== 1) throw failure();
        settled = true; resolve(values); socket.end();
      }
    });
  });
}

export async function receiveCodexEnvironment(file: string, snapshot: CodexLaunchEnvironment, timeout = 30000): Promise<NodeJS.ProcessEnv> {
  // A replacement creation helper may reopen the same private handoff after
  // controller loss. Wait only for that original channel; never spawn/replay a
  // provider or accept different environment bytes during this wait.
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try { return await receiveOnce(file, snapshot, Math.min(5000, deadline - Date.now())); }
    catch (error) {
      if (!['ENOENT', 'ECONNREFUSED'].includes((error as NodeJS.ErrnoException).code ?? '')) throw failure();
      await new Promise(resolve => setTimeout(resolve, Math.min(100, Math.max(0, deadline - Date.now()))));
    }
  }
  throw failure();
}
