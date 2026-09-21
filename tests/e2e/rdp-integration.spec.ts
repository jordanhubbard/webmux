import { test, expect } from '@playwright/test';
import { spawn, spawnSync, type ChildProcess } from 'node:child_process';
import { createServer, createConnection } from 'node:net';
import { once } from 'node:events';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { verifyXKeyboard } from './x-keyboard-fixture';

test('real guacd and FreeRDP screen updates, reconnection and keyboard input', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_RDP !== '1', 'Requires Linux Xvfb, guacd and FreeRDP shadow server');
  test.setTimeout(90_000);
  const temporary = mkdtempSync(join(tmpdir(), 'webmux-real-rdp-'));
  const children: ChildProcess[] = [];
  let diagnostics = '';
  let failure: Error | undefined;
  let testFailed = false;
  let id: string | undefined;
  const environment: NodeJS.ProcessEnv = { ...process.env, XDG_CONFIG_HOME: temporary };
  function start(command: string, args: string[], env = environment) {
    const child = spawn(command, args, { env, detached: true, stdio: ['ignore', 'pipe', 'pipe'] });
    children.push(child);
    child.on('error', error => { failure = error; });
    for (const stream of [child.stdout!, child.stderr!]) stream.on('data', chunk => {
      diagnostics = (diagnostics + `[${command}] ${String(chunk)}`).slice(-131072);
    });
    return child;
  }
  async function availablePort(port = 0) {
    const reservation = createServer();
    reservation.listen(port, '127.0.0.1'); await once(reservation, 'listening');
    const address = reservation.address();
    if (!address || typeof address === 'string') throw new Error('No fixture port');
    await new Promise<void>((resolve, reject) => reservation.close(error => error ? reject(error) : resolve()));
    return address.port;
  }
  async function listening(port: number) {
    await expect.poll(async () => {
      if (failure) throw failure;
      return new Promise<boolean>(resolve => {
        const socket = createConnection({ host: '127.0.0.1', port });
        socket.setTimeout(500);
        socket.once('connect', () => { socket.destroy(); resolve(true); });
        socket.once('error', () => { socket.destroy(); resolve(false); });
        socket.once('timeout', () => { socket.destroy(); resolve(false); });
      });
    }, { timeout: 15000 }).toBe(true);
  }
  try {
    let number = '';
    const display = start('Xvfb', ['-displayfd', '1', '-screen', '0', '1024x768x24', '-nolisten', 'tcp', '-noreset']);
    display.stdout!.on('data', chunk => { number += String(chunk); });
    await expect.poll(() => {
      if (failure) throw failure;
      if (display.exitCode !== null || display.signalCode !== null) throw new Error(`Xvfb exited before display allocation: ${diagnostics}`);
      return /^\d+\n$/.test(number);
    }, { timeout: 15000, message: 'Private Xvfb must allocate its display' }).toBe(true);
    const displayName = `:${number.trim()}`;
    const paint = (color: string) => {
      const result = spawnSync('xsetroot', ['-display', displayName, '-solid', color], { encoding: 'utf8', timeout: 5000 });
      if (result.error) throw result.error;
      expect(result.status, result.stderr).toBe(0);
    };
    paint('#0000ff');
    const port = await availablePort();
    start('freerdp-shadow-cli', [`/port:${port}`, '/bind-address:127.0.0.1', '/sec:rdp', '-auth', '+may-view', '+may-interact'], {
      ...environment, DISPLAY: displayName,
    });
    await listening(port);
    const guacdPort = await availablePort(14823);
    start('/usr/sbin/guacd', ['-f', '-b', '127.0.0.1', '-l', String(guacdPort), '-p', join(temporary, 'guacd.pid'), '-L', 'info']);
    await listening(guacdPort);
    const response = await request.post('/api/rdp/sessions', { data: { hostname: '127.0.0.1', rdp_port: port, rdp_username: 'fixture' } });
    expect(response.ok()).toBe(true);
    id = (await response.json() as { id: string }).id;
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.goto('/');
    await page.getByRole('button', { name: 'Desktops', exact: true }).click();
    const canvas = page.locator('canvas[width="1024"]').filter({ visible: true }).first();
    const pixel = () => canvas.evaluate(node => Array.from((node as HTMLCanvasElement).getContext('2d')!.getImageData(100, 100, 1, 1).data));
    await expect.poll(pixel, { timeout: 20000 }).toEqual([0, 0, 255, 255]);
    paint('#00ff00');
    await expect.poll(pixel).toEqual([0, 255, 0, 255]);
    await page.reload();
    await page.getByRole('button', { name: 'Desktops', exact: true }).click();
    await expect.poll(pixel, { timeout: 20000 }).toEqual([0, 255, 0, 255]);
    await canvas.locator('xpath=ancestor::div[@data-1p-ignore]/..').dblclick({ position: { x: 100, y: 100 } });
    await expect(page.getByTitle('Back to grid', { exact: true })).toBeVisible();
    await expect.poll(pixel, { timeout: 20000 }).toEqual([0, 255, 0, 255]);
    // Guacamole's layered canvases are covered by its input container. Click
    // the same focusable viewer used by the browser interaction parity test.
    await page.locator('div[data-1p-ignore][tabindex="0"]').click({ position: { x: 80, y: 60 }, timeout: 5000 });
    await verifyXKeyboard(page, displayName, start, () => {
      // start() already retains every child stdout/stderr chunk in diagnostics.
    });
    expect(errors).toEqual([]);
  } catch (error) {
    testFailed = true;
    throw error;
  } finally {
    try {
      if (id) expect((await request.delete(`/api/rdp/sessions/${id}`)).ok()).toBe(true);
    } catch (error) {
      if (!testFailed) throw error;
      diagnostics += `\nSession cleanup after test failure: ${String(error)}\n`;
    } finally {
      // Each child owns a new process group, including guacd connection workers.
      for (const child of children.reverse()) {
        if (!child.pid) continue;
        const exited = child.exitCode !== null || child.signalCode !== null ? Promise.resolve() : once(child, 'exit');
        const signal = (value: NodeJS.Signals) => {
          try { process.kill(-child.pid!, value); } catch (error) {
            if ((error as NodeJS.ErrnoException).code !== 'ESRCH') throw error;
          }
        };
        signal('SIGTERM');
        const timer = setTimeout(() => signal('SIGKILL'), 3000);
        try { await exited; } finally { clearTimeout(timer); signal('SIGKILL'); }
      }
      try {
        const log = info.outputPath('rdp-server.log');
        writeFileSync(log, diagnostics);
        await info.attach('rdp-server-log', { path: log, contentType: 'text/plain' });
      }
      finally { rmSync(temporary, { recursive: true, force: true }); }
    }
  }
});
