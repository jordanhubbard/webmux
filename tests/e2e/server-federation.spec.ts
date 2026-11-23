import { test, expect, type Locator } from '@playwright/test';
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { createFederationDiagnostics } from './federation-diagnostics';
import { installFederationReplayBarrier, releaseFederationReplayOffscreen } from './federation-replay-control';

// xterm pauses offscreen painting. Its DOM is an output oracle only after
// exposing the existing screen; this neither reconnects nor requests replay.
async function visibleRows(terminal: Locator) {
  // Refresh replaces xterm's screen element inside the stable terminal card.
  await terminal.scrollIntoViewIfNeeded({ timeout: 5000 });
  return terminal.locator('.xterm-rows');
}

test('two servers share interactive terminal views, messages and failure state without duplicating providers', async ({ page, request }, info) => {
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
  const startsFile = path.join(peerHome, 'provider-starts.jsonl');
  const observation = createFederationDiagnostics(page, request, info, peerURL, startsFile);
  const offscreenControl = observation.enabled && process.env.WEBMUX_FEDERATION_DIAGNOSTIC_CONTROL === 'offscreen-replay';
  if (offscreenControl) await installFederationReplayBarrier(page);
  let completed = false;
  const ids: string[] = [];
  // Hold the real policy response through directory save/connection feedback.
  // Independent status messages must coexist without ambiguous assertions.
  let releasePolicyResponse!: () => void;
  const policyResponseGate = new Promise<void>(resolve => { releasePolicyResponse = resolve; });
  const policyResponses: Promise<void>[] = [];
  const stopPeer = async () => {
    if (peer.exitCode !== null || peer.signalCode !== null) return;
    const exited = new Promise<void>(resolve => peer.once('exit', () => resolve()));
    peer.kill();
    const timer = setTimeout(() => peer.kill('SIGKILL'), 5000);
    try { await exited; } finally { clearTimeout(timer); }
  };
  try {
    await page.route('**/api/session-coordination/policy', route => {
      const response = (async () => {
        const upstream = await route.fetch();
        await policyResponseGate;
        await route.fulfill({ response: upstream });
      })();
      policyResponses.push(response);
      return response;
    });
    await expect.poll(async () => {
      try { return (await request.get(`${peerURL}/api/health`, { timeout: 1000 })).status(); } catch { return 0; }
    }, { timeout: 15000, message: 'Peer WebMux should become healthy' }).toBe(200);
    // Start with no peers: management must be discoverable before the selector exists.
    fs.writeFileSync(directoryFile, 'version: 1\nservers: []\ngrants: []\n');
    for (const base of ['', peerURL]) {
      const fixtureArgs = observation.enabled ? ` "${startsFile}" ${base ? 'peer' : 'local'}` : '';
      const created = await request.post(`${base}/api/sessions`, { data: { hostname: 'localhost', username: 'fixture', transport: 'exec', exec_command: `"${process.execPath}" "${path.resolve(__dirname, 'federation-fixture.mts')}"${fixtureArgs}` } });
      expect(created.ok()).toBe(true);
      const id = (await created.json()).id as string;
      if (!base) ids.push(id);
      expect((await request.patch(`${base}/api/sessions/${id}`, { data: { project_label: 'WebMux' } })).ok()).toBe(true);
    }
    await page.setViewportSize({ width: 900, height: offscreenControl ? 1100 : 650 });
    await page.goto('/');
    await expect(page.getByRole('button', { name: 'More', exact: true })).toBeInViewport();
    const cluster = page.getByRole('group', { name: 'WebMux cluster' });
    await cluster.getByRole('button', { name: 'Refresh servers', exact: true }).click();
    await cluster.getByRole('button', { name: 'Add server', exact: true }).click();
    const manager = page.getByRole('dialog', { name: 'Manage servers' });
    await expect(manager.getByLabel('ID', { exact: true })).toHaveCount(0);
    await manager.getByLabel('Name (optional)', { exact: true }).fill('Other host');
    await manager.getByLabel('Server URL', { exact: true }).fill(peerURL);
    await manager.getByLabel('Allow HTTP for a trusted private network or tunnel').check();
    await manager.getByLabel('Credential', { exact: true }).fill(fs.readFileSync(path.join(controllerHome, 'federation-token'), 'utf8'));
    await manager.getByRole('button', { name: 'Save server', exact: true }).click();
    await expect(manager.getByRole('status', { name: 'Server directory status' })).toContainText('Saved');
    const policy = manager.getByRole('region', { name: 'Automatic session coordination' });
    await expect(policy.getByRole('status')).toHaveText('Loading coordination policy…');
    const directory = await request.get('/api/server-directory').then(response => response.json());
    const peerID = directory.servers.find((server: { url: string }) => server.url === peerURL)?.id as string;
    expect(peerID).toBeTruthy();
    await manager.getByRole('button', { name: 'Test connection to Other host' }).click();
    await expect(manager.getByRole('status', { name: 'Server directory status' })).toContainText('connection successful');
    await expect(policy.getByRole('status')).toHaveText('Loading coordination policy…');
    releasePolicyResponse();
    await expect(policy.getByLabel('Enable automatic session coordination')).toBeVisible();
    await page.screenshot({ path: path.resolve(__dirname, '../../webmux/test-results/server-management.png'), fullPage: true });
    await page.keyboard.press('Escape');
    const selector = page.getByRole('combobox', { name: 'Server view' });
    await expect(selector).toHaveValue('local');
    await expect(selector.locator('option[value="local"]')).toContainText('Show my windows only');
    await expect(selector.locator(`option[value="${peerID}"]`)).toHaveText('Show Other host windows only');
    await expect(selector.locator('option[value="all"]')).toHaveText('Show all connected servers’ windows');
    await expect(page.locator('[data-testid^="server-terminal-"]')).toHaveCount(0);
    await page.getByRole('combobox', { name: 'Server view' }).selectOption('all');
    observation.phase('initial-federated-view');
    const remote = page.locator(`[data-testid^="server-terminal-${peerID}-"]`);
    await expect(await visibleRows(remote)).toContainText('FEDERATION_READY:');
    await expect(page.locator('[data-testid^="server-terminal-local-"]')).toHaveCount(1);
    await expect(remote).toHaveAccessibleName(/Other host · WebMux/);
    await expect(remote.locator('.server-badge')).toHaveText(`Other host · ${peerID}`);
    const local = page.locator('[data-testid^="server-terminal-local-"]');
    await expect(local.locator('.server-badge')).toContainText('this server');
    expect(await remote.evaluate(el => el.parentElement === document.querySelector('[aria-label="Server terminal matrix"]'))).toBe(true);
    expect(await local.evaluate(el => el.parentElement === document.querySelector('[aria-label="Server terminal matrix"]'))).toBe(true);
    const providerPid = (await remote.locator('.xterm-rows').innerText()).match(/FEDERATION_READY:(\d+)/)![1];
    if (offscreenControl) {
      observation.phase('diagnostic-offscreen-replay');
      await page.evaluate(() => window.federationReplayBarrier!.arm());
      await remote.getByRole('button', { name: 'Reconnect viewer', exact: true }).click();
      await page.setViewportSize({ width: 900, height: 650 });
      await expect.poll(() => page.evaluate(() => window.federationReplayBarrier!.held)).toBeGreaterThan(0);
      // Confirm that connected status cleared the old paint before releasing
      // the same native replay bytes outside xterm's visible render region.
      await expect(remote.locator('.xterm-rows')).not.toContainText(`FEDERATION_READY:${providerPid}`);
      await releaseFederationReplayOffscreen(page);
      await expect.poll(() => page.evaluate(() => window.federationReplayBarrier!.delivered)).toBeGreaterThan(0);
      const control = await page.evaluate(() => {
        const { armed, held, delivered, callbackError } = window.federationReplayBarrier!;
        return { armed, held, delivered, callbackError };
      });
      expect(control.callbackError).toBe(false);
      const controlFile = info.outputPath('offscreen-replay-control.json');
      // This intentionally retains the original, unscrolled DOM assertion.
      // Its failure must survive the diagnostic visibility intervention.
      try {
        await expect(remote.locator('.xterm-rows')).toContainText(`FEDERATION_READY:${providerPid}`);
      } catch (error) {
        const failure = error as { matcherResult?: { name?: unknown; actual?: unknown } };
        if (failure.matcherResult?.name === 'toContainText' && typeof failure.matcherResult.actual === 'string' && !failure.matcherResult.actual.trim()) {
          fs.writeFileSync(controlFile, JSON.stringify({ ...control, expectedPID: Number(providerPid), originalBlankAssertionFailed: true }));
        }
        throw error;
      }
      throw new Error('Offscreen replay control did not reproduce blank paint');
    }
    if (observation.enabled && process.env.WEBMUX_FEDERATION_DIAGNOSTIC_CONTROL === 'fail-before-revocation') {
      observation.phase('diagnostic-control-before-revocation');
      expect(false, 'FEDERATION_DIAGNOSTIC_CONTROL_478').toBe(true);
    }
    // A peer may revoke its incoming grant while the controller login remains
    // valid. That must close only the remote viewer, never all local sockets.
    const peerDirectory = path.join(peerHome, 'config/servers.yaml');
    const peerGrants = fs.readFileSync(peerDirectory);
    const replacePeerGrants = (contents: string | Buffer) => {
      const temporary = `${peerDirectory}.replacement`;
      fs.writeFileSync(temporary, contents);
      fs.renameSync(temporary, peerDirectory);
    };
    await expect(local.getByTitle('Browser viewer connection')).toHaveText('Viewer: connected');
    observation.phase('grant-revocation');
    replacePeerGrants('version: 1\nservers: []\ngrants: []\n');
    await expect(remote.getByTitle('Browser viewer connection')).toHaveText('Viewer: disconnected', { timeout: 10000 });
    await expect(local.getByTitle('Browser viewer connection')).toHaveText('Viewer: connected');
    expect((await request.post(`/api/servers/local/sessions/${ids[0]}/messages`, { data: { message: 'local-after-peer-revocation' } })).ok()).toBe(true);
    await expect(await visibleRows(local)).toContainText('FEDERATION_REPLY:[WebMux local:anonymous] local-after-peer-revocation');
    replacePeerGrants(peerGrants);
    observation.phase('reconnect-after-grant-restoration');
    await remote.getByRole('button', { name: 'Reconnect viewer', exact: true }).click();
    await expect(await visibleRows(remote)).toContainText(`FEDERATION_READY:${providerPid}`);
    expect((await request.get(`${peerURL}/api/sessions`).then(response => response.json())).length).toBe(1);
    const overview = page.getByTitle('Workspace overview — click or drag to jump', { exact: true });
    await expect(overview).toBeVisible();
    await expect(overview.locator('rect title')).toHaveCount(2);
    await expect(overview).toContainText('Other host');
    const scrollArea = page.locator('.server-workspace');
    const overviewBox = (await overview.locator('svg').boundingBox())!;
    await overview.locator('svg').click({ position: { x: overviewBox.width / 2, y: overviewBox.height - 2 } });
    await expect.poll(() => scrollArea.evaluate(element => element.scrollTop)).toBeGreaterThan(0);
    // The navigator stays in the viewport while the terminal grid scrolls.
    await expect(overview).toBeInViewport();
    observation.phase('refresh-after-overview-scroll');
    await page.getByRole('button', { name: 'Refresh Other host', exact: true }).click();
    await expect(await visibleRows(remote)).toContainText(`FEDERATION_READY:${providerPid}`);
    // Reproduce an unavailable service credential, repair it through the UI,
    // and prove the existing provider survives both failure and reattachment.
    fs.writeFileSync(directoryFile, `version: 1\nservers:\n  - id: ${peerID}\n    name: Other host\n    url: ${peerURL}\n    allow_http: true\n    token_env: WEBMUX_E2E_MISSING_TOKEN\ngrants: []\n`);
    observation.phase('credential-unavailable');
    await page.getByRole('button', { name: 'Refresh Other host', exact: true }).click();
    await expect(page.getByRole('region', { name: 'Other host sessions', exact: true }).getByRole('alert')).toContainText('Manage servers');
    await page.getByRole('button', { name: 'Manage connection to Other host' }).click();
    await manager.getByRole('button', { name: 'Edit Other host' }).click();
    await manager.getByLabel('Replacement credential').fill(fs.readFileSync(path.join(controllerHome, 'federation-token'), 'utf8'));
    observation.phase('credential-repair');
    await manager.getByRole('button', { name: 'Save server', exact: true }).click();
    await expect(manager.getByRole('status', { name: 'Server directory status' })).toContainText('Saved');
    await page.keyboard.press('Escape');
    await page.getByRole('group', { name: 'WebMux cluster' }).getByRole('button', { name: 'Refresh servers', exact: true }).click();
    await expect(await visibleRows(remote)).toContainText(`FEDERATION_READY:${providerPid}`);
    expect((await (await request.get(`${peerURL}/api/sessions`)).json()).length).toBe(1);
    observation.phase('pack');
    await page.getByRole('toolbar', { name: 'Terminal layout' }).getByRole('button', { name: 'Pack', exact: true }).click();
    const matrix = page.getByLabel('Server terminal matrix');
    await expect(matrix.locator('.server-terminal')).toHaveCount(2);
    await expect(overview).toBeVisible();
    await expect(overview.locator('rect title')).toHaveCount(2);
    await expect(await visibleRows(remote)).toContainText(`FEDERATION_READY:${providerPid}`);
    await remote.locator('.xterm-helper-textarea').focus();
    observation.phase('native-input');
    await page.keyboard.type('native-input-proof');
    await page.keyboard.press('Enter');
    await expect(await visibleRows(remote)).toContainText('FEDERATION_REPLY:native-input-proof');
    await expect(await visibleRows(local)).not.toContainText('native-input-proof');
    const initialSize = await request.get(`/api/servers/${peerID}/sessions`).then(r => r.json()).then(s => `${s[0].cols}x${s[0].rows}`);
    observation.phase('resize');
    // Change width as well as height: terminal tiles have a fixed height.
    await page.setViewportSize({ width: 1100, height: 720 });
    await expect.poll(async () => {
      const sessions = await (await request.get(`/api/servers/${peerID}/sessions`)).json();
      return `${sessions[0].cols}x${sessions[0].rows}`;
    }).not.toBe(initialSize);
    await page.setViewportSize({ width: 1440, height: 900 });
    observation.phase('explicit-reconnect');
    await remote.getByRole('button', { name: 'Reconnect viewer', exact: true }).click();
    await expect(await visibleRows(remote)).toContainText(`FEDERATION_READY:${providerPid}`);
    await remote.getByRole('button', { name: 'Message', exact: true }).click();
    observation.phase('message');
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('textbox', { name: 'Message' }).fill('agent-message-proof');
    await dialog.getByRole('button', { name: 'Send to this session' }).click();
    await expect(dialog.getByRole('status')).toContainText('processing is not confirmed');
    await page.keyboard.press('Escape');
    await expect(await visibleRows(remote)).toContainText('FEDERATION_REPLY:[WebMux controller] agent-message-proof');
    // Leave room for one terminal below the wrapped cluster toolbar.
    await page.setViewportSize({ width: 900, height: 750 });
    observation.phase('hide-and-show');
    await remote.getByRole('button', { name: 'Hide', exact: true }).click();
    await expect(remote).toHaveCount(0);
    await expect(overview).toBeHidden();
    await page.getByRole('button', { name: 'Show hidden viewers' }).click();
    await expect(await visibleRows(remote)).toContainText('native-input-proof');
    await expect(await visibleRows(remote)).toContainText(`FEDERATION_READY:${providerPid}`);
    await expect(overview).toBeVisible();
    await page.getByRole('combobox', { name: 'Server view' }).selectOption(peerID);
    observation.phase('selected-peer-and-reload');
    await expect(page.locator('[data-testid^="server-terminal-local-"]')).toHaveCount(0);
    await page.reload();
    await expect(page.getByRole('combobox', { name: 'Server view' })).toHaveValue(peerID);
    await expect(page.getByLabel('Server terminal matrix').locator('.server-terminal')).toHaveCount(1);
    await expect(await visibleRows(remote)).toContainText('native-input-proof');
    await page.setViewportSize({ width: 900, height: 380 });
    await expect(overview).toBeVisible();
    await expect(overview.locator('rect title')).toHaveCount(1);
    await expect(overview).toContainText('Other host');
    await page.setViewportSize({ width: 1600, height: 1000 });
    await expect(overview).toBeHidden();
    await page.setViewportSize({ width: 900, height: 650 });
    expect((await (await request.get(`${peerURL}/api/sessions`)).json()).length).toBe(1);
    await selector.selectOption('local');
    observation.phase('switch-local-and-peer');
    await expect(page.locator('[data-testid^="server-terminal-"]')).toHaveCount(0);
    await expect(await visibleRows(page.getByTestId(`tile-cell-${ids[0]}`))).toContainText('FEDERATION_READY:');
    await selector.selectOption(peerID);
    await expect(await visibleRows(remote)).toContainText(`FEDERATION_READY:${providerPid}`);
    await remote.locator('.xterm-helper-textarea').focus();
    await page.keyboard.type('finish-provider');
    observation.phase('explicit-provider-stop');
    await page.keyboard.press('Enter');
    await expect(remote.getByText('disconnected', { exact: true })).toBeVisible();
    await expect(remote.getByText('Viewer: connected', { exact: true })).toBeVisible();
    await expect(remote.getByRole('button', { name: 'Restart provider' })).toBeVisible();
    page.once('dialog', dialog => { void dialog.accept(); });
    observation.phase('explicit-provider-restart');
    await remote.getByRole('button', { name: 'Restart provider' }).click();
    await expect(remote.getByText('connected', { exact: true })).toBeVisible();
    await expect(await visibleRows(remote)).toContainText('FEDERATION_READY:');
    await page.getByRole('combobox', { name: 'Server view' }).selectOption('all');
    observation.phase('peer-outage');
    await stopPeer();
    await expect(page.getByRole('region', { name: 'Other host sessions', exact: true }).locator('.server-heading').getByRole('alert')).toContainText('Unavailable', { timeout: 15000 });
    await expect(remote.getByText('Viewer: disconnected', { exact: true })).toBeVisible();
    // The server outage makes provider state stale; it does not prove a stop.
    await expect(remote.getByText('connected', { exact: true })).toBeVisible();
    await expect(await visibleRows(page.locator('[data-testid^="server-terminal-local-"]'))).toContainText('FEDERATION_READY:');
    await page.screenshot({ path: path.resolve(__dirname, '../../webmux/test-results/server-federation.png'), fullPage: true });
    await cluster.getByRole('button', { name: 'Manage servers', exact: true }).click();
    page.once('dialog', dialog => { void dialog.accept(); });
    await manager.getByRole('button', { name: 'Remove Other host', exact: true }).click();
    await expect(manager.getByText('No server peers configured. Add a server to browse its terminals.')).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.getByRole('combobox', { name: 'Server view' })).toHaveCount(0);
    observation.phase('complete');
    completed = true;
  } catch (error) {
    // Keep Playwright's original assertion stack and locator information.
    console.error('Federation peer diagnostics bytes:', Buffer.byteLength(diagnostics));
    throw error;
  } finally {
    releasePolicyResponse();
    await Promise.allSettled(policyResponses);
    await page.unroute('**/api/session-coordination/policy');
    let observationError: unknown;
    try { await observation.capture(completed, peer, Buffer.byteLength(diagnostics)); }
    catch (error) { observationError = error; console.error('Federation diagnostic capture failed'); }
    await page.evaluate(() => sessionStorage.removeItem('webmux-server-view')).catch(() => {});
    fs.writeFileSync(directoryFile, original);
    await stopPeer();
    for (const id of ids) await request.delete(`/api/sessions/${id}`);
    fs.rmSync(peerHome, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
    if (completed && observationError) throw observationError;
  }
});
