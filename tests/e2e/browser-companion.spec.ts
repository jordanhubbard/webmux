import { test, expect } from '@playwright/test';
import http from 'node:http';
import { writeFileSync, rmSync } from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

for (const linkKind of ['plain', 'osc8', 'gh-browser', 'browser', 'open', 'xdg-open'] as const) {
test(`browser companion routes ${linkKind} authentication and completes a loopback callback`, async ({ page, request }, info) => {
  test.setTimeout(90_000);
  const replies: unknown[] = [];
  page.on('response', async response => {
    if (!response.url().endsWith('/browser')) return;
    try {
      const { url, error } = await response.json();
      const sent = response.request().postDataJSON() as { action?: string; url?: string } | null;
      replies.push({ status: response.status(), action: sent?.action, requestedURL: sent?.url, url, error });
      if (replies.length > 20) replies.shift();
    } catch { /* request may have been canceled during teardown */ }
  });
  const shellLaunch = linkKind !== 'plain' && linkKind !== 'osc8';
  test.skip(shellLaunch && process.platform === 'win32', 'Local tmux shell handoff requires Unix');
  // This is a real browser acceptance test, not a mocked frame or protocol.
  let received = '';
  let pageReady = false;
  const inputReceipts: unknown[] = [];
  let inputGeometry: unknown;
  const callback = http.createServer((req, res) => {
    const url = new URL(req.url!, 'http://localhost');
    if (url.pathname === '/callback') received = url.searchParams.get('code') ?? '';
    if (url.pathname === '/ready') pageReady = true;
    res.end('<h1>Authorization complete. Return to your terminal.</h1>');
  });
  await new Promise<void>(resolve => callback.listen(0, '127.0.0.1', resolve));
  const callbackPort = (callback.address() as { port: number }).port;
  const identity = http.createServer((req, res) => {
    if (req.url === '/fixture-input' && req.method === 'POST') {
      // Only this synthetic identity document can send fixture receipts. Bound
      // storage so diagnostics cannot turn a failing input loop into a leak.
      let body = '';
      req.on('data', chunk => { if (body.length <= 8192) body += chunk.toString().slice(0, 8193 - body.length); });
      req.on('end', () => {
        if (body.length <= 8192 && inputReceipts.length < 100) {
          try { inputReceipts.push(JSON.parse(body)); } catch { /* malformed fixture receipt */ }
        }
        res.end();
      });
      return;
    }
    res.setHeader('Content-Type', 'text/html');
    res.end(`<html><body style="margin:0"><form action="http://127.0.0.1:${callbackPort}/callback">
      <input name="code" aria-label="Authorization code" style="position:absolute;left:40px;top:40px;width:300px;height:40px">
      <button style="position:absolute;left:40px;top:120px;width:200px;height:40px">Authorize</button>
      </form><script>
      let sequence = 0;
      for (const type of ['pointerdown', 'focusin', 'focusout', 'keydown', 'beforeinput', 'input', 'submit']) {
        document.addEventListener(type, event => {
          const input = document.querySelector('input');
          const rect = input.getBoundingClientRect();
          const describe = element => ({ tag: element?.tagName, name: element?.getAttribute?.('name') });
          navigator.sendBeacon('/fixture-input', JSON.stringify({
            sequence: ++sequence, type, key: event.key, target: describe(event.target),
            active: describe(document.activeElement), value: input.value,
            x: event.clientX, y: event.clientY, width: innerWidth, height: innerHeight,
            input: { x: rect.x, y: rect.y, width: rect.width, height: rect.height }
          }));
        }, true);
      }
      addEventListener('DOMContentLoaded', () => fetch('http://127.0.0.1:${callbackPort}/ready', { mode: 'no-cors' }));
      </script></body></html>`);
  });
  await new Promise<void>(resolve => identity.listen(0, '127.0.0.1', resolve));
  const identityURL = `http://127.0.0.1:${(identity.address() as { port: number }).port}/authorize`;
  const created = await request.post('/api/sessions', { data: { transport: 'local', cols: 80, rows: 24 } });
  expect(created.ok()).toBe(true);
  const session = await created.json();
  const launchConfig = path.resolve(__dirname, '.test-home', `browser-launch-${session.id}.json`);
  // Keep an earlier viewer connected: the active terminal must claim browser
  // requests instead of allowing the first tab to receive them forever.
  const idleViewer = linkKind === 'gh-browser' ? await page.context().newPage() : null;
  if (idleViewer) {
    await idleViewer.goto('/');
    await expect(idleViewer.getByTestId(`tile-cell-${session.id}`).locator('.xterm-screen')).toBeVisible();
  }
  try {
    if (linkKind === 'osc8') {
      // Some tmux/terminfo combinations strip OSC 8. Inject that exact PTY
      // output at the transport boundary to exercise xterm's native link path.
      await page.routeWebSocket(/\/api\/term\//, socket => {
        const server = socket.connectToServer();
        server.onMessage(raw => {
          const message = JSON.parse(raw.toString());
          if (message.type === 'output') message.data = message.data.replaceAll('WEBMUX_OSC8_FIXTURE',
            `\x1b]8;;${identityURL}\x1b\\Remote sign-in\x1b]8;;\x1b\\`);
          socket.send(JSON.stringify(message));
        });
      });
    }
    await page.goto('/');
    let popups = 0;
    page.on('popup', () => { popups++; });
    if (shellLaunch) {
      writeFileSync(launchConfig, JSON.stringify({ url: identityURL, mode: linkKind }), { mode: 0o600 });
      const quote = (value: string) => "'" + value.replaceAll("'", "'\\''") + "'";
      await page.getByTestId(`tile-cell-${session.id}`).locator('.xterm-screen').click({ position: { x: 30, y: 100 } });
      await page.keyboard.type([process.execPath, path.resolve(__dirname, 'browser-launch-fixture.mts'), launchConfig].map(quote).join(' '));
      await page.keyboard.press('Enter');
      await expect(page.locator('.xterm-rows')).toContainText('Press Enter to launch the authentication browser.');
      // This is the user's exact action: Enter inside the CLI, no link click or URL paste.
      await page.keyboard.press('Enter');
    } else if (process.platform !== 'win32') {
      // Drive an actual CLI-produced terminal link, not the companion URL field.
      const input = page.getByTestId(`tile-cell-${session.id}`).locator('.xterm-helper-textarea');
      await input.focus();
      const command = linkKind === 'plain'
        ? String.raw`printf '\n${identityURL}\n'`
        : String.raw`printf '\nWEBMUX_OSC8_FIXTURE\n'`;
      await page.keyboard.type(command);
      await page.keyboard.press('Enter');
      const linkText = linkKind === 'plain' ? identityURL : 'Remote sign-in';
      const line = page.locator('.xterm-rows').getByText(linkText, { exact: true }).last();
      await expect(line).toBeVisible();
      // Row spans are an accessibility mirror; xterm-screen receives pointer
      // events. Reacquire geometry if a redraw replaces the span mid-click.
      await expect(async () => {
        const choice = page.getByRole('button', { name: 'Use browser on terminal host' });
        if (await choice.isVisible()) return;
        const box = await line.boundingBox();
        if (!box) throw new Error('Terminal row was redrawn before link activation');
        await page.mouse.move(box.x + 25, box.y + box.height / 2);
        await page.mouse.click(box.x + 25, box.y + box.height / 2);
        await expect(choice).toBeVisible({ timeout: 1000 });
      }).toPass({ timeout: 10_000 });
    } else {
      await page.getByRole('button', { name: 'Session tools', exact: true }).click();
      await page.getByRole('button', { name: 'Open browser', exact: true }).click();
    }
    if (process.platform !== 'win32') {
      await page.getByRole('button', { name: 'Use browser on terminal host' }).click();
    }
    const viewport = page.getByAltText('Interactive remote browser viewport');
    await expect(viewport).toBeVisible({ timeout: 30_000 });
    if (process.platform === 'win32') {
      await page.getByRole('textbox', { name: 'Remote browser URL' }).fill(identityURL);
      await page.getByRole('button', { name: 'Go', exact: true }).click();
    }
    // Wait for navigation to reach the worker before interacting with its frame.
    await expect.poll(async () => {
      const res = await request.post(`/api/sessions/${session.id}/browser`, { data: { action: 'frame' } });
      return (await res.json()).url;
    }).toBe(identityURL);
    // Navigation can report its URL before the new document can receive input.
    await expect.poll(() => pageReady).toBe(true);
    // The viewport now follows the drawer. Match the displayed frame to the
    // worker geometry before converting page coordinates into viewer pixels.
    let geometry = { width: 1100, height: 760 };
    await expect.poll(async () => {
      const response = await request.post(`/api/sessions/${session.id}/browser`, { data: { action: 'frame' } });
      const remote = await response.json();
      const displayed = await viewport.evaluate(el => ({ width: (el as HTMLImageElement).naturalWidth, height: (el as HTMLImageElement).naturalHeight }));
      geometry = { width: remote.width, height: remote.height };
      return displayed.width === remote.width && displayed.height === remote.height;
    }).toBe(true);
    const bounds = (await viewport.boundingBox())!;
    inputGeometry = { bounds, geometry };
    await viewport.click({ position: { x: 100 * bounds.width / geometry.width, y: 60 * bounds.height / geometry.height } });
    if (linkKind === 'gh-browser') {
      // Real combined modifiers previously failed JSON decoding at both hops.
      await page.keyboard.press('Control+Shift+ArrowRight');
      await page.keyboard.press('Meta+Shift+ArrowRight');
      await expect(page.getByRole('alert')).toHaveCount(0);
    }
    await page.keyboard.type('callback-proof');
    await page.keyboard.press('Tab');
    await page.keyboard.press('Enter');
    await expect(page.getByRole('alert')).toHaveCount(0);
    await expect.poll(() => received, { timeout: 15_000 }).toBe('callback-proof');
    await expect.poll(() => inputReceipts).toEqual(expect.arrayContaining([
      expect.objectContaining({ type: 'pointerdown', target: { tag: 'INPUT', name: 'code' } }),
      expect.objectContaining({ type: 'input', value: 'callback-proof' }),
      expect.objectContaining({ type: 'submit', value: 'callback-proof' }),
    ]));
    const receiptPath = info.outputPath('browser-input-receipts.json');
    writeFileSync(receiptPath, JSON.stringify({ linkKind, received, inputGeometry, inputReceipts }));
    await info.attach('browser-input-receipts', {
      path: receiptPath,
      contentType: 'application/json',
    });
    expect(popups).toBe(0);
    expect(page.context().pages()).toHaveLength(idleViewer ? 2 : 1);
    if (idleViewer) await expect(idleViewer.locator('aside.browser-companion')).toHaveCount(0);
    await expect(page.getByRole('textbox', { name: 'Remote browser URL' })).toHaveValue(/\/callback\?code=callback-proof/, { timeout: 20_000 }).catch(async error => {
      const state = await page.evaluate(() => ({ visibility: document.visibilityState, active: document.activeElement?.tagName }));
      await info.attach('browser-navigation-diagnostics', { body: JSON.stringify({ state, replies }), contentType: 'application/json' });
      console.error(JSON.stringify({ state, replies }));
      throw error;
    });
    const before = await request.post(`/api/sessions/${session.id}/browser`, { data: { action: 'frame' } });
    const target = (await before.json()).target;
    await page.screenshot({ path: 'test-results/browser-companion.png' });
    await page.getByRole('button', { name: 'Return to terminal' }).click();
    await expect(viewport).not.toBeVisible();
    await page.getByRole('button', { name: 'Session tools', exact: true }).click();
    await page.getByRole('button', { name: 'Open browser', exact: true }).click();
    await expect(viewport).toBeVisible();
    const reopened = await request.post(`/api/sessions/${session.id}/browser`, { data: { action: 'frame' } });
    expect((await reopened.json()).target).toBe(target);
    await page.getByRole('button', { name: 'End browser', exact: true }).click();
    await expect(viewport).not.toBeVisible();
    const ended = await request.post(`/api/sessions/${session.id}/browser`, { data: { action: 'frame' } });
    expect(ended.status()).toBe(503);
  } catch (error) {
    const viewer = await page.evaluate(() => ({
      visibility: document.visibilityState,
      active: { tag: document.activeElement?.tagName, label: document.activeElement?.getAttribute('aria-label') },
    })).catch(() => null);
    const diagnosticsPath = info.outputPath('browser-input-diagnostics.json');
    writeFileSync(diagnosticsPath, JSON.stringify({ linkKind, received, pageReady, inputGeometry, inputReceipts, viewer, replies }));
    await info.attach('browser-input-diagnostics', {
      path: diagnosticsPath,
      contentType: 'application/json',
    });
    console.error('Browser companion workflow failed:', String(error).split('\n').slice(0, 4).join('\n'));
    throw error;
  } finally {
    await idleViewer?.close();
    rmSync(launchConfig, { force: true });
    await request.delete(`/api/sessions/${session.id}`).catch(() => { /* the request fixture may already be closed after a timeout */ });
    // Local terminals intentionally persist in tmux after disconnection.
    try { execFileSync('tmux', ['kill-session', '-t', `webmux-${session.id}`]); } catch { /* already exited */ }
    identity.closeAllConnections(); callback.closeAllConnections();
    await Promise.all([new Promise<void>(resolve => identity.close(() => resolve())), new Promise<void>(resolve => callback.close(() => resolve()))]);
  }
});

}

// Each case loads a complete workspace; the full suite shares one client IP.
// Let the production API budget reset before unrelated workspace tests start.
// Browser input itself has its own independently tested rate limit.
test.afterAll(async ({ request }) => {
  test.setTimeout(75_000);
  const response = await request.get('/api/auth/status');
  const seconds = Number(response.headers()['ratelimit-reset']);
  if (Number.isFinite(seconds) && seconds > 0) {
    await new Promise(resolve => setTimeout(resolve, Math.min(seconds, 60) * 1000 + 100));
  }
});
