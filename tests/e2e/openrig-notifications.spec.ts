import { test, expect } from '@playwright/test';
import type { Response } from '@playwright/test';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import path from 'node:path';

test.afterAll(() => {
  if (process.env.WEBMUX_REAL_OPENRIG !== '1') return;
  const rig = path.resolve(__dirname, '.test-home/openrig-bin/rig');
  if (existsSync(rig)) expect(spawnSync(rig, ['daemon', 'stop'], { timeout: 15000 }).status).toBe(0);
});

test('native Rig notification refreshes canonical tile state while fallback timers are paused', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_OPENRIG !== '1', 'Requires isolated published Rig and actual tmux');
  test.setTimeout(90000);
  const home = path.resolve(__dirname, '.test-home');
  const rig = path.join(home, 'openrig-bin/rig');
  const env = { ...process.env, OPENRIG_HOST_SELECTED: 'local', TMUX_TMPDIR: readFileSync(path.join(home, 'openrig-tmux-directory'), 'utf8') };
  const run = (binary: string, args: string[]) => {
    const result = spawnSync(binary, args, { env, encoding: 'utf8', timeout: 15000 });
    expect(result.status, result.stdout + result.stderr).toBe(0);
    return result.stdout.trim();
  };
  const name = `changes-${randomBytes(5).toString('hex')}`;
  const spec = path.join(home, `${name}.json`);
  writeFileSync(spec, JSON.stringify({ version: '0.2', name, pods: [{ id: 'agents', label: name,
    members: ['first', 'sibling'].map(id => ({ id, label: id, agent_ref: 'builtin:terminal', profile: 'none', runtime: 'terminal', cwd: home })) }], edges: [] }));
  const { rigId } = JSON.parse(run(rig, ['up', spec, '--yes', '--json'])) as { rigId: string };
  let viewer = '';
  const active = new Set<Response>();
  page.on('response', response => {
    if (!response.url().endsWith('/api/coordination/changes')) return;
    void response.headerValue('x-webmux-rig-changes').then(value => {
      if (value !== 'subscribed') return;
      active.add(response);
      void response.finished().then(() => active.delete(response));
    });
  });
  try {
    type Seat = { id: string; projectId: string; logicalId: string; attachmentTarget: string };
    const inventory = await (await request.get('/api/coordination?provider=rig')).json() as { seats: Seat[] };
    const seats = inventory.seats.filter(seat => seat.projectId === rigId);
    const first = seats.find(seat => seat.logicalId === 'agents.first')!;
    const sibling = seats.find(seat => seat.logicalId === 'agents.sibling')!;
    expect(first).toBeTruthy(); expect(sibling).toBeTruthy();
    const siblingPID = run('tmux', ['display-message', '-p', '-t', sibling.attachmentTarget, '#{pane_pid}']);
    const opened = await request.post('/api/coordination/attach', { data: { provider: 'rig', target: first.id, row: 1, col: 1 } });
    expect(opened.status()).toBe(200); viewer = (await opened.json() as { id: string }).id;
    await page.goto('/');
    const status = page.getByTestId(`tile-cell-${viewer}`).getByLabel('Rig seat status');
    await expect(status).toContainText('Rig running · Bootstrap ready');
    // The ready header is flushed only after the real daemon accepts a live-only
    // SSE subscription. Avoid a sleep or a request-start assumption.
    await expect.poll(() => active.size).toBeGreaterThan(0);
    const subscription = [...active].at(-1)!;
    await page.clock.install();
    await page.clock.pauseAt(new Date());
    run(rig, ['seat', 'stop', `agents.first@${name}`, '--reason', 'Owned notification qualification', '--operator', 'webmux-test-owner', '--json']);
    const invalidation = await subscription.json() as Record<string, unknown>;
    expect(invalidation).toEqual({ contract: 'webmux-rig-changes/v1', available: true, changed: true });
    await expect(status).toContainText('Rig detached', { timeout: 8000 });
    await expect(status).toContainText('Agent activity unknown');
    expect(run('tmux', ['display-message', '-p', '-t', sibling.attachmentTarget, '#{pane_pid}'])).toBe(siblingPID);
    const retained = await (await request.get(`/api/sessions/${viewer}`)).json() as { id: string; row: number; col: number };
    expect([retained.id, retained.row, retained.col]).toEqual([viewer, 1, 1]);
    await page.close();
    const base = new URL('/api/coordination/changes', opened.url()).href;
    let canceledSubscriptions = 0;
    for (let trial = 0; trial < 2; trial++) {
      await expect.poll(async () => {
        const controller = new AbortController();
        const deadline = setTimeout(() => controller.abort(), 3000);
        try {
          const response = await fetch(base, { signal: controller.signal });
          if (response.headers.get('x-webmux-rig-changes') !== 'subscribed') { await response.json(); return false; }
          controller.abort();
          await response.body?.cancel().catch(() => {});
          canceledSubscriptions++;
          return true;
        } finally { clearTimeout(deadline); controller.abort(); }
      }, { timeout: 5000, intervals: [50, 100, 200] }).toBe(true);
    }
    writeFileSync(info.outputPath('native-rig-notifications.json'), JSON.stringify({
      rigVersion: run(rig, ['--version']), tmuxVersion: run('tmux', ['-V']), nodeVersion: process.version,
      browserVersion: page.context().browser()?.version(), platform: process.platform, architecture: process.arch,
      invalidation, canceledSubscriptions, fallbackTimersPaused: true, canonicalDetachedObserved: true, siblingPIDUnchanged: true, viewerPreserved: true,
      scope: 'Actual published Rig daemon events through production helper/API/browser to canonical re-read. Builtin terminals only; no authenticated coding provider, remote notifications or Agent Deck bridge.' }, null, 2));
  } finally {
    await page.close();
    if (viewer) await request.delete(`/api/sessions/${viewer}`);
    run(rig, ['down', rigId]);
  }
});
