import { test, expect } from '@playwright/test';
import type { Terminal } from '@xterm/xterm';
import http from 'node:http';
import { execFileSync } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { transformWithOxc } from 'vite';
import { isTerminalLocalInput, shouldSuppressTerminalInput } from '../../webmux/frontend/src/utils/terminalInput';

type Fixture = { name: string; cols: number; rows: number; original: string; snapshot: string; future: string; recaptured: string };
const names = ['wrap-toggle-pending', 'wrap-toggle-resolved', 'link-named', 'link-anonymous', 'link-replace', 'link-unicode', 'link-semicolon', 'link-wrapped', 'link-id-reuse', 'link-save', 'link-alternate', 'link-close-params', 'palette-short-hex', 'palette-short-rgb', 'palette-long-rgb', 'palette-long-hex', 'palette-single', 'palette-batch', 'palette-query', 'palette-reset', 'dynamic-colors', 'dynamic-query', 'dynamic-reset', 'saved-underline-csi', 'saved-underline-single', 'saved-underline-color', 'saved-underline-color-reset', 'saved-underline-batched', 'saved-underline-twice', 'saved-underline', 'saved-underline-1048', 'saved-underline-1049', 'alternate-wrapped-row', 'alternate-wrapped-blank', 'origin-clamp-top', 'origin-clamp-bottom', 'origin-clamp-then-widen', 'origin-batched-reset', 'origin-batched-reset-reverse', 'origin-active-at-restore', 'origin-after-ris', 'origin-after-soft-reset', 'saved-origin', 'saved-pending-wrap', 'saved-origin-csi', 'saved-origin-1048', 'saved-pending-1049', 'immediate', 'colored-underline', 'unicode', 'combining', 'pending-wrap', 'wide-pending-wrap', 'insert-mode', 'after-sgr', 'after-bell', 'after-private-osc', 'after-query', 'after-repeat', 'c0-inside-csi', 'del-inside-csi', 'after-del', 'after-erased-character'];
const NATIVE_SCREEN = Boolean(process.env.WEBMUX_NATIVE_SCREEN_CAPTURES || process.env.WEBMUX_NATIVE_SCREEN_TEST_LOG) || (process.platform === 'win32' && process.env.WEBMUX_SCREEN_REPLAY_ONLY === '1');
const nativeNames = ['hyperlink-named', 'hyperlink-anonymous', 'hyperlink-unicode', 'hyperlink-reuse', 'palette-live-change', 'dynamic-colors', 'underline-restore', 'underline-1049', 'origin-restore', 'origin-clamp', 'origin-mode-batch', 'pending-wrap', 'pending-wrap-alt'];
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
      if (c.name.startsWith('hyperlink-')) expect(c.original_before, 'ConPTY must actually carry the hyperlink').toContain('\x1b]8;');
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
  const profilePath = path.join(root, 'webmux/frontend/src/utils/terminalHyperlinks.ts');
  const profile = (await transformWithOxc(readFileSync(profilePath, 'utf8'), profilePath)).code;
  server = http.createServer((req, res) => {
    if (req.url === '/xterm.js') { res.setHeader('Content-Type', 'text/javascript; charset=utf-8'); res.end(library); }
    else if (req.url === '/hyperlink-profile.js') { res.setHeader('Content-Type', 'text/javascript'); res.end(profile); }
    else if (req.url === '/xterm.css') { res.setHeader('Content-Type', 'text/css; charset=utf-8'); res.end(css); }
    else { res.setHeader('Content-Type', 'text/html; charset=utf-8'); res.end('<link rel="stylesheet" href="/xterm.css"><div id="reference"></div><div id="restored"></div><div id="recaptured"></div><script src="/xterm.js"></script><script type="module">import{retainActiveTerminalHyperlink}from"/hyperlink-profile.js";window.installHyperlinkProfile=retainActiveTerminalHyperlink;</script>'); }
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
        (window as unknown as { installHyperlinkProfile(t: Terminal): { dispose(): void } }).installHyperlinkProfile(t);
        t.open(document.getElementById(id)!);
        return t;
      });
      const [a, b, c] = terminals;
      const write = (t: Terminal, data: string) => new Promise<void>(resolve => t.write(data, resolve));
      const state = (t: Terminal) => {
        const v = t.buffer.active, cells = [], wraps = [];
        // Renderer IDs are local handles. Compare URI and the equivalence
        // classes of cells sharing a link, including the current pen.
        type LinkCell = { hasExtendedAttrs(): boolean; extended: { urlId: number } };
        const core = (t as unknown as { _core: { _oscLinkService: { getLinkData(id: number): { uri: string } | undefined }; _inputHandler: { _curAttrData: LinkCell } } })._core;
        const groups = new Map<number, number>();
        const link = (cell: LinkCell) => {
          const id = cell.hasExtendedAttrs() ? cell.extended.urlId : 0;
          if (!id) return null;
          const data = core._oscLinkService.getLinkData(id);
          // xterm may leave an orphaned numeric attribute after a buffer's
          // markers are disposed. It has no target and cannot activate a link.
          if (!data) return null;
          if (!groups.has(id)) groups.set(id, groups.size + 1);
          return { uri: data.uri, group: groups.get(id) };
        };
        for (let y = 0; y < f.rows; y++) {
          const line = v.getLine(v.viewportY + y)!, row = [];
          wraps.push(line.isWrapped);
          for (let x = 0; x < f.cols; x++) {
            const p = line.getCell(x)!;
            // The shared renderer stores extended underline details on its
            // cell object; compare their values, not internal flag bit masks.
            const underline = p as typeof p & { getUnderlineStyle(): number; getUnderlineColorMode(): number; getUnderlineColor(): number };
            row.push([p.getWidth() === 1 ? p.getChars() || ' ' : p.getChars(), p.getWidth(), p.getFgColorMode(), p.getFgColor(), p.getBgColorMode(), p.getBgColor(), !!p.isBold(), !!p.isItalic(), !!p.isUnderline(), !!p.isInverse(), underline.getUnderlineStyle(), underline.getUnderlineColorMode(), underline.getUnderlineColor(), link(p as unknown as LinkCell)]);
          }
          cells.push(row);
        }
        const theme = (t as unknown as { _core: { _themeService: { colors: { ansi: { rgba: number }[]; foreground: { rgba: number }; background: { rgba: number }; cursor: { rgba: number } } } } })._core._themeService.colors;
        const colors = { palette: theme.ansi.map(c => c.rgba), fg: theme.foreground.rgba, bg: theme.background.rgba, cursor: theme.cursor.rgba };
        return { cells, wraps, colors, link: link(core._inputHandler._curAttrData), cursor: [v.cursorX, v.cursorY], buffer: v.type, modes: t.modes };
      };
      const compare = (left: Terminal, right: Terminal) => {
        const reference = state(left), restored = state(right);
        const equal = JSON.stringify(reference) === JSON.stringify(restored);
        return equal ? { equal } : { equal, reference, restored };
      };
      const replayReplies: string[] = [];
      const focusReports: { phase: string; data: string }[] = [];
      let phase = 'snapshot';
      const onReply = (data: string) => {
        // DECSET 1004 reports this viewer's current focus immediately. These
        // are owner-local input notifications in the production input policy,
        // distinct from device/status/color query replies, which remain forbidden.
        if (data === '\x1b[I' || data === '\x1b[O') focusReports.push({ phase, data });
        else replayReplies.push(data);
      };
      b.onData(onReply); c.onData(onReply);
      await write(a, f.original); await write(b, f.snapshot);
      const initial = compare(a, b);
      phase = 'live';
      await write(a, f.future); await write(b, f.future);
      const continued = compare(a, b);
      phase = 'recaptured';
      await write(c, f.recaptured);
      const recaptured = compare(a, c);
      phase = 'teardown';
      terminals.forEach(t => t.dispose());
      return { initial, continued, recaptured, replayReplies, focusReports };
    }, fixture);
    const receipt = info.outputPath('screen-replay.json');
    writeFileSync(receipt, JSON.stringify(result));
    await info.attach('screen replay', { path: receipt, contentType: 'application/json' });
    expect(result.initial.equal, 'initial snapshot').toBe(true);
    expect(result.continued.equal, 'output after first attachment').toBe(true);
    expect(result.recaptured.equal, 'second attachment after more output').toBe(true);
    expect(result.replayReplies, 'snapshot must not repeat terminal queries').toEqual([]);
    for (const report of result.focusReports) {
      expect(isTerminalLocalInput(report.data), 'focus notifications stay with their owning terminal').toBe(true);
      expect(shouldSuppressTerminalInput(report.data), 'focus is an input mode, not a terminal query').toBe(false);
    }
  });
}

