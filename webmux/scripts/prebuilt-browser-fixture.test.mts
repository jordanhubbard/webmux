import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { prebuiltBrowserFixture } from './prebuilt-browser-fixture.mts';

test('ordinary browser tests retain their build path', () => {
  assert.equal(prebuiltBrowserFixture({}, process.platform), undefined);
});

test('prebuilt admission binds the exact hosted fixture image', t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'wm-prebuilt-browser-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const image = path.join(root, 'fixture.exe');
  fs.writeFileSync(image, 'private fixture image');
  const env = { GITHUB_ACTIONS: 'true', RUNNER_ENVIRONMENT: 'github-hosted', WEBMUX_E2E_PREBUILT_BINARY: image,
    WEBMUX_E2E_PREBUILT_SHA256: createHash('sha256').update(fs.readFileSync(image)).digest('hex') };
  assert.equal(prebuiltBrowserFixture(env, 'win32'), image);
  assert.throws(() => prebuiltBrowserFixture(env, 'darwin'), /disposable Windows/);
  assert.throws(() => prebuiltBrowserFixture({ ...env, GITHUB_ACTIONS: undefined }, 'win32'), /disposable Windows/);
  assert.throws(() => prebuiltBrowserFixture({ ...env, RUNNER_ENVIRONMENT: 'self-hosted' }, 'win32'), /disposable Windows/);
  assert.throws(() => prebuiltBrowserFixture({ ...env, WEBMUX_E2E_PREBUILT_SHA256: undefined }, 'win32'), /Incomplete/);
  assert.throws(() => prebuiltBrowserFixture({ ...env, WEBMUX_E2E_PREBUILT_BINARY: 'relative.exe' }, 'win32'), /Incomplete/);
  assert.throws(() => prebuiltBrowserFixture({ ...env, WEBMUX_E2E_PREBUILT_BINARY: root }, 'win32'), /Invalid/);
  fs.appendFileSync(image, 'changed');
  assert.throws(() => prebuiltBrowserFixture(env, 'win32'), /image changed/);
});
