import { test, expect } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import path from 'node:path';


test('grid observes native Rig bootstrap and lifecycle without inferring provider readiness', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_OPENRIG !== '1', 'Requires isolated published Rig and actual tmux');
  test.setTimeout(90000);
  const home = path.resolve(__dirname, '.test-home');
  const rig = path.join(home, 'openrig-bin/rig');
  const project = `activity-${randomBytes(5).toString('hex')}`;
  const env = { ...process.env, OPENRIG_HOST_SELECTED: 'local', TMUX_TMPDIR: readFileSync(path.join(home, 'openrig-tmux-directory'), 'utf8') };
  const run = (binary: string, args: string[]) => {
    const result = spawnSync(binary, args, { env, encoding: 'utf8', timeout: 15000 });
    expect(result.status, result.stdout + result.stderr).toBe(0);
    return result.stdout.trim();
  };
  const spec = path.join(home, `${project}.json`);
  writeFileSync(spec, JSON.stringify({ version: '0.2', name: project, pods: [{ id: 'agents', label: project,
    members: ['first', 'sibling'].map(id => ({ id, label: id, agent_ref: 'builtin:terminal', profile: 'none', runtime: 'terminal', cwd: home })) }], edges: [] }));
  const { rigId } = JSON.parse(run(rig, ['up', spec, '--yes', '--json'])) as { rigId: string };
  type Seat = { id: string; logicalId: string; projectId: string; managerTarget: string; attachmentTarget: string; startupStatus: string; state: string; managed: boolean; enrollmentOrigin: string; enrollmentSource: string; agentActivity?: Record<string, unknown> };
  const observe = async () => {
    const status = await (await request.get('/api/coordination?provider=rig')).json() as { seats: Seat[] };
    expect(status.seats.some(s => s.projectId === rigId), JSON.stringify(status).slice(0, 6000)).toBe(true);
    return status.seats.filter(s => s.projectId === rigId);
  };
  let viewer = '';
  try {
    const seats = await observe();
    const first = seats.find(s => s.logicalId === 'agents.first')!;
    const sibling = seats.find(s => s.logicalId === 'agents.sibling')!;
    expect(first).toBeTruthy(); expect(sibling).toBeTruthy();
    const nativeInventory = JSON.parse(run(rig, ['ps', '--nodes', '-A', '--json', '--full'])) as Record<string, unknown>[] | { entries: Record<string, unknown>[] };
    const attachmentFacts = (Array.isArray(nativeInventory) ? nativeInventory : nativeInventory.entries).filter(row => row.rigId === rigId).map(row => ({
      canonicalSessionName: row.canonicalSessionName, attachmentType: row.attachmentType,
      tmuxAttachCommand: row.tmuxAttachCommand, tmuxServerArgs: row.tmuxServerArgs,
    }));
    expect((first as Seat & { openable?: boolean }).openable, JSON.stringify(attachmentFacts)).toBe(true);
    expect(first.startupStatus).toBe('ready');
    expect(first.managed).toBe(false); expect(first.enrollmentOrigin).toBe('launched'); expect(first.enrollmentSource).toBe('rig-session-registry');
    expect(first.agentActivity).toBeTruthy();
    expect(Object.keys(first.agentActivity!).every(key => ['state', 'evidenceSource', 'sampledAt', 'stale', 'generationProvenance'].includes(key))).toBe(true);
    const siblingPID = run('tmux', ['display-message', '-p', '-t', sibling.attachmentTarget, '#{pane_pid}']);
    const response = await request.post('/api/coordination/attach', { data: { provider: 'rig', target: first.id, row: 1, col: 1 } });
    expect(response.status(), (await response.text()).slice(0, 6000)).toBe(200);
    const opened = await response.json() as { id: string }; viewer = opened.id;
    await page.goto('/');
    const tile = page.getByTestId(`tile-cell-${viewer}`);
    const status = tile.getByLabel('Rig seat status');
    await expect(status).toContainText('Rig running · Attachment unknown · Bootstrap ready');
    await expect(status).toContainText('Enrollment Launched by Rig');
    await expect(status).not.toContainText('Agent running');
    await expect(status).not.toContainText('Agent ready');
    await tile.locator('.xterm-helper-textarea').focus();
    await page.keyboard.type("printf 'NATIVE_ACTIVITY_MARKER\\n'", { delay: 5 });
    await page.keyboard.press('Enter');
    await expect(tile.locator('.xterm-rows')).toContainText('NATIVE_ACTIVITY_MARKER');
    await expect(status).not.toContainText('Agent ready');
    await tile.screenshot({ path: info.outputPath('rig-tile-status.png') });
    run(rig, ['seat', 'stop', `agents.first@${project}`, '--reason', 'Owned native activity qualification', '--operator', 'webmux-test-owner', '--json']);
    await expect(status).toContainText('Rig detached', { timeout: 20000 });
    await expect(status).toContainText('Agent activity unknown');
    const stopped = (await observe()).find(s => s.id === first.id)!;
    expect(stopped.state).toBe('detached');
    run('tmux', ['has-session', '-t', sibling.attachmentTarget]);
    expect(run('tmux', ['display-message', '-p', '-t', sibling.attachmentTarget, '#{pane_pid}'])).toBe(siblingPID);
    const finalViewer = await (await request.get(`/api/sessions/${viewer}`)).json() as { id: string; row: number; col: number };
    expect([finalViewer.id, finalViewer.row, finalViewer.col]).toEqual([viewer, 1, 1]);
    writeFileSync(info.outputPath('native-rig-activity.json'), JSON.stringify({ rigVersion: run(rig, ['--version']), tmuxVersion: run('tmux', ['-V']), nodeVersion: process.version,
      browserVersion: page.context().browser()?.version(), platform: process.platform, architecture: process.arch,
      before: first, after: stopped, siblingPIDUnchanged: true, viewerPreserved: true,
      scope: 'Real Rig-managed builtin terminals and tmux through native app/browser. No authenticated coding provider or hook activity claimed.' }, null, 2));
  } finally {
    if (viewer) await request.delete(`/api/sessions/${viewer}`);
    run(rig, ['down', rigId]);
  }
});
