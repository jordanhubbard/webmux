import { test, expect } from '@playwright/test';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { readFileSync, rmSync } from 'node:fs';

const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg==', 'base64');
const quote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
const nativeClipboard = () => execFileSync(process.platform === 'darwin' ? 'pbpaste' : 'xclip',
 process.platform === 'darwin' ? [] : ['-selection', 'clipboard', '-o'], { encoding: 'utf8', timeout: 5000 });

test('selection automatically copies to native paste in another app and WebMux window without permissions or dialogs', async ({ page, context, request }) => {
 const ids: string[] = [];
 const destination = await context.newPage();
 try {
  for (let index = 0; index < 2; index++) {
   const created = await request.post('/api/sessions', { data: { transport: 'exec', hostname: 'localhost', username: `clipboard-window-${index}`, exec_command: `"${process.execPath}" "${path.resolve(__dirname, 'clipboard-terminal-fixture.mts')}"` } });
   expect(created.ok()).toBe(true); ids.push((await created.json()).id);
  }
  await page.goto('/'); await destination.goto('/');
  await page.bringToFront();
  const source = page.getByTestId(`tile-cell-${ids[0]}`);
  await expect(source.locator('.xterm-rows')).toContainText('CLIPBOARD_READY');
  // Exercise the native copy command used on insecure HTTP hosts too.
  await page.evaluate(() => Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined }));
  const screen = source.locator('.xterm-screen');
  const bounds = (await screen.boundingBox())!;
  const cellWidth = await source.locator('.xterm-rows').evaluate(element => {
   const style = getComputedStyle(element); const canvas = document.createElement('canvas');
   const drawing = canvas.getContext('2d')!; drawing.font = `${style.fontSize} ${style.fontFamily}`;
   return drawing.measureText('W').width;
  });
  await page.mouse.move(bounds.x + 1, bounds.y + 7);
  await page.mouse.down();
  await page.mouse.move(bounds.x + cellWidth * 15 + 1, bounds.y + 7, { steps: 12 });
  await page.mouse.up();
  if (process.env.WEBMUX_TEST_NATIVE_CLIPBOARD === '1') {
   expect(nativeClipboard()).toBe('CLIPBOARD_READY');
  }
  const mac = await page.evaluate(() => /Mac|iPhone|iPad/.test(navigator.platform));
  // No copy shortcut, Clipboard API grants, or WebMux dialog. A separate page
  // receives the host clipboard through the browser's real native paste event.
  const otherApp = await context.newPage();
  await otherApp.goto('data:text/html,<textarea aria-label="Other application"></textarea>');
  await otherApp.getByRole('textbox').focus();
  await otherApp.keyboard.press(mac ? 'Meta+v' : 'Control+v');
  await expect(otherApp.getByRole('textbox')).toHaveValue('CLIPBOARD_READY');
  await otherApp.close();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Terminal clipboard', exact: true })).toHaveCount(0);
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

test('desktop shortcuts paste bracketed text, copy selection and handle immediate OSC 52 copies and application pasteboard reads', async ({ page, request }) => {
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
  if (process.env.WEBMUX_TEST_NATIVE_CLIPBOARD === '1') {
   await expect.poll(nativeClipboard).toBe('CLIPBOARD_READY');
  }
  await page.keyboard.press(mac ? 'Meta+c' : 'Control+c');
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('CLIPBOARD_READY');
  await input.focus();
  await page.keyboard.press('Control+d');
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('copied from terminal 😀');
  await page.evaluate(() => navigator.clipboard.writeText('desktop pasteboard 😀'));
  inputs.length = 0;
  await page.keyboard.press('Control+e');
  const reply = '\x1b]52;c;' + Buffer.from('desktop pasteboard 😀').toString('base64') + '\x1b\\';
  await expect(tile.locator('.xterm-rows')).toContainText(`INPUT_HEX:${Buffer.from(reply).toString('hex')};`);
  expect(inputs.join('')).not.toContain('desktop pasteboard');
  // An asynchronous Clipboard API rejection falls back to the native copy command.
  await page.evaluate(() => { Object.defineProperty(navigator.clipboard, 'writeText', { configurable: true, value: () => Promise.reject(new Error('blocked')) }); });
  await page.keyboard.press('Control+d');
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('copied from terminal 😀');
  await expect(tile.getByRole('button', { name: 'Copy to desktop', exact: true })).toHaveCount(0);

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

test('application pasteboard replies stay with the focused session under Type to All', async ({ page, request }) => {
 const ids: string[] = [];
 const replies = new Map<string, string[]>();
 page.on('websocket', socket => {
  const id = ids.find(id => socket.url().includes(id));
  if (id) socket.on('framesent', frame => {
   const message = JSON.parse(frame.payload.toString());
   if (message.type === 'clipboard_reply') replies.get(id)!.push(message.data);
  });
 });
 try {
  for (let index = 0; index < 2; index++) {
   const response = await request.post('/api/sessions', { data: { transport: 'exec', hostname: 'localhost', username: `clipboard-scope-${index}`, exec_command: `"${process.execPath}" "${path.resolve(__dirname, 'clipboard-terminal-fixture.mts')}"` } });
   expect(response.ok()).toBe(true); const { id } = await response.json(); ids.push(id); replies.set(id, []);
  }
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
  await page.goto('/');
  const source = page.getByTestId(`tile-cell-${ids[0]}`);
  const peer = page.getByTestId(`tile-cell-${ids[1]}`);
  for (const tile of [source, peer]) await expect(tile.locator('.xterm-rows')).toContainText('CLIPBOARD_READY');
  await page.getByRole('button', { name: /^Type to All/ }).click();
  await source.locator('.xterm-helper-textarea').focus();
  await page.evaluate(() => navigator.clipboard.writeText('session-private clipboard'));
  await page.keyboard.press('Control+e');
  await expect.poll(() => replies.get(ids[0])).toEqual(['session-private clipboard']);
  expect(replies.get(ids[1])).toEqual([]);
  const encoded = Buffer.from('session-private clipboard').toString('base64');
  await expect(source.locator('.xterm-rows')).toContainText(Buffer.from(`\x1b]52;c;${encoded}\x1b\\`).toString('hex'));
  await expect(peer.locator('.xterm-rows')).not.toContainText(encoded);
  // Resolve a clipboard read after switching terminals: no late response is sent.
  await page.getByRole('button', { name: /^Type to All/ }).click();
  await source.locator('.xterm-helper-textarea').focus();
  await page.evaluate(() => {
   Object.defineProperty(navigator.clipboard, 'readText', { configurable: true, value: () => new Promise<string>(resolve => {
    Object.assign(window, { finishClipboardRead: resolve });
   }) });
  });
  await page.keyboard.press('Control+e');
  await expect.poll(() => page.evaluate(() => typeof Reflect.get(window, 'finishClipboardRead'))).toBe('function');
  await peer.locator('.xterm-helper-textarea').focus();
  await source.locator('.xterm-helper-textarea').focus();
  await page.evaluate(() => { Reflect.get(window, 'finishClipboardRead')('late clipboard'); });
  await peer.locator('.xterm-helper-textarea').focus();
  await page.keyboard.insertText('focus-barrier');
  await expect(peer.locator('.xterm-rows')).toContainText(Buffer.from('focus-barrier').toString('hex').slice(-8));
  expect(replies.get(ids[0])).toEqual(['session-private clipboard']);
  expect(replies.get(ids[1])).toEqual([]);
 } finally {
  for (const id of ids) await request.delete(`/api/sessions/${id}`);
 }
});
