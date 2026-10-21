import { test, expect } from '@playwright/test';
import path from 'node:path';
import { readFileSync, rmSync } from 'node:fs';

const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg==', 'base64');
const quote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;

test('selected text copies between separate WebMux browser windows', async ({ page, context, request }) => {
 const ids: string[] = [];
 const destination = await context.newPage();
 try {
  for (let index = 0; index < 2; index++) {
   const created = await request.post('/api/sessions', { data: { transport: 'exec', hostname: 'localhost', username: `clipboard-window-${index}`, exec_command: `"${process.execPath}" "${path.resolve(__dirname, 'clipboard-terminal-fixture.mts')}"` } });
   expect(created.ok()).toBe(true); ids.push((await created.json()).id);
  }
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await page.goto('/'); await destination.goto('/');
  await page.bringToFront();
  const source = page.getByTestId(`tile-cell-${ids[0]}`);
  await expect(source.locator('.xterm-rows')).toContainText('CLIPBOARD_READY');
  // xterm 6's rendered rows ignore pointer events; select through its interactive surface.
  await source.locator('.xterm-screen').dblclick({ position: { x: 15, y: 7 } });
  const mac = await page.evaluate(() => /Mac|iPhone|iPad/.test(navigator.platform));
  await page.keyboard.press(mac ? 'Meta+c' : 'Control+c');
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('CLIPBOARD_READY');
  await destination.bringToFront();
  const target = destination.getByTestId(`tile-cell-${ids[1]}`);
  await expect(target.locator('.xterm-rows')).toContainText('CLIPBOARD_READY');
  await target.locator('.xterm-helper-textarea').focus();
  await destination.keyboard.press(mac ? 'Meta+v' : 'Control+v');
  // Verify the destination process received bracketed paste, beyond clipboard success.
  const expected = Buffer.from('\x1b[200~CLIPBOARD_READY\x1b[201~').toString('hex');
  await expect(target.locator('.xterm-rows')).toContainText(`INPUT_HEX:${expected};`);
 } finally {
  await destination.close();
  for (const id of ids) expect((await request.delete(`/api/sessions/${id}`)).ok()).toBe(true);
 }
});

test('desktop shortcuts paste bracketed text, copy selection and handle OSC 52 writes without reads', async ({ page, request }) => {
 const fixture = path.resolve(__dirname, 'clipboard-terminal-fixture.mts');
 const created = await request.post('/api/sessions', { data: { transport: 'exec', hostname: 'localhost', username: 'clipboard-test', exec_command: `"${process.execPath}" "${fixture}"` } });
 expect(created.ok()).toBe(true);
 const { id } = await created.json();
 const inputs: string[] = [];
 page.on('websocket', socket => { if (socket.url().includes(id)) socket.on('framesent', frame => { const msg = JSON.parse(frame.payload.toString()); if (msg.type === 'input') inputs.push(msg.data); }); });
 try {
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
  await page.goto('/');
  const tile = page.getByTestId(`tile-cell-${id}`); const input = tile.locator('.xterm-helper-textarea');
  await expect(tile.locator('.xterm-rows')).toContainText('CLIPBOARD_READY');
  const mac = await page.evaluate(() => /Mac|iPhone|iPad/.test(navigator.platform));
  await input.focus();
  await page.evaluate(() => navigator.clipboard.writeText('first line\nsecond line'));
  await page.keyboard.press(mac ? 'Meta+v' : 'Control+v');
  await expect.poll(() => inputs.join('')).toContain('\x1b[200~first line\rsecond line\x1b[201~');
  await tile.locator('.xterm-screen').dblclick({ position: { x: 15, y: 7 } });
  await page.keyboard.press(mac ? 'Meta+c' : 'Control+c');
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('CLIPBOARD_READY');
  await input.focus();
  await page.keyboard.press('Control+d');
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('copied from terminal 😀');
  await page.evaluate(() => navigator.clipboard.writeText('desktop-private-read-must-not-leak'));
  inputs.length = 0;
  await page.keyboard.press('Control+e'); await page.keyboard.type('z');
  await expect.poll(() => inputs.join('')).toContain('z');
  expect(inputs.join('')).not.toContain('desktop-private');
  // Force a permission failure to exercise the visible one-click fallback.
  await page.evaluate(() => { Object.defineProperty(navigator.clipboard, 'writeText', { configurable: true, value: () => Promise.reject(new Error('blocked')) }); });
  await page.keyboard.press('Control+d');
  await expect(tile.getByRole('button', { name: 'Copy to desktop', exact: true })).toBeVisible();
 } finally { expect((await request.delete(`/api/sessions/${id}`)).ok()).toBe(true); }
});

