import { test, expect } from '@playwright/test';
import path from 'node:path';

test('tile drag scrolls all edges, tracks the current grid and stops on drop or cancellation', async ({ page, request }) => {
  test.setTimeout(60_000);
  await page.setViewportSize({ width: 1000, height: 720 });
  const ids: string[] = [];
  try {
    for (const [row, col] of [[0, 0], [2, 2]]) {
      const response = await request.post('/api/sessions', { data: {
        hostname: 'localhost', username: 'edge-test', transport: 'exec', row, col,
        exec_command: `"${process.execPath}" "${path.resolve(__dirname, 'terminal-fixture.mts')}"`,
      } });
      expect(response.ok()).toBe(true);
      ids.push((await response.json()).id);
    }
    await page.goto('/');
    const scroll = page.getByTestId('workspace-scroll');
    const tile = page.getByTestId(`tile-cell-${ids[0]}`);
    const title = tile.getByTestId('session-title');
    await expect(title).toBeVisible();
    await tile.locator('.xterm-helper-textarea').focus();
    const bounds = (await scroll.boundingBox())!;
    const middle = { x: bounds.x + bounds.width / 2, y: bounds.y + bounds.height / 2 };
    const position = () => scroll.evaluate(el => ({ x: el.scrollLeft, y: el.scrollTop }));
    const grabTitle = async () => {
      await title.evaluate(element => element.scrollIntoView({ block: 'center', inline: 'center', behavior: 'instant' }));
      // A raw screen coordinate can hit another element after scrolling.
      await title.hover({ position: { x: 15, y: 8 } });
      await page.mouse.down();
      await expect(page.getByTestId('workspace-drag-ghost')).toBeVisible();
    };
    const begin = async () => {
      await scroll.evaluate(el => { el.scrollLeft = 0; el.scrollTop = 0; });
      await grabTitle();
    };
    await begin();
    // A stationary pointer must keep moving the grid; no synthetic scroll events.
    await page.mouse.move(bounds.x + bounds.width - 3, middle.y);
    await expect.poll(async () => (await position()).x).toBeGreaterThan(240);
    await page.mouse.move(middle.x, bounds.y + bounds.height - 3);
    await expect.poll(async () => (await position()).y).toBeGreaterThan(240);
    await page.mouse.move(bounds.x + 2, middle.y);
    await expect.poll(async () => (await position()).x).toBeLessThan(5);
    await page.mouse.move(middle.x, bounds.y + 2);
    await expect.poll(async () => (await position()).y).toBeLessThan(5);
    // Diagonal scrolling, including a pointer just below the workspace viewport.
    await page.mouse.move(bounds.x + bounds.width - 3, Math.min(719, bounds.y + bounds.height + 4));
    await expect.poll(async () => Math.min((await position()).x, (await position()).y)).toBeGreaterThan(400);
    await page.mouse.move(middle.x, middle.y);
    const target = await tile.evaluate((el, pointer) => {
      const grid = el.parentElement!;
      const style = getComputedStyle(grid);
      const box = grid.getBoundingClientRect();
      const column = parseFloat(style.gridTemplateColumns.split(' ')[0]);
      const row = parseFloat(style.gridTemplateRows.split(' ')[0]);
      return { row: Math.floor((pointer.y - box.top - parseFloat(style.paddingTop)) / (row + parseFloat(style.rowGap))),
        col: Math.floor((pointer.x - box.left - parseFloat(style.paddingLeft)) / (column + parseFloat(style.columnGap))) };
    }, middle);
    expect(target.row + target.col).toBeGreaterThan(0);
    await page.mouse.up();
    await expect.poll(async () => {
      const sessions = await (await request.get('/api/sessions')).json();
      const value = sessions.find((s: { id: string }) => s.id === ids[0]);
      return { row: value.row, col: value.col };
    }).toEqual(target);
    await expect(tile).toHaveCSS('grid-column-start', String(target.col + 1));
    await expect(tile).toHaveCSS('grid-row-start', String(target.row + 1));
    await expect(page.getByTestId('workspace-drag-ghost')).toHaveCount(0);
    const afterDrop = await position();
    await page.mouse.move(bounds.x + bounds.width - 3, bounds.y + bounds.height - 3);
    await page.waitForTimeout(150); // negative control: no active drag means no RAF scrolling
    expect(await position()).toEqual(afterDrop);

    // Bring the moved title into view, then cancel an active edge drag.
    await grabTitle();
    await page.mouse.move(bounds.x + 2, bounds.y + 2);
    await page.keyboard.press('Escape');
    await expect(page.getByTestId('workspace-drag-ghost')).toHaveCount(0);
    const afterCancel = await position();
    await page.waitForTimeout(150);
    expect(await position()).toEqual(afterCancel);
    await expect(tile.locator('.xterm-rows')).not.toContainText('KEY:1b;');
    await page.mouse.up();
    await grabTitle();
    await page.mouse.move(bounds.x + bounds.width - 3, bounds.y + bounds.height - 3);
    const beforeBlur = await position();
    await expect.poll(async () => (await position()).y).toBeGreaterThan(beforeBlur.y + 20);
    await page.evaluate(() => window.dispatchEvent(new Event('blur')));
    await expect(page.getByTestId('workspace-drag-ghost')).toHaveCount(0);
    const afterBlur = await position();
    await page.waitForTimeout(150);
    expect(await position()).toEqual(afterBlur);
    await page.mouse.up();
    const saved = await (await request.get('/api/sessions')).json();
    expect(saved.find((s: { id: string }) => s.id === ids[0])).toMatchObject(target);
    expect(await page.evaluate(() => ({ x: window.scrollX, y: window.scrollY }))).toEqual({ x: 0, y: 0 });
  } finally {
    await page.mouse.up();
    for (const id of ids) await request.delete(`/api/sessions/${id}`);
  }
});
