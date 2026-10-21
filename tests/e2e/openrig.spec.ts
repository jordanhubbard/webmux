import { test, expect } from '@playwright/test';
import { spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import path from 'node:path';

test('OpenRig is on demand and can open as an authenticated panel', async ({ page, request }) => {
  const original = await (await request.get('/api/sessions')).json();
  let attachments = 0;
  page.on('request', value => { if (value.url().endsWith('/api/openrig/attach')) attachments++; });
  await page.goto('/');
  const checking = page.waitForResponse(value => value.url().endsWith('/api/openrig') && value.request().method() === 'GET');
  await page.getByRole('button', { name: 'OpenRig', exact: true }).click();
  expect((await checking).status()).toBe(200);
  await expect(page.getByRole('region', { name: 'OpenRig dashboard' })).toBeVisible();
  await expect(page.getByText('Agent coordination alongside your terminals')).toBeVisible();
  expect(attachments).toBe(0);
  const popup = page.waitForEvent('popup');
  await page.getByRole('button', { name: 'Pop out', exact: true }).click();
  const poppedOut = await popup;
  await expect(poppedOut).toHaveURL(/panel=openrig/);
  await expect(poppedOut.getByRole('region', { name: 'OpenRig dashboard' })).toBeVisible();
  await expect(poppedOut.getByRole('button', { name: 'Terminals', exact: true })).toHaveCount(0);
  await poppedOut.close();
  await page.getByRole('button', { name: 'Terminals', exact: true }).click();
  await expect(page.getByTestId('workspace-scroll')).toBeVisible();
  expect(await (await request.get('/api/sessions')).json()).toEqual(original);
});

test('published OpenRig CLI attaches once, shares views and leaves its tmux session alive', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_OPENRIG !== '1', 'Requires the isolated published-CLI fixture');
  const home = path.resolve(__dirname, '.test-home');
  const socket = readFileSync(path.join(home, 'openrig-tmux-socket'), 'utf8');
  const tmuxAlive = () => spawnSync('tmux', ['-S', socket, 'has-session', '-t', '=webmux-openrig-dashboard'], { timeout: 2000 }).status;
  const terminal = page.getByTestId('openrig-terminal');
  let id: string | null = null;
  try {
    await page.goto('/');
    await page.getByRole('button', { name: 'OpenRig', exact: true }).click();
    await page.getByRole('button', { name: 'Connect dashboard', exact: true }).click();
    await expect(terminal.locator('.xterm-rows')).toContainText('fixture ready');
    id = await terminal.getAttribute('data-session-id');
    await terminal.locator('.xterm-helper-textarea').focus();
    await page.keyboard.type('dashboard-input');
    await page.keyboard.press('Enter');
    await expect(terminal.locator('.xterm-rows')).toContainText('input: dashboard-input');
    await page.screenshot({ path: info.outputPath('openrig-prototype.png') });
    await page.getByRole('button', { name: 'Terminals', exact: true }).click();
    expect(tmuxAlive()).toBe(0);
    await page.getByRole('button', { name: 'OpenRig', exact: true }).click();
    await expect(terminal).toHaveAttribute('data-session-id', id!);
    const popup = page.waitForEvent('popup');
    await page.getByRole('button', { name: 'Pop out', exact: true }).click();
    const poppedOut = await popup;
    await expect(poppedOut.getByTestId('openrig-terminal')).toHaveAttribute('data-session-id', id!);
    await expect(poppedOut.locator('.xterm-rows')).toContainText('input: dashboard-input');
    await poppedOut.close();
    expect(tmuxAlive()).toBe(0);
    expect(await (await request.get('/api/sessions')).json()).toEqual([]);
    await page.getByRole('button', { name: 'Detach dashboard', exact: true }).click();
    await expect(terminal).toHaveCount(0);
    expect(tmuxAlive()).toBe(0);
    expect((await (await request.get('/api/openrig')).json()).session).toBeNull();
  } finally {
    try { if (id) await request.delete(`/api/sessions/${id}`); }
    finally {
      // Playwright may terminate its webServer process group before the Node
      // exit handler runs. Own this fixture's detached tmux session here too.
      spawnSync('tmux', ['-S', socket, 'kill-session', '-t', '=webmux-openrig-dashboard'], { timeout: 5000 });
    }
  }
});
