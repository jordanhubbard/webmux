import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { test } from 'node:test';
import { registerService } from './check-launchd.mts';
import { prepareLaunchdFixture } from './launchd-fixture.mts';

test('configure initializes a fresh home, preserves other settings, and refuses invalid updates', { skip: process.platform === 'win32' }, () => {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-configure space-'));
  const root = path.resolve(import.meta.dirname, '..');
  const run = (...args: string[]) => spawnSync('make', ['--no-print-directory', '-o', 'deps', 'configure', `WEBMUX_HOME=${home}`, ...args], {
    cwd: root, encoding: 'utf8', timeout: 10000,
  });
  try {
    const initial = run('HTTP_PORT=19271', 'HTTPS_PORT=0', 'AUTH_MODE=none', 'SECURE_MODE=false');
    assert.equal(initial.status, 0, initial.stdout + initial.stderr);
    const app = path.join(home, 'config/app.yaml');
    const auth = path.join(home, 'config/auth.yaml');
    assert.match(fs.readFileSync(app, 'utf8'), /http_port: 19271/);
    assert.match(fs.readFileSync(app, 'utf8'), /https_port: 0/);
    fs.appendFileSync(app, '\ncustom_setting: retained\n');
    fs.appendFileSync(auth, '\ncustom_auth: retained\n');
    const update = run('HTTP_PORT=19272', 'LISTEN_HOST=::1');
    assert.equal(update.status, 0, update.stdout + update.stderr);
    assert.match(fs.readFileSync(app, 'utf8'), /http_port: 19272/);
    assert.match(fs.readFileSync(app, 'utf8'), /custom_setting: retained/);
    assert.match(fs.readFileSync(auth, 'utf8'), /custom_auth: retained/);
    const before = [app, auth].map(file => fs.readFileSync(file, 'utf8'));
    for (const bad of ['HTTP_PORT=65536', 'HTTPS_PORT=-1', 'AUTH_MODE=unknown', 'SECURE_MODE=maybe']) {
      const refused = run(bad);
      assert.notEqual(refused.status, 0, `accepted ${bad}`);
      assert.deepEqual([app, auth].map(file => fs.readFileSync(file, 'utf8')), before);
    }
    fs.writeFileSync(auth, 'auth: [broken\n');
    assert.notEqual(run('HTTP_PORT=19273').status, 0);
    assert.equal(fs.readFileSync(app, 'utf8'), before[0], 'partial write before YAML validation');
  } finally { fs.rmSync(home, { recursive: true, force: true }); }
});

test('nested Makefile delegates every public target and quotes repository paths', { skip: process.platform === 'win32' }, () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-delegate space-'));
  const root = path.resolve(import.meta.dirname, '..');
  try {
    fs.mkdirSync(path.join(directory, 'webmux'));
    fs.copyFileSync(path.join(root, 'webmux/Makefile'), path.join(directory, 'webmux/Makefile'));
    const makefile = fs.readFileSync(path.join(root, 'Makefile'), 'utf8');
    const targets = [...makefile.matchAll(/^([a-z][a-z0-9-]*):/gm)].map(match => match[1]);
    fs.writeFileSync(path.join(directory, 'Makefile'), [...new Set(targets)].map(target => `.PHONY: ${target}\n${target}:\n\t@echo delegated-${target}\n`).join(''));
    for (const target of [...new Set(targets)]) {
      const result = spawnSync('make', ['--no-print-directory', '-C', path.join(directory, 'webmux'), target], { encoding: 'utf8', timeout: 10000 });
      assert.equal(result.status, 0, `${target}: ${result.stdout}${result.stderr}`);
      assert.match(result.stdout, new RegExp(`delegated-${target}\\b`));
    }
    const result = spawnSync('make', ['--no-print-directory', '-C', path.join(directory, 'webmux')], { encoding: 'utf8', timeout: 10000 });
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.match(result.stdout, /delegated-all/);
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});

