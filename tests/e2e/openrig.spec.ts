import { test, expect } from '@playwright/test';
import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { randomBytes } from 'node:crypto';
import { spawnSync } from 'node:child_process';

test.afterAll(() => {
  if (process.env.WEBMUX_REAL_OPENRIG !== '1') return;
  const executable = path.resolve(__dirname, '.test-home', 'openrig-bin', 'rig');
  if (!existsSync(executable)) return;
  // Stop before Playwright terminates the web-server process group. Waiting for
  // the server's exit handler alone can leave the detached Rig daemon behind.
  const stopped = spawnSync(executable, ['daemon', 'stop'], { timeout: 15000, encoding: 'utf8' });
  expect(stopped.status, stopped.stdout + stopped.stderr).toBe(0);
});

test('coordination is a dismissible modal and leaves the terminal grid intact', async ({ page, request }) => {
  const original = await (await request.get('/api/sessions')).json();
  let attachments = 0;
  page.on('request', value => { if (value.method() === 'POST' && /coordination|openrig/.test(value.url())) attachments++; });
  await page.goto('/');
  await page.getByRole('button', { name: 'Rig', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Rig', exact: true });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole('heading', { name: 'Create agent', exact: true })).toBeVisible();
  await expect(dialog.getByLabel('Project directory on this host')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Run fresh challenge' })).toHaveCount(0);
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await expect(page.getByTestId('workspace-scroll')).toBeVisible();
  expect(attachments).toBe(0);
  expect(await (await request.get('/api/sessions')).json()).toEqual(original);
});

test('managed agents enroll, exchange attributed Rig messages and reuse ordinary grid terminals', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_OPENRIG !== '1', 'Requires the isolated published-CLI fixture');
  test.setTimeout(120_000);
  page.setDefaultTimeout(10_000);
  const home = path.resolve(__dirname, '.test-home');
  const project = 'browser-workload';
  const sessions: string[] = [];
  let workloadCompleted = false;
  type Agent = { name: string; target: string; connected: boolean; sessionId?: string };
  type Session = { id: string; openrig_view?: string; rig?: { target: string; project: string; name: string; host: string; runtime: string } };
  const status = async () => (await (await request.get('/api/coordination')).json()) as {
    agents: Agent[]; messages: { from: string; to: string; body: string }[];
  };
  const openDialog = async () => {
    await page.getByRole('button', { name: 'Rig', exact: true }).click();
    return page.getByRole('dialog', { name: 'Rig', exact: true });
  };
  const openAgent = async (name: string, reconnect = false) => {
    const dialog = await openDialog();
    const response = page.waitForResponse(value => value.url().endsWith('/api/coordination/attach') && value.request().method() === 'POST');
    await dialog.locator('article').filter({ has: page.getByText(name, { exact: true }) }).getByRole('button', { name: reconnect ? 'Reconnect agent' : 'Open terminal', exact: true }).click();
    const attached = await response;
    expect(attached.status()).toBe(200);
    const session = await attached.json() as Session;
    await expect(dialog).toHaveCount(0);
    return session;
  };
  try {
    await page.goto('/');
    for (const name of ['coder', 'reviewer']) {
      const dialog = await openDialog();
      await dialog.getByLabel('Project ID', { exact: true }).fill(project);
      await dialog.getByLabel('Agent name', { exact: true }).fill(name);
      await dialog.getByLabel('Execution host', { exact: true }).fill('localhost');
      await dialog.getByLabel('Project directory on this host', { exact: true }).fill(home);
      await dialog.getByRole('combobox', { name: 'Agent', exact: true }).selectOption('codex');
      const response = page.waitForResponse(value => value.url().endsWith('/api/coordination/agents') && value.request().method() === 'POST');
      await dialog.getByRole('button', { name: 'Create and open terminal', exact: true }).click();
      expect((await response).status()).toBe(200);
      await expect(dialog).toHaveCount(0);
      const agent = (await status()).agents.find(value => value.name === name)!;
      const listing: Session[] = await (await request.get('/api/sessions')).json();
      const session = listing.find(value => value.openrig_view === `managed:${agent.target}`)!;
      sessions.push(session.id);
      expect(session.rig).toEqual({ target: agent.target, project, name, host: 'localhost', runtime: 'codex' });
      const terminal = page.getByTestId(`tile-cell-${session.id}`);
      await expect(terminal.getByLabel('Rig-managed worker')).toContainText(`Rig · ${project} · ${name} · localhost`);
      await expect(terminal.locator('.xterm-rows')).toContainText('FIXTURE_READY');
      await expect(terminal.locator('.xterm-rows')).toContainText(agent.target);
    }
    const agents = (await status()).agents;
    const coder = agents.find(value => value.name === 'coder')!;
    const reviewer = agents.find(value => value.name === 'reviewer')!;
    expect(agents).toHaveLength(2);
    expect(agents.every(value => value.connected)).toBe(true);
    const nonce = randomBytes(16).toString('hex');
    const terminal = page.getByTestId(`tile-cell-${sessions[0]}`);
    await terminal.locator('.xterm-helper-textarea').focus();
    await page.keyboard.insertText(`fixture-send ${reviewer.target} WM_CHALLENGE:${coder.target}:${nonce}`);
    await page.keyboard.press('Enter');
    // The ACK is generated by the peer only after receiving Rig's tmux delivery.
    await expect(terminal.locator('.xterm-rows')).toContainText(`WM_ACK:${nonce}`, { timeout: 30_000 });
    await expect.poll(async () => (await status()).messages.some(value => value.from === reviewer.target && value.to === coder.target && value.body.includes(`WM_ACK:${nonce}`))).toBe(true);
    const dialog = await openDialog();
    await expect(dialog).toContainText(`WM_ACK:${nonce}`);
    await page.keyboard.press('Escape');
    await page.reload();
    for (const id of sessions) await expect(page.getByTestId(`tile-cell-${id}`)).toBeVisible();
    expect((await status()).agents.map(value => value.target)).toEqual(agents.map(value => value.target));
    expect((await openAgent('coder')).id).toBe(sessions[0]);
    expect((await (await request.get('/api/sessions')).json()).map((value: Session) => value.id)).toEqual(sessions);
    // Close the actual tile, then reopen the same agent without launching a third.
    await terminal.getByTitle('Close', { exact: true }).click();
    await expect(terminal).toHaveCount(0);
    expect((await status()).agents.every(value => value.connected)).toBe(true);
    const coderSession = await openAgent('coder');
    sessions.push(coderSession.id);
    await expect(page.getByTestId(`tile-cell-${coderSession.id}`).locator('.xterm-rows')).toContainText(`WM_ACK:${nonce}`);
    expect((await status()).agents).toHaveLength(2);
    const tmuxEnv = { ...process.env, TMUX_TMPDIR: readFileSync(path.join(home, 'openrig-tmux-directory'), 'utf8') };
    let previousId = (await status()).agents.find(value => value.target === coder.target)!.sessionId;
    expect(previousId).toBeTruthy();
    for (let reconnect = 0; reconnect < 2; reconnect++) {
      const stopped = spawnSync('tmux', ['kill-session', '-t', `=${coder.target}`], { env: tmuxEnv, timeout: 5000 });
      expect(stopped.status).toBe(0);
      await expect.poll(async () => (await status()).agents.find(value => value.target === coder.target)!.connected).toBe(false);
      expect((await openAgent('coder', true)).id).toBe(coderSession.id);
      await expect(page.getByTestId(`tile-cell-${coderSession.id}`).locator('.xterm-rows')).toContainText('FIXTURE_READY');
      const current = (await status()).agents.find(value => value.target === coder.target)!;
      expect(current.connected).toBe(true);
      expect(current.sessionId).toBeTruthy();
      expect(current.sessionId).not.toBe(previousId);
      previousId = current.sessionId;
      expect((await status()).agents).toHaveLength(2);
    }
    await page.screenshot({ path: info.outputPath('managed-rig-workload.png') });
    workloadCompleted = true;
  } finally {
    for (const id of sessions) await request.delete(`/api/sessions/${id}`);
    // Only this test's isolated project is stopped; server teardown owns daemon
    // shutdown so other real-Rig tests can use the same fixture.
    const state = path.join(home, 'data', 'coordination');
    const { readdirSync, existsSync } = await import('node:fs');
    for (const scope of readdirSync(state)) {
      const file = path.join(state, scope, project, 'project.json');
      if (!existsSync(file)) continue;
      const team = JSON.parse(readFileSync(file, 'utf8')) as { rigId: string };
      const stopped = spawnSync(path.join(home, 'openrig-bin', 'rig'), ['down', team.rigId], { timeout: 15000, encoding: 'utf8' });
      if (workloadCompleted && !/already stopped/.test(stopped.stdout + stopped.stderr)) {
        expect(stopped.status, stopped.stdout + stopped.stderr).toBe(0);
      }
    }
    const sentinel = spawnSync('tmux', ['has-session', '-t', '=webmux-fixture-path'], {
      env: { ...process.env, TMUX_TMPDIR: readFileSync(path.join(home, 'openrig-tmux-directory'), 'utf8') }, timeout: 5000,
    });
    if (workloadCompleted) expect(sentinel.status).toBe(0);
  }
});

test('real diagnostic dashboard is optional and detaching preserves its daemon', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_OPENRIG !== '1', 'Requires the isolated published-CLI fixture');
  const home = path.resolve(__dirname, '.test-home');
  const daemon = readFileSync(path.join(home, 'openrig-daemon-url'), 'utf8');
  let id: string | null = null;
  try {
    await page.goto('/?panel=openrig');
    const terminal = page.getByTestId('openrig-terminal');
    await page.getByRole('button', { name: 'Connect dashboard', exact: true }).click();
    await expect(terminal.locator('.xterm-rows')).toContainText('EXPLORER');
    await expect(terminal.locator('.xterm-rows')).toContainText('TOPOLOGY');
    id = await terminal.getAttribute('data-session-id');
    await terminal.locator('.xterm-helper-textarea').focus();
    await page.keyboard.type('system');
    await page.keyboard.press('Enter');
    await expect(terminal.locator('.xterm-rows')).toContainText('SYSTEM · HEALTH');
    await page.screenshot({ path: info.outputPath('openrig-real-dashboard.png') });
    await page.reload();
    await expect(terminal).toHaveAttribute('data-session-id', id!);
    expect(await (await request.get('/api/sessions')).json()).toEqual([]);
    await page.getByRole('button', { name: 'Detach dashboard', exact: true }).click();
    await expect(terminal).toHaveCount(0);
    expect((await (await request.get('/api/openrig')).json()).session).toBeNull();
    expect((await request.get(`${daemon}/healthz`)).ok()).toBe(true);
  } finally {
    if (id) await request.delete(`/api/sessions/${id}`);
  }
});


