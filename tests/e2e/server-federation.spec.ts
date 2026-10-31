import { test, expect } from '@playwright/test';
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';

test('two servers share interactive terminal views, messages and failure state without duplicating providers', async ({ page, request }) => {
  test.setTimeout(90000);
  const controllerHome = path.resolve(__dirname, '.test-home');
  const directoryFile = path.join(controllerHome, 'config/servers.yaml');
  const original = fs.readFileSync(directoryFile);
  const peerHome = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-federation-e2e-'));
  const listener = net.createServer();
  await new Promise<void>(resolve => listener.listen(0, '127.0.0.1', resolve));
  const port = (listener.address() as net.AddressInfo).port;
  await new Promise<void>((resolve, reject) => listener.close(error => error ? reject(error) : resolve()));
  fs.cpSync(path.resolve(__dirname, '../../webmux/config.defaults'), path.join(peerHome, 'config'), { recursive: true });
  fs.writeFileSync(path.join(peerHome, 'config/auth.yaml'), 'auth:\n  mode: none\n  users: []\n');
  fs.writeFileSync(path.join(peerHome, 'config/servers.yaml'), 'version: 1\nservers: []\ngrants:\n  - id: controller\n    owner: anonymous\n    token_env: WEBMUX_E2E_FEDERATION_TOKEN\n');
  const peer = spawn(path.join(controllerHome, process.platform === 'win32' ? 'webmux.exe' : 'webmux'), [], {
    env: { ...process.env, WEBMUX_HOME: peerHome, WEBMUX_ROOT: '', HTTP_PORT: String(port), WEBMUX_COORDINATION_DISABLED: '1', WEBMUX_E2E_FEDERATION_TOKEN: fs.readFileSync(path.join(controllerHome, 'federation-token'), 'utf8') },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  // Drain bounded fixture diagnostics; never retain credential-bearing environment.
  let diagnostics = '';
  peer.stdout?.on('data', chunk => { diagnostics = (diagnostics + String(chunk)).slice(-8000); });
  peer.stderr?.on('data', chunk => { diagnostics = (diagnostics + String(chunk)).slice(-8000); });
  const peerURL = `http://127.0.0.1:${port}`;
  const ids: string[] = [];
  const stopPeer = async () => {
    if (peer.exitCode !== null || peer.signalCode !== null) return;
    const exited = new Promise<void>(resolve => peer.once('exit', () => resolve()));
    peer.kill();
    const timer = setTimeout(() => peer.kill('SIGKILL'), 5000);
    try { await exited; } finally { clearTimeout(timer); }
  };
  try {
    await expect.poll(async () => {
      try { return (await request.get(`${peerURL}/api/health`, { timeout: 1000 })).status(); } catch { return 0; }
    }, { timeout: 15000, message: 'Peer WebMux should become healthy' }).toBe(200);
    fs.writeFileSync(directoryFile, `version: 1\nservers:\n  - id: remote\n    name: Other host\n    url: ${peerURL}\n    allow_http: true\n    token_env: WEBMUX_E2E_FEDERATION_TOKEN\ngrants: []\n`);
    for (const base of ['', peerURL]) {
      const created = await request.post(`${base}/api/sessions`, { data: { hostname: 'localhost', username: 'fixture', transport: 'exec', exec_command: `"${process.execPath}" "${path.resolve(__dirname, 'federation-fixture.mts')}"` } });
      expect(created.ok()).toBe(true);
      const id = (await created.json()).id as string;
      if (!base) ids.push(id);
      expect((await request.patch(`${base}/api/sessions/${id}`, { data: { project_label: 'WebMux' } })).ok()).toBe(true);
    }
    await page.goto('/');
    await page.getByRole('combobox', { name: 'Server view' }).selectOption('all');
    const remote = page.locator('[data-testid^="server-terminal-remote-"]');
    await expect(remote.locator('.xterm-rows')).toContainText('FEDERATION_READY:');
    await expect(page.locator('[data-testid^="server-terminal-local-"]')).toHaveCount(1);
    await expect(remote).toHaveAccessibleName(/Other host · WebMux/);
    await remote.locator('.xterm-helper-textarea').focus();
    await page.keyboard.type('native-input-proof');
    await page.keyboard.press('Enter');
    await expect(remote.locator('.xterm-rows')).toContainText('FEDERATION_REPLY:native-input-proof');
    await remote.getByRole('button', { name: 'Message', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('textbox', { name: 'Message' }).fill('agent-message-proof');
    await dialog.getByRole('button', { name: 'Send to this session' }).click();
    await expect(dialog.getByRole('status')).toContainText('processing is not confirmed');
    await page.keyboard.press('Escape');
    await expect(remote.locator('.xterm-rows')).toContainText('FEDERATION_REPLY:[WebMux controller] agent-message-proof');
    await remote.getByRole('button', { name: 'Hide', exact: true }).click();
    await expect(remote).toHaveCount(0);
    await page.getByRole('button', { name: 'Show hidden viewers' }).click();
    await expect(remote.locator('.xterm-rows')).toContainText('native-input-proof');
    await page.getByRole('combobox', { name: 'Server view' }).selectOption('remote');
    await expect(page.locator('[data-testid^="server-terminal-local-"]')).toHaveCount(0);
    await page.reload();
    await expect(page.getByRole('combobox', { name: 'Server view' })).toHaveValue('remote');
    await expect(remote.locator('.xterm-rows')).toContainText('native-input-proof');
    expect((await (await request.get(`${peerURL}/api/sessions`)).json()).length).toBe(1);
    await remote.locator('.xterm-helper-textarea').focus();
    await page.keyboard.type('finish-provider');
    await page.keyboard.press('Enter');
    await expect(remote.getByText('disconnected', { exact: true })).toBeVisible();
    await expect(remote.getByText('Viewer: connected', { exact: true })).toBeVisible();
    await expect(remote.getByRole('button', { name: 'Restart provider' })).toBeVisible();
    page.once('dialog', dialog => { void dialog.accept(); });
    await remote.getByRole('button', { name: 'Restart provider' }).click();
    await expect(remote.getByText('connected', { exact: true })).toBeVisible();
    await expect(remote.locator('.xterm-rows')).toContainText('FEDERATION_READY:');
    await page.getByRole('combobox', { name: 'Server view' }).selectOption('all');
    await stopPeer();
    await expect(page.getByRole('region', { name: 'Other host sessions', exact: true }).locator('.server-heading').getByRole('alert')).toContainText('Unavailable', { timeout: 15000 });
    await expect(remote.getByText('Viewer: disconnected', { exact: true })).toBeVisible();
    // The server outage makes provider state stale; it does not prove a stop.
    await expect(remote.getByText('connected', { exact: true })).toBeVisible();
    await expect(page.locator('[data-testid^="server-terminal-local-"] .xterm-rows')).toContainText('FEDERATION_READY:');
    await page.screenshot({ path: path.resolve(__dirname, '../../webmux/test-results/server-federation.png'), fullPage: true });
  } catch (error) {
    throw new Error(`${String(error)}\nPeer diagnostics: ${diagnostics}`);
  } finally {
    await page.evaluate(() => sessionStorage.removeItem('webmux-server-view')).catch(() => {});
    fs.writeFileSync(directoryFile, original);
    await stopPeer();
    for (const id of ids) await request.delete(`/api/sessions/${id}`);
    fs.rmSync(peerHome, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
  }
});
