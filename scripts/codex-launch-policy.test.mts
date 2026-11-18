import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { projectCodexOwnerPlan } from '../webmux/server/internal/openrigcoord/codex-owner-plan.mts';
import { filterRigCodexLaunch } from '../webmux/server/internal/openrigcoord/codex-launch-policy.mts';
const wrapper = fileURLToPath(new URL('../webmux/server/internal/openrigcoord/codex-launch-wrapper.mts', import.meta.url));

function plan(cwd = '/project', home = '/account', executable = '/bin/codex', executableSHA256 = 'a'.repeat(64)) {
  return projectCodexOwnerPlan({}, { thread: { ephemeral: true }, cwd, model: 'owner-model', modelProvider: 'openai',
    reasoningEffort: 'medium', approvalsReviewer: 'user', approvalPolicy: 'never',
    sandbox: { type: 'workspaceWrite', writableRoots: ['/extra root'], networkAccess: false, excludeSlashTmp: true, excludeTmpdirEnvVar: true } },
  { requiresOpenaiAuth: true, userId: 'private-user', account: { type: 'chatgpt' },
    workspaceRouting: { chatgptAccountId: 'private-account', backendOrigin: 'https://example.invalid', accountRoutingOverride: 'NO_CONSTRAINT' } },
  { executable, executableSHA256, cwd, home, profile: null });
}
const args = (cwd = '/project') => ['--no-daemon', '-s', 'workspace-write', '-a', 'never',
  '-c', 'sandbox_workspace_write.writable_roots=["/extra root"]', '-c', 'sandbox_workspace_write.network_access=false',
  '-c', 'sandbox_workspace_write.exclude_tmpdir_env_var=true', '-c', 'sandbox_workspace_write.exclude_slash_tmp=true',
  '-c', 'check_for_update_on_startup=false', '-c', 'allow_login_shell=false', '-C', cwd,
  '--add-dir', '/project/.git', '--add-dir', '/rig/seat', '--add-dir', '/rig/queue',
  '-c', 'developer_instructions="OpenRig seat guidance for this session follows. Fixture team role."'];
