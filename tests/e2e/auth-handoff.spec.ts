import { test, expect } from '@playwright/test';
import http from 'node:http';
import { writeFileSync, rmSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import path from 'node:path';

for (const mode of ['device', 'callback']) {
  test(`session sign-in completes ${mode} flow in viewer browser and originating CLI`, async ({ page, request }) => {
    test.skip(process.platform === 'win32', 'Fixture runs in a local tmux shell');
    test.setTimeout(90_000);
    let approved = false;
    const identity = http.createServer((req, res) => {
      const url = new URL(req.url!, 'http://localhost');
      res.setHeader('Content-Type', 'text/html');
      if (url.pathname.endsWith('/token')) { res.end(JSON.stringify({ approved })); return; }
      if (url.pathname === '/approve') {
        approved = url.searchParams.get('code') === 'FIXTURE-1234';
        res.end('Return to WebMux'); return;
      }
      const callback = url.searchParams.get('redirect_uri');
      if (callback) {
        res.end(`<a href="${callback}?state=fixture-state&code=fixture-code">Approve sign-in</a>`);
      } else {
        res.end('<form action="/approve"><input name="code" aria-label="Device code"><button>Approve sign-in</button></form>');
      }
    });
    await new Promise<void>(resolve => identity.listen(0, '127.0.0.1', resolve));
    const port = (identity.address() as { port: number }).port;
    // Use a real provider-shaped HTTPS URL intercepted only at the browser for
    // device flow. The CLI polls the fixture's separate HTTP token endpoint.
    const identityURL = `http://127.0.0.1:${port}/authorize`;
    const created = await request.post('/api/sessions', { data: { transport: 'local', cols: 100, rows: 30 } });
    expect(created.ok()).toBe(true);
    const session = await created.json();
    const config = path.resolve(__dirname, '.test-home', `auth-${session.id}.json`);
    let callbackURL = '';
    try {
      writeFileSync(config, JSON.stringify({ identity: identityURL, mode, verification: mode === 'device' ? 'https://github.com/login/device' : undefined }), { mode: 0o600 });
      if (mode === 'device') {
        await page.context().route('https://github.com/login/device', async route => {
          const response = await request.get(identityURL);
          await route.fulfill({ contentType: 'text/html', body: (await response.text()).replace('action="/approve"', `action="http://127.0.0.1:${port}/approve"`) });
        });
      } else {
        // Simulate a viewer on another computer: its localhost cannot reach the
        // CLI listener. Only the subsequent WebMux relay may deliver the code.
        await page.context().route(/http:\/\/127\.0\.0\.1:\d+\/callback\?/, route => {
          callbackURL = route.request().url(); return route.abort('connectionrefused');
        });
      }
      await page.goto('/');
      const tile = page.getByTestId(`tile-cell-${session.id}`);
      await tile.locator('.xterm-helper-textarea').focus();
      const quote = (value: string) => "'" + value.replaceAll("'", "'\\''") + "'";
      await page.keyboard.type([process.execPath, path.resolve(__dirname, 'auth-handoff-fixture.mts'), config].map(quote).join(' '));
      await page.keyboard.press('Enter');
      const panel = tile.getByRole('region', { name: `Sign-in for ${session.title}` });
      await expect(panel).toBeVisible();
      await expect(page.getByAltText('Interactive remote browser viewport')).toHaveCount(0);
      // Pending helper requests survive a viewer reload until explicit dismissal.
      await page.reload();
      await expect(panel).toBeVisible();
      if (mode === 'callback') await panel.getByRole('button', { name: 'Prepare callback relay' }).click();
      const popupPromise = page.waitForEvent('popup');
      await panel.getByRole('link', { name: 'Continue in this browser' }).click();
      const popup = await popupPromise;
      if (mode === 'device') await popup.getByRole('textbox', { name: 'Device code' }).fill('FIXTURE-1234');
      await popup.getByText('Approve sign-in', { exact: true }).click();
      if (mode === 'callback') {
        await expect.poll(() => callbackURL).toContain('code=fixture-code');
        await expect(tile.locator('.xterm-rows')).not.toContainText('CLI_AUTHENTICATED_ON_ORIGINATING_HOST');
        await panel.getByRole('textbox', { name: 'Final callback URL' }).fill(callbackURL.replace('fixture-state', 'wrong-state'));
        await panel.getByRole('button', { name: 'Deliver callback' }).click();
        await expect(panel.getByRole('alert')).toContainText('must match');
        await panel.getByRole('textbox', { name: 'Final callback URL' }).fill(callbackURL);
        await panel.getByRole('button', { name: 'Deliver callback' }).click();
        await expect(panel.getByRole('status')).toContainText('Callback delivered');
      }
      await expect(tile.locator('.xterm-rows')).toContainText('CLI_AUTHENTICATED_ON_ORIGINATING_HOST');
      await popup.close();
      await panel.getByRole('button', { name: 'Dismiss sign-in' }).click();
      await page.reload();
      await expect(panel).toHaveCount(0);
    } finally {
      rmSync(config, { force: true });
      await request.delete(`/api/sessions/${session.id}`);
      try { execFileSync('tmux', ['kill-session', '-t', `webmux-${session.id}`]); } catch { /* exited */ }
      await new Promise<void>(resolve => identity.close(() => resolve()));
    }
  });
}
