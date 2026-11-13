import assert from 'node:assert/strict';
import { test } from 'node:test';
import path from 'node:path';
import { isolatedPlaywrightRuns } from './playwright-runs.mts';

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
