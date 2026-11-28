import fs from 'node:fs';
import { randomBytes } from 'node:crypto';
import path from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import { startOpenRigFixture } from './openrig-fixture.mts';
import { prepareBrowserTestHome } from '../../webmux/scripts/browser-test-home.mts';
import { selectBrowserCompanion } from '../../webmux/scripts/browser-companion-preflight.mts';
import { prebuiltBrowserFixture } from '../../webmux/scripts/prebuilt-browser-fixture.mts';

const prebuilt = prebuiltBrowserFixture(process.env, process.platform);
const testHome = path.resolve(import.meta.dirname, '.test-home');
const defaultsDir = path.resolve(import.meta.dirname, '../../webmux/config.defaults');
if (process.platform !== 'win32' && !process.env.TMUX_TMPDIR) {
  throw new Error('Run browser tests through npm run test:e2e so the outer runner owns private tmux cleanup');
}
const authMode = process.env.WEBMUX_E2E_AUTH ?? 'none';
if (authMode !== 'none' && authMode !== 'local') throw new Error(`Unsupported browser-test auth mode: ${authMode}`);

// Initialize before starting the server: Playwright's globalSetup runs AFTER its
// webServer starts. Never remove watched storage from a running server, including
// during teardown (especially on Windows). Preserve prior Windows homes because
// an unconfirmed native owner can outlive the controller; Unix fixtures use the
// outer runner's private tmux teardown before their next allocation.
const retainedHome = prepareBrowserTestHome(testHome, process.platform === 'win32');
if (retainedHome) console.error(`Retained previous Windows fixture home for original-provider recovery: ${retainedHome}`);
for (const directory of ['config', 'logs', 'data']) {
  fs.mkdirSync(path.join(testHome, directory), { recursive: true });
}
for (const entry of fs.readdirSync(defaultsDir, { withFileTypes: true })) {
  if (entry.isDirectory()) continue;
  let content = fs.readFileSync(path.join(defaultsDir, entry.name), 'utf8');
  if (entry.name === 'auth.yaml') content = content.replace('mode: local', `mode: ${authMode}`);
  if (entry.name === 'app.yaml' && prebuilt) {
    if (!content.includes('  listen_host: 0.0.0.0')) throw new Error('Expected the fixture listener default');
    content = content.replace('  listen_host: 0.0.0.0', '  listen_host: 127.0.0.1');
  }
  if (entry.name === 'app.yaml' && process.env.WEBMUX_REAL_OPENRIG === '1') content = content.replace('enabled_providers: []', 'enabled_providers: [rig]');
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
// Terminal workflows use an already-configured installation. First-run Mac
// setup has its own component and API tests; its dialog would steal input here.
fs.writeFileSync(path.join(testHome, 'config', 'macos-setup.yaml'), 'skipped: true\n');
// A private, per-run credential for the two-server federation browser fixture.
process.env.WEBMUX_E2E_FEDERATION_TOKEN = randomBytes(32).toString('hex');
fs.writeFileSync(path.join(testHome, 'federation-token'), process.env.WEBMUX_E2E_FEDERATION_TOKEN, { mode: 0o600 });
process.env.WEBMUX_HOME = testHome;
// The companion and Playwright driver have independent launch lifecycles. On
// macOS use the installed companion browser, as production does, when available.
// A working managed headless-shell driver does not qualify managed full Chrome
// for the companion's native CDP and saved-profile operations (#282, #764).
process.env.WEBMUX_BROWSER_EXECUTABLE = selectBrowserCompanion().executablePath;
if (process.env.WEBMUX_REAL_SSH === '1' || process.env.WEBMUX_REAL_MOSH === '1') {
  if (process.platform !== 'linux' && !(process.env.WEBMUX_REAL_MOSH === '1' && process.platform === 'darwin')) throw new Error('The real SSH/Mosh fixture requires Linux, or macOS for Mosh');
  const directory = path.join(testHome, 'ssh-bin');
  fs.mkdirSync(directory, { mode: 0o700 });
  const quote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
  // Invoke the actual system client with isolated trust/configuration files.
  const args = ['-F', '/dev/null', '-o', `UserKnownHostsFile=${path.join(testHome, 'known_hosts')}`,
    '-o', 'GlobalKnownHostsFile=/dev/null', '-o', 'IdentityAgent=none', '-o', 'IdentitiesOnly=yes', '-o', 'BatchMode=yes'];
  fs.writeFileSync(path.join(directory, 'ssh'), `#!/bin/sh\nexec /usr/bin/ssh ${args.map(quote).join(' ')} "$@"\n`, { mode: 0o700 });
  process.env.PATH = `${directory}${path.delimiter}${process.env.PATH ?? ''}`;
}
if (process.env.WEBMUX_VISUAL_PARITY === '1' || process.env.WEBMUX_REAL_VNC === '1' || process.env.WEBMUX_REAL_RDP === '1' || process.env.WEBMUX_REAL_MOSH === '1') process.env.WEBMUX_ALLOW_LOCAL_TARGETS = '1';
const binary = prebuilt ?? path.join(testHome, process.platform === 'win32' ? 'webmux.exe' : 'webmux');
if (!prebuilt) {
  const build = spawnSync('go', ['run', './cmd/build', '-o', binary], {
    cwd: path.resolve(import.meta.dirname, '../../webmux/server'), stdio: 'inherit',
  });
  if (build.error) throw build.error;
  if (build.status !== 0) process.exit(build.status ?? 1);
}
const stopOpenRig = process.env.WEBMUX_REAL_OPENRIG === '1' ? await startOpenRigFixture(testHome) : () => {};
const child = spawn(binary, [], { stdio: 'inherit', env: { ...process.env, WEBMUX_COORDINATION_DISABLED: process.env.WEBMUX_REAL_OPENRIG === '1' ? '0' : '1', WEBMUX_ROOT: '', WEBMUX_DEBUG_TERMINAL: '1' } });
for (const signal of ['SIGTERM', 'SIGINT'] as const) process.on(signal, () => child.kill(signal));
child.on('error', error => { console.error(error); process.exitCode = 1; });
child.on('exit', code => {
  stopOpenRig();
  process.exitCode = code ?? 0;
});
