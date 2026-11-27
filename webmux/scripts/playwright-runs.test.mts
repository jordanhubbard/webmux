import assert from 'node:assert/strict';
import { test } from 'node:test';
import path from 'node:path';
import { createRequire } from 'node:module';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, readFileSync, rmSync } from 'node:fs';
import os from 'node:os';
import { isolatedPlaywrightRuns, withPlaywrightOutput } from './playwright-runs.mts';
import { checkBrowser } from './check-browser.mts';
import type { BrowserType } from 'playwright-core';

test('browser readiness rejects exit after a closed context and always closes its browser', async () => {
  for (const exits of [false, true]) {
    let contexts = 0, closed = false, connected = true;
    const candidate = { launch: async () => ({
      newContext: async () => {
        contexts++;
        return {
          newPage: async () => ({ goto: async () => {}, title: async () => 'WebMux browser readiness' }),
          close: async () => { if (exits) connected = false; },
        };
      },
      isConnected: () => connected,
      close: async () => { closed = true; },
    }) } as unknown as Pick<BrowserType, 'launch'>;
    if (exits) await assert.rejects(checkBrowser(candidate), /exited after closing/);
    else await checkBrowser(candidate);
    assert.equal(contexts, exits ? 1 : 2);
    assert(closed, 'browser process was not closed after its probe');
  }
});

test('explicit unusable E2E browser refuses before provisioning or starting a suite', { skip: process.platform === 'win32' }, () => {
  const result = spawnSync('bash', [path.resolve(import.meta.dirname, '../../scripts/ensure-e2e-browser.sh')], {
    encoding: 'utf8', timeout: 10000,
    env: { ...process.env, PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH: '/missing/webmux-browser-fixture' },
  });
  assert.ifError(result.error);
  assert.notEqual(result.status, 0);
  assert.equal(result.stdout, '');
  assert.match(result.stderr, /Explicit E2E browser cannot render and reopen/);
  assert.doesNotMatch(result.stderr, /Provisioning Playwright/);
});

test('every discovered file gets an exact invocation and separate output', () => {
  const rootDir = path.resolve('fixture');
  const files = ['a.spec.ts', 'nested/b+[special].spec.ts'];
  const runs = isolatedPlaywrightRuns({ config: { rootDir }, suites: files.map(file => ({ file })), errors: [] });
  assert.equal(runs.length, files.length);
  for (const [i, file] of files.entries()) {
    const filter = new RegExp(runs[i][0]);
    assert(filter.test(path.resolve(rootDir, file)));
    assert(!filter.test(path.resolve(rootDir, 'prefix-' + file)));
    assert(!filter.test(path.resolve(rootDir, file + '.extra')));
  }
  assert.equal(new Set(runs.map(args => args[1])).size, files.length);
});

test('multiple discovered project suites for one file run all its projects once', () => {
  const runs = isolatedPlaywrightRuns({ config: { rootDir: path.resolve('fixture') }, suites: [{ file: 'a.spec.ts' }, { file: 'a.spec.ts' }] });
  assert.equal(runs.length, 1);
  assert(!runs[0].some(arg => arg.startsWith('--project')));
});

test('discovery errors or incomplete inventory cannot silently omit tests', () => {
  for (const report of [null, {}, { config: { rootDir: '/' }, suites: [] },
    { config: { rootDir: '' }, suites: [{ file: 'a.spec.ts' }] },
    { config: { rootDir: '/' }, suites: [{ file: 'a.spec.ts' }], errors: {} },
    { config: { rootDir: '/' }, suites: [{ file: 'a.spec.ts' }], errors: [{}] },
    { config: { rootDir: '/' }, suites: [{ file: 'a.spec.ts' }, {}] }]) {
    assert.throws(() => isolatedPlaywrightRuns(report));
  }
});

test('invocations share no implicit output parent and honor explicit CLI output', () => {
  const report = { config: { rootDir: path.resolve('fixture') }, suites: [{ file: 'a.spec.ts' }] };
  const first = isolatedPlaywrightRuns(report, path.join('test-results', 'run-one'));
  const second = isolatedPlaywrightRuns(report, path.join('test-results', 'run-two'));
  assert.notEqual(first[0][1], second[0][1]);
  assert.deepEqual(withPlaywrightOutput(['visual.spec.ts'], 'owned'), ['--output=owned', 'visual.spec.ts']);
  for (const args of [['visual.spec.ts', '--output=chosen'], ['--output', 'chosen', 'visual.spec.ts']]) {
    assert.deepEqual(withPlaywrightOutput(args, 'owned'), args);
  }
  assert.deepEqual(withPlaywrightOutput(['--', '--output=filter'], 'owned'), ['--output=owned', '--', '--output=filter']);
});

test('later real Playwright invocation preserves the earlier attached receipt', { timeout: 30_000 }, () => {
  const fixture = mkdtempSync(path.join(os.tmpdir(), 'wm-pw-evidence-'));
  const webmuxDir = path.resolve(import.meta.dirname, '..');
  const ownedRoots = new Set<string>();
  const playwright = createRequire(import.meta.url).resolve('@playwright/test');
  const config = path.join(fixture, 'playwright.config.mts');
  // Without the runner fix, the second invocation clears this private default
  // and loses the first attachment. No browser or app server is needed.
  writeFileSync(config, `export default { testDir: ${JSON.stringify(fixture)}, outputDir: ${JSON.stringify(path.join(fixture, 'default-output'))}, workers: 1, reporter: [['list']] };`);
  try {
    const receipts: string[] = [];
    for (const phase of ['profile', 'visual']) {
      const index = path.join(fixture, `${phase}-index.json`);
      const spec = path.join(fixture, `${phase}.spec.ts`);
      writeFileSync(spec, `import { test } from ${JSON.stringify(playwright)};
import { writeFileSync } from 'node:fs';
test('retained ${phase} diagnostic', async ({}, info) => {
 const receipt = info.outputPath('diagnostic.json');
 writeFileSync(receipt, JSON.stringify({ phase: ${JSON.stringify(phase)} }));
 await info.attach('diagnostic', { path: receipt, contentType: 'application/json' });
 writeFileSync(${JSON.stringify(index)}, JSON.stringify({ receipt }));
});`);
      // Positional arguments are regex filters, not literal filesystem paths;
      // a Windows absolute path would turn backslashes into regex escapes.
      const run = spawnSync(process.execPath, [path.join(import.meta.dirname, 'run-playwright.mts'), '--config', config, path.basename(spec)], {
        cwd: webmuxDir, env: { ...process.env, FORCE_COLOR: '0' }, encoding: 'utf8', timeout: 12_000,
      });
      assert.equal(run.status, 0, `${run.error?.message ?? ''}\n${run.stdout}\n${run.stderr}`);
      const { receipt } = JSON.parse(readFileSync(index, 'utf8')) as { receipt: string };
      const outputRoot = path.dirname(path.dirname(receipt));
      if (path.dirname(outputRoot) === path.join(webmuxDir, 'test-results') && /^run-[a-f0-9-]+$/.test(path.basename(outputRoot))) {
        ownedRoots.add(outputRoot);
      }
      receipts.push(receipt);
    }
    assert.equal(JSON.parse(readFileSync(receipts[0], 'utf8')).phase, 'profile');
    assert.equal(JSON.parse(readFileSync(receipts[1], 'utf8')).phase, 'visual');
    assert.equal(ownedRoots.size, 2);
  } finally {
    for (const directory of ownedRoots) rmSync(directory, { recursive: true, force: true });
    rmSync(fixture, { recursive: true, force: true });
  }
});
