import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);

const webmuxDir = path.resolve(import.meta.dirname, '..');
const nodeModules = path.join(webmuxDir, 'node_modules');
const playwrightCli = require.resolve('@playwright/test/cli', { paths: [webmuxDir] });
const childEnv = {
  ...process.env,
  NODE_PATH: [nodeModules, process.env.NODE_PATH].filter(Boolean).join(path.delimiter),
};

const result = spawnSync(
  process.execPath,
  [playwrightCli, 'test', ...process.argv.slice(2)],
  { cwd: webmuxDir, env: childEnv, stdio: 'inherit' },
);

if (result.error) throw result.error;
process.exitCode = result.status ?? 1;
