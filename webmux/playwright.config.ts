import { defineConfig } from '@playwright/test';
import path from 'path';

const TEST_PORT = 18080;
const TEST_HOME = path.resolve(__dirname, '../tests/e2e/.test-home');
const WEBMUX_DIR = path.resolve(__dirname);

export default defineConfig({
  testDir: '../tests/e2e',
  testMatch: '*.spec.ts',
  timeout: 30_000,
  retries: 0,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: `http://localhost:${TEST_PORT}`,
    headless: true,
  },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
  ],
  globalSetup: '../tests/e2e/global-setup.ts',
  globalTeardown: '../tests/e2e/global-teardown.ts',
  webServer: {
    command: `node backend/dist/index.js`,
    port: TEST_PORT,
    cwd: WEBMUX_DIR,
    reuseExistingServer: false,
    timeout: 15_000,
    env: {
      WEBMUX_ROOT: WEBMUX_DIR,
      WEBMUX_HOME: TEST_HOME,
      HTTP_PORT: String(TEST_PORT),
      NODE_ENV: 'test',
      NODE_OPTIONS: '--no-deprecation',
    },
  },
});
