import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { test } from 'node:test';

test('Make does not report successful service control after a failed manager command or rebuild', { skip: process.platform === 'win32' }, () => {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-make-failure-'));
  const marker = path.join(home, 'restarted');
  const run = (action: string, ...overrides: string[]) => spawnSync('make', [
    '--no-print-directory', '-o', 'build', '-o', 'check-guacd', action,
    `WEBMUX_HOME=${home}`, 'WEBMUX_BACKEND=go',
    // Every service-manager command is replaced. No installed service is used.
    'SVC_INSTALLED=true', 'SVC_MGR=fixture', 'SVC_STATUS=true',
    'SVC_START=true', 'SVC_RESTART=true', 'MAKE=true', ...overrides,
  ], { cwd: path.resolve(import.meta.dirname, '..'), encoding: 'utf8', timeout: 10000,
    env: { ...process.env, WEBMUX_RESTART_MARKER: marker } });
  try {
    for (const [action, override] of [['start', 'SVC_START=false'], ['restart', 'SVC_RESTART=false']] as const) {
      const failed = run(action, override);
      assert.ifError(failed.error);
      assert.notEqual(failed.status, 0, `${action} swallowed the manager error: ${failed.stdout}`);
      assert.doesNotMatch(failed.stdout, /webmux (?:started|restarted)/);
    }
    const rebuild = run('restart', 'MAKE=false', 'SVC_RESTART=printf restarted > "$$WEBMUX_RESTART_MARKER"');
    assert.ifError(rebuild.error);
    assert.notEqual(rebuild.status, 0, 'Restart swallowed the rebuild error');
    assert(!fs.existsSync(marker), 'A failed rebuild must leave the running service untouched');
    const started = run('start'); assert.ifError(started.error); assert.equal(started.status, 0, started.stderr);
    const restarted = run('restart'); assert.ifError(restarted.error); assert.equal(restarted.status, 0, restarted.stderr);
  } finally { fs.rmSync(home, { recursive: true, force: true }); }
});
