import { test, expect } from '@playwright/test';

test('USB setup works across workspaces and fits a narrow screen', async ({ page }) => {
  await page.goto('/');
  for (const workspace of ['Terminals', 'Desktops']) {
    await page.getByRole('button', { name: workspace, exact: true }).click();
    const opener = page.getByRole('button', { name: 'USB redirection' });
    await opener.focus();
    await page.keyboard.press('Enter');
    const dialog = page.getByRole('dialog', { name: 'USB redirection' });
    await expect(dialog.getByLabel('Receiving SSH host')).toBeFocused();
    await dialog.getByLabel('Receiving SSH host').fill('developer@build-host');
    await expect(dialog.getByLabel('USB forwarding command')).toHaveValue(/--host developer@build-host/);
    await expect(dialog.getByRole('button', { name: 'Copy local command' })).toBeEnabled();
    await page.getByRole('button', { name: 'Settings', exact: true }).evaluate(element => element.focus());
    expect(await dialog.evaluate(element => element.contains(document.activeElement))).toBe(true);
    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible();
    await expect(opener).toBeFocused();
  }
  await page.setViewportSize({ width: 375, height: 667 });
  await page.getByRole('button', { name: 'USB redirection' }).click();
  const panel = page.locator('.webmux-dialog-panel');
  const bounds = await panel.boundingBox();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(375);
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(667);
  expect(await panel.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
});
