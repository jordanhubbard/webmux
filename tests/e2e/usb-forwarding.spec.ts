import { test, expect } from '@playwright/test';

test('USB setup works across workspaces and fits a narrow screen', async ({ page, request }) => {
  await page.goto('/');
  for (const workspace of ['Terminals', 'Desktops']) {
    await page.getByRole('button', { name: workspace, exact: true }).click();
    await page.getByRole('button', { name: 'More', exact: true }).click();
    const opener = page.getByRole('button', { name: 'USB redirection' });
    await opener.focus();
    await page.keyboard.press('Enter');
    const dialog = page.getByRole('dialog', { name: 'USB redirection' });
    await expect(dialog.getByLabel('Receiving SSH host')).toBeFocused();
    await dialog.getByLabel('Receiving SSH host').fill('developer@build-host');
    const created = page.waitForResponse(response => response.request().method() === 'POST'
      && response.url().endsWith('/api/usb-forwardings'));
    await dialog.getByRole('button', { name: 'Create local command', exact: true }).click();
    const lease = await (await created).json() as { lease: { id: string } };
    await expect(dialog.getByLabel('USB forwarding command')).toHaveValue(/--host developer@build-host .*--dashboard-link [A-Za-z0-9_-]+$/);
    await expect(dialog.getByRole('button', { name: 'Copy local command' })).toBeEnabled();
    await page.getByRole('button', { name: 'More', exact: true }).evaluate(element => element.focus());
    expect(await dialog.evaluate(element => element.contains(document.activeElement))).toBe(true);
    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible();
    await expect(opener).toBeFocused();
    expect((await request.post('/api/usb-forwardings/' + lease.lease.id + '/stop')).ok()).toBe(true);
    await page.getByRole('button', { name: 'Close more' }).click();
  }
  await page.setViewportSize({ width: 375, height: 667 });
  await page.getByRole('button', { name: 'More', exact: true }).click();
  await page.getByRole('button', { name: 'USB redirection' }).click();
  const panel = page.getByRole('dialog', { name: 'USB redirection' }).locator('.webmux-dialog-panel');
  const bounds = await panel.boundingBox();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(375);
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(667);
  expect(await panel.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
});

test('USB dashboard pairs a helper and waits for confirmed release after Stop', async ({ page, request }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'More', exact: true }).click();
  await page.getByRole('button', { name: 'USB redirection' }).click();
  const dialog = page.getByRole('dialog', { name: 'USB redirection' });
  await dialog.getByLabel('Receiving SSH host').fill('browser-fixture@receiver');
  const created = page.waitForResponse(response => response.request().method() === 'POST'
    && response.url().endsWith('/api/usb-forwardings'));
  await dialog.getByRole('button', { name: 'Create local command', exact: true }).click();
  const ticket = await (await created).json() as {
    lease: { id: string; spec: { host: string; ssh_port: number; export_port: number; seconds: number } }; pair_code: string;
  };
  const path = '/api/usb-forwardings/' + ticket.lease.id;
  try {
    const claim = await request.post(path + '/claim', { data: { pair_code: ticket.pair_code, spec: ticket.lease.spec } });
    expect(claim.ok()).toBe(true);
    const { credential } = await claim.json() as { credential: string };
    const report = (data: object) => request.post(path + '/report', {
      headers: { Authorization: 'Bearer ' + credential }, data,
    });
    expect((await report({ phase: 'ready', remote_port: 23240 })).ok()).toBe(true);
    await expect(dialog.getByText(/Transport ready on 127.0.0.1:23240/)).toBeVisible();
    await dialog.getByRole('button', { name: 'Stop forwarding to browser-fixture@receiver' }).click();
    await expect(dialog.getByText('Stop requested; waiting for remote release')).toBeVisible();
    const control = await report({ phase: 'ready', remote_port: 23240 });
    expect(await control.json()).toEqual({ stop: true });
    await expect(dialog.getByText('Stop requested; waiting for remote release')).toBeVisible();
    expect((await report({ phase: 'ended', remote_port: 23240, release_confirmed: true })).ok()).toBe(true);
    await expect(dialog.getByText('Forwarding ended; remote listener released').last()).toBeVisible();
  } finally {
    expect((await request.post(path + '/stop')).ok()).toBe(true);
  }
});
