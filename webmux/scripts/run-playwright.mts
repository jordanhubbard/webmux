import path from 'node:path';
import { createRequire } from 'node:module';
import { spawnSync } from 'node:child_process';
import { runWithPrivateTmux } from './private-test-tmux.mts';
import { isolatedPlaywrightRuns } from './playwright-runs.mts';

const require = createRequire(import.meta.url);

const webmuxDir = path.resolve(import.meta.dirname, '..');
const nodeModules = path.join(webmuxDir, 'node_modules');
const playwrightCli = require.resolve('@playwright/test/cli', { paths: [webmuxDir] });
const args = process.argv.slice(2);
const childEnv = {
  ...process.env,
  NODE_PATH: [nodeModules, process.env.NODE_PATH].filter(Boolean).join(path.delimiter),
};

// The suite shares one client address. Give each discovered file a fresh server
// so unrelated browser workflows cannot exhaust its production request budget.
// Focused invocations retain their exact CLI arguments and output directory.
let runs = [args];
if (!args.length) {
  const discovery = spawnSync(process.execPath, [playwrightCli, 'test', '--list', '--reporter=json'], {
    cwd: webmuxDir, env: childEnv, encoding: 'utf8', timeout: 60_000, maxBuffer: 8 << 20,
  });
  if (discovery.error) throw discovery.error;
  if (discovery.status !== 0) throw new Error(`Playwright discovery failed (${discovery.status}): ${discovery.stderr.slice(-4000)}`);
  runs = isolatedPlaywrightRuns(JSON.parse(discovery.stdout));
}
let exitCode = 0;
for (const runArgs of runs) {
  const status = await runWithPrivateTmux(
    process.execPath,
    [playwrightCli, 'test', ...runArgs],
    { cwd: webmuxDir, env: childEnv },
  );
  if (status !== 0) exitCode = status;
  if (status === 130 || status === 143) break;
}
process.exitCode = exitCode;
