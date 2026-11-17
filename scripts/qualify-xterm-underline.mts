/// <reference path="../webmux/node_modules/@xterm/xterm/typings/xterm.d.ts" />
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { createRequire } from 'node:module';
import { parseArgs } from 'node:util';
import type { Terminal } from '@xterm/xterm';

const { values } = parseArgs({ options: { output: { type: 'string' }, chromium: { type: 'string' } } });
assert(values.output, 'Pass --output pointing to a new JSON file');
const output = path.resolve(values.output);
assert(!fs.existsSync(output) && !fs.existsSync(output + '.receipt.json'), 'Output already exists');
const root = path.resolve(import.meta.dirname, '..');
const require = createRequire(path.join(root, 'webmux/package.json'));
const { chromium } = require('@playwright/test') as typeof import('../webmux/node_modules/@playwright/test/index.js');
const xterm = require.resolve('@xterm/xterm');
const hash = (file: string) => createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const fixture = path.join(root, 'webmux/server/internal/thirdparty/vt/testdata/webmux-xterm-underline.json');
type Input = { name: string; data: string; future: string; resizes?: [number, number][] };
// Never read expected states when capturing an independent reference.
const inputs = (JSON.parse(fs.readFileSync(fixture, 'utf8')) as Input[]).map(({ name, data, future, resizes }) => ({ name, data, future, resizes }));
assert(inputs.length === 126);
const browser = await chromium.launch({ headless: true, ...(values.chromium ? { executablePath: values.chromium } : {}) });
try {
  const page = await browser.newPage();
  await page.addScriptTag({ content: fs.readFileSync(xterm, 'utf8') });
  const result = await page.evaluate(async inputs => {
    const Constructor = (window as unknown as { Terminal: typeof Terminal }).Terminal;
    type Attr = {
      fg: number; bg: number; extended: { _ext: number; underlineStyle: number; underlineColor: number; urlId: number };
      getFgColorMode(): number; getFgColor(): number; getBgColorMode(): number; getBgColor(): number;
      isBold(): number; isDim(): number; isItalic(): number; isBlink(): number; isInverse(): number; isInvisible(): number; isStrikethrough(): number;
    };
    type Core = { _inputHandler: { _curAttrData: Attr }; _oscLinkService: { getLinkData(id: number): { uri: string } | undefined } };
    const color = (mode: number, value: number) => mode === 0 ? '' : mode === 0x1000000 ? `a${value}` : mode === 0x2000000 ? `i${value}` : `#${value.toString(16).padStart(6, '0')}`;
    const out = [];
    for (const input of inputs) {
      const t = new Constructor({ cols: 8, rows: 4, allowProposedApi: true });
      const core = (t as unknown as { _core: Core })._core;
      const style = (p: Attr) => ({ flags: 1 + (p.fg & 0x10000000 ? 1 : 0) + (p.bg & 0x10000000 ? 2 : 0),
        u: p.extended._ext >>> 26 & 7,
        vu: p.fg & 0x10000000 ? (p.bg & 0x10000000 ? p.extended.underlineStyle : 1) : 0,
        a: (p.isBold() ? 1 : 0) + (p.isDim() ? 2 : 0) + (p.isItalic() ? 4 : 0) + (p.isBlink() ? 8 : 0) + (p.isInverse() ? 32 : 0) + (p.isInvisible() ? 64 : 0) + (p.isStrikethrough() ? 128 : 0),
        fg: color(p.getFgColorMode(), p.getFgColor()), bg: color(p.getBgColorMode(), p.getBgColor()),
        uc: p.extended.underlineColor === -1 ? '' : color(p.extended.underlineColor & 0x3000000, p.extended.underlineColor & ((p.extended.underlineColor & 0x3000000) === 0x3000000 ? 0xffffff : 0xff)),
        l: core._oscLinkService.getLinkData(p.extended.urlId)?.uri ?? '' });
      const state = () => ({ cursor: [t.buffer.active.cursorX, t.buffer.active.cursorY], alt: t.buffer.active.type === 'alternate', pen: style(core._inputHandler._curAttrData),
        cells: Array.from({ length: t.rows }, (_, y) => Array.from({ length: t.cols }, (_, x) => {
          const c = t.buffer.active.getLine(t.buffer.active.baseY + y)!.getCell(x)!;
          return { c: c.getChars(), w: c.getWidth(), s: style(c as unknown as Attr) };
        })) });
      const write = (data: string) => new Promise<void>(resolve => t.write(data, resolve));
      await write(input.data);
      for (const [cols, rows] of input.resizes ?? []) t.resize(cols, rows);
      const initial = state(); await write(input.future); const continued = state();
      out.push({ ...input, initial, continued }); t.dispose();
    }
    return out;
  }, inputs);
  fs.mkdirSync(path.dirname(output), { recursive: true });
  // Keep each complete row on one line: full cell evidence without a
  // six-figure-line dump of repeated attribute keys.
  const stateJSON = (state: typeof result[number]['initial']) => [
    '{', `      "cursor": ${JSON.stringify(state.cursor)},`, `      "alt": ${state.alt},`,
    `      "pen": ${JSON.stringify(state.pen)},`, '      "cells": [',
    ...state.cells.map((row, i) => `        ${JSON.stringify(row)}${i + 1 === state.cells.length ? '' : ','}`),
    '      ]', '    }',
  ].join('\n');
  const text = '[\n' + result.map(item => {
    const { initial, continued, ...input } = item;
    return '  {\n' + Object.entries(input).filter(([, value]) => value !== undefined).map(([key, value]) => `    ${JSON.stringify(key)}: ${JSON.stringify(value)},\n`).join('')
      + `    "initial": ${stateJSON(initial)},\n    "continued": ${stateJSON(continued)}\n  }`;
  }).join(',\n') + '\n]\n';
  fs.writeFileSync(output, text, { flag: 'wx' });
  fs.writeFileSync(output + '.receipt.json', JSON.stringify({ captured_at: new Date().toISOString(), browser: browser.version(), xterm_sha256: hash(xterm), qualifier_sha256: hash(import.meta.filename), fixture_sha256: hash(fixture), output_sha256: hash(output), cases: result.length, scope: 'Unmodified xterm model reference. No snapshot adapter installed; no native/application qualification implied.' }, null, 2) + '\n', { flag: 'wx' });
  console.log(`Captured ${result.length} real xterm underline model cases`);
} finally { await browser.close(); }
