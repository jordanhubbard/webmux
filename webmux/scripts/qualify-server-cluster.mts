// Opt-in native two-host qualification. Uses only private homes, loopback
// listeners and SSH forwards; never restarts either host's installed service.
import { spawn, spawnSync, type ChildProcess } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import path from 'node:path';
import { randomBytes, createHash } from 'node:crypto';
import { chromium, expect } from '@playwright/test';
import argon2 from 'argon2';
import yaml from 'js-yaml';

const root = path.resolve(import.meta.dirname, '../..');
const peer = process.argv[2];
if (!peer || !/^[a-zA-Z0-9][a-zA-Z0-9._-]*$/.test(peer)) throw new Error('Usage: npm run qualify:cluster -- SSH_HOST');
fs.mkdirSync(path.join(root, '_build'), { recursive: true });
const output = fs.mkdtempSync(path.join(root, '_build/server-cluster-'));
fs.chmodSync(output, 0o700);
const sshArgs = ['-o', 'BatchMode=yes', '-o', 'ConnectTimeout=10'];
const quote = (s: string) => `'${s.replaceAll("'", "'\\''")}'`;
function run(program: string, args: string[], input?: string, cwd = root): string {
  const result = spawnSync(program, args, { cwd, input, encoding: 'utf8', timeout: 180_000, maxBuffer: 4 * 1024 * 1024 });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${program} failed (${result.status}): ${result.stderr.slice(-4000)}`);
  return result.stdout.trim();
}
const remote = (command: string, input?: string) => run('ssh', [...sshArgs, peer, command], input);
const freePort = async () => {
  const listener = net.createServer();
  await new Promise<void>(resolve => listener.listen(0, '127.0.0.1', resolve));
  const port = (listener.address() as net.AddressInfo).port;
  await new Promise<void>((resolve, reject) => listener.close(error => error ? reject(error) : resolve()));
  return port;
};
function child(program: string, args: string[], log: string, env = process.env): ChildProcess {
  const fd = fs.openSync(path.join(output, log), 'a', 0o600);
  try { return spawn(program, args, { env, stdio: ['ignore', fd, fd] }); }
  finally { fs.closeSync(fd); }
}
async function stop(process: ChildProcess | undefined) {
  if (!process || process.exitCode !== null || process.signalCode !== null) return;
  const exited = new Promise<void>(resolve => process.once('exit', () => resolve()));
  process.kill('SIGTERM');
  const timer = setTimeout(() => process.kill('SIGKILL'), 5000);
  try { await exited; } finally { clearTimeout(timer); }
}

const checks: string[] = [];
const binaryHashes: Record<string, string> = {};
const sourceRevision = run('git', ['rev-parse', 'HEAD']);
const sourceDirty = !!run('git', ['status', '--porcelain']);
const passed = (name: string) => { checks.push(name); console.log(`PASS ${name}`); };
let remoteRoot = '';
let localProcess: ChildProcess | undefined, remoteProcess: ChildProcess | undefined, tunnel: ChildProcess | undefined;
let browser: Awaited<ReturnType<typeof chromium.launch>> | undefined;
let canceled = false;
for (const signal of ['SIGINT', 'SIGTERM'] as const) process.on(signal, () => { canceled = true; void browser?.close(); });
const checkCancellation = () => { if (canceled) throw new Error('Qualification canceled'); };
try {
  console.log(`Evidence: ${output}`);
  const platform = remote('uname -sm');
  const architectures: Record<string, string> = { 'Linux aarch64': 'arm64', 'Linux x86_64': 'amd64' };
  const arch = architectures[platform];
  if (!arch) throw new Error(`Unsupported native peer: ${platform}`);
  console.log(`Building controller and ${platform} peer from this checkout`);
  run('npm', ['run', 'build', '--workspace=frontend'], undefined, path.join(root, 'webmux'));
  run('go', ['run', './cmd/build', '-o', path.join(output, 'controller')], undefined, path.join(root, 'webmux/server'));
  run('go', ['run', './cmd/build', '-goos', 'linux', '-goarch', arch, '-o', path.join(output, 'peer')], undefined, path.join(root, 'webmux/server'));
  checkCancellation();
  remoteRoot = remote('mktemp -d /tmp/wm-cluster-XXXXXXXX');
  if (!/^\/tmp\/wm-cluster-[a-zA-Z0-9]+$/.test(remoteRoot)) throw new Error('Unexpected remote evidence path');
  run('scp', ['-o', 'BatchMode=yes', path.join(output, 'peer'), `${peer}:${remoteRoot}/webmux`]);
  for (const name of ['controller', 'peer']) binaryHashes[name] = createHash('sha256').update(fs.readFileSync(path.join(output, name))).digest('hex');
  const receivedHash = remote('python3 -c '+quote(`import hashlib,pathlib;print(hashlib.sha256(pathlib.Path(${JSON.stringify(remoteRoot+'/webmux')}).read_bytes()).hexdigest())`));
  expect(receivedHash).toBe(binaryHashes.peer);
  const [localPort, forwardPort] = await Promise.all([freePort(), freePort()]);
  const remotePorts = JSON.parse(remote('python3 -c '+quote('import socket,json\ns=[]\nfor _ in range(2):\n p=socket.socket();p.bind(("127.0.0.1",0));s.append(p)\nprint(json.dumps([p.getsockname()[1] for p in s]))'))) as number[];
  const [remotePort, reversePort] = remotePorts;
  const localURL = `http://127.0.0.1:${localPort}`, peerURL = `http://127.0.0.1:${forwardPort}`;
  const toPeer = randomBytes(32).toString('hex'), toLocal = randomBytes(32).toString('hex');
  const password = randomBytes(24).toString('hex');
  // Match the server's canonical PHC parameter order; recent node-argon2
  // emits the same costs as m,p,t rather than the server's m,t,p.
  const hash = (await argon2.hash(password, { type: argon2.argon2id }))
    .replace(/m=(\d+),p=(\d+),t=(\d+)/, 'm=$1,t=$3,p=$2');
  const auth = yaml.dump({ auth: { mode: 'local', users: [{ username: 'qualifier', password_hash: hash, admin: true }, { username: 'other', password_hash: hash }] } });
  const defaults = fs.readdirSync(path.join(root, 'webmux/config.defaults'), { withFileTypes: true }).filter(e => e.isFile());
  const baseFiles = Object.fromEntries(defaults.map(e => [e.name, fs.readFileSync(path.join(root, 'webmux/config.defaults', e.name), 'utf8')]));
  function files(name: string, id: string, url: string) {
    const app = yaml.load(baseFiles['app.yaml']) as { app: Record<string, unknown> };
    app.app.name = name; app.app.listen_host = '127.0.0.1';
    return { ...baseFiles, 'auth.yaml': auth, 'app.yaml': yaml.dump(app), 'macos-setup.yaml': 'skipped: true\n', 'servers.yaml': yaml.dump({ version: 1, servers: [{ id, name: id === 'peer' ? 'Peer' : 'Controller', url, allow_http: true, token_env: 'CLUSTER_OUT' }], grants: [{ id: 'incoming', owner: 'qualifier', token_env: 'CLUSTER_IN' }] }) };
  }
  const fixture = `import os,socket,sys,signal,re\ndef size(*_):\n s=os.get_terminal_size();print("SIZE:%dx%d"%(s.columns,s.lines),flush=True)\nsignal.signal(signal.SIGWINCH,size)\nprint("HOST:"+socket.gethostname()+" PID:"+str(os.getpid()),flush=True)\nfor line in sys.stdin:\n probe=re.search(r"IDENTITY-PROBE:([a-f0-9]+)",line)\n if probe: print("IDENTITY:"+probe.group(1)+" PID:"+str(os.getpid()),flush=True)\n else: print("REPLY:"+line.strip(),flush=True)\n`;
  const localHome = path.join(output, 'home');
  for (const dir of ['config', 'tmux']) fs.mkdirSync(path.join(localHome, dir), { recursive: true, mode: 0o700 });
  for (const [name, content] of Object.entries(files('Controller', 'peer', peerURL))) fs.writeFileSync(path.join(localHome, 'config', name), content, { mode: 0o600 });
  fs.writeFileSync(path.join(localHome, 'echo.py'), fixture);
  const localEnv = { ...process.env, WEBMUX_HOME: localHome, WEBMUX_ROOT: '', HTTP_PORT: String(localPort), CLUSTER_IN: toLocal, CLUSTER_OUT: toPeer, TMUX_TMPDIR: path.join(localHome, 'tmux'), WEBMUX_COORDINATION_DISABLED: '1' };
  const remotePayload = { home: `${remoteRoot}/home`, executable: `${remoteRoot}/webmux`, files: files('Peer', 'controller', `http://127.0.0.1:${reversePort}`), fixture, env: { WEBMUX_ROOT: '', HTTP_PORT: String(remotePort), CLUSTER_IN: toPeer, CLUSTER_OUT: toLocal, WEBMUX_COORDINATION_DISABLED: '1' } };
  const launcher = `import os,sys,json,pathlib,subprocess\np=json.load(sys.stdin);h=pathlib.Path(p["home"]);(h/"config").mkdir(parents=True,exist_ok=True);(h/"tmux").mkdir(mode=0o700,exist_ok=True)\nfor n,v in p["files"].items():\n f=h/"config"/n;f.write_text(v);f.chmod(0o600)\n(h/"echo.py").write_text(p["fixture"])\nenv=os.environ|p["env"]|{"WEBMUX_HOME":str(h),"TMUX_TMPDIR":str(h/"tmux")}\nwith open(h.parent/"server.log","ab") as log:\n child=subprocess.Popen([p["executable"]],env=env,stdout=log,stderr=log);(h.parent/"pid").write_text(str(child.pid));sys.exit(child.wait())`;
  const launchPeer = () => {
    const p = spawn('ssh', [...sshArgs, peer, 'python3 -c '+quote(launcher)], { stdio: ['pipe', 'ignore', 'pipe'] });
    p.stdin.end(JSON.stringify(remotePayload));
    p.stderr.on('data', () => {});
    return p;
  };
  const stopPeer = async () => {
    remote('python3 -c '+quote(`import os,signal,pathlib\np=pathlib.Path(${JSON.stringify(remoteRoot)})/"pid"\ntry: os.kill(int(p.read_text()),signal.SIGTERM)\nexcept ProcessLookupError: pass`));
    await stop(remoteProcess);
  };
  localProcess = child(path.join(output, 'controller'), [], 'controller.log', localEnv);
  remoteProcess = launchPeer();
  tunnel = child('ssh', [...sshArgs, '-o', 'ExitOnForwardFailure=yes', '-N', '-L', `127.0.0.1:${forwardPort}:127.0.0.1:${remotePort}`, '-R', `127.0.0.1:${reversePort}:127.0.0.1:${localPort}`, peer], 'tunnel.log');
  for (const url of [localURL, peerURL]) await expect.poll(async () => {
    checkCancellation();
    try { return (await fetch(url+'/api/health', { signal: AbortSignal.timeout(2000) })).status; } catch { return 0; }
  }, { timeout: 20_000 }).toBe(200);
  passed('independent native servers reachable across SSH forwards');
  async function login(url: string, username = 'qualifier') {
    const response = await fetch(url+'/api/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password }) });
    expect(response.status).toBe(200);
    return (await response.json() as { token: string }).token;
  }
  const localToken = await login(localURL), peerToken = await login(peerURL);
  async function api(url: string, token: string, endpoint: string, method = 'GET', body?: unknown) {
    return fetch(url+'/api'+endpoint, { method, headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(15_000) });
  }
  for (const [url, token, id] of [[localURL, localToken, 'peer'], [peerURL, peerToken, 'controller']]) {
    expect((await api(url, token, `/servers/${id}/sessions`)).status).toBe(200);
    expect((await fetch(url+'/api/federation/sessions')).status).toBe(401);
    expect((await api(url, 'invalid-credential-'+randomBytes(32).toString('hex'), '/federation/sessions')).status).toBe(401);
  }
  expect((await api(peerURL, toPeer, '/config')).status).toBe(401);
  expect((await api(localURL, toLocal, '/servers')).status).toBe(401);
  passed('bidirectional discovery, missing/bad credentials and restricted grant scope');
  const sessions: { id: string; pid?: string }[] = [];
  for (const [url, token, home] of [[localURL, localToken, localHome], [peerURL, peerToken, `${remoteRoot}/home`]]) {
    const response = await api(url, token, '/sessions', 'POST', { transport: 'local', initial_cmd: `python3 -u ${quote(home+'/echo.py')}` });
    expect(response.status).toBe(201);
    sessions.push(await response.json() as { id: string });
  }
  const foreignToken = await login(peerURL, 'other');
  const foreign = await api(peerURL, foreignToken, '/sessions', 'POST', { transport: 'local' });
  expect(foreign.status).toBe(201);
  const foreignSession = await foreign.json() as { id: string };
  expect((await api(peerURL, foreignToken, '/servers')).status).toBe(403);
  expect((await api(localURL, localToken, `/servers/peer/sessions/${foreignSession.id}/messages`, 'POST', { message: 'must not arrive' })).status).toBe(404);
  const discovered = await (await api(localURL, localToken, '/servers/peer/sessions')).json() as { id: string }[];
  expect(discovered.map(s => s.id)).toEqual([sessions[1].id]);
  passed('remote inventory excludes another owner’s terminal');
  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH } : {}) });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  await context.addInitScript(({ localURL, localToken, peerToken }) => {
    localStorage.setItem('webmux_token', location.origin === localURL ? localToken : peerToken);
  }, { localURL, localToken, peerToken });
  const page = await context.newPage();
  await page.goto(localURL);
  const selector = page.getByRole('combobox', { name: 'Server view' });
  await expect(selector).toHaveValue('local');
  await expect(page.getByTestId(`tile-cell-${sessions[0].id}`).locator('.xterm-rows')).toContainText('HOST:');
  await expect(page.locator('[data-testid^="server-terminal-"]')).toHaveCount(0);
  await selector.selectOption('peer');
  const peerTile = page.getByTestId(`server-terminal-peer-${sessions[1].id}`);
  await expect(peerTile.locator('.xterm-rows')).toContainText('HOST:');
  await expect(page.locator('[data-testid^="server-terminal-"]')).toHaveCount(1);
  sessions[1].pid = (await peerTile.locator('.xterm-rows').innerText()).match(/PID:(\d+)/)![1];
  await selector.selectOption('all');
  const localTile = page.getByTestId(`server-terminal-local-${sessions[0].id}`);
  await expect(localTile.locator('.xterm-rows')).toContainText('HOST:');
  await expect(page.locator('[aria-label="Server terminal matrix"] > .server-terminal')).toHaveCount(2);
  await expect(peerTile.locator('.server-badge')).toHaveText('Peer · peer');
  await expect(localTile.locator('.server-badge')).toHaveText('Controller · this server');
  expect((await peerTile.boundingBox())!.y).toBe((await localTile.boundingBox())!.y);
  await page.screenshot({ path: path.join(output, 'all-servers.png'), fullPage: true });
  passed('all three display modes and identified host tiles in one 2D matrix');
  const nonce = randomBytes(12).toString('hex');
  async function verifyPeerIdentity() {
    await expect.poll(async () => {
      const values = await (await api(localURL, localToken, '/servers/peer/sessions')).json() as { id: string; state: string }[];
      return values.find(s => s.id === sessions[1].id)?.state;
    }, { timeout: 20_000 }).toBe('connected');
    const probe = randomBytes(8).toString('hex');
    expect((await api(localURL, localToken, `/servers/peer/sessions/${sessions[1].id}/messages`, 'POST', { message: 'IDENTITY-PROBE:'+probe })).status).toBe(202);
    await expect(peerTile.locator('.xterm-rows')).toContainText(`IDENTITY:${probe} PID:${sessions[1].pid}`, { timeout: 15_000 });
  }
  await peerTile.locator('.xterm-helper-textarea').focus();
  await page.keyboard.type('input-'+nonce); await page.keyboard.press('Enter');
  await expect(peerTile.locator('.xterm-rows')).toContainText('REPLY:input-'+nonce);
  await expect(localTile.locator('.xterm-rows')).not.toContainText(nonce);
  await peerTile.getByRole('button', { name: 'Message', exact: true }).click();
  await page.getByRole('textbox', { name: 'Message' }).fill('message-'+nonce);
  await page.getByRole('button', { name: 'Send to this session' }).click();
  await expect(page.getByRole('status')).toContainText('processing is not confirmed');
  await page.keyboard.press('Escape');
  await expect(peerTile.locator('.xterm-rows')).toContainText('message-'+nonce);
  const reverse = await api(peerURL, peerToken, `/servers/controller/sessions/${sessions[0].id}/messages`, 'POST', { message: 'reverse-'+nonce });
  expect(reverse.status).toBe(202);
  expect(await reverse.json()).toMatchObject({ queued: true, processed: false });
  await expect(localTile.locator('.xterm-rows')).toContainText('reverse-'+nonce);
  await expect(peerTile.locator('.xterm-rows')).not.toContainText('reverse-'+nonce);
  passed('cross-host terminal input and bidirectional targeted messages without cross-talk');
  await page.setViewportSize({ width: 1000, height: 800 });
  await expect(peerTile.locator('.xterm-rows')).toContainText('SIZE:');
  await peerTile.getByRole('button', { name: 'Reconnect viewer' }).click();
  await verifyPeerIdentity();
  await peerTile.getByRole('button', { name: 'Hide', exact: true }).click();
  await expect(peerTile).toHaveCount(0);
  await page.getByRole('button', { name: 'Show hidden viewers' }).click();
  await verifyPeerIdentity();
  await selector.selectOption('peer'); await page.reload();
  await expect(selector).toHaveValue('peer');
  await verifyPeerIdentity();
  passed('remote resize, hide/show, viewer reattach and persisted selection preserve provider PID');
  const peerPage = await context.newPage(); await peerPage.goto(peerURL);
  await peerPage.getByRole('combobox', { name: 'Server view' }).selectOption('controller');
  await expect(peerPage.getByTestId(`server-terminal-controller-${sessions[0].id}`).locator('.xterm-rows')).toContainText('reverse-'+nonce);
  await peerPage.close();
  passed('peer browser displays controller-owned terminal through reverse federation');
  await selector.selectOption('all');
  await page.getByRole('button', { name: 'New terminal on Peer' }).click();
  await expect(page.locator('[data-server-id="peer"]')).toHaveCount(2);
  const expanded = await (await api(localURL, localToken, '/servers/peer/sessions')).json() as { id: string }[];
  const extra = expanded.find(s => s.id !== sessions[1].id)!;
  expect(extra).toBeTruthy();
  expect((await api(peerURL, peerToken, `/sessions/${extra.id}`, 'DELETE')).status).toBe(204);
  await expect(page.locator('[data-server-id="peer"]')).toHaveCount(1, { timeout: 10_000 });
  await verifyPeerIdentity();
  passed('new terminal creation targets the selected remote host without replacing existing providers');
  const writePeerDirectory = (text: string) => remote('python3 -c '+quote(`import pathlib,sys;pathlib.Path(${JSON.stringify(remoteRoot+'/home/config/servers.yaml')}).write_text(sys.stdin.read())`), text);
  const directory = yaml.load(remotePayload.files['servers.yaml']) as { grants: unknown[] };
  writePeerDirectory(yaml.dump({ ...directory, grants: [] }));
  await expect(peerTile.getByText('Viewer: disconnected', { exact: true })).toBeVisible({ timeout: 12_000 });
  expect((await api(localURL, localToken, '/servers/peer/sessions')).status).toBe(502);
  expect((await (await api(peerURL, peerToken, '/sessions')).json() as unknown[]).length).toBe(1);
  await expect(localTile.getByText('Viewer: connected', { exact: true })).toBeVisible();
  writePeerDirectory(remotePayload.files['servers.yaml']);
  await peerTile.getByRole('button', { name: 'Reconnect viewer' }).click();
  await verifyPeerIdentity();
  passed('live grant revocation closes remote viewers, preserves providers and recovers after restoration');
  await stopPeer();
  await expect(page.getByRole('region', { name: 'Peer sessions', exact: true }).getByRole('alert')).toContainText('Unavailable', { timeout: 20_000 });
  await expect(peerTile.getByText('Viewer: disconnected', { exact: true })).toBeVisible();
  await expect(localTile.locator('.xterm-rows')).toContainText('reverse-'+nonce);
  remoteProcess = launchPeer();
  await expect(peerTile.getByText('Viewer: connected', { exact: true })).toBeVisible({ timeout: 30_000 });
  await verifyPeerIdentity();
  await expect(page.getByRole('region', { name: 'Peer sessions', exact: true }).getByRole('alert')).toHaveCount(0, { timeout: 20_000 });
  const inventory = await (await api(localURL, localToken, '/servers/peer/sessions')).json() as { id: string }[];
  expect(inventory.map(s => s.id)).toEqual([sessions[1].id]);
  passed('peer outage isolation and native service restart recover the same tmux provider');
  await page.setViewportSize({ width: 1440, height: 1000 });
  await expect(peerTile.getByText('Viewer: connected', { exact: true })).toBeVisible();
  await page.screenshot({ path: path.join(output, 'recovered.png'), fullPage: true });
  checkCancellation();
} catch (error) {
  fs.writeFileSync(path.join(output, 'failure.txt'), String(error));
  process.exitCode = 1;
  console.error(String(error));
} finally {
  await browser?.close();
  await stop(localProcess);
  if (remoteRoot) {
    try {
      const cleanup = `import pathlib,os,signal,subprocess\np=pathlib.Path(${JSON.stringify(remoteRoot)})\ntry: os.kill(int((p/"pid").read_text()),signal.SIGTERM)\nexcept (ProcessLookupError,FileNotFoundError): pass\nfor s in p.glob("home/tmux/tmux-*/default"):\n subprocess.run(["tmux","-S",str(s),"kill-server"],timeout=5,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)\n`;
      remote('python3 -c '+quote(cleanup));
      run('scp', ['-o', 'BatchMode=yes', `${peer}:${remoteRoot}/server.log`, path.join(output, 'peer.log')]);
    } catch (error) { console.error('Peer cleanup failed:', String(error)); process.exitCode = 1; }
  }
  await stop(remoteProcess); await stop(tunnel);
  const tmuxRoot = path.join(output, 'home/tmux');
  if (fs.existsSync(tmuxRoot)) for (const dir of fs.readdirSync(tmuxRoot)) {
    const socket = path.join(tmuxRoot, dir, 'default');
    if (fs.existsSync(socket)) run('tmux', ['-S', socket, 'kill-server']);
  }
  fs.writeFileSync(path.join(output, 'report.json'), JSON.stringify({ passed: !process.exitCode && !canceled, revision: sourceRevision, dirty: sourceDirty, binaryHashes, checks, controllerPlatform: process.platform, peerPlatform: 'linux', completedAt: new Date().toISOString() }, null, 2));
  console.log(`Report: ${path.join(output, 'report.json')}`);
}
