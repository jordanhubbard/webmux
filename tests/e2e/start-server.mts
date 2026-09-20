import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { spawn, spawnSync } from 'node:child_process';

const require = createRequire(import.meta.url);

const testHome = path.resolve(import.meta.dirname, '.test-home');
const defaultsDir = path.resolve(import.meta.dirname, '../../webmux/config.defaults');
const authMode = process.env.WEBMUX_E2E_AUTH ?? 'none';
if (authMode !== 'none' && authMode !== 'local') throw new Error(`Unsupported browser-test auth mode: ${authMode}`);

// Initialize before requiring the server: Playwright's globalSetup runs AFTER its
// webServer starts. Never remove watched storage from a running server, including
// during teardown (especially on Windows). The next launch clears this fixture.
fs.rmSync(testHome, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
for (const directory of ['config', 'logs', 'data']) {
  fs.mkdirSync(path.join(testHome, directory), { recursive: true });
}
for (const entry of fs.readdirSync(defaultsDir, { withFileTypes: true })) {
  if (entry.isDirectory()) continue;
  let content = fs.readFileSync(path.join(defaultsDir, entry.name), 'utf8');
  if (entry.name === 'auth.yaml') content = content.replace('mode: local', `mode: ${authMode}`);
  fs.writeFileSync(path.join(testHome, 'config', entry.name), content);
}
process.env.WEBMUX_HOME = testHome;
const backend = process.env.WEBMUX_E2E_BACKEND ?? 'node';
if (backend === 'go') {
  const binary = path.join(testHome, process.platform === 'win32' ? 'webmux.exe' : 'webmux');
  const build = spawnSync('go', ['build', '-o', binary, './cmd/webmux'], {
    cwd: path.resolve(import.meta.dirname, '../../webmux/server'), stdio: 'inherit',
  });
  if (build.error) throw build.error;
  if (build.status !== 0) process.exit(build.status ?? 1);
  const child = spawn(binary, [], { stdio: 'inherit', env: process.env });
  for (const signal of ['SIGTERM', 'SIGINT'] as const) process.on(signal, () => child.kill(signal));
  child.on('error', error => { console.error(error); process.exitCode = 1; });
  child.on('exit', code => { process.exitCode = code ?? 0; });
} else if (backend === 'node') {
  require('../../webmux/backend/dist/index.js');
} else {
  throw new Error(`Unsupported browser-test backend: ${backend}`);
}
