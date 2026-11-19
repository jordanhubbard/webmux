import { test, expect } from '@playwright/test';
import { spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { mkdtempSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { captureBoth } from './visual-capture';

test.use({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 1, locale: 'en-US', timezoneId: 'UTC', colorScheme: 'dark', contextOptions: { reducedMotion: 'reduce' } });


test('real tmux discovery, browser attachment, reload and scratch shell', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_AGENTS !== '1', 'Requires an isolated Unix tmux fixture');
  test.setTimeout(60000);
  if (process.platform === 'win32') throw new Error('The real tmux fixture requires Unix');
  const directory = mkdtempSync(path.join(tmpdir(), 'wm-agent-'));
  const socket = path.join(directory, 'tmux');
  const config = path.resolve(__dirname, '.test-home/config/app.yaml');
  const original = readFileSync(config, 'utf8');
  const replaceConfig = (text: string) => {
    writeFileSync(`${config}.agent-test`, text, { mode: 0o600 });
    renameSync(`${config}.agent-test`, config);
  };
  const tmux = (...args: string[]) => {
    const result = spawnSync('tmux', ['-S', socket, '-f', '/dev/null', ...args], { encoding: 'utf8', timeout: 5000 });
    if (result.error) throw result.error;
    expect(result.status, result.stderr).toBe(0);
    return result.stdout.trim();
  };
  const home = path.resolve(__dirname, '.test-home');
  const rig = path.join(home, 'openrig-bin/rig');
  const rigEnv = process.env.WEBMUX_REAL_OPENRIG === '1' ? { ...process.env, OPENRIG_HOST_SELECTED: 'local',
    TMUX_TMPDIR: readFileSync(path.join(home, 'openrig-tmux-directory'), 'utf8') } : process.env;
  const rigRun = (binary: string, args: string[]) => {
    const result = spawnSync(binary, args, { env: rigEnv, encoding: 'utf8', timeout: 15000 });
    expect(result.status, result.stdout + result.stderr).toBe(0);
    return result.stdout.trim();
  };
  let rigProject = '', rigSeat = '', rigViewer = '', rigTarget = '', rigPID = '', observedPID = '';
  type AgentSession = { id: string; agent_id?: string; agent_role?: string; exec_cwd?: string };
  const ids = new Set<string>();
  const responseFor = (role: 'attach' | 'scratch') => page.waitForResponse(response =>
    response.request().method() === 'POST' && response.url().endsWith(`/api/agents/fixture/${role}`));
  const sessionFrom = async (pending: ReturnType<typeof responseFor>) => {
    const response = await pending;
    expect(response.ok()).toBe(true);
    const session = await response.json() as AgentSession;
    ids.add(session.id);
    expect(session.agent_id).toBe('fixture');
    return session;
  };
  let failed = false;
  try {
    const agents = { enabled: true, combined_pane: true, disable_in_multi_user_mode: true,
      definitions: [{ id: 'fixture', label: 'Fixture', plural_label: 'Fixtures', badge: 'TEST',
        tmux_socket: socket, workspace: 'agents', enabled: true }] };
    const section = /^  agents:\n(?: {4}.*\n)+/m;
    expect(original).toMatch(section);
    replaceConfig(original.replace(section, `  agents: ${JSON.stringify(agents)}\n`));
    if (process.env.WEBMUX_REAL_OPENRIG === '1') {
      const project = `coexist-${randomBytes(5).toString('hex')}`;
      const spec = path.join(home, `${project}.json`);
      writeFileSync(spec, JSON.stringify({ version: '0.2', name: project, pods: [{ id: 'agents', label: project,
        members: [{ id: 'native', label: 'Native seat', agent_ref: 'builtin:terminal', profile: 'none', runtime: 'terminal', cwd: home }] }], edges: [] }));
      rigProject = (JSON.parse(rigRun(rig, ['up', spec, '--yes', '--json'])) as { rigId: string }).rigId;
      const inventory = await (await request.get('/api/coordination?provider=rig')).json() as {
        seats: { id: string; projectId: string; logicalId: string; attachmentTarget: string }[] };
      const seat = inventory.seats.find(value => value.projectId === rigProject && value.logicalId === 'agents.native');
      expect(seat).toBeTruthy(); rigSeat = seat!.id; rigTarget = seat!.attachmentTarget;
      rigPID = rigRun('tmux', ['display-message', '-p', '-t', rigTarget, '#{pane_pid}']);
      const opened = await request.post('/api/coordination/attach', { data: { provider: 'rig', target: rigSeat, row: 1, col: 1 } });
      expect(opened.status()).toBe(200); rigViewer = (await opened.json() as { id: string }).id;
      ids.add(rigViewer);
    }
    await page.goto('/');
    await page.getByRole('button', { name: 'Agents', exact: true }).click();
    await expect(page.getByRole('region', { name: 'Session ownership and activity' })).toContainText('do not verify Rig ownership or readiness');
    await expect(page.getByText('No agent sessions', { exact: true })).toBeVisible();
    await expect(page.getByText('No session selected', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: '+ Shell', exact: true })).toBeDisabled();
    if (process.env.WEBMUX_VISUAL_PARITY === '1') {
      await page.mouse.move(0, 0);
      await captureBoth(page, info, 'agents-empty');
    }

    tmux('new-session', '-d', '-s', 'fixture-task', '-c', directory,
      process.execPath, path.resolve(__dirname, 'visual-terminal-fixture.mts'));
    // Keep fixture output deterministic; tmux's default status bar includes time
    // and hostname, unrelated to the application's rendering.
    tmux('set-option', '-g', 'status', 'off');
    const attaching = responseFor('attach');
    await page.getByTitle('Refresh Agents', { exact: true }).click();
    const attached = await sessionFrom(attaching);
    expect(attached.agent_role).toBe('attach');
    const providerPID = tmux('display-message', '-p', '-t', 'fixture-task', '#{pane_pid}');
    observedPID = providerPID;
    const layout = page.getByTestId('agents-layout');
    const rows = layout.locator('.xterm-rows').first();
    await expect(page.locator('[data-session-key="fixture:fixture-task"]')).toBeVisible();
    await expect(rows).toContainText('fixture ready');
    await expect(layout.getByText('Observed tmux', { exact: true })).toBeVisible();
    await expect(page.getByTestId('agent-session-row')).toContainText(/Hook report|WebMux report|tmux activity estimate|Source unknown/);
    expect(tmux('display-message', '-p', '-t', 'fixture-task', '#{session_attached}')).toBe('1');

    await layout.locator('.xterm-helper-textarea').first().focus();
    await page.keyboard.type('agent-browser-input');
    await page.keyboard.press('Enter');
    await expect(rows).toContainText('input: agent-browser-input');

    // Navigation reveals the primary inventory/grid without recreating the
    // observed provider or relabeling a configured tmux group as Rig ownership.
    await page.getByRole('button', { name: 'Coordinated sessions', exact: true }).click();
    await expect(page.getByRole('region', { name: 'Coordination', exact: true })).toBeVisible();
    if (process.env.WEBMUX_REAL_OPENRIG === '1') {
      const status = await (await request.get('/api/coordination?provider=rig')).json();
      expect(status.providers.find((provider: { id: string }) => provider.id === 'rig')).toMatchObject({ enabled: true, available: true });
      expect(status.errors.rig).toBeUndefined();
      const native = status.seats.find((seat: { id: string }) => seat.id === rigSeat);
      expect(native).toMatchObject({ projectId: rigProject, logicalId: 'agents.native', state: 'running', managed: false, enrollmentOrigin: 'launched' });
      expect(attached.id).not.toBe(rigViewer);
      const daemon = readFileSync(path.resolve(__dirname, '.test-home/openrig-daemon-url'), 'utf8');
      expect((await request.get(`${daemon}/healthz`)).ok()).toBe(true);
    }
    await page.getByRole('button', { name: 'Agents', exact: true }).click();
    await expect(rows).toContainText('input: agent-browser-input');
    await page.getByRole('button', { name: 'Terminal grid', exact: true }).click();
    await expect(page.getByTestId('workspace-scroll')).toBeVisible();
    if (rigViewer) {
      await expect(page.getByTestId(`tile-cell-${rigViewer}`).getByLabel('Rig seat status')).toContainText('Rig running · Attachment unknown · Bootstrap ready');
      const nativeViewer = await (await request.get(`/api/sessions/${rigViewer}`)).json();
      expect([nativeViewer.id, nativeViewer.row, nativeViewer.col, nativeViewer.rig.seat_id]).toEqual([rigViewer, 1, 1, rigSeat]);
      expect(rigRun('tmux', ['display-message', '-p', '-t', rigTarget, '#{pane_pid}'])).toBe(rigPID);
    }
    await page.getByRole('button', { name: 'Agents', exact: true }).click();
    await expect(rows).toContainText('input: agent-browser-input');
    expect(tmux('display-message', '-p', '-t', 'fixture-task', '#{pane_pid}')).toBe(providerPID);
    expect(tmux('display-message', '-p', '-t', 'fixture-task', '#{session_attached}')).toBe('1');
    const identity = await (await request.get(`/api/sessions/${attached.id}`)).json();
    expect(identity.agent_role).toBe('attach');
    expect(identity.rig).toBeUndefined();
    await page.screenshot({ path: info.outputPath('observed-tmux-pane.png') });
    if (process.env.WEBMUX_VISUAL_PARITY === '1') {
      await page.mouse.move(0, 0);
      await captureBoth(page, info, 'agents-active');
    }

    const reattaching = responseFor('attach');
    await page.reload();
    await page.getByRole('button', { name: 'Agents', exact: true }).click();
    expect((await sessionFrom(reattaching)).id).toBe(attached.id);
    await expect(rows).toContainText('input: agent-browser-input');
    expect(tmux('display-message', '-p', '-t', 'fixture-task', '#{session_attached}')).toBe('1');

    const opening = responseFor('scratch');
    await page.getByTitle('Open scratch shell', { exact: true }).click();
    const scratch = await sessionFrom(opening);
    expect(scratch.agent_role).toBe('scratch');
    const panePath = tmux('display-message', '-p', '-t', 'fixture-task', '#{pane_current_path}');
    expect(scratch.exec_cwd).toBe(panePath);
    await expect(layout.locator('.xterm-helper-textarea')).toHaveCount(2);
    await layout.locator('.xterm-helper-textarea').nth(1).focus();
    const quotedPath = `'${panePath.replaceAll("'", "'\\''")}'`;
    await page.keyboard.type(`test "$(pwd -P)" = ${quotedPath} && printf '\\nSCRATCH_%s\\n' READY`);
    await page.keyboard.press('Enter');
    await expect(layout.locator('.xterm-rows').nth(1)).toContainText('SCRATCH_READY');
    await page.getByTitle('Close scratch shell', { exact: true }).click();
    await expect.poll(async () => (await request.get(`/api/sessions/${scratch.id}`)).status()).toBe(404);
    ids.delete(scratch.id);
    await expect(layout.locator('.xterm-helper-textarea')).toHaveCount(1);
    const providerSessions = tmux('list-sessions', '-F', '#{session_name}').split('\n');
    expect(providerSessions).toEqual(['fixture-task']);
    if (rigViewer) {
      const nativeViewer = await (await request.get(`/api/sessions/${rigViewer}`)).json();
      expect([nativeViewer.id, nativeViewer.row, nativeViewer.col, nativeViewer.rig.seat_id]).toEqual([rigViewer, 1, 1, rigSeat]);
      expect(rigRun('tmux', ['display-message', '-p', '-t', rigTarget, '#{pane_pid}'])).toBe(rigPID);
    }
    const receipt = { activeRigSeat: Boolean(rigSeat), rigViewerPreserved: Boolean(rigViewer), rigProviderPIDUnchanged: Boolean(rigPID), platform: process.platform, node: process.version, browser: page.context().browser()?.version(), tmux: tmux('-V'), originalPID: providerPID,
      finalPID: tmux('display-message', '-p', '-t', 'fixture-task', '#{pane_pid}'), observedViewer: attached.id, viewerReusedAfterReload: true,
      coordinationAndGridNavigation: true, noRigIdentityInferred: true, scratchClosed: true,
      liveRigInventory: process.env.WEBMUX_REAL_OPENRIG === '1', providerSessions };
    expect(receipt.finalPID).toBe(providerPID);
    const receiptPath = info.outputPath('observed-tmux-provenance.json');
    writeFileSync(receiptPath, JSON.stringify(receipt));
    await info.attach('observed tmux lifecycle', { path: receiptPath, contentType: 'application/json' });
  } catch (error) { failed = true; throw error; }
  finally {
    try {
      await page.close();
      for (const id of ids) {
        const removed = await request.delete(`/api/sessions/${id}`);
        expect([204, 404]).toContain(removed.status());
      }
    } catch (error) { if (!failed) throw error; }
    finally {
      try {
        try {
          if (!failed && observedPID) expect(tmux('display-message', '-p', '-t', 'fixture-task', '#{pane_pid}')).toBe(observedPID);
          if (!failed && rigPID) expect(rigRun('tmux', ['display-message', '-p', '-t', rigTarget, '#{pane_pid}'])).toBe(rigPID);
        } finally {
          if (rigProject) rigRun(rig, ['down', rigProject]);
        }
      } finally {
        try { replaceConfig(original); }
        finally {
          // The absolute socket is private to this fixture; never touch a default server.
          spawnSync('tmux', ['-S', socket, 'kill-server'], { timeout: 5000 });
          rmSync(directory, { recursive: true, force: true });
        }
      }
    }
  }
});
