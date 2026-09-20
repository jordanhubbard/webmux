import { test, expect, type Page, type TestInfo } from '@playwright/test';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { randomBytes } from 'node:crypto';

test.skip(process.env.WEBMUX_VISUAL_PARITY !== '1', 'Run through test:visual-parity to create the Node baseline first');
test.use({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 1, locale: 'en-US', timezoneId: 'UTC', colorScheme: 'dark', contextOptions: { reducedMotion: 'reduce' } });

async function capture(page: Page, info: TestInfo, name: string): Promise<void> {
  await page.evaluate(() => document.fonts.ready);
  const session = await page.context().newCDPSession(page);
  let renderingArguments: string[];
  try {
    const command = await session.send('Browser.getBrowserCommandLine');
    renderingArguments = command.arguments.filter(argument => /^--(?:disable-gpu$|force-color-profile=|disable-partial-raster$)/.test(argument));
  } finally { await session.detach(); }
  const diagnostic = JSON.stringify({ renderingArguments, ...await page.evaluate(() => ({
    userAgent: navigator.userAgent,
    viewport: { width: innerWidth, height: innerHeight, scale: devicePixelRatio },
    fonts: [...document.fonts].map(font => ({ family: font.family, status: font.status })),
    buttons: [...document.querySelectorAll('button')].map(button => {
      const style = getComputedStyle(button);
      return { text: button.textContent, title: button.title, bounds: button.getBoundingClientRect().toJSON(),
        styles: Object.fromEntries([...style].map(key => [key, style.getPropertyValue(key)])) };
    }),
  })) }, null, 2);
  const baselineDiagnostic = info.snapshotPath(`${name}.json`);
  if (info.config.updateSnapshots === 'all') {
    await mkdir(path.dirname(baselineDiagnostic), { recursive: true });
    await writeFile(baselineDiagnostic, diagnostic);
  }
  try { await comparePixels(page, info, name); }
  catch (error) {
    for (const [label, data] of [['actual', diagnostic], ['baseline', await readFile(baselineDiagnostic)]] as const) {
      const file = info.outputPath(`${name}-${label}-layout.json`);
      await writeFile(file, data);
      await info.attach(`${name}-${label}-layout`, { path: file, contentType: 'application/json' });
    }
    throw error;
  }
}

async function comparePixels(page: Page, info: TestInfo, name: string): Promise<void> {
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
  if (differences !== 0) {
    const file = info.outputPath(`raw-${name}`);
    await writeFile(file, actual);
    await info.attach(`raw-${name}`, { path: file, contentType: 'image/png' });
  }
  expect(differences, `${name}: raw RGBA pixel differences`).toBe(0);
}

async function captureBoth(page: Page, info: TestInfo, name: string): Promise<void> {
  await capture(page, info, `${name}.png`);
  await page.setViewportSize({ width: 375, height: 667 });
  await capture(page, info, `${name}-narrow.png`);
  await page.setViewportSize({ width: 1280, height: 800 });
}

test('workspace and dialogs match the Node rendering exactly', async ({ page }, info) => {
  test.skip(process.env.WEBMUX_E2E_AUTH === 'local', 'Trusted workspace uses the no-auth fixture');
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

test('setup and sign-in match the Node rendering exactly', async ({ page }, info) => {
  test.skip(process.env.WEBMUX_E2E_AUTH !== 'local', 'Authentication uses an isolated local-auth fixture');
  const password = randomBytes(12).toString('hex');
  const incorrect = randomBytes(12).toString('hex');
  await page.goto('/');
  await expect(page.getByText('First-time setup', { exact: true })).toBeVisible();
  await captureBoth(page, info, 'auth-setup');
  await page.getByLabel('Username', { exact: true }).fill('visual-owner');
  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByLabel('Confirm Password', { exact: true }).fill(incorrect);
  await page.getByRole('button', { name: 'Create Account', exact: true }).click();
  await expect(page.getByText('Passwords do not match', { exact: true })).toBeVisible();
  await captureBoth(page, info, 'auth-setup-error');
  await page.getByLabel('Confirm Password', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Create Account', exact: true }).click();
  await expect(page.getByTestId('add-cell-0-0').first()).toBeVisible();
  await page.getByRole('button', { name: 'Sign out', exact: true }).click();
  await expect(page.getByText('Sign in to your session', { exact: true })).toBeVisible();
  await expect(page.getByLabel('Confirm Password', { exact: true })).toHaveCount(0);
  await captureBoth(page, info, 'auth-sign-in');
  await page.getByLabel('Username', { exact: true }).fill('visual-owner');
  await page.getByLabel('Password', { exact: true }).fill(incorrect);
  await page.getByRole('button', { name: 'Sign In', exact: true }).click();
  await expect(page.getByText('Invalid credentials', { exact: true })).toBeVisible();
  await captureBoth(page, info, 'auth-sign-in-error');
  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Sign In', exact: true }).click();
  await expect(page.getByTestId('add-cell-0-0').first()).toBeVisible();
  await page.reload();
  await expect(page.getByTestId('add-cell-0-0').first()).toBeVisible();
  await expect(page.getByRole('button', { name: 'Sign out', exact: true })).toBeVisible();
});

test('active terminal, search and reconnect match Node exactly', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_E2E_AUTH === 'local', 'Terminal rendering uses the trusted fixture');
  const fixture = path.resolve(__dirname, 'visual-terminal-fixture.mts');
  const response = await request.post('/api/sessions', { data: {
    hostname: 'localhost', username: 'visual-test', transport: 'exec',
    exec_command: `"${process.execPath}" "${fixture}"`,
  } });
  expect(response.ok()).toBe(true);
  const session: { id: string } = await response.json();
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  try {
    await page.goto('/');
    const tile = page.getByTestId(`tile-cell-${session.id}`);
    const rows = tile.locator('.xterm-rows');
    const input = tile.locator('.xterm-helper-textarea');
    await expect(rows).toContainText('fixture ready');
    await input.focus();
    await page.keyboard.type('hello');
    await page.keyboard.press('Enter');
    await expect(rows).toContainText('input: hello');
    await captureBoth(page, info, 'terminal-active');
    await tile.getByRole('button', { name: 'Search terminal', exact: true }).click();
    const search = tile.getByRole('textbox', { name: 'Search terminal scrollback' });
    await search.fill('needle');
    await expect(tile.getByText('1/2', { exact: true })).toBeVisible();
    await captureBoth(page, info, 'terminal-search');
    await search.press('Escape');
    await input.focus();
    await page.keyboard.type('exit');
    await page.keyboard.press('Enter');
    const reconnect = tile.getByRole('button', { name: 'Reconnect', exact: true }).last();
    await expect(reconnect).toBeVisible();
    await captureBoth(page, info, 'terminal-disconnected');
    await reconnect.click();
    await expect(tile.getByRole('button', { name: 'Reconnect', exact: true })).toHaveCount(0);
    await expect(rows).toContainText('fixture ready');
    await expect(rows).not.toContainText('fixture stopped');
    expect(errors).toEqual([]);
  } finally {
    expect((await request.delete(`/api/sessions/${session.id}`)).ok()).toBe(true);
  }
});
