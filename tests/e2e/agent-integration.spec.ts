import { test, expect } from '@playwright/test';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';

test('real tmux discovery, browser attachment, reload and scratch shell', async ({ page, request }) => {
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
    tmux('new-session', '-d', '-s', 'fixture-task', '-c', directory,
      process.execPath, path.resolve(__dirname, 'visual-terminal-fixture.mts'));
    const agents = { enabled: true, combined_pane: true, disable_in_multi_user_mode: true,
      definitions: [{ id: 'fixture', label: 'Fixture', plural_label: 'Fixtures', badge: 'TEST',
        tmux_socket: socket, workspace: 'agents', enabled: true }] };
    const section = /^  agents:\n(?: {4}.*\n)+/m;
    expect(original).toMatch(section);
    replaceConfig(original.replace(section, `  agents: ${JSON.stringify(agents)}\n`));
    const attaching = responseFor('attach');
    await page.goto('/');
    await page.getByRole('button', { name: 'Agents', exact: true }).click();
    const attached = await sessionFrom(attaching);
    expect(attached.agent_role).toBe('attach');
    const layout = page.getByTestId('agents-layout');
    const rows = layout.locator('.xterm-rows').first();
    await expect(page.locator('[data-session-key="fixture:fixture-task"]')).toBeVisible();
    await expect(rows).toContainText('fixture ready');
    expect(tmux('display-message', '-p', '-t', 'fixture-task', '#{session_attached}')).toBe('1');
    await layout.locator('.xterm-helper-textarea').first().focus();
    await page.keyboard.type('agent-browser-input');
    await page.keyboard.press('Enter');
    await expect(rows).toContainText('input: agent-browser-input');

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
      try { replaceConfig(original); }
      finally {
        // The absolute socket is private to this fixture; never touch a default server.
        spawnSync('tmux', ['-S', socket, 'kill-server'], { timeout: 5000 });
        rmSync(directory, { recursive: true, force: true });
      }
    }
  }
});