test('image drop and clipboard paste deliver exact bytes into the local tmux host and never broadcast paths', async ({ page, request }) => {
 test.skip(process.platform === 'win32', 'Local tmux requires Unix');
 test.setTimeout(60000);
 const created = await request.post('/api/sessions', { data: { transport: 'local', cols: 100, rows: 30 } });
 expect(created.ok()).toBe(true); const { id } = await created.json();
 const peerCreated = await request.post('/api/sessions', { data: { transport: 'exec', hostname: 'localhost', username: 'clipboard-peer', exec_command: `"${process.execPath}" "${path.resolve(__dirname, 'clipboard-terminal-fixture.mts')}"` } });
 expect(peerCreated.ok()).toBe(true); const peer = await peerCreated.json();
 const inputs = new Map<string, string[]>([[id, []], [peer.id, []]]); const paths: string[] = [];
 page.on('websocket', socket => { const key = [id, peer.id].find(key => socket.url().includes(key)); if (key) socket.on('framesent', frame => { const msg = JSON.parse(frame.payload.toString()); if (msg.type === 'input') inputs.get(key)!.push(msg.data); }); });
 try {
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write']); await page.goto('/');
  const tile = page.getByTestId(`tile-cell-${id}`); const input = tile.locator('.xterm-helper-textarea');
  await input.focus();
  await page.keyboard.type([process.execPath, path.resolve(__dirname, 'clipboard-terminal-fixture.mts')].map(quote).join(' ')); await page.keyboard.press('Enter');
  await expect(tile.locator('.xterm-rows')).toContainText('CLIPBOARD_READY');
  await page.keyboard.press('Control+f');
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('copied through tmux');
  await page.getByRole('button', { name: /^Type to All/ }).click(); inputs.get(peer.id)!.length = 0;
  const uploaded = page.waitForResponse(response => response.url().endsWith(`/api/sessions/${id}/files`) && response.request().method() === 'POST');
  await tile.locator('.xterm').evaluate((element, base64) => {
   const bytes = Uint8Array.from(atob(base64), char => char.charCodeAt(0)); const dataTransfer = new DataTransfer();
   dataTransfer.items.add(new File([bytes], 'screen shot.png', { type: 'image/png' }));
   element.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer }));
  }, png.toString('base64'));
  const response = await uploaded; expect(response.ok()).toBe(true); const dropped = await response.json(); paths.push(dropped.path);
  expect(readFileSync(dropped.path)).toEqual(png);
  await expect.poll(() => inputs.get(id)!.join('')).toContain(quote(dropped.path));
  expect(inputs.get(peer.id)).toEqual([]);
  await page.getByRole('button', { name: /^Type to All/ }).click();
  await page.evaluate(async base64 => {
   const bytes = Uint8Array.from(atob(base64), char => char.charCodeAt(0));
   await navigator.clipboard.write([new ClipboardItem({ 'image/png': new Blob([bytes], { type: 'image/png' }) })]);
  }, png.toString('base64'));
  const pasted = page.waitForResponse(response => response.url().endsWith(`/api/sessions/${id}/files`) && response.request().method() === 'POST');
  await input.focus(); const mac = await page.evaluate(() => /Mac|iPhone|iPad/.test(navigator.platform)); await page.keyboard.press(mac ? 'Meta+v' : 'Control+v');
  const pasteResponse = await pasted; expect(pasteResponse.ok()).toBe(true); const image = await pasteResponse.json(); paths.push(image.path);
  // Browser clipboard normalization may re-encode PNG, so compare with clipboard bytes.
  const clipboardBytes = await page.evaluate(async () => Array.from(new Uint8Array(await (await (await navigator.clipboard.read())[0].getType('image/png')).arrayBuffer())));
  expect(readFileSync(image.path)).toEqual(Buffer.from(clipboardBytes));
  await expect.poll(() => inputs.get(id)!.join('')).toContain(quote(image.path));
  await expect(tile.getByRole('status')).toContainText('Delivered');
 } finally {
  await request.delete(`/api/sessions/${id}`); await request.delete(`/api/sessions/${peer.id}`);
  for (const file of paths) rmSync(file, { force: true });
 }
});
