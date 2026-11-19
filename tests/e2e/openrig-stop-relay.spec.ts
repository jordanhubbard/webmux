import { test, expect } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import path from 'node:path';


test('checked local Rig Stop ends its actual SSH relay while the remote tmux owner and provider survive', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_OPENRIG !== '1' || !process.env.WEBMUX_RIG_RELAY_MANIFEST, 'Requires the owned Linux SSH/tmux fixture');
  test.setTimeout(90000);
  const remote = JSON.parse(readFileSync(process.env.WEBMUX_RIG_RELAY_MANIFEST!, 'utf8')) as {
    ssh: string[]; attach: string; probe: string; pid: string; executable: string; marker: string;
  };
  expect(remote.ssh[0]).toBe('/usr/bin/ssh');
  expect(remote.pid).toMatch(/^[1-9]\d*$/);
  expect(remote.executable).toBe('/usr/bin/sleep');
  expect(remote.marker).toMatch(/^WM_REMOTE_RELAY_[a-f0-9]{32}$/);
  const home = path.resolve(__dirname, '.test-home');
  const rig = path.join(home, 'openrig-bin/rig');
  const env = { ...process.env, OPENRIG_HOST_SELECTED: 'local', TMUX_TMPDIR: readFileSync(path.join(home, 'openrig-tmux-directory'), 'utf8') };
  const run = (binary: string, args: string[]) => {
    const result = spawnSync(binary, args, { env, encoding: 'utf8', timeout: 15000 });
    expect(result.status, result.stdout + result.stderr).toBe(0);
    return result.stdout.trim();
  };
  const remotePID = () => {
    const pid = run(remote.ssh[0], [...remote.ssh.slice(1), remote.probe]);
    expect(pid).toBe(remote.pid);
    run(remote.ssh[0], [...remote.ssh.slice(1), 'kill -0 ' + remote.pid]);
    expect(run(remote.ssh[0], [...remote.ssh.slice(1), 'readlink /proc/' + remote.pid + '/exe'])).toBe(remote.executable);
    return pid;
  };
  const version = run(rig, ['--version']);
  expect(version).toBe('0.7.18 (3bf87c62)');
  const project = `relay-${randomBytes(5).toString('hex')}`;
  const spec = path.join(home, project + '.json');
  writeFileSync(spec, JSON.stringify({ version: '0.2', name: project, pods: [{ id: 'agents', label: project,
    members: ['relay', 'sibling'].map(id => ({ id, label: id, runtime: 'terminal', agent_ref: 'builtin:terminal', profile: 'none', cwd: home })) }], edges: [] }));
  const { rigId } = JSON.parse(run(rig, ['up', spec, '--yes', '--json'])) as { rigId: string };
  type Seat = { id: string; projectId: string; logicalId: string; attachmentTarget: string; canReviewStop?: boolean };
  const observe = async () => (await (await request.get('/api/coordination?provider=rig')).json() as { seats: Seat[] }).seats.filter(seat => seat.projectId === rigId);
  const pane = (name: string) => {
    const rows = run('tmux', ['list-panes', '-a', '-F', '#{session_name}\t#{pane_pid}']).split('\n').map(row => row.split('\t'));
    const matches = rows.filter(row => row[0] === name);
    expect(matches.length).toBeLessThanOrEqual(1);
    if (!matches.length) return '';
    expect(matches[0][1]).toMatch(/^[1-9]\d*$/);
    process.kill(Number(matches[0][1]), 0);
    return matches[0][1];
  };
  const original = await (await request.get('/api/config')).json();
  let viewer = '';
  try {
    expect((await request.put('/api/config', { data: { app: { session_logging: { enabled: true } } } })).ok()).toBe(true);
    const relay = (await observe()).find(seat => seat.logicalId === 'agents.relay')!;
    const sibling = (await observe()).find(seat => seat.logicalId === 'agents.sibling')!;
    expect(relay.canReviewStop).toBe(true);
    const siblingPID = pane(sibling.attachmentTarget);
    expect(siblingPID).toBeTruthy();
    const opened = await request.post('/api/coordination/attach', { data: { provider: 'rig', target: relay.id, row: 1, col: 1 } });
    expect(opened.status(), await opened.text()).toBe(200);
    viewer = (await opened.json() as { id: string }).id;
    await page.goto('/');
    const tile = page.getByTestId(`tile-cell-${viewer}`);
    const quote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
    // Real owner input replaces only this managed terminal's shell with the
    // local SSH transport. The remote owner already runs its persistent pane.
    const transport = [...remote.ssh.slice(0, -1), '-tt', remote.ssh.at(-1)!, 'exec ' + remote.attach];
    await tile.locator('.xterm-helper-textarea').focus();
    await page.keyboard.insertText('exec ' + transport.map(quote).join(' '));
    await page.keyboard.press('Enter');
    await expect(tile.locator('.xterm-rows')).toContainText(remote.marker);
    const localPID = pane(relay.attachmentTarget);
    expect(localPID).toBeTruthy();
    expect(run('/bin/ps', ['-p', localPID, '-o', 'comm='])).toMatch(/(?:^|\/)ssh$/);
    const remoteBefore = remotePID();

    await page.getByRole('button', { name: 'Coordination', exact: true }).click();
    const panel = page.getByRole('region', { name: 'Coordination', exact: true });
    await panel.getByLabel('Show sessions').selectOption('rig');
    await panel.getByRole('article', { name: `agents.relay in ${project}` }).getByRole('button', { name: 'Review local seat Stop', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Stop local Rig seat', exact: true });
    await dialog.getByLabel('Reason for stopping').fill('Owned real SSH relay lifecycle qualification');
    await dialog.getByRole('button', { name: 'Review local Stop', exact: true }).click();
    await expect(dialog).toContainText('No remote provider stop is performed');
    const confirm = dialog.getByRole('button', { name: 'Confirm local seat Stop', exact: true });
    await expect(confirm).toBeDisabled();
    expect(remotePID()).toBe(remoteBefore);
    expect(pane(relay.attachmentTarget)).toBe(localPID);
    await dialog.getByRole('checkbox').check();
    const stopped = page.waitForResponse(reply => reply.request().method() === 'POST' && reply.url().endsWith('/api/coordination/stop'));
    await confirm.click();
    const response = await stopped;
    expect(response.status(), await response.text()).toBe(200);
    const receipt = await response.json() as { id: string; state: string };
    expect(receipt.state).toBe('stopped');
    await expect(dialog.getByRole('status')).toContainText('Rig confirmed that the reviewed local seat stopped');
    await expect.poll(() => pane(relay.attachmentTarget)).toBe('');
    await expect.poll(() => {
      try { process.kill(Number(localPID), 0); return true; }
      catch (error) {
        if ((error as NodeJS.ErrnoException).code === 'ESRCH') return false;
        throw error;
      }
    }).toBe(false);
    expect(pane(sibling.attachmentTarget)).toBe(siblingPID);
    expect(remotePID()).toBe(remoteBefore);
    const duplicate = await request.post('/api/coordination/stop', { data: { id: receipt.id, confirm: true } });
    expect(await duplicate.json()).toMatchObject({ id: receipt.id, state: 'stopped' });
    expect(remotePID()).toBe(remoteBefore);
    const retained = await (await request.get(`/api/sessions/${viewer}`)).json() as { id: string; row: number; col: number };
    expect([retained.id, retained.row, retained.col]).toEqual([viewer, 1, 1]);
    const history = await (await request.get(`/api/session-recovery/${viewer}`)).json() as { history: string };
    expect(history.history).toContain(remote.marker);
    writeFileSync(info.outputPath('checked-stop-relay.json'), JSON.stringify({ passed: true, rig: version, node: process.version,
      platform: process.platform, architecture: process.arch, browser: page.context().browser()?.version(),
      localPID, remotePID: remoteBefore, receipt, checks: { actualSSHTransportObserved: true, localRelayStopped: true,
        localSSHPIDExited: true, remotePersistentPIDUnchanged: true, remoteExecutableUnchanged: true,
        siblingUnchanged: true, duplicateConsumed: true, viewerLayoutAndHistoryRetained: true },
      scope: 'Actual native Rig/WebMux/browser/local SSH transport to isolated Linux SSH/tmux. Synthetic sleep provider; not authenticated coding recovery, physical-host failure or native WebMux launch conversion.' }));
  } finally {
    if (viewer) await request.delete(`/api/sessions/${viewer}`);
    expect((await request.put('/api/config', { data: { app: { session_logging: original.app.session_logging } } })).ok()).toBe(true);
    run(rig, ['down', rigId]);
  }
});
