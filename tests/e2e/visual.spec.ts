import { test, expect, type Page } from '@playwright/test';

test.skip(process.env.WEBMUX_VISUAL_PARITY !== '1', 'Run through test:visual-parity to create the Node baseline first');
test.use({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 1, locale: 'en-US', timezoneId: 'UTC', colorScheme: 'dark', contextOptions: { reducedMotion: 'reduce' } });

async function capture(page: Page, name: string): Promise<void> {
  await page.evaluate(() => document.fonts.ready);
  await expect(page).toHaveScreenshot(name, { animations: 'disabled', caret: 'hide', threshold: 0, maxDiffPixels: 0 });
}

test('workspace and dialogs match the Node rendering exactly', async ({ page }) => {
  await page.goto('/');
  const add = page.getByTestId('add-cell-0-0').first();
  await expect(add).toBeVisible();
  await capture(page, 'workspace.png');
  await add.click();
  await expect(page.getByRole('dialog', { name: 'Connect to Host' })).toBeVisible();
  await capture(page, 'terminal-dialog.png');
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'Settings', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'Settings' }).getByLabel('Application name')).toBeVisible();
  await capture(page, 'settings.png');
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'Desktops', exact: true }).click();
  await capture(page, 'desktops.png');
  for (const protocol of ['VNC', 'RDP']) {
    await page.getByTestId('add-cell-0-0').filter({ visible: true }).click();
    await page.getByRole('dialog', { name: 'Add Graphics Session' }).getByRole('button', { name: new RegExp(protocol) }).click();
    await expect(page.getByRole('dialog', { name: `Connect to ${protocol} Desktop` })).toBeVisible();
    await capture(page, `${protocol.toLowerCase()}-dialog.png`);
    await page.setViewportSize({ width: 375, height: 667 });
    await capture(page, `${protocol.toLowerCase()}-dialog-narrow.png`);
    await page.keyboard.press('Escape');
    await page.setViewportSize({ width: 1280, height: 800 });
  }
});
