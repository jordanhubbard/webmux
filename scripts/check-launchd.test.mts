import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import { once } from 'node:events';
import { setTimeout } from 'node:timers/promises';
import { test } from 'node:test';

import { prepareLaunchdFixture as fixture } from './launchd-fixture.mts';

test('installer waits for removal, retries bootstrap errors, and recovers lost registration', { skip: process.platform === 'win32' }, () => {
  const f = fixture('race');
  try {
    const result = spawnSync(process.execPath, f.args, { env: f.env, encoding: 'utf8', timeout: 20000 });
    assert.ifError(result.error);
    assert.equal(result.status, 0, result.stdout + result.stderr + (fs.existsSync(f.log) ? fs.readFileSync(f.log, 'utf8') : ''));
    assert.equal(f.state().bootouts, 1);
    assert.equal(f.state().bootstraps, 3);
    assert.equal(f.state().registered, true);
    assert.match(fs.readFileSync(f.log, 'utf8'), /installation completed/);
  } finally { f.cleanup(); }
});

test('installer accepts a fresh installation but preserves a service it cannot stop', { skip: process.platform === 'win32' }, () => {
  for (const mode of ['fresh', 'stop-denied'] as const) {
    const f = fixture(mode);
    try {
      const result = spawnSync(process.execPath, f.args, { env: f.env, encoding: 'utf8', timeout: 10000 });
      assert.ifError(result.error);
      assert.equal(result.status, mode === 'fresh' ? 0 : 1, result.stdout + result.stderr);
      assert.equal(f.state().bootstraps, mode === 'fresh' ? 1 : 0);
      if (mode === 'stop-denied') assert.match(fs.readFileSync(f.log, 'utf8'), /Stop failed: permission denied/);
    } finally { f.cleanup(); }
  }
});

test('installer survives loss of its invoking terminal process group', { skip: process.platform === 'win32' }, async () => {
  const f = fixture('interrupt');
  const child = spawn(process.execPath, f.args, { env: f.env, detached: true, stdio: 'ignore' });
  fs.writeFileSync(path.join(f.directory, 'parent.pid'), String(child.pid));
  try {
    const [, signal] = await once(child, 'exit', { signal: AbortSignal.timeout(10000) });
    assert.equal(signal, 'SIGHUP');
    const deadline = Date.now() + 15000;
    while (!fs.existsSync(f.log) || !fs.readFileSync(f.log, 'utf8').includes('installation completed')) {
      assert(Date.now() < deadline, 'Detached worker did not finish after terminal loss');
      await setTimeout(100);
    }
    assert.equal(f.state().registered, true);
    assert.equal(f.state().bootouts, 1);
    assert.equal(f.state().bootstraps, 1);
  } finally { f.cleanup(); }
});

test('native launchd replacement survives terminal loss during slow shutdown', { skip: process.platform !== 'darwin', timeout: 25000 }, async t => {
  const domain = `gui/${process.getuid!()}`;
  const launch = (...args: string[]) => spawnSync('/bin/launchctl', args, { encoding: 'utf8', timeout: 5000 });
  if (launch('print', domain).status !== 0) { t.skip('No GUI launchd domain'); return; }
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-launchd-native-'));
  const service = `${domain}/com.webmux.install-test.${process.pid}`;
  const ready = path.join(directory, 'ready');
  const stopping = path.join(directory, 'stopping');
  const plist = path.join(directory, 'fixture.plist');
  const log = path.join(directory, 'install.log');
  const fixtureScript = path.join(directory, 'service.cjs');
  fs.writeFileSync(fixtureScript, `const fs = require('node:fs');
process.on('SIGTERM', () => {
  fs.writeFileSync(${JSON.stringify(stopping)}, 'stopping');
  setTimeout(() => process.exit(0), 1000);
});
fs.writeFileSync(${JSON.stringify(ready)}, String(process.pid));
setInterval(() => {}, 1000);
`);
  const definition = { Label: service.slice(service.lastIndexOf('/') + 1),
    ProgramArguments: [process.execPath, fixtureScript], RunAtLoad: true };
  const converted = spawnSync('/usr/bin/plutil', ['-convert', 'xml1', '-o', plist, '--', '-'], {
    input: JSON.stringify(definition), encoding: 'utf8', timeout: 5000,
  });
  assert.equal(converted.status, 0, converted.stderr);
  const waitFor = async (check: () => boolean) => {
    const deadline = Date.now() + 10000;
    while (!check()) {
      assert(Date.now() < deadline, 'Timed out waiting for native service transition');
      await setTimeout(50);
    }
  };
  let installer: ReturnType<typeof spawn> | undefined;
  try {
    const initial = launch('bootstrap', domain, plist);
    assert.equal(initial.status, 0, initial.stderr);
    await waitFor(() => fs.existsSync(ready));
    const previous = fs.readFileSync(ready, 'utf8');
    installer = spawn(process.execPath, [path.join(import.meta.dirname, 'check-launchd.mts'), service, log, plist], {
      detached: true, stdio: 'ignore',
    });
    const exited = once(installer, 'exit', { signal: AbortSignal.timeout(15000) });
    await waitFor(() => fs.existsSync(stopping));
    process.kill(-installer.pid!, 'SIGHUP');
    const [, signal] = await exited;
    assert.equal(signal, 'SIGHUP');
    await waitFor(() => fs.existsSync(log) && fs.readFileSync(log, 'utf8').includes('installation completed'));
    assert.notEqual(fs.readFileSync(ready, 'utf8'), previous);
    const current = launch('print', service);
    assert.equal(current.status, 0, current.stderr);
    assert.match(current.stdout, /state = running/);
  } finally {
    if (installer?.exitCode === null && installer.signalCode === null) installer.kill();
    launch('bootout', service);
    await waitFor(() => launch('print', service).status !== 0);
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

test('installer reports persistent bootstrap errors and does not restart a crashing registered job', { skip: process.platform === 'win32' }, () => {
  for (const mode of ['denied', 'crash'] as const) {
    const f = fixture(mode);
    try {
      const result = spawnSync(process.execPath, f.args, { env: f.env, encoding: 'utf8', timeout: 20000 });
      assert.ifError(result.error);
      assert.equal(result.status, 1, result.stdout + result.stderr);
      assert.match(result.stderr, /installation failed/);
      assert.match(fs.readFileSync(f.log, 'utf8'), mode === 'denied' ? /permission denied/ : /did not remain running/);
      if (mode === 'crash') assert.equal(f.state().bootstraps, 1);
    } finally { f.cleanup(); }
  }
});
