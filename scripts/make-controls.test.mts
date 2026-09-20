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

test('Make user-service controls target the configured unit instead of the default service', { skip: process.platform === 'win32' }, () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-make-unit-'));
  const unit = path.join(directory, 'private-fixture.service');
  const log = path.join(directory, 'commands');
  try {
    fs.writeFileSync(unit, 'fixture');
    fs.writeFileSync(path.join(directory, 'systemctl'), '#!/bin/sh\nprintf "%s " "$@" >> "$WEBMUX_COMMAND_LOG"\nprintf "\\n" >> "$WEBMUX_COMMAND_LOG"\n', { mode: 0o700 });
    for (const action of ['start', 'stop', 'restart']) {
      const result = spawnSync('make', ['--no-print-directory', '-o', 'build', '-o', 'check-guacd', action,
        'OS=Linux', `UNIT=${unit}`, `WEBMUX_HOME=${directory}`, 'MAKE=true', 'SVC_STATUS=true'], {
        cwd: path.resolve(import.meta.dirname, '..'), encoding: 'utf8', timeout: 10000,
        env: { ...process.env, PATH: `${directory}:${process.env.PATH ?? '/usr/bin:/bin'}`, WEBMUX_COMMAND_LOG: log },
      });
      assert.ifError(result.error); assert.equal(result.status, 0, result.stdout + result.stderr);
    }
    assert.deepEqual(fs.readFileSync(log, 'utf8').trim().split('\n').map(line => line.trim()), [
      '--user start private-fixture.service', '--user stop private-fixture.service', '--user restart private-fixture.service',
    ]);
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});
