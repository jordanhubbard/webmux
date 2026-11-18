import { test, expect } from '@playwright/test';
import { spawn, spawnSync, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import { createServer, createConnection } from 'node:net';
import http from 'node:http';
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { userInfo } from 'node:os';
import path from 'node:path';

const quote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;

test('real Mosh carries an automatic browser request without logging and acknowledges it across reconnect', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_MOSH !== '1' || process.platform === 'win32', 'Requires native Mosh and an owned SSH fixture');
  test.setTimeout(120_000);
  const testHome = path.resolve(__dirname, '.test-home');
  const root = mkdtempSync(path.join(testHome, 'mosh-owned-'));
  for (const dir of ['home', 'bin', 'tmux', 'cache']) mkdirSync(path.join(root, dir), { mode: 0o700 });
  const lookup = (name: string) => {
    const result = spawnSync('/usr/bin/which', [name], { encoding: 'utf8', timeout: 5000 });
    expect(result.status, result.stderr).toBe(0); return result.stdout.trim();
  };
  const tmux = lookup('tmux'), moshServer = lookup('mosh-server');
  let daemon: ChildProcess | undefined, id: string | undefined, serverPID: number | undefined;
  let startupError: Error | undefined;
  let diagnostics = '', callbackURL = '', identityURL = '', outputTail = '', printedIdentityURL = false;
  const keyID = 'mosh-fixture';
  const receipt = { nativeMosh: false, loggingDisabled: false, noLaunchURLInTerminalOutput: false, automaticPanel: false, callbackOnOriginalHost: false, callbackViaSSH: false, acknowledgedNotReplayed: false, providerStopped: false, moshServerExited: false, sshDaemonExited: false };
  page.on('websocket', socket => {
    if (!id || !socket.url().includes(`/api/term/${id}`)) return;
    socket.on('framereceived', frame => {
      const message = JSON.parse(frame.payload.toString()) as { type: string; data?: string };
      if (message.type !== 'output' || !message.data) return;
      outputTail = (outputTail + message.data).slice(-32768);
      if (identityURL && outputTail.includes(identityURL)) printedIdentityURL = true;
    });
  });
  const identity = http.createServer((req, res) => {
    const target = new URL(req.url!, 'http://localhost').searchParams.get('redirect_uri');
    res.setHeader('Content-Type', 'text/html');
    res.end(`<a href="${target}?state=fixture-state&code=fixture-code">Approve sign-in</a>`);
  });
  try {
    for (const name of ['host', 'identity']) {
      const result = spawnSync('/usr/bin/ssh-keygen', ['-q', '-t', 'ed25519', '-N', '', '-f', path.join(root, name)], { encoding: 'utf8', timeout: 10_000 });
      expect(result.status, result.stderr).toBe(0);
    }
    const reservation = createServer(); reservation.listen(0, '127.0.0.1'); await once(reservation, 'listening');
    const address = reservation.address(); if (!address || typeof address === 'string') throw new Error('No SSH fixture port');
    const port = address.port;
    await new Promise<void>((resolve, reject) => reservation.close(error => error ? reject(error) : resolve()));
    writeFileSync(path.join(testHome, 'known_hosts'), `[127.0.0.1]:${port} ${readFileSync(path.join(root, 'host.pub'), 'utf8')}`, { mode: 0o600 });
    const remotePath = [path.join(root, 'bin'), testHome, path.dirname(process.execPath), path.dirname(tmux), process.env.PATH ?? ''].join(path.delimiter);
    writeFileSync(path.join(root, 'bin', 'mosh-server'), `#!/bin/sh\n: > ${quote(path.join(root, 'mosh-invoked'))}\nexec ${quote(moshServer)} "$@" 2>${quote(path.join(root, 'mosh-server.log'))}\n`, { mode: 0o700 });
    writeFileSync(path.join(root, 'remote'), [
      '#!/bin/sh', `export HOME=${quote(path.join(root, 'home'))}`, 'export SHELL=/bin/bash',
      `export PATH=${quote(remotePath)}`, `export TMUX_TMPDIR=${quote(path.join(root, 'tmux'))}`,
      `export WEBMUX_RECOVERY_CACHE_HOME=${quote(path.join(root, 'cache'))}`,
      // Fixture-only cleanup bound; ordinary Mosh roaming policy is unchanged.
      'export MOSH_SERVER_NETWORK_TMOUT=15', 'unset TMUX TMUX_PANE',
      'case "$SSH_ORIGINAL_COMMAND" in',
      `  *--recovery-rpc) printf '%s\\n' recovery-rpc >> ${quote(path.join(root, 'operations'))} ;;`,
      `  *--auth-callback) printf '%s\\n' auth-callback >> ${quote(path.join(root, 'operations'))} ;;`,
      'esac', 'exec /bin/sh -c "$SSH_ORIGINAL_COMMAND"',
    ].join('\n') + '\n', { mode: 0o700 });
    writeFileSync(path.join(root, 'sshd_config'), [
      'ListenAddress 127.0.0.1', `Port ${port}`, `HostKey ${path.join(root, 'host')}`, `PidFile ${path.join(root, 'sshd.pid')}`,
      `AuthorizedKeysFile ${path.join(root, 'identity.pub')}`, `AllowUsers ${userInfo().username}`,
      'PasswordAuthentication no', 'KbdInteractiveAuthentication no', 'UsePAM no', 'PermitRootLogin no',
      'AllowAgentForwarding no', 'AllowTcpForwarding no', 'X11Forwarding no', 'PermitTunnel no', 'PermitUserRC no',
      `ForceCommand ${quote(path.join(root, 'remote'))}`, 'LogLevel VERBOSE',
    ].join('\n') + '\n', { mode: 0o600 });
    daemon = spawn('/usr/sbin/sshd', ['-D', '-e', '-f', path.join(root, 'sshd_config')], { detached: true, stdio: ['ignore', 'ignore', 'pipe'] });
    daemon.on('error', error => { startupError = error; });
    daemon.stderr!.on('data', chunk => { diagnostics = (diagnostics + String(chunk)).slice(-65536); });
    await expect.poll(async () => {
      if (startupError) throw startupError;
      if (daemon!.exitCode !== null || daemon!.signalCode !== null) throw new Error(`Private sshd exited: ${diagnostics}`);
      return new Promise<boolean>(resolve => {
        const socket = createConnection({ host: '127.0.0.1', port }); socket.setTimeout(500);
        socket.once('connect', () => { socket.destroy(); resolve(true); });
        socket.once('error', () => { socket.destroy(); resolve(false); });
        socket.once('timeout', () => { socket.destroy(); resolve(false); });
      });
    }, { timeout: 10_000 }).toBe(true);
    expect((await request.post('/api/keys', { data: { id: keyID, private_key_path: path.join(root, 'identity') } })).ok()).toBe(true);
    const created = await request.post('/api/sessions', { data: { transport: 'mosh', hostname: '127.0.0.1', port, username: userInfo().username, key_id: keyID, cols: 100, rows: 30 } });
    expect(created.ok()).toBe(true);
    const session = await created.json() as { id: string; title: string }; id = session.id;
    await page.goto('/');
    const tile = page.getByTestId(`tile-cell-${id}`), terminal = tile.locator('.xterm-helper-textarea');
    await terminal.focus(); await page.keyboard.type("printf '\\nMOSH_%s\\n' FEATURE_READY"); await page.keyboard.press('Enter');
    await expect(tile.locator('.xterm-rows')).toContainText('MOSH_FEATURE_READY', { timeout: 20_000 });
    const sessions = await (await request.get('/api/sessions')).json() as { id: string; transport: string }[];
    expect(sessions.find(value => value.id === id)?.transport).toBe('mosh');
    expect(readFileSync(path.join(root, 'mosh-invoked'), 'utf8')).toBe('');
    const pid = /detached, pid\s*=\s*(\d+)/.exec(readFileSync(path.join(root, 'mosh-server.log'), 'utf8'));
    expect(pid, 'Native mosh-server must identify the owned detached process').not.toBeNull(); serverPID = Number(pid![1]);
    receipt.nativeMosh = true;
    await new Promise<void>(resolve => identity.listen(0, '127.0.0.1', resolve));
    identityURL = `http://127.0.0.1:${(identity.address() as { port: number }).port}/authorize`;
    const config = path.join(root, 'auth.json'); writeFileSync(config, JSON.stringify({ identity: identityURL, mode: 'callback' }), { mode: 0o600 });
    await page.context().route(/http:\/\/127\.0\.0\.1:\d+\/callback\?/, route => { callbackURL = route.request().url(); return route.abort('connectionrefused'); });
    await terminal.focus(); await page.keyboard.type([process.execPath, path.resolve(__dirname, 'auth-handoff-fixture.mts'), config].map(quote).join(' ')); await page.keyboard.press('Enter');
    const panel = tile.getByRole('region', { name: `Sign-in for ${session.title}` });
    await expect(panel).toBeVisible({ timeout: 15_000 }); receipt.automaticPanel = true;
    const cache = readFileSync(path.join(root, 'cache', 'webmux', 'recovery', id, 'state.json'), 'utf8');
    const hostState = JSON.parse(cache) as { recording: boolean; browser_control: boolean; browser_request: unknown };
    expect(hostState.recording).toBe(false); expect(hostState.browser_control).toBe(true); expect(hostState.browser_request).toBeTruthy();
    expect(cache).not.toContain(identityURL); receipt.loggingDisabled = true;
    await page.reload(); await expect(panel).toBeVisible();
    await panel.getByRole('button', { name: 'Prepare callback relay' }).click();
    const popupPromise = page.waitForEvent('popup'); await panel.getByRole('link', { name: 'Continue in this browser' }).click();
    const popup = await popupPromise; await popup.getByText('Approve sign-in', { exact: true }).click();
    await expect.poll(() => callbackURL).toContain('code=fixture-code');
    await expect(tile.locator('.xterm-rows')).not.toContainText('CLI_AUTHENTICATED_ON_ORIGINATING_HOST');
    await panel.getByRole('textbox', { name: 'Final callback URL' }).fill(callbackURL);
    await panel.getByRole('button', { name: 'Deliver callback' }).click();
    await expect(tile.locator('.xterm-rows')).toContainText('CLI_AUTHENTICATED_ON_ORIGINATING_HOST'); receipt.callbackOnOriginalHost = true;
    const operations = readFileSync(path.join(root, 'operations'), 'utf8').split('\n');
    expect(operations).toContain('recovery-rpc'); expect(operations).toContain('auth-callback'); receipt.callbackViaSSH = true;
    await popup.close(); await panel.getByRole('button', { name: 'Dismiss sign-in' }).click();
    await expect.poll(() => JSON.parse(readFileSync(path.join(root, 'cache', 'webmux', 'recovery', id!, 'state.json'), 'utf8')).browser_request).toBeUndefined();
    await page.reload(); await expect(panel).toHaveCount(0); receipt.acknowledgedNotReplayed = true;
    expect(printedIdentityURL).toBe(false); receipt.noLaunchURLInTerminalOutput = true;
  } finally {
    const cleanupErrors: unknown[] = [];
    const clean = async (action: () => unknown | Promise<unknown>) => { try { await action(); } catch (error) { cleanupErrors.push(error); } };
    if (id) await clean(() => request.delete(`/api/sessions/${id}`));
    await clean(() => request.delete(`/api/keys/${keyID}`));
    await clean(() => {
      const env: NodeJS.ProcessEnv = { ...process.env, TMUX_TMPDIR: path.join(root, 'tmux') }; delete env.TMUX; delete env.TMUX_PANE;
      spawnSync(tmux, ['kill-server'], { env, timeout: 5000, stdio: 'ignore' });
      receipt.providerStopped = spawnSync(tmux, ['list-sessions'], { env, timeout: 5000, stdio: 'ignore' }).status !== 0;
    });
    if (serverPID) await clean(async () => {
      await expect.poll(() => { try { process.kill(serverPID!, 0); return false; } catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ESRCH') throw error; return true; } }, { timeout: 20_000 }).toBe(true);
      receipt.moshServerExited = true;
    });
    await clean(async () => {
      if (!daemon?.pid || daemon.exitCode !== null || daemon.signalCode !== null) return;
      const exited = once(daemon, 'exit'); process.kill(-daemon.pid, 'SIGTERM');
      const timer = setTimeout(() => { try { process.kill(-daemon!.pid!, 'SIGKILL'); } catch { /* already exited */ } }, 3000);
      try { await exited; } finally { clearTimeout(timer); }
    });
    receipt.sshDaemonExited = !daemon || daemon.exitCode !== null || daemon.signalCode !== null;
    await clean(async () => { identity.closeAllConnections(); await new Promise<void>(resolve => identity.close(() => resolve())); });
    mkdirSync(info.outputDir, { recursive: true });
    const file = info.outputPath('native-mosh-browser-receipt.json'); writeFileSync(file, JSON.stringify(receipt, null, 2) + '\n');
    writeFileSync(info.outputPath('private-ssh-diagnostics.log'), diagnostics);
    await info.attach('native-mosh-browser-receipt', { path: file, contentType: 'application/json' });
    expect(receipt.providerStopped).toBe(true); expect(receipt.sshDaemonExited).toBe(true);
    if (cleanupErrors.length) throw new AggregateError(cleanupErrors, 'Owned Mosh fixture cleanup failed');
  }
});
