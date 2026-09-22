import { test, expect } from '@playwright/test';
import path from 'node:path';
import { randomUUID } from 'node:crypto';

test.use({ trace: 'retain-on-failure' });

for (const scenario of ['broadcast', 'restart', 'selection'] as const) {
  test(`mouse reports stay with their process across ${scenario}`, async ({ page, request }) => {
    const ids: string[] = [];
    const outputs = new Map<string, string>();
    const inputs = new Map<string, string[]>();
    page.on('websocket', socket => {
      const id = ids.find(id => socket.url().includes(id));
      if (!id) return;
      socket.on('framereceived', frame => {
        const message = JSON.parse(frame.payload.toString());
        if (message.type === 'output') outputs.set(id, (outputs.get(id) ?? '') + message.data);
      });
      socket.on('framesent', frame => {
        const message = JSON.parse(frame.payload.toString());
        if (message.type === 'input') inputs.get(id)!.push(message.data);
      });
    });
    async function create(mode: string) {
      const fixture = path.resolve(__dirname, 'mouse-terminal-fixture.mts');
      const marker = path.resolve(__dirname, '.test-home', `mouse-${randomUUID()}`);
      const response = await request.post('/api/sessions', { data: {
        hostname: 'localhost', username: 'mouse-test', transport: 'exec',
        exec_command: `"${process.execPath}" "${fixture}" "${mode}" "${marker}"`,
      } });
      expect(response.ok()).toBe(true);
      const { id } = await response.json(); ids.push(id); inputs.set(id, []); return id as string;
    }
    try {
      const source = await create(scenario === 'restart' ? 'restart' : 'mouse');
      const peer = scenario === 'broadcast' ? await create('plain') : null;
      await page.goto('/');
      const tile = page.getByTestId(`tile-cell-${source}`);
      await expect(tile.locator('.xterm-rows')).toContainText('MOUSE READY');
      const screen = tile.locator('.xterm-screen');
      const box = (await screen.boundingBox())!;
      await page.mouse.move(box.x + 20, box.y + 12);
      await tile.locator('.xterm-helper-textarea').focus();
      await page.keyboard.type('h');
      await expect.poll(() => outputs.get(source)).toContain('INPUT:68;');
      expect(inputs.get(source)).toEqual(['h']);
      // Explicit button input remains available to applications.
      await page.mouse.down();
      await page.mouse.up();
      await expect.poll(() => outputs.get(source)).toContain('INPUT:1b5b3c');
      if (scenario === 'broadcast') {
        await expect(page.getByTestId(`tile-cell-${peer}`).locator('.xterm-rows')).toContainText('PLAIN READY');
        await page.getByRole('button', { name: 'Type to All', exact: true }).click();
        inputs.get(source)!.length = 0; inputs.get(peer!)!.length = 0;
        await page.mouse.move(box.x + 45, box.y + 12);
        await page.mouse.down();
        await page.mouse.move(box.x + 60, box.y + 12);
        await page.mouse.up();
        await expect.poll(() => inputs.get(source)!.length).toBeGreaterThan(0);
        expect(inputs.get(peer!)).toEqual([]);
        await tile.locator('.xterm-helper-textarea').focus();
        await page.keyboard.type('k');
        await expect.poll(() => outputs.get(peer!)).toContain('INPUT:6b;');
        expect(inputs.get(peer!)).toEqual(['k']);
      } else if (scenario === 'selection') {
        const modifier = await page.evaluate(() => /Mac|iPhone|iPad/.test(navigator.platform)) ? 'Alt' : 'Shift';
        await page.keyboard.down(modifier);
        await tile.locator('.xterm-rows > div').first().dblclick({ position: { x: 16, y: 7 } });
        await page.keyboard.up(modifier);
        const input = tile.locator('.xterm-helper-textarea');
        const copied = await input.evaluate(element => {
          const clipboardData = new DataTransfer();
          element.dispatchEvent(new ClipboardEvent('copy', { clipboardData, bubbles: true, cancelable: true }));
          return clipboardData.getData('text/plain');
        });
        expect(copied).toBe('MOUSE');
        await tile.getByRole('button', { name: 'Log session', exact: true }).click();
        await expect(input).toBeFocused();
      } else {
        const response = await request.post(`/api/sessions/${source}/reconnect`, { data: {}, timeout: 10_000 });
        expect(response.ok()).toBe(true);
        await expect(tile.locator('.xterm-rows')).toContainText('PLAIN READY');
        await expect(tile.locator('.xterm-rows')).not.toContainText('MOUSE READY');
        inputs.get(source)!.length = 0;
        await page.mouse.move(box.x + 50, box.y + 14);
        // A keyboard round trip orders the assertion after the mouse event.
        await tile.locator('.xterm-helper-textarea').focus();
        await page.keyboard.type('k');
        await expect.poll(() => outputs.get(source)).toContain('INPUT:6b;');
        expect(inputs.get(source)).toEqual(['k']);
      }
    } finally {
      for (const id of ids) expect((await request.delete(`/api/sessions/${id}`, { timeout: 5_000 })).ok()).toBe(true);
    }
  });
}
