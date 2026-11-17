import { test, expect } from '@playwright/test';
import type { Terminal } from '@xterm/xterm';
import http from 'node:http';
import { execFileSync } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { transformWithOxc } from 'vite';
import { XTERM_SNAPSHOT_PROFILE, XTERM_UNDERLINE_SNAPSHOT_PROFILE } from '../../webmux/frontend/src/utils/terminalSnapshotProfile';
import { isTerminalLocalInput, shouldSuppressTerminalInput } from '../../webmux/frontend/src/utils/terminalInput';

type Fixture = { name: string; cols: number; rows: number; original: string; snapshot: string; future: string; recaptured: string; beforeCols?: number; beforeRows?: number; resizes?: [number, number][]; snapshotError?: string; recapturedError?: string; snapshotProfile?: string; snapshotToken?: string; recapturedProfile?: string; recapturedToken?: string; futureResizes?: [number, number][]; afterCols?: number; afterRows?: number };
const names = ['wrap-toggle-pending', 'wrap-toggle-resolved', 'link-named', 'link-anonymous', 'link-replace', 'link-unicode', 'link-semicolon', 'link-wrapped', 'link-id-reuse', 'link-save', 'link-alternate', 'link-close-params', 'palette-short-hex', 'palette-short-rgb', 'palette-long-rgb', 'palette-long-hex', 'palette-single', 'palette-batch', 'palette-query', 'palette-reset', 'dynamic-colors', 'dynamic-query', 'dynamic-reset', 'saved-underline-csi', 'saved-underline-single', 'saved-underline-color', 'saved-underline-color-reset', 'saved-underline-batched', 'saved-underline-twice', 'saved-underline', 'saved-underline-1048', 'saved-underline-1049', 'alternate-wrapped-row', 'alternate-wrapped-blank', 'origin-clamp-top', 'origin-clamp-bottom', 'origin-clamp-then-widen', 'origin-batched-reset', 'origin-batched-reset-reverse', 'origin-active-at-restore', 'origin-after-ris', 'origin-after-soft-reset', 'saved-origin', 'saved-pending-wrap', 'saved-origin-csi', 'saved-origin-1048', 'saved-pending-1049', 'immediate', 'colored-underline', 'unicode', 'combining', 'pending-wrap', 'wide-pending-wrap', 'insert-mode', 'after-sgr', 'after-bell', 'after-private-osc', 'after-query', 'after-repeat', 'c0-inside-csi', 'del-inside-csi', 'after-del', 'after-erased-character'];
const noWrapCases = JSON.parse(readFileSync(path.resolve(__dirname, '../../webmux/server/internal/thirdparty/vt/testdata/webmux-xterm-nowrap.json'), 'utf8')) as { name: string }[];
noWrapCases.push(...['rep-space', 'rep-ascii', 'rep-wide-placeholder', 'rep-colored-null', 'soft-reset-focus', 'soft-reset-paste', 'soft-reset-sync', 'region-bottom-wrap', 'region-top-reverse-index', 'region-bottom-index', 'region-saved-restore'].map(name => ({ name })));
names.push(...noWrapCases.flatMap(c => [`nowrap-${c.name}`, `nowrap-${c.name}-byte-split`]));
for (const base of ['address', 'advanced', 'wide', 'overlap', 'overlap-cleared', 'saved', 'saved-wide', 'alternate']) {
  for (const size of ['grow', 'shrink', 'shrink-grow']) {
    names.push(`nowrap-resize-${base}-${size}`, `nowrap-resize-${base}-${size}-byte-split`);
  }
}
const recordedInputs = process.env.WEBMUX_SCREEN_REFERENCE_INPUTS;
if (recordedInputs) {
  const inputs = JSON.parse(readFileSync(recordedInputs, 'utf8')) as { name: string }[];
  names.push(...inputs.map(input => `recorded-conpty-${input.name}`));
}
const NATIVE_SCREEN = Boolean(process.env.WEBMUX_NATIVE_SCREEN_CAPTURES || process.env.WEBMUX_NATIVE_SCREEN_TEST_LOG) || (process.platform === 'win32' && process.env.WEBMUX_SCREEN_REPLAY_ONLY === '1');
const nativeNames = ['hyperlink-named', 'hyperlink-anonymous', 'hyperlink-unicode', 'hyperlink-reuse', 'palette-live-change', 'dynamic-colors', 'underline-restore', 'underline-1049', 'origin-restore', 'origin-clamp', 'origin-mode-batch', 'pending-wrap', 'pending-wrap-alt', 'nowrap-address', 'nowrap-advanced', 'nowrap-overwrite', 'nowrap-toggle', 'nowrap-wide', 'nowrap-wide-overlap', 'nowrap-wide-discard', 'nowrap-backspace', 'nowrap-soft-reset'];
names.push(...names.map(name => `underline-baseline-${name}`));
const underlineInputs = JSON.parse(readFileSync(path.resolve(__dirname, '../../webmux/server/internal/thirdparty/vt/testdata/webmux-xterm-underline.json'), 'utf8')) as { name: string }[];
underlineInputs.push(...JSON.parse(readFileSync(path.resolve(__dirname, '../../webmux/server/internal/screenstate/testdata/underline-cases.json'), 'utf8')) as { name: string }[]);
names.push(...underlineInputs.flatMap(input => [`underline-${input.name}`, `underline-${input.name}-byte-split`]));
if (NATIVE_SCREEN) names.push(...nativeNames.map(name => `conpty-${name}`));
let fixtures: Fixture[];
let server: http.Server;
let address: string;

