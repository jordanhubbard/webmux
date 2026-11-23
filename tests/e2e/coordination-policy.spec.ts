import { test, expect } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

test('server-wide coordination policy survives browser reload without restarting an existing provider', async ({ page, request }, info) => {
  const original = await request.get('/api/session-coordination/policy');
  expect(original.ok()).toBe(true);
  const enabled = (await original.json()).enabled as boolean;
  const starts = info.outputPath('provider-starts.jsonl');
  fs.mkdirSync(path.dirname(starts), { recursive: true });
  let id: string | undefined;
  const records = () => fs.existsSync(starts) ? fs.readFileSync(starts, 'utf8').trim().split('\n').filter(Boolean).map(line => JSON.parse(line) as { pid: number; instance: string }) : [];
  try {
    expect((await request.put('/api/session-coordination/policy', { data: { enabled: true } })).ok()).toBe(true);
    const result = await request.post('/api/sessions', { data: { hostname: 'localhost', transport: 'exec', username: 'fixture',
      exec_command: `"${process.execPath}" "${path.resolve(__dirname, 'federation-fixture.mts')}" "${starts}" local` } });
    expect(result.ok()).toBe(true);
    const created = await result.json(); id = created.id;
    await expect.poll(() => records().length).toBe(1);
    const provider = records()[0];
    await page.goto('/');
    const open = async () => {
      await page.getByRole('group', { name: 'WebMux cluster' }).getByRole('button', { name: 'Manage servers', exact: true }).click();
      return page.getByRole('region', { name: 'Automatic session coordination', exact: true });
    };
    let policy = await open();
    let toggle = policy.getByRole('checkbox', { name: 'Enable automatic session coordination' });
    await expect(toggle).toBeChecked(); await expect(toggle).toBeEnabled();
    // This controlled checkbox changes only after authoritative readback.
    await toggle.click();
    await expect(policy.getByRole('status')).toContainText('disabled on this server');
    await expect(toggle).not.toBeChecked();
    expect((await request.get('/api/session-coordination/policy').then(r => r.json())).enabled).toBe(false);
    await page.reload(); policy = await open();
    toggle = policy.getByRole('checkbox', { name: 'Enable automatic session coordination' });
    await expect(toggle).not.toBeChecked(); await expect(toggle).toBeEnabled();
    await toggle.click();
    await expect(policy.getByRole('status')).toContainText('enabled on this server');
    await expect(toggle).toBeChecked();
    await page.reload(); policy = await open();
    await expect(policy.getByRole('checkbox', { name: 'Enable automatic session coordination' })).toBeChecked();
    const live = await request.get(`/api/sessions/${id}`).then(r => r.json());
    expect(live.id).toBe(created.id); expect(live.created_at).toBe(created.created_at); expect(live.state).toBe('connected');
    expect(records()).toEqual([provider]);
    expect(() => process.kill(provider.pid, 0)).not.toThrow();
    await page.screenshot({ path: info.outputPath('coordination-policy.png'), fullPage: true });
  } finally {
    try {
      expect((await request.put('/api/session-coordination/policy', { data: { enabled } })).ok()).toBe(true);
    } finally {
      if (id) expect((await request.delete(`/api/sessions/${id}`)).ok()).toBe(true);
    }
  }
});
