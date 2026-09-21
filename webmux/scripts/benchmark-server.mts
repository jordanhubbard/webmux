// Compare both backends under the same isolated local workload.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import net from 'node:net';
import { spawn, execFileSync, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import { randomBytes } from 'node:crypto';

const root = path.resolve(import.meta.dirname, '..');
const outputFile = path.resolve(process.argv[2] ?? path.join(root, 'benchmark-results.json'));
const temporary = await fs.mkdtemp(path.join(os.tmpdir(), 'webmux-benchmark-'));
const binary = path.join(temporary, process.platform === 'win32' ? 'webmux.exe' : 'webmux');
const rounds = 5;
type Backend = 'node' | 'go';
interface TerminalSample { pingMs: number[]; bulkMiBPerSecond: number }
interface Sample extends TerminalSample { startupMs: number; idleRssBytes: number; activeRssBytes: number; concurrent: TerminalSample[]; concurrentRssBytes: number }
const samples: Record<Backend, Sample[]> = { node: [], go: [] };
const sleep = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));
function record(value: unknown): Record<string, unknown> {
  assert(value && typeof value === 'object' && !Array.isArray(value)); return value as Record<string, unknown>;
}
function stringField(value: unknown, key: string): string {
  const field = record(value)[key]; assert.equal(typeof field, 'string'); return field as string;
}
function rss(pid: number): number {
  const result = process.platform === 'win32'
    ? execFileSync('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', `(Get-Process -Id ${pid}).WorkingSet64`], { encoding: 'utf8', timeout: 5000 })
    : execFileSync('ps', ['-o', 'rss=', '-p', String(pid)], { encoding: 'utf8', timeout: 5000 });
  const value = Number(result.trim()) * (process.platform === 'win32' ? 1 : 1024);
  assert(Number.isFinite(value) && value > 0, 'Cannot measure backend RSS'); return value;
}
async function stop(child: ChildProcess): Promise<void> {
  if (child.exitCode !== null || child.signalCode !== null) return;
  const closed = once(child, 'close');
  child.kill('SIGTERM');
  const timer = setTimeout(() => child.kill('SIGKILL'), 10000);
  try {
    const [code] = await closed;
    if (process.platform !== 'win32') assert.equal(code, 0);
  } finally { clearTimeout(timer); }
}
async function sample(backend: Backend): Promise<Sample> {
  const home = await fs.mkdtemp(path.join(temporary, `${backend}-`));
  await fs.cp(path.join(root, 'config.defaults'), path.join(home, 'config'), { recursive: true });
  const appFile = path.join(home, 'config/app.yaml');
  await fs.writeFile(appFile, (await fs.readFile(appFile, 'utf8')).replace('listen_host: 0.0.0.0', 'listen_host: 127.0.0.1'));
  await fs.writeFile(path.join(home, 'config/auth.yaml'), `auth:\n  mode: none\n  users: []\n  jwt_secret: ${randomBytes(32).toString('hex')}\n`);
  const listener = net.createServer(); listener.listen(0, '127.0.0.1'); await once(listener, 'listening');
  const address = listener.address(); assert(address && typeof address === 'object');
  await new Promise<void>((resolve, reject) => listener.close(error => error ? reject(error) : resolve()));
  const base = `http://127.0.0.1:${address.port}`;
  const quote = (value: string) => process.platform === 'win32' ? `"${value}"` : `'${value.replaceAll("'", `'"'"'`)}'`;
  const started = performance.now();
  const child = spawn(backend === 'go' ? binary : process.execPath, backend === 'go' ? [] : [path.join(root, 'backend/dist/index.js')], {
    cwd: root, stdio: ['ignore', 'pipe', 'pipe'], env: {
      ...process.env, WEBMUX_ROOT: root, WEBMUX_HOME: home, HTTP_PORT: String(address.port), HTTPS_PORT: '0',
      JWT_SECRET: '', WEBMUX_SLAVE_HOST: '', WEBMUX_SLAVE_PORT: '',
      WEBMUX_RCC_URL: '', WEBMUX_RCC_TOKEN: '', LOOM_RCC_BRAIN_URL: '', LOOM_RCC_AGENT_TOKEN: '', NVIDIA_API_KEY: '', OPENAI_API_KEY: '',
      WEBMUX_EXEC_COMMAND: `${quote(process.execPath)} ${quote(path.join(root, 'scripts/performance-fixture.mts'))}`,
    },
  });
  let logs = '', spawnError: Error | undefined;
  const sockets: WebSocket[] = [];
  const sessionIDs: string[] = [];
  child.on('error', error => { spawnError = error; });
  child.stdout?.on('data', (data: Buffer) => { logs = (logs + data.toString()).slice(-8192); });
  child.stderr?.on('data', (data: Buffer) => { logs = (logs + data.toString()).slice(-8192); });
  async function request(method: string, route: string, body?: unknown): Promise<unknown> {
    const response = await fetch(base + route, { method, signal: AbortSignal.timeout(10000),
      headers: { 'Content-Type': 'application/json' }, ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    });
    assert(response.ok, `${route}: ${response.status}`);
    return response.status === 204 ? undefined : response.json();
  }
  try {
    let healthy = false;
    while (performance.now() - started < 15000) {
      if (spawnError) throw spawnError;
      if (child.exitCode !== null || child.signalCode !== null) throw new Error(logs);
      try { await request('GET', '/api/health'); healthy = true; break; } catch { await sleep(10); }
    }
    assert(healthy, `Startup timed out: ${logs}`);
    const startupMs = performance.now() - started;
    assert(child.pid !== undefined);
    // Allow startup maintenance to settle before measuring backend-only RSS.
    await sleep(100);
    const idleRssBytes = rss(child.pid);
    const token = stringField(await request('POST', '/api/auth/refresh'), 'token');
    async function openTerminal() {
      const sessionID = stringField(await request('POST', '/api/sessions', { hostname: 'localhost', username: 'fixture', transport: 'exec' }), 'id');
      sessionIDs.push(sessionID);
      let tail = '', payloadBytes = 0, socketError: Error | undefined;
      const changes = new Set<() => void>();
      const socket = new WebSocket(`${base.replace(/^http/, 'ws')}/api/term/${sessionID}?token=${encodeURIComponent(token)}`);
      sockets.push(socket);
      socket.addEventListener('error', () => { socketError = new Error('Terminal WebSocket failed'); for (const fn of changes) fn(); });
      socket.addEventListener('close', event => { socketError = new Error(`Terminal WebSocket closed (${event.code}: ${event.reason})`); for (const fn of changes) fn(); });
      socket.addEventListener('message', event => {
        const message = record(JSON.parse(String(event.data)));
        if (message.type === 'output' && typeof message.data === 'string') {
          tail = (tail + message.data).slice(-16384);
          for (const character of message.data) if (character === 'x') payloadBytes++;
          for (const fn of changes) fn();
        }
      });
      function waitFor(marker: string): Promise<void> {
        return new Promise((resolve, reject) => {
          const timeout = setTimeout(() => { changes.delete(check); reject(new Error(`Missing ${marker}: ${tail.slice(-512)}`)); }, 10000);
          function check(): void {
            if (socketError || tail.includes(marker)) {
              clearTimeout(timeout); changes.delete(check);
              if (socketError) reject(socketError); else resolve();
            }
          }
          changes.add(check); check();
        });
      }
      await waitFor('benchmark-ready');
      return { sessionID, socket, measure: async (): Promise<TerminalSample> => {
        const pingMs: number[] = [];
        for (let i = 0; i < 35; i++) {
          const start = performance.now();
          socket.send(JSON.stringify({ type: 'input', data: `ping-${i}\r` }));
          await waitFor(`pong-${i}:done`);
          if (i >= 5) pingMs.push(performance.now() - start);
        }
        const beforeBytes = payloadBytes, transferStart = performance.now();
        socket.send(JSON.stringify({ type: 'input', data: 'bulk\r' }));
        await waitFor('benchmark-bulk-done');
        const transferMs = performance.now() - transferStart;
        assert.equal(payloadBytes - beforeBytes, 1024 * 1024, 'PTY payload lost or duplicated');
        return { pingMs, bulkMiBPerSecond: 1000 / transferMs };
      } };
    }
    const single = await openTerminal();
    const singleSample = await single.measure();
    const activeRssBytes = rss(child.pid);
    single.socket.close();
    await request('DELETE', `/api/sessions/${single.sessionID}`);
    sessionIDs.splice(sessionIDs.indexOf(single.sessionID), 1);
    const clients = [];
    for (let index = 0; index < 4; index++) clients.push(await openTerminal());
    const concurrent = await Promise.all(clients.map(client => client.measure()));
    const concurrentRssBytes = rss(child.pid);
    return { startupMs, idleRssBytes, activeRssBytes, ...singleSample, concurrent, concurrentRssBytes };
  } catch (error) { console.error(`${backend}: ${logs}`); throw error; }
  finally {
    for (const socket of sockets) socket.close();
    try { for (const id of sessionIDs) await request('DELETE', `/api/sessions/${id}`); }
    finally { await stop(child); }
  }
}
function percentile(values: number[], fraction: number): number {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.min(sorted.length - 1, Math.ceil(sorted.length * fraction) - 1)];
}
try {
  execFileSync('go', ['build', '-trimpath', '-o', binary, './cmd/webmux'], { cwd: path.join(root, 'server'), stdio: 'inherit' });
  for (let round = 0; round < rounds; round++) {
    for (const backend of (round % 2 === 0 ? ['node', 'go'] : ['go', 'node']) as Backend[]) {
      samples[backend].push(await sample(backend));
    }
  }
  const summary = Object.fromEntries((['node', 'go'] as const).map(backend => [backend, {
    medianStartupMs: percentile(samples[backend].map(sample => sample.startupMs), 0.5),
    medianIdleRssBytes: percentile(samples[backend].map(sample => sample.idleRssBytes), 0.5),
    medianActiveRssBytes: percentile(samples[backend].map(sample => sample.activeRssBytes), 0.5),
    p95PingMs: percentile(samples[backend].flatMap(sample => sample.pingMs), 0.95),
    p95ConcurrentPingMs: percentile(samples[backend].flatMap(sample => sample.concurrent.flatMap(client => client.pingMs)), 0.95),
    medianConcurrentRssBytes: percentile(samples[backend].map(sample => sample.concurrentRssBytes), 0.5),
    medianConcurrentClientMiBPerSecond: percentile(samples[backend].flatMap(sample => sample.concurrent.map(client => client.bulkMiBPerSecond)), 0.5),
    medianBulkMiBPerSecond: percentile(samples[backend].map(sample => sample.bulkMiBPerSecond), 0.5),
  }]));
  const report = { timestamp: new Date().toISOString(), platform: process.platform, arch: process.arch, os: os.release(),
    cpu: os.cpus()[0]?.model, totalMemoryBytes: os.totalmem(),
    node: process.version, go: execFileSync('go', ['version'], { encoding: 'utf8' }).trim(),
    revision: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim(),
    workingTreeDirty: execFileSync('git', ['status', '--porcelain'], { cwd: root, encoding: 'utf8' }).trim().length > 0,
    rounds, workload: 'auth=none; one local PTY, then four concurrent PTYs; each: 5 warmup + 30 measured pings; 1 MiB output; RSS excludes child/client; 10ms startup polling', summary, samples };
  await fs.mkdir(path.dirname(outputFile), { recursive: true });
  await fs.writeFile(outputFile, JSON.stringify(report, null, 2) + '\n');
  console.log(JSON.stringify(summary, null, 2));
  console.log(`Performance report: ${outputFile}`);
} finally { await fs.rm(temporary, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }); }
