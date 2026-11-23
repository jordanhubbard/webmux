import { test, expect } from '@playwright/test';
import path from 'node:path';

test('wheel stays with the selected terminal; Shift pans both grid axes without scrolling either terminal', async ({ page, request }) => {
  test.setTimeout(60_000);
  await page.setViewportSize({ width: 1500, height: 750 });
  const ids: string[] = [];
  try {
    for (const [row, col] of [[0, 0], [0, 1], [2, 2]]) {
      const created = await request.post('/api/sessions', { data: {
        hostname: 'localhost', username: 'scroll-test', transport: 'exec', row, col,
        exec_command: `"${process.execPath}" "${path.resolve(__dirname, 'scroll-terminal-fixture.mts')}"`,
      } });
      expect(created.ok()).toBe(true); ids.push((await created.json()).id);
    }
    await page.goto('/');
    const source = page.getByTestId(`tile-cell-${ids[0]}`);
    const peer = page.getByTestId(`tile-cell-${ids[1]}`);
    const grid = page.getByTestId('workspace-scroll');
    const sourceRows = source.locator('.xterm-rows');
    const peerRows = peer.locator('.xterm-rows');
    await expect(source.locator('[data-terminal-session]')).toHaveAttribute('data-terminal-session', ids[0]);
    await expect(sourceRows).toContainText('SCROLL READY');
    await expect(peerRows).toContainText('SCROLL READY');
    const earlyBox = (await peer.locator('.xterm-screen').boundingBox())!;
    await page.mouse.move(earlyBox.x + 30, earlyBox.y + 30);
    await page.keyboard.down('Shift');
    try { await page.mouse.wheel(40, 40); } finally { await page.keyboard.up('Shift'); }
    await expect.poll(() => grid.evaluate(el => el.scrollLeft > 0 && el.scrollTop > 0)).toBe(true);
    await source.locator('.xterm-helper-textarea').focus();
    await grid.evaluate(el => { el.scrollTop = 0; el.scrollLeft = 0; });
    const position = () => grid.evaluate(el => [el.scrollLeft, el.scrollTop]);
    const initial = await position(); const before = await sourceRows.innerText(); const peerBefore = await peerRows.innerText();
    const peerBox = (await peer.locator('.xterm-screen').boundingBox())!;
    await page.mouse.move(peerBox.x + 30, peerBox.y + 30);
    await page.mouse.wheel(0, -240);
    await expect.poll(() => sourceRows.innerText()).not.toBe(before);
    expect(await peerRows.innerText()).toBe(peerBefore);
    expect(await position()).toEqual(initial);
    // Saturate the selected window's history in both directions. Browser
    // ancestor scroll chaining must not move the grid at either boundary.
    for (let step = 0; step < 60; step++) await page.mouse.wheel(0, -120);
    await expect(sourceRows).toContainText('SCROLL-LINE-0');
    for (let step = 0; step < 60; step++) await page.mouse.wheel(0, 120);
    await expect(sourceRows).toContainText('SCROLL READY');
    expect(await position()).toEqual(initial);
    const terminalBeforeShift = await sourceRows.innerText();
    await page.keyboard.down('Shift');
    try { await page.mouse.wheel(140, 180); } finally { await page.keyboard.up('Shift'); }
    await expect.poll(async () => (await position()).every(value => value > 0)).toBe(true);
    expect(await sourceRows.innerText()).toBe(terminalBeforeShift);
    expect(await peerRows.innerText()).toBe(peerBefore);
  } finally {
    for (const id of ids) expect((await request.delete(`/api/sessions/${id}`)).ok()).toBe(true);
  }
});

test('Shift wheel is intercepted before an application mouse report', async ({ page, request }) => {
  const inputs: string[] = [];
  page.on('websocket', socket => socket.on('framesent', frame => {
    const message = JSON.parse(frame.payload.toString());
    if (message.type === 'input') inputs.push(message.data);
  }));
  const created = await request.post('/api/sessions', { data: {
    hostname: 'localhost', username: 'wheel-mouse-test', transport: 'exec',
    exec_command: `"${process.execPath}" "${path.resolve(__dirname, 'mouse-terminal-fixture.mts')}" mouse unused`,
  } });
  expect(created.ok()).toBe(true); const { id } = await created.json();
  try {
    await page.goto('/'); const tile = page.getByTestId(`tile-cell-${id}`);
    await expect(tile.locator('.xterm-rows')).toContainText('MOUSE READY');
    await tile.locator('.xterm-helper-textarea').focus();
    await tile.locator('.xterm-screen').hover();
    const mouseWheels = () => inputs.filter(data => /\x1b\[<6[45];/.test(data));
    await page.mouse.wheel(0, -120);
    await expect.poll(() => mouseWheels().length).toBeGreaterThan(0);
    const count = mouseWheels().length;
    await page.keyboard.down('Shift');
    try { await page.mouse.wheel(0, -120); } finally { await page.keyboard.up('Shift'); }
    // Drain the browser event loop after the real wheel event, not a DOM-only dispatch.
    await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
    expect(mouseWheels()).toHaveLength(count);
    expect(inputs.filter(data => /\x1b\[<(?:68|69);/.test(data))).toEqual([]);
  } finally { expect((await request.delete(`/api/sessions/${id}`)).ok()).toBe(true); }
});

test('wheel-generated application arrows stay local while keyboard broadcast remains enabled', async ({ page, request }) => {
  const ids: string[] = []; const input = new Map<string, string[]>();
  page.on('websocket', socket => socket.on('framesent', frame => {
    const id = ids.find(value => socket.url().includes(value));
    const message = JSON.parse(frame.payload.toString());
    if (id && message.type === 'input') input.get(id)!.push(message.data);
  }));
  try {
    for (const mode of ['alternate', 'normal']) {
      const response = await request.post('/api/sessions', { data: {
        hostname: 'localhost', username: 'wheel-broadcast-test', transport: 'exec',
        exec_command: `"${process.execPath}" "${path.resolve(__dirname, 'scroll-terminal-fixture.mts')}" ${mode}`,
      } });
      expect(response.ok()).toBe(true); const { id } = await response.json(); ids.push(id); input.set(id, []);
    }
    await page.goto('/'); const source = page.getByTestId(`tile-cell-${ids[0]}`);
    await expect(source.locator('.xterm-rows')).toContainText('ALT WHEEL READY');
    await expect(page.getByTestId(`tile-cell-${ids[1]}`).locator('.xterm-rows')).toContainText('SCROLL READY');
    await source.locator('.xterm-helper-textarea').focus();
    await page.getByRole('button', { name: 'Type to All', exact: true }).click();
    await source.locator('.xterm-helper-textarea').focus();
    await page.keyboard.type('k');
    await expect.poll(() => ids.every(id => input.get(id)!.includes('k'))).toBe(true);
    ids.forEach(id => { input.get(id)!.length = 0; });
    await source.locator('.xterm-screen').hover(); await page.mouse.wheel(0, -120);
    await expect.poll(() => input.get(ids[0])!.some(data => /^\x1b(?:\[|O)[AB]$/.test(data))).toBe(true);
    expect(input.get(ids[1])!.filter(data => data.length > 0 && data !== '\x1b[I' && data !== '\x1b[O').map(data => Buffer.from(data).toString('hex'))).toEqual([]);
  } finally { for (const id of ids) expect((await request.delete(`/api/sessions/${id}`)).ok()).toBe(true); }
});
