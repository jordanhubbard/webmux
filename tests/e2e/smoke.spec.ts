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

  test('add cell opens connection dialog on click', async ({ page }) => {
    await page.goto('/');
    // Both panes render add-cell-0-0; .first() targets the terminals pane.
    const addCell = page.getByTestId('add-cell-0-0').first();
    await expect(addCell).toBeVisible({ timeout: 10_000 });

    await addCell.click();
    await expect(page.locator('text=Connect to Host')).toBeVisible();
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
});
