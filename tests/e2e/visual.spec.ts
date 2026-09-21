import { test, expect, type Page, type TestInfo } from '@playwright/test';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { randomBytes } from 'node:crypto';
import { startVisualVnc } from './visual-vnc-fixture.mts';
import { startVisualRdp } from './visual-rdp-fixture.mts';

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
    // Both the tile and overview must reflect the live connection state.
    const overview = page.getByTitle('Workspace overview — click or drag to jump').locator('rect').first();
    await expect(overview).toHaveAttribute('fill', '#4aaa6a');
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
    await expect(overview).toHaveAttribute('fill', '#555');
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

test('active VNC pixels and input match Node exactly', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_E2E_AUTH === 'local', 'Desktop uses the trusted fixture');
  const desktop = await startVisualVnc();
  let id: string | undefined;
  try {
    const response = await request.post('/api/vnc/sessions', { data: { hostname: '127.0.0.1', vnc_port: desktop.port } });
    expect(response.ok()).toBe(true);
    id = (await response.json() as { id: string }).id;
    await page.goto('/');
    await page.getByRole('button', { name: 'Desktops', exact: true }).click();
    const canvas = page.locator('canvas').filter({ visible: true }).first();
    await expect.poll(() => canvas.evaluate(node => {
      const pixel = (node as HTMLCanvasElement).getContext('2d')?.getImageData(0, 0, 1, 1).data;
      return pixel ? Array.from(pixel) : [];
    })).toEqual([48, 80, 112, 255]);
    await captureBoth(page, info, 'vnc-active');
    // The thumbnail itself disables pointer events; double-click its body.
    await canvas.locator('..').locator('..').locator('..').dblclick({ position: { x: 100, y: 100 } });
    await expect(page.getByTitle('Back to grid', { exact: true })).toBeVisible();
    const fullscreen = page.locator('canvas').filter({ visible: true }).first();
    await expect.poll(() => fullscreen.evaluate(node => (node as HTMLCanvasElement).width)).toBe(320);
    await fullscreen.click({ position: { x: 20, y: 20 } });
    await page.keyboard.press('a');
    await expect.poll(() => desktop.keys.includes(97)).toBe(true);
    await expect.poll(() => desktop.pointers.some(mask => (mask & 1) !== 0)).toBe(true);
    await page.mouse.move(0, 0);
    await captureBoth(page, info, 'vnc-fullscreen');
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'], { origin: new URL(page.url()).origin });
    const clipboard = 'WebMux clipboard\nsecond line';
    await page.evaluate(text => navigator.clipboard.writeText(text), clipboard);
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(clipboard);
    await page.getByTitle('VNC Options', { exact: true }).click();
    await page.getByText('Paste Clipboard', { exact: true }).click();
    await expect.poll(() => desktop.clipboard).toContain(clipboard);
    expect(desktop.errors).toEqual([]);
  } finally {
    try {
      if (id) expect((await request.delete(`/api/vnc/sessions/${id}`)).ok()).toBe(true);
    } finally {
      await desktop.close();
    }
  }
});

test('active RDP pixels and input match Node exactly', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_E2E_AUTH === 'local', 'Desktop uses the trusted fixture');
  page.setDefaultTimeout(5000);
  const desktop = await startVisualRdp();
  let id: string | undefined;
  try {
    const response = await request.post('/api/rdp/sessions', { data: {
      hostname: '127.0.0.1', rdp_port: 3389, rdp_username: 'visual-user',
    } });
    expect(response.ok()).toBe(true);
    id = (await response.json() as { id: string }).id;
    await page.goto('/');
    await page.getByRole('button', { name: 'Desktops', exact: true }).click();
    const canvas = page.locator('canvas[width="640"]').filter({ visible: true }).first();
    const pixel = () => canvas.evaluate(node => Array.from((node as HTMLCanvasElement).getContext('2d')!.getImageData(0, 0, 1, 1).data));
    await expect.poll(pixel).toEqual([48, 80, 112, 255]);
    await captureBoth(page, info, 'rdp-active');
    // Click the tile's body, outside the scaled display's disabled pointer events.
    await canvas.locator('xpath=ancestor::div[@data-1p-ignore]/..').dblclick({ position: { x: 100, y: 100 }, timeout: 5000 });
    await expect(page.getByTitle('Back to grid', { exact: true })).toBeVisible();
    const viewer = page.locator('div[data-1p-ignore][tabindex="0"]');
    const fullscreen = viewer.locator('canvas[width="640"]');
    await expect.poll(() => fullscreen.evaluate(node => Array.from((node as HTMLCanvasElement).getContext('2d')!.getImageData(0, 0, 1, 1).data))).toEqual([48, 80, 112, 255]);
    // Assert composited screen pixels too: a painted canvas can sit behind an
    // opaque background and still pass getImageData() and baseline comparisons.
    await expect.poll(async () => {
      const bounds = await viewer.boundingBox();
      if (!bounds) return [];
      const screen = await page.screenshot({ clip: { x: bounds.x + 10, y: bounds.y + 10, width: 1, height: 1 } });
      return page.evaluate(async encoded => {
        const image = new Image(); image.src = `data:image/png;base64,${encoded}`; await image.decode();
        const canvas = document.createElement('canvas'); canvas.width = canvas.height = 1;
        const context = canvas.getContext('2d')!; context.drawImage(image, 0, 0);
        return Array.from(context.getImageData(0, 0, 1, 1).data);
      }, screen.toString('base64'));
    }).toEqual([48, 80, 112, 255]);
    await viewer.click({ position: { x: 20, y: 20 } });
    await page.keyboard.press('a');
    await expect.poll(() => desktop.keys.some(args => args[0] === '97' && args[1] === '1')).toBe(true);
    await expect.poll(() => desktop.pointers.some(args => (Number(args[2]) & 1) !== 0)).toBe(true);
    await page.mouse.move(0, 0);
    await captureBoth(page, info, 'rdp-fullscreen');
    expect(desktop.errors).toEqual([]);
  } finally {
    try { if (id) expect((await request.delete(`/api/rdp/sessions/${id}`)).ok()).toBe(true); }
    finally { await desktop.close(); }
  }
});
