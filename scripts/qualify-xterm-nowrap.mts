/// <reference path="../webmux/node_modules/@xterm/xterm/typings/xterm.d.ts" />
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { createRequire } from 'node:module';
import { parseArgs } from 'node:util';
import type { Terminal } from '@xterm/xterm';

// Capture expected state directly from the shipped xterm implementation.
// Only names and input bytes are read from the checked-in fixture.
const { values } = parseArgs({ options: {
  output: { type: 'string' }, chromium: { type: 'string' },
} });
assert(values.output, 'Pass --output pointing to a new JSON file');
const output = path.resolve(values.output);
assert(!fs.existsSync(output) && !fs.existsSync(output + '.receipt.json'), 'Output already exists');
const source = path.resolve(import.meta.dirname, '..');
const require = createRequire(path.join(source, 'webmux/package.json'));
const { chromium } = require('@playwright/test') as typeof import('../webmux/node_modules/@playwright/test/index.js');
const xterm = require.resolve('@xterm/xterm');
const hash = (file: string) => createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const fixture = path.join(source, 'webmux/server/internal/thirdparty/vt/testdata/webmux-xterm-nowrap.json');
const cases: { name: string; data: string }[] = JSON.parse(fs.readFileSync(fixture, 'utf8'));
assert(cases.length === 138 && cases.every(c => typeof c.name === 'string' && typeof c.data === 'string'));
const browser = await chromium.launch({ headless: true, ...(values.chromium ? { executablePath: values.chromium } : {}) });
try {
  const page = await browser.newPage();
  await page.setContent('<div id="root"></div>');
  await page.addScriptTag({ content: fs.readFileSync(xterm, 'utf8') });
  const results = await page.evaluate(async inputs => {
    const Constructor = (window as unknown as { Terminal: typeof Terminal }).Terminal;
    const out = [];
    for (const { name, data } of inputs) {
      const terminal = new Constructor({ cols: 8, rows: 4, allowProposedApi: true });
      terminal.open(document.getElementById('root')!);
      await new Promise<void>(resolve => terminal.write(data, resolve));
      const buffer = terminal.buffer.active;
      out.push({ name, data, cursor: [buffer.cursorX, buffer.cursorY],
        cells: Array.from({ length: 4 }, (_, y) => Array.from({ length: 8 }, (_, x) => buffer.getLine(y)!.getCell(x)!.getChars() || ' ')),
        lines: Array.from({ length: 4 }, (_, y) => buffer.getLine(y)!.translateToString(false)),
        widths: Array.from({ length: 8 }, (_, x) => buffer.getLine(1)!.getCell(x)!.getWidth()),
      });
      terminal.dispose();
    }
    return out;
  }, cases.map(({ name, data }) => ({ name, data })));
  fs.mkdirSync(path.dirname(output), { recursive: true });
  fs.writeFileSync(output, JSON.stringify(results, null, 2) + '\n', { flag: 'wx' });
  fs.writeFileSync(output + '.receipt.json', JSON.stringify({
    captured_at: new Date().toISOString(), browser: browser.version(),
    xterm_sha256: hash(xterm), qualifier_sha256: hash(import.meta.filename),
    fixture_sha256: hash(fixture), output_sha256: hash(output), cases: results.length,
  }, null, 2) + '\n', { flag: 'wx' });
  console.log(`Captured ${results.length} real xterm cases in ${output}`);
} finally {
  await browser.close();
}
