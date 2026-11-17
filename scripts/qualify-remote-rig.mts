import fs from 'node:fs';
import path from 'node:path';
import net from 'node:net';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { spawn, spawnSync, execFileSync, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import { randomBytes, createHash } from 'node:crypto';
import { parseArgs } from 'node:util';

const { values } = parseArgs({ options: {
  'vm-root': { type: 'string' }, 'guest-fixture': { type: 'string' },
  'server-binary': { type: 'string' }, 'client-binary': { type: 'string' },
  chromium: { type: 'string' }, output: { type: 'string' }, 'expected-revision': { type: 'string' },
} });
function required(name: keyof typeof values): string { const value = values[name]; assert(value, `Missing --${name}`); return value; }
const source = path.resolve(import.meta.dirname, '..');
const vmRoot = path.resolve(required('vm-root'));
const vm = JSON.parse(fs.readFileSync(path.join(vmRoot, 'launch.json'), 'utf8'));
const guest = JSON.parse(fs.readFileSync(required('guest-fixture'), 'utf8'));
assert(vm.identity && execFileSync('ps', ['-p', String(vm.pid), '-o', 'command='], { encoding: 'utf8' }).includes(vm.identity), 'Owned QEMU identity changed');
assert(guest.contract === 'webmux-remote-rig-fixture/v1', 'Guest must be a disposable remote-Rig fixture');
assert(/^\/home\/[a-zA-Z0-9_-]+\/qualification\/rig-200\/run-[a-f0-9]+$/.test(guest.root));
assert(/^[0-9A-HJKMNP-TV-Z]{26}$/.test(guest.rig_id) && /^wm-remote200-[a-f0-9]+$/.test(guest.tmux));
const expectedRevision = required('expected-revision'); assert(/^[a-f0-9]{40}$/.test(expectedRevision));
const ssh = [...vm.ssh] as string[];
const trustOption = ssh.indexOf('StrictHostKeyChecking=accept-new'); if (trustOption >= 0) ssh[trustOption] = 'StrictHostKeyChecking=yes';
assert(ssh.includes('StrictHostKeyChecking=yes') && ssh.includes('BatchMode=yes'));
const destination = ssh.at(-1)!; assert(/^[a-zA-Z0-9_-]+@127\.0\.0\.1$/.test(destination));
const username = destination.split('@')[0];
const sshPort = Number(ssh[ssh.indexOf('-p') + 1]); assert(Number.isInteger(sshPort) && sshPort > 0 && sshPort < 65536);
const identity = ssh[ssh.indexOf('-i') + 1]; assert(path.isAbsolute(identity));
const trustFile = ssh.find(value => value.startsWith('UserKnownHostsFile='))?.slice('UserKnownHostsFile='.length); assert(trustFile && path.isAbsolute(trustFile));
const q = (v: string) => `'${v.replaceAll("'", "'\\''")}'`;
const run = path.resolve(required('output')); assert(!fs.existsSync(run), 'Output must be a new path');
fs.mkdirSync(run, { mode: 0o700 });
const logs: { stage: string; [key: string]: unknown }[] = [];
const record = (stage: string, detail: Record<string, unknown> = {}) => { const row = { stage, ...detail }; logs.push(row); console.log(JSON.stringify(row)); fs.appendFileSync(path.join(run, 'events.jsonl'), JSON.stringify(row) + '\n'); };
const remote = (code: string, input?: Buffer | string): string => {
  const r = spawnSync(ssh[0], [...ssh.slice(1), code], { input, encoding: 'utf8', timeout: 20000, maxBuffer: 1024 * 1024 });
  assert.equal(r.status, 0, `SSH control: ${r.error?.message ?? r.stderr}`); return r.stdout;
};
async function reserve() { const s = net.createServer(); s.listen(0, '127.0.0.1'); await once(s, 'listening'); const a = s.address(); assert(a && typeof a !== 'string'); const p = a.port; await new Promise<void>((resolve, reject) => s.close(e => e ? reject(e) : resolve())); return p; }
async function until(check: () => Promise<boolean>, label: string, ms = 20000) { const end = Date.now() + ms; do { if (await check()) return; await new Promise(r => setTimeout(r, 100)); } while (Date.now() < end); throw new Error(label); }
async function stop(p: ChildProcess | undefined) { if (!p || p.exitCode !== null || p.signalCode !== null) return; const closed = once(p, 'close'); p.kill('SIGTERM'); const t = setTimeout(() => p.kill('SIGKILL'), 8000); try { await closed; } finally { clearTimeout(t); } }
const require = createRequire(path.join(source, 'webmux/package.json'));
const { chromium } = require('playwright');
let controller: ChildProcess | undefined, tunnel: ChildProcess | undefined, client: ChildProcess | undefined;
let browser: Awaited<ReturnType<typeof chromium.launch>> | undefined;
const home = path.join(run, 'home'); fs.mkdirSync(home, { mode: 0o700 });
fs.mkdirSync(path.join(home, 'config'));
for (const name of fs.readdirSync(path.join(source, 'webmux/config.defaults'))) {
  const p = path.join(source, 'webmux/config.defaults', name); if (!fs.statSync(p).isFile()) continue;
  let value = fs.readFileSync(p, 'utf8'); if (name === 'app.yaml') value = value.replace('enabled_providers: []', 'enabled_providers: [rig]');
  fs.writeFileSync(path.join(home, 'config', name), value, { mode: 0o600 });
}
fs.writeFileSync(path.join(home, 'config/macos-setup.yaml'), 'skipped: true\n');
const bin = path.join(run, 'tools'); fs.mkdirSync(bin);
fs.writeFileSync(path.join(bin, 'node'), `#!/bin/sh\nexec ${q(process.execPath)} "$@"\n`, { mode: 0o700 });
fs.writeFileSync(path.join(bin, 'ssh'), `#!/bin/sh\nexec /usr/bin/ssh -F /dev/null -o ${q('UserKnownHostsFile=' + trustFile)} -o GlobalKnownHostsFile=/dev/null -o IdentityAgent=none "$@"\n`, { mode: 0o700 });
const port = await reserve(), base = `http://127.0.0.1:${port}`;
const serverBinary = path.resolve(required('server-binary')), clientBinary = path.resolve(required('client-binary'));
const hash = (file: string) => createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const clientBuild = execFileSync('go', ['version', '-m', clientBinary], { encoding: 'utf8' });
assert(clientBuild.includes(`vcs.revision=${expectedRevision}`), 'Client build revision differs from requested source');
const serverLog = fs.openSync(path.join(run, 'controller.log'), 'a', 0o600);
const env = { ...process.env, PATH: `${bin}:/usr/bin:/bin:/usr/sbin:/sbin`, WEBMUX_HOME: home, WEBMUX_ROOT: '', HTTP_PORT: String(port), HTTPS_PORT: '0', WEBMUX_COORDINATION_DISABLED: '0' };
let token = '';
async function api(route: string, method = 'GET', body?: unknown): Promise<any> {
  const r = await fetch(base + route, { method, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) }, ...(body === undefined ? {} : { body: JSON.stringify(body) }), signal: AbortSignal.timeout(5000) });
  assert(r.ok, `${method} ${route}: ${r.status} ${await (r.ok ? Promise.resolve('') : r.text())}`); return r.status === 204 ? null : r.json();
}
async function startController() { controller = spawn(serverBinary, ['--listen', `127.0.0.1:${port}`], { cwd: home, env, stdio: ['ignore', serverLog, serverLog] }); await until(async () => { try { return (await fetch(base + '/api/health', { signal: AbortSignal.timeout(500) })).ok; } catch { return false; } }, 'controller startup'); }
const clientLog = fs.openSync(path.join(run, 'client.log'), 'a', 0o600);
const remoteClient = guest.root + '/webmux-client';
const remoteEnvironment = `env HOME=${q(guest.root)} OPENRIG_HOME=${q(guest.root + '/instance')} PATH=${q(guest.root + '/bin:' + path.posix.dirname(guest.node) + ':/usr/bin:/bin')}`;
let forwardPort = 0;
async function startClient() { client = spawn(ssh[0], [...ssh.slice(1), `${remoteEnvironment} ${q(remoteClient)} --server ${q(`http://127.0.0.1:${forwardPort}`)} --state ${q(guest.root + '/client.json')} --allow-insecure-http --observe-rig`], { stdio: ['ignore', clientLog, clientLog] }); }
async function stopClient() {
  const code = `import pathlib,os,signal,select,json
target=${JSON.stringify(remoteClient)}
held=[]
for p in pathlib.Path('/proc').glob('[0-9]*/cmdline'):
 try:
  fd=os.pidfd_open(int(p.parent.name))
  if p.read_bytes().split(b'\\0')[0].decode(errors='replace')!=target:os.close(fd);continue
  signal.pidfd_send_signal(fd,signal.SIGTERM);held.append(fd)
 except (OSError,ProcessLookupError):pass
for fd in held:
 assert select.select([fd],[],[],10)[0], 'owned client did not exit'
 os.close(fd)
print(json.dumps({'stopped_clients':len(held)}))
`;
  const result = JSON.parse(remote(`python3 -c ${q(code)}`));
  record('client-stopped', result);
  await stop(client); client = undefined;
}
async function seats() { return (await api('/api/coordination')).seats.filter((s: any) => s.provider === 'rig'); }
const nativeSeat = () => remote(`tmux list-panes -t ${q(guest.tmux)} -F '#{pane_pid}'`).trim();
let remotePID = '';
let finished = false;
try {
  await startController();
  assert.equal(remote('hostname').trim(), 'webmux-rdp-qa', 'Unexpected qualification guest');
  const bootstrap = await api('/api/auth/bootstrap', 'POST', { username: 'qualification', password: randomBytes(24).toString('hex') }); token = bootstrap.token; assert(token);
  const producer = await api('/api/server-info');
  assert.equal(producer.build.revision, expectedRevision); assert.equal(producer.build.modified, false);
  record('payload-identity', { serverSHA256: hash(serverBinary), clientSHA256: hash(clientBinary), producer: producer.build, clientBuild });
  const key = await api('/api/keys', 'POST', { private_key_path: identity, type: 'ed25519', description: 'Owned disposable guest' });
  const host = await api('/api/hosts', 'POST', { hostname: '127.0.0.1', username, port: sshPort, key_id: key.id });
  const registered = await api('/api/clients/enroll', 'POST', { label: 'Owned Linux qualification guest' });
  await api(`/api/hosts/${host.id}/client`, 'POST', { pair_code: registered.pair_code });
  remote(`python3 -c ${q(`import pathlib,sys; p=pathlib.Path(${JSON.stringify(remoteClient)}); p.write_bytes(sys.stdin.buffer.read()); p.chmod(0o700)`)}`, fs.readFileSync(clientBinary));
  assert.equal(remote(`sha256sum ${q(remoteClient)}`).trim().split(/\s+/)[0], hash(clientBinary), 'Remote client payload digest changed');
  forwardPort = Number(remote(`python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()'`).trim());
  const state = JSON.stringify({ server: `http://127.0.0.1:${forwardPort}`, id: registered.id, credential: registered.credential });
  remote(`python3 -c ${q(`import pathlib,sys; p=pathlib.Path(${JSON.stringify(guest.root + '/client.json')}); p.write_text(sys.stdin.read()); p.chmod(0o600)`)}`, state);
  tunnel = spawn(ssh[0], [...ssh.slice(1), '-N', '-o', 'ExitOnForwardFailure=yes', '-R', `${forwardPort}:127.0.0.1:${port}`], { stdio: ['ignore', 'ignore', 'pipe'] });
  await new Promise(r => setTimeout(r, 200)); assert.equal(tunnel.exitCode, null);
  await startClient();
  await until(async () => (await seats()).some((s: any) => s.openable && s.observationState === 'current'), 'authenticated host Rig check-in');
  const seat = (await seats())[0]; assert.equal(seat.id, `host-rig/${registered.id}/` + createHash('sha256').update(guest.rig_id + '\0agents.worker').digest('hex'));
  remotePID = nativeSeat(); assert(/^[1-9][0-9]*$/.test(remotePID)); record('check-in', { seat: seat.id, remotePanePID: remotePID, rig: guest.rig_id, rigVersion: guest.version });
  browser = await chromium.launch({ executablePath: required('chromium'), headless: true });
  record('environment', { controllerOS: process.platform, controllerArch: process.arch, node: process.version, chromium: await browser.version(), guest: remote('uname -srm').trim(), guestNode: remote(`${q(guest.node)} --version`).trim(), tmux: remote('tmux -V').trim(), rigVersion: remote(`${remoteEnvironment} rig --version`).trim() });
  const context = await browser.newContext(); await context.addInitScript((t: string) => localStorage.setItem('webmux_token', t), token);
  const page = await context.newPage(); await page.goto(base);
  await page.getByRole('button', { name: 'Coordination', exact: true }).click();
  const worker = page.getByRole('article', { name: 'agents.worker in remote200' }); await worker.waitFor();
  await worker.getByRole('button', { name: 'Open session', exact: true }).click();
  await until(async () => (await api('/api/sessions')).some((s: any) => s.rig?.target === seat.id), 'remote grid viewer creation');
  const session = (await api('/api/sessions')).find((s: any) => s.rig?.target === seat.id); assert(session);
  await page.getByRole('button', { name: 'Terminals', exact: true }).click();
  const tile = page.getByTestId(`tile-cell-${session.id}`); await tile.waitFor();
  await until(async () => (await tile.locator('.xterm-rows').textContent()).includes('REMOTE-SEAT-READY'), 'real SSH baseline');
  const nonce = 'native200-' + randomBytes(12).toString('hex');
  await tile.locator('.xterm-helper-textarea').focus(); await page.keyboard.insertText(nonce); await page.keyboard.press('Enter');
  await until(async () => (await tile.locator('.xterm-rows').textContent()).includes('REMOTE-SEAT:' + nonce), 'real SSH keyboard response');
  record('real-ssh-keyboard', { session: session.id, row: session.row, col: session.col, remotePanePID: nativeSeat() });
  const second = await context.newPage(); await second.goto(base);
  await second.getByRole('button', { name: 'Coordination', exact: true }).click(); await second.getByRole('article', { name: 'agents.worker in remote200' }).getByRole('button', { name: 'Open session' }).click();
  const sessions = (await api('/api/sessions')).filter((s: any) => s.rig?.target === seat.id); assert.equal(sessions.length, 1); assert.equal(sessions[0].id, session.id); assert.equal(nativeSeat(), remotePID);
  await until(async () => (await second.getByTestId(`tile-cell-${session.id}`).locator('.xterm-rows').textContent()).includes('REMOTE-SEAT:' + nonce), 'second window sees the actual SSH response');
  record('two-windows-one-viewer', { session: session.id, remotePanePID: nativeSeat() });
  const beforeLoss = (await seats()).find((s: any) => s.id === seat.id).state;
  const canonical = JSON.parse(remote(`${remoteEnvironment} rig ps --nodes -A --no-cleanup --json --fields canonicalSessionName,lifecycleState`)).entries.find((s: any) => s.canonicalSessionName === guest.tmux);
  assert.equal(beforeLoss, canonical.lifecycleState);
  record('canonical-before-client-loss', { lifecycle: beforeLoss });
  await stopClient();
  await until(async () => (await seats()).some((s: any) => s.id === seat.id && s.observationState === 'offline' && !s.openable && s.state === beforeLoss), 'offline is distinct from stopped');
  await second.reload(); await second.getByRole('button', { name: 'Coordination', exact: true }).click();
  const offline = second.getByRole('article', { name: 'agents.worker in remote200' }); await offline.getByText('Last observed Rig lifecycle').waitFor(); assert((await offline.textContent()).includes('offline')); assert.equal(await offline.getByRole('button', { name: 'Open session' }).count(), 0); assert.equal(nativeSeat(), remotePID);
  record('offline-preserves-canonical-lifecycle', { remotePanePID: nativeSeat() });
  await startClient(); await until(async () => (await seats()).some((s: any) => s.id === seat.id && s.openable), 'client reconnect stable identity');
  await stop(controller); controller = undefined; await startController();
  await until(async () => (await seats()).some((s: any) => s.id === seat.id && s.openable), 'controller restart host check-in', 40000);
  await page.reload(); await page.getByRole('button', { name: 'Coordination', exact: true }).click(); await page.getByRole('article', { name: 'agents.worker in remote200' }).getByRole('button', { name: 'Open session' }).click();
  const restored = (await api('/api/sessions')).filter((s: any) => s.rig?.target === seat.id); assert.equal(restored.length, 1); assert.equal(restored[0].id, session.id); assert.equal(restored[0].row, session.row); assert.equal(restored[0].col, session.col); assert.equal(nativeSeat(), remotePID);
  const fresh = 'resumed200-' + randomBytes(12).toString('hex');
  const restoredTile = page.getByTestId(`tile-cell-${session.id}`);
  await until(async () => (await restoredTile.locator('.xterm-rows').textContent()).includes('REMOTE-SEAT-READY'), 'restored SSH baseline');
  await restoredTile.locator('.xterm-helper-textarea').focus(); await page.keyboard.insertText(fresh); await page.keyboard.press('Enter');
  await until(async () => (await restoredTile.locator('.xterm-rows').textContent()).includes('REMOTE-SEAT:' + fresh), 'fresh SSH response after controller restart');
  record('controller-restart-retains-seat-and-layout', { session: session.id, remotePanePID: nativeSeat() });
  await api(`/api/clients/${registered.id}`, 'DELETE'); await until(async () => (await seats()).length === 0, 'revoked host observation removed'); assert.equal(nativeSeat(), remotePID);
  const denied = await fetch(base + '/api/coordination/attach', { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` }, body: JSON.stringify({ provider: 'rig', target: seat.id }), signal: AbortSignal.timeout(5000) });
  assert([409, 502].includes(denied.status), 'Revoked host still permits a new attachment');
  record('revocation-preserves-provider', { remotePanePID: nativeSeat() });
  await api('/api/config', 'PUT', { app: { coordination: { enabled_providers: [] } } });
  const ordinary = await api('/api/sessions', 'POST', { hostname: '127.0.0.1', username, port: sshPort, key_id: key.id, transport: 'ssh' });
  await page.reload();
  await page.getByRole('button', { name: 'Terminals', exact: true }).click();
  const ordinaryTile = page.getByTestId(`tile-cell-${ordinary.id}`);
  await until(async () => (await ordinaryTile.locator('.xterm-rows').textContent()).includes('@webmux-rdp-qa'), 'ordinary SSH prompt without Rig');
  const ordinaryNonce = randomBytes(12).toString('hex');
  await ordinaryTile.locator('.xterm-helper-textarea').focus();
  await page.keyboard.insertText(`printf '%s%s\\n' 'ORDINARY-' '${ordinaryNonce}'`); await page.keyboard.press('Enter');
  await until(async () => (await ordinaryTile.locator('.xterm-rows').textContent()).includes('ORDINARY-' + ordinaryNonce), 'ordinary SSH keyboard without Rig');
  await api(`/api/sessions/${ordinary.id}`, 'DELETE'); assert.equal(nativeSeat(), remotePID);
  record('ordinary-ssh-independent-of-rig', { remotePanePID: nativeSeat() });
  finished = true;
} catch (error) {
  try { record('viewer-diagnostics', { seats: await seats(), sessions: (await api('/api/sessions')).map((s: any) => ({ id: s.id, state: s.state, rig: s.rig })), pages: await Promise.all(browser ? browser.contexts().flatMap((c: any) => c.pages()).map(async (p: any) => ({ text: await p.locator('.xterm-rows').allTextContents() })) : []) }); } catch {}
  record('failure', { error: String(error) }); throw error; }
finally {
  await browser?.close(); await stopClient(); await stop(tunnel); await stop(controller); fs.closeSync(serverLog); fs.closeSync(clientLog);
  const receipt = { passed: finished, harnessSHA256: hash(import.meta.filename), harnessRevision: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: source, encoding: 'utf8' }).trim(), expectedRevision, serverSHA256: hash(serverBinary), clientSHA256: hash(clientBinary), vmIdentity: vm.identity, rigVersion: guest.version, remotePanePID: remotePID, logs };
  fs.writeFileSync(path.join(run, 'receipt.json'), JSON.stringify(receipt, null, 2) + '\n'); console.log('RECEIPT ' + path.join(run, 'receipt.json'));
}
