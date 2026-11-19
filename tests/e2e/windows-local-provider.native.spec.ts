import { test, expect } from '@playwright/test';
import { randomBytes, randomUUID } from 'node:crypto';
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
import { disposeFixtureTerminal } from './terminal-lifecycle.mts';

test('Windows application connects a real local shell and confirms owned Stop', async ({ page, request }, info) => {
  test.skip(process.platform !== 'win32', 'Requires the actual Windows application provider');
  const computer = process.env.COMPUTERNAME;
  expect(computer).toBeTruthy();
  const created = await request.post('/api/sessions', { data: { transport: 'local' } });
  expect(created.status()).toBe(201);
  const { id } = await created.json() as { id: string };
  let deleted = false;
  try {
    await expect.poll(async () => {
      const response = await request.get(`/api/sessions/${id}`);
      expect(response.status()).toBe(200);
      return (await response.json() as { state: string }).state;
    }).toBe('connected');
    await page.goto('/');
    const tile = page.getByTestId(`tile-cell-${id}`);
    const input = tile.locator('.xterm-helper-textarea');
    await input.focus();
    const nonce = randomUUID().replaceAll('-', '');
    const marker = `WM_NATIVE_${computer}_${nonce}`;
    const command = `cmd.exe /d /c "echo WM_NATIVE_%COMPUTERNAME%_${nonce}"`;
    // Input echo cannot satisfy this oracle: only the executing shell expands
    // the machine-name reference into the complete, freshly chosen marker.
    expect(command).not.toContain(marker);
    await page.keyboard.type(command);
    await page.keyboard.press('Enter');
    await expect(tile.locator('.xterm-rows')).toContainText(marker);

    const require = createRequire(path.resolve(__dirname, '../../webmux/package.json'));
    const yaml = require('js-yaml') as { load(value: string): unknown };
    const saved = yaml.load(fs.readFileSync(path.join(__dirname, '.test-home/data/sessions/sessions.yaml'), 'utf8')) as {
      sessions: { id: string; local_owner?: { owner_pid: number; root_pid: number } }[];
    };
    const owner = saved.sessions.find(session => session.id === id)?.local_owner;
    expect(owner).toBeTruthy();
    const pids = [owner!.owner_pid, owner!.root_pid];
    for (const pid of pids) { expect(Number.isSafeInteger(pid) && pid > 0).toBe(true); process.kill(pid, 0); }

    const reviewed = await request.get(`/api/sessions/${id}`);
    expect(reviewed.status()).toBe(200);
    const lifecycle = (await reviewed.json() as {
      local_lifecycle: { contract: string; state: string; selection: string; operation_id?: string };
    }).local_lifecycle;
    expect(lifecycle.contract).toBe('webmux-local-provider-lifecycle/v1');
    expect(lifecycle.state).toBe('running');
    expect(lifecycle.operation_id).toBeFalsy();
    const operation = { id: randomUUID(), selection: lifecycle.selection, mode: 'stop', confirm: true };
    const stopped = await request.post(`/api/sessions/${id}/local-stop`, { data: operation });
    expect(stopped.status()).toBe(200);
    const result = await stopped.json() as { outcome: string; session: { id: string; local_lifecycle: { operation_id: string; selection: string } } };
    expect(result.outcome).toBe('stopped');
    expect(result.session.id).toBe(id);
    expect(result.session.local_lifecycle.operation_id).toBe(operation.id);
    expect(result.session.local_lifecycle.selection).toBe(operation.selection);
    await expect.poll(() => pids.every(pid => {
      try { process.kill(pid, 0); return false; }
      catch (error) { return (error as NodeJS.ErrnoException).code === 'ESRCH'; }
    })).toBe(true);
    expect((await request.delete(`/api/sessions/${id}`)).status()).toBe(204);
    deleted = true;
    expect((await request.get(`/api/sessions/${id}`)).status()).toBe(404);
    await info.attach('native-windows-application-provider', {
      contentType: 'application/json',
      body: JSON.stringify({ id, freshShellOutput: true, markerAbsentFromInput: true, originalStop: operation.id, ownerAndRootGone: true, deletedAfterStop: true }),
    });
  } finally {
    if (!deleted) await disposeFixtureTerminal(request, id, info);
  }
});

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