test('active hyperlink profile retains the pen and releases disposed targets', async ({ page }, info) => {
  await page.goto(address);
  const result = await page.evaluate(async () => {
    const Constructor = (window as unknown as { Terminal: typeof Terminal }).Terminal;
    const fixed = new Constructor({ cols: 80, rows: 24, scrollback: 0 });
    const stock = new Constructor({ cols: 80, rows: 24, scrollback: 0 });
    const install = (window as unknown as { installHyperlinkProfile(t: Terminal): { dispose(): void } }).installHyperlinkProfile;
    const profile = install(fixed);
    fixed.open(document.getElementById('reference')!); stock.open(document.getElementById('restored')!);
    type Core = { _oscLinkService: { getLinkData(id: number): { uri: string } | undefined; _dataByLinkId: Map<number, unknown> }; _inputHandler: { _curAttrData: { extended: { urlId: number } } } };
    const core = (t: Terminal) => (t as unknown as { _core: Core })._core;
    const target = (t: Terminal) => { const c = core(t); return c._oscLinkService.getLinkData(c._inputHandler._curAttrData.extended.urlId)?.uri; };
    const write = (t: Terminal, data: string) => new Promise<void>(resolve => t.write(data, resolve));
    const sample = '\x1b]8;id=main;https://example.test/main\x07A\x1b[?1049h\x1b]8;id=alt;https://example.test/alt\x07B\x1b[?1049l';
    await write(stock, sample); await write(fixed, sample);
    const before = { stock: target(stock) ?? null, fixed: target(fixed) ?? null };
    await write(fixed, 'C');
    const continued = target(fixed);
    await write(fixed, '\x1bc');
    const resetSize = core(fixed)._oscLinkService._dataByLinkId.size;
    let maxHeld = 0, maxClosed = 0;
    for (let i = 0; i < 64; i++) {
      await write(fixed, `\x1b[?1049h\x1b]8;;https://example.test/${i}\x07B\x1b[?1049l`);
      if (target(fixed) !== `https://example.test/${i}`) throw new Error(`Active URI expired during switch ${i}: id=${core(fixed)._inputHandler._curAttrData.extended.urlId}, entries=${core(fixed)._oscLinkService._dataByLinkId.size}`);
      maxHeld = Math.max(maxHeld, core(fixed)._oscLinkService._dataByLinkId.size);
      await write(fixed, '\x1b]8;;\x07');
      maxClosed = Math.max(maxClosed, core(fixed)._oscLinkService._dataByLinkId.size);
    }
    fixed.reset();
    const directResetSize = core(fixed)._oscLinkService._dataByLinkId.size;
    await write(fixed, '\x1b]8;;https://example.test/scroll\x07A' + '\r\n'.repeat(60));
    const scrolled = target(fixed);
    fixed.reset();
    await write(fixed, '\x1b[?1049h\x1b]8;;https://example.test/dispose\x07B\x1b[?1049l');
    profile.dispose();
    const disposedSize = core(fixed)._oscLinkService._dataByLinkId.size;
    fixed.dispose(); stock.dispose();
    return { before, continued, resetSize, maxHeld, maxClosed, directResetSize, scrolled, disposedSize };
  });
  const receipt = info.outputPath('hyperlink-lifecycle.json');
  writeFileSync(receipt, JSON.stringify(result));
  await info.attach('hyperlink lifecycle', { path: receipt, contentType: 'application/json' });
  expect(result.before.stock, 'unmodified renderer reproduces the lost target').toBeNull();
  expect(result.before.fixed).toBe('https://example.test/alt');
  expect(result.continued).toBe('https://example.test/alt');
  expect(result.resetSize).toBe(0);
  expect(result.maxHeld, 'only the current markerless target is retained').toBe(1);
  expect(result.maxClosed, 'retired targets cannot accumulate').toBe(0);
  expect(result.directResetSize).toBe(0);
  expect(result.scrolled).toBe('https://example.test/scroll');
  expect(result.disposedSize).toBe(0);
});
