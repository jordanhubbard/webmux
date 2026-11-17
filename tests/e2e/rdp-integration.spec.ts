import { test, expect, type Page, type WebSocketRoute } from '@playwright/test';
import { spawn, spawnSync, type ChildProcess } from 'node:child_process';
import { createServer, createConnection } from 'node:net';
import { once } from 'node:events';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { verifyXKeyboard } from './x-keyboard-fixture';
import { rdpPixelMatches } from './rdp-pixels';

for (const stalledPrevious of [false, true]) {
test(`real guacd and FreeRDP screen updates, reconnection and keyboard input${stalledPrevious ? ' with an unresponsive previous viewer' : ''}`, async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_RDP !== '1', 'Requires Linux Xvfb, guacd and FreeRDP shadow server');
  test.setTimeout(90_000);
  const temporary = mkdtempSync(join(tmpdir(), 'webmux-real-rdp-'));
  const children: ChildProcess[] = [];
  let diagnostics = '';
  let guacdOutput = '';
  let guacdExecutable: string | undefined;
  let failure: Error | undefined;
  let testFailed = false;
  let id: string | undefined;
  const startedAt = Date.now();
  let phase = 'setup';
  const events: Record<string, unknown>[] = [];
  const record = (kind: string, detail: Record<string, unknown> = {}) => {
    events.push({ ms: Date.now() - startedAt, phase, kind, ...detail });
    if (events.length > 512) events.shift();
  };
  const enter = (name: string) => { phase = name; record('phase'); };
  const currentConnection = new Map<Page, { jpeg: boolean; guacdId?: string }>();
  const connections: { index: number; viewer: string; clientClosed: boolean; jpeg: boolean; guacdId?: string; sent: Record<string, number>; received: Record<string, number> }[] = [];
  const observe = (viewer: Page, name: string) => viewer.on('websocket', socket => {
    if (!new URL(socket.url()).pathname.startsWith('/api/rdp/ws/')) return;
    expect(connections.length, 'bounded RDP connection inventory').toBeLessThan(16);
    const connection = { index: connections.length, viewer: name, clientClosed: false, jpeg: false, guacdId: undefined as string | undefined, sent: {} as Record<string, number>, received: {} as Record<string, number> };
    currentConnection.set(viewer, connection);
    connections.push(connection); record('websocket-open', { index: connection.index });
    const frame = (direction: 'sent' | 'received', payload: string | Buffer) => {
      const data = payload.toString();
      if (direction === 'received' && !connection.jpeg && (/(?:^|;)4\.jpeg,/.test(data) || data.includes(',10.image/jpeg,'))) {
        connection.jpeg = true;
        record('image-codec', { index: connection.index, mime: 'image/jpeg' });
      }
      let opcode = /^\d+\.([a-z]{1,16})[,;]/.exec(data)?.[1] ?? 'unknown';
      const counts = connection[direction];
      if (!(opcode in counts) && Object.keys(counts).length >= 31) opcode = 'other';
      counts[opcode] = (counts[opcode] ?? 0) + 1;
      if (direction === 'received' && opcode === 'ready') connection.guacdId = /\$[a-f0-9-]{36}/i.exec(data)?.[0];
      if (['ready', 'disconnect', 'error'].includes(opcode)) record('instruction', { index: connection.index, direction, opcode });
    };
    socket.on('framesent', value => frame('sent', value.payload));
    socket.on('framereceived', value => frame('received', value.payload));
    socket.on('close', () => { connection.clientClosed = true; if (name === 'previous' && held) held.closed = true; record('websocket-close', { index: connection.index }); });
  });
  observe(page, 'active');
  let previous: Page | undefined;
  let stallOld = false;
  let held: { route: WebSocketRoute; closed: boolean; blocked: number } | undefined;
  const environment: NodeJS.ProcessEnv = { ...process.env, XDG_CONFIG_HOME: temporary };
  function start(command: string, args: string[], env = environment) {
    const child = spawn(command, args, { env, detached: true, stdio: ['ignore', 'pipe', 'pipe'] });
    children.push(child);
    child.once('spawn', () => record('process-start', { command, pid: child.pid }));
    child.once('exit', (code, signal) => record('process-exit', { command, pid: child.pid, code, signal }));
    child.on('error', error => { failure = error; });
    for (const stream of [child.stdout!, child.stderr!]) stream.on('data', chunk => {
      if (command === guacdExecutable) guacdOutput = (guacdOutput + String(chunk)).slice(-131072);
      diagnostics = (diagnostics + `[${Date.now() - startedAt}ms ${command}] ${String(chunk)}`).slice(-131072);
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
    if (stalledPrevious) {
      // Keep the old document alive while withholding its outbound protocol
      // traffic; navigation would dispose its route and close the upstream.
      previous = await page.context().newPage();
      observe(previous, 'previous');
      await previous.routeWebSocket(/\/api\/rdp\/ws\//, socket => {
        const upstream = { route: socket.connectToServer(), closed: false, blocked: 0 };
        held = upstream;
        socket.onMessage(data => {
          if (stallOld) { if (/^\d+\.(?:ack|sync),/.test(data.toString())) upstream.blocked++; }
          else upstream.route.send(data);
        });
      });
    }
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
      record('paint', { color });
    };
    paint('#0000ff');
    const port = await availablePort();
    start('freerdp-shadow-cli', [`/port:${port}`, '/bind-address:127.0.0.1', '/sec:rdp', '-auth', '+may-view', '+may-interact'], {
      ...environment, DISPLAY: displayName,
    });
    await listening(port);
    const guacdPort = await availablePort(14823);
    const guacd = process.env.WEBMUX_GUACD_EXECUTABLE || '/usr/sbin/guacd';
    const registrationShim = process.env.WEBMUX_GUACD_REGISTRATION_SHIM;
    const codecShim = join(temporary, 'codec.so');
    const flags = spawnSync('pkg-config', ['--cflags', '--libs', 'freerdp2'], { encoding: 'utf8', timeout: 5000 });
    expect(flags.status, flags.stderr).toBe(0);
    const compile = spawnSync('cc', ['-Wall', '-Wextra', '-Werror', '-shared', '-fPIC',
      resolve('../tests/e2e/guacd-codec-fixture.c'), '-Wl,--no-as-needed', ...flags.stdout.trim().split(/\s+/), '-ldl', '-o', codecShim],
      { encoding: 'utf8', timeout: 30000 });
    expect(compile.status, compile.stderr).toBe(0);
    guacdExecutable = guacd;
    start(guacd, ['-f', '-b', '127.0.0.1', '-l', String(guacdPort), '-p', join(temporary, 'guacd.pid'), '-L', 'info'], {
      ...environment, LD_PRELOAD: [codecShim, registrationShim].filter(Boolean).join(':'),
      ...(registrationShim ? { WEBMUX_RDP_SCHEDULE_MARKER: join(temporary, 'schedule.marker') } : {}),
    });
    await listening(guacdPort);
    const response = await request.post('/api/rdp/sessions', { data: { hostname: '127.0.0.1', rdp_port: port, rdp_username: 'fixture' } });
    expect(response.ok()).toBe(true);
    id = (await response.json() as { id: string }).id;
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    const readPixel = (viewer: Page) => viewer.locator('canvas[width="1024"]').filter({ visible: true }).first()
      .evaluate(node => Array.from((node as HTMLCanvasElement).getContext('2d')!.getImageData(100, 100, 1, 1).data));
    const expectPixel = async (viewer: Page, expected: number[], timeout = 5000) => {
      await expect.poll(async () => {
        const rgba = await readPixel(viewer);
        const connection = currentConnection.get(viewer);
        const joined = connection?.guacdId && guacdOutput.split('\n').find(line => line.includes(`joined connection "${connection.guacdId}"`));
        const pid = joined && /guacd\[(\d+)\]/.exec(joined)?.[1];
        const rfx = pid && guacdOutput.includes(`RDP_CODEC pid=${pid} codec=remotefx`);
        const codec = connection?.jpeg ? 'jpeg' : rfx ? 'remotefx' : undefined;
        record('pixel', { rgba, codec, guacdPid: pid, viewer: viewer === page ? 'active' : 'previous' });
        return rdpPixelMatches(rgba, expected, codec);
      }, { timeout, message: `RDP frame must match ${expected} (rounding requires actual lossy codec evidence)` }).toBe(true);

    };
    enter('initial');
    if (previous) {
      await previous.goto('/');
      await previous.getByRole('button', { name: 'Desktops', exact: true }).click();
      await expectPixel(previous, [0, 0, 255, 255], 20000);
      expect(connections.filter(value => value.viewer === 'previous')).toHaveLength(1);
    }
    await page.goto('/');
    await page.getByRole('button', { name: 'Desktops', exact: true }).click();
    const canvas = page.locator('canvas[width="1024"]').filter({ visible: true }).first();
    await expectPixel(page, [0, 0, 255, 255], 20000);
    enter('update');
    paint('#00ff00');
    await expectPixel(page, [0, 255, 0, 255]);
    if (previous) await expectPixel(previous, [0, 255, 0, 255]);
    enter('reload');
    stallOld = stalledPrevious;
    if (stalledPrevious) {
      record('old-upstream-stalled');
      // guacd 1.6 does not keep sending frames for an unchanged desktop.
      // Produce a real update so this control proves acknowledgements were
      // withheld before the replacement viewer connects.
      paint('#ff00ff');
      await expect.poll(() => held?.blocked ?? 0, { message: 'old viewer must withhold a real update acknowledgement' }).toBeGreaterThan(0);
      paint('#00ff00');
    }
    await page.reload();
    await page.getByRole('button', { name: 'Desktops', exact: true }).click();
    await expectPixel(page, [0, 255, 0, 255], 20000);
    if (stalledPrevious) {
      await expect.poll(() => held?.blocked ?? 0, { message: 'old viewer acknowledgements must actually be withheld' }).toBeGreaterThan(0);
      expect(held?.closed, 'new viewer must draw before the old upstream expires').toBe(false);
    }
    enter('fullscreen');
    await canvas.locator('xpath=ancestor::div[@data-1p-ignore]/..').dblclick({ position: { x: 100, y: 100 } });
    await expect(page.getByTitle('Back to grid', { exact: true })).toBeVisible();
    await expectPixel(page, [0, 255, 0, 255], 20000);
    // Guacamole's layered canvases are covered by its input container. Click
    // the same focusable viewer used by the browser interaction parity test.
    await page.locator('div[data-1p-ignore][tabindex="0"]').click({ position: { x: 80, y: 60 }, timeout: 5000 });
    enter('keyboard');
    await verifyXKeyboard(page, displayName, start, () => {
      // start() already retains every child stdout/stderr chunk in diagnostics.
    });
    expect(errors).toEqual([]);
  } catch (error) {
    testFailed = true;
    record('failure', { message: String(error).slice(0, 1000) });
    const canvases = await page.locator('canvas').evaluateAll(nodes => nodes.slice(0, 16).map(node => {
      const canvas = node as HTMLCanvasElement;
      const bounds = canvas.getBoundingClientRect();
      try { return { width: canvas.width, height: canvas.height, visible: bounds.width > 0 && bounds.height > 0, rgba: canvas.width > 100 && canvas.height > 100 ? Array.from(canvas.getContext('2d')!.getImageData(100, 100, 1, 1).data) : null }; }
      catch { return { width: canvas.width, height: canvas.height, unreadable: true }; }
    })).catch(() => []);
    record('failure-canvases', { canvases });
    await page.screenshot({ path: info.outputPath('rdp-failure.png') }).catch(() => {});
    throw error;
  } finally {
    enter('cleanup');
    try {
      record('old-upstream-final', { closed: held?.closed, blocked: held?.blocked });
      stallOld = false;
      await previous?.close();
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
        const trace = info.outputPath('rdp-reconnect.json');
        writeFileSync(trace, JSON.stringify({ stalledPrevious, connections, events }, null, 2));
        await info.attach('rdp-reconnect', { path: trace, contentType: 'application/json' });
      }
      finally { rmSync(temporary, { recursive: true, force: true }); }
    }
  }
});
}
