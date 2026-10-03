import { test, expect } from '@playwright/test';
import http from 'node:http';
import { writeFileSync, rmSync } from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

for (const linkKind of ['plain', 'osc8', 'gh-browser', 'browser', 'open', 'xdg-open'] as const) {
test(`browser companion routes ${linkKind} authentication and completes a loopback callback`, async ({ page, request }) => {
  test.setTimeout(90_000);
  const shellLaunch = linkKind !== 'plain' && linkKind !== 'osc8';
  test.skip(shellLaunch && process.platform === 'win32', 'Local tmux shell handoff requires Unix');
  // This is a real browser acceptance test, not a mocked frame or protocol.
  let received = '';
  const callback = http.createServer((req, res) => {
    const url = new URL(req.url!, 'http://localhost');
    if (url.pathname === '/callback') received = url.searchParams.get('code') ?? '';
    res.end('<h1>Authorization complete. Return to your terminal.</h1>');
  });
  await new Promise<void>(resolve => callback.listen(0, '127.0.0.1', resolve));
  const callbackPort = (callback.address() as { port: number }).port;
  const identity = http.createServer((_req, res) => {
    res.setHeader('Content-Type', 'text/html');
    res.end(`<html><body style="margin:0"><form action="http://127.0.0.1:${callbackPort}/callback">
      <input name="code" aria-label="Authorization code" style="position:absolute;left:40px;top:40px;width:300px;height:40px">
      <button style="position:absolute;left:40px;top:120px;width:200px;height:40px">Authorize</button>
      </form></body></html>`);
  });
  await new Promise<void>(resolve => identity.listen(0, '127.0.0.1', resolve));
  const identityURL = `http://127.0.0.1:${(identity.address() as { port: number }).port}/authorize`;
  const created = await request.post('/api/sessions', { data: { transport: 'local', cols: 80, rows: 24 } });
  expect(created.ok()).toBe(true);
  const session = await created.json();
  const launchConfig = path.resolve(__dirname, '.test-home', `browser-launch-${session.id}.json`);
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
      await page.getByTestId(`tile-cell-${session.id}`).locator('.xterm-helper-textarea').focus();
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
      const box = (await line.boundingBox())!;
      await page.mouse.move(box.x + 25, box.y + box.height / 2);
      await page.mouse.click(box.x + 25, box.y + box.height / 2);
    } else {
      await page.getByRole('button', { name: 'Open browser', exact: true }).click();
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
    const bounds = (await viewport.boundingBox())!;
    await viewport.click({ position: { x: 100 * bounds.width / 1100, y: 60 * bounds.height / 760 } });
    await page.keyboard.type('callback-proof');
    await page.keyboard.press('Tab');
    await page.keyboard.press('Enter');
    await expect.poll(() => received, { timeout: 15_000 }).toBe('callback-proof');
    expect(popups).toBe(0);
    expect(page.context().pages()).toHaveLength(1);
    await expect(page.getByRole('textbox', { name: 'Remote browser URL' })).toHaveValue(/\/callback\?code=callback-proof/);
    const before = await request.post(`/api/sessions/${session.id}/browser`, { data: { action: 'frame' } });
    const target = (await before.json()).target;
    await page.screenshot({ path: 'test-results/browser-companion.png' });
    await page.getByRole('button', { name: 'Return to terminal' }).click();
    await expect(viewport).not.toBeVisible();
    await page.getByRole('button', { name: 'Open browser', exact: true }).click();
    await expect(viewport).toBeVisible();
    const reopened = await request.post(`/api/sessions/${session.id}/browser`, { data: { action: 'frame' } });
    expect((await reopened.json()).target).toBe(target);
    await page.getByRole('button', { name: 'End browser', exact: true }).click();
    await expect(viewport).not.toBeVisible();
    const ended = await request.post(`/api/sessions/${session.id}/browser`, { data: { action: 'frame' } });
    expect(ended.status()).toBe(503);
  } finally {
    rmSync(launchConfig, { force: true });
    await request.delete(`/api/sessions/${session.id}`);
    // Local terminals intentionally persist in tmux after disconnection.
    try { execFileSync('tmux', ['kill-session', '-t', `webmux-${session.id}`]); } catch { /* already exited */ }
    await Promise.all([new Promise<void>(resolve => identity.close(() => resolve())), new Promise<void>(resolve => callback.close(() => resolve()))]);
  }
});

}
