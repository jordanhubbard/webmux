import { test, expect, type APIRequestContext, type Page } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { randomUUID, randomBytes } from 'node:crypto';
import { execFileSync, spawn, type ChildProcess } from 'node:child_process';

type FixtureConfig = { home: string; listen: string; node_id: string; ready_file: string; account_password: string; session_secret: string; fake_keys: string[]; provider_url: string };
type FixtureManifest = { version: number; nodes: Record<string, FixtureConfig> };

test('credential hub UI stores, grants, selects and uses a key across real HTTPS servers without exposing it', async ({ browser, playwright }, info) => {
  test.setTimeout(180_000);
  const repo = path.resolve(__dirname, '../..');
  const root = path.join(os.tmpdir(), `wmx-cred-ui-${randomUUID()}`);
  const binary = info.outputPath(process.platform === 'win32' ? 'credential-hub-fixture.exe' : 'credential-hub-fixture');
  fs.mkdirSync(path.dirname(binary), { recursive: true });
  execFileSync('go', ['build', '-o', binary, './cmd/credential-hub-fixture'], { cwd: path.join(repo, 'webmux/server'), timeout: 120_000, stdio: 'pipe' });
  execFileSync(binary, ['-initialize', root, '-defaults', path.join(repo, 'webmux/config.defaults'), '-web', path.join(repo, 'webmux/web')], { timeout: 15_000, stdio: 'pipe' });
  const manifest = JSON.parse(fs.readFileSync(path.join(root, 'manifest.json'), 'utf8')) as FixtureManifest;
  const knownSecrets = Object.values(manifest.nodes).flatMap(node => [node.account_password, node.session_secret, ...node.fake_keys]);
  const link = `wmx-canary-link-${randomBytes(32).toString('hex')}`;
  knownSecrets.push(link);
  let diagnostics = '';
  const redact = (text: string) => knownSecrets.reduce((result, value) => result.split(value).join('[redacted fixture credential]'), text);
  const children: ChildProcess[] = [];
  const contexts: { dispose: () => Promise<void> }[] = [];
  const start = async (name: string) => {
    const cfg = manifest.nodes[name];
    fs.rmSync(cfg.ready_file, { force: true });
    const env: NodeJS.ProcessEnv = { PATH: process.env.PATH, HOME: cfg.home, TMPDIR: cfg.home, XDG_CONFIG_HOME: path.join(cfg.home, 'xdg-config'), XDG_DATA_HOME: path.join(cfg.home, 'xdg-data'), TMUX_TMPDIR: cfg.home, WEBMUX_COORDINATION_DISABLED: '1' };
    for (const key of ['SYSTEMROOT', 'WINDIR', 'COMSPEC', 'TEMP', 'TMP']) if (process.env[key]) env[key] = process.env[key];
    const child = spawn(binary, ['-config', path.join(root, name, 'config.json')], { env, stdio: ['ignore', 'pipe', 'pipe'] });
    children.push(child);
    for (const stream of [child.stdout, child.stderr]) stream?.on('data', chunk => { diagnostics = redact(diagnostics + String(chunk)).slice(-10000); });
    await expect.poll(() => fs.existsSync(cfg.ready_file), { timeout: 15_000, message: `${name} fixture must become ready` }).toBe(true);
    const ready = JSON.parse(fs.readFileSync(cfg.ready_file, 'utf8')) as { address: string; pid: number };
    cfg.listen = ready.address;
    fs.writeFileSync(path.join(root, name, 'config.json'), JSON.stringify(cfg));
    return { child, url: `https://${ready.address}` };
  };
  const stop = async (child: ChildProcess) => {
    if (child.exitCode !== null || child.signalCode !== null) return;
    const exited = new Promise<void>(resolve => child.once('exit', () => resolve()));
    child.kill(); const timer = setTimeout(() => child.kill('SIGKILL'), 5000);
    try { await exited; } finally { clearTimeout(timer); }
  };
  const login = async (baseURL: string, cfg: FixtureConfig): Promise<APIRequestContext> => {
    const api = await playwright.request.newContext({ baseURL, ignoreHTTPSErrors: true });
    const response = await api.post('/api/auth/login', { data: { username: 'fixture', password: cfg.account_password } });
    expect(response.status()).toBe(200); const token = (await response.json()).token as string;
    knownSecrets.push(token); await api.dispose();
    const authenticated = await playwright.request.newContext({ baseURL, ignoreHTTPSErrors: true, extraHTTPHeaders: { Authorization: `Bearer ${token}` } });
    contexts.push(authenticated); return authenticated;
  };
  const ui = async (page: Page, url: string, cfg: FixtureConfig) => {
    page.setDefaultTimeout(15_000);
    console.log('credential UI: navigate', cfg.node_id);
    await page.goto(url, { waitUntil: 'domcontentloaded' }); await page.getByLabel('Username', { exact: true }).fill('fixture'); await page.getByLabel('Password', { exact: true }).fill(cfg.account_password); await page.getByRole('button', { name: 'Sign In', exact: true }).click();
    await page.getByRole('group', { name: 'WebMux cluster' }).getByRole('button', { name: 'Manage servers', exact: true }).click();
    await page.getByRole('button', { name: 'Manage shared credentials', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Refresh credential status' })).toBeEnabled();
  };
  let complete = false;
  try {
    let hub = await start('hub');
    manifest.nodes.receiver.provider_url = hub.url;
    fs.writeFileSync(path.join(root, 'receiver/config.json'), JSON.stringify(manifest.nodes.receiver));
    const receiver = await start('receiver');
    const hubAPI = await login(hub.url, manifest.nodes.hub);
    const receiverAPI = await login(receiver.url, manifest.nodes.receiver);
    const context = await browser.newContext({ ignoreHTTPSErrors: true });
    contexts.push({ dispose: () => context.close() });
    const hubPage = await context.newPage();
    await ui(hubPage, hub.url, manifest.nodes.hub);
    if (process.platform === 'win32') {
      await expect(hubPage.getByRole('button', { name: 'Enable encrypted credential storage' })).toBeDisabled();
      await expect(hubPage.getByText('This platform cannot provide the supported protected hub storage. Select a supported linked hub below.')).toBeVisible();
      complete = true; return;
    }
    await hubPage.getByRole('button', { name: 'Add incoming grant', exact: true }).click();
    await hubPage.getByLabel('Grant name', { exact: true }).fill('receiver');
    await hubPage.getByLabel('Session owner', { exact: true }).fill('fixture');
    await hubPage.getByLabel('Allow credential hub requests', { exact: true }).check();
    await expect(hubPage.getByLabel('Allow project coordination summaries', { exact: true })).not.toBeChecked();
    await hubPage.getByLabel('Sending node ID', { exact: true }).fill(manifest.nodes.receiver.node_id);
    await hubPage.getByLabel('Sending account', { exact: true }).fill('fixture');
    await hubPage.getByLabel('Credential', { exact: true }).fill(link);
    await hubPage.getByRole('button', { name: 'Save grant', exact: true }).click();
    await expect(hubPage.getByRole('region', { name: 'receiver incoming grant', exact: true })).toBeVisible();
    expect((await receiverAPI.put('/api/servers/hub', { data: { name: 'Fixture hub', url: hub.url, credential: link, allow_http: false } })).status()).toBe(204);
    expect((await receiverAPI.post('/api/servers/hub/verify-identity', { data: {} })).status()).toBe(204);
    // The UI loads its directory at open time; reopen after explicit enrollment.
    await hubPage.getByRole('button', { name: 'Close manage servers', exact: true }).click();
    await hubPage.getByRole('group', { name: 'WebMux cluster' }).getByRole('button', { name: 'Manage servers', exact: true }).click();
    await hubPage.getByRole('button', { name: 'Manage shared credentials', exact: true }).click();
    await hubPage.getByRole('button', { name: 'Enable encrypted credential storage' }).click();
    const form = hubPage.getByRole('form', { name: 'Store shared credential' });
    await form.getByLabel('Credential name', { exact: true }).fill('Browser credential');
    await form.getByLabel('API key', { exact: true }).fill(manifest.nodes.hub.fake_keys[0]);
    await form.getByRole('button', { name: 'Store credential', exact: true }).click();
    await expect(hubPage.getByRole('region', { name: 'Browser credential shared credential' })).toBeVisible();
    const grantForm = hubPage.getByRole('form', { name: 'Grant shared credential' });
    await grantForm.getByLabel('Credential to share').selectOption({ label: 'Browser credential' });
    await grantForm.getByLabel('Receiving enrollment').selectOption('receiver');
    await grantForm.getByRole('button', { name: 'Grant credential access' }).click();
    await expect(hubPage.getByRole('status', { name: 'Credential operation status' })).toContainText('Credential access granted');
    console.log('credential UI: grant created; opening receiver');
    const receiverContext = await browser.newContext({ ignoreHTTPSErrors: true });
    contexts.push({ dispose: () => receiverContext.close() });
    const receiverPage = await receiverContext.newPage();
    const receiverInfo = await receiverAPI.get('/api/credentials');
    expect(receiverInfo.status(), await receiverInfo.text()).toBe(200);
    await ui(receiverPage, receiver.url, manifest.nodes.receiver);
    await expect(receiverPage.getByRole('alert', { name: 'Credential operation error' })).toHaveCount(0);
    await receiverPage.getByLabel('Credential hub server', { exact: true }).selectOption('hub');
    await receiverPage.getByRole('button', { name: 'Load granted credential names' }).click();
    await receiverPage.getByLabel('Granted credential', { exact: true }).selectOption({ label: 'Browser credential · Version 1' });
    await receiverPage.getByRole('button', { name: 'Use selected credential hub' }).click();
    await expect(receiverPage.getByRole('status', { name: 'Selected credential hub' })).toContainText('Fixture hub');
    await receiverPage.getByLabel('AI request', { exact: true }).fill('credential qualification canary');
    await receiverPage.getByRole('button', { name: 'Ask AI with shared credential' }).click();
    await expect(receiverPage.getByRole('status', { name: 'Shared credential AI reply' })).toHaveText('CREDENTIAL_CANARY_PROVIDER_1');
    for (const api of [hubAPI, receiverAPI]) { const response = await api.get('/api/credentials'); expect(response.status()).toBe(200); const text = await response.text(); expect(manifest.nodes.hub.fake_keys.some(key => text.includes(key)), 'metadata excludes all credential values').toBe(false); }
    await receiverPage.screenshot({ path: info.outputPath('credential-hub-selected.png'), fullPage: true });
    await hubPage.getByRole('button', { name: 'Edit Browser credential', exact: true }).click();
    await form.getByLabel('Replacement API key', { exact: true }).fill(manifest.nodes.hub.fake_keys[1]);
    await form.getByRole('button', { name: 'Save credential changes' }).click();
    await expect(hubPage.getByRole('status', { name: 'Credential operation status' })).toContainText('Credential saved');
    await receiverPage.getByRole('button', { name: 'Ask AI with shared credential' }).click();
    await expect(receiverPage.getByRole('status', { name: 'Shared credential AI reply' })).toHaveText('CREDENTIAL_CANARY_PROVIDER_2');
    await stop(hub.child); hub = await start('hub');
    await receiverPage.getByRole('button', { name: 'Ask AI with shared credential' }).click();
    await expect(receiverPage.getByRole('status', { name: 'Shared credential AI reply' })).toHaveText('CREDENTIAL_CANARY_PROVIDER_2');
    await hubPage.getByRole('button', { name: 'Revoke credential grant receiver', exact: true }).click();
    await expect(hubPage.getByRole('status', { name: 'Credential operation status' })).toContainText('Grant revoked');
    await receiverPage.getByRole('button', { name: 'Ask AI with shared credential' }).click();
    await expect(receiverPage.getByRole('alert', { name: 'Credential operation error' })).toHaveText('credential access refused');
    await expect(receiverPage.getByRole('status', { name: 'Shared credential AI reply' })).toHaveCount(0);
    // NVIDIA uses a different selected key from the fixture's ambient canary.
    // A fallback to ambient NVIDIA configuration would return PROVIDER_2.
    await form.getByLabel('Credential name', { exact: true }).fill('Browser NVIDIA credential');
    await form.getByLabel('Credential consumer', { exact: true }).selectOption('ai.nvidia');
    await form.getByLabel('API key', { exact: true }).fill(manifest.nodes.hub.fake_keys[0]);
    await form.getByRole('button', { name: 'Store credential', exact: true }).click();
    await expect(hubPage.getByRole('region', { name: 'Browser NVIDIA credential shared credential', exact: true })).toBeVisible();
    await grantForm.getByLabel('Credential to share', { exact: true }).selectOption({ label: 'Browser NVIDIA credential' });
    await grantForm.getByRole('button', { name: 'Grant credential access', exact: true }).click();
    await expect(hubPage.getByRole('status', { name: 'Credential operation status' })).toContainText('Credential access granted');
    await receiverPage.getByLabel('Hub credential consumer', { exact: true }).selectOption('ai.nvidia');
    await receiverPage.getByRole('button', { name: 'Load granted credential names', exact: true }).click();
    await receiverPage.getByLabel('Granted credential', { exact: true }).selectOption({ label: 'Browser NVIDIA credential · Version 1' });
    await receiverPage.getByRole('button', { name: 'Use selected credential hub', exact: true }).click();
    await expect(receiverPage.getByRole('status', { name: 'Selected credential hub' })).toContainText('NVIDIA');
    await receiverPage.getByRole('button', { name: 'Ask AI with shared credential', exact: true }).click();
    await expect(receiverPage.getByRole('status', { name: 'Shared credential AI reply' })).toHaveText('CREDENTIAL_CANARY_PROVIDER_1');
    complete = true;
  } finally {
    let cleanupError: unknown;
    for (const context of contexts.reverse()) { try { await context.dispose(); } catch (error) { cleanupError ??= error; } }
    for (const child of children.reverse()) { try { await stop(child); } catch (error) { cleanupError ??= error; } }
    if (complete && cleanupError) throw cleanupError;
    if (!complete) await info.attach('credential-fixture-diagnostics', { body: redact(diagnostics), contentType: 'text/plain' });
    if (complete) fs.rmSync(root, { recursive: true, force: true });
  }
});
