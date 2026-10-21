// Exercise the published OpenRig CLI and real tmux with a controlled daemon
// projection. This validates attachment, not autonomous agent coordination.
import fs from 'node:fs';
import path from 'node:path';
import { createServer } from 'node:http';
import { spawnSync } from 'node:child_process';

export async function startOpenRigFixture(home: string): Promise<() => void> {
  const cli = process.env.WEBMUX_OPENRIG_CLI;
  if (!cli || !fs.existsSync(cli)) throw new Error('Set WEBMUX_OPENRIG_CLI to the installed OpenRig dist/bin-wrapper.js');
  const target = 'webmux-openrig-dashboard';
  const result = spawnSync('tmux', ['-f', '/dev/null', 'new-session', '-d', '-s', target,
    process.execPath, path.resolve(import.meta.dirname, 'visual-terminal-fixture.mts')], { encoding: 'utf8', timeout: 5000 });
  if (result.status !== 0) throw new Error(`Cannot start dashboard terminal fixture: ${result.stderr}`);
  const socket = spawnSync('tmux', ['display-message', '-p', '-t', target, '#{socket_path}'], { encoding: 'utf8', timeout: 5000 });
  if (socket.status !== 0) throw new Error('Cannot locate the fixture tmux socket');
  fs.writeFileSync(path.join(home, 'openrig-tmux-socket'), socket.stdout.trim());
  spawnSync('tmux', ['set-option', '-g', 'status', 'off'], { timeout: 5000 });
  const daemon = createServer((request, response) => {
    response.setHeader('Content-Type', 'application/json');
    if (request.url === '/api/ps') response.end(JSON.stringify([{ rigId: 'fixture', rigName: 'kernel' }]));
    else if (request.url === '/api/rigs/fixture/nodes') response.end(JSON.stringify([
      { logicalId: 'operator.human', runtime: 'terminal', canonicalSessionName: target, tmuxAttachCommand: `tmux attach -t ${target}` },
    ]));
    else { response.statusCode = 404; response.end('{}'); }
  });
  await new Promise<void>(resolve => daemon.listen(0, '127.0.0.1', resolve));
  const address = daemon.address();
  if (!address || typeof address === 'string') throw new Error('Cannot resolve fixture daemon address');
  const directory = path.join(home, 'openrig-bin');
  const rigHome = path.join(home, 'openrig');
  fs.mkdirSync(directory);
  fs.mkdirSync(rigHome);
  fs.writeFileSync(path.join(rigHome, '.openrig-fixture'), '');
  const quote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
  fs.writeFileSync(path.join(directory, 'rig'), `#!/bin/sh\nexport OPENRIG_URL=${quote(`http://127.0.0.1:${address.port}`)}\nexport OPENRIG_HOME=${quote(rigHome)}\nexport HOME=${quote(rigHome)}\nunset OPENRIG_SESSION_NAME RIGGED_SESSION_NAME\nexec ${quote(process.execPath)} ${quote(cli)} "$@"\n`, { mode: 0o700 });
  process.env.PATH = `${directory}${path.delimiter}${process.env.PATH ?? ''}`;
  return () => {
    daemon.closeAllConnections();
    daemon.close();
    spawnSync('tmux', ['-S', socket.stdout.trim(), 'kill-session', '-t', `=${target}`], { timeout: 5000 });
  };
}
