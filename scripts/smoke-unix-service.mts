#!/usr/bin/env node
// Exercise shipped native Unix service templates under a private, temporary label.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import net from 'node:net';
import { once } from 'node:events';
import { randomBytes } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { renderService } from './render-service.mts';

assert(process.platform === 'darwin' || process.platform === 'linux', 'Unix service smoke requires macOS or Linux');
const darwin = process.platform === 'darwin';
const makeMode = process.argv.includes('--make');
const sourceRoot = path.resolve(import.meta.dirname, '../webmux');
await fs.access(path.join(sourceRoot, 'bin/webmux'));
const temporary = await fs.mkdtemp(path.join(os.tmpdir(), 'webmux-unix-service-'));
const root = makeMode ? path.join(temporary, 'application') : sourceRoot;
const home = path.join(temporary, 'state & config');
const nonce = `${process.pid}-${randomBytes(6).toString('hex')}`;
const label = darwin ? `com.webmux.smoke.${nonce}` : `webmux-smoke-${nonce}.service`;
const uid = process.getuid!();
function launch(args: string[], required = true): string {
  const result = spawnSync('launchctl', args, { encoding: 'utf8', timeout: 15000 });
  if (result.error) throw result.error;
  if (required) assert.equal(result.status, 0, `launchctl ${args.join(' ')}: ${result.stderr}`);
  return result.status === 0 ? result.stdout : '';
}
// Template mode uses the system manager with an unprivileged service identity;
// Make mode exercises the actual user manager and requires its bus to be available.
function systemctl(args: string[], required = true): string {
  const result = spawnSync(makeMode ? 'systemctl' : 'sudo',
    [...(makeMode ? ['--user'] : ['-n', 'systemctl']), '--no-pager', ...args], { encoding: 'utf8', timeout: 45000 });
  if (result.error) throw result.error;
  if (required) assert.equal(result.status, 0, `systemctl ${args.join(' ')}: ${result.stderr}`);
  return result.stdout;
}
function property(name: string): string {
  // A missing unit can produce a nonzero status; still require its explicit
  // not-found value at the call site, rather than treating any failure as absence.
  return systemctl(['show', label, `--property=${name}`, '--value'], name !== 'LoadState').trim();
}
const domain = darwin ? (launch(['print', `gui/${uid}`], false) ? `gui/${uid}` : `user/${uid}`) : 'system';
const target = `${domain}/${label}`;
const definition = !darwin && makeMode ? path.join(os.homedir(), '.config/systemd/user', label)
  : path.join(temporary, darwin ? 'fixture.plist' : label);
