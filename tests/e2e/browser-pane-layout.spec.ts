import { test, expect, type Route } from '@playwright/test';
import http from 'node:http';
import { writeFileSync } from 'node:fs';

test('browser docking preserves the live page, control and desktop layout', async ({ page, request }, info) => {
  test.setTimeout(120_000);
  let documents = 0, presses = 0;
  const site = http.createServer((req, res) => {
    if (req.url === '/pressed') { presses++; res.end('ok'); return; }
    if (req.url === '/fixture') documents++;
    res.setHeader('Content-Type', 'text/html');
    res.end('<title>Layout fixture</title><style>body{margin:0}button{width:180px;height:60px}</style><button onclick="fetch(\'/pressed\')">Keep live page</button>');
  });
  await new Promise<void>(resolve => site.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${(site.address() as { port: number }).port}/fixture`;
  const created = await request.post('/api/sessions', { data: { transport: 'local' } });
  expect(created.ok()).toBe(true);
  const { id } = await created.json() as { id: string };
  const endpoint = `/api/sessions/${id}/browser`;
  const actions: string[] = [];
  const leases = new Set<string>();
  page.on('request', outgoing => {
    if (new URL(outgoing.url()).pathname !== endpoint || outgoing.method() !== 'POST') return;
    const body = outgoing.postDataJSON();
    actions.push(body.action);
    if (body.controlToken) leases.add(body.controlToken);
  });
  const frame = async () => {
    const response = await request.post(endpoint, { data: { action: 'frame' } });
    expect(response.ok()).toBe(true);
    return response.json();
  };
  const open = async () => {
    await page.getByTestId(`tile-cell-${id}`).getByRole('button', { name: 'Session tools', exact: true }).click();
    await page.getByRole('button', { name: 'Open browser', exact: true }).click();
    await expect(page.getByAltText('Interactive remote browser viewport')).toBeVisible({ timeout: 30_000 });
  };
  const companion = page.getByRole('complementary', { name: /^Browser for / });
  const bounds = async () => {
    const box = await companion.boundingBox();
    expect(box).not.toBeNull();
    return box!;
  };
  const image = page.getByAltText('Interactive remote browser viewport');
  const pressPage = async (count: number) => {
    // A recoverable worker error changes the viewport while the live page,
    // browser control and navigation remain owned by the original session.
    let notice = count === 1;
    const temporaryNotice = async (route: Route) => {
      const response = await route.fetch();
      const body = await response.json();
      await route.fulfill({ response, json: notice
        ? { ...body, error: 'Temporary layout qualification notice.' } : body });
    };
    if (count === 1) {
      await page.route('**' + endpoint, temporaryNotice);
      await expect(companion.getByRole('alert')).toContainText('Temporary layout qualification notice.');
    }
    await image.scrollIntoViewIfNeeded();
    let dimensions = await image.evaluate(element => ({ width: element.clientWidth, height: element.clientHeight }));
    if (count === 1) {
      notice = false;
      await expect(companion.getByRole('alert')).toHaveCount(0);
      // This test owns only the notice route. A polling frame can still be in
      // route.fetch after the alert clears; drain it before removing the route.
      await page.unrouteAll({ behavior: 'wait' });
      await expect.poll(async () => (await frame()).height).not.toBe(Math.max(240, Math.round(dimensions.height)));
      const geometry = info.outputPath('transient-viewport-geometry.json');
      writeFileSync(geometry, JSON.stringify({ captured: dimensions,
        visible: await image.evaluate(element => ({ width: element.clientWidth, height: element.clientHeight })),
        native: await frame().then(({ width, height }) => ({ width, height })), scope: 'Synthetic transient worker-error response with real Go/Chromium viewport resizing; no extra navigation.' }));
      await info.attach('transient-viewport-geometry', { path: geometry, contentType: 'application/json' });
    }
    // The notice and other live controls can change the visible viewport.
    // Retry the exact native/DOM comparison against the current measurement.
    await expect(async () => {
      dimensions = await image.evaluate(element => ({ width: element.clientWidth, height: element.clientHeight }));
      const current = await frame();
      expect({ width: current.width, height: current.height }).toEqual({ width: Math.max(320, Math.round(dimensions.width)), height: Math.max(240, Math.round(dimensions.height)) });
    }).toPass({ timeout: 5000 });
    await expect.poll(async () => image.evaluate(element => (element as HTMLImageElement).naturalWidth)).toBe(Math.max(320, Math.round(dimensions.width)));
    const natural = await image.evaluate(element => ({ width: (element as HTMLImageElement).naturalWidth, height: (element as HTMLImageElement).naturalHeight }));
    await image.click({ position: { x: 80 * dimensions.width / natural.width, y: 30 * dimensions.height / natural.height } });
    await expect.poll(() => presses).toBe(count);
  };
  try {
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto('/');
    await test.step('open companion', open);
    const address = page.getByRole('textbox', { name: 'Remote browser URL', exact: true });
    await address.fill(origin);
    await page.getByRole('button', { name: 'Go', exact: true }).click();
    await expect.poll(async () => (await frame()).url).toBe(origin);
    const initial = await frame();
    expect(initial.target).toBeTruthy();
    await test.step('initial page input', () => pressPage(1));
    const docked = await bounds();
    const edge = page.getByRole('separator', { name: 'Resize browser width', exact: true });
    await edge.focus();
    await page.keyboard.press('Shift+ArrowLeft');
    expect((await bounds()).width).toBe(docked.width + 50);
    const dockWidth = (await bounds()).width;
    await page.getByRole('button', { name: 'Undock browser', exact: true }).click();
    const move = page.getByRole('button', { name: 'Move browser pane', exact: true });
    const beforeMove = await bounds();
    const handle = (await move.boundingBox())!;
    await page.mouse.move(handle.x + handle.width / 2, handle.y + handle.height / 2);
    await page.mouse.down();
    await page.mouse.move(handle.x + handle.width / 2 - 45, handle.y + handle.height / 2 + 35, { steps: 5 });
    await page.mouse.up();
    expect((await bounds()).x).toBe(beforeMove.x - 45);
    expect((await bounds()).y).toBe(beforeMove.y + 35);
    const corner = page.getByRole('button', { name: 'Resize browser pane', exact: true });
    const beforeResize = await bounds();
    const grip = (await corner.boundingBox())!;
    await page.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2);
    await page.mouse.down();
    await page.mouse.move(grip.x + grip.width / 2 - 80, grip.y + grip.height / 2 - 80, { steps: 5 });
    await page.mouse.up();
    expect((await bounds()).width).toBe(beforeResize.width - 80);
    expect((await bounds()).height).toBe(beforeResize.height - 80);
    await move.focus();
    await page.keyboard.press('ArrowRight');
    const floating = await bounds();
    await test.step('input after floating resize', () => pressPage(2));
    await page.getByRole('button', { name: 'Expand browser', exact: true }).click();
    expect((await bounds()).width).toBe(1424);
    await page.getByRole('button', { name: 'Restore browser size', exact: true }).click();
    expect(await bounds()).toEqual(floating);
    await page.getByRole('button', { name: 'Dock browser', exact: true }).click();
    expect((await bounds()).width).toBe(dockWidth);
    await page.getByRole('button', { name: 'Undock browser', exact: true }).click();
    expect(await bounds()).toEqual(floating);
    const stable = await frame();
    expect({ target: stable.target, url: stable.url, profileId: stable.profileId }).toEqual({ target: initial.target, url: initial.url, profileId: initial.profileId });
    expect(actions.filter(action => action === 'start')).toHaveLength(1);
    expect(actions.filter(action => action === 'navigate')).toHaveLength(1);
    expect(actions).not.toContain('release');
    expect(leases.size).toBe(1);
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(move).toBeHidden();
    await expect.poll(bounds).toEqual({ x: 0, y: 0, width: 390, height: 844 });
    await page.setViewportSize({ width: 1440, height: 1000 });
    await expect.poll(bounds).toEqual(floating);
    await page.getByRole('button', { name: 'Return to terminal', exact: true }).click();
    await expect(companion).toHaveCount(0);
    await open();
    expect(await bounds()).toEqual(floating);
    await page.reload();
    await expect(page.getByTestId(`tile-cell-${id}`)).toBeVisible();
    // Browser visibility is session UI state; reopen only if reload hid it.
    if (await companion.count() === 0) await open();
    await expect(image).toBeVisible({ timeout: 30_000 });
    expect(await bounds()).toEqual(floating);
    await test.step('input after reload', async () => {
      const takeControl = page.getByRole('button', { name: 'Take control', exact: true });
      if (await takeControl.count()) {
        page.once('dialog', dialog => dialog.accept());
        await takeControl.click();
        await expect(page.getByRole('button', { name: 'Go', exact: true })).toBeEnabled();
      }
      await pressPage(3);
    });
    const reopened = await frame();
    expect({ target: reopened.target, url: reopened.url, profileId: reopened.profileId }).toEqual({ target: initial.target, url: initial.url, profileId: initial.profileId });
    expect(documents).toBe(1);
    const storage = await page.evaluate(() => localStorage.getItem('webmux-browser-pane-layout-v1'));
    expect(Object.keys(JSON.parse(storage!)).sort()).toEqual(['dockWidth', 'floating', 'mode', 'version']);
    expect(storage).not.toContain(origin);
    expect(storage).not.toContain(id);
    for (const lease of leases) expect(storage).not.toContain(lease);
    await info.attach('layout-lifecycle', { body: JSON.stringify({ dockWidth, floating, documents, presses, targetPreserved: reopened.target === initial.target, layoutKeys: Object.keys(JSON.parse(storage!)) }), contentType: 'application/json' });
    await page.screenshot({ path: info.outputPath('floating-browser.png') });
  } finally {
    try { await page.unrouteAll({ behavior: 'wait' }); }
    finally {
      try { await request.delete(`/api/sessions/${id}`, { timeout: 10_000 }); }
      finally { site.closeAllConnections(); await new Promise<void>(resolve => site.close(() => resolve())); }
    }
  }
});
