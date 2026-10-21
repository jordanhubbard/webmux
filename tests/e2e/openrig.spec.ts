import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';
import path from 'node:path';

test('coordination is a dismissible modal and leaves the terminal grid intact', async ({ page, request }) => {
  const original = await (await request.get('/api/sessions')).json();
  let attachments = 0;
  page.on('request', value => { if (value.method() === 'POST' && /coordination|openrig/.test(value.url())) attachments++; });
  await page.goto('/');
  await page.getByRole('button', { name: 'Coordination', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Coordination', exact: true });
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
    try { if (id) await request.delete(`/api/sessions/${id}`); }
    finally {
      const { spawnSync } = await import('node:child_process');
      spawnSync(path.join(home, 'openrig-bin', 'rig'), ['daemon', 'stop'], { timeout: 15000, stdio: 'ignore' });
    }
  }
});
