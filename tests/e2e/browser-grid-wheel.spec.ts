import { test, expect } from '@playwright/test';
import http from 'node:http';
import { writeFileSync } from 'node:fs';

test('Shift wheel over a browser portal pans only its owning grid', async ({ page, request }, info) => {
  test.setTimeout(120_000);
  let pageScrolls = 0;
  const scrollPositions: string[] = [];
  const site = http.createServer((incoming, response) => {
    if (incoming.url?.startsWith('/scrolled')) { pageScrolls++; scrollPositions.push(incoming.url); response.end('ok'); return; }
    response.setHeader('Content-Type', 'text/html');
    response.end('<title>Browser wheel fixture</title><style>body{width:4000px;height:4000px}</style>Scroll this page<script>addEventListener("scroll",()=>fetch("/scrolled?x="+scrollX+"&y="+scrollY))</script>');
  });
  await new Promise<void>(resolve => site.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${(site.address() as { port: number }).port}/`;
  const ids: string[] = [];
  const scrollActions: Array<{ modifiers?: number; deltaX?: number; deltaY?: number; x?: number; y?: number }> = [];
  const scrollResults: Array<{ status: number; error?: string; width?: number; height?: number; owned?: boolean; readError?: string }> = [];
  const pendingResults: Array<Promise<void>> = [];
  let endpoint = '';
  const grid = page.getByTestId('workspace-scroll');
  try {
    for (const [row, col] of [[0, 0], [0, 1], [2, 2]]) {
      const created = await request.post('/api/sessions', { data: { transport: 'local', row, col } });
      expect(created.ok()).toBe(true);
      ids.push((await created.json()).id);
    }
    endpoint = `/api/sessions/${ids[0]}/browser`;
    page.on('request', outgoing => {
      if (new URL(outgoing.url()).pathname === endpoint && outgoing.method() === 'POST') {
        const body = outgoing.postDataJSON();
        if (body.action === 'scroll') scrollActions.push({ modifiers: body.modifiers, deltaX: body.deltaX, deltaY: body.deltaY, x: body.x, y: body.y });
      }
    });
    page.on('response', incoming => {
      const outgoing = incoming.request();
      if (new URL(outgoing.url()).pathname === endpoint && outgoing.method() === 'POST' && outgoing.postDataJSON().action === 'scroll') {
        pendingResults.push(incoming.json().then(body => { scrollResults.push({ status: incoming.status(), error: body.error, width: body.width, height: body.height, owned: body.control?.owned }); }, error => { scrollResults.push({ status: incoming.status(), readError: String(error) }); }));
      }
    });
    await page.setViewportSize({ width: 1440, height: 850 });
    await page.goto('/');
    const tile = page.getByTestId(`tile-cell-${ids[0]}`);
    await tile.locator('.xterm-helper-textarea').focus();
    await tile.getByRole('button', { name: 'Session tools', exact: true }).click();
    await page.getByRole('button', { name: 'Open browser', exact: true }).click();
    const image = page.getByAltText('Interactive remote browser viewport');
    await expect(image).toBeVisible({ timeout: 30_000 });
    await page.getByRole('textbox', { name: 'Remote browser URL', exact: true }).fill(origin);
    await page.getByRole('button', { name: 'Go', exact: true }).click();
    await expect.poll(async () => {
      const response = await request.post(endpoint, { data: { action: 'frame' } });
      expect(response.ok()).toBe(true);
      return (await response.json()).url;
    }).toBe(origin);
    await grid.evaluate(element => { element.scrollLeft = 0; element.scrollTop = 0; });
    expect(await grid.evaluate(element => element.scrollWidth > element.clientWidth && element.scrollHeight > element.clientHeight)).toBe(true);
    const box = (await image.boundingBox())!;
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    const before = pageScrolls;
    await page.keyboard.down('Shift');
    try { await page.mouse.wheel(120, 180); } finally { await page.keyboard.up('Shift'); }
    await expect.poll(() => grid.evaluate(element => element.scrollLeft > 0 && element.scrollTop > 0)).toBe(true);
    const position = await grid.evaluate(element => [element.scrollLeft, element.scrollTop]);
    expect(pageScrolls).toBe(before);

    // Positive browser control also drains actions queued before this gesture.
    await page.mouse.wheel(0, 220);
    await expect.poll(() => pageScrolls).toBeGreaterThan(before);
    expect(Number(new URL(scrollPositions.at(-1)!, origin).searchParams.get('y'))).toBeGreaterThan(0);
    const vertical = pageScrolls;
    await page.mouse.wheel(180, 0);
    await expect.poll(() => pageScrolls).toBeGreaterThan(vertical);
    expect(Number(new URL(scrollPositions.at(-1)!, origin).searchParams.get('x'))).toBeGreaterThan(0);
    await expect.poll(() => scrollResults.length).toBe(2);
    expect(scrollResults.every(result => result.status === 200 && !result.error && !result.readError && result.owned)).toBe(true);
    expect(scrollActions.some(action => (action.modifiers ?? 0) & 8)).toBe(false);
    expect(scrollActions.some(action => !action.modifiers)).toBe(true);
    expect(await grid.evaluate(element => [element.scrollLeft, element.scrollTop])).toEqual(position);
  } finally {
    await Promise.allSettled(pendingResults);
    const evidence = { pageScrolls, scrollPositions, scrollActions, scrollResults,
      viewport: await page.getByAltText('Interactive remote browser viewport').evaluate(element => ({ width: element.clientWidth, height: element.clientHeight, naturalWidth: (element as HTMLImageElement).naturalWidth, naturalHeight: (element as HTMLImageElement).naturalHeight })).catch(() => null),
      position: await grid.evaluate(element => [element.scrollLeft, element.scrollTop]).catch(() => null) };
    writeFileSync(info.outputPath('browser-portal-wheel.json'), JSON.stringify(evidence));
    await info.attach('browser-portal-wheel', {
      body: JSON.stringify(evidence),
      contentType: 'application/json',
    });
    try {
      for (const id of ids) await request.delete(`/api/sessions/${id}`, { timeout: 10_000 });
    } finally {
      site.closeAllConnections();
      await new Promise<void>(resolve => site.close(() => resolve()));
    }
  }
});
