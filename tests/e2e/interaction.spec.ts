import { test, expect } from '@playwright/test';
import path from 'node:path';

test('connection dialog contains focus, dismisses with Escape, and restores its keyboard opener', async ({ page }) => {
  await page.goto('/');
  const add = page.getByTestId('add-cell-0-0').first();
  await expect(add).toBeVisible();
  await add.focus();
  await page.keyboard.press('Enter');
  const dialog = page.getByRole('dialog', { name: 'Connect to Host' });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByPlaceholder('hostname or IP')).toBeFocused();

  // The workspace is inert. Native Tab navigation may visit browser chrome
  // (reported as document.body), but must never focus a background control.
  await page.getByRole('button', { name: 'Settings', exact: true }).evaluate(element => element.focus());
  expect(await dialog.evaluate(element => element.contains(document.activeElement))).toBe(true);
  for (let i = 0; i < 16; i++) {
    await page.keyboard.press(i < 8 ? 'Tab' : 'Shift+Tab');
    expect(await dialog.evaluate(element => element.contains(document.activeElement) || document.activeElement === document.body)).toBe(true);
  }
  await dialog.getByRole('button', { name: 'Close connect to host' }).focus();
  await page.keyboard.press('Escape');
  await expect(dialog).not.toBeVisible();
  await expect(add).toBeFocused();
});

test('clicking the backdrop dismisses a dialog without activating the workspace', async ({ page }) => {
  await page.goto('/');
  const settings = page.getByRole('button', { name: 'Settings', exact: true });
  await settings.click();
  const dialog = page.getByRole('dialog', { name: 'Settings' });
  await expect(dialog.getByLabel('Application name')).toBeVisible();
  await page.mouse.click(2, 2);
  await expect(dialog).not.toBeVisible();
  await expect(settings).toBeFocused();
});

test('desktop picker and connection forms share keyboard dismissal and fit narrow screens', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'Desktops', exact: true }).click();
  await page.setViewportSize({ width: 375, height: 667 });
  for (const protocol of ['VNC', 'RDP']) {
    const add = page.getByTestId('add-cell-0-0').filter({ visible: true });
    await add.focus();
    await page.keyboard.press('Enter');
    const picker = page.getByRole('dialog', { name: 'Add Graphics Session' });
    await picker.getByRole('button', { name: new RegExp(protocol) }).click();
    const dialog = page.getByRole('dialog', { name: `Connect to ${protocol} Desktop` });
    await expect(dialog).toBeVisible();
    const bounds = await dialog.locator('.webmux-dialog-panel').boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(375);
    expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(667);
    await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeInViewport();
    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible();
  }
});

test('terminal control keys reach a real PTY and search and logging use explicit controls', async ({ page, request }) => {
  // An isolated raw-mode process reports the bytes it actually receives from xterm.
  const fixture = path.resolve(__dirname, 'terminal-fixture.mts');
  const response = await request.post('/api/sessions', { data: {
    hostname: 'localhost', username: 'keyboard-test', transport: 'exec',
    exec_command: `"${process.execPath}" "${fixture}"`,
  } });
  expect(response.ok()).toBe(true);
  const session = await response.json();
  let output = '';
  const pageErrors: string[] = [];
  page.on('pageerror', error => pageErrors.push(error.message));
  page.on('websocket', socket => {
    if (!socket.url().includes(session.id)) return;
    socket.on('framereceived', frame => {
      const message = JSON.parse(frame.payload.toString());
      if (message.type === 'output') output += message.data;
    });
  });
  try {
    await page.goto('/');
    const tile = page.getByTestId(`tile-cell-${session.id}`);
    const input = tile.locator('.xterm-helper-textarea');
    await expect(input).toBeAttached();
    await expect.poll(() => output).toContain('READY');
    await input.focus();
    for (const [key, hex] of [['Control+f', '06'], ['Control+b', '02'], ['Control+l', '0c'], ['F3', '1b4f52']]) {
      output = '';
      await page.keyboard.press(key);
      await expect.poll(() => output).toContain(`KEY:${hex};`);
      await expect(input).toBeFocused();
      await expect(tile.getByRole('textbox', { name: 'Search terminal scrollback' })).not.toBeVisible();
      await expect(tile.getByRole('button', { name: 'Log session' })).toHaveAttribute('aria-pressed', 'false');
    }

    // xterm 5 does not encode all Ctrl+Shift chords. WebMux must still leave
    // them alone instead of switching panes or toggling transcript logging.
    for (const key of ['Control+Shift+L', 'Control+Shift+,', 'Control+Shift+.']) {
      await page.keyboard.press(key);
      await expect(input).toBeFocused();
      await expect(tile.getByRole('button', { name: 'Log session' })).toHaveAttribute('aria-pressed', 'false');
    }

    await page.getByRole('button', { name: 'Type to All', exact: true }).click();
    await tile.getByRole('button', { name: 'Search terminal', exact: true }).click();
    const search = tile.getByRole('textbox', { name: 'Search terminal scrollback' });
    await expect(search).toBeFocused();
    await search.fill('READY');
    await expect(tile.getByText('1/1', { exact: true })).toBeVisible();
    await search.press('Escape');
    await expect(input).toBeFocused();
    await page.getByRole('button', { name: 'Type to All: ON', exact: true }).click();
    const log = tile.getByRole('button', { name: 'Log session' });
    await log.click();
    await expect(log).toHaveAttribute('aria-pressed', 'true');
    await log.click();
    await expect(log).toHaveAttribute('aria-pressed', 'false');
    expect(pageErrors).toEqual([]);
  } finally {
    const deleted = await request.delete(`/api/sessions/${session.id}`);
    expect(deleted.ok()).toBe(true);
  }
});