test('coordination groups explicit worker identities in keyboard-expandable projects', async ({ page }) => {
  await page.route('**/api/coordination', route => route.fulfill({ json: { messages: [], agents: [
    { target: 'opaque-one', project: 'demo-release', name: 'reviewer', host: 'build-host', runtime: 'codex', cwd: '/work/demo', connected: true, activityState: 'needs_input' },
    { target: 'opaque-two', project: 'demo-release', name: 'coder', host: 'localhost', runtime: 'claude-code', cwd: '/work/demo', connected: false },
    { target: 'opaque-three', project: 'other-project', name: 'reviewer', host: 'other-host', runtime: 'codex', cwd: '/work/other', connected: true },
  ] } }));
  await page.goto('/');
  await page.getByRole('button', { name: 'Rig', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Rig', exact: true });
  const workers = dialog.getByRole('list', { name: 'demo-release workers' });
  await expect(workers.getByRole('article')).toHaveCount(2);
  await expect(workers.getByText('build-host', { exact: true })).toBeVisible();
  await expect(workers.getByText('needs_input')).toBeVisible();
  const summary = dialog.locator('summary').filter({ hasText: 'demo-release' });
  await summary.focus();
  await page.keyboard.press('Enter');
  await expect(workers).not.toBeVisible();
  await page.keyboard.press('Space');
  await expect(workers).toBeVisible();
  await expect(workers.getByRole('button', { name: 'Open terminal' })).toHaveCount(1);
  await expect(workers.getByRole('button', { name: 'Reconnect agent' })).toHaveCount(1);
  await page.setViewportSize({ width: 390, height: 800 });
  expect(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
});
