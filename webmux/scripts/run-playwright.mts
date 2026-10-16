import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);

const webmuxDir = path.resolve(import.meta.dirname, '..');
const nodeModules = path.join(webmuxDir, 'node_modules');
const playwrightCli = require.resolve('@playwright/test/cli', { paths: [webmuxDir] });
const args = process.argv.slice(2);
const childEnv = {
  ...process.env,
  NODE_PATH: [nodeModules, process.env.NODE_PATH].filter(Boolean).join(path.delimiter),
};

// The full suite shares one client address. Keep each half on a fresh server
// so accumulated browser requests do not exhaust the production rate limit.
// Focused invocations retain their exact CLI arguments and output directory.
const runs = args.length ? [args] : [1, 2].map(shard => [
  `--shard=${shard}/2`, `--output=test-results/shard-${shard}`,
]);
let exitCode = 0;
for (const runArgs of runs) {
  const result = spawnSync(
    process.execPath,
    [playwrightCli, 'test', ...runArgs],
    { cwd: webmuxDir, env: childEnv, stdio: 'inherit' },
  );
  if (result.error) throw result.error;
  if (result.status !== 0) exitCode = result.status ?? 1;
}
process.exitCode = exitCode;
