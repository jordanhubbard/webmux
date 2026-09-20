import { defineConfig } from '@playwright/test';
import path from 'path';

const TEST_PORT = 18080;
const TEST_HOME = path.resolve(__dirname, '../tests/e2e/.test-home');
const WEBMUX_DIR = path.resolve(__dirname);
const CHROMIUM_EXECUTABLE = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH;
const VISUAL_PARITY = process.env.WEBMUX_VISUAL_PARITY === '1';

export default defineConfig({
  testDir: '../tests/e2e',
  testMatch: '*.spec.ts',
  timeout: 30_000,
  retries: 0,
  workers: 1,
  reporter: [['list']],
  snapshotPathTemplate: '{testDir}/.visual-baseline/{platform}/{arg}{ext}',
  use: {
    baseURL: `http://localhost:${TEST_PORT}`,
    headless: true,
  },
  projects: [
    {
      name: 'chromium',
      use: {
        browserName: 'chromium',
        launchOptions: {
          ...(CHROMIUM_EXECUTABLE ? { executablePath: CHROMIUM_EXECUTABLE } : {}),
          // Partial raster reuse in Chromium 145 can change rounded-border
          // channels between identical runs. Render whole tiles for this exact
          // comparison; ordinary browser workflow tests keep default rendering.
          ...(VISUAL_PARITY ? { args: ['--disable-gpu', '--force-color-profile=srgb', '--disable-partial-raster'] } : {}),
        },
      },
    },
  ],
  webServer: {
    command: 'node ../tests/e2e/start-server.mts',
    port: TEST_PORT,
    cwd: WEBMUX_DIR,
    reuseExistingServer: false,
    timeout: 120_000,
    env: {
      WEBMUX_ROOT: WEBMUX_DIR,
      WEBMUX_HOME: TEST_HOME,
      HTTP_PORT: String(TEST_PORT),
      NODE_ENV: 'test',
      NODE_OPTIONS: '--no-deprecation',
    },
  },
});
