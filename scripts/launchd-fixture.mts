import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

type FixtureMode = 'race' | 'fresh' | 'stop-denied' | 'interrupt' | 'denied' | 'crash' | 'state' | 'query-error' | 'query-timeout';

// Prepare the fake manager before entering the production command deadlines.
// A new generated executable per case makes cold fixture initialization part
// of the one-second state query. The shared executable never calls launchctl;
// each case still has its own state, environment, commands and log directory.
export function prepareLaunchdFixture(mode: FixtureMode, options: { state?: string; preparationDelayMs?: number } = {}) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-launchd-install-'));
  const log = path.join(directory, 'install.log');
  const bin = path.resolve(import.meta.dirname, '../tests/fixtures/launchd-bin');
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
    assert.equal(JSON.parse(fs.readFileSync(path.join(directory, 'ready.json'), 'utf8')).version, process.version);
    // Preparation must not simulate any service-manager command or operation.
    assert(!fs.existsSync(path.join(directory, 'commands')));
    assert(!fs.existsSync(path.join(directory, 'state.json')));
  } catch (error) { cleanup(); throw error; }
  const args = [path.join(import.meta.dirname, 'check-launchd.mts'), 'gui/501/fixture', log, path.join(directory, 'fixture.plist')];
  return { directory, log, env, args, preparationMs: performance.now() - started,
    state: () => JSON.parse(fs.readFileSync(path.join(directory, 'state.json'), 'utf8')) as { bootouts: number; bootstraps: number; registered: boolean },
    cleanup };
}
