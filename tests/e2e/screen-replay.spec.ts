import { test, expect } from '@playwright/test';
import type { Terminal } from '@xterm/xterm';
import http from 'node:http';
import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';

type Fixture = { name: string; cols: number; rows: number; original: string; snapshot: string; future: string; recaptured: string };
const names = ['immediate', 'unicode', 'combining', 'pending-wrap', 'wide-pending-wrap', 'insert-mode', 'after-sgr', 'after-bell', 'after-private-osc', 'after-query', 'after-repeat', 'c0-inside-csi', 'del-inside-csi', 'after-del', 'after-erased-character'];
let fixtures: Fixture[];
let server: http.Server;
let address: string;

test.beforeAll(async () => {
  const root = path.resolve(__dirname, '../..');
  fixtures = JSON.parse(execFileSync('go', ['run', './internal/screenstate/testdata/replay-fixtures'], {
    cwd: path.join(root, 'webmux/server'), encoding: 'utf8', timeout: 60_000, maxBuffer: 2 << 20,
  })) as Fixture[];
  expect(fixtures.map(f => f.name)).toEqual(names);
  const library = readFileSync(path.join(root, 'webmux/node_modules/@xterm/xterm/lib/xterm.js'));
  const css = readFileSync(path.join(root, 'webmux/node_modules/@xterm/xterm/css/xterm.css'));
  server = http.createServer((req, res) => {
    if (req.url === '/xterm.js') { res.setHeader('Content-Type', 'text/javascript'); res.end(library); }
    else if (req.url === '/xterm.css') { res.setHeader('Content-Type', 'text/css'); res.end(css); }
    else { res.setHeader('Content-Type', 'text/html'); res.end('<link rel="stylesheet" href="/xterm.css"><div id="reference"></div><div id="restored"></div><div id="recaptured"></div><script src="/xterm.js"></script>'); }
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  address = `http://127.0.0.1:${(server.address() as { port: number }).port}`;
});
test.afterAll(async () => { if (server) await new Promise<void>(resolve => server.close(() => resolve())); });

for (const name of names) {
  test(`Go screen snapshot preserves xterm REP ${name}`, async ({ page }, info) => {
    const fixture = fixtures.find(f => f.name === name)!;
    await page.goto(address);
    const result = await page.evaluate(async f => {
      const Constructor = (window as unknown as { Terminal: typeof Terminal }).Terminal;
      const terminals = ['reference', 'restored', 'recaptured'].map(id => {
        const t = new Constructor({ cols: f.cols, rows: f.rows, allowProposedApi: true });
        t.open(document.getElementById(id)!); return t;
      });
      const [a, b, c] = terminals;
      const write = (t: Terminal, data: string) => new Promise<void>(resolve => t.write(data, resolve));
      const state = (t: Terminal) => {
        const v = t.buffer.active, cells = [], wraps = [];
        for (let y = 0; y < f.rows; y++) {
          const line = v.getLine(v.viewportY + y)!, row = [];
          wraps.push(line.isWrapped);
          for (let x = 0; x < f.cols; x++) {
            const p = line.getCell(x)!;
            row.push([p.getWidth() === 1 ? p.getChars() || ' ' : p.getChars(), p.getWidth(), p.getFgColorMode(), p.getFgColor(), p.getBgColorMode(), p.getBgColor(), p.isBold(), p.isItalic(), p.isUnderline(), p.isInverse()]);
          }
          cells.push(row);
        }
        return { cells, wraps, cursor: [v.cursorX, v.cursorY], buffer: v.type, modes: t.modes };
      };
      const compare = (left: Terminal, right: Terminal) => {
        const reference = state(left), restored = state(right);
        const equal = JSON.stringify(reference) === JSON.stringify(restored);
        return equal ? { equal } : { equal, reference, restored };
      };
      const replayReplies: string[] = [];
      b.onData(data => replayReplies.push(data)); c.onData(data => replayReplies.push(data));
      await write(a, f.original); await write(b, f.snapshot);
      const initial = compare(a, b);
      await write(a, f.future); await write(b, f.future);
      const continued = compare(a, b);
      await write(c, f.recaptured);
      const recaptured = compare(a, c);
      terminals.forEach(t => t.dispose());
      return { initial, continued, recaptured, replayReplies };
    }, fixture);
    const receipt = info.outputPath('screen-replay.json');
    writeFileSync(receipt, JSON.stringify(result));
    await info.attach('screen replay', { path: receipt, contentType: 'application/json' });
    expect(result.initial.equal, 'initial snapshot').toBe(true);
    expect(result.continued.equal, 'output after first attachment').toBe(true);
    expect(result.recaptured.equal, 'second attachment after more output').toBe(true);
    expect(result.replayReplies, 'snapshot must not repeat terminal queries').toEqual([]);
  });
}
