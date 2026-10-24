import { test, expect } from '@playwright/test';

test.describe('Health check', () => {
  test('API health endpoint returns ok', async ({ request }) => {
    const res = await request.get('/api/health');
    expect(res.ok()).toBe(true);
    const body = await res.json();
    expect(body.status).toBe('ok');
    expect(body.name).toBe('webmux');
  });

  test('auth status reports mode none', async ({ request }) => {
    const res = await request.get('/api/auth/status');
    expect(res.ok()).toBe(true);
    const body = await res.json();
    expect(body.mode).toBe('none');
  });
});

test.describe('UI loads', () => {
  test('serves index.html with correct title', async ({ page }) => {
    await page.goto('/');
    await expect(page).toHaveTitle('WebMux');
  });

  test('skips login in no-auth mode and shows workspace', async ({ page }) => {
    await page.goto('/');
    // Both panes render in DOM (one hidden via display:none); .first() picks the
    // active terminals pane which is always rendered first.
    await expect(page.locator('text=Click to add a session').first()).toBeVisible({ timeout: 10_000 });
  });

  test('top bar renders with logo and controls', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('text=Click to add a session').first()).toBeVisible({ timeout: 10_000 });

    await expect(page.locator('text=WebMux').first()).toBeVisible();
    await expect(page.locator('text=Type to All')).toBeVisible();
  });

  test('add cell offers Rig seats and host connections', async ({ page }) => {
    await page.goto('/');
    // Both panes render add-cell-0-0; .first() targets the terminals pane.
    const addCell = page.getByTestId('add-cell-0-0').first();
    await expect(addCell).toBeVisible({ timeout: 10_000 });

    await addCell.click();
    await expect(page.getByRole('dialog', { name: 'Add to grid' }).getByRole('button', { name: 'Open or create Rig seat' })).toBeVisible();
    await page.getByRole('dialog', { name: 'Add to grid' }).getByRole('button', { name: 'Connect to host' }).click();
    await expect(page.getByRole('dialog', { name: 'Connect to Host' })).toBeVisible();
  });

  test('config endpoint returns valid app config', async ({ request }) => {
    // Auth mode is none, but config endpoint still needs a token for the
    // middleware to pass — in "none" mode, middleware lets everything through.
    const res = await request.get('/api/config');
    expect(res.ok()).toBe(true);
    const body = await res.json();
    expect(body.app).toBeDefined();
    expect(body.app.default_term).toBeDefined();
    expect(body.app.default_term.cols).toBe(80);
  });

  test('settings gear enables session transcript logging', async ({ page, request }) => {
    await page.goto('/');
    await expect(page.locator('text=Click to add a session').first()).toBeVisible({ timeout: 10_000 });

    await page.getByRole('button', { name: 'Settings' }).click();
    const dialog = page.getByRole('dialog', { name: 'Settings' });
    await expect(dialog).toBeVisible();
    await dialog.getByRole('checkbox', { name: /Log terminal sessions to disk/ }).check();
    await dialog.getByRole('button', { name: 'Save settings' }).click();
    await expect(dialog.getByText('Settings saved')).toBeVisible();

    const configRes = await request.get('/api/config');
    expect(configRes.ok()).toBe(true);
    expect((await configRes.json()).app.session_logging.enabled).toBe(true);

    const resetRes = await request.put('/api/config', {
      data: { app: { session_logging: { enabled: false } } },
    });
    expect(resetRes.ok()).toBe(true);
  });
});
