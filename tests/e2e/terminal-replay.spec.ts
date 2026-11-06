import { test, expect } from '@playwright/test';
import path from 'node:path';

test('rejoining a truncated full-screen replay keeps status bars out of scrollback', async ({ page, request }) => {
  const captures: string[] = [];
  page.on('websocket', socket => socket.on('framereceived', frame => {
    const message = JSON.parse(frame.payload.toString());
    if (message.type === 'output') captures.push(message.data);
  }));
  const fixture = path.resolve(__dirname, 'replay-terminal-fixture.mts');
  const created = await request.post('/api/sessions', { data: {
    hostname: 'localhost', username: 'replay-test', transport: 'exec',
    exec_command: `"${process.execPath}" "${fixture}"`,
  } });
  expect(created.ok()).toBe(true);
  const { id } = await created.json() as { id: string };
  try {
    await page.goto('/');
    const tile = page.getByTestId(`tile-cell-${id}`);
    const rows = tile.locator('.xterm-rows');
    await expect(rows).toContainText('REPLAY READY');
    // Rejoin after the provider has emitted >64 KiB; its entry sequence is gone.
    await page.reload();
    await expect(rows).toContainText('AGENT STATUS');
    await page.setViewportSize({ width: 1000, height: 650 });
    await page.setViewportSize({ width: 1280, height: 900 });
    const screen = tile.locator('.xterm-screen');
    await screen.hover();
    await page.mouse.wheel(0, -3000);
    // Order the assertion after the browser applies its wheel/scroll events.
    await expect.poll(() => tile.locator('.xterm-viewport').evaluate(el => el.scrollTop)).toBe(0);
    await expect(rows).toContainText('REPLAY READY');
    await expect(rows).toContainText('TMUX STATUS');
    await expect(rows).not.toContainText('OLD HISTORY');
  } finally {
    if (process.platform === 'win32') {
      console.log('WINDOWS_REPLAY_VT', JSON.stringify(captures.map(data => ({ bytes: data.length, head: data.slice(0, 128), tail: data.slice(-512), alternate: data.includes('\x1b[?1049h') }))));
      await test.info().attach('terminal-replay-vt', { body: Buffer.from(JSON.stringify(captures)), contentType: 'application/json' });
    }
    expect((await request.delete(`/api/sessions/${id}`, { timeout: 10_000 })).ok()).toBe(true);
  }
});
