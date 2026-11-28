// Companion selection is independent of the Playwright test driver's launch.
import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { chromium, type BrowserType } from 'playwright-core';
import { checkBrowser } from './check-browser.mts';

export interface CompanionSelection { executablePath: string; source: string }

export function selectBrowserCompanion(
  env: NodeJS.ProcessEnv = process.env,
  platform: NodeJS.Platform = process.platform,
  managedExecutable: () => string = () => chromium.executablePath(),
  executable: (candidate: string) => boolean = candidate => {
    try { fs.accessSync(candidate, fs.constants.X_OK); return true; } catch { return false; }
  },
): CompanionSelection {
  if (env.WEBMUX_BROWSER_EXECUTABLE) return { executablePath: env.WEBMUX_BROWSER_EXECUTABLE, source: 'WEBMUX_BROWSER_EXECUTABLE' };
  const installed = platform === 'darwin' ? [
    '/Applications/Chromium.app/Contents/MacOS/Chromium',
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  ].find(executable) : undefined;
  if (installed) return { executablePath: installed, source: 'installed macOS browser' };
  if (env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH) return { executablePath: env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH, source: 'PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH' };
  return { executablePath: managedExecutable(), source: 'Playwright managed Chromium' };
}

// This is OS sandbox/CDP readiness, not application/profile qualification. Keep
// the companion's normal sandbox enabled; the driver readiness check is separate.
export async function preflightBrowserCompanion(
  selection: CompanionSelection,
  browserType: Pick<BrowserType, 'launch'> = chromium,
): Promise<{ executablePath: string; source: string; version: string; sandbox: true }> {
  let version = '';
  try {
    await checkBrowser({ launch: async options => {
      const browser = await browserType.launch({ ...options, chromiumSandbox: true });
      try { version = browser.version(); }
      catch (error) { await browser.close(); throw error; }
      return browser;
    } }, selection.executablePath);
  } catch (cause) {
    throw new Error(`Linux browser companion preflight failed for ${JSON.stringify(selection.executablePath)} (${selection.source}) with the sandbox enabled. `
      + 'The Playwright driver is qualified separately. Check that this executable can run with the host sandbox policy, '
      + 'or explicitly select an installed sandbox-capable browser with WEBMUX_BROWSER_EXECUTABLE. No fallback was attempted.', { cause });
  }
  return { ...selection, version, sandbox: true };
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href && process.platform === 'linux') {
  console.error('Browser companion preflight:', JSON.stringify(await preflightBrowserCompanion(selectBrowserCompanion())));
}
