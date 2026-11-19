import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';

type FixtureMode = 'race' | 'fresh' | 'stop-denied' | 'interrupt' | 'denied' | 'crash' | 'state' | 'query-error' | 'query-timeout';

let nativeBin: string | undefined;
function managerBin() {
  if (nativeBin) return nativeBin;
  const bin = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-launchd-manager-'));
  try {
    // Match the default Make client build's path profile so this private fake
    // does not populate an incompatible standard-library cache first. Its
    // readiness receipt binds the actual image hash; no binary is distributed.
    const compiled = spawnSync('go', ['build', '-o', path.join(bin, 'launchctl'), path.resolve(import.meta.dirname, '../tests/fixtures/launchd-native/main.go')], {
      env: { ...process.env, CGO_ENABLED: '0' }, encoding: 'utf8', timeout: 60_000,
    });
    assert.ifError(compiled.error);
    assert.equal(compiled.status, 0, `Cannot prepare native launchd fixture: ${compiled.stderr}`);
  } catch (error) { fs.rmSync(bin, { recursive: true, force: true }); throw error; }
  process.once('exit', () => fs.rmSync(bin, { recursive: true, force: true }));
  nativeBin = bin;
  return bin;
}

// Compile and prepare before entering the unchanged production deadlines.
// Timed queries execute a native fake manager with no Node startup dependency.
// It never calls launchctl; cases retain separate state, commands and logs.
export function prepareLaunchdFixture(mode: FixtureMode, options: { state?: string; preparationDelayMs?: number } = {}) {
  const bin = managerBin();
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-launchd-install-'));
  const log = path.join(directory, 'install.log');
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    PATH: [bin, path.dirname(process.execPath), process.env.PATH || '/usr/bin:/bin'].join(path.delimiter),
    WEBMUX_LAUNCHD_FIXTURE: directory, WEBMUX_LAUNCHD_MODE: mode,
    WEBMUX_LAUNCHD_STATE: options.state,
    WEBMUX_LAUNCHD_PREPARATION_DELAY_MS: String(options.preparationDelayMs || 0),
  };
  const cleanup = () => fs.rmSync(directory, { recursive: true, force: true });
  const started = performance.now();
  try {
    const prepared = spawnSync(path.join(bin, 'launchctl'), ['fixture-ready'], { env, encoding: 'utf8', timeout: 10000 });
    assert.ifError(prepared.error);
    assert.equal(prepared.status, 0, `Launchd fixture preparation failed: ${prepared.stdout}${prepared.stderr}`);
    const ready = JSON.parse(fs.readFileSync(path.join(directory, 'ready.json'), 'utf8'));
    assert.equal(ready.kind, 'webmux-native-launchd-fixture/v1');
    assert.equal(ready.imageSHA256, createHash('sha256').update(fs.readFileSync(path.join(bin, 'launchctl'))).digest('hex'));
    // Preparation must not simulate any service-manager command or operation.
    assert(!fs.existsSync(path.join(directory, 'commands')));
    assert(!fs.existsSync(path.join(directory, 'state.json')));
  } catch (error) { cleanup(); throw error; }
  const args = [path.join(import.meta.dirname, 'check-launchd.mts'), 'gui/501/fixture', log, path.join(directory, 'fixture.plist')];
  return { directory, log, env, args, preparationMs: performance.now() - started,
    state: () => JSON.parse(fs.readFileSync(path.join(directory, 'state.json'), 'utf8')) as { bootouts: number; bootstraps: number; registered: boolean },
    cleanup };
}
