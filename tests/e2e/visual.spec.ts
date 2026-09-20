import { test, expect, type Page, type TestInfo } from '@playwright/test';
import { readFile } from 'node:fs/promises';

test.skip(process.env.WEBMUX_VISUAL_PARITY !== '1', 'Run through test:visual-parity to create the Node baseline first');
test.use({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 1, locale: 'en-US', timezoneId: 'UTC', colorScheme: 'dark', contextOptions: { reducedMotion: 'reduce' } });

async function capture(page: Page, info: TestInfo, name: string): Promise<void> {
  await page.evaluate(() => document.fonts.ready);
  await expect(page).toHaveScreenshot(name, { animations: 'disabled', caret: 'hide', threshold: 0, maxDiffPixels: 0 });
  // Playwright's comparator may ignore anti-aliasing differences even at zero
  // threshold. Compare every decoded RGBA pixel as well, without that exemption.
  const actual = await page.screenshot({ animations: 'disabled', caret: 'hide' });
  const expected = await readFile(info.snapshotPath(name));
  const differences = await page.evaluate(async ({ actual, expected }) => {
    const pixels = async (encoded: string): Promise<ImageData> => {
      const image = new Image();
      image.src = `data:image/png;base64,${encoded}`;
      await image.decode();
      const canvas = document.createElement('canvas');
      canvas.width = image.naturalWidth; canvas.height = image.naturalHeight;
      const context = canvas.getContext('2d');
      if (!context) throw new Error('Screenshot decoder unavailable');
      context.drawImage(image, 0, 0);
      return context.getImageData(0, 0, canvas.width, canvas.height);
    };
    const a = await pixels(actual), b = await pixels(expected);
    if (a.width !== b.width || a.height !== b.height) return -1;
    let changed = 0;
    for (let i = 0; i < a.data.length; i += 4) {
      if (a.data[i] !== b.data[i] || a.data[i+1] !== b.data[i+1] || a.data[i+2] !== b.data[i+2] || a.data[i+3] !== b.data[i+3]) changed++;
    }
    return changed;
  }, { actual: actual.toString('base64'), expected: expected.toString('base64') });
  if (differences !== 0) await info.attach(`raw-${name}`, { body: actual, contentType: 'image/png' });
  expect(differences, `${name}: raw RGBA pixel differences`).toBe(0);
}

test('workspace and dialogs match the Node rendering exactly', async ({ page }, info) => {
  await page.goto('/');
  const add = page.getByTestId('add-cell-0-0').first();
  await expect(add).toBeVisible();
  await capture(page, info, 'workspace.png');
  await add.click();
  await expect(page.getByRole('dialog', { name: 'Connect to Host' })).toBeVisible();
  await capture(page, info, 'terminal-dialog.png');
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'Settings', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'Settings' }).getByLabel('Application name')).toBeVisible();
  await capture(page, info, 'settings.png');
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'Desktops', exact: true }).click();
  await capture(page, info, 'desktops.png');
  for (const protocol of ['VNC', 'RDP']) {
    await page.getByTestId('add-cell-0-0').filter({ visible: true }).click();
    await page.getByRole('dialog', { name: 'Add Graphics Session' }).getByRole('button', { name: new RegExp(protocol) }).click();
    await expect(page.getByRole('dialog', { name: `Connect to ${protocol} Desktop` })).toBeVisible();
    await capture(page, info, `${protocol.toLowerCase()}-dialog.png`);
    await page.setViewportSize({ width: 375, height: 667 });
    await capture(page, info, `${protocol.toLowerCase()}-dialog-narrow.png`);
    await page.keyboard.press('Escape');
    await page.setViewportSize({ width: 1280, height: 800 });
  }
});
