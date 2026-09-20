import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);

const webmuxDir = path.resolve(import.meta.dirname, '..');
const nodeModules = path.join(webmuxDir, 'node_modules');
const playwrightCli = require.resolve('@playwright/test/cli', { paths: [webmuxDir] });
const args = process.argv.slice(2);
const backendOption = args.find(arg => arg.startsWith('--backend='));
const backend = backendOption?.slice('--backend='.length) ?? process.env.WEBMUX_E2E_BACKEND ?? 'node';
if (backend !== 'node' && backend !== 'go') throw new Error(`Unsupported browser-test backend: ${backend}`);
const childEnv = {
  ...process.env,
  NODE_PATH: [nodeModules, process.env.NODE_PATH].filter(Boolean).join(path.delimiter),
  WEBMUX_E2E_BACKEND: backend,
};

const result = spawnSync(
  process.execPath,
  [playwrightCli, 'test', ...args.filter(arg => arg !== backendOption)],
  { cwd: webmuxDir, env: childEnv, stdio: 'inherit' },
);

if (result.error) throw result.error;
process.exitCode = result.status ?? 1;
