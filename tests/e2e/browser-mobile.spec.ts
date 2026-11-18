import { test, expect } from '@playwright/test';
import http from 'node:http';

test.use({ hasTouch: true });

test('small browser companion keeps address entry and page controls usable', async ({ page, request }, info) => {
  test.setTimeout(90_000);
  let actions = 0;
  const site = http.createServer((req, res) => {
    if (req.url === '/action') { actions++; res.end('ok'); return; }
    res.setHeader('Content-Type', 'text/html');
    res.end('<title>Small screen fixture</title><button style="width:180px;height:60px" onclick="fetch(\'/action\')">Confirm page action</button>');
  });
  await new Promise<void>(resolve => site.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${(site.address() as { port: number }).port}`;
  const created = await request.post('/api/sessions', { data: { transport: 'local' } });
  expect(created.ok()).toBe(true);
  const { id } = await created.json() as { id: string };
  try {
    await page.goto('/');
    await page.getByTestId(`tile-cell-${id}`).getByRole('button', { name: 'Session tools', exact: true }).click();
    await page.getByRole('button', { name: 'Open browser', exact: true }).click();
    const image = page.getByAltText('Interactive remote browser viewport');
    await expect(image).toBeVisible({ timeout: 30_000 });
    const address = page.getByRole('textbox', { name: 'Remote browser URL', exact: true });
    await page.setViewportSize({ width: 320, height: 568 });
    await address.scrollIntoViewIfNeeded();
    await page.screenshot({ path: info.outputPath('portrait.png') });
    const width = await address.evaluate(element => element.getBoundingClientRect().width);
    await info.attach('address-width', { body: JSON.stringify({ width }), contentType: 'application/json' });
    expect(width, 'address remains wide enough to review a destination on the narrow panel').toBeGreaterThanOrEqual(180);
    await expect(page.getByRole('button', { name: 'Return to terminal', exact: true })).toBeInViewport();
    await expect(page.getByRole('combobox', { name: 'Browser profile', exact: true })).toBeHidden();
    await page.getByText('Browser settings', { exact: true }).click();
    await expect(page.getByRole('combobox', { name: 'Browser profile', exact: true })).toBeVisible();
    await page.getByText('Browser settings', { exact: true }).click();
    await address.fill(origin);
    await page.getByRole('button', { name: 'Go', exact: true }).click();
    await expect(address).toHaveValue(origin + '/');
    const companion = page.getByRole('complementary', { name: /^Browser for / });
    let expectedActions = 0;
    for (const size of [{ width: 320, height: 568 }, { width: 568, height: 320 }, { width: 390, height: 400 }]) {
      await page.setViewportSize(size);
      await address.scrollIntoViewIfNeeded();
      const geometry = await companion.evaluate(element => ({ width: element.clientWidth, scrollWidth: element.scrollWidth }));
      expect(geometry.scrollWidth).toBeLessThanOrEqual(geometry.width);
      expect(await address.evaluate(element => element.getBoundingClientRect().width)).toBeGreaterThanOrEqual(180);
      await image.scrollIntoViewIfNeeded();
      await expect(image).toBeInViewport();
      const dimensions = await image.evaluate(element => ({ width: element.clientWidth, height: element.clientHeight }));
      expect(dimensions.width).toBeGreaterThan(200);
      expect(dimensions.height).toBeGreaterThanOrEqual(240);
      // Wait for the real remote worker to match the resized viewer before
      // testing a touch coordinate. The request does not take over its lease.
      await expect.poll(async () => {
        const response = await request.post(`/api/sessions/${id}/browser`, { data: { action: 'frame' } });
        const frame = await response.json();
        return { width: frame.width, height: frame.height };
      }).toEqual({ width: Math.max(320, Math.round(dimensions.width)), height: Math.max(240, Math.round(dimensions.height)) });
      const natural = await image.evaluate(element => ({ width: (element as HTMLImageElement).naturalWidth, height: (element as HTMLImageElement).naturalHeight }));
      await image.tap({ position: { x: 80 * dimensions.width / natural.width, y: 30 * dimensions.height / natural.height } });
      await expect.poll(() => actions).toBe(++expectedActions);
      await page.screenshot({ path: info.outputPath(`page-${size.width}x${size.height}.png`) });
    }
    await page.getByRole('button', { name: 'Return to terminal', exact: true }).click();
    await expect(companion).toHaveCount(0);
    await expect(page.getByTestId(`tile-cell-${id}`)).toBeVisible();
  } finally {
    await request.delete(`/api/sessions/${id}`);
    await new Promise<void>(resolve => site.close(() => resolve()));
  }
});
