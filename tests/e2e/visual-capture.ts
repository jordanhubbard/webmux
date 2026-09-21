import { expect, type Page, type TestInfo } from '@playwright/test';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';

export async function capture(page: Page, info: TestInfo, name: string): Promise<void> {
  await page.evaluate(() => document.fonts.ready);
  const session = await page.context().newCDPSession(page);
  let renderingArguments: string[];
  try {
    const command = await session.send('Browser.getBrowserCommandLine');
    renderingArguments = command.arguments.filter(argument => /^--(?:disable-gpu$|force-color-profile=|disable-partial-raster$)/.test(argument));
  } finally { await session.detach(); }
  const diagnostic = JSON.stringify({ renderingArguments, ...await page.evaluate(() => ({
    userAgent: navigator.userAgent,
    viewport: { width: innerWidth, height: innerHeight, scale: devicePixelRatio },
    fonts: [...document.fonts].map(font => ({ family: font.family, status: font.status })),
    buttons: [...document.querySelectorAll('button')].map(button => {
      const style = getComputedStyle(button);
      return { text: button.textContent, title: button.title, bounds: button.getBoundingClientRect().toJSON(),
        styles: Object.fromEntries([...style].map(key => [key, style.getPropertyValue(key)])) };
    }),
  })) }, null, 2);
  const baselineDiagnostic = info.snapshotPath(`${name}.json`);
  if (info.config.updateSnapshots === 'all') {
    await mkdir(path.dirname(baselineDiagnostic), { recursive: true });
    await writeFile(baselineDiagnostic, diagnostic);
  }
  try { await comparePixels(page, info, name); }
  catch (error) {
    for (const [label, data] of [['actual', diagnostic], ['baseline', await readFile(baselineDiagnostic)]] as const) {
      const file = info.outputPath(`${name}-${label}-layout.json`);
      await writeFile(file, data);
      await info.attach(`${name}-${label}-layout`, { path: file, contentType: 'application/json' });
    }
    throw error;
  }
}

async function comparePixels(page: Page, info: TestInfo, name: string): Promise<void> {
  await expect(page).toHaveScreenshot(name, { animations: 'disabled', caret: 'hide', threshold: 0, maxDiffPixels: 0 });
  // Playwright's comparator may ignore anti-aliasing differences even at zero
  // threshold. Compare every decoded RGBA pixel as well, without that exemption.
  const actual = await page.screenshot({ animations: 'disabled', caret: 'hide' });
  const expected = await readFile(info.snapshotPath(name));
  const differences = await page.evaluate(async ({ actual, expected }) => {
    const pixels = async (encoded: string): Promise<ImageData> => {
      const image = new Image();
      image.src = `data:image/png;base64,${encoded}`;
      await image.decode();
      const canvas = document.createElement('canvas');
      canvas.width = image.naturalWidth; canvas.height = image.naturalHeight;
      const context = canvas.getContext('2d');
      if (!context) throw new Error('Screenshot decoder unavailable');
      context.drawImage(image, 0, 0);
      return context.getImageData(0, 0, canvas.width, canvas.height);
    };
    const a = await pixels(actual), b = await pixels(expected);
    if (a.width !== b.width || a.height !== b.height) return -1;
    let changed = 0;
    for (let i = 0; i < a.data.length; i += 4) {
      if (a.data[i] !== b.data[i] || a.data[i+1] !== b.data[i+1] || a.data[i+2] !== b.data[i+2] || a.data[i+3] !== b.data[i+3]) changed++;
    }
    return changed;
  }, { actual: actual.toString('base64'), expected: expected.toString('base64') });
  if (differences !== 0) {
    const file = info.outputPath(`raw-${name}`);
    await writeFile(file, actual);
    await info.attach(`raw-${name}`, { path: file, contentType: 'image/png' });
  }
  expect(differences, `${name}: raw RGBA pixel differences`).toBe(0);
}

export async function captureBoth(page: Page, info: TestInfo, name: string): Promise<void> {
  await capture(page, info, `${name}.png`);
  await page.setViewportSize({ width: 375, height: 667 });
  await capture(page, info, `${name}-narrow.png`);
  await page.setViewportSize({ width: 1280, height: 800 });
}
