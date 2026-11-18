import { test, expect } from '@playwright/test';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import path from 'node:path';

test.afterAll(() => {
  if (process.env.WEBMUX_REAL_OPENRIG !== '1') return;
  const rig = path.resolve(__dirname, '.test-home/openrig-bin/rig');
  if (!existsSync(rig)) return;
  const result = spawnSync(rig, ['daemon', 'stop'], { timeout: 15000, encoding: 'utf8' });
  expect(result.status, result.stdout + result.stderr).toBe(0);
});

test('checked local Rig Stop refuses replaced and adopted occupants, then stops only the confirmed native seat', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_OPENRIG !== '1', 'Requires the isolated published Rig fixture');
  test.setTimeout(90000);
  const home = path.resolve(__dirname, '.test-home');
  const rig = path.join(home, 'openrig-bin/rig');
  const env = { ...process.env, OPENRIG_HOST_SELECTED: 'local', TMUX_TMPDIR: readFileSync(path.join(home, 'openrig-tmux-directory'), 'utf8') };
  const run = (binary: string, args: string[]) => {
    const value = spawnSync(binary, args, { env, encoding: 'utf8', timeout: 15000 });
    expect(value.status, value.stdout + value.stderr).toBe(0);
    return value.stdout.trim();
  };
  const version = run(rig, ['--version']);
  test.skip(version !== '0.7.18 (3bf87c62)', 'Checked Stop is admitted only for the published qualified build');
  const project = `stop-${randomBytes(5).toString('hex')}`;
  const spec = path.join(home, project + '.json');
  writeFileSync(spec, JSON.stringify({ version: '0.2', name: project, pods: [{ id: 'agents', label: project,
    members: ['first', 'sibling'].map(id => ({ id, label: id, runtime: 'terminal', agent_ref: 'builtin:terminal', profile: 'none', cwd: home })) }], edges: [] }));
  const { rigId } = JSON.parse(run(rig, ['up', spec, '--yes', '--json'])) as { rigId: string };
  type Seat = { id: string; projectId: string; logicalId: string; managerTarget: string; attachmentTarget: string; canReviewStop?: boolean; enrollmentOrigin?: string };
  const observe = async () => (await (await request.get('/api/coordination?provider=rig')).json() as { seats: Seat[] }).seats.filter(seat => seat.projectId === rigId);
  const pane = (name: string) => {
    const rows = run('tmux', ['list-panes', '-a', '-F', '#{session_name}\t#{pane_pid}']).split('\n').map(row => row.split('\t'));
    const candidates = rows.filter(row => row[0] === name);
    expect(candidates.length).toBeLessThanOrEqual(1);
    if (!candidates.length) return '';
    expect(candidates[0][1]).toMatch(/^[1-9]\d*$/);
    process.kill(Number(candidates[0][1]), 0);
    return candidates[0][1];
  };
  let viewer = '';
  const original = await (await request.get('/api/config')).json();
  try {
    expect((await request.put('/api/config', { data: { app: { session_logging: { enabled: true } } } })).ok()).toBe(true);
    const first = (await observe()).find(seat => seat.logicalId === 'agents.first')!;
    const sibling = (await observe()).find(seat => seat.logicalId === 'agents.sibling')!;
    expect(first.canReviewStop).toBe(true);
    const originalPID = pane(first.attachmentTarget), siblingPID = pane(sibling.attachmentTarget);
    expect(originalPID).toBeTruthy(); expect(siblingPID).toBeTruthy();
    const opened = await request.post('/api/coordination/attach', { data: { provider: 'rig', target: first.id, row: 1, col: 1 } });
    expect(opened.status(), await opened.text()).toBe(200);
    viewer = (await opened.json() as { id: string }).id;
    await page.goto('/');
    const tile = page.getByTestId(`tile-cell-${viewer}`);
    await tile.locator('.xterm-helper-textarea').focus();
    await page.keyboard.insertText("printf 'WM_BEFORE_LOCAL_STOP\\n'");
    await page.keyboard.press('Enter');
    await expect(tile.locator('.xterm-rows')).toContainText('WM_BEFORE_LOCAL_STOP');

    const reviewed = await request.post('/api/coordination/stop/preview', { data: { provider: 'rig', target: first.id, reason: 'Owned replacement race control' } });
    expect(reviewed.status(), await reviewed.text()).toBe(200);
    const old = await reviewed.json() as { id: string; sessionId: string };
    expect(pane(first.attachmentTarget)).toBe(originalPID);
    run(rig, ['seat', 'launch', `agents.first@${project}`, '--fresh', '--stop', '--reason', 'Owned native replacement after review', '--json']);
    const replacementPID = pane(first.attachmentTarget);
    expect(replacementPID).not.toBe(originalPID);
    const refused = await request.post('/api/coordination/stop', { data: { id: old.id, confirm: true } });
    expect(refused.status(), await refused.text()).toBe(200);
    expect(await refused.json()).toMatchObject({ id: old.id, state: 'refused', code: 'selection_changed', posted: false, stopPhase: 'none' });
    expect(pane(first.attachmentTarget)).toBe(replacementPID);
    expect(pane(sibling.attachmentTarget)).toBe(siblingPID);

    // Adopted processes have no native Stop authority, even though they have
    // canonical Rig identities. The API independently refuses a forged review.
    const adoptedTarget = `adopted-${randomBytes(4).toString('hex')}`;
    const launcher = path.join(home, adoptedTarget + '.sh');
    const quote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
    writeFileSync(launcher, `#!/bin/sh\n${quote(rig)} attach --self --rig ${quote(rigId)} --pod agents --member adopted --runtime terminal --cwd ${quote(home)} --json > ${quote(path.join(home, adoptedTarget + '.json'))} || exit 1\nexec /bin/sleep 600\n`, { mode: 0o700 });
    run('tmux', ['new-session', '-d', '-s', adoptedTarget, '/bin/sh', launcher]);
    await expect.poll(async () => (await observe()).find(seat => seat.logicalId === 'agents.adopted')?.enrollmentOrigin).toBe('claimed');
    const adopted = (await observe()).find(seat => seat.logicalId === 'agents.adopted')!;
    const adoptedPID = pane(adopted.attachmentTarget);
    expect(adoptedPID).toBeTruthy(); expect(adopted.canReviewStop).not.toBe(true);
    expect((await request.post('/api/coordination/stop/preview', { data: { provider: 'rig', target: adopted.id, reason: 'Must refuse adopted fixture' } })).status()).toBe(409);
    expect(pane(adopted.attachmentTarget)).toBe(adoptedPID);

    await page.getByRole('button', { name: 'Coordination', exact: true }).click();
    const panel = page.getByRole('region', { name: 'Coordination', exact: true });
    await panel.getByLabel('Show sessions').selectOption('rig');
    const article = panel.getByRole('article', { name: `agents.first in ${project}` });
    await article.getByRole('button', { name: 'Review local seat Stop', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Stop local Rig seat', exact: true });
    await dialog.getByLabel('Reason for stopping').fill('Owned native browser Stop qualification');
    await dialog.getByRole('button', { name: 'Review local Stop', exact: true }).click();
    const confirm = dialog.getByRole('button', { name: 'Confirm local seat Stop', exact: true });
    await expect(confirm).toBeDisabled();
    await expect(dialog).toContainText('No remote provider stop is performed');
    expect(pane(first.attachmentTarget)).toBe(replacementPID);
    await dialog.getByRole('checkbox').check();
    const response = page.waitForResponse(reply => reply.request().method() === 'POST' && reply.url().endsWith('/api/coordination/stop'));
    await confirm.click();
    const reply = await response;
    expect(reply.status(), await reply.text()).toBe(200);
    const receipt = await reply.json() as { id: string; state: string };
    expect(receipt.state).toBe('stopped');
    await expect(dialog.getByRole('status')).toContainText('Rig confirmed that the reviewed local seat stopped');
    await expect.poll(() => pane(first.attachmentTarget)).toBe('');
    expect(pane(sibling.attachmentTarget)).toBe(siblingPID);
    expect(pane(adopted.attachmentTarget)).toBe(adoptedPID);
    const duplicate = await request.post('/api/coordination/stop', { data: { id: receipt.id, confirm: true } });
    expect(await duplicate.json()).toMatchObject({ id: receipt.id, state: 'stopped' });
    const retained = await (await request.get(`/api/sessions/${viewer}`)).json() as { id: string; row: number; col: number };
    expect([retained.id, retained.row, retained.col]).toEqual([viewer, 1, 1]);
    const recovery = await (await request.get(`/api/session-recovery/${viewer}`)).json() as { history: string };
    expect(recovery.history).toContain('WM_BEFORE_LOCAL_STOP');
    await page.screenshot({ path: info.outputPath('checked-stop.png') });
    const proof = info.outputPath('checked-stop.json');
    writeFileSync(proof, JSON.stringify({ rig: version, node: process.version, tmux: run('tmux', ['-V']), browser: page.context().browser()?.version(), rigId, viewer, originalPID, replacementPID, siblingPID, adoptedPID, staleReviewRefused: true, adoptedReviewRefused: true, confirmedSeatStopped: true, siblingAndAdoptedSurvive: true, retainedLayoutAndHistory: true, duplicateConsumed: true, receipt, scope: 'Actual private native Rig/tmux/browser. Synthetic terminal/sleep fixtures only; not authenticated coding, remote SSH relay or managed launch conversion acceptance.' }));
    await info.attach('checked local stop', { path: proof, contentType: 'application/json' });
    run('tmux', ['kill-session', '-t', '=' + adoptedTarget]);
  } finally {
    if (viewer) await request.delete(`/api/sessions/${viewer}`);
    expect((await request.put('/api/config', { data: { app: { session_logging: original.app.session_logging } } })).ok()).toBe(true);
    run(rig, ['down', rigId]);
  }
});
