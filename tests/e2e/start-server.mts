import fs from 'node:fs';
import path from 'node:path';
import { spawn, spawnSync } from 'node:child_process';

const testHome = path.resolve(import.meta.dirname, '.test-home');
const defaultsDir = path.resolve(import.meta.dirname, '../../webmux/config.defaults');
const authMode = process.env.WEBMUX_E2E_AUTH ?? 'none';
if (authMode !== 'none' && authMode !== 'local') throw new Error(`Unsupported browser-test auth mode: ${authMode}`);

// Initialize before starting the server: Playwright's globalSetup runs AFTER its
// webServer starts. Never remove watched storage from a running server, including
// during teardown (especially on Windows). The next launch clears this fixture.
fs.rmSync(testHome, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
for (const directory of ['config', 'logs', 'data']) {
  fs.mkdirSync(path.join(testHome, directory), { recursive: true });
}
for (const entry of fs.readdirSync(defaultsDir, { withFileTypes: true })) {
  if (entry.isDirectory()) continue;
  let content = fs.readFileSync(path.join(defaultsDir, entry.name), 'utf8');
  if (entry.name === 'auth.yaml') content = content.replace('mode: local', `mode: ${authMode}`);
  if (entry.name === 'app.yaml' && process.env.WEBMUX_VISUAL_PARITY === '1') {
    if (!content.includes('    port: 4822')) throw new Error('Expected the default guacd port in the visual fixture');
    content = content.replace('    port: 4822', '    port: 14822');
  }
  if (entry.name === 'app.yaml' && process.env.WEBMUX_REAL_RDP === '1') {
    if (!content.includes('    port: 4822')) throw new Error('Expected default guacd port for real RDP fixture');
    content = content.replace('    port: 4822', '    port: 14823');
  }
  fs.writeFileSync(path.join(testHome, 'config', entry.name), content);
}
process.env.WEBMUX_HOME = testHome;
if (process.env.WEBMUX_REAL_SSH === '1') {
  if (process.platform !== 'linux') throw new Error('The real SSH fixture requires Linux');
  const directory = path.join(testHome, 'ssh-bin');
  fs.mkdirSync(directory, { mode: 0o700 });
  const quote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
  // Invoke the actual system client with isolated trust/configuration files.
  const args = ['-F', '/dev/null', '-o', `UserKnownHostsFile=${path.join(testHome, 'known_hosts')}`,
    '-o', 'GlobalKnownHostsFile=/dev/null', '-o', 'IdentityAgent=none', '-o', 'IdentitiesOnly=yes', '-o', 'BatchMode=yes'];
  fs.writeFileSync(path.join(directory, 'ssh'), `#!/bin/sh\nexec /usr/bin/ssh ${args.map(quote).join(' ')} "$@"\n`, { mode: 0o700 });
  process.env.PATH = `${directory}${path.delimiter}${process.env.PATH ?? ''}`;
}
if (process.env.WEBMUX_VISUAL_PARITY === '1' || process.env.WEBMUX_REAL_VNC === '1' || process.env.WEBMUX_REAL_RDP === '1') process.env.WEBMUX_ALLOW_LOCAL_TARGETS = '1';
const binary = path.join(testHome, process.platform === 'win32' ? 'webmux.exe' : 'webmux');
const build = spawnSync('go', ['run', './cmd/build', '-o', binary], {
  cwd: path.resolve(import.meta.dirname, '../../webmux/server'), stdio: 'inherit',
});
if (build.error) throw build.error;
if (build.status !== 0) process.exit(build.status ?? 1);
const child = spawn(binary, [], { stdio: 'inherit', env: { ...process.env, WEBMUX_ROOT: '', WEBMUX_DEBUG_TERMINAL: '1' } });
for (const signal of ['SIGTERM', 'SIGINT'] as const) process.on(signal, () => child.kill(signal));
child.on('error', error => { console.error(error); process.exitCode = 1; });
child.on('exit', code => { process.exitCode = code ?? 0; });