test('Make reports absent managed jobs and bootstraps into the configured launchd domain', { skip: process.platform === 'win32' }, () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-make-domain-'));
  const root = path.resolve(import.meta.dirname, '..');
  const log = path.join(directory, 'commands');
  const env = { ...process.env, PATH: `${directory}:${process.env.PATH}`, WEBMUX_COMMAND_LOG: log };
  try {
    const status = spawnSync('make', ['--no-print-directory', 'status', 'SVC_INSTALLED=true', 'SVC_STATUS=false'], { cwd: root, env, encoding: 'utf8', timeout: 5000 });
    assert.equal(status.status, 0, status.stderr);
    assert.match(status.stdout, /not running/);
    fs.writeFileSync(path.join(directory, 'launchctl'), '#!/bin/sh\nprintf "%s\\n" "$*" >> "$WEBMUX_COMMAND_LOG"\ncase "$1" in kickstart) exit 1;; esac\n', { mode: 0o700 });
    const start = spawnSync('make', ['--no-print-directory', '-o', 'build-server', '-o', 'check-tmux', '-o', 'check-guacd',
      'start', 'OS=Darwin', 'SVC_INSTALLED=true', 'SVC_STATUS=true', 'LAUNCHD_SVC=user/501/private-fixture',
      `PLIST=${path.join(directory, 'fixture.plist')}`, `WEBMUX_HOME=${path.join(directory, 'home')}`], { cwd: root, env, encoding: 'utf8', timeout: 5000 });
    assert.equal(start.status, 0, start.stdout + start.stderr);
    assert.deepEqual(fs.readFileSync(log, 'utf8').trim().split('\n'), [
      'kickstart user/501/private-fixture', `bootstrap user/501 ${path.join(directory, 'fixture.plist')}`,
    ]);
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});

