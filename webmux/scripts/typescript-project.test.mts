import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createRequire } from 'node:module';
import { test } from 'node:test';
import { checkedProjectSources, sourceCoverageProblems } from './typescript-project.mts';

const compiler = process.env.WEBMUX_TEST_TYPESCRIPT_CLI || createRequire(import.meta.url).resolve('typescript/bin/tsc');
test('real compiler discovery resolves inherited strict config and preserves Git source-inventory enforcement', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux type coverage '));
  const write = (file: string, value: string) => { fs.mkdirSync(path.dirname(path.join(root, file)), { recursive: true }); fs.writeFileSync(path.join(root, file), value); };
  try {
    execFileSync('git', ['init', '--quiet', root]);
    write('base.json', JSON.stringify({ compilerOptions: { strict: true } }));
    write('config/tsconfig.json', JSON.stringify({ extends: '../base.json', include: ['../source/**/*.ts', './*.ts'], exclude: ['../source/excluded.ts'] }));
    write('config/local.ts', 'export const sibling: number = 0;');
    write('source/retained.ts', 'export const retained: number = 1;');
    write('source/not-added.ts', 'export const local: number = 2;');
    write('source/excluded.ts', 'export const excluded = 3;');
    write('loose.ts', 'export const outside = 4;');
    write('maintained.js', 'const retained = 1;');
    write('.gitignore', 'ignored/\n'); write('ignored/generated.js', 'const generated = 1;');
    execFileSync('git', ['-C', root, 'add', 'source/retained.ts']);
    const project = path.join(root, 'config/tsconfig.json');
    const covered = checkedProjectSources(compiler, project);
    assert.deepEqual([...covered].filter(file => file.startsWith(fs.realpathSync(root) + path.sep)).sort(), ['config/local.ts', 'source/not-added.ts', 'source/retained.ts'].map(file => fs.realpathSync(path.join(root, file))).sort());
    const files = execFileSync('git', ['-C', root, 'ls-files', '-z', '--cached', '--others', '--exclude-standard'], { encoding: 'utf8' }).split('\0').filter(Boolean);
    assert.deepEqual(sourceCoverageProblems(root, files, covered).sort(), [
      'TypeScript is outside all checked projects: loose.ts',
      'Maintained JavaScript must be TypeScript: maintained.js',
      'TypeScript is outside all checked projects: source/excluded.ts',
    ].sort());
    write('config/tsconfig.json', JSON.stringify({ extends: '../base.json', compilerOptions: { strict: false }, include: ['../source/**/*.ts'] }));
    assert.throws(() => checkedProjectSources(compiler, project), /must enable strict/);
    write('config/tsconfig.json', '{"extends":"./missing.json","compilerOptions":{"strict":true},"files":["../source/retained.ts"]}');
    assert.throws(() => checkedProjectSources(compiler, project), /Command failed/);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});
