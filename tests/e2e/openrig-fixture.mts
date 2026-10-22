// Published OpenRig daemon + actual text UI, isolated from developer state.
// An interactive provider stand-in tests enrollment/delivery without credentials.
// Actual provider coding is qualified separately in the live workload record.
import fs from 'node:fs';
import path from 'node:path';
import { createServer } from 'node:net';
import { spawnSync } from 'node:child_process';

export async function startOpenRigFixture(home: string): Promise<() => void> {
  // Drop the caller's routing once, before starting the isolated server. The
  // wrapper must preserve identities subsequently supplied by Rig enrollment.
  for (const key of ['TMUX', 'TMUX_PANE', 'OPENRIG_SESSION_NAME', 'RIGGED_SESSION_NAME', 'OPENRIG_URL', 'RIGGED_URL', 'OPENRIG_HOST', 'RIGGED_HOST', 'OPENRIG_NODE_ID', 'OPENRIG_ACTIVITY_HOOK_TOKEN']) delete process.env[key];
  const cli = process.env.WEBMUX_OPENRIG_CLI;
  if (!cli || !fs.existsSync(cli)) throw new Error('Set WEBMUX_OPENRIG_CLI to the installed OpenRig dist/bin-wrapper.js');
  const reserved = createServer();
  await new Promise<void>(resolve => reserved.listen(0, '127.0.0.1', resolve));
  const address = reserved.address();
  if (!address || typeof address === 'string') throw new Error('Cannot reserve a daemon port');
  await new Promise<void>(resolve => reserved.close(() => resolve()));
  const directory = path.join(home, 'openrig-bin');
  const rigHome = path.join(home, 'openrig');
  fs.mkdirSync(directory, { mode: 0o700 });
  fs.mkdirSync(rigHome, { mode: 0o700 });
  const quote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
  const executable = path.join(directory, 'rig');
  // Keep transport/sender identity for clients launched by the managed runner.
  // Isolate the tmux server even when the wrapper is called by the test process.
  fs.writeFileSync(executable, `#!/bin/sh\nexport OPENRIG_HOME=${quote(rigHome)}\nexport HOME=${quote(rigHome)}\nexport TMUX_TMPDIR=${quote(process.env.TMUX_TMPDIR ?? '')}\nexec ${quote(process.execPath)} ${quote(cli)} "$@"\n`, { mode: 0o700 });
  fs.writeFileSync(path.join(directory, 'codex'), `#!${process.execPath}
const { execFileSync } = require('node:child_process');
const readline = require('node:readline');
const run = args => execFileSync(${JSON.stringify(executable)}, args, { encoding: 'utf8', timeout: 10000 });
console.log('FIXTURE_IDENTITY ' + run(['whoami', '--json']).trim());
console.log('FIXTURE_READY ' + process.env.OPENRIG_SESSION_NAME);
readline.createInterface({ input: process.stdin, terminal: false }).on('line', raw => {
  const line = raw.replace(/\\x1b\\[(?:200|201)~/g, '');
  console.log('FIXTURE_INPUT ' + line);
  try {
    const send = line.match(/^fixture-send (\\S+) (.+)$/);
    if (send) { console.log(run(['send', send[1], send[2], '--json'])); return; }
    const challenge = line.match(/WM_CHALLENGE:([a-z0-9-]+):([a-f0-9]+)/);
    if (challenge) console.log(run(['send', challenge[1], 'WM_ACK:' + challenge[2], '--json']));
  } catch (error) { console.log('FIXTURE_ERROR ' + error.message); }
});
`, { mode: 0o700 });
  // Simulate an already-running tmux server whose PATH predates the service's
  // dedicated Rig/provider wrappers. Managed launchers must bind the service
  // PATH explicitly, and project shutdown must preserve this unrelated session.
  const sentinel = spawnSync('tmux', ['-f', '/dev/null', 'new-session', '-d', '-s', 'webmux-fixture-path', '/bin/sleep 600'], { encoding: 'utf8', timeout: 5000 });
  if (sentinel.status !== 0) throw new Error(`Cannot start isolated tmux fixture: ${sentinel.stderr}`);
  fs.writeFileSync(path.join(home, 'openrig-tmux-directory'), process.env.TMUX_TMPDIR ?? '');
  process.env.PATH = `${directory}${path.delimiter}${process.env.PATH ?? ''}`;
  const result = spawnSync(executable, ['daemon', 'start', '--host', '127.0.0.1', '--port', String(address.port), '--no-kernel'], { encoding: 'utf8', timeout: 30000 });
  if (result.status !== 0) throw new Error(`Cannot start real OpenRig daemon: ${result.stdout} ${result.stderr}`);
  fs.writeFileSync(path.join(home, 'openrig-daemon-url'), `http://127.0.0.1:${address.port}`);
  return () => {
    spawnSync(executable, ['daemon', 'stop'], { timeout: 15000, stdio: 'ignore' });
  };
}
