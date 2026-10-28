import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { test } from 'node:test';
import { registerService } from './check-launchd.mts';

test('launchd registration retries asynchronous teardown and propagates persistent failures', async () => {
  let attempts = 0;
  await registerService('gui/501', '/fixture.plist', () => {
    if (++attempts < 3) throw new Error('Bootstrap failed: 5: Input/output error');
  }, async () => {}, 1000);
  assert.equal(attempts, 3);
  await assert.rejects(registerService('gui/501', '/fixture.plist', () => {
    throw new Error('permission denied');
  }, async () => {}, 0), /permission denied/);
});

test('failed server builds preserve the installed service, including parallel install-all', { skip: process.platform === 'win32' }, () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-install-build-failure-'));
  const marker = path.join(directory, 'stopped');
  try {
    for (const action of ['install-server', 'install-all']) {
      const result = spawnSync('make', ['--no-print-directory', '-j4', '-o', 'install-agent-deck', '-o', 'build-client',
        action, 'NPM=false', `CLIENT_BINDIR=${path.join(directory, 'bin')}`,
        'SVC_INSTALLED=true', 'SVC_STOP=touch "$$WEBMUX_STOP_MARKER"'], {
        cwd: path.resolve(import.meta.dirname, '..'), encoding: 'utf8', timeout: 10000,
        env: { ...process.env, WEBMUX_STOP_MARKER: marker },
      });
      assert.ifError(result.error);
      assert.notEqual(result.status, 0, 'Build failure was swallowed');
      assert(!fs.existsSync(marker), `${action} stopped the installed server before the build succeeded`);
    }
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});

test('launchd startup verification rejects exited jobs and accepts a stable running PID', { skip: process.platform === 'win32' }, () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-launchd-startup-'));
  try {
    fs.writeFileSync(path.join(directory, 'launchctl'), '#!/bin/sh\nprintf "%s\\n" "$WEBMUX_LAUNCHD_STATE"\n', { mode: 0o700 });
    for (const running of [false, true]) {
      const result = spawnSync(process.execPath, [path.join(import.meta.dirname, 'check-launchd.mts'), 'gui/501/fixture', '/fixture/server.log'], {
        encoding: 'utf8', timeout: 15000,
        env: { ...process.env, PATH: `${directory}:${process.env.PATH ?? '/usr/bin:/bin'}`,
          WEBMUX_LAUNCHD_STATE: running ? 'state = running\npid = 123' : 'state = not running\nlast exit code = 0' },
      });
      assert.ifError(result.error);
      assert.equal(result.status, running ? 0 : 1, result.stdout + result.stderr);
      if (!running) assert.match(result.stderr, /did not remain running.*\/fixture\/server.log/);
    }
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});

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

test('Make installs the qualified Agent Deck release idempotently and supports an explicit opt-out', { skip: process.platform === 'win32' }, () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-agent-deck-install-'));
  const bin = path.join(directory, 'bin');
  const executable = path.join(bin, 'agent-deck');
  const installer = path.join(directory, 'installer.sh');
  const marker = path.join(directory, 'installer-ran');
  const root = path.resolve(import.meta.dirname, '..');
  fs.mkdirSync(bin);
  const writeVersion = (version: string) => fs.writeFileSync(executable, `#!/bin/sh
printf 'Agent Deck v${version}\\n'
`, { mode: 0o700 });
  fs.writeFileSync(installer, `#!/bin/bash
set -eu
version=
bindir=
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) version="\${2#v}"; shift 2 ;;
    --dir) bindir="$2"; shift 2 ;;
    --non-interactive) shift ;;
    *) exit 64 ;;
  esac
done
printf ran > "$WEBMUX_AGENT_DECK_MARKER"
mkdir -p "$bindir"
printf '#!/bin/sh\\nprintf "Agent Deck v%s\\\\n"\\n' "$version" > "$bindir/agent-deck"
chmod +x "$bindir/agent-deck"
`, { mode: 0o700 });
  const run = (...extra: string[]) => spawnSync('make', ['--no-print-directory', 'install-agent-deck',
    `AGENT_DECK_BINDIR=${bin}`, 'AGENT_DECK_VERSION=v1.16.34', ...extra], {
    cwd: root, encoding: 'utf8', timeout: 30000,
    env: { ...process.env, PATH: `${bin}:${process.env.PATH ?? '/usr/bin:/bin'}`,
      AGENT_DECK_INSTALLER_URL: `file://${installer}`, WEBMUX_AGENT_DECK_MARKER: marker },
  });
  try {
    writeVersion('1.17.0');
    const current = run();
    assert.ifError(current.error); assert.equal(current.status, 0, current.stdout + current.stderr);
    assert.match(current.stdout, /Agent Deck 1\.17\.0 found/);
    assert(!fs.existsSync(marker), 'current Agent Deck unexpectedly ran the installer');

    writeVersion('1.15.0');
    const upgraded = run();
    assert.ifError(upgraded.error); assert.equal(upgraded.status, 0, upgraded.stdout + upgraded.stderr);
    assert.match(upgraded.stdout, /older than WebMux requires/);
    assert.match(spawnSync(executable, ['version'], { encoding: 'utf8' }).stdout, /v1\.16\.34/);
    assert(fs.existsSync(marker), 'old Agent Deck did not run the installer');

    fs.rmSync(marker);
    const skipped = run('INSTALL_AGENT_DECK=no');
    assert.ifError(skipped.error); assert.equal(skipped.status, 0, skipped.stdout + skipped.stderr);
    assert.match(skipped.stdout, /installation skipped/);
    assert(!fs.existsSync(marker), 'opt-out ran the Agent Deck installer');

    const invalid = run('INSTALL_AGENT_DECK=maybe');
    assert.ifError(invalid.error); assert.notEqual(invalid.status, 0);
    assert.match(invalid.stderr, /must be yes or no/);

    const service = spawnSync('make', ['--no-print-directory', '-n', 'install-server',
      '-o', 'install-agent-deck', '-o', 'stop', '-o', 'build-server',
      'AGENT_DECK_BINDIR=/qualified/agent-deck'], { cwd: root, encoding: 'utf8', timeout: 10000 });
    assert.ifError(service.error); assert.equal(service.status, 0, service.stdout + service.stderr);
    assert.match(service.stdout, /PATH="\/qualified\/agent-deck:\$PATH" node scripts\/render-service\.mts/);
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});
