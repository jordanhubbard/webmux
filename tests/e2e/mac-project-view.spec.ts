import { test, expect } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

test('project activity exposes incomplete observations and clears them on access refusal without restarting the provider', async ({ page, request }, info) => {
  const starts = info.outputPath('provider-starts.jsonl');
  fs.mkdirSync(path.dirname(starts), { recursive: true });
  let id: string | undefined;
  let reads = 0;
  let refused = false;
  try {
    const response = await request.post('/api/sessions', { data: { hostname: 'localhost', transport: 'exec', username: 'fixture', exec_command: `"${process.execPath}" "${path.resolve(__dirname, 'federation-fixture.mts')}" "${starts}" local` } });
    expect(response.ok()).toBe(true);
    const created = await response.json(); id = created.id;
    await expect.poll(() => fs.existsSync(starts) ? fs.readFileSync(starts, 'utf8').trim().split('\n').filter(Boolean).length : 0).toBe(1);
    const provider = fs.readFileSync(starts, 'utf8');
    // The project DTO is controlled here; real receiving-account, capability,
    // MAC HTTP auth and admission boundaries are exercised in Go integration.
    await page.route(`**/api/session-coordination/projects/${id}?*`, async route => {
      reads++;
      if (refused) { await route.fulfill({ status: 503, json: { error: 'MAC project observation is unavailable' } }); return; }
      await route.fulfill({ json: {
        schema: 'webmux.mac.session-project-view.v1', subject: { node_id: 'node', owner: 'fixture', session_id: id, incarnation: 'run' }, standalone_history_state: 'retained_unreconciled',
        source_state: 'observed', source_task_link_state: 'unlinked', source: { schema: 'webmux.mac.source-observation.v1', worktree: '/controlled/project', ref: 'refs/heads/review', commit: 'a'.repeat(40), detached: false, dirty: true, observed_at: '2026-11-23T07:00:00Z' },
        roadmap_state: 'observed_committed_snapshot', roadmap: { schema: 'webmux.mac.source-roadmap-observation.v1', path: 'docs/roadmap.md', commit: 'a'.repeat(40), blob: 'b'.repeat(40), sha256: 'c'.repeat(64), text: '# Source intent\n<script>execute()</script>', working_copy_dirty: true, task_link_state: 'unlinked', observed_at: '2026-11-23T07:00:00Z' },
        project: { schema: 'webmux.mac.project-observation.v1', project_key: 'Controlled project', observed_at: '2026-11-23T07:00:00Z', coverage: 'incomplete', reported_total: 12, next_offset: null,
          conflict_verdict: 'unknown', ordering_verdict: 'unknown', unavailable_tasks: 1, unavailable_workers: 0, adapter_omitted_tasks: 0, upstream_omitted_tasks: 0, withheld_dependencies: 1,
          tasks: [{ task: { task_id: 'controlled-task', title: '<img src=x onerror="execute()">', state: 'running', priority: 10, owner_worker_id: 'historical-owner', attempt_count: 2, lease_id: 'historical-lease', authority_time: '2026-11-23T07:00:00Z', updated_at: '2026-11-22T04:00:00Z', observed_at: '2026-11-23T07:00:00Z', lease_observation: 'observed_expired', leased_until: null }, worker: null, worker_state: 'unavailable' }], dependencies: [] },
        sessions: [{ session: { node_id: 'node', owner: 'fixture', session_id: id, incarnation: 'run' }, execution_host: 'puck.local', transport: 'exec', availability: 'revoked', broker_active: false, mac_link_state: 'unlinked', observed_at: '2026-11-23T06:00:00Z', projected_at: '2026-11-23T07:00:00Z', revoked_at: '2026-11-23T06:30:00Z', freshness: 'unqualified' }],
      } });
    });
    await page.goto('/');
    const project = page.getByRole('button', { name: 'Project activity', exact: true }).last();
    await expect(project).toBeVisible();
    expect(reads).toBe(0);
    await project.click();
    const dialog = page.getByRole('dialog', { name: 'Project activity', exact: true });
    await expect(dialog.getByText('Controlled project', { exact: true })).toBeVisible();
    await expect(dialog.getByRole('cell', { name: '<img src=x onerror="execute()"> controlled-task', exact: true })).toBeVisible();
    await expect(dialog.locator('img')).toHaveCount(0);
    await expect(dialog.getByText(/Coverage: incomplete/)).toBeVisible();
    await expect(dialog.getByText(/Owner: historical-owner/)).toBeVisible();
    await expect(dialog.getByText(/Lease ID: historical-lease/)).toBeVisible();
    await expect(dialog.getByText(/puck.local.*revoked.*MAC task link: unlinked/)).toBeVisible();
    await dialog.getByText('Session identity and observation', { exact: true }).click();
    await expect(dialog.getByText(/Node: node.*Account: fixture.*Incarnation: run/)).toBeVisible();
    await expect(dialog.getByText(/Revoked: 2026-11-23T06:30:00Z/)).toBeVisible();
    await expect(dialog.getByRole('heading', { name: 'Are we working in roadmap order?' })).toBeVisible();
    await expect(dialog.getByText(/Assessment: unknown. The source roadmap/)).toBeVisible();
    await expect(dialog.getByText(/Worktree: \/controlled\/project/)).toBeVisible();
    await expect(dialog.getByText(/not linked to a MAC task or roadmap revision/)).toBeVisible();
    await dialog.getByText('Read committed source roadmap', { exact: true }).click();
    await expect(dialog.getByLabel('Source roadmap text')).toContainText('<script>execute()</script>');
    await expect(dialog.locator('script')).toHaveCount(0);
    await page.screenshot({ path: info.outputPath('mac-project-view.png'), fullPage: true });
    refused = true;
    await dialog.getByRole('button', { name: 'Refresh project' }).click();
    await expect(dialog.getByRole('alert')).toHaveText('MAC project observation is unavailable');
    await expect(dialog.getByText('controlled-task', { exact: true })).toHaveCount(0);
    await expect(dialog.getByText('Controlled project', { exact: true })).toHaveCount(0);
    await expect(dialog.getByText(/Worktree: \/controlled\/project/)).toHaveCount(0);
    await expect(dialog.getByLabel('Source roadmap text')).toHaveCount(0);
    await expect(dialog.getByText(/Owner: historical-owner/)).toHaveCount(0);
    await expect(dialog.getByText(/Revoked: 2026-11-23T06:30:00Z/)).toHaveCount(0);
    const live = await request.get(`/api/sessions/${id}`).then(result => result.json());
    expect(live.created_at).toBe(created.created_at); expect(live.state).toBe('connected');
    expect(fs.readFileSync(starts, 'utf8')).toBe(provider);
  } finally {
    if (id) expect((await request.delete(`/api/sessions/${id}`)).ok()).toBe(true);
  }
});
