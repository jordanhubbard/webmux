import { disposeFixtureTerminal, observeFixtureBrowser } from './terminal-lifecycle.mts';
import { test, expect } from '@playwright/test';
import http from 'node:http';

test.use({ hasTouch: true });

test('small browser page-text controls retain native typing focus', async ({ page, request }, info) => {
  test.setTimeout(90_000);
  let actions = 0;
  let typed = '';
  const site = http.createServer((req, res) => {
    if (req.url === '/action') { actions++; res.end('ok'); return; }
    if (req.url?.startsWith('/typed?')) {
      typed = new URL(req.url, 'http://fixture').searchParams.get('value') ?? '';
      res.end('ok'); return;
    }
    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.end(`<title>Mobile page text fixture</title>
      <button onclick="fetch('/action')">Confirm mobile action</button>
      <label>Mobile note<input oninput="fetch('/typed?value='+encodeURIComponent(this.value))"></label>`);
  });
  await new Promise<void>(resolve => site.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${(site.address() as { port: number }).port}`;
  const created = await request.post('/api/sessions', { data: { transport: 'local' } });
  expect(created.ok()).toBe(true);
  const { id } = await created.json() as { id: string };
  const endFixtureBrowser = observeFixtureBrowser(page, request, id);
  try {
    await page.goto('/');
    await page.getByTestId(`tile-cell-${id}`).getByRole('button', { name: 'Session tools', exact: true }).click();
    await page.getByRole('button', { name: 'Open browser', exact: true }).click();
    await expect(page.getByAltText('Interactive remote browser viewport')).toBeVisible({ timeout: 30_000 });
    const companion = page.getByRole('complementary', { name: /^Browser for / });
    const address = page.getByRole('textbox', { name: 'Remote browser URL', exact: true });
    const panel = page.getByRole('region', { name: 'Remote page text', exact: true });
    const keyboard = page.getByRole('textbox', { name: 'Type into remote page', exact: true });
    for (const [index, size] of [{ width: 320, height: 568 }, { width: 568, height: 320 }, { width: 390, height: 400 }].entries()) {
      await page.setViewportSize(size);
      const url = `${origin}/?layout=${size.width}x${size.height}`;
      await address.fill(url);
      await page.getByRole('button', { name: 'Go', exact: true }).tap();
      await expect(address).toHaveValue(url);
      await page.getByRole('button', { name: 'Read page text', exact: true }).tap();
      await expect(panel).toBeVisible();
      const activate = panel.getByRole('button', { name: 'Activate Confirm mobile action', exact: true });
      await activate.scrollIntoViewIfNeeded();
      await expect(activate).toBeInViewport();
      expect(await activate.evaluate(el => el.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44);
      const geometry = await companion.evaluate(el => ({ width: el.clientWidth, scrollWidth: el.scrollWidth }));
      expect(geometry.scrollWidth).toBeLessThanOrEqual(geometry.width);
      await page.screenshot({ path: info.outputPath(`page-text-${size.width}x${size.height}.png`) });
      await activate.tap();
      await expect.poll(() => actions).toBe(index + 1);
      await page.getByRole('button', { name: 'Read page text', exact: true }).tap();
      await panel.getByRole('button', { name: 'Focus Mobile note', exact: true }).tap();
      await expect(keyboard).toBeFocused();
      const value = `native-${size.width}`;
      await page.keyboard.insertText(value);
      await expect.poll(() => typed).toBe(value);
      await expect(keyboard).toBeFocused();
      await page.keyboard.insertText('-continued');
      await expect.poll(() => typed).toBe(value + '-continued');
      await page.keyboard.press('Shift+Escape');
      await expect(address).toBeFocused();
    }
    const back = page.getByRole('button', { name: 'Return to terminal', exact: true });
    await back.scrollIntoViewIfNeeded();
    await back.tap();
    await expect(companion).toHaveCount(0);
    await expect(page.getByTestId(`tile-cell-${id}`)).toBeVisible();
  } finally {
    try {
      await endFixtureBrowser();
      await disposeFixtureTerminal(request, id, info);
    } finally {
      site.closeAllConnections();
      await new Promise<void>(resolve => site.close(() => resolve()));
    }
  }
});