test('missing tmux blocks Unix server installation and startup before service changes', { skip: process.platform === 'win32' }, () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-tmux-check-'));
  const root = path.resolve(import.meta.dirname, '..');
  const marker = path.join(directory, 'service-changed');
  const make = spawnSync('/bin/sh', ['-c', 'command -v make'], { encoding: 'utf8' }).stdout.trim();
  try {
    // Isolate PATH so the test is independent of the host's tmux installation.
    for (const command of ['uname', 'id', 'sh']) {
      const executable = spawnSync('/bin/sh', ['-c', `command -v ${command}`], { encoding: 'utf8' }).stdout.trim();
      fs.symlinkSync(executable, path.join(directory, command));
    }
    const env = { ...process.env, PATH: directory };
    for (const action of ['install-server', 'start', 'restart', '_start_manual']) {
      const result = spawnSync(make, ['--no-print-directory', '-j4',
        '-o', 'build-server', '-o', 'install-optional-agent-deck', '-o', 'check-guacd', action,
        `AGENT_DECK_BINDIR=${directory}`, `WEBMUX_HOME=${path.join(directory, 'home')}`,
        'SVC_INSTALLED=true', `SVC_STOP=/usr/bin/touch ${marker}`,
        `SVC_START=/usr/bin/touch ${marker}`, `SVC_RESTART=/usr/bin/touch ${marker}`,
        `NODE=/usr/bin/touch ${marker}`, 'SHELL=/bin/sh'], {
        cwd: root, encoding: 'utf8', timeout: 10000, env,
      });
      assert.ifError(result.error);
      assert.notEqual(result.status, 0, `${action} accepted missing tmux`);
      assert.match(result.stderr, /tmux is required.*not found on PATH/);
      assert.match(result.stderr, /(?:brew install tmux|apt-get install -y tmux)/);
      assert(!fs.existsSync(marker), `${action} changed the service without tmux`);
      assert(!fs.existsSync(path.join(directory, 'home')), `${action} changed server state without tmux`);
    }
    const npmStart = spawnSync(process.execPath, [path.join(root, 'webmux/scripts/native.mts'), 'start'], {
      cwd: root, encoding: 'utf8', timeout: 10000, env,
    });
    assert.ifError(npmStart.error);
    assert.equal(npmStart.status, 1);
    assert.match(npmStart.stderr, /tmux is required/);
    assert.doesNotMatch(npmStart.stderr, /Unable to start native WebMux/);

    fs.writeFileSync(path.join(directory, 'tmux'), '#!/bin/sh\nexit 0\n', { mode: 0o700 });
    const installed = spawnSync(make, ['--no-print-directory', 'check-tmux', `AGENT_DECK_BINDIR=${directory}`], {
      cwd: root, encoding: 'utf8', timeout: 10000, env,
    });
    assert.ifError(installed.error);
    assert.equal(installed.status, 0, installed.stdout + installed.stderr);
    assert.match(installed.stdout, /tmux found:/);
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});

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
      const result = spawnSync('make', ['--no-print-directory', '-j4', '-o', 'check-tmux', '-o', 'install-optional-agent-deck', '-o', 'build-client',
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

test('parallel install-all finishes client installation before server replacement', { skip: process.platform === 'win32' }, () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-install-order-'));
  const makefile = path.join(directory, 'fixture.mk');
  const marker = path.join(directory, 'client-installed');
  try {
    fs.writeFileSync(makefile, `include ${path.resolve(import.meta.dirname, '../Makefile')}
install-client:
\t@sleep 0.2
\t@touch "${marker}"
install-server:
\t@test -f "${marker}"
`);
    const result = spawnSync('make', ['--no-print-directory', '-j4', '-f', makefile,
      '-o', 'build-client', '-o', 'build-server', '-o', 'install-optional-agent-deck',
      'install-all', `MAKE=make -f ${makefile} -o build-server -o install-optional-agent-deck -o check-tmux`], {
      cwd: path.resolve(import.meta.dirname, '..'), encoding: 'utf8', timeout: 10000,
    });
    assert.ifError(result.error);
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert(fs.existsSync(marker));
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});

test('launchd startup verification rejects exited jobs and accepts a stable running PID', { skip: process.platform === 'win32' }, t => {
  for (const running of [false, true]) {
    const f = prepareLaunchdFixture('state', { state: running ? 'state = running\npid = 123' : 'state = not running\nlast exit code = 0' });
    try {
      t.diagnostic(`Fake manager prepared in ${Math.round(f.preparationMs)} ms`);
      const result = spawnSync(process.execPath, f.args.slice(0, 3), { encoding: 'utf8', timeout: 15000, env: f.env });
      assert.ifError(result.error);
      assert.equal(result.status, running ? 0 : 1, result.stdout + result.stderr);
      if (!running) {
        assert.match(result.stderr, /did not remain running/);
        assert(result.stderr.includes(f.log));
      }
    } finally { f.cleanup(); }
  }
});

test('cold fixture preparation completes before the real startup query deadline', { skip: process.platform === 'win32' }, () => {
  const f = prepareLaunchdFixture('state', { state: 'state = running\npid = 123', preparationDelayMs: 1500 });
  try {
    assert(f.preparationMs >= 1500);
    // The same fake binary, state and delay fail the real checker if its first
    // invocation has not completed. This reproduces the old stage ordering.
    const coldDirectory = path.join(f.directory, 'unprepared');
    fs.mkdirSync(coldDirectory);
    const cold = spawnSync(process.execPath, f.args.slice(0, 3), {
      encoding: 'utf8', timeout: 10000, env: { ...f.env, WEBMUX_LAUNCHD_FIXTURE: coldDirectory },
    });
    assert.ifError(cold.error);
    assert.equal(cold.status, 1, cold.stdout + cold.stderr);
    assert.match(cold.stderr, /launchctl print.*ETIMEDOUT/);
    const result = spawnSync(process.execPath, f.args.slice(0, 3), { encoding: 'utf8', timeout: 10000, env: f.env });
    assert.ifError(result.error);
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert(fs.readFileSync(path.join(f.directory, 'commands'), 'utf8').trim().split('\n').every(command => command === 'print'));
  } finally { f.cleanup(); }
});

test('prepared fake managers still fail closed on query errors and timeouts', { skip: process.platform === 'win32' }, () => {
  for (const mode of ['query-error', 'query-timeout'] as const) {
    const f = prepareLaunchdFixture(mode);
    try {
      const result = spawnSync(process.execPath, f.args.slice(0, 3), { encoding: 'utf8', timeout: 10000, env: f.env });
      assert.ifError(result.error);
      assert.equal(result.status, 1, result.stdout + result.stderr);
      assert.match(result.stderr, mode === 'query-timeout' ? /launchctl print.*ETIMEDOUT/ : /Query failed: permission denied/);
    } finally { f.cleanup(); }
  }
});

test('timed manager queries have no Node interpreter startup dependency', { skip: process.platform === 'win32' }, () => {
  const f = prepareLaunchdFixture('state', { state: 'state = running\npid = 123' });
  try {
    const trap = path.join(f.directory, 'blocked-node'); fs.mkdirSync(trap);
    fs.writeFileSync(path.join(trap, 'node'), '#!/bin/sh\nexit 87\n', { mode: 0o700 });
    const parts = f.env.PATH!.split(path.delimiter); parts.splice(1, 0, trap);
    const env = { ...f.env, PATH: parts.join(path.delimiter) };
    assert.equal(spawnSync('node', ['--version'], { env }).status, 87);
    // The old Node fake deterministically fails the same real checker. Its
    // readiness has already completed; only per-query interpreter startup fails.
    const legacy = path.resolve(import.meta.dirname, '../tests/fixtures/launchd-bin');
    const negative = spawnSync(process.execPath, f.args.slice(0, 3), {
      env: { ...env, PATH: [legacy, trap, ...parts].join(path.delimiter) }, encoding: 'utf8', timeout: 10000,
    });
    assert.ifError(negative.error); assert.equal(negative.status, 1);
    assert.match(negative.stderr, /launchctl print/);
    const positive = spawnSync(process.execPath, f.args.slice(0, 3), { env, encoding: 'utf8', timeout: 10000 });
    assert.ifError(positive.error); assert.equal(positive.status, 0, positive.stdout + positive.stderr);
    assert(fs.readFileSync(path.join(f.directory, 'commands'), 'utf8').trim().split('\n').every(command => command === 'print'));
  } finally { f.cleanup(); }
});

test('Make does not report successful service control after a failed manager command or rebuild', { skip: process.platform === 'win32' }, () => {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-make-failure-'));
  const marker = path.join(home, 'restarted');
  const run = (action: string, ...overrides: string[]) => spawnSync('make', [
    '--no-print-directory', '-o', 'build', '-o', 'build-server', '-o', 'check-guacd', '-o', 'check-tmux', action,
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
      const result = spawnSync('make', ['--no-print-directory', '-o', 'build', '-o', 'build-server', '-o', 'check-guacd', '-o', 'check-tmux', action,
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
    assert.match(installed.stdout, /--server WEBMUX_HOST --background/);
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

test('Make keeps Agent Deck optional and installs its qualified release idempotently when requested', { skip: process.platform === 'win32' }, () => {
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
  const run = (target = 'install-agent-deck', ...extra: string[]) => spawnSync('make', ['--no-print-directory', target,
    `AGENT_DECK_BINDIR=${bin}`, 'AGENT_DECK_VERSION=v1.16.39', ...extra], {
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
    assert.match(spawnSync(executable, ['version'], { encoding: 'utf8' }).stdout, /v1\.16\.39/);
    assert(fs.existsSync(marker), 'old Agent Deck did not run the installer');

    fs.rmSync(marker);
    const skipped = run('install-optional-agent-deck');
    assert.ifError(skipped.error); assert.equal(skipped.status, 0, skipped.stdout + skipped.stderr);
    assert.match(skipped.stdout, /installation skipped/);
    assert(!fs.existsSync(marker), 'default installation ran the Agent Deck installer');
    const optedIn = run('install-optional-agent-deck', 'INSTALL_AGENT_DECK=yes');
    assert.ifError(optedIn.error); assert.equal(optedIn.status, 0, optedIn.stdout + optedIn.stderr);
    assert.match(optedIn.stdout, /Agent Deck 1\.16\.39 found/);

    const invalid = run('install-optional-agent-deck', 'INSTALL_AGENT_DECK=maybe');
    assert.ifError(invalid.error); assert.notEqual(invalid.status, 0);
    assert.match(invalid.stderr, /must be yes or no/);

    const service = spawnSync('make', ['--no-print-directory', '-n', 'install-server',
      '-o', 'install-optional-agent-deck', '-o', 'stop', '-o', 'build-server',
      'AGENT_DECK_BINDIR=/qualified/agent-deck'], { cwd: root, encoding: 'utf8', timeout: 10000 });
    assert.ifError(service.error); assert.equal(service.status, 0, service.stdout + service.stderr);
    assert.match(service.stdout, /PATH="\/qualified\/agent-deck:\$PATH" node scripts\/render-service\.mts/);
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});

test('manual Make startup preserves literal runtime data, zero ports and fresh logs', { skip: process.platform === 'win32' }, async () => {
  const root = path.resolve(import.meta.dirname, '..');
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-runtime values-'));
  const quote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
  try {
    for (const mode of ['command-line', 'environment', 'zero-ports', 'fresh-log']) {
      const base = path.join(directory, mode);
      fs.mkdirSync(base);
      const home = path.join(base, 'state & config');
      const receipt = path.join(base, 'runtime.json');
      const marker = path.join(base, 'must-not-exist');
      const worker = path.join(base, 'fixture worker.mjs');
      // Observe the Make-to-runtime boundary. The worker never opens sockets or
      // allocates a PTY; native server lifecycle is qualified separately.
      fs.writeFileSync(worker, `import fs from 'node:fs';
fs.writeFileSync(process.env.WEBMUX_ENV_RECEIPT, JSON.stringify({
  pid: process.pid, jwt: process.env.JWT_SECRET ?? '',
  http: process.env.HTTP_PORT, https: process.env.HTTPS_PORT,
}));
console.log('listening fixture');
setTimeout(() => process.exit(2), 15000);
setInterval(() => {}, 1000);
`);
      const secret = ['command-line', 'environment'].includes(mode)
        ? `qa secret $literal $(shell touch ${quote(marker)}) \`touch ${quote(marker)}\`; 'single' "double"` : '';
      const env = { ...process.env, JWT_SECRET: mode === 'environment' ? secret : '', WEBMUX_ENV_RECEIPT: receipt };
      const common = ['--no-print-directory', '-o', 'check-tmux', `WEBMUX_HOME=${home}`,
        `SERVER_COMMAND=${quote(process.execPath)} ${quote(worker)}`];
      try {
        const zero = mode === 'zero-ports';
        const started = spawnSync('make', [...common, '_start_manual',
          `HTTP_PORT=${zero ? '0' : '49101'}`, `HTTPS_PORT=${zero ? '0' : '49102'}`,
          ...(mode === 'command-line' ? [`JWT_SECRET=${secret}`] : [])], {
          cwd: root, env, encoding: 'utf8', timeout: 10000,
        });
        assert.ifError(started.error);
        assert.equal(started.status, 0, `${mode}: ${started.stdout}${started.stderr}`);
        // Starting a process and observing its ready message are distinct. Wait
        // only for this same worker's receipt; never replay the Make operation.
        const deadline = Date.now() + 5000;
        while (!fs.existsSync(receipt) && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 20));
        const actual = JSON.parse(fs.readFileSync(receipt, 'utf8')) as { pid: number; jwt: string; http: string; https: string };
        assert.equal(actual.jwt, secret, `${mode}: runtime secret changed`);
        assert.equal(Number(fs.readFileSync(path.join(home, '.webmux.pid'), 'utf8')), actual.pid);
        assert(!fs.existsSync(marker), `${mode}: runtime data was evaluated`);
        assert.doesNotMatch(started.stderr, /No such file or directory/, `${mode}: fresh log printed an error`);
        if (zero) {
          assert.equal(actual.http, '0');
          assert.equal(actual.https, '0');
        }
      } finally {
        if (fs.existsSync(receipt)) {
          const stopped = spawnSync('make', [...common, '_stop_manual'], { cwd: root, env, encoding: 'utf8', timeout: 10000 });
          assert.ifError(stopped.error);
          assert.equal(stopped.status, 0, `${mode}: ${stopped.stdout}${stopped.stderr}`);
          assert(!fs.existsSync(path.join(home, '.webmux.pid')), `${mode}: stop retained pidfile`);
        }
      }
    }
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});
