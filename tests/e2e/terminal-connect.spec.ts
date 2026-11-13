import { test, expect } from '@playwright/test';

test('empty slots offer local tmux, saved tuples, one-time connections and optional coordinators', async ({ page, request }) => {
  test.skip(process.platform === 'win32', 'Local tmux requires Unix');
  const saved = await request.post('/api/hosts', { data: { hostname: 'saved.example.test', username: 'saved-user', port: 2222, transport: 'ssh', tags: [], mosh_allowed: false } });
  expect(saved.ok()).toBe(true);
  const { id: hostId } = await saved.json();
  const sessionIds: string[] = [];
  const attempts: Record<string, unknown>[] = [];
  try {
    await page.goto('/');
    const add = page.getByTestId('add-cell-0-0').first();
    await add.click();
    const connection = page.getByRole('dialog', { name: 'Connect to Host', exact: true });
    await expect(connection.getByRole('button', { name: 'Localhost' })).toBeFocused();
    await expect(connection.getByLabel('Coordinator', { exact: true })).toHaveValue('none');
    await expect(connection.getByPlaceholder('hostname or IP')).toBeHidden();
    await expect(connection.getByTitle('Connect to saved-user@saved.example.test:2222')).toBeVisible();
    // Localhost creates a real persistent tmux terminal at the chosen position.
    const created = page.waitForResponse(response => response.url().endsWith('/api/sessions') && response.request().method() === 'POST');
    await page.keyboard.press('Enter');
    const response = await created; expect(response.ok()).toBe(true);
    const session = await response.json(); sessionIds.push(session.id);
    expect(session.transport).toBe('local'); expect(session.row).toBe(0); expect(session.col).toBe(0);
    await expect(connection).toHaveCount(0);
    await expect(page.getByTestId(`tile-cell-${session.id}`).locator('.xterm-helper-textarea')).toBeVisible();
    await request.delete(`/api/sessions/${session.id}`);
    await page.reload();
    await expect(add).toBeVisible();
    await add.click();
    // A controlled connection failure lets us inspect remote requests without
    // depending on an external SSH service or opening an unknown network target.
    await page.route('**/api/sessions', async route => {
      if (route.request().method() !== 'POST') { await route.continue(); return; }
      attempts.push(route.request().postDataJSON());
      await route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ error: 'SSH test connection refused' }) });
    });
    await connection.getByTitle('Connect to saved-user@saved.example.test:2222').click();
    await expect(connection.getByRole('alert')).toContainText('SSH test connection refused');
    expect(attempts[0]).toMatchObject({ host_id: hostId, hostname: 'saved.example.test', username: 'saved-user', port: 2222, transport: 'ssh' });
    await connection.getByText('New or one-time remote connection', { exact: true }).click();
    await connection.getByPlaceholder('hostname or IP').fill('one-time.example.test');
    await connection.getByPlaceholder('user', { exact: true }).fill('temporary-user');
    await connection.getByRole('button', { name: 'Connect', exact: true }).click();
    await expect.poll(() => attempts.length).toBe(2);
    expect(attempts[1]).toMatchObject({ hostname: 'one-time.example.test', username: 'temporary-user', port: 22, transport: 'ssh' });
    expect((await (await request.get('/api/hosts')).json()).some((host: { hostname: string }) => host.hostname === 'one-time.example.test')).toBe(false);
    // Wait for the response to finish: in-flight connections deliberately
    // keep the modal open, so Escape before this boundary is ignored.
    await expect(connection.getByRole('button', { name: 'Connect', exact: true })).toBeEnabled();
    // The saved-host list survives closing and reopening the dialog.
    await page.keyboard.press('Escape');
    await expect(connection).toHaveCount(0);
    await add.click();
    await expect(connection.getByTitle('Connect to saved-user@saved.example.test:2222')).toBeVisible();
    for (const [provider, label] of [['agent-deck', 'Agent Deck'], ['rig', 'Rig']]) {
      await connection.getByLabel('Coordinator', { exact: true }).selectOption(provider);
      const launch = page.getByRole('dialog', { name: `New ${label} session`, exact: true });
      await expect(launch.getByLabel('Code folder', { exact: true })).toBeVisible();
      await expect(launch.getByLabel('Launch owner', { exact: true })).toHaveCount(0);
      await expect(launch.getByText('Your projects and sessions', { exact: true })).toHaveCount(0);
      await launch.getByRole('button', { name: 'Back to terminal connection' }).click();
      await expect(connection.getByLabel('Coordinator', { exact: true })).toHaveValue('none');
    }
    await page.screenshot({ path: 'test-results/terminal-connect.png' });
  } finally {
    await request.delete(`/api/hosts/${hostId}`);
    for (const id of sessionIds) await request.delete(`/api/sessions/${id}`);
  }
});
