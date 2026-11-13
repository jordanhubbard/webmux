import { test, expect } from '@playwright/test';
import type { Terminal } from '@xterm/xterm';
import { execFileSync } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import http from 'node:http';
import path from 'node:path';

test('native recovery history stays above the restored viewport', async ({ page }, info) => {
  test.setTimeout(90_000);
  const root = path.resolve(__dirname, '../..');
  const capture = info.outputPath('native-history-fixture.json');
  mkdirSync(path.dirname(capture), { recursive: true });
  // The real broker emits the history, provider metadata and image in order.
  // This portable renderer control does not claim native Windows execution.
  execFileSync('go', ['test', './internal/session', '-run', '^TestWindowsNativeHistoryPrecedesSnapshotAndViewerReset$', '-count=1', '-timeout=30s'], {
    cwd: path.join(root, 'webmux/server'), timeout: 60_000,
    env: { ...process.env, WEBMUX_RECOVERY_HISTORY_CAPTURE: capture },
  });
  const fixture = JSON.parse(readFileSync(capture, 'utf8')) as { cols: number; rows: number; original: string; snapshot: string };
  const library = readFileSync(path.join(root, 'webmux/node_modules/@xterm/xterm/lib/xterm.js'));
  const css = readFileSync(path.join(root, 'webmux/node_modules/@xterm/xterm/css/xterm.css'));
  const server = http.createServer((req, res) => {
    if (req.url === '/x.js') { res.setHeader('Content-Type', 'text/javascript; charset=utf-8'); res.end(library); }
    else if (req.url === '/x.css') { res.setHeader('Content-Type', 'text/css; charset=utf-8'); res.end(css); }
    else { res.setHeader('Content-Type', 'text/html; charset=utf-8'); res.end('<link rel="stylesheet" href="/x.css"><div id="a"></div><div id="b"></div><script src="/x.js"></script>'); }
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  try {
    await page.goto(`http://127.0.0.1:${(server.address() as { port: number }).port}`);
    const result = await page.evaluate(async f => {
      const Constructor = (window as unknown as { Terminal: typeof Terminal }).Terminal;
      const a = new Constructor({ cols: f.cols, rows: f.rows, allowProposedApi: true });
      const b = new Constructor({ cols: f.cols, rows: f.rows, allowProposedApi: true });
      a.open(document.getElementById('a')!); b.open(document.getElementById('b')!);
      const write = (t: Terminal, data: string) => new Promise<void>(resolve => t.write(data, resolve));
      await write(a, f.original); await write(b, f.snapshot);
      const state = (t: Terminal) => {
        const v = t.buffer.active, lines = [], wraps = [];
        for (let y = 0; y < t.rows; y++) {
          const line = v.getLine(v.viewportY + y)!, cells = [];
          wraps.push(line.isWrapped);
          for (let x = 0; x < t.cols; x++) {
            const c = line.getCell(x)!;
            cells.push([c.getChars() || ' ', c.getWidth(), c.getFgColorMode(), c.getFgColor(), c.getBgColorMode(), c.getBgColor(), !!c.isBold(), !!c.isItalic(), !!c.isUnderline()]);
          }
          lines.push(cells);
        }
        return { lines, wraps, cursor: [v.cursorX, v.cursorY], modes: t.modes, buffer: v.type };
      };
      const history = [];
      for (let y = 0; y < b.buffer.active.viewportY; y++) history.push(b.buffer.active.getLine(y)!.translateToString(true));
      const result = { reference: state(a), restored: state(b), history: history.join('\n') };
      a.dispose(); b.dispose(); return result;
    }, fixture);
    const receipt = info.outputPath('native-history-renderer.json');
    writeFileSync(receipt, JSON.stringify(result));
    await info.attach('native history renderer', { path: receipt, contentType: 'application/json' });
    expect(result.restored).toEqual(result.reference);
    expect(result.history).toContain('OLD_HISTORY_MARKER\nSECOND_HISTORY_LINE');
  } finally {
    await new Promise<void>(resolve => server.close(() => resolve()));
  }
});
