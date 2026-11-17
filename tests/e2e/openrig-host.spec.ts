import { test, expect } from '@playwright/test';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { randomBytes } from 'node:crypto';
import { spawnSync } from 'node:child_process';

// Own a fresh server/request budget, as other native browser files do.
test.afterAll(() => {
  if (process.env.WEBMUX_REAL_OPENRIG !== '1') return;
  const executable = path.resolve(__dirname, '.test-home', 'openrig-bin', 'rig');
  if (!existsSync(executable)) return;
  // Stop before Playwright terminates the web-server process group. Waiting for
  // the server's exit handler alone can leave the detached Rig daemon behind.
  const stopped = spawnSync(executable, ['daemon', 'stop'], { timeout: 15000, encoding: 'utf8' });
  expect(stopped.status, stopped.stdout + stopped.stderr).toBe(0);
});

test('an external Rig seat keeps its execution host unknown across viewer recovery', async ({ page, request }, info) => {
  test.skip(process.env.WEBMUX_REAL_OPENRIG !== '1', 'Requires the isolated published-Rig/tmux fixture');
  test.setTimeout(90_000);
  const home = path.resolve(__dirname, '.test-home');
  const rig = path.join(home, 'openrig-bin', 'rig');
  const suffix = randomBytes(5).toString('hex');
  const project = `external-host-${suffix}`;
  const target = `wm-external-${suffix}`;
  // Direct fixture enrollment targets its private local daemon. Keep the
  // hostile inherited selection on the app server to verify controller routing.
  const tmuxEnv = { ...process.env, OPENRIG_HOST_SELECTED: 'local', TMUX_TMPDIR: readFileSync(path.join(home, 'openrig-tmux-directory'), 'utf8') };
  const run = (binary: string, args: string[]) => {
    const result = spawnSync(binary, args, { env: tmuxEnv, encoding: 'utf8', timeout: 15_000 });
    expect(result.status, result.stdout + result.stderr).toBe(0);
    return result.stdout.trim();
  };
  const q = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
  const spec = path.join(home, `${project}.json`);
  writeFileSync(spec, JSON.stringify({ version: '0.2', name: project, pods: [{ id: 'agents', label: project, members: [] }], edges: [] }));
  // Enrollment comes from Rig itself, not WebMux's private launch registry.
  // The provider is an owned synthetic Node terminal, not an authenticated CLI.
  const { rigId } = JSON.parse(run(rig, ['up', spec, '--yes', '--json'])) as { rigId: string };
  const enrolled = path.join(home, `${target}.attached.json`);
  const starts = path.join(home, `${target}.starts`);
  const child = path.join(home, `${target}.cjs`);
  const launcher = path.join(home, `${target}.sh`);
  writeFileSync(child, `const fs=require('fs');fs.appendFileSync(${JSON.stringify(starts)},process.pid+'\\n');process.stdin.setRawMode(true);process.stdin.resume();process.stdout.write('EXTERNAL_READY:'+process.pid+';');process.stdin.on('data',d=>process.stdout.write('EXTERNAL_INPUT:'+d.toString('hex')+';'));`);
  writeFileSync(launcher, `#!/bin/sh
${q(rig)} attach --self --rig ${q(rigId)} --pod agents --member external --runtime codex --cwd ${q(home)} --json > ${q(enrolled)} || exit 1
exec ${q(process.execPath)} ${q(child)}
`, { mode: 0o700 });
  let viewer = '';
  try {
    run('tmux', ['new-session', '-d', '-s', target, '/bin/sh', launcher]);
    await expect.poll(() => existsSync(starts)).toBe(true);
    const pid = readFileSync(starts, 'utf8').trim();
    const panePID = () => run('tmux', ['display-message', '-p', '-t', target, '#{pane_pid}']);
    expect(panePID()).toBe(pid);
    type Seat = { provider: string; id: string; name: string; projectName: string; host: string; attachmentTarget: string; rigHostId?: string; managed: boolean; openable: boolean };
    const observe = async () => (await (await request.get('/api/coordination?provider=rig')).json() as { seats: Seat[] }).seats.find(s => s.attachmentTarget === target);
    await expect.poll(async () => (await observe())?.openable).toBe(true);
    const seat = (await observe())!;
    expect(seat.host).toBe(''); expect(seat.managed).toBe(false);
    const attach = async () => {
      await page.getByRole('button', { name: 'Coordination', exact: true }).click();
      const panel = page.getByRole('region', { name: 'Coordination', exact: true });
      await panel.getByLabel('Show sessions').selectOption('rig');
      const article = panel.getByRole('article', { name: `${seat.name} in ${project}` });
      await expect(article).toContainText('Unknown execution host');
      if (seat.rigHostId) await expect(article).toContainText(seat.rigHostId);
      const reply = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/api/coordination/attach'));
      await article.getByRole('button', { name: 'Open session', exact: true }).click();
      const response = await reply; expect(response.status()).toBe(200);
      return await response.json() as { id: string; rig: { host: string; rig_host_id?: string } };
    };
    await page.goto('/');
    const first = await attach(); viewer = first.id;
    expect(first.rig.host).toBe(''); expect(first.rig.rig_host_id).toBe(seat.rigHostId);
    const tile = page.getByTestId(`tile-cell-${viewer}`);
    await expect(tile.locator('.xterm-rows')).toContainText(`EXTERNAL_READY:${pid};`);
    await expect(tile.getByLabel('Rig-managed worker')).toHaveAttribute('title', /Unknown execution host/);
    await tile.locator('.xterm-helper-textarea').focus();
    await page.keyboard.insertText('native-key');
    await expect(tile.locator('.xterm-rows')).toContainText(`EXTERNAL_INPUT:${Buffer.from('native-key').toString('hex')};`);
    const recovery = await (await request.get(`/api/session-recovery/${viewer}`)).json() as { hostname: string; can_resume_managed: boolean };
    expect(recovery.hostname).toBe(''); expect(recovery.can_resume_managed).toBe(false);
    await page.reload();
    await expect(tile.locator('.xterm-rows')).toContainText(`EXTERNAL_READY:${pid};`);
    expect((await request.patch(`/api/sessions/${viewer}`, { data: { minimized: true } })).ok()).toBe(true);
    await page.reload();
    const reopened = await attach();
    expect(reopened.id).toBe(viewer); expect(reopened.rig).toEqual(first.rig);
    expect(panePID()).toBe(pid); expect(readFileSync(starts, 'utf8').trim().split(/\r?\n/)).toEqual([pid]);
    const receipt = info.outputPath('external-rig-host.json');
    writeFileSync(receipt, JSON.stringify({ contract: 'openrig-cli/0.6.14', rigVersion: run(rig, ['--version']), tmuxVersion: run('tmux', ['-V']), nodeVersion: process.version, browserVersion: page.context().browser()?.version(), platform: process.platform, architecture: process.arch, externalEnrollment: true, target, originalPID: pid, finalPID: panePID(), starts: 1, host: seat.host, rigHostID: seat.rigHostId, viewerReused: true, recoveryHost: recovery.hostname, canResumeManaged: recovery.can_resume_managed, scope: 'Real isolated Rig/tmux + app/browser using an owned synthetic Node terminal. No authenticated CLI or remote execution claimed.' }));
    await info.attach('external Rig execution-host provenance', { path: receipt, contentType: 'application/json' });
  } finally {
    if (viewer) await request.delete(`/api/sessions/${viewer}`);
    run(rig, ['down', rigId]);
    // Limit fallback cleanup to this test's unique tmux session, if Rig retained it.
    const remains = spawnSync('tmux', ['has-session', '-t', `=${target}`], { env: tmuxEnv, timeout: 5000 });
    if (remains.status === 0) run('tmux', ['kill-session', '-t', `=${target}`]);
  }
});
