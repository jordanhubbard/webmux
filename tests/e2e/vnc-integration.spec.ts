import { test, expect } from '@playwright/test';
import { spawn, spawnSync, type ChildProcess } from 'node:child_process';
import { createServer, createConnection } from 'node:net';
import { once } from 'node:events';
import { randomBytes } from 'node:crypto';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { verifyXKeyboard } from './x-keyboard-fixture';

for (const authenticated of [false, true]) test(`real x11vnc updates and input (${authenticated ? 'password' : 'no password'})`, async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_VNC !== '1', 'Requires Linux Xvfb, x11vnc, xsetroot, xdotool, xev and xclip');
  test.setTimeout(60_000);
  const children: ChildProcess[] = [];
  const temporary = mkdtempSync(join(tmpdir(), 'webmux-real-vnc-'));
  const password = authenticated ? randomBytes(4).toString('hex') : '';
  let diagnostics = '';
  let keyboardEvents = '';
  let failure: Error | undefined;
  let id: string | undefined;
  function start(command: string, args: string[], env = process.env) {
    const child = spawn(command, args, { env, stdio: ['ignore', 'pipe', 'pipe'] });
    children.push(child);
    child.on('error', error => { failure = error; });
    child.stderr!.on('data', chunk => { diagnostics = (diagnostics + String(chunk)).slice(-65536); });
    return child;
  }
  try {
    // Xvfb allocates a free display; do not attach to an existing desktop.
    let displayNumber = '';
    const display = start('Xvfb', ['-displayfd', '1', '-screen', '0', '640x480x24', '-nolisten', 'tcp', '-noreset']);
    display.stdout!.on('data', chunk => { displayNumber += String(chunk); });
    await expect.poll(() => {
      if (failure) throw failure;
      if (display.exitCode !== null || display.signalCode !== null) throw new Error(`Xvfb exited before display allocation: ${diagnostics}`);
      return /^\d+\n$/.test(displayNumber);
    }, { timeout: 15000, message: 'Private Xvfb must allocate its display' }).toBe(true);
    const displayName = `:${displayNumber.trim()}`;
    const paint = (color: string) => {
      const result = spawnSync('xsetroot', ['-display', displayName, '-solid', color], { encoding: 'utf8', timeout: 5000 });
      if (result.error) throw result.error;
      expect(result.status, result.stderr).toBe(0);
    };
    paint('#305070');
    const reservation = createServer();
    reservation.listen(0, '127.0.0.1'); await once(reservation, 'listening');
    const address = reservation.address();
    if (!address || typeof address === 'string') throw new Error('No fixture port');
    const port = address.port;
    await new Promise<void>((resolve, reject) => reservation.close(error => error ? reject(error) : resolve()));
    const authArgs = ['-nopw'];
    if (authenticated) {
      const passwordFile = join(temporary, 'vnc-password');
      const saved = spawnSync('x11vnc', ['-storepasswd', password, passwordFile], { encoding: 'utf8', timeout: 5000 });
      if (saved.error) throw saved.error;
      expect(saved.status).toBe(0);
      authArgs.splice(0, 1, '-rfbauth', passwordFile);
    }
    // This owned Xvfb display has no login manager. Avoid x11vnc's delayed
    // selection-window creation while it waits for a display manager to exit.
    start('x11vnc', ['-display', displayName, '-rfbport', String(port), '-localhost', '-forever', '-shared', ...authArgs, '-noxdamage', '-seldir', 'debug'], {
      ...process.env, X11VNC_AVOID_WINDOWS: 'never',
    });
    await expect.poll(() => {
      if (failure) throw failure;
      return diagnostics.includes('created selwin:');
    }, { timeout: 10000, message: 'x11vnc must create its X selection owner before clipboard input' }).toBe(true);
    await expect.poll(async () => {
      if (failure) throw failure;
      return new Promise<boolean>(resolve => {
        const socket = createConnection({ host: '127.0.0.1', port });
        socket.setTimeout(500);
        socket.once('connect', () => { socket.destroy(); resolve(true); });
        socket.once('error', () => { socket.destroy(); resolve(false); });
        socket.once('timeout', () => { socket.destroy(); resolve(false); });
      });
    }).toBe(true);
    const errors: string[] = [];
    const clipboardFrames: string[] = [];
    page.on('websocket', socket => socket.on('framesent', ({ payload }) => {
      // This x11vnc fixture negotiates the original RFB ClientCutText message.
      if (typeof payload !== 'string' && payload.length >= 8 && payload[0] === 6
        && payload.readUInt32BE(4) === payload.length - 8) {
        clipboardFrames.push(payload.subarray(8).toString('latin1'));
      }
    }));
    page.on('pageerror', error => errors.push(error.message));
    const canvas = page.locator('canvas[width="640"]').filter({ visible: true }).first();
    const pixel = () => canvas.evaluate(node => Array.from((node as HTMLCanvasElement).getContext('2d')!.getImageData(100, 100, 1, 1).data));
    await page.goto('/');
    await page.getByRole('button', { name: 'Desktops', exact: true }).click();
    await page.getByTestId('add-cell-0-0').filter({ visible: true }).click();
    await page.getByRole('dialog', { name: 'Add Graphics Session' }).getByRole('button', { name: /VNC/ }).click();
    const dialog = page.getByRole('dialog', { name: 'Connect to VNC Desktop' });
    await dialog.getByPlaceholder('hostname or IP').fill('127.0.0.1');
    await dialog.getByPlaceholder('5900').fill(String(port));
    await dialog.getByPlaceholder('Leave blank if none').fill(password);
    const created = page.waitForResponse(response => response.url().endsWith('/api/vnc/sessions') && response.request().method() === 'POST');
    await dialog.getByRole('button', { name: 'Connect', exact: true }).click();
    const response = await created;
    expect(response.ok()).toBe(true);
    id = (await response.json() as { id: string }).id;
    await expect.poll(pixel).toEqual([48, 80, 112, 255]);
    paint('#70a030');
    await expect.poll(pixel).toEqual([112, 160, 48, 255]);
    if (!authenticated) {
      // Passwords intentionally live only in browser memory; page reload is
      // covered by the unauthenticated case, not by persisting credentials.
      await page.reload();
      await page.getByRole('button', { name: 'Desktops', exact: true }).click();
      await expect.poll(pixel).toEqual([112, 160, 48, 255]);
    }
    await canvas.locator('..').locator('..').locator('..').dblclick({ position: { x: 100, y: 100 } });
    await expect(page.getByTitle('Back to grid', { exact: true })).toBeVisible();
    await expect.poll(pixel).toEqual([112, 160, 48, 255]);
    const bounds = await canvas.boundingBox();
    if (!bounds) throw new Error('No fullscreen canvas bounds');
    await canvas.click({ position: { x: 80, y: 60 } });
    await expect.poll(() => {
      const location = spawnSync('xdotool', ['getmouselocation', '--shell'], {
        encoding: 'utf8', timeout: 5000, env: { ...process.env, DISPLAY: displayName },
      });
      if (location.error) throw location.error;
      expect(location.status, location.stderr).toBe(0);
      const x = Number(/^X=(\d+)$/m.exec(location.stdout)?.[1]);
      const y = Number(/^Y=(\d+)$/m.exec(location.stdout)?.[1]);
      return Math.abs(x - 80 * 640 / bounds.width) <= 2 && Math.abs(y - 60 * 480 / bounds.height) <= 2;
    }).toBe(true);
    await verifyXKeyboard(page, displayName, start, chunk => {
      keyboardEvents = (keyboardEvents + chunk).slice(-65536);
    });
    // Exercise the actual browser menu and clipboard API. Read only the private
    // X display's clipboard, never the developer/runner's desktop clipboard.
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'], { origin: new URL(page.url()).origin });
    const clipboard = `WebMux clipboard ${randomBytes(8).toString('hex')}\nsecond line`;
    await page.evaluate(text => navigator.clipboard.writeText(text), clipboard);
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(clipboard);
    await page.getByTitle('VNC Options', { exact: true }).click();
    await page.getByText('Paste Clipboard', { exact: true }).click();
    await expect.poll(() => clipboardFrames, { message: 'Browser must send the clipboard payload over RFB' }).toContain(clipboard);
    await expect.poll(() => {
      const result = spawnSync('xclip', ['-selection', 'clipboard', '-out'], {
        encoding: 'utf8', timeout: 1000, maxBuffer: 65536,
        env: { ...process.env, DISPLAY: displayName },
      });
      if (result.error && (result.error as NodeJS.ErrnoException).code !== 'ETIMEDOUT') throw result.error;
      diagnostics = (diagnostics + `\nxclip status=${result.status} signal=${result.signal}: ${result.stderr}\n`).slice(-65536);
      return result.status === 0 ? result.stdout : null;
    }).toBe(clipboard);
    expect(errors).toEqual([]);
  } finally {
    try {
      if (id) expect((await request.delete(`/api/vnc/sessions/${id}`)).ok()).toBe(true);
    } finally {
      for (const child of children.reverse()) {
        if (!child.pid || child.exitCode !== null || child.signalCode !== null) continue;
        const exited = once(child, 'exit');
        child.kill('SIGTERM');
        const timer = setTimeout(() => child.kill('SIGKILL'), 3000);
        try { await exited; } finally { clearTimeout(timer); }
      }
      const log = info.outputPath('x11vnc.log');
      writeFileSync(log, diagnostics + '\nX keyboard observer:\n' + keyboardEvents);
      await info.attach('x11vnc-log', { path: log, contentType: 'text/plain' });
      rmSync(temporary, { recursive: true, force: true });
    }
  }
});
