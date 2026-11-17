import { test, expect } from '@playwright/test';
import type { Terminal } from '@xterm/xterm';
import http from 'node:http';
import { execFileSync } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';

type Fixture = { name: string; cols: number; rows: number; original: string; snapshot: string; future: string; recaptured: string };
const names = ['saved-underline-csi', 'saved-underline-single', 'saved-underline-color', 'saved-underline-color-reset', 'saved-underline-batched', 'saved-underline-twice', 'saved-underline', 'saved-underline-1048', 'saved-underline-1049', 'alternate-wrapped-row', 'alternate-wrapped-blank', 'origin-clamp-top', 'origin-clamp-bottom', 'origin-clamp-then-widen', 'origin-batched-reset', 'origin-batched-reset-reverse', 'origin-active-at-restore', 'origin-after-ris', 'origin-after-soft-reset', 'saved-origin', 'saved-pending-wrap', 'saved-origin-csi', 'saved-origin-1048', 'saved-pending-1049', 'immediate', 'colored-underline', 'unicode', 'combining', 'pending-wrap', 'wide-pending-wrap', 'insert-mode', 'after-sgr', 'after-bell', 'after-private-osc', 'after-query', 'after-repeat', 'c0-inside-csi', 'del-inside-csi', 'after-del', 'after-erased-character'];
const NATIVE_SCREEN = process.platform === 'win32' && process.env.WEBMUX_SCREEN_REPLAY_ONLY === '1';
const nativeNames = ['underline-restore', 'underline-1049', 'origin-restore', 'origin-clamp', 'origin-mode-batch', 'pending-wrap', 'pending-wrap-alt'];
if (NATIVE_SCREEN) names.push(...nativeNames.map(name => `conpty-${name}`));
let fixtures: Fixture[];
let server: http.Server;
let address: string;

test.beforeAll(async () => {
  test.setTimeout(NATIVE_SCREEN ? 240_000 : 60_000);
  const root = path.resolve(__dirname, '../..');
  fixtures = JSON.parse(execFileSync('go', ['run', './internal/screenstate/testdata/replay-fixtures'], {
    cwd: path.join(root, 'webmux/server'), encoding: 'utf8', timeout: 60_000, maxBuffer: 2 << 20,
  })) as Fixture[];
  if (NATIVE_SCREEN) {
    const target = path.join(test.info().project.outputDir, 'native-screen-captures.json');
    mkdirSync(path.dirname(target), { recursive: true });
    const suppliedCaptures = process.env.WEBMUX_NATIVE_SCREEN_CAPTURES;
    const suppliedLog = process.env.WEBMUX_NATIVE_SCREEN_TEST_LOG;
    expect(Boolean(suppliedCaptures), 'external native captures and test log must be supplied together').toBe(Boolean(suppliedLog));
    if (suppliedCaptures) writeFileSync(target, readFileSync(suppliedCaptures));
    const output = suppliedLog ? readFileSync(suppliedLog, 'utf8') : execFileSync('go', ['test', '-json', '-count=1', '-timeout=120s', './internal/nativeprovider', '-run', '^TestNativeProviderSavedCursorCaptures$'], {
      cwd: path.join(root, 'webmux/server'), encoding: 'utf8', timeout: 150_000, maxBuffer: 4 << 20,
      env: { ...process.env, WEBMUX_NATIVE_CURSOR_CAPTURES: target },
    });
    writeFileSync(path.join(path.dirname(target), 'native-screen-tests.jsonl'), output);
    const events = output.trim().split('\n').map(line => JSON.parse(line) as { Action: string; Test?: string });
    expect(events.filter(event => event.Action === 'skip'), 'native capture cases must execute').toEqual([]);
    expect(events.some(event => event.Action === 'pass' && event.Test === 'TestNativeProviderSavedCursorCaptures')).toBe(true);
    const captures = JSON.parse(readFileSync(target, 'utf8')) as {
      name: string; cols: number; rows: number; original: string; original_before: string;
      snapshot_before: string; snapshot: string; recaptured: string; starts: number;
    }[];
    expect(captures.map(c => c.name)).toEqual(nativeNames);
    for (const c of captures) {
      expect(c.starts, 'one native job per control').toBe(1);
      expect(c.original.startsWith(c.original_before)).toBe(true);
      expect(c.snapshot.startsWith(c.snapshot_before)).toBe(true);
      const future = c.original.slice(c.original_before.length);
      expect(c.snapshot.slice(c.snapshot_before.length), 'both viewers receive identical later ConPTY output').toBe(future);
      fixtures.push({ name: `conpty-${c.name}`, cols: c.cols, rows: c.rows, original: c.original_before, snapshot: c.snapshot_before, future, recaptured: c.recaptured });
    }
  }
  expect(fixtures.map(f => f.name)).toEqual(names);
  const library = readFileSync(path.join(root, 'webmux/node_modules/@xterm/xterm/lib/xterm.js'));
  const css = readFileSync(path.join(root, 'webmux/node_modules/@xterm/xterm/css/xterm.css'));
  server = http.createServer((req, res) => {
    if (req.url === '/xterm.js') { res.setHeader('Content-Type', 'text/javascript; charset=utf-8'); res.end(library); }
    else if (req.url === '/xterm.css') { res.setHeader('Content-Type', 'text/css; charset=utf-8'); res.end(css); }
    else { res.setHeader('Content-Type', 'text/html; charset=utf-8'); res.end('<link rel="stylesheet" href="/xterm.css"><div id="reference"></div><div id="restored"></div><div id="recaptured"></div><script src="/xterm.js"></script>'); }
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  address = `http://127.0.0.1:${(server.address() as { port: number }).port}`;
});
test.afterAll(async () => { if (server) await new Promise<void>(resolve => server.close(() => resolve())); });

for (const name of names) {
  test(`Go screen snapshot matches xterm ${name}`, async ({ page }, info) => {
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
            // The shared renderer stores extended underline details on its
            // cell object; compare their values, not internal flag bit masks.
            const underline = p as typeof p & { getUnderlineStyle(): number; getUnderlineColorMode(): number; getUnderlineColor(): number };
            row.push([p.getWidth() === 1 ? p.getChars() || ' ' : p.getChars(), p.getWidth(), p.getFgColorMode(), p.getFgColor(), p.getBgColorMode(), p.getBgColor(), !!p.isBold(), !!p.isItalic(), !!p.isUnderline(), !!p.isInverse(), underline.getUnderlineStyle(), underline.getUnderlineColorMode(), underline.getUnderlineColor()]);
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
