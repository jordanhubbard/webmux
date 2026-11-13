import path from 'node:path';
import { createRequire } from 'node:module';
import { runWithPrivateTmux } from './private-test-tmux.mts';

const require = createRequire(import.meta.url);

const webmuxDir = path.resolve(import.meta.dirname, '..');
const nodeModules = path.join(webmuxDir, 'node_modules');
const playwrightCli = require.resolve('@playwright/test/cli', { paths: [webmuxDir] });
const args = process.argv.slice(2);
const childEnv = {
  ...process.env,
  NODE_PATH: [nodeModules, process.env.NODE_PATH].filter(Boolean).join(path.delimiter),
};

// The full suite shares one client address. Keep each shard on a fresh server
// so accumulated browser requests do not exhaust the production rate limit.
// Five groups include headroom for the two-viewer mode-query control; with four,
// its added page loads caused HTTP 429s in the following mouse-selection test.
// Focused invocations retain their exact CLI arguments and output directory.
const shards = 5;
const runs = args.length ? [args] : Array.from({ length: shards }, (_, index) => [
  `--shard=${index + 1}/${shards}`, `--output=test-results/shard-${index + 1}`,
]);
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
