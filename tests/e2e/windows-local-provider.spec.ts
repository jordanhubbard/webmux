import { test, expect } from '@playwright/test';
import { randomBytes, randomUUID } from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { disposeFixtureTerminal } from './terminal-lifecycle.mts';

test('native Windows local application connects, produces fresh shell output and confirms Stop', async ({ page, request }, info) => {
  test.skip(process.platform !== 'win32', 'Native Windows application lifecycle gate');
  test.setTimeout(90_000);
  const created = await request.post('/api/sessions', { data: { transport: 'local', cols: 100, rows: 30 } });
  expect(created.ok()).toBe(true);
  const session = await created.json();
  const id = session.id as string;
  const nonce = 'WIN_LOCAL_' + randomBytes(16).toString('hex');
  const receipt = path.resolve(__dirname, '.test-home', nonce + '.json');
  try {
    await info.attach('native-local-creation', { contentType: 'application/json', body: JSON.stringify({
      id, state: session.state, lifecycle: session.local_lifecycle,
    }) });
    expect(session.state, 'An error-state session is not a connected native terminal').not.toBe('error');
    expect(session.local_lifecycle?.contract).toBe('webmux-local-provider-lifecycle/v1');
    expect(session.local_lifecycle?.selection, 'Native owner handshake must precede shell qualification').toMatch(/^[a-f0-9]{64}$/);
    await page.goto('/');
    const tile = page.getByTestId(`tile-cell-${id}`);
    const input = tile.locator('.xterm-helper-textarea');
    await expect(input).toBeVisible();
    const child = path.resolve(__dirname, 'windows-local-child.mts');
    await input.focus(); await page.keyboard.type(`"${process.execPath}" "${child}" "${receipt}" "${nonce}"`); await page.keyboard.press('Enter');
    await expect.poll(async () => (await tile.locator('.xterm-rows').innerText()).split(/\r?\n/).some(line => line.trim() === nonce)).toBe(true);
    const launched = JSON.parse(fs.readFileSync(receipt, 'utf8')) as { pid: number; nonce: string; expiresAfter: number };
    expect(launched.nonce).toBe(nonce); expect(launched.pid).toBeGreaterThan(0);
    const before = await request.get(`/api/sessions/${id}`); expect(before.ok()).toBe(true);
    const original = (await before.json()).local_lifecycle;
    expect(original.selection).toBe(session.local_lifecycle.selection);
    process.kill(launched.pid, 0); // Read-only liveness check before dispatch.
    expect(Date.now()).toBeLessThan(launched.expiresAfter - 15_000);
    const action = { id: randomUUID(), selection: original.selection, mode: 'stop', confirm: true };
    const stopped = await request.post(`/api/sessions/${id}/local-stop`, { data: action });
    expect(stopped.status()).toBe(200);
    const result = await stopped.json();
    expect(result.outcome).toBe('stopped'); expect(result.session.id).toBe(id);
    expect(result.session.local_lifecycle).toMatchObject({ state: 'stopped', selection: action.selection, operation_id: action.id });
    await expect.poll(() => {
      try { process.kill(launched.pid, 0); return false; }
      catch (error) { if ((error as NodeJS.ErrnoException).code === 'ESRCH') return true; throw error; }
    }, { message: 'Confirmed Stop must also end the owned shell child' }).toBe(true);
    expect(Date.now(), 'Independent child expiry must not explain the teardown').toBeLessThan(launched.expiresAfter);
    const duplicate = await request.post(`/api/sessions/${id}/local-stop`, { data: action });
    expect(duplicate.status()).toBe(200); expect((await duplicate.json()).outcome).toBe('stopped');
    await info.attach('native-local-confirmed-stop', { contentType: 'application/json', body: JSON.stringify({ id, nonce, childPID: launched.pid, action, outcome: result.outcome }) });
  } finally {
    await disposeFixtureTerminal(request, id, info);
  }
});
