#!/usr/bin/env node
// Verify Make's manual controls without touching installed OS services.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import net from 'node:net';
import { once } from 'node:events';
import { spawnSync } from 'node:child_process';

const backend = process.argv[2] ?? 'go';
assert(backend === 'go' || backend === 'node', 'Expected go or node');
assert(process.platform !== 'win32', 'Source Make controls are Unix-only');
const repo = path.resolve(import.meta.dirname, '..');
const home = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-source-smoke-'));
async function freePort(): Promise<number> {
  const listener = net.createServer();
  listener.listen(0, '127.0.0.1'); await once(listener, 'listening');
  const address = listener.address(); assert(address && typeof address === 'object');
  await new Promise<void>((resolve, reject) => listener.close(error => error ? reject(error) : resolve()));
  return address.port;
}
let port = 0;
const app = fs.readFileSync(path.join(repo, 'webmux/config.defaults/app.yaml'), 'utf8')
  .replace('name: webmux', 'name: source-control-smoke').replace('listen_host: 0.0.0.0', 'listen_host: 127.0.0.1');
fs.mkdirSync(path.join(home, 'config')); fs.writeFileSync(path.join(home, 'config/app.yaml'), app);
function control(target: '_start_manual' | '_stop_manual'): void {
  const result = spawnSync('make', ['--no-print-directory', target, `WEBMUX_BACKEND=${backend}`,
    `WEBMUX_HOME=${home}`, `HTTP_PORT=${port}`, 'HTTPS_PORT=8443'], {
    cwd: repo, timeout: 20000, encoding: 'utf8',
    env: { ...process.env, JWT_SECRET: '', WEBMUX_SLAVE_HOST: '', WEBMUX_SLAVE_PORT: '' },
  });
  if (result.error) throw result.error;
  assert.equal(result.status, 0, result.stdout + result.stderr);
}
let started = false;
try {
  for (let attempt = 0; attempt < 2; attempt++) {
    // The existing port helper avoids ports in TIME_WAIT, so choose a fresh
    // candidate on restart while keeping the same persisted state directory.
    port = await freePort();
    const base = `http://127.0.0.1:${port}`;
    started = true; control('_start_manual');
    const pid = fs.readFileSync(path.join(home, '.webmux.pid'), 'utf8').trim();
    assert.match(pid, /^[1-9]\d*$/); process.kill(Number(pid), 0);
    let healthy = false;
    for (let retry = 0; retry < 50; retry++) {
      try {
        const response = await fetch(`${base}/api/health`, { signal: AbortSignal.timeout(1000) });
        const value: unknown = await response.json();
        assert(value && typeof value === 'object' && 'status' in value && 'name' in value);
        assert.equal(value.status, 'ok'); assert.equal(value.name, 'source-control-smoke');
        healthy = true; break;
      } catch { await new Promise(resolve => setTimeout(resolve, 100)); }
    }
    assert(healthy, 'Manual source server did not become healthy');
    assert.equal(fs.readFileSync(path.join(home, 'config/app.yaml'), 'utf8'), app);
    assert(fs.existsSync(path.join(home, 'config/auth.yaml')));
    control('_stop_manual'); started = false;
    assert(!fs.existsSync(path.join(home, '.webmux.pid')), 'Stop retained pidfile');
    await assert.rejects(fetch(`${base}/api/health`, { signal: AbortSignal.timeout(1000) }));
  }
  console.log(`${backend}: Make manual start/stop, restart and preserved state passed.`);
} catch (error) {
  const log = path.join(home, 'logs/webmux.log');
  if (fs.existsSync(log)) console.error(fs.readFileSync(log, 'utf8'));
  throw error;
} finally {
  if (started) control('_stop_manual');
  fs.rmSync(home, { recursive: true, force: true });
}
