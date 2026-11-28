import test from 'node:test';
import assert from 'node:assert/strict';
import type { Browser, BrowserContext, BrowserType } from 'playwright-core';
import { selectBrowserCompanion, preflightBrowserCompanion } from './browser-companion-preflight.mts';

const managed = () => '/managed/chrome';
const installed = '/Applications/Chromium.app/Contents/MacOS/Chromium';

test('companion selection preserves explicit, native macOS, driver, managed precedence', () => {
  const env = { WEBMUX_BROWSER_EXECUTABLE: '/explicit/chrome', PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH: '/driver/chrome' };
  for (const platform of ['linux', 'darwin', 'win32'] as const) {
    assert.equal(selectBrowserCompanion(env, platform, managed, () => true).executablePath, '/explicit/chrome');
  }
  assert.equal(selectBrowserCompanion({ ...env, WEBMUX_BROWSER_EXECUTABLE: '' }, 'darwin', managed, () => true).executablePath, installed);
  assert.equal(selectBrowserCompanion({ ...env, WEBMUX_BROWSER_EXECUTABLE: '' }, 'darwin', managed, () => false).executablePath, '/driver/chrome');
  assert.equal(selectBrowserCompanion({ ...env, WEBMUX_BROWSER_EXECUTABLE: '' }, 'linux', managed, () => true).executablePath, '/driver/chrome');
  assert.equal(selectBrowserCompanion({}, 'linux', managed).executablePath, '/managed/chrome');
  assert.equal(selectBrowserCompanion({}, 'win32', managed).executablePath, '/managed/chrome');
});

function fixture({ badTitle = false, badVersion = false } = {}) {
  let contexts = 0, closedContexts = 0, browserCloses = 0;
  const launches: unknown[] = [];
  const browser = {
    version() { if (badVersion) throw new Error('version failed'); return 'fixture-version'; },
    isConnected: () => true,
    async newContext() {
      contexts++;
      return {
        async newPage() { return {
          async goto(url: string, options: { timeout: number }) {
            assert.match(url, /^data:text\/html,/);
            assert.equal(options.timeout, 5000);
          },
          async title() { return badTitle ? 'wrong title' : 'WebMux browser readiness'; },
        }; },
        async close() { closedContexts++; },
      } as unknown as BrowserContext;
    },
    async close() { browserCloses++; },
  } as unknown as Browser;
  const launcher: Pick<BrowserType, 'launch'> = { async launch(options) { launches.push(options); return browser; } };
  return { launcher, launches, counts: () => ({ contexts, closedContexts, browserCloses }) };
}

const selection = { executablePath: '/explicit/chrome', source: 'WEBMUX_BROWSER_EXECUTABLE' };
test('sandbox admission uses the selected executable and renders/reopens two private contexts', async () => {
  const f = fixture();
  assert.deepEqual(await preflightBrowserCompanion(selection, f.launcher), { ...selection, version: 'fixture-version', sandbox: true });
  assert.deepEqual(f.launches, [{ headless: true, timeout: 20_000, executablePath: selection.executablePath, chromiumSandbox: true }]);
  assert.deepEqual(f.counts(), { contexts: 2, closedContexts: 2, browserCloses: 1 });
});

test('sandbox refusal is actionable and never tries a replacement executable', async () => {
  let launches = 0;
  const cause = new Error('No usable sandbox');
  await assert.rejects(preflightBrowserCompanion(selection, { async launch(options) {
    launches++; assert.equal(options?.executablePath, selection.executablePath); throw cause;
  } }), (error: Error & { cause?: unknown }) => {
    assert.match(error.message, /WEBMUX_BROWSER_EXECUTABLE/);
    assert.match(error.message, /sandbox enabled/);
    assert.match(error.message, /No fallback/);
    assert.equal(error.cause, cause); return true;
  });
  assert.equal(launches, 1);
});

for (const scenario of ['badTitle', 'badVersion'] as const) {
  test(`${scenario} refuses readiness and closes owned resources`, async () => {
    const f = fixture({ [scenario]: true });
    await assert.rejects(preflightBrowserCompanion(selection, f.launcher), /preflight failed/);
    assert.deepEqual(f.counts(), { contexts: scenario === 'badTitle' ? 1 : 0, closedContexts: scenario === 'badTitle' ? 1 : 0, browserCloses: 1 });
  });
}
