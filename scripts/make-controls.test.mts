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
    '--no-print-directory', '-o', 'build', '-o', 'build-server', '-o', 'check-guacd', action,
    `WEBMUX_HOME=${home}`,
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
      const result = spawnSync('make', ['--no-print-directory', '-o', 'build', '-o', 'build-server', '-o', 'check-guacd', action,
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

test('default Make install delivers only the client executable and does not require server tooling', { skip: process.platform === 'win32' }, () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-client-install-'));
  const bin = path.join(directory, 'bin');
  const executable = path.join(bin, 'webmux-client');
  const args = ['--no-print-directory', `CLIENT_BINDIR=${bin}`, `WEBMUX_HOME=${path.join(directory, 'home')}`, 'NPM=false', 'NODE=false'];
  try {
    const installed = spawnSync('make', ['install', ...args], {
      cwd: path.resolve(import.meta.dirname, '..'), encoding: 'utf8', timeout: 30000,
    });
    assert.ifError(installed.error);
    assert.equal(installed.status, 0, installed.stdout + installed.stderr);
    assert(fs.statSync(executable).isFile());
    assert(fs.statSync(executable).mode & 0o111, 'client executable is not runnable');
    assert.match(installed.stdout, /--server WEBMUX_URL --background/);
    assert.doesNotMatch(installed.stdout, /https:\/\/WEBMUX_HOST:8443/);
    assert(!fs.existsSync(path.join(directory, 'home')), 'client installation initialized server state');
    const removed = spawnSync('make', ['uninstall', ...args], {
      cwd: path.resolve(import.meta.dirname, '..'), encoding: 'utf8', timeout: 10000,
    });
    assert.ifError(removed.error);
    assert.equal(removed.status, 0, removed.stdout + removed.stderr);
    assert(!fs.existsSync(executable), 'client executable was not removed');
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});
