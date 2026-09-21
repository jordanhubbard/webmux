import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { test } from 'node:test';

const helper = path.join(import.meta.dirname, 'release-metadata.mts');
test('release version updates preserve custom/dependency metadata and reject incomplete inputs before writes', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-release-metadata-'));
  const rootPackage = path.join(root, 'webmux/package.json');
  const frontend = path.join(root, 'webmux/frontend/package.json');
  const lock = path.join(root, 'webmux/package-lock.json');
  const run = (...args: string[]) => spawnSync(process.execPath, [helper, ...args], { cwd: root, encoding: 'utf8' });
  const original = { version: '1.2.3', custom: { version: 'preserve-me' } };
  try {
    for (const file of [rootPackage, frontend]) {
      fs.mkdirSync(path.dirname(file), { recursive: true }); fs.writeFileSync(file, JSON.stringify(original));
    }
    fs.writeFileSync(lock, JSON.stringify({ version: '1.2.3', packages: {
      '': original, frontend: original,
      'node_modules/dependency': { version: '9.8.7', integrity: 'fixture' },
    } }));
    assert.equal(run('validate').stdout.trim(), '1.2.3');
    const changed = run('bump', '2.0.0'); assert.equal(changed.status, 0, changed.stderr);
    assert.equal(run('read', rootPackage).stdout.trim(), '2.0.0');
    assert.equal(run('validate').stdout.trim(), '2.0.0');
    const saved = JSON.parse(fs.readFileSync(lock, 'utf8')) as { version: string; packages: Record<string, { version: string; custom?: { version: string } }> };
    assert.equal(saved.version, '2.0.0'); assert.equal(saved.packages[''].version, '2.0.0');
    assert.equal(saved.packages['node_modules/dependency'].version, '9.8.7');
    assert.equal(saved.packages[''].custom?.version, 'preserve-me');
    const before = fs.readFileSync(rootPackage, 'utf8');
    assert.notEqual(run('bump', 'not-a-version').status, 0);
    assert.equal(fs.readFileSync(rootPackage, 'utf8'), before);
    saved.packages.frontend.version = '0.0.1';
    fs.writeFileSync(lock, JSON.stringify(saved));
    assert.notEqual(run('validate').status, 0);
    delete saved.packages.frontend;
    fs.writeFileSync(lock, JSON.stringify(saved));
    assert.notEqual(run('bump', '3.0.0').status, 0);
    assert.equal(fs.readFileSync(rootPackage, 'utf8'), before);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});
