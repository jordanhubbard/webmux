import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { spawn } from 'node:child_process';
import { captureCodexLaunchEnvironment } from '../webmux/server/internal/openrigcoord/codex-launch-environment.mts';
import { openCodexEnvironmentChannel, receiveCodexEnvironment } from '../webmux/server/internal/openrigcoord/codex-environment-channel.mts';
const posixTest = process.platform === 'win32' ? test.skip : test;
function fixture(t: { after(fn: () => void): void }) {
  const root = fs.mkdtempSync(path.join(fs.realpathSync('/tmp'), 'wm-env-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const env = { HOME: root, PATH: '/usr/bin:/bin', PRIVATE_KEY: 'SYNTHETIC_PRIVATE_CHANNEL_VALUE' };
  return { root, env, socket: path.join(root, 'e.sock'), snapshot: captureCodexLaunchEnvironment(env, root) };
}
posixTest('live handoff is once-only, private and leaves no secret file or socket', async t => {
  const f = fixture(t), receipt = path.join(f.root, 'receipt.json'); let deliveries = 0;
  const server = await openCodexEnvironmentChannel(f.socket, f.snapshot, f.env, () => {
    deliveries++; fs.writeFileSync(receipt, JSON.stringify({ fingerprint: f.snapshot.fingerprint }), { mode: 0o600 });
  });
  try {
    assert.equal(fs.statSync(f.socket).mode & 0o777, 0o600);
    await assert.rejects(openCodexEnvironmentChannel(f.socket, f.snapshot, f.env, () => {}), /handoff_unavailable/);
    assert.deepEqual(await receiveCodexEnvironment(f.socket, f.snapshot), { PRIVATE_KEY: f.env.PRIVATE_KEY });
    assert.equal(await server.wait(), true); assert.equal(deliveries, 1);
    assert.equal(fs.readFileSync(receipt, 'utf8').includes(f.env.PRIVATE_KEY), false);
  } finally { await server.close(); }
  assert.equal(fs.existsSync(f.socket), false);
});
posixTest('an unconsumed handoff expires and cannot become a receipt', async t => {
  const f = fixture(t); let delivered = false;
  const server = await openCodexEnvironmentChannel(f.socket, f.snapshot, f.env, () => { delivered = true; }, 20);
  try { assert.equal(await server.wait(), false); assert.equal(delivered, false); }
  finally { await server.close(); }
  await assert.rejects(receiveCodexEnvironment(f.socket, f.snapshot, 20));
});
posixTest('the same receiver can wait for a replacement handoff after socket loss', async t => {
  const f = fixture(t), receiving = receiveCodexEnvironment(f.socket, f.snapshot, 3000);
  await new Promise(resolve => setTimeout(resolve, 40));
  const server = await openCodexEnvironmentChannel(f.socket, f.snapshot, f.env, () => {});
  try { assert.deepEqual(await receiving, { PRIVATE_KEY: f.env.PRIVATE_KEY }); assert.equal(await server.wait(), true); }
  finally { await server.close(); }
});
posixTest('foreign file and symlink are refused without replacement', async t => {
  for (const symlink of [false, true]) {
    const f = fixture(t), target = path.join(f.root, 'retained'); fs.writeFileSync(target, 'retain', { mode: 0o600 });
    if (symlink) fs.symlinkSync(target, f.socket); else fs.writeFileSync(f.socket, 'original', { mode: 0o600 });
    await assert.rejects(openCodexEnvironmentChannel(f.socket, f.snapshot, f.env, () => {}));
    assert.equal(fs.readFileSync(target, 'utf8'), 'retain'); assert.equal(fs.lstatSync(f.socket).isSymbolicLink(), symlink);
    if (!symlink) assert.equal(fs.readFileSync(f.socket, 'utf8'), 'original');
  }
});
posixTest('after the owning process crashes a stale owned socket can be reclaimed', async t => {
  const f = fixture(t), script = path.join(f.root, 'server.mjs');
  fs.writeFileSync(script, `import net from 'node:net';import fs from 'node:fs';const file=process.argv[2];net.createServer(()=>{}).listen(file,()=>{fs.chmodSync(file,0o600);console.log('ready')});`);
  const child = spawn(process.execPath, [script, f.socket], { stdio: ['ignore', 'pipe', 'ignore'] });
  t.after(() => { child.kill('SIGKILL'); });
  await new Promise<void>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('socket readiness deadline')), 5000);
    child.stdout.once('data', () => { clearTimeout(timer); resolve(); }); child.once('error', reject);
  });
  const exited = new Promise<void>(resolve => child.once('close', () => resolve())); child.kill('SIGKILL'); await exited;
  assert.equal(fs.lstatSync(f.socket).isSocket(), true);
  const server = await openCodexEnvironmentChannel(f.socket, f.snapshot, f.env, () => {});
  try { assert.deepEqual(await receiveCodexEnvironment(f.socket, f.snapshot), { PRIVATE_KEY: f.env.PRIVATE_KEY }); assert.equal(await server.wait(), true); }
  finally { await server.close(); }
});