test.beforeAll(async () => {
  test.setTimeout(NATIVE_SCREEN ? 240_000 : 60_000);
  const root = path.resolve(__dirname, '../..');
  fixtures = JSON.parse(execFileSync('go', ['run', './internal/screenstate/testdata/replay-fixtures'], {
    // This is the aggregate fixture inventory, not the per-snapshot wire bound.
    cwd: path.join(root, 'webmux/server'), encoding: 'utf8', timeout: 60_000, maxBuffer: 8 << 20,
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
  const snapshotProfilePath = path.join(root, 'webmux/frontend/src/utils/terminalSnapshotProfile.ts');
  const snapshotProfile = (await transformWithOxc(readFileSync(snapshotProfilePath, 'utf8'), snapshotProfilePath)).code;
  server = http.createServer((req, res) => {
    if (req.url === '/xterm.js') { res.setHeader('Content-Type', 'text/javascript; charset=utf-8'); res.end(library); }
    else if (req.url === '/snapshot-profile.js') { res.setHeader('Content-Type', 'text/javascript'); res.end(snapshotProfile); }
    else if (req.url === '/hyperlink-profile.js') { res.setHeader('Content-Type', 'text/javascript'); res.end(profile); }
    else if (req.url === '/xterm.css') { res.setHeader('Content-Type', 'text/css; charset=utf-8'); res.end(css); }
    else { res.setHeader('Content-Type', 'text/html; charset=utf-8'); res.end('<link rel="stylesheet" href="/xterm.css"><div id="reference"></div><div id="restored"></div><div id="recaptured"></div><script src="/xterm.js"></script><script type="module">import{retainActiveTerminalHyperlink}from"/hyperlink-profile.js";import{installTerminalSnapshotProfile}from"/snapshot-profile.js";window.installSnapshotProfile=installTerminalSnapshotProfile;window.installHyperlinkProfile=retainActiveTerminalHyperlink;</script>'); }
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  address = `http://127.0.0.1:${(server.address() as { port: number }).port}`;
});
test.afterAll(async () => { if (server) await new Promise<void>(resolve => server.close(() => resolve())); });

for (const name of names) {
  test(`Go screen snapshot matches xterm ${name}`, async ({ page }, info) => {
    const fixture = fixtures.find(f => f.name === name)!;

    for (const [legacyError, profile, token] of [[fixture.snapshotError, fixture.snapshotProfile, fixture.snapshotToken], [fixture.recapturedError, fixture.recapturedProfile, fixture.recapturedToken]]) {
      if (profile === XTERM_UNDERLINE_SNAPSHOT_PROFILE) {
        expect(name.startsWith('underline-')).toBe(true); expect(token).toMatch(/^[a-f0-9]{32}$/);
        if (legacyError !== undefined) expect(legacyError).toBe('terminal screen state cannot be restored safely; retain the existing job and recover manually');
      } else if (legacyError !== undefined) {
        expect(legacyError).toBe('terminal screen state cannot be restored safely; retain the existing job and recover manually');
        expect(profile, 'unsupported ANSI state requires explicit capability').toBe(XTERM_SNAPSHOT_PROFILE);
        expect(token).toMatch(/^[a-f0-9]{32}$/);
      } else {
        expect(profile).toBeUndefined(); expect(token).toBeUndefined();
      }
    }
    await page.goto(address);
    const result = await page.evaluate(async f => {
      const Constructor = (window as unknown as { Terminal: typeof Terminal }).Terminal;

      type SnapshotProfile = { setToken(token?: string, profile?: string): void; dispose(): void };
      const snapshotProfiles: (SnapshotProfile | undefined)[] = [];
      const profiles: { dispose(): void }[] = [];
      const terminals = ['reference', 'restored', 'recaptured'].map(id => {
        const t = new Constructor({ cols: id === 'reference' ? f.beforeCols ?? f.cols : id === 'recaptured' ? f.afterCols ?? f.cols : f.cols, rows: id === 'reference' ? f.beforeRows ?? f.rows : id === 'recaptured' ? f.afterRows ?? f.rows : f.rows, allowProposedApi: true });

        profiles.push((window as unknown as { installHyperlinkProfile(t: Terminal): { dispose(): void } }).installHyperlinkProfile(t));
        const snapshotProfile = id === 'reference' ? undefined : (window as unknown as { installSnapshotProfile(t: Terminal): SnapshotProfile }).installSnapshotProfile(t);
        snapshotProfiles.push(snapshotProfile);
        if (snapshotProfile) profiles.push(snapshotProfile);
        t.open(document.getElementById(id)!);
        return t;
      });
      const [a, b, c] = terminals;
      snapshotProfiles[1]!.setToken(f.snapshotToken, f.snapshotProfile);
      snapshotProfiles[2]!.setToken(f.recapturedToken, f.recapturedProfile);
      const referenceUsesSnapshotProfile = typeof Object.getOwnPropertyDescriptor((a as unknown as { _core: { _inputHandler: { _parser: object } } })._core._inputHandler._parser, 'precedingJoinState')?.get === 'function';
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
        for (let y = 0; y < t.rows; y++) {
          const line = v.getLine(v.viewportY + y)!, row = [];
          wraps.push(line.isWrapped);
          for (let x = 0; x < t.cols; x++) {
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
        type RawAttr = { fg: number; bg: number; extended: { _ext: number; underlineColor: number; urlId: number } };
        const rawAttr = (p: RawAttr) => {
          const raw = p.extended.underlineColor, mode = raw & 0x3000000;
          return { fg: p.fg, bg: p.bg, u: p.extended._ext >>> 26 & 7, color: mode ? mode | raw & (mode === 0x3000000 ? 0xffffff : 0xff) : 0, uri: core._oscLinkService.getLinkData(p.extended.urlId)?.uri ?? '' };
        };
        const raw = f.name.startsWith('underline-') ? { pen: rawAttr(core._inputHandler._curAttrData as unknown as RawAttr),
          rows: Array.from({ length: t.rows }, (_, y) => Array.from({ length: t.cols }, (_, x) => { const c = v.getLine(v.baseY + y)!.getCell(x)!; return { c: c.getChars(), w: c.getWidth(), s: rawAttr(c as unknown as RawAttr) }; })),
        } : undefined;
        return { cells, wraps, colors, link: link(core._inputHandler._curAttrData), cursor: [v.cursorX, v.cursorY], buffer: v.type, modes: t.modes, raw };
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
      await write(a, f.original);
      for (const [cols, rows] of f.resizes ?? []) a.resize(cols, rows);
      await write(b, f.snapshot);
      const scratchCount = (t: Terminal) => {
        type Links = { _dataByLinkId: Map<number, unknown>; getLinkData(id: number): { uri: string } | undefined };
        const service = (t as unknown as { _core: { _oscLinkService: Links } })._core._oscLinkService;
        return [...service._dataByLinkId.keys()].filter(id => service.getLinkData(id)?.uri === 'https://webmux.invalid/snapshot-pen').length;
      };
      const initialScratchLinks = scratchCount(b);
      const initial = compare(a, b);
      phase = 'live';
      for (const [cols, rows] of f.futureResizes ?? []) { a.resize(cols, rows); b.resize(cols, rows); }
      await write(a, f.future); await write(b, f.future);
      const continued = compare(a, b);
      phase = 'recaptured';
      await write(c, f.recaptured);
      const recaptured = compare(a, c);
      const recapturedScratchLinks = scratchCount(c);
      phase = 'teardown';
      profiles.forEach(profile => profile.dispose());
      terminals.forEach(t => t.dispose());
      return { initial, continued, recaptured, replayReplies, focusReports, referenceUsesSnapshotProfile, initialScratchLinks, recapturedScratchLinks };
    }, fixture);
    const receipt = info.outputPath('screen-replay.json');
    writeFileSync(receipt, JSON.stringify(result));
    await info.attach('screen replay', { path: receipt, contentType: 'application/json' });
    expect(result.referenceUsesSnapshotProfile, 'independent reference must use the unmodified parser').toBe(false);
    expect(result.initialScratchLinks).toBeLessThanOrEqual(1); expect(result.recapturedScratchLinks).toBeLessThanOrEqual(1);
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


test('snapshot parser profile is token bound, bounded, split safe and disposable', async ({ page }) => {
  await page.goto(address);
  const result = await page.evaluate(async () => {
    const Constructor = (window as unknown as { Terminal: typeof Terminal }).Terminal;
    const term = new Constructor({ cols: 12, rows: 4, allowProposedApi: true });
    term.open(document.getElementById('restored')!);
    type Profile = { setToken(token?: string): void; dispose(): void };
    const install = (window as unknown as { installSnapshotProfile(t: Terminal): Profile }).installSnapshotProfile;
    const parser = (term as unknown as { _core: { _inputHandler: { _parser: { precedingJoinState: number } } } })._core._inputHandler._parser;
    const write = (data: string) => new Promise<void>(resolve => term.write(data, resolve));
    const token = '0123456789abcdef0123456789abcdef';
    const packet = (payload: string, nonce = token) => `\x1b]7777;webmux.replay.v1;${nonce};${payload}\x07`;
    const input: string[] = []; term.onData(data => input.push(data));
    await write('Z'); const expected = parser.precedingJoinState;
    const profile = install(term);
    let duplicateRefused = false;
    try { install(term); } catch { duplicateRefused = true; }
    await write(packet('Wg==')); const unarmed = parser.precedingJoinState;
    profile.setToken(token);
    const valid = packet('Wg==');
    for (let split = 0; split <= valid.length; split++) {
      term.reset(); profile.setToken(token); await write('Z');
      await write(valid.slice(0, split)); await write(valid.slice(split));
      if (parser.precedingJoinState !== expected) throw new Error(`join state lost at split ${split}`);
      await write('\x1b[2b');
      if (term.buffer.active.cursorX !== 3 || term.buffer.active.getLine(0)!.translateToString(true) !== 'ZZZ') throw new Error(`REP failed at split ${split}`);
    }
    const bad = [packet('Wg==', 'f'.repeat(32)), packet(''), packet('***='), packet('/w=='), packet('AA=='), packet('woU='), packet(btoa('x'.repeat(65))), packet('Wg==') + '\x1b[0m'];
    for (const data of bad) {
      term.reset(); profile.setToken(token); await write('Z'); await write(data);
      if (parser.precedingJoinState !== 0 || term.buffer.active.cursorX !== 1) throw new Error('invalid packet retained repeat state or changed cursor');
      await write('\x1b[2b');
      if (term.buffer.active.cursorX !== 1) throw new Error('invalid packet enabled REP');
    }
    term.reset(); profile.setToken(token); await write('Z'); await write(valid); await write('\u0301');
    const combined = term.buffer.active.getLine(0)!.getCell(0)!.getChars();
    await write(valid); const reused = parser.precedingJoinState;
    profile.setToken(token); await write(valid); term.reset(); const reset = parser.precedingJoinState;
    const stock = new Constructor(); await new Promise<void>(resolve => stock.write('Z', resolve)); stock.reset();
    const stockReset = (stock as unknown as { _core: { _inputHandler: { _parser: { precedingJoinState: number } } } })._core._inputHandler._parser.precedingJoinState; stock.dispose();
    await write('\x1bc'); const ris = parser.precedingJoinState;
    profile.setToken(undefined); await write(valid); const disarmed = parser.precedingJoinState;
    let invalidTokenRefused = false;
    try { profile.setToken('invalid'); } catch { invalidTokenRefused = true; }
    profile.setToken(token); await write(valid); const beforeDispose = parser.precedingJoinState;
    profile.dispose(); profile.dispose();
    const descriptor = Object.getOwnPropertyDescriptor(parser, 'precedingJoinState')!;
    const retainedOnDispose = parser.precedingJoinState;
    await write(valid); const afterDispose = parser.precedingJoinState;
    const again = install(term); again.dispose(); term.dispose();
    return { expected, splits: valid.length + 1, duplicateRefused, unarmed, combined, reused, reset, stockReset, ris, disarmed, invalidTokenRefused, beforeDispose, retainedOnDispose, afterDispose, restoredDataProperty: descriptor.writable === true && descriptor.get === undefined, input };
  });
  expect(result.expected).toBeGreaterThan(0);
  expect(result.splits).toBeGreaterThan(50);
  expect(result.duplicateRefused).toBe(true);
  expect(result.unarmed).toBe(0);
  expect(result.combined).toBe('Z\u0301'); expect(result.reused).toBe(0);
  expect(result.reset).toBe(result.stockReset); expect(result.ris).toBe(0); expect(result.disarmed).toBe(0);
  expect(result.invalidTokenRefused).toBe(true);
  expect(result.beforeDispose).toBe(result.expected);
  expect(result.retainedOnDispose).toBe(result.expected);
  expect(result.afterDispose).toBe(0);
  expect(result.restoredDataProperty).toBe(true);
  expect(result.input).toEqual([]);
});


// Isolated v2 decoder qualification. This does not enable the production screen
// encoder or claim that an actual application/native attach can restore it yet.
test('snapshot underline v2 preserves saved attributes with bounded cursor columns', async ({ page }, info) => {
  await page.goto(address);
  const result = await page.evaluate(async () => {
    const Constructor = (window as unknown as { Terminal: typeof Terminal }).Terminal;
    type Profile = { setToken(token?: string, profile?: string): void; dispose(): void };
    type Attr = { fg: number; bg: number; extended: { _ext: number; underlineColor: number; urlId: number } };
    type Core = { _inputHandler: { _curAttrData: Attr; _parser: { precedingJoinState: number } };
      _bufferService: { buffers: { normal: { savedCurAttrData: Attr; savedX: number; savedY: number; savedCharset: unknown }; alt: { savedCurAttrData: Attr; savedX: number; savedY: number; savedCharset: unknown } } };
      _oscLinkService: { getLinkData(id: number): { uri: string } | undefined } };
    const core = (t: Terminal) => (t as unknown as { _core: Core })._core;
    const install = (window as unknown as { installSnapshotProfile(t: Terminal): Profile }).installSnapshotProfile;
    const token = '0123456789abcdef0123456789abcdef';
    const version = 'webmux-xterm-state-v2';
    const packet = (value: unknown, nonce = token, v = 'v2') => `\x1b]7777;webmux.replay.${v};${nonce};${btoa(String.fromCharCode(...new TextEncoder().encode(JSON.stringify(value))))}\x07`;
    const write = (t: Terminal, data: string) => new Promise<void>(resolve => t.write(data, resolve));
    const attr = (t: Terminal, p: Attr) => ({ fg: p.fg, bg: p.bg, raw: p.extended._ext, color: p.extended.underlineColor,
      link: core(t)._oscLinkService.getLinkData(p.extended.urlId)?.uri ?? '' });
    const state = (t: Terminal) => ({ pen: attr(t, core(t)._inputHandler._curAttrData), cursor: [t.buffer.active.cursorX, t.buffer.active.cursorY], type: t.buffer.active.type,
      join: core(t)._inputHandler._parser.precedingJoinState, modes: t.modes,
      cells: Array.from({ length: t.rows }, (_, y) => Array.from({ length: t.cols }, (_, x) => {
        const c = t.buffer.active.getLine(t.buffer.active.baseY + y)!.getCell(x)!;
        return { c: c.getChars(), w: c.getWidth(), a: attr(t, c as unknown as Attr) };
      })) });
    const saved = (t: Terminal) => Object.fromEntries((['normal', 'alt'] as const).map(name => { const b = core(t)._bufferService.buffers[name]; return [name, { x: b.savedX, y: b.savedY, attr: attr(t, b.savedCurAttrData), charset: b.savedCharset }]; }));
    const cases = [
      'A\x1b7\x1b[4:3;58;5;9mB\x1b8',
      '\x1b[4:3mA\x1b7\x1b[0mB\x1b8',
      '\x1b[4:3mA\x1b7\x1b[24mB\x1b8',
      '\x1b]8;;https://example.test/a\x07A\x1b7\x1b]8;;\x07\x1b[4:5mB\x1b8',
      'A\x1b7\x1b[58;5;9mB\x1b8',
      '\x1b[31;42;1;3m\x1b[1"q\x1b(0\x1b7\x1b[0m\x1b(B',
      '\x1b[4:3;58;5;2mA\x1b[?1049h\x1b[4:5;58;5;9mB',
      '\x1b[4:3mA\x1b[?1049h\x1b[0mB\x1b[?1049l',
    ];
    const futures = ['X', '\x1b[59mX', '\x1b[58;5;4mX', '\x1b[4mX', '\x1b[24mX', '\x1b8q', '\x1b[?1049lX', '\x1b[2b\u0301'];
    let controls = 0;
    const failures: unknown[] = [];
    for (const original of cases) for (const future of futures) for (const cluster of ['', 'Z', 'e\u0301', '日']) {
      const a = new Constructor({ cols: 12, rows: 4, allowProposedApi: true });
      const b = new Constructor({ cols: 12, rows: 4, allowProposedApi: true });
      const profile = install(b);
      await write(a, original + '\x1b]777;boundary\x07' + cluster); await write(b, original + '\x1b]777;boundary\x07' + cluster);
      const source = core(a)._inputHandler._curAttrData;
      const flags = 1 + (source.fg & 0x10000000 ? 1 : 0) + (source.bg & 0x10000000 ? 2 : 0);
      const beforeSaved = JSON.stringify(saved(b));
      const target = core(b)._inputHandler._curAttrData;
      const extended = target.extended;
      // Deliberately disturb only the flags, simulating the missing final tail
      // of a future ANSI encoder. All expected state comes from stock xterm.
      target.fg ^= 0x10000000; target.bg ^= 0x10000000;
      profile.setToken(token, version); await write(b, packet({ u: flags, r: cluster }));
      const initial = JSON.stringify(state(a)) === JSON.stringify(state(b));
      const unchanged = beforeSaved === JSON.stringify(saved(b)) && extended === core(b)._inputHandler._curAttrData.extended;
      await write(a, future); await write(b, future);
      const continued = JSON.stringify(state(a)) === JSON.stringify(state(b));
      if (!initial || !continued || !unchanged) failures.push({ original, future, cluster, initial, continued, unchanged, reference: state(a), restored: state(b) });
      controls++; profile.dispose(); a.dispose(); b.dispose();
    }
    const a = new Constructor({ cols: 12, rows: 4, allowProposedApi: true });
    const profile = install(a);
    const replies: string[] = []; a.onData(data => replies.push(data));
    const valid = packet({ u: 2, r: 'Z' });
    for (let split = 0; split <= valid.length; split++) {
      a.reset(); await write(a, 'Z\x1b[4:3m'); profile.setToken(token, version);
      await write(a, valid.slice(0, split)); await write(a, valid.slice(split));
      await write(a, '\x1b[2b');
      if (a.buffer.active.cursorX !== 3 || a.buffer.active.getLine(0)!.translateToString(true) !== 'ZZZ' || (core(a)._inputHandler._curAttrData.bg & 0x10000000)) throw new Error(`v2 lost flags/REP at split ${split}`);
    }
    const full = packet({ u: 2, r: 'Z', x: 12, n: 12, a: 12 });
    for (let split = 0; split <= full.length; split++) {
      a.reset(); await write(a, 'Z'.repeat(12) + '\x1b[4:3m'); profile.setToken(token, version);
      await write(a, full.slice(0, split)); await write(a, full.slice(split));
      const buffers = core(a)._bufferService.buffers;
      if (a.buffer.active.cursorX !== 12 || buffers.normal.savedX !== 12 || buffers.alt.savedX !== 12 || (core(a)._inputHandler._curAttrData.bg & 0x10000000)) throw new Error(`cursor tail lost at split ${split}`);
      await write(a, '\x1b[2b');
      const current = a.buffer.active;
      if (current.cursorX !== 2 || current.cursorY !== 1 || current.getLine(1)!.translateToString(true) !== 'ZZ') throw new Error(`cursor tail REP failed at split ${split}`);
    }
    const invalid = [null, [], {}, { u: 1 }, { r: '' }, { u: 0, r: '' }, { u: 5, r: '' }, { u: 1.5, r: '' }, { u: '2', r: '' }, { u: 2, r: null }, { u: 2, r: '', extra: 1 }, { u: 2, r: 'x'.repeat(65) }, { u: 2, r: '\x1b' }, { u: 2, r: '\u0085' }, { u: 2, r: '\ud800' }, { u: 2, r: '', padding: 'x'.repeat(200) }, ...[{ x: -1 }, { x: 13 }, { x: 1.5 }, { x: null }, { n: '0' }, { n: -1 }, { a: 13 }, { a: null }].map(column => ({ u: 2, r: '', ...column }))];
    for (const payload of invalid) {
      a.reset(); await write(a, '\x1b[4:3mZ'); profile.setToken(token, version);
      const before = JSON.stringify(attr(a, core(a)._inputHandler._curAttrData));
      await write(a, packet(payload));
      if (before !== JSON.stringify(attr(a, core(a)._inputHandler._curAttrData)) || core(a)._inputHandler._parser.precedingJoinState !== 0) throw new Error('invalid v2 packet mutated pen or retained REP');
      await write(a, valid); // Invalid packets must not consume the nonce.
      if (core(a)._inputHandler._curAttrData.bg & 0x10000000) throw new Error('invalid packet consumed nonce');
    }
    const badEncoding = ['***=', '/w==', btoa('{'), btoa('{"u":2,"r":""}') + '='];
    for (const encoded of badEncoding) {
      a.reset(); await write(a, '\x1b[4:3mZ'); profile.setToken(token, version);
      const before = JSON.stringify(attr(a, core(a)._inputHandler._curAttrData));
      await write(a, `\x1b]7777;webmux.replay.v2;${token};${encoded}\x07`);
      if (before !== JSON.stringify(attr(a, core(a)._inputHandler._curAttrData)) || core(a)._inputHandler._parser.precedingJoinState !== 0) throw new Error('bad v2 encoding mutated pen or retained REP');
      await write(a, valid);
      if (core(a)._inputHandler._curAttrData.bg & 0x10000000) throw new Error('bad encoding consumed nonce');
    }
    for (const wrongVersion of [false, true]) {
      a.reset(); await write(a, '\x1b[4:3mZ'); profile.setToken(token, wrongVersion ? version : undefined);
      const before = JSON.stringify(attr(a, core(a)._inputHandler._curAttrData));
      const data = wrongVersion ? `\x1b]7777;webmux.replay.v1;${token};Wg==\x07` : valid;
      await write(a, data);
      if (before !== JSON.stringify(attr(a, core(a)._inputHandler._curAttrData)) || core(a)._inputHandler._parser.precedingJoinState !== 0) throw new Error('packet version crossed metadata boundary');
    }
    a.reset(); await write(a, '\x1b[4:3mZ'); profile.setToken(token, version);
    await write(a, packet({ u: 2, r: '' }, 'f'.repeat(32)));
    if (!(core(a)._inputHandler._curAttrData.bg & 0x10000000)) throw new Error('stale nonce changed flags');
    await write(a, packet({ u: 2, r: '' })); await write(a, '\x1b[2b');
    const clearedRepeat = a.buffer.active.cursorX === 1;
    await write(a, '\x1b[4:3m'); await write(a, valid);
    const reusedRefused = Boolean(core(a)._inputHandler._curAttrData.bg & 0x10000000) && core(a)._inputHandler._parser.precedingJoinState === 0;
    profile.setToken(undefined, version); await write(a, valid);
    const disarmed = Boolean(core(a)._inputHandler._curAttrData.bg & 0x10000000);
    profile.dispose(); await write(a, valid);
    const disposed = Boolean(core(a)._inputHandler._curAttrData.bg & 0x10000000);
    a.dispose();
    return { controls, failures, splits: valid.length + 1, cursorSplits: full.length + 1, invalid: invalid.length, badEncoding: badEncoding.length, clearedRepeat, reusedRefused, disarmed, disposed, replies };
  });
  writeFileSync(info.outputPath('underline-v2-decoder.json'), JSON.stringify(result));
  expect(result.failures).toEqual([]);
  expect(result.controls).toBe(256);
  expect(result.splits).toBeGreaterThan(60); expect(result.cursorSplits).toBeGreaterThan(90);
  expect(result.invalid).toBe(24); expect(result.badEncoding).toBe(4);
  expect(result.clearedRepeat).toBe(true); expect(result.reusedRefused).toBe(true);
  expect(result.disarmed).toBe(true); expect(result.disposed).toBe(true); expect(result.replies).toEqual([]);
});
