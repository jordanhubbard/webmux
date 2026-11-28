import { test, expect } from '@playwright/test';
import path from 'node:path';

const logical = 'WRAP_BEGIN ' + 'alpha beta gamma '.repeat(22) + ' WRAP_END';
for (const stage of ['live', 'reconnected', 'prewrapped'] as const) {
  test(stage === 'prewrapped' ? 'application hard wraps remain distinct from source-text clipboard copies' : `copy preserves soft wraps and real newlines in ${stage} terminal output`, async ({ page, request }) => {
    await page.setViewportSize({ width: 1280, height: 900 });
    const response = await request.post('/api/sessions', { data: { transport: 'exec', hostname: 'localhost', username: 'wrap-copy',
      exec_command: `"${process.execPath}" "${path.resolve(__dirname, 'terminal-wrap-fixture.mts')}"${stage === 'prewrapped' ? ' --prewrapped' : ''}` } });
    expect(response.ok()).toBe(true);
    const { id } = await response.json();
    try {
      await page.context().grantPermissions(['clipboard-read','clipboard-write']);
      const clipboard = () => page.evaluate(async () => (await navigator.clipboard.readText()).replace(/\r\n/g, '\n'));
      await page.goto('/');
      const tile = page.getByTestId(`tile-cell-${id}`);
      await expect(tile.locator('.xterm-rows')).toContainText('WRAP_COPY_READY');
      await tile.locator('.xterm-helper-textarea').focus();
      await page.keyboard.press('Control+g');
      await expect(tile.locator('.xterm-rows')).toContainText('REAL_NEWLINE');
      if (stage === 'reconnected') await page.reload();
      await expect(tile.locator('.xterm-rows')).toContainText('REAL_NEWLINE');
      const geometry = await tile.locator('.xterm-rows').evaluate(element => {
        const rows = [...element.children];
        const first = rows.find(row => row.textContent?.startsWith('WRAP_BEGIN'));
        const last = rows.find(row => row.textContent?.startsWith('REAL_NEWLINE'));
        if (!first || !last) throw new Error('Missing logical line markers');
        const box = first.getBoundingClientRect(), end = last.getBoundingClientRect();
        return { x: box.x, y: box.y + box.height / 2, endY: end.y + end.height / 2,
          cellWidth: box.width / first.textContent!.length, columns: first.textContent!.length };
      });
      expect(geometry.columns).toBeLessThan(logical.length);
      await page.mouse.move(geometry.x + 1, geometry.y);
      await page.mouse.down();
      await page.mouse.move(geometry.x + geometry.cellWidth * 12 + 1, geometry.endY, { steps: 20 });
      await page.mouse.up();
      const original = logical + '\nREAL_NEWLINE';
      if (stage === 'prewrapped') {
        await expect.poll(async () => (await clipboard()).split('\n').length).toBeGreaterThan(2);
        // The application emitted these breaks. Selection must not guess which
        // ones were formatting and which were intentional command boundaries.
        const displayed = await clipboard();
        expect(displayed).not.toBe(original);
        expect(displayed).toContain('\nREAL_NEWLINE');
        await tile.locator('.xterm-helper-textarea').focus();
        await page.keyboard.press('Control+o');
        await expect.poll(clipboard).toBe(original);
      } else {
        await expect.poll(clipboard).toBe(original);
        const mac = await page.evaluate(() => /Mac|iPhone|iPad/.test(navigator.platform));
        await page.evaluate(() => navigator.clipboard.writeText('keyboard-copy sentinel'));
        await page.keyboard.press(mac ? 'Meta+c' : 'Control+c');
        await expect.poll(clipboard).toBe(original);
        await page.evaluate(() => navigator.clipboard.writeText('button-copy sentinel'));
        await tile.getByRole('button', { name: 'Copy terminal selection', exact: true }).click();
        await expect.poll(clipboard).toBe(original);
      }
    } finally { await request.delete(`/api/sessions/${id}`); }
  });
}
