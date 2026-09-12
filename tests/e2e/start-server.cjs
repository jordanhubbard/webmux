const fs = require('node:fs');
const path = require('node:path');

const testHome = path.resolve(__dirname, '.test-home');
const defaultsDir = path.resolve(__dirname, '../../webmux/config.defaults');

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
  if (entry.name === 'auth.yaml') content = content.replace('mode: local', 'mode: none');
  fs.writeFileSync(path.join(testHome, 'config', entry.name), content);
}
process.env.WEBMUX_HOME = testHome;
require('../../webmux/backend/dist/index.js');
