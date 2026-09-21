import { test, expect } from '@playwright/test';
import { spawn, spawnSync, type ChildProcess } from 'node:child_process';
import { createServer, createConnection } from 'node:net';
import { once } from 'node:events';

test('real x11vnc screen updates and browser reconnection', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_VNC !== '1', 'Requires Linux Xvfb, x11vnc and xsetroot');
  test.setTimeout(60_000);
  const children: ChildProcess[] = [];
  let diagnostics = '';
  let failure: Error | undefined;
  let id: string | undefined;
  function start(command: string, args: string[]) {
    const child = spawn(command, args, { stdio: ['ignore', 'pipe', 'pipe'] });
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
      return /^\d+\n$/.test(displayNumber);
    }).toBe(true);
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
    start('x11vnc', ['-display', displayName, '-rfbport', String(port), '-localhost', '-forever', '-shared', '-nopw', '-noxdamage']);
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
    const response = await request.post('/api/vnc/sessions', { data: { hostname: '127.0.0.1', vnc_port: port } });
    expect(response.ok()).toBe(true);
    id = (await response.json() as { id: string }).id;
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    const canvas = page.locator('canvas[width="640"]').filter({ visible: true }).first();
    const pixel = () => canvas.evaluate(node => Array.from((node as HTMLCanvasElement).getContext('2d')!.getImageData(100, 100, 1, 1).data));
    await page.goto('/');
    await page.getByRole('button', { name: 'Desktops', exact: true }).click();
    await expect.poll(pixel).toEqual([48, 80, 112, 255]);
    paint('#70a030');
    await expect.poll(pixel).toEqual([112, 160, 48, 255]);
    await page.reload();
    await page.getByRole('button', { name: 'Desktops', exact: true }).click();
    await expect.poll(pixel).toEqual([112, 160, 48, 255]);
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
      await info.attach('x11vnc-log', { body: diagnostics, contentType: 'text/plain' });
    }
  }
});
