import { test, expect } from '@playwright/test';
import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { randomBytes } from 'node:crypto';
import { spawnSync } from 'node:child_process';

test('home gives a fresh 2 by 2 terminal grid the page without coordinator inventory', async ({ page }) => {
  let coordinationRequests = 0;
  await page.route('**/api/sessions', route => route.fulfill({ json: [] }));
  page.on('request', request => { if (request.url().endsWith('/api/coordination')) coordinationRequests++; });
  await page.goto('/');
  for (const [row, col] of [[0, 0], [0, 1], [1, 0], [1, 1]]) await expect(page.getByTestId(`add-cell-${row}-${col}`).first()).toBeVisible();
  await expect(page.getByText(/Coordinated sessions \(/)).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'USB redirection' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Settings', exact: true })).toHaveCount(0);
  expect(coordinationRequests).toBe(0);
});

test.afterAll(() => {
  if (process.env.WEBMUX_REAL_OPENRIG !== '1') return;
  const executable = path.resolve(__dirname, '.test-home', 'openrig-bin', 'rig');
  if (!existsSync(executable)) return;
  // Stop before Playwright terminates the web-server process group. Waiting for
  // the server's exit handler alone can leave the detached Rig daemon behind.
  const stopped = spawnSync(executable, ['daemon', 'stop'], { timeout: 15000, encoding: 'utf8' });
  expect(stopped.status, stopped.stdout + stopped.stderr).toBe(0);
});

test('coordination lives in its own tab and leaves the terminal grid intact', async ({ page, request }) => {
  const original = await (await request.get('/api/sessions')).json();
  let attachments = 0;
  page.on('request', value => { if (value.method() === 'POST' && /coordination|openrig/.test(value.url())) attachments++; });
  await page.goto('/');
  await page.getByRole('button', { name: 'Coordination', exact: true }).click();
  const dialog = page.getByRole('region', { name: 'Coordination', exact: true });
  await expect(dialog).toBeVisible();
  const experience = dialog.getByLabel('Coordinator');
  if (await experience.locator('option[value="rig"]').count()) {
    await experience.selectOption('rig');
    await dialog.getByRole('button', { name: 'Use this time' }).click();
    await expect(dialog.getByRole('heading', { name: 'Start a coding agent', exact: true })).toBeVisible();
    await expect(dialog.getByLabel('Code folder')).toBeVisible();
  } else {
    await expect(experience).toBeVisible();
  }
  await expect(page.getByRole('button', { name: 'Run fresh challenge' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Terminals', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByTestId('workspace-scroll')).toBeVisible();
  expect(attachments).toBe(0);
  expect(await (await request.get('/api/sessions')).json()).toEqual(original);
});

test('Agent Deck UI queues work and drains remote completion records', async ({ page }) => {
  const status = {
    contract: 'webmux-coordination/v1', defaultProvider: 'agent-deck', errors: {}, attentionCount: 1,
    providers: [{ id: 'agent-deck', label: 'Agent Deck', available: true, contract: 'agent-deck-cli/v1', capabilities: ['sessions', 'remote', 'parent-child', 'send', 'queue', 'send-status', 'completion-inbox', 'remote-drain'] }],
    projects: [
      { provider: 'agent-deck', id: 'local:webmux', name: 'webmux', host: 'localhost', state: 'active', historical: false, seatCount: 1, runningCount: 1, attentionCount: 1 },
      { provider: 'agent-deck', id: 'sparky:webmux', name: 'webmux', host: 'sparky', state: 'active', historical: false, seatCount: 1, runningCount: 1 },
    ],
    seats: [
      { provider: 'agent-deck', id: 'local:conductor', projectId: 'local:webmux', projectName: 'webmux', name: 'conductor', host: 'localhost', runtime: 'codex', state: 'waiting', openable: true, managed: true, role: 'conductor', attention: true, canReceive: true },
      { provider: 'agent-deck', id: 'sparky:worker', projectId: 'sparky:webmux', projectName: 'webmux', name: 'reviewer', host: 'sparky', runtime: 'claude-code', state: 'running', openable: true, managed: true, role: 'worker', parentId: 'conductor', parentName: 'conductor', canReceive: true },
    ],
    messages: [{ provider: 'agent-deck', from: 'reviewer', to: 'conductor', body: 'Remote review completed', time: '2026-10-26T10:00:00Z', state: 'completed', kind: 'completion', sourceRemote: 'sparky' }],
  };
  let sent: unknown;
  let drained: unknown;
  await page.route('**/api/coordination', route => route.fulfill({ json: status }));
  await page.route('**/api/coordination/messages', async route => {
    sent = route.request().postDataJSON();
    await route.fulfill({ json: { target: 'sparky:worker', sendId: 'send-1', state: 'queued', queued: true, submitted: true } });
  });
  await page.route('**/api/coordination/send-status', route => route.fulfill({ json: { state: 'landed', confirmation: 'accepted', submitted: true } }));
  await page.route('**/api/coordination/drain', async route => {
    drained = route.request().postDataJSON();
    await route.fulfill({ json: { remote: 'sparky', target: 'local:conductor', fetched: 2, written: 1, duplicates: 1, unknown: 0, woke: true, messages: [] } });
  });

  await page.goto('/');
  await expect(page.getByRole('button', { name: 'Coordination', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Coordination', exact: true }).click();
  const dialog = page.getByRole('region', { name: 'Coordination', exact: true });
  const worker = dialog.getByRole('article', { name: 'reviewer in webmux' });
  await expect(worker).toContainText('Parentconductor');
  await worker.getByRole('button', { name: 'Send instruction' }).click();
  await worker.getByLabel('Instruction for reviewer').fill('Review the authentication changes');
  await worker.getByRole('button', { name: 'Queue instruction' }).click();
  await expect(worker.getByRole('status')).toContainText('queued');
  expect(sent).toEqual({ provider: 'agent-deck', target: 'sparky:worker', message: 'Review the authentication changes', queue: true });
  await dialog.getByRole('button', { name: 'Collect completions' }).click();
  await expect(dialog.getByText(/Collected 1 new of 2 records/)).toBeVisible();
  expect(drained).toEqual({ provider: 'agent-deck', remote: 'sparky', target: 'local:conductor' });
  await dialog.getByText(/Agent Deck activity/).click();
  await expect(dialog.getByText('Remote review completed')).toBeVisible();
});

test('managed agents enroll, exchange attributed Rig messages and reuse ordinary grid terminals', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_OPENRIG !== '1', 'Requires the isolated published-CLI fixture');
  test.setTimeout(120_000);
  page.setDefaultTimeout(10_000);
  const home = path.resolve(__dirname, '.test-home');
  const project = 'browser-workload';
  const sessions: string[] = [];
  let workloadCompleted = false;
  type Agent = { name: string; id: string; state: string; openable: boolean; nativeSessionId?: string };
  type Session = { id: string; row: number; col: number; openrig_view?: string; rig?: { target: string; project: string; name: string; host: string; runtime: string } };
  const status = async () => (await (await request.get('/api/coordination')).json()) as {
    seats: Agent[]; messages: { from: string; to: string; body: string }[];
  };
  const useRig = async (dialog: ReturnType<typeof page.getByRole>) => {
    await expect(dialog.locator('.coordination-current')).toBeVisible();
    await dialog.getByLabel('Coordinator').selectOption('rig');
    await dialog.getByRole('button', { name: 'Use this time' }).click();
    return dialog;
  };
  const openDialog = async () => {
    await page.getByRole('button', { name: 'Coordination', exact: true }).click();
    return useRig(page.getByRole('region', { name: 'Coordination', exact: true }));
  };
  const openAgent = async (name: string, reconnect = false) => {
    const dialog = await openDialog();
    const endpoint = reconnect ? '/api/coordination/agents' : '/api/coordination/attach';
    const response = page.waitForResponse(value => value.url().endsWith(endpoint) && value.request().method() === 'POST');
    await dialog.locator('article').filter({ has: page.getByText(name, { exact: true }) }).getByRole('button', { name: reconnect ? 'Restart provider and open' : 'Open session', exact: true }).click();
    const attached = await response;
    expect(attached.status()).toBe(200);
    const body = await attached.json() as Session | { session: Session };
    const session = reconnect ? (body as { session: Session }).session : body as Session;
    await expect(dialog).toHaveCount(0);
    return session;
  };
  try {
    await page.goto('/');
    for (const [index, name] of ['coder', 'reviewer'].entries()) {
      let dialog;
      if (index === 0) {
        await page.getByTestId('add-cell-0-0').first().click();
        await page.getByRole('dialog', { name: 'Add to grid' }).getByRole('button', { name: 'Open coordinated session' }).click();
        dialog = await useRig(page.getByRole('dialog', { name: 'Coordination', exact: true }));
        await dialog.getByLabel('Start a new project').check();
      } else {
        await page.getByRole('button', { name: 'Desktops', exact: true }).click();
        dialog = await openDialog();
        await dialog.getByLabel('Add an agent to an existing project').check();
      }
      if (index === 0) await dialog.getByLabel('Project name', { exact: true }).fill(project);
      await dialog.getByLabel('Session name', { exact: true }).fill(name);
      await dialog.getByLabel('Run on host', { exact: true }).fill('localhost');
      await dialog.getByLabel('Code folder', { exact: true }).fill(home);
      await dialog.getByRole('combobox', { name: 'Coding agent', exact: true }).selectOption('codex');
      const response = page.waitForResponse(value => value.url().endsWith('/api/coordination/agents') && value.request().method() === 'POST');
      await dialog.getByRole('button', { name: 'Start agent and open terminal', exact: true }).click();
      expect((await response).status()).toBe(200);
      await expect(dialog).toHaveCount(0);
      const agent = (await status()).seats.find(value => value.name === name)!;
      const listing: Session[] = await (await request.get('/api/sessions')).json();
      const session = listing.find(value => value.openrig_view === `managed:${agent.id}`)!;
      if (index === 0) expect([session.row, session.col]).toEqual([0, 0]);
      sessions.push(session.id);
      expect(session.rig).toEqual({ target: agent.id, project, name, host: 'localhost', runtime: 'codex', workspace: home });
      const terminal = page.getByTestId(`tile-cell-${session.id}`);
      await expect(page.getByTestId('workspace-scroll')).toBeVisible();
      await expect(terminal).toBeInViewport({ ratio: 0.1 });
      await expect(terminal.locator('.xterm-helper-textarea')).toBeFocused();
      await expect(terminal.getByLabel('Rig-managed worker')).toHaveAttribute('title', new RegExp(project));
      await expect(terminal.getByText(`${project} / ${name}`, { exact: true })).toBeVisible();
      await expect(page).toHaveTitle(new RegExp(project));
      await expect(terminal.locator('.xterm-rows')).toContainText('FIXTURE_READY');
      await expect(terminal.locator('.xterm-rows')).toContainText(agent.id);
    }
    const agents = (await status()).seats;
    const coder = agents.find(value => value.name === 'coder')!;
    const reviewer = agents.find(value => value.name === 'reviewer')!;
    expect(agents).toHaveLength(2);
    expect(agents.every(value => value.openable)).toBe(true);
    const tmuxEnv = { ...process.env, TMUX_TMPDIR: readFileSync(path.join(home, 'openrig-tmux-directory'), 'utf8') };
    const tmux = (...args: string[]) => {
      const result = spawnSync('tmux', args, { env: tmuxEnv, encoding: 'utf8', timeout: 5000 });
      expect(result.status, result.stderr).toBe(0);
      return result.stdout.trim();
    };
    expect(tmux('display-message', '-p', '-t', coder.id, '#{mouse}')).toBe('1');
    expect(tmux('show-option', '-gv', 'mouse')).toBe('off');
    expect(tmux('display-message', '-p', '-t', 'webmux-fixture-path', '#{mouse}')).toBe('0');
    tmux('set-option', '-t', coder.id, 'mouse', 'off');
    expect((await openAgent('coder')).id).toBe(sessions[0]);
    expect(tmux('display-message', '-p', '-t', coder.id, '#{mouse}')).toBe('1');
    tmux('send-keys', '-t', coder.id, 'WM_SCROLL_OLD', 'Enter');
    for (let index = 0; index < 40; index++) tmux('send-keys', '-t', coder.id, `WM_SCROLL_${index}`, 'Enter');
    tmux('send-keys', '-t', coder.id, 'WM_SCROLL_NEW', 'Enter');
    const scrollTile = page.getByTestId(`tile-cell-${sessions[0]}`);
    const rows = scrollTile.locator('.xterm-rows');
    await expect(rows).toContainText('WM_SCROLL_NEW');
    await expect(rows).not.toContainText('WM_SCROLL_OLD');
    const workspace = page.getByTestId('workspace-scroll');
    const beforeWheel = await workspace.evaluate(el => ({ x: el.scrollLeft, y: el.scrollTop }));
    const screen = scrollTile.locator('.xterm-screen');
    const box = (await screen.boundingBox())!;
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    for (let index = 0; index < 20; index++) await page.mouse.wheel(0, -120);
    await expect.poll(() => tmux('display-message', '-p', '-t', coder.id, '#{pane_in_mode}')).toBe('1');
    await expect(rows).toContainText('WM_SCROLL_OLD');
    expect(await workspace.evaluate(el => ({ x: el.scrollLeft, y: el.scrollTop }))).toEqual(beforeWheel);
    tmux('send-keys', '-t', coder.id, 'q');
    const nonce = randomBytes(16).toString('hex');
    const terminal = page.getByTestId(`tile-cell-${sessions[0]}`);
    await terminal.locator('.xterm-helper-textarea').focus();
    await page.keyboard.insertText(`fixture-send ${reviewer.id} WM_CHALLENGE:${coder.id}:${nonce}`);
    await page.keyboard.press('Enter');
    // The ACK is generated by the peer only after receiving Rig's tmux delivery.
    await expect(terminal.locator('.xterm-rows')).toContainText(`WM_ACK:${nonce}`, { timeout: 30_000 });
    await expect.poll(async () => (await status()).messages.some(value => value.from === reviewer.id && value.to === coder.id && value.body.includes(`WM_ACK:${nonce}`))).toBe(true);
    const dialog = await openDialog();
    await expect(dialog).toContainText(`WM_ACK:${nonce}`);
    await page.keyboard.press('Escape');
    await page.reload();
    for (const id of sessions) await expect(page.getByTestId(`tile-cell-${id}`)).toBeVisible();
    expect((await status()).seats.map(value => value.id)).toEqual(agents.map(value => value.id));
    expect((await openAgent('coder')).id).toBe(sessions[0]);
    expect((await (await request.get('/api/sessions')).json()).map((value: Session) => value.id)).toEqual(sessions);

    // An existing viewer outside the visible grid must actually be revealed.
    const moved = await request.patch(`/api/sessions/${sessions[0]}`, { data: { row: 2, col: 2 } });
    expect(moved.ok()).toBe(true);
    await page.reload();
    await page.getByTestId('workspace-scroll').evaluate(el => el.scrollTo(0, 0));
    await expect(terminal).not.toBeInViewport();
    expect((await openAgent('coder')).id).toBe(sessions[0]);
    await expect(terminal).toBeInViewport({ ratio: 0.1 });
    await expect(terminal.locator('.xterm-helper-textarea')).toBeFocused();

    // Restoring a minimized viewer must keep its identity and avoid another tile.
    await terminal.getByTitle('Minimize', { exact: true }).click();
    await expect(terminal).toBeHidden();
    const occupied = await request.patch(`/api/sessions/${sessions[1]}`, { data: { row: 2, col: 2 } });
    expect(occupied.ok()).toBe(true);
    await page.reload();
    await expect(terminal).toBeHidden();
    const restored = await openAgent('coder');
    expect(restored.id).toBe(sessions[0]);
    expect([restored.row, restored.col]).not.toEqual([2, 2]);
    await expect(terminal).toBeInViewport({ ratio: 0.1 });
    await expect(terminal.locator('.xterm-helper-textarea')).toBeFocused();
    expect((await status()).seats).toHaveLength(2);

    // Close the actual tile, then reopen the same agent without launching a third.
    await terminal.getByTitle('Detach viewer; seat keeps running', { exact: true }).click();
    await expect(terminal).toHaveCount(0);
    expect((await status()).seats.every(value => value.state !== 'stopped')).toBe(true);
    const coderSession = await openAgent('coder');
    sessions.push(coderSession.id);
    await expect(page.getByTestId(`tile-cell-${coderSession.id}`).locator('.xterm-rows')).toContainText(`WM_ACK:${nonce}`);
    expect((await status()).seats).toHaveLength(2);
    let previousId = (await status()).seats.find(value => value.id === coder.id)!.nativeSessionId;
    expect(previousId).toBeTruthy();
    for (let reconnect = 0; reconnect < 2; reconnect++) {
      const stopped = spawnSync('tmux', ['kill-session', '-t', `=${coder.id}`], { env: tmuxEnv, timeout: 5000 });
      expect(stopped.status).toBe(0);
      await expect.poll(async () => (await status()).seats.find(value => value.id === coder.id)!.state).toBe('stopped');
      expect((await openAgent('coder', true)).id).toBe(coderSession.id);
      await expect(page.getByTestId(`tile-cell-${coderSession.id}`).locator('.xterm-rows')).toContainText('FIXTURE_READY');
      const current = (await status()).seats.find(value => value.id === coder.id)!;
      expect(current.state !== 'stopped').toBe(true);
      expect(current.nativeSessionId).toBeTruthy();
      expect(current.nativeSessionId).not.toBe(previousId);
      previousId = current.nativeSessionId;
      expect((await status()).seats).toHaveLength(2);
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


test('coordination groups provider-owned sessions in keyboard-expandable projects', async ({ page }) => {
  await page.route('**/api/coordination', route => route.fulfill({ json: {
    contract: 'webmux-coordination/v1',
    defaultProvider: 'rig',
    providers: [{ id: 'rig', label: 'Rig', available: true, capabilities: ['projects', 'attach'], contract: 'openrig-cli/0.6.14' }],
    projects: [
      { provider: 'rig', id: 'rig-one', name: 'demo-release', host: 'localhost', state: 'running', historical: false, seatCount: 2, runningCount: 1, contexts: ['agents'] },
      { provider: 'rig', id: 'rig-two', name: 'other-project', host: 'other-host', state: 'running', historical: false, seatCount: 1, runningCount: 1, contexts: ['agents'] },
    ],
    seats: [
      { provider: 'rig', id: 'opaque-one', projectId: 'rig-one', projectName: 'demo-release', name: 'reviewer', host: 'build-host', runtime: 'codex', workspace: '/work/demo', state: 'waiting', openable: true, managed: true, context: 'agents' },
      { provider: 'rig', id: 'opaque-two', projectId: 'rig-one', projectName: 'demo-release', name: 'coder', host: 'localhost', runtime: 'claude-code', workspace: '/work/demo', state: 'stopped', openable: false, managed: true, context: 'agents' },
      { provider: 'rig', id: 'opaque-three', projectId: 'rig-two', projectName: 'other-project', name: 'reviewer', host: 'other-host', runtime: 'codex', workspace: '/work/other', state: 'running', openable: true, managed: false, context: 'agents' },
    ],
    messages: [], errors: {},
  } }));
  await page.goto('/');
  await page.getByRole('button', { name: 'Coordination', exact: true }).click();
  const dialog = page.getByRole('region', { name: 'Coordination', exact: true });
  const workers = dialog.getByRole('list', { name: 'demo-release sessions' });
  await expect(workers.getByRole('article')).toHaveCount(2);
  await expect(workers.getByText('build-host', { exact: true })).toBeVisible();
  await expect(workers.getByText('waiting')).toBeVisible();
  const summary = dialog.locator('summary').filter({ hasText: 'demo-release' });
  await summary.focus();
  await page.keyboard.press('Enter');
  await expect(workers).not.toBeVisible();
  await page.keyboard.press('Space');
  await expect(workers).toBeVisible();
  await expect(workers.getByRole('button', { name: 'Open session' })).toHaveCount(1);
  await expect(workers.getByRole('button', { name: 'Restart provider and open' })).toHaveCount(1);
  await page.setViewportSize({ width: 390, height: 800 });
  expect(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
});