function makeControl(action: 'install' | 'start' | 'stop' | 'restart' | 'uninstall'): void {
  assert(makeMode && (!darwin || domain === `gui/${uid}`), 'Make fixture requires an isolated service');
  const result = spawnSync('make', ['--no-print-directory', '-o', 'build', action,
    'WEBMUX_BACKEND=go', 'MAKE=make -o build', `NODE=${process.execPath}`, `WEBMUX_DIR=${root}`, `WEBMUX_HOME=${home}`,
    ...(darwin ? [`PLIST=${definition}`, `LAUNCHD_SVC=${target}`] : [`UNIT=${definition}`])], {
    cwd: path.dirname(sourceRoot), encoding: 'utf8', timeout: 30000,
    env: { ...process.env, JWT_SECRET: '', WEBMUX_SLAVE_HOST: '', WEBMUX_SLAVE_PORT: '' },
  });
  if (result.error) throw result.error;
  assert.equal(result.status, 0, result.stdout + result.stderr);
}
let registered = false;
let definitionCreated = false;
let socket: WebSocket | undefined;
async function waitFor(check: () => Promise<boolean>, description: string): Promise<void> {
  const until = Date.now() + 15000;
  while (!await check()) {
    assert(Date.now() < until, `Timed out: ${description}`);
    await new Promise(resolve => setTimeout(resolve, 50));
  }
}
try {
  if (makeMode) {
    if (darwin) assert.equal(domain, `gui/${uid}`, 'Make installer needs a GUI launchd domain');
    else systemctl(['show-environment']);
    await fs.mkdir(path.join(root, 'service'), { recursive: true });
    for (const entry of ['bin', 'web', 'config.defaults']) await fs.symlink(path.join(sourceRoot, entry), path.join(root, entry));
  }
  if (darwin) assert.equal(launch(['print', target], false), '', 'Fixture service label already exists');
  else {
    assert(uid > 0, 'Run the Linux fixture as an unprivileged user with noninteractive sudo');
    assert.equal(property('LoadState'), 'not-found', 'Fixture unit already exists');
    await assert.rejects(fs.lstat(definition), { code: 'ENOENT' });
    await fs.mkdir(path.dirname(definition), { recursive: true });
  }
  await fs.cp(path.join(sourceRoot, 'config.defaults'), path.join(home, 'config'), { recursive: true });
  assert((await fs.lstat(path.join(home, 'config'))).isDirectory(), 'Fixture configuration must be a private directory, not a symlink');
  await fs.mkdir(path.join(home, 'logs'), { recursive: true });
  const listener = net.createServer(); listener.listen(0, '127.0.0.1'); await once(listener, 'listening');
  const address = listener.address(); assert(address && typeof address === 'object');
  await new Promise<void>((resolve, reject) => listener.close(error => error ? reject(error) : resolve()));
  const appFile = path.join(home, 'config/app.yaml');
  const app = (await fs.readFile(appFile, 'utf8')).replace('name: webmux', `name: ${label}`)
    .replace('listen_host: 0.0.0.0', 'listen_host: 127.0.0.1')
    .replace('http_port: 8080', `http_port: ${address.port}`).replace('https_port: 8443', 'https_port: 0')
    .replace('session_logging:\n    enabled: false', 'session_logging:\n    enabled: true');
  await fs.writeFile(appFile, app);
  await fs.writeFile(path.join(home, 'config/auth.yaml'), 'auth:\n  mode: none\n  users: []\n');
  const environment = { HTTP_PORT: String(address.port), HTTPS_PORT: '0', JWT_SECRET: '', WEBMUX_SLAVE_HOST: '', WEBMUX_SLAVE_PORT: '' };
  const templateName = darwin ? 'com.webmux.server.plist.native.template' : 'webmux.service.native.template';
  let template = renderService(await fs.readFile(path.join(sourceRoot, 'service', templateName), 'utf8'), darwin ? 'darwin' : 'linux', {
    root, home, node: process.execPath, searchPath: process.env.PATH ?? '/usr/bin:/bin',
  });
  if (darwin) {
    template = template.replace('<string>com.webmux.server</string>', `<string>${label}</string>`)
      .replace('<key>EnvironmentVariables</key>\n    <dict>', '<key>EnvironmentVariables</key>\n    <dict>\n' +
        Object.entries(environment).map(([key, value]) => `<key>${key}</key><string>${value}</string>`).join('\n'));
    await fs.writeFile(definition, template);
    const lint = spawnSync('plutil', ['-lint', definition], { encoding: 'utf8' });
    assert.equal(lint.status, 0, lint.stdout + lint.stderr);
    // Fail before registration if a template edit stopped any isolation rewrite.
    for (const [key, expected] of Object.entries({ Label: label, 'EnvironmentVariables.WEBMUX_HOME': home,
      'EnvironmentVariables.HTTP_PORT': String(address.port), 'EnvironmentVariables.WEBMUX_SLAVE_HOST': '' })) {
      const extracted = spawnSync('plutil', ['-extract', key, 'raw', '-o', '-', definition], { encoding: 'utf8' });
      assert.equal(extracted.status, 0, extracted.stderr); assert.equal(extracted.stdout.trim(), expected);
    }
  } else {
    // Appending a Service section overrides inherited manager environment.
    // Identity overrides adapt the user-service template to the system manager.
    template += '\n[Service]\n' + (makeMode ? '' : `User=${uid}\nGroup=${process.getgid!()}\n`) +
      Object.entries(environment).map(([key, value]) => `Environment="${key}=${value}"`).join('\n') + '\n';
    await fs.writeFile(definition, template, { mode: 0o600, flag: 'wx' });
    definitionCreated = true;
  }
  const base = `http://127.0.0.1:${address.port}`;
  async function healthy(): Promise<boolean> {
    try {
      const response = await fetch(`${base}/api/health`, { signal: AbortSignal.timeout(1000) });
      const value: unknown = await response.json();
      return response.ok && typeof value === 'object' && value !== null && 'name' in value && value.name === label;
    } catch { return false; }
  }
  registered = true;
  if (makeMode) {
    // Supply the same validated template with only fixture identity/environment
    // substitutions. Make still invokes the real serializer and service commands.
    await fs.writeFile(path.join(root, 'service', templateName), template);
    await fs.unlink(definition);
    makeControl('install');
  } else if (darwin) launch(['bootstrap', domain, definition]);
  else {
    systemctl(['link', '--runtime', definition]);
    systemctl(['daemon-reload']);
  }
  let auth = '';
  let previousPID = '';
  for (let round = 0; round < 2; round++) {
    if (makeMode) { if (round) makeControl('start'); }
    else if (darwin) { if (round) launch(['kickstart', target]); }
    else systemctl(['start', label]);
    await waitFor(healthy, 'native service startup');
    let pid: string | undefined;
    if (darwin) {
      const service = launch(['print', target]);
      assert.match(service, /state = running/);
      pid = service.match(/\bpid = ([1-9][0-9]*)/)?.[1];
    } else {
      assert.equal(property('ActiveState'), 'active');
      if (!makeMode) assert.equal(property('User'), String(uid));
      assert.equal(property('KillMode'), 'mixed');
      pid = property('MainPID');
      assert.match(pid, /^[1-9][0-9]*$/);
    }
    assert(pid); assert.notEqual(pid, previousPID, 'Restart reused the prior process'); previousPID = pid;
    const response = await fetch(base, { signal: AbortSignal.timeout(5000) });
    assert(response.ok); assert.match(await response.text(), /<html/i);
    assert.equal(await fs.readFile(appFile, 'utf8'), app, 'Service rewrote application configuration');
    const current = await fs.readFile(path.join(home, 'config/auth.yaml'), 'utf8');
    assert.match(current, /jwt_secret:/);
    if (round) assert.equal(current, auth, 'Signing secret changed across service restart');
    auth = current;
    const created = await fetch(`${base}/api/sessions`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, signal: AbortSignal.timeout(5000),
      body: JSON.stringify({ hostname: 'localhost', username: 'fixture', transport: 'exec', exec_command: '/bin/sh' }),
    });
    assert(created.ok, `Session creation: ${created.status}`);
    const session: unknown = await created.json();
    assert(session && typeof session === 'object' && 'id' in session && typeof session.id === 'string');
    let output = '';
    let socketError = false;
    socket = new WebSocket(`ws://127.0.0.1:${address.port}/api/term/${session.id}`);
    socket.addEventListener('error', () => { socketError = true; });
    socket.addEventListener('message', event => {
      const value: unknown = JSON.parse(String(event.data));
      if (value && typeof value === 'object' && 'type' in value && value.type === 'output'
        && 'data' in value && typeof value.data === 'string') output += value.data;
    });
    await waitFor(async () => { assert(!socketError); return socket?.readyState === WebSocket.OPEN; }, 'terminal connection');
    // The resulting marker and PID are absent from input, so input echo cannot satisfy the check.
    const marker = `service-${round}-${randomBytes(8).toString('hex')}`;
    socket.send(JSON.stringify({ type: 'input', data: `printf 'child:%s\\n' "$$"; i=0; while [ "$i" -lt 128 ]; do printf '%s:%s\\n' '${marker}' "$i"; i=$((i+1)); done\r` }));
    const expected = Array.from({ length: 128 }, (_, index) => `${marker}:${index}\n`).join('');
    await waitFor(async () => output.replaceAll('\r', '').includes(expected), 'complete terminal output');
    const childPID = Number(output.match(/child:([1-9][0-9]*)/)?.[1]);
    assert(Number.isSafeInteger(childPID) && childPID > 1);
    process.kill(childPID, 0);
    const restarting = makeMode && round === 1;
    if (makeMode) makeControl(restarting ? 'restart' : 'stop');
    else if (darwin) launch(['kill', 'SIGTERM', target]);
    else systemctl(['stop', label]);
    if (restarting) {
      await waitFor(healthy, 'Make restart startup');
      let nextPID: string | undefined;
      if (darwin) {
        const restarted = launch(['print', target]);
        assert.match(restarted, /state = running/);
        nextPID = restarted.match(/\bpid = ([1-9][0-9]*)/)?.[1];
      } else {
        assert.equal(property('ActiveState'), 'active');
        nextPID = property('MainPID'); assert.match(nextPID, /^[1-9][0-9]*$/);
      }
      assert(nextPID && nextPID !== pid, 'Make restart did not replace the backend');
      assert.equal(await fs.readFile(appFile, 'utf8'), app);
      assert.equal(await fs.readFile(path.join(home, 'config/auth.yaml'), 'utf8'), auth);
      await waitFor(async () => {
        const restored = await fetch(`${base}/api/sessions/${session.id}`, { signal: AbortSignal.timeout(1000) });
        const value: unknown = await restored.json();
        return restored.ok && value !== null && typeof value === 'object' && 'id' in value && value.id === session.id
          && 'state' in value && value.state === 'connected';
      }, 'persistent terminal restoration after Make restart');
    } else {
      await waitFor(async () => !await healthy(), 'service listener shutdown');
      await waitFor(async () => {
        if (!darwin) return property('ActiveState') === 'inactive' && property('MainPID') === '0' &&
          property('Result') === 'success' && property('ExecMainStatus') === '0';
        const stopped = launch(['print', target]);
        return /last exit code = 0/.test(stopped) && !/\bpid = [1-9]/.test(stopped) && !/state = running/.test(stopped);
      }, 'graceful native exit');
    }
    await waitFor(async () => socket?.readyState === WebSocket.CLOSED, 'terminal socket shutdown');
    socket = undefined;
    await waitFor(async () => {
      try { process.kill(childPID, 0); return false; }
      catch (error) { assert(error instanceof Error && 'code' in error && error.code === 'ESRCH'); return true; }
    }, 'PTY child exit');
    const directory = path.join(home, 'logs/sessions');
    const files = (await fs.readdir(directory)).filter(name => name.startsWith(`session-${session.id}-`));
    if (!restarting) assert.equal(files.length, 1);
    // Restart restores persistent terminals and opens a new transcript. Check
    // the generation which actually produced this round's acknowledged output.
    const transcripts = await Promise.all(files.map(file => fs.readFile(path.join(directory, file), 'utf8')));
    const matching = transcripts.filter(text => text.replaceAll('\r', '').includes(expected));
    assert.equal(matching.length, 1, 'Expected one transcript containing this generation’s output');
    const transcript = matching[0]!;
    assert(transcript.replaceAll('\r', '').includes(expected), 'Shutdown lost acknowledged terminal output');
    assert.match(transcript, /\[webmux transcript stopped .* reason=shutdown\]\r?\n$/);
  }
  console.log(`Native Unix ${makeMode ? 'Make installer' : 'service template'} passed startup, UI, active PTY shutdown, transcript drain and restart in ${darwin ? domain : makeMode ? 'systemd user manager' : 'systemd (unprivileged service)'}`);
} finally {
  socket?.close();
  if (!registered && definitionCreated) await fs.rm(definition, { force: true });
  if (registered && makeMode) {
    makeControl('uninstall');
    await waitFor(async () => darwin ? launch(['print', target], false) === '' : property('LoadState') === 'not-found', 'Make uninstall registration cleanup');
    await assert.rejects(fs.access(definition), { code: 'ENOENT' });
  }
  if (registered && !darwin && !makeMode) {
    systemctl(['stop', label]);
    systemctl(['disable', '--runtime', label]);
    systemctl(['daemon-reload']);
    assert.equal(property('LoadState'), 'not-found', 'Fixture unit remains registered');
  }
  if (registered && darwin && launch(['print', target], false)) {
    launch(['bootout', target]);
    assert.equal(launch(['print', target], false), '', 'Fixture service remains registered');
  }
  await fs.rm(temporary, { recursive: true, force: true });
}
