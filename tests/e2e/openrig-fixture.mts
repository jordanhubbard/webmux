// Published OpenRig daemon + actual text UI, isolated from developer state.
// No provider credentials or autonomous agents are used by this browser test.
import fs from 'node:fs';
import path from 'node:path';
import { createServer } from 'node:net';
import { spawnSync } from 'node:child_process';

export async function startOpenRigFixture(home: string): Promise<() => void> {
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
  fs.writeFileSync(executable, `#!/bin/sh\nexport OPENRIG_HOME=${quote(rigHome)}\nexport HOME=${quote(rigHome)}\nunset OPENRIG_SESSION_NAME RIGGED_SESSION_NAME OPENRIG_URL RIGGED_URL OPENRIG_HOST RIGGED_HOST\nexec ${quote(process.execPath)} ${quote(cli)} "$@"\n`, { mode: 0o700 });
  process.env.PATH = `${directory}${path.delimiter}${process.env.PATH ?? ''}`;
  const result = spawnSync(executable, ['daemon', 'start', '--host', '127.0.0.1', '--port', String(address.port), '--no-kernel'], { encoding: 'utf8', timeout: 30000 });
  if (result.status !== 0) throw new Error(`Cannot start real OpenRig daemon: ${result.stdout} ${result.stderr}`);
  fs.writeFileSync(path.join(home, 'openrig-daemon-url'), `http://127.0.0.1:${address.port}`);
  return () => {
    spawnSync(executable, ['daemon', 'stop'], { timeout: 15000, stdio: 'ignore' });
  };
}
