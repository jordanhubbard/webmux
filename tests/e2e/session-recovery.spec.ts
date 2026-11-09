import { test, expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import path from 'node:path';

test('local recovery preserves a surviving job and replays an interrupted command once after approval', async ({ page, request }) => {
  test.skip(process.platform === 'win32', 'Native tmux lifecycle qualification requires Unix');
  test.setTimeout(90_000);
  const previous = (await (await request.get('/api/config')).json()).app.session_logging?.enabled ?? false;
  expect((await request.put('/api/config', { data: { app: { session_logging: { enabled: true } } } })).ok()).toBe(true);
  const created = await request.post('/api/sessions', { data: { transport: 'local', cols: 100, rows: 30 } });
  expect(created.ok()).toBe(true);
  const session = await created.json();
  const target = `webmux-${session.id}`;
  const marker = `RECOVERY_MARKER_${session.id}`;
  const executions = path.resolve(__dirname, '.test-home', `recovery-executions-${session.id}.txt`);
  const panePid = () => execFileSync('tmux', ['display-message', '-p', '-t', target, '#{pane_pid}'], { encoding: 'utf8' }).trim();
  const count = () => { try { return readFileSync(executions, 'utf8').trim().split('\n').length; } catch { return 0; } };
  const recovery = async () => (await request.get(`/api/session-recovery/${session.id}`)).json();
  try {
    await page.goto('/');
    const tile = page.getByTestId(`tile-cell-${session.id}`);
    await expect(tile.locator('.xterm-screen')).toBeVisible();
    await expect.poll(async () => (await recovery()).shell_ready).toBe(true);
    const originalPid = panePid();
    const code = `require('node:fs').appendFileSync(${JSON.stringify(executions)}, 'execution\\n'); console.log(${JSON.stringify(marker)}); setInterval(() => {}, 1000)`;
    const quote = (value: string) => "'" + value.replaceAll("'", "'\\''") + "'";
    const command = [process.execPath, '-e', code].map(quote).join(' ');
    await tile.locator('.xterm-helper-textarea').focus();
    await page.keyboard.type(command);
    await page.keyboard.press('Enter');
    await expect.poll(count).toBe(1);
    await expect.poll(async () => (await recovery()).state?.activity?.command ?? '').toContain(marker);

    // Client replacement must retain the original process and running job.
    expect((await request.post(`/api/sessions/${session.id}/reconnect`, { data: {} })).ok()).toBe(true);
    await expect.poll(panePid).toBe(originalPid);
    await expect.poll(async () => (await recovery()).state?.disposition).toBe('reattached');
    expect(count()).toBe(1);

    // Destroy only this test's provider. Reconnect must wait for a choice.
    execFileSync('tmux', ['kill-session', '-t', target]);
    await expect.poll(async () => (await (await request.get(`/api/sessions/${session.id}`)).json()).state).toBe('disconnected');
    expect((await request.post(`/api/sessions/${session.id}/reconnect`, { data: {} })).ok()).toBe(true);
    await expect.poll(async () => (await recovery()).waiting).toBe(true);
    await expect.poll(async () => (await recovery()).history).toContain(marker);
    expect(count()).toBe(1);
    const replacementPid = panePid();
    expect(replacementPid).not.toBe(originalPid);
    await page.reload();
    await tile.getByRole('button', { name: 'Recover work', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Recover work', exact: true });
    await expect(dialog.getByLabel('Previous terminal output')).toContainText(marker);
    const run = dialog.getByRole('button', { name: 'Run recorded command', exact: true });
    await expect(run).toBeDisabled();
    expect(count()).toBe(1);
    await dialog.getByLabel(/I approve running the displayed command again/).check();
    await page.screenshot({ path: 'test-results/session-recovery.png' });
    const actionResponse = page.waitForResponse(response => response.url().endsWith(`/api/session-recovery/${session.id}`) && response.request().method() === 'POST');
    await run.click();
    const response = await actionResponse;
    expect(response.ok(), await response.text()).toBe(true);
    const action = response.request().postDataJSON();
    await expect.poll(count).toBe(2);
    // Repeating the exact request is a receipt lookup, not another execution.
    expect((await request.post(`/api/session-recovery/${session.id}`, { data: action })).ok()).toBe(true);
    expect((await request.post(`/api/sessions/${session.id}/reconnect`, { data: {} })).ok()).toBe(true);
    await expect.poll(panePid).toBe(replacementPid);
    expect(count()).toBe(2);
  } finally {
    await request.delete(`/api/sessions/${session.id}`).catch(() => {});
    try { execFileSync('tmux', ['kill-session', '-t', target], { stdio: 'ignore' }); } catch { /* owned provider already stopped */ }
    await request.put('/api/config', { data: { app: { session_logging: { enabled: previous } } } }).catch(() => {});
  }
});
