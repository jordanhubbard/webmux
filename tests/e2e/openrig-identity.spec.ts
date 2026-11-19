import { test, expect } from '@playwright/test';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import path from 'node:path';


test('logical Rig seat keeps its viewer and layout after an explicit native rebind', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_OPENRIG !== '1', 'Requires isolated published Rig and tmux');
  test.setTimeout(90_000);
  const home = path.resolve(__dirname, '.test-home');
  const rig = path.join(home, 'openrig-bin/rig');
  const suffix = randomBytes(5).toString('hex');
  const project = `identity-${suffix}`;
  const targets = [`wm-first-${suffix}`, `wm-second-${suffix}`];
  const env = { ...process.env, OPENRIG_HOST_SELECTED: 'local', TMUX_TMPDIR: readFileSync(path.join(home, 'openrig-tmux-directory'), 'utf8') };
  const run = (binary: string, args: string[]) => {
    const result = spawnSync(binary, args, { env, encoding: 'utf8', timeout: 15000 });
    expect(result.status, result.stdout + result.stderr).toBe(0);
    return result.stdout.trim();
  };
  const q = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
  const spec = path.join(home, `${project}.json`);
  writeFileSync(spec, JSON.stringify({ version: '0.2', name: project, pods: [{ id: 'agents', label: project, members: [] }], edges: [] }));
  const { rigId } = JSON.parse(run(rig, ['up', spec, '--yes', '--json'])) as { rigId: string };
  type Enrollment = { sessionId: string; nodeId: string; logicalId: string };
  const enroll = async (index: number) => {
    const target = targets[index];
    const file = path.join(home, `${target}.attached.json`);
    const starts = path.join(home, `${target}.starts`);
    const input = path.join(home, `${target}.input`);
    const child = path.join(home, `${target}.cjs`);
    const launcher = path.join(home, `${target}.sh`);
    const label = index === 0 ? 'FIRST' : 'SECOND';
    writeFileSync(child, `const fs=require('fs');fs.appendFileSync(${JSON.stringify(starts)},process.pid+';');process.stdin.setRawMode(true);process.stdin.resume();process.stdout.write('${label}_READY:'+process.pid+';');process.stdin.on('data',d=>{fs.appendFileSync(${JSON.stringify(input)},d.toString('hex')+';');process.stdout.write('${label}_INPUT:'+d.toString('hex')+';');});`);
    const binding = index === 0 ? ['--pod', 'agents', '--member', 'external'] : ['--node', 'agents.external'];
    const args = ['attach', '--self', '--rig', rigId, ...binding, '--runtime', 'codex', '--cwd', home, '--json'];
    writeFileSync(launcher, `#!/bin/sh\n${q(rig)} ${args.map(q).join(' ')} > ${q(file)} || exit 1\nexec ${q(process.execPath)} ${q(child)}\n`, { mode: 0o700 });
    run('tmux', ['new-session', '-d', '-s', target, '/bin/sh', launcher]);
    await expect.poll(() => existsSync(starts)).toBe(true);
    const pids = readFileSync(starts, 'utf8').split(';').filter(Boolean);
    expect(pids).toHaveLength(1);
    expect(run('tmux', ['display-message', '-p', '-t', target, '#{pane_pid}'])).toBe(pids[0]);
    return { enrollment: JSON.parse(readFileSync(file, 'utf8')) as Enrollment, pid: pids[0], starts, input };
  };
  type Seat = { id: string; projectId: string; name: string; managerTarget?: string; attachmentTarget?: string };
  type Viewer = { id: string; row: number; col: number; rig: { target: string; seat_id?: string; project_id?: string } };
  const observe = async () => (await (await request.get('/api/coordination?provider=rig')).json() as { seats: Seat[] }).seats.find(s => s.projectId === rigId && s.name === 'agents.external');
  const open = async () => {
    await page.getByRole('button', { name: 'Coordination', exact: true }).click();
    const panel = page.getByRole('region', { name: 'Coordination', exact: true });
    await panel.getByLabel('Show sessions').selectOption('rig');
    const response = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/api/coordination/attach'));
    await panel.getByRole('article', { name: `agents.external in ${project}` }).getByRole('button', { name: 'Open session', exact: true }).click();
    const reply = await response;
    expect(reply.status()).toBe(200);
    return await reply.json() as Viewer;
  };
  let viewer = '';
  const originalConfig = await (await request.get('/api/config')).json();
  try {
    expect((await request.put('/api/config', { data: { app: { session_logging: { enabled: true } } } })).ok()).toBe(true);
    const first = await enroll(0);
    await expect.poll(async () => (await observe())?.id).toBeTruthy();
    const original = (await observe())!;
    await page.goto('/');
    const firstView = await open(); viewer = firstView.id;
    expect(firstView.rig.project_id).toBe(rigId);
    const tile = page.getByTestId(`tile-cell-${viewer}`);
    await expect(tile.locator('.xterm-rows')).toContainText(`FIRST_READY:${first.pid};`);
    expect((await request.patch(`/api/sessions/${viewer}`, { data: { row: 1, col: 1 } })).ok()).toBe(true);
    await tile.locator('.xterm-helper-textarea').focus();
    await page.keyboard.insertText('before-rebind');
    await expect(tile.locator('.xterm-rows')).toContainText(`FIRST_INPUT:${Buffer.from('before-rebind').toString('hex')};`);

    // Only the external owner changes provider binding. WebMux receives no
    // launch/recovery request, and must not stop the old unclaimed provider.
    run(rig, ['unclaim', first.enrollment.sessionId]);
    const second = await enroll(1);
    expect(second.enrollment.nodeId).toBe(first.enrollment.nodeId);
    expect(second.enrollment.logicalId).toBe(first.enrollment.logicalId);
    expect(second.enrollment.sessionId).not.toBe(first.enrollment.sessionId);
    await expect.poll(async () => (await observe())?.id).toBe(original.id);
    const current = (await observe())!;
    expect(current.id).toMatch(/^local-rig\/[a-f0-9]{64}$/);
    expect(current.attachmentTarget).toBe(targets[1]);
    expect(current.managerTarget).toBe(targets[1]);
    const beforeOpen = await (await request.get(`/api/sessions/${viewer}`)).json() as Viewer;
    expect(beforeOpen.rig.target).toBe(targets[0]);
    expect(run('tmux', ['display-message', '-p', '-t', targets[0], '#{session_attached}'])).toBe('1');
    // Transport reconnect must not silently follow a changed native binding.
    const refused = await request.post(`/api/sessions/${viewer}/reconnect`, { data: {} });
    expect(refused.status()).toBe(409);
    expect((await (await request.get(`/api/sessions/${viewer}`)).json() as Viewer).rig.target).toBe(targets[0]);
    expect((await request.patch(`/api/sessions/${viewer}`, { data: { minimized: true } })).ok()).toBe(true);
    await page.reload();
    const reopened = await open();
    expect(reopened.id).toBe(viewer);
    expect([reopened.row, reopened.col]).toEqual([1, 1]);
    expect(reopened.rig.seat_id).toBe(original.id);
    expect(reopened.rig.target).toBe(targets[1]);
    await expect(tile.locator('.xterm-rows')).toContainText(`SECOND_READY:${second.pid};`);
    await tile.locator('.xterm-helper-textarea').focus();
    await page.keyboard.insertText('after-rebind');
    await expect(tile.locator('.xterm-rows')).toContainText(`SECOND_INPUT:${Buffer.from('after-rebind').toString('hex')};`);
    expect(readFileSync(first.input, 'utf8')).not.toContain(Buffer.from('after-rebind').toString('hex'));
    expect(run('tmux', ['display-message', '-p', '-t', targets[0], '#{pane_pid}'])).toBe(first.pid);
    expect(run('tmux', ['display-message', '-p', '-t', targets[0], '#{session_attached}'])).toBe('0');
    expect(run('tmux', ['display-message', '-p', '-t', targets[1], '#{session_attached}'])).toBe('1');
    for (const value of [first, second]) expect(readFileSync(value.starts, 'utf8').split(';').filter(Boolean)).toEqual([value.pid]);
    const recovery = await (await request.get(`/api/session-recovery/${viewer}`)).json();
    expect(recovery.history).toContain('FIRST_READY:');
    expect(recovery.can_resume_managed).toBe(false);
    await page.reload();
    await expect(tile.locator('.xterm-rows')).toContainText(`SECOND_READY:${second.pid};`);
    expect((await open()).id).toBe(viewer);
    const artifact = info.outputPath('canonical-rig-seat.json');
    writeFileSync(artifact, JSON.stringify({ platform: process.platform, architecture: process.arch, node: process.version, rig: run(rig, ['--version']), tmux: run('tmux', ['-V']), browser: page.context().browser()?.version(), rigId, logicalId: first.enrollment.logicalId, nodeId: first.enrollment.nodeId, originalSeat: original.id, finalSeat: current.id, viewer, layout: [1, 1], firstPID: first.pid, secondPID: second.pid, firstSession: first.enrollment.sessionId, secondSession: second.enrollment.sessionId, firstTarget: targets[0], secondTarget: targets[1], providerStarts: [1, 1], firstProviderPreserved: true, inputRoutedToCurrentProvider: true, retainedHistory: true, scope: 'External owner explicitly unclaims and attaches two synthetic Node terminals. No authenticated coding-conversation or automatic recovery/adoption claim.' }));
    await info.attach('canonical Rig seat identity', { path: artifact, contentType: 'application/json' });
  } finally {
    if (viewer) await request.delete(`/api/sessions/${viewer}`);
    expect((await request.put('/api/config', { data: { app: { session_logging: originalConfig.app.session_logging } } })).ok()).toBe(true);
    run(rig, ['down', rigId]);
    for (const target of targets) {
      const remains = spawnSync('tmux', ['has-session', '-t', `=${target}`], { env, timeout: 5000 });
      if (remains.status === 0) run('tmux', ['kill-session', '-t', `=${target}`]);
    }
  }
});
