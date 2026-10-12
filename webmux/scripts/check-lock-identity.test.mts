import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

test('lock guard rejects release-version corruption and unexpected package identity', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-lock-test-'));
  try {
    fs.mkdirSync(path.join(root, 'node_modules/fixture'), { recursive: true });
    const manifest = path.join(root, 'node_modules/fixture/package.json');
    const lock = path.join(root, 'package-lock.json');
    fs.writeFileSync(manifest, JSON.stringify({ name: 'fixture', version: '9.0.0' }));
    const check = () => spawnSync(process.execPath, [path.join(import.meta.dirname, 'check-lock-identity.mts'), root], { encoding: 'utf8' });
    fs.writeFileSync(lock, JSON.stringify({ packages: { 'node_modules/fixture': { version: '1.1.2' } } }));
    assert.notEqual(check().status, 0);
    fs.writeFileSync(lock, JSON.stringify({ packages: { 'node_modules/fixture': { version: '9.0.0' }, 'node_modules/platform-only': { version: '1.0.0', optional: true } } }));
    assert.equal(check().status, 0);
    fs.writeFileSync(manifest, JSON.stringify({ name: 'wrong-package', version: '9.0.0' }));
    assert.notEqual(check().status, 0);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});