test('removes Rig additions while retaining the exact explicit owner policy', () => {
  const result = filterRigCodexLaunch(plan(), args());
  assert.deepEqual(result.removedDirectories, ['/project/.git', '/rig/seat', '/rig/queue']);
  assert.equal(result.removedLoginShellOverride, true); assert.equal(result.removedManagedInstructions, true);
  assert.equal(result.args.includes('--add-dir'), false);
  assert.equal(result.args.includes('allow_login_shell=false'), false);
  assert.equal(result.args.some(arg => arg.startsWith('developer_instructions=')), false);
  for (const flag of args().filter(arg => arg.startsWith('sandbox_workspace_write.'))) assert.ok(result.args.includes(flag));
});
test('policy, cwd and model drift refuse without quoting private arguments', () => {
  const before = args();
  for (const [from, to] of [['workspace-write', 'danger-full-access'], ['never', 'on-request'], ['/project', '/other'],
    ['sandbox_workspace_write.writable_roots=["/extra root"]', 'sandbox_workspace_write.writable_roots=["/private-owner-secret"]'],
    ['sandbox_workspace_write.network_access=false', 'sandbox_workspace_write.network_access=true'],
    ['sandbox_workspace_write.exclude_slash_tmp=true', 'sandbox_workspace_write.exclude_slash_tmp=false'],
    ['sandbox_workspace_write.exclude_tmpdir_env_var=true', 'sandbox_workspace_write.exclude_tmpdir_env_var=false']]) {
    const altered = before.map(arg => arg === from ? to : arg);
    assert.throws(() => filterRigCodexLaunch(plan(), altered), { message: 'codex_launch_policy_mismatch' });
  }
  assert.throws(() => filterRigCodexLaunch(plan(), [...before, '--model', 'other-model']), /codex_launch_policy_mismatch/);
});
test('unknown, duplicate, truncated, profile and resume options cannot change the launch', () => {
  for (const tail of [['--config', 'sandbox_mode="danger-full-access"'], ['-c', 'allow_login_shell=true'],
    ['-c', 'sandbox_workspace_write.network_access=false'], ['-C', '/project'], ['--add-dir'],
    ['--add-dir', 'relative'], ['--add-dir', '/rig/seat'], ['-p', 'other'], ['resume', '--last'],
    ['-c', 'developer_instructions="private unrecognized instruction"']]) {
    assert.throws(() => filterRigCodexLaunch(plan(), [...args(), ...tail]), /codex_launch_policy_mismatch/);
  }
  assert.throws(() => filterRigCodexLaunch(plan(), args().filter(arg => arg !== 'sandbox_workspace_write.network_access=false')), /codex_launch_policy_mismatch/);
});
test('changed fingerprints and unrepresentable read-only networking refuse', () => {
  const changed = { ...plan(), approval: 'on-request' };
  assert.throws(() => filterRigCodexLaunch(changed, args()), /codex_launch_policy_mismatch/);
  const original = plan();
  const content = { ...original, policy: { type: 'readOnly', networkAccess: true } }; delete (content as { fingerprint?: string }).fingerprint;
  const fingerprint = createHash('sha256').update(JSON.stringify(content)).digest('hex');
  assert.throws(() => filterRigCodexLaunch({ ...content, fingerprint }, ['-s', 'read-only', '-a', 'never', '-C', '/project']), /codex_launch_policy_mismatch/);
});
test('model, effort and service tier remain frozen when Rig omits them', () => {
  const { fingerprint: _previous, ...content } = plan();
  content.effort = 'high'; content.serviceTier = 'priority';
  const fingerprint = createHash('sha256').update(JSON.stringify(content)).digest('hex');
  const result = filterRigCodexLaunch({ ...content, fingerprint }, args());
  assert.deepEqual(result.args.slice(-6), ['--model', 'owner-model', '-c', 'model_reasoning_effort="high"', '-c', 'service_tier="priority"']);
});
test('representable read-only and full-access choices keep their original sandbox', () => {
  for (const policy of [{ type: 'dangerFullAccess' } as const, { type: 'readOnly', networkAccess: false } as const]) {
    const { fingerprint: _previous, ...content } = plan(); content.policy = policy;
    const fingerprint = createHash('sha256').update(JSON.stringify(content)).digest('hex');
    const sandbox = policy.type === 'dangerFullAccess' ? 'danger-full-access' : 'read-only';
    const result = filterRigCodexLaunch({ ...content, fingerprint }, ['-s', sandbox, '-a', 'never', '-C', '/project']);
    assert.deepEqual(result.args.slice(0, 6), ['-s', sandbox, '-a', 'never', '-C', '/project']);
  }
});
const posixTest = process.platform === 'win32' ? test.skip : test;
function fixture(t: { after(fn: () => void): void }) {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'wm-launch-policy-')));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  fs.chmodSync(root, 0o700);
  const cwd = path.join(root, 'project'), home = path.join(root, 'home'); fs.mkdirSync(cwd); fs.mkdirSync(home);
  const capture = path.join(root, 'capture.mjs');
  fs.writeFileSync(capture, `console.log(JSON.stringify({pid:process.pid,args:process.argv.slice(2),node:process.env.OPENRIG_NODE_ID,home:process.env.CODEX_HOME}));`);
  const executable = path.join(root, 'native');
  const q = (s: string) => `'${s.replaceAll("'", "'\\''")}'`;
  fs.writeFileSync(executable, `#!/bin/sh\nexec ${q(process.execPath)} ${q(capture)} "$@"\n`, { mode: 0o700 });
  const digest = createHash('sha256').update(fs.readFileSync(executable)).digest('hex');
  const value = plan(cwd, home, executable, digest), manifest = path.join(root, 'plan.json');
  fs.writeFileSync(manifest, JSON.stringify(value), { mode: 0o600 });
  const run = () => spawnSync(process.execPath, [wrapper, manifest, executable, ...args(cwd)],
    { cwd, env: { ...process.env, OPENRIG_NODE_ID: 'fixture-node', OPENRIG_OCCUPANT_GENERATION: 'fixture-generation', CODEX_HOME: home }, encoding: 'utf8', timeout: 5000 });
  return { root, manifest, executable, value, run };
}
posixTest('wrapper execve retains PID and native identity while applying the policy', t => {
  const f = fixture(t), result = f.run();
  assert.equal(result.status, 0, result.stderr);
  const captured = JSON.parse(result.stdout);
  assert.equal(captured.pid, result.pid); assert.equal(captured.node, 'fixture-node'); assert.equal(captured.home, f.value.home);
  assert.deepEqual(captured.args, filterRigCodexLaunch(f.value, args(f.value.cwd)).args);
});
posixTest('unsafe manifest modes, symlinks and changed executable bytes never execute', t => {
  for (const mutation of ['mode', 'symlink', 'executable']) {
    const f = fixture(t);
    if (mutation === 'mode') fs.chmodSync(f.manifest, 0o644);
    if (mutation === 'symlink') { fs.renameSync(f.manifest, f.manifest + '.real'); fs.symlinkSync(f.manifest + '.real', f.manifest); }
    if (mutation === 'executable') fs.appendFileSync(f.executable, '# changed\n');
    const result = f.run(); assert.equal(result.status, 1); assert.equal(result.stdout, '');
    assert.match(result.stderr, /native Codex launch policy could not be preserved/); assert.equal(result.stderr.includes(f.root), false);
  }
});
