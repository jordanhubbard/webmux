import { test, expect } from '@playwright/test';
import http from 'node:http';
import { execFileSync } from 'node:child_process';

test('browser companion completes a loopback callback and retains state until ended', async ({ page, request }) => {
  test.setTimeout(90_000);
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
  try {
    await page.goto('/');
    await page.getByRole('button', { name: 'Browser', exact: true }).click();
    const viewport = page.getByAltText('Interactive remote browser viewport');
    await expect(viewport).toBeVisible({ timeout: 30_000 });
    await page.getByRole('textbox', { name: 'Remote browser URL' }).fill(identityURL);
    await page.getByRole('button', { name: 'Go', exact: true }).click();
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
    await expect(page.getByRole('textbox', { name: 'Remote browser URL' })).toHaveValue(/\/callback\?code=callback-proof/);
    const before = await request.post(`/api/sessions/${session.id}/browser`, { data: { action: 'frame' } });
    const target = (await before.json()).target;
    await page.screenshot({ path: 'test-results/browser-companion.png' });
    await page.getByRole('button', { name: 'Return to terminal' }).click();
    await expect(viewport).not.toBeVisible();
    await page.getByRole('button', { name: 'Browser', exact: true }).click();
    await expect(viewport).toBeVisible();
    const reopened = await request.post(`/api/sessions/${session.id}/browser`, { data: { action: 'frame' } });
    expect((await reopened.json()).target).toBe(target);
    await page.getByRole('button', { name: 'End browser', exact: true }).click();
    await expect(viewport).not.toBeVisible();
    const ended = await request.post(`/api/sessions/${session.id}/browser`, { data: { action: 'frame' } });
    expect(ended.status()).toBe(503);
  } finally {
    await request.delete(`/api/sessions/${session.id}`);
    // Local terminals intentionally persist in tmux after disconnection.
    try { execFileSync('tmux', ['kill-session', '-t', `webmux-${session.id}`]); } catch { /* already exited */ }
    await Promise.all([new Promise<void>(resolve => identity.close(() => resolve())), new Promise<void>(resolve => callback.close(() => resolve()))]);
  }
});
