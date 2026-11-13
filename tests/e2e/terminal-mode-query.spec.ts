import { test, expect, type Page, type WebSocketRoute } from '@playwright/test';
import path from 'node:path';
import { writeFileSync } from 'node:fs';

test('mode queries from two viewers never become PTY or broadcast input', async ({ page, request }, info) => {
  const inputs: { viewer: string; id: string; data: string }[] = [];
  const outputs = new Map<string, string>();
  const injected = new Map<string, string>();
  const sockets = new Map<string, WebSocketRoute>();
  const ids: string[] = [];
  const earlier = await page.context().newPage();
  const observe = async (viewer: Page, name: string) => {
    // ConPTY and tmux can consume terminal queries themselves. Inject exact
    // display output here to deterministically exercise both frontend parsers;
    // the native tmux/browser-companion case covers unmodified tmux output.
    await viewer.routeWebSocket(/\/api\/term\//, socket => {
      const id = ids.find(id => socket.url().includes(`/api/term/${id}`));
      if (id) sockets.set(`${name}:${id}`, socket);
      socket.connectToServer();
    });
    viewer.on('websocket', socket => {
      const id = ids.find(id => socket.url().includes(`/api/term/${id}`));
      if (!id) return;
      socket.on('framesent', frame => {
        const message = JSON.parse(frame.payload.toString()) as { type: string; data: string };
        if (message.type === 'input' && message.data !== '\x1b[I' && message.data !== '\x1b[O') inputs.push({ viewer: name, id, data: message.data });
      });
      socket.on('framereceived', frame => {
        const message = JSON.parse(frame.payload.toString()) as { type: string; data: string };
        if (message.type === 'output') {
          const key = `${name}:${id}`;
          outputs.set(key, ((outputs.get(key) ?? '') + message.data).slice(-32768));
        }
      });
    });
  };
  await observe(page, 'active'); await observe(earlier, 'earlier');
  try {
    for (let n = 0; n < 2; n++) {
      const fixture = path.resolve(__dirname, 'mouse-terminal-fixture.mts');
      const response = await request.post('/api/sessions', { data: {
        hostname: 'localhost', username: 'mode-query-test', transport: 'exec',
        exec_command: `"${process.execPath}" "${fixture}" plain`,
      } });
      expect(response.ok()).toBe(true);
      ids.push((await response.json() as { id: string }).id);
    }
    const [source] = ids;
    const tile = (viewer: Page, id: string) => viewer.getByTestId(`tile-cell-${id}`);
    await earlier.goto('/');
    await expect(tile(earlier, source).locator('.xterm-rows')).toContainText('PLAIN READY');
    await page.goto('/');
    for (const id of ids) await expect(tile(page, id).locator('.xterm-rows')).toContainText('PLAIN READY');
    // The active page broadcasts; the earlier viewer can still send ordinary
    // input. Wait for its actual PTY receipt before injecting the two frames.
    await page.getByRole('button', { name: 'Type to All', exact: true }).click();
    await tile(earlier, source).locator('.xterm-helper-textarea').focus();
    await earlier.keyboard.type('!');
    await expect.poll(() => outputs.get(`earlier:${source}`)).toContain('INPUT:21;');
    const controls = ['\x1b[4$p\x1b[?2026$', 'p\x1b[?2004h\x1b[?2004$p\x1b[?2004l\x1b[?2004$p\x1b[?9999$p\x1b[?2004h\x1b[31mMODE QUERY DONE\x1b[0m'];
    for (const name of ['earlier', 'active']) {
      const socket = sockets.get(`${name}:${source}`)!;
      injected.set(name, controls.join(''));
      for (const data of controls) socket.send(JSON.stringify({ type: 'output', data }));
    }
    for (const viewer of [earlier, page]) await expect(tile(viewer, source).locator('.xterm-rows')).toContainText('MODE QUERY DONE');
    expect(inputs).toEqual([{ viewer: 'earlier', id: source, data: '!' }]);
    // Function/editor keys keep their original bytes and broadcast behavior.
    await tile(page, source).locator('.xterm-helper-textarea').focus();
    for (const key of ['ArrowLeft', 'Control+f', 'Control+l', 'F1', 'Tab']) await page.keyboard.press(key);
    const keys = '\x1b[D\x06\x0c\x1bOP\t';
    for (const id of ids) await expect.poll(() => inputs.filter(i => i.viewer === 'active' && i.id === id).map(i => i.data).join('')).toBe(keys);
    // Preserve bracketed paste, including literal bytes resembling a reply.
    await tile(page, source).locator('.xterm-helper-textarea').evaluate(element => {
      const clipboardData = new DataTransfer();
      clipboardData.setData('text/plain', 'literal:\x1b[?2026;2$y');
      element.dispatchEvent(new ClipboardEvent('paste', { clipboardData, bubbles: true, cancelable: true }));
    });
    const pasted = '\x1b[200~literal:\x1b[?2026;2$y\x1b[201~';
    for (const id of ids) await expect.poll(() => inputs.filter(i => i.viewer === 'active' && i.id === id).map(i => i.data).join('')).toBe(keys + pasted);
    for (const id of ids) {
      const expected = Buffer.from((id === source ? '!' : '') + keys + pasted).toString('hex');
      await expect.poll(() => [...(outputs.get(`active:${id}`) ?? '').matchAll(/INPUT:([a-f0-9]+);/g)].map(match => match[1]).join('')).toBe(expected);
    }
  } finally {
    const trafficPath = info.outputPath('terminal-mode-query-traffic.json');
    writeFileSync(trafficPath, JSON.stringify({ inputs, outputs: Object.fromEntries(outputs), injected: Object.fromEntries(injected) }));
    await info.attach('terminal-mode-query-traffic', { path: trafficPath, contentType: 'application/json' });
    await earlier.close();
    for (const id of ids) expect((await request.delete(`/api/sessions/${id}`)).ok()).toBe(true);
  }
});
