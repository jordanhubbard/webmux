import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import net from 'node:net';
import { spawn, spawnSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';
import { runWithPrivateTmux } from '../webmux/scripts/private-test-tmux.mts';

test('outer browser runner cleans real tmux after success, failure, fixture death and interruption', {
  skip: process.platform === 'win32', timeout: 30_000,
}, async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'wm-owner-'));
  const sentinel = path.join(root, 'sentinel');
  const defaultEnv = { ...process.env };
  delete defaultEnv.TMUX;
  const tmux = (...args: string[]) => spawnSync('tmux', args, { env: defaultEnv, encoding: 'utf8', timeout: 5000 });
  const fixture = path.join(root, 'fixture.mts');
  const receipt = path.join(root, 'receipt.json');
  fs.writeFileSync(fixture, `
import fs from 'node:fs';
import { spawnSync } from 'node:child_process';
const created = spawnSync('tmux', ['-f', '/dev/null', 'new-session', '-d', '-P', '-F', '#{pane_pid}', '-s', 'browser-test', 'sleep 60'], { encoding: 'utf8' });
if (created.status !== 0) throw new Error(created.stderr);
const server = spawnSync('tmux', ['display-message', '-p', '#{pid}'], { encoding: 'utf8' });
fs.writeFileSync(process.env.RECEIPT, JSON.stringify({ directory: process.env.TMUX_TMPDIR, pane: Number(created.stdout.trim()), server: Number(server.stdout.trim()) }));
if (process.argv[2] === 'killed') process.kill(process.pid, 'SIGKILL');
else if (process.argv[2] === 'wait') setInterval(() => {}, 1000);
else process.exit(Number(process.argv[2]));
`);
  const env = { ...process.env, RECEIPT: receipt, TMUX_TMPDIR: root, TMUX: `${sentinel},123,0` };
  const assertStopped = async () => {
    const { directory, pane, server } = JSON.parse(fs.readFileSync(receipt, 'utf8')) as { directory: string; pane: number; server: number };
    assert.notEqual(directory, root, 'must replace the inherited tmux location');
    const socket = path.join(directory, `tmux-${process.getuid!()}`, 'default');
    assert.notEqual(tmux('-S', socket, 'has-session', '-t', '=browser-test').status, 0);
    assert.equal(fs.existsSync(directory), false);
    for (const pid of [pane, server]) {
      assert(pid > 0, 'fixture recorded a real process');
      const alive = () => { try { process.kill(pid, 0); return true; } catch { return false; } };
      const deadline = Date.now() + 2000;
      while (alive() && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 20));
      assert.equal(alive(), false, `private process ${pid} exited`);
    }
    assert.equal(tmux('-S', sentinel, 'has-session', '-t', '=sentinel').status, 0, 'unrelated server survives');
  };
  try {
    assert.equal(tmux('-S', sentinel, '-f', '/dev/null', 'new-session', '-d', '-s', 'sentinel', 'sleep 60').status, 0);
    for (const [mode, expected] of [['0', 0], ['7', 7], ['killed', 1]] as const) {
      assert.equal(await runWithPrivateTmux(process.execPath, [fixture, mode], { cwd: root, env }), expected);
      await assertStopped();
    }
    fs.unlinkSync(receipt);
    const wrapper = path.join(root, 'wrapper.mts');
    const moduleURL = pathToFileURL(path.resolve(import.meta.dirname, '../webmux/scripts/private-test-tmux.mts')).href;
    fs.writeFileSync(wrapper, `import { runWithPrivateTmux } from ${JSON.stringify(moduleURL)};
process.exitCode = await runWithPrivateTmux(process.execPath, [${JSON.stringify(fixture)}, 'wait'], {cwd: ${JSON.stringify(root)}, env: process.env});`);
    const child = spawn(process.execPath, [wrapper], { env, stdio: 'inherit' });
    const exited = new Promise<number | null>((resolve, reject) => { child.once('exit', resolve); child.once('error', reject); });
    try {
      const deadline = Date.now() + 5000;
      while (!fs.existsSync(receipt) && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 20));
      assert.equal(fs.existsSync(receipt), true, 'fixture started before interruption');
      child.kill('SIGTERM');
      assert.equal(await exited, 143);
      await assertStopped();
    } finally { child.kill('SIGKILL'); }
  } finally {
    if (fs.existsSync(receipt)) {
      const { directory } = JSON.parse(fs.readFileSync(receipt, 'utf8'));
      tmux('-S', path.join(directory, `tmux-${process.getuid!()}`, 'default'), 'kill-server');
      fs.rmSync(directory, { recursive: true, force: true });
    }
    tmux('-S', sentinel, 'kill-server');
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test('stale Unix sockets are removed without changing successful or failed test status', {
  skip: process.platform === 'win32', timeout: 15_000,
}, async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'wm-stale-'));
  const receipt = path.join(root, 'receipt');
  const fixture = path.join(root, 'fixture.mts');
  // SIGKILL prevents Node from unlinking the listener. This retains an actual
  // orphaned Unix socket, reproducing Linux teardown without depending on text.
  const listener = `import net from 'node:net'; net.createServer().listen(process.argv[1], () => process.kill(process.pid, 'SIGKILL'));`;
  fs.writeFileSync(fixture, `
import fs from 'node:fs'; import path from 'node:path'; import { spawnSync } from 'node:child_process';
const directory = process.env.TMUX_TMPDIR;
const socketDirectory = path.join(directory, 'tmux-' + process.getuid());
fs.mkdirSync(socketDirectory);
const socket = path.join(socketDirectory, 'default');
const result = spawnSync(process.execPath, ['--input-type=module', '-e', ${JSON.stringify(listener)}, socket]);
if (result.signal !== 'SIGKILL' || !fs.statSync(socket).isSocket()) throw new Error('Expected a retained orphaned socket');
fs.writeFileSync(process.env.RECEIPT, directory);
process.exit(Number(process.argv[2]));
`);
  try {
    for (const status of [0, 7]) {
      assert.equal(await runWithPrivateTmux(process.execPath, [fixture, String(status)], {
        cwd: root, env: { ...process.env, RECEIPT: receipt },
      }), status);
      assert.equal(fs.existsSync(fs.readFileSync(receipt, 'utf8')), false);
    }
  } finally {
    if (fs.existsSync(receipt)) fs.rmSync(fs.readFileSync(receipt, 'utf8'), { recursive: true, force: true });
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test('failed shutdown preserves a listening socket and reports unexpected probe errors', {
  skip: process.platform === 'win32', timeout: 15_000,
}, async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'wm-live-'));
  const bin = path.join(root, 'bin'); fs.mkdirSync(bin);
  fs.writeFileSync(path.join(bin, 'tmux'), `#!${process.execPath}\nprocess.exit(1);\n`, { mode: 0o700 });
  const fixture = path.join(root, 'fixture.mts');
  const receipt = path.join(root, 'receipt');
  const release = path.join(root, 'release');
  fs.writeFileSync(fixture, `import fs from 'node:fs';
fs.writeFileSync(process.env.RECEIPT, process.env.TMUX_TMPDIR);
const timer = setInterval(() => { if (fs.existsSync(process.env.RELEASE)) clearInterval(timer); }, 20);
`);
  const env = { ...process.env, PATH: bin + path.delimiter + process.env.PATH, RECEIPT: receipt, RELEASE: release };
  let directory: string | undefined;
  const server = net.createServer(connection => connection.destroy());
  try {
    const run = runWithPrivateTmux(process.execPath, [fixture], { cwd: root, env });
    // Attach rejection handling before yielding to avoid an unhandled rejection.
    const refused = assert.rejects(run, /Cannot stop private test tmux/);
    const deadline = Date.now() + 5000;
    while (!fs.existsSync(receipt) && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 20));
    assert(fs.existsSync(receipt));
    directory = fs.readFileSync(receipt, 'utf8');
    const socketDirectory = path.join(directory, `tmux-${process.getuid!()}`);
    fs.mkdirSync(socketDirectory);
    const socket = path.join(socketDirectory, 'default');
    await new Promise<void>((resolve, reject) => { server.once('error', reject); server.listen(socket, resolve); });
    fs.writeFileSync(release, 'go');
    await refused;
    assert(fs.statSync(socket).isSocket(), 'live listener must not be unlinked');
    assert(server.listening);
    await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
    // A non-directory path is neither absent nor a refused connection.
    fs.rmSync(socketDirectory, { recursive: true, force: true });
    fs.writeFileSync(socketDirectory, 'unexpected socket directory type');
    fs.writeFileSync(fixture, `import fs from 'node:fs'; import path from 'node:path';
fs.writeFileSync(path.join(process.env.TMUX_TMPDIR, 'tmux-' + process.getuid()), 'not a directory');
fs.writeFileSync(process.env.RECEIPT, process.env.TMUX_TMPDIR);`);
    await assert.rejects(runWithPrivateTmux(process.execPath, [fixture], { cwd: root, env }), { code: 'ENOTDIR' });
    assert(fs.existsSync(fs.readFileSync(receipt, 'utf8')), 'unexpected probe error preserves evidence');
  } finally {
    fs.writeFileSync(release, 'go');
    if (server.listening) await new Promise<void>(resolve => server.close(() => resolve()));
    if (directory) fs.rmSync(directory, { recursive: true, force: true });
    if (fs.existsSync(receipt)) fs.rmSync(fs.readFileSync(receipt, 'utf8'), { recursive: true, force: true });
    fs.rmSync(root, { recursive: true, force: true });
  }
});
