import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
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
