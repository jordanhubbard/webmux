// A browser that launches but exits after the first test is not E2E-ready.
import { chromium, type BrowserType } from 'playwright-core';
import { pathToFileURL } from 'node:url';
import path from 'node:path';

export async function checkBrowser(browserType: Pick<BrowserType, 'launch'> = chromium, executablePath?: string): Promise<void> {
  const browser = await browserType.launch({ headless: true, timeout: 20_000, ...(executablePath ? { executablePath } : {}) });
  try {
    for (let round = 0; round < 2; round++) {
      const context = await browser.newContext();
      try {
        const page = await context.newPage();
        await page.goto('data:text/html,<title>WebMux browser readiness</title>', { timeout: 5000 });
        if (await page.title() !== 'WebMux browser readiness') throw new Error('Browser readiness page did not render');
      } finally { await context.close(); }
      // Some system browsers acknowledge context closure before exiting.
      await new Promise(resolve => setTimeout(resolve, 200));
      if (!browser.isConnected()) throw new Error('Browser exited after closing an isolated context');
    }
  } finally { await browser.close(); }
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  try { await checkBrowser(chromium, process.argv[2]); }
  catch { process.exitCode = 1; }
}
