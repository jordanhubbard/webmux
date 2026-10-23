import { test, expect } from '@playwright/test';
import { spawnSync } from 'node:child_process';

test('wheel scrolls the pane history of a managed tmux terminal', async ({ page, request }) => {
  test.skip(process.platform === 'win32', 'Local tmux terminals require Unix');
  const created = await request.post('/api/sessions', { data: { transport: 'local', cols: 80, rows: 24 } });
  expect(created.ok()).toBe(true);
  const { id } = await created.json() as { id: string };
  const target = `webmux-${id}`;
  const tmux = (...args: string[]) => {
    const result = spawnSync('tmux', args, { encoding: 'utf8', timeout: 5000 });
    if (result.error) throw result.error;
    expect(result.status, result.stderr).toBe(0);
    return result.stdout.trim();
  };
  try {
    await page.goto('/');
    const tile = page.getByTestId(`tile-cell-${id}`);
    const rows = tile.locator('.xterm-rows');
    await expect.poll(() => tmux('display-message', '-p', '-t', target, '#{session_attached}')).toBe('1');
    await expect(tile.locator('.enable-mouse-events')).toBeVisible();
    tmux('send-keys', '-t', target, "printf 'WM_OLD\\n'; seq 1 35; printf 'WM_NEW\\n'", 'Enter');
    await expect(rows).toContainText('WM_NEW');
    await expect(rows).not.toContainText('WM_OLD');

    const screen = tile.locator('.xterm-screen');
    const box = (await screen.boundingBox())!;
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    for (let i = 0; i < 20; i++) await page.mouse.wheel(0, -120);
    await expect.poll(() => tmux('display-message', '-p', '-t', target, '#{pane_in_mode}')).toBe('1');
    await expect(rows).toContainText('WM_OLD');
  } finally {
    await request.delete(`/api/sessions/${id}`);
    spawnSync('tmux', ['kill-session', '-t', target], { timeout: 5000 });
  }
});
