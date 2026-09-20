// Exit successfully only when Playwright's managed Chromium can launch.
import { chromium } from 'playwright-core';

try {
  const browser = await chromium.launch({ headless: true, timeout: 20_000 });
  await browser.close();
} catch { process.exitCode = 1; }
