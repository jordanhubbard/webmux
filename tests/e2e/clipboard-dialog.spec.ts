import { test, expect } from '@playwright/test';

test('Copy text in a modal dialog reaches native paste in another window', async ({ page, context }) => {
  // Clipboard copying does not require a remote desktop connection.
  await page.route('**/api/vnc/sessions', route => route.fulfill({ json: [{
    id: 'clipboard-dialog', kind: 'vnc', title: 'Clipboard test desktop',
    hostname: 'localhost', vnc_port: 5900, row: 0, col: 0, state: 'disconnected',
  }] }));
  await page.goto('/');
  await page.getByRole('button', { name: 'Desktops', exact: true }).click();
  const tile = page.getByText('Clipboard test desktop', { exact: true }).locator('..').locator('..').locator('..');
  await tile.locator(':scope > div').last().dblclick({ position: { x: 10, y: 10 } });
  await page.getByTitle('VNC Options', { exact: true }).click();
  await page.getByRole('button', { name: 'Clipboard', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Clipboard', exact: true });
  const text = 'Host clipboard copy 😀';
  await dialog.getByRole('textbox').fill(text);
  // Exercise HTTP-compatible copying without API permission grants.
  await page.evaluate(() => Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined }));
  const copy = dialog.getByRole('button', { name: 'Copy text', exact: true });
  await copy.click();
  await expect(dialog.getByRole('status')).toHaveText('Copied.');
  await expect(copy).toBeFocused();
  const mac = await page.evaluate(() => /Mac|iPhone|iPad/.test(navigator.platform));
  const destination = await context.newPage();
  try {
    await destination.goto('data:text/html,<textarea aria-label="Other application"></textarea>');
    await destination.getByRole('textbox').focus();
    await destination.keyboard.press(mac ? 'Meta+v' : 'Control+v');
    await expect(destination.getByRole('textbox')).toHaveValue(text);
  } finally { await destination.close(); }
});
