import { test, expect } from '@playwright/test';
import { spawn, spawnSync, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import { createServer, createConnection } from 'node:net';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { homedir, userInfo } from 'node:os';
import { join } from 'node:path';

test('real OpenSSH key authentication, shell input and PTY resize', async ({ request, baseURL }, info) => {
  test.skip(process.env.WEBMUX_REAL_SSH !== '1', 'Requires Linux OpenSSH server and the isolated client wrapper');
  test.setTimeout(60000);
  // sshd StrictModes checks every ancestor of AuthorizedKeysFile. /tmp is
  // world-writable, so keep this owned 0700 fixture under the user's home.
  const temporary = mkdtempSync(join(homedir(), '.webmux-real-ssh-'));
  let daemon: ChildProcess | undefined;
  let client: WebSocket | undefined;
  let id: string | undefined;
  let diagnostics = '';
  let failed = false;
  try {
    for (const name of ['host', 'identity']) {
      const key = spawnSync('/usr/bin/ssh-keygen', ['-q', '-t', 'ed25519', '-N', '', '-f', join(temporary, name)], { encoding: 'utf8', timeout: 10000 });
      if (key.error) throw key.error;
      expect(key.status, key.stderr).toBe(0);
    }
    const reservation = createServer();
    reservation.listen(0, '127.0.0.1'); await once(reservation, 'listening');
    const address = reservation.address();
    if (!address || typeof address === 'string') throw new Error('No SSH fixture port');
    const port = address.port;
    await new Promise<void>((resolve, reject) => reservation.close(error => error ? reject(error) : resolve()));
    const config = join(temporary, 'sshd_config');
    writeFileSync(config, [
      'ListenAddress 127.0.0.1', `Port ${port}`, `HostKey ${join(temporary, 'host')}`,
      `PidFile ${join(temporary, 'sshd.pid')}`, `AuthorizedKeysFile ${join(temporary, 'identity.pub')}`,
      'PasswordAuthentication no', 'KbdInteractiveAuthentication no', 'UsePAM no',
      `AllowUsers ${userInfo().username}`,
      'PermitRootLogin no', 'AllowAgentForwarding no', 'AllowTcpForwarding no',
      'X11Forwarding no', 'PermitTunnel no', 'PermitUserRC no', 'LogLevel VERBOSE',
    ].join('\n') + '\n', { mode: 0o600 });
    let startupError: Error | undefined;
    daemon = spawn('/usr/sbin/sshd', ['-D', '-e', '-f', config], { detached: true, stdio: ['ignore', 'ignore', 'pipe'] });
    daemon.on('error', error => { startupError = error; });
    daemon.stderr!.on('data', chunk => { diagnostics = (diagnostics + String(chunk)).slice(-65536); });
    await expect.poll(async () => {
      if (startupError) throw startupError;
      if (daemon!.exitCode !== null || daemon!.signalCode !== null) throw new Error(`sshd exited: ${diagnostics}`);
      return new Promise<boolean>(resolve => {
        const socket = createConnection({ host: '127.0.0.1', port });
        socket.setTimeout(500);
        socket.once('connect', () => { socket.destroy(); resolve(true); });
        socket.once('error', () => { socket.destroy(); resolve(false); });
        socket.once('timeout', () => { socket.destroy(); resolve(false); });
      });
    }, { timeout: 10000 }).toBe(true);
    expect((await request.post('/api/keys', { data: { id: 'ssh-fixture', private_key_path: join(temporary, 'identity') } })).ok()).toBe(true);
    const created = await request.post('/api/sessions', { data: {
      hostname: '127.0.0.1', port, username: userInfo().username, transport: 'ssh', key_id: 'ssh-fixture', cols: 90, rows: 31,
    } });
    expect(created.ok()).toBe(true);
    id = (await created.json() as { id: string }).id;
    const auth = await request.post('/api/auth/refresh');
    const { token } = await auth.json() as { token: string };
    expect(typeof token).toBe('string');
    if (!baseURL) throw new Error('Missing backend URL');
    client = new WebSocket(`${baseURL.replace('http:', 'ws:')}/api/term/${id}?token=${encodeURIComponent(token)}`);
    let output = '';
    client.addEventListener('message', event => {
      const message = JSON.parse(String(event.data)) as { type?: string; data?: string };
      if (message.type === 'output' && typeof message.data === 'string') output = (output + message.data).slice(-65536);
    });
    await expect.poll(() => client!.readyState).toBe(WebSocket.OPEN);
    const input = (data: string) => client!.send(JSON.stringify({ type: 'input', data }));
    input("printf '\\nWEBMUX_%s\\n' SSH_READY\r");
    await expect.poll(() => output, { timeout: 15000 }).toContain('WEBMUX_SSH_READY');
    // Shell prompt controls may sit between the line break and stty's output.
    // A computed marker verifies dimensions without depending on prompt bytes
    // or mistaking the echoed command for the actual command result.
    const sizeCommand = "printf '\\nWEBMUX_SIZE_%s\\n' \"$(stty size)\"\r";
    input(sizeCommand);
    await expect.poll(() => output).toContain('WEBMUX_SIZE_31 90');
    client.send(JSON.stringify({ type: 'resize', cols: 88, rows: 33 }));
    await expect.poll(() => { input(sizeCommand); return output; }).toContain('WEBMUX_SIZE_33 88');
    expect(diagnostics).toContain('Accepted publickey');
  } catch (error) { failed = true; throw error; }
  finally {
    client?.close();
    try {
      if (id) expect((await request.delete(`/api/sessions/${id}`)).ok()).toBe(true);
      await request.delete('/api/keys/ssh-fixture');
    } catch (error) { if (!failed) throw error; diagnostics += `\nCleanup: ${String(error)}`; }
    finally {
      if (daemon?.pid && daemon.exitCode === null && daemon.signalCode === null) {
        const pid = daemon.pid;
        const signal = (value: NodeJS.Signals) => {
          try { process.kill(-pid, value); }
          catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ESRCH') throw error; }
        };
        const exited = once(daemon, 'exit');
        signal('SIGTERM');
        const timer = setTimeout(() => signal('SIGKILL'), 3000);
        try { await exited; } finally { clearTimeout(timer); signal('SIGKILL'); }
      }
      try {
        const log = info.outputPath('sshd.log'); writeFileSync(log, diagnostics);
        await info.attach('sshd-log', { path: log, contentType: 'text/plain' });
      } finally { rmSync(temporary, { recursive: true, force: true }); }
    }
  }
});
