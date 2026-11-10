import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createServer } from 'node:http';
import { observeRigRecovery, planRigRecovery, observeRigNativeProcess, rigBootIdentity, inspectRigRecoveryProcess, type RigRecoveryIdentity } from '../webmux/server/internal/openrigcoord/rig-recovery.mts';

async function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-rig-recovery-'));
  const cwd = path.join(root, 'project');
  const directory = path.join(root, 'owner');
  const file = path.join(root, 'native/projects/project/native.jsonl');
  fs.mkdirSync(cwd, { recursive: true }); fs.mkdirSync(directory); fs.mkdirSync(path.dirname(file), { recursive: true });
  const native = '08bbfe0e-d31c-4cb6-9d18-546eea56a08c';
  fs.writeFileSync(file, JSON.stringify({ sessionId: native, cwd }) + '\n');
  const rig = '01M71ZM6DE3RTCXHPJRXJ6XKPE', nodeId = '01M71ZM6TZ91BH689AGCFP2WCN';
  const snapshotIds = ['01M71ZM6QSB0G9GNTTXX0242KJ', '01M71ZM6QSB0G9GNTTXX0242KK'];
  const target = 'agents-coder@project';
  const node: Record<string, unknown> = { nodeId, rigId: rig, logicalId: 'agents.coder', canonicalSessionName: target, runtime: 'claude-code', cwd, profile: 'default', agentRef: 'local:fixture', resolvedSpecHash: 'source-hash' };
  const policy = { desired: { runtime: 'claude-code', mode: 'plan' }, effective: { permissionMode: 'plan', effectiveMode: 'plan', source: 'explicit' } };
  const snapshots = new Map<string, unknown>();
  const calls: string[] = [];
  let inheritedPolicy = false;
  let created = 0, metadataStatus = 200, corruptSnapshot = false;
  const plan: Record<string, unknown> = { ok: true, planOnly: true, snapshotSelection: { snapshotId: snapshotIds[0] }, targetNodes: [{ nodeId, logicalId: 'agents.coder' }], unmatchedIds: [], nonTargetEffects: { mode: 'unchanged', reason: null, affected: [] } };
  const server = createServer((req, res) => {
    calls.push(`${req.method} ${req.url}`); res.setHeader('content-type', 'application/json');
    const send = (body: unknown) => res.end(JSON.stringify(body));
    if (req.url === `/api/rigs/${rig}/nodes/agents.coder`) return send(node);
    if (req.url === `/api/seat/status/${encodeURIComponent(target)}`) return send({ permissions: inheritedPolicy ? { selectionState: 'inherit', desired: null, effective: { effectiveMode: 'acceptEdits', source: 'system_default', launchPosture: 'floor' } } : policy });
    if (req.url === `/api/sessions/${encodeURIComponent(target)}/generation-record`) {
      res.statusCode = metadataStatus;
      const stat = fs.statSync(file);
      return send({ runtime: 'claude-code', sessionId: native, totalBytes: stat.size, generationId: JSON.stringify(['18bbfe0e-d31c-4cb6-9d18-546eea56a08c', native, file, stat.dev, stat.ino, stat.birthtimeMs]) });
    }
    if (req.method === 'POST' && req.url === `/api/rigs/${rig}/snapshots`) {
      const id = snapshotIds[created++];
      const body = { id, rigId: rig, data: { nodes: [{ id: nodeId, rigId: rig, logicalId: 'agents.coder', runtime: 'claude-code', profile: 'default', cwd }], sessions: [{ id: 'enrollment', nodeId, sessionName: target, status: 'running', resumeType: 'claude_id', resumeToken: corruptSnapshot ? 'foreign' : native, resumeProvenance: 'hook' }], activeSessionIdByNode: { [nodeId]: 'enrollment' }, nodeStartupContext: { [nodeId]: { runtime: 'claude-code', startupActions: [] } }, unrelatedSecret: 'DO_NOT_EXPORT' } };
      snapshots.set(id, body); return send(body);
    }
    const id = req.url?.split('/').at(-1);
    if (id && snapshots.has(id)) return send(snapshots.get(id));
    res.statusCode = 404; send({ error: 'not found' });
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  const address = server.address(); assert.ok(address && typeof address !== 'string');
  const run = (command: string, args: string[]) => {
    assert.equal(command, 'rig');
    if (args[0] === 'launch') {
      assert.deepEqual(args, ['launch', rig, 'agents.coder', '--snapshot-id', snapshotIds[0], '--plan', '--json']);
      calls.push('CLI exact-seat plan'); return JSON.stringify(plan);
    }
    if (args.join(' ') === '--version') return '0.6.14 (9d0200f3)';
    if (args.join(' ') === 'daemon status') return `Daemon running on port ${address.port}`;
    assert.deepEqual(args.slice(0, 4), ['ps', '--nodes', '-A', '--json']);
    return JSON.stringify([{ rigId: rig, logicalId: 'agents.coder', canonicalSessionName: target, runtime: 'claude-code' }]);
  };
  const identity: RigRecoveryIdentity = { target, project: 'project', name: 'coder', host: 'localhost', runtime: 'claude-code', workspace: cwd };
  return { root, cwd, directory, file, run, identity, node, policy, calls, plan, snapshots, inherit: () => { inheritedPolicy = true; }, created: () => created, metadataStatus: (value: number) => { metadataStatus = value; }, corruptSnapshot: () => { corruptSnapshot = true; }, close: async () => { server.closeAllConnections(); await new Promise<void>(resolve => server.close(() => resolve())); fs.rmSync(root, { recursive: true, force: true }); } };
}

test('native Rig capture binds exact snapshot and record, reuses unchanged state, and exports no conversation or settings', async () => {
  const f = await fixture();
  try {
    const first = await observeRigRecovery(f.run, f.identity, f.directory);
    assert.equal(first.native_source, 'rig-snapshot'); assert.equal(first.native_state_file, f.file);
    assert.match(first.manager_snapshot_hash!, /^[a-f0-9]{64}$/); assert.match(first.manager_policy_hash!, /^[a-f0-9]{64}$/);
    assert.equal(JSON.stringify(first).includes('DO_NOT_EXPORT'), false);
    const repeated = await observeRigRecovery(f.run, f.identity, f.directory);
    assert.deepEqual(repeated, first); assert.equal(f.created(), 1);
    assert.equal(f.calls.some(call => call.includes('sinceBytes')), false);
    const cache = fs.readdirSync(f.directory).find(name => name.endsWith('.json'))!;
    // Native Rig is supported on macOS/Linux; Windows does not implement
    // POSIX owner-only mode bits. Keep its metadata/rejection checks active.
    if (process.platform !== 'win32') assert.equal(fs.statSync(path.join(f.directory, cache)).mode & 0o777, 0o600);
    fs.mkdirSync(path.join(f.cwd, '.claude'));
    fs.writeFileSync(path.join(f.cwd, '.claude/settings.local.json'), '{"permissions":{"deny":["Bash"]}}');
    const changed = await observeRigRecovery(f.run, f.identity, f.directory);
    assert.notEqual(changed.manager_policy_hash, first.manager_policy_hash);
    assert.notEqual(changed.manager_snapshot_id, first.manager_snapshot_id); assert.equal(f.created(), 2);
  } finally { await f.close(); }
});

test('native Rig capture rejects mismatched conversation and manager authorization without checkpoint creation', async () => {
  const f = await fixture();
  try {
    f.metadataStatus(401);
    await assert.rejects(observeRigRecovery(f.run, f.identity, f.directory), /cannot verify/);
    assert.equal(f.created(), 0);
    f.metadataStatus(200);
    fs.writeFileSync(f.file, JSON.stringify({ sessionId: 'foreign', cwd: f.cwd }));
    await assert.rejects(observeRigRecovery(f.run, f.identity, f.directory), /does not verify/);
    assert.equal(f.created(), 0);
  } finally { await f.close(); }
});

test('native Rig capture rejects changed destination, profile, policy and snapshot identity', async () => {
  const f = await fixture();
  try {
    f.node.profile = 'other';
    await assert.rejects(observeRigRecovery(f.run, f.identity, f.directory), /profile/);
    f.node.profile = 'default'; f.policy.desired.mode = 'full_bypass';
    await assert.rejects(observeRigRecovery(f.run, f.identity, f.directory), /policy/);
    assert.equal(f.created(), 0);
    f.policy.desired.mode = 'plan'; f.corruptSnapshot();
    await assert.rejects(observeRigRecovery(f.run, f.identity, f.directory), /snapshot/);
    assert.equal(fs.readdirSync(f.directory).length, 0);
  } finally { await f.close(); }
});

test('Rig recovery preflight validates immutable checkpoint and current policy without native launches', async () => {
  const f = await fixture();
  try {
    const saved = await observeRigRecovery(f.run, f.identity, f.directory);
    const created = f.created();
    const plan = await planRigRecovery(f.run, saved);
    assert.equal(plan.plan_only, true); assert.equal(plan.snapshot_id, saved.manager_snapshot_id);
    assert.equal(plan.native_session_id, saved.native_session_id);
    assert.equal(f.created(), created);
    assert.equal(f.calls.filter(call => call === 'CLI exact-seat plan').length, 1);
    f.policy.effective.permissionMode = 'acceptEdits';
    await assert.rejects(planRigRecovery(f.run, saved), /policy/);
    f.policy.effective.permissionMode = 'plan';
    fs.mkdirSync(path.join(f.cwd, '.claude'));
    fs.writeFileSync(path.join(f.cwd, '.claude/settings.json'), '{}');
    await assert.rejects(planRigRecovery(f.run, saved), /policy changed/);
    assert.equal(f.calls.filter(call => call === 'CLI exact-seat plan').length, 1);
  } finally { await f.close(); }
});

test('Rig recovery preflight refuses changed snapshot, selected node, native file and plan side effects', async () => {
  const f = await fixture();
  try {
    const saved = await observeRigRecovery(f.run, f.identity, f.directory);
    const original = structuredClone(f.snapshots.get(saved.manager_snapshot_id!));
    const snapshot = f.snapshots.get(saved.manager_snapshot_id!) as { data: { nodes: { cwd: string }[]; unrelatedSecret: string } };
    snapshot.data.nodes[0].cwd = f.directory;
    await assert.rejects(planRigRecovery(f.run, saved), /snapshot does not verify/);
    snapshot.data.nodes[0].cwd = f.cwd;
    snapshot.data.unrelatedSecret = 'changed';
    await assert.rejects(planRigRecovery(f.run, saved), /checkpoint changed/);
    f.snapshots.set(saved.manager_snapshot_id!, original);
    f.plan.nonTargetEffects = { mode: 'changed', reason: 'other seat', affected: ['foreign'] };
    await assert.rejects(planRigRecovery(f.run, saved), /other effects/);
    f.plan.nonTargetEffects = { mode: 'unchanged', reason: null, affected: [] };
    f.plan.targetNodes = [{ nodeId: saved.manager_node_id, logicalId: 'foreign' }];
    await assert.rejects(planRigRecovery(f.run, saved), /other effects/);
    fs.renameSync(f.file, f.file + '.old');
    fs.writeFileSync(f.file, fs.readFileSync(f.file + '.old'));
    await assert.rejects(planRigRecovery(f.run, saved), /state or Rig recovery policy changed/);
    assert.equal(f.created(), 1);
  } finally { await f.close(); }
});

test('Rig native process proof joins only the owned descendant to executable, native ID, cwd and stable process', { skip: process.platform === 'win32' }, async () => {
  const f = await fixture();
  try {
    const executable = path.join(f.root, 'claude');
    fs.writeFileSync(executable, 'synthetic native binary'); fs.chmodSync(executable, 0o700);
    const settings = { permissions: { deny: ['*'] }, privateValue: 'DO_NOT_EXPORT_PROCESS_SETTINGS' };
    const identity = { ...f.identity, native_session_id: '08bbfe0e-d31c-4cb6-9d18-546eea56a08c' };
    let nativeId = identity.native_session_id, kernelCwd = f.cwd, binary = executable, duplicate = false, changed = false, foreignUID = false, argvChanged = false, fresh = false, legacy = false, noSettings = false, reverseKeys = false, version = '2.1.322 (Claude Code)';
    let paneReads = 0, versionReads = 0;
    const calls: { command: string; args: string[] }[] = [];
    const run = (command: string, args: string[]) => {
      calls.push({ command, args });
      if (command === 'tmux') { assert.equal(args[3], '=' + identity.target + ':'); paneReads++; return changed && paneReads > 1 ? '200' : '100'; }
      if (command === executable) { versionReads++; assert.deepEqual(args, ['--version']); return version; }
      if (command === 'lsof') return 'p101\nn' + (args.includes('txt') ? binary : kernelCwd);
      if (command === 'env') { assert.deepEqual(args.slice(0, 3), ['LC_ALL=C', 'TZ=UTC', 'ps']); return 'Mon Nov  9 22:21:02 2026'; }
      assert.equal(command, 'ps');
      if (args[0] === '-axo') {
        assert.equal(args[1], 'pid=,ppid=,uid=');
        const uid = String(process.getuid!());
        return '100 1 ' + uid + '\n101 100 ' + (foreignUID ? String(Number(uid) + 1) : uid) + '\n300 1 -2' + (duplicate ? '\n102 100 ' + uid : '');
      }
      if (args.at(-1) === 'lstart=') return 'Mon Nov  9 22:21:02 2026';
      assert.equal(args[0], '-ww');
      assert.equal(args.includes('300'), false);
      const policy = JSON.stringify(reverseKeys ? { privateValue: settings.privateValue, permissions: settings.permissions } : settings);
      const settingsArg = noSettings ? '' : ' --settings ' + policy;
      const native = fresh ? executable + ' --permission-mode acceptEdits' + settingsArg + ' --session-id ' + nativeId + ' --name ' + identity.target : legacy ? executable + ' --permission-mode acceptEdits' + settingsArg + ' --resume ' + nativeId + ' --name ' + identity.target : executable + ' --permission-mode plan --resume ' + nativeId + ' --name ' + identity.target + settingsArg;
      if (args[2] === '101') return '101 100 ' + (argvChanged ? native.replace(nativeId, '18bbfe0e-d31c-4cb6-9d18-546eea56a08c') : native);
      return '100 1 /bin/bash\n101 100 ' + native + (duplicate ? '\n102 100 ' + native.replace(nativeId, '18bbfe0e-d31c-4cb6-9d18-546eea56a08c') : '');
    };
    const proof = observeRigNativeProcess(run, identity, false, 'darwin');
    assert.equal(proof.native_pid, 101); assert.equal(proof.pane_pid, 100);
    assert.equal(proof.native_session_id, identity.native_session_id); assert.equal(proof.permission_mode, 'plan');
    assert.match(proof.executable_hash, /^[a-f0-9]{64}$/); assert.match(proof.settings_hash, /^[a-f0-9]{64}$/);
    assert.equal(JSON.stringify(proof).includes(settings.privateValue), false);
    assert.equal(calls.some(call => call.args.includes('args=') && call.args.includes('-axo')), false);
    reverseKeys = true;
    assert.equal(observeRigNativeProcess(run, identity, false, 'darwin').settings_hash, proof.settings_hash, 'Object key order changed semantic policy identity');
    reverseKeys = false;
    fresh = true;
    const initial = observeRigNativeProcess(run, identity, true, 'darwin');
    assert.equal(initial.native_session_id, identity.native_session_id); assert.equal(initial.permission_mode, 'acceptEdits');
    assert.throws(() => observeRigNativeProcess(run, identity, false, 'darwin'), /missing or ambiguous/);
    fresh = false;
    legacy = true;
    assert.equal(observeRigNativeProcess(run, identity, false, 'darwin').permission_mode, 'acceptEdits');
    noSettings = true;
    assert.match(observeRigNativeProcess(run, identity, false, 'darwin').settings_hash, /^[a-f0-9]{64}$/);
    legacy = false; noSettings = false;
    foreignUID = true;
    const argumentsRead = calls.filter(call => call.args.includes('pid=,ppid=,args=')).length;
    assert.throws(() => observeRigNativeProcess(run, identity, false, 'darwin'), /execution-account boundary/);
    assert.equal(calls.filter(call => call.args.includes('pid=,ppid=,args=')).length, argumentsRead);
    foreignUID = false;
    nativeId = '18bbfe0e-d31c-4cb6-9d18-546eea56a08c';
    assert.throws(() => observeRigNativeProcess(run, identity, false, 'darwin'), /missing or ambiguous/);
    nativeId = identity.native_session_id; kernelCwd = f.directory;
    assert.throws(() => observeRigNativeProcess(run, identity, false, 'darwin'), /kernel project changed/);
    kernelCwd = f.cwd; binary = f.file;
    const beforeSpoof = versionReads;
    assert.throws(() => observeRigNativeProcess(run, identity, false, 'darwin'), /OS executable/);
    assert.equal(versionReads, beforeSpoof, 'A forged argv executable was invoked before OS identity verification');
    binary = executable; duplicate = true;
    assert.throws(() => observeRigNativeProcess(run, identity, false, 'darwin'), /missing or ambiguous/);
    duplicate = false; version = 'future';
    assert.throws(() => observeRigNativeProcess(run, identity, false, 'darwin'), /version/);
    version = '2.1.322 (Claude Code)'; argvChanged = true;
    assert.throws(() => observeRigNativeProcess(run, identity, false, 'darwin'), /arguments changed during verification/);
    argvChanged = false; changed = true; paneReads = 0;
    assert.throws(() => observeRigNativeProcess(run, identity, false, 'darwin'), /changed during verification/);
  } finally { await f.close(); }
});

test('Rig capture stores bounded original process and boot evidence; failed observation clears stale process proof', async () => {
  const f = await fixture();
  try {
    const process = { pane_pid: 100, native_pid: 101, execution_uid: 501, started_at: 'Mon Nov  9 22:21:02 2026 UTC', permission_mode: 'acceptEdits', executable_hash: 'd'.repeat(64), settings_hash: 'e'.repeat(64) };
    let boot = 'f'.repeat(64);
    const observer = () => process;
    const first = await observeRigRecovery(f.run, f.identity, f.directory, observer, () => boot);
    const saved = JSON.parse(first.manager_process!);
    assert.equal(saved.native_pid, 101); assert.equal(saved.pane_pid, 100);
    assert.equal(saved.native_session_id, first.native_session_id); assert.equal(saved.native_identity_source, 'manager-hook');
    assert.equal(saved.boot_id, boot); assert.equal(saved.execution_uid, 501);
    assert.equal(saved.permission_mode, 'acceptEdits');
    assert.equal(saved.args, undefined); assert.equal(saved.settings, undefined);
    boot = 'a'.repeat(64);
    const changed = await observeRigRecovery(f.run, first, f.directory, observer, () => boot);
    assert.notEqual(changed.manager_process, first.manager_process);
    assert.equal(changed.manager_snapshot_id, first.manager_snapshot_id);
    const failed = await observeRigRecovery(f.run, changed, f.directory, () => { throw new Error('Unverified native process'); }, () => boot);
    assert.equal(failed.manager_process, ''); assert.equal(failed.native_session_id, first.native_session_id);
    const snapshot = f.snapshots.get(first.manager_snapshot_id!) as { data: { sessions: { resumeProvenance: string }[] } };
    snapshot.data.sessions[0].resumeProvenance = 'operator';
    const operator = await observeRigRecovery(f.run, first, f.directory, observer, () => boot);
    assert.equal(operator.manager_process, '', 'Operator token alone qualified a fresh process conversation');
    const exact = await observeRigRecovery(f.run, first, f.directory, () => ({ ...process, permission_mode: 'plan', native_session_id: first.native_session_id }), () => boot);
    assert.equal(JSON.parse(exact.manager_process!).native_identity_source, 'process-argv');
  } finally { await f.close(); }
});

test('Rig boot fingerprint remains stable across UUID formatting and changes for another kernel boot', () => {
  const run = (value: string) => (command: string, args: string[]) => {
    assert.equal(command, 'sysctl'); assert.deepEqual(args, ['-n', 'kern.bootsessionuuid']); return value;
  };
  const first = rigBootIdentity(run('08bbfe0e-d31c-4cb6-9d18-546eea56a08c'), 'darwin');
  assert.equal(first, rigBootIdentity(run('08BBFE0E-D31C-4CB6-9D18-546EEA56A08C\n'), 'darwin'));
  assert.notEqual(first, rigBootIdentity(run('18bbfe0e-d31c-4cb6-9d18-546eea56a08c'), 'darwin'));
  assert.throws(() => rigBootIdentity(run('unknown'), 'darwin'), /boot identity/);
});

test('Rig survivor inspection refuses duplicates, escaped originals, unknown absence and policy drift', { skip: process.platform === 'win32' }, async () => {
  const f = await fixture();
  try {
    const uid = process.getuid!();
    const nativeId = '08bbfe0e-d31c-4cb6-9d18-546eea56a08c';
    const original = { pane_pid: 100, native_pid: 101, execution_uid: uid, started_at: 'Mon Nov  9 22:21:02 2026 UTC', native_session_id: nativeId, permission_mode: 'acceptEdits', executable_hash: 'd'.repeat(64), settings_hash: 'e'.repeat(64) };
    const storedBoot = 'f'.repeat(64);
    const saved = await observeRigRecovery(f.run, f.identity, f.directory, () => original, () => storedBoot);
    let boot = storedBoot, alive = true, pane = true, census = true, denied = false, changedSettings = false, currentPid = 101;
    let processReads = 0;
    const run = (command: string, args: string[]) => {
      if (command === 'ps') {
        assert.deepEqual(args, ['-axo', 'pid=,ppid=,uid=']); processReads++;
        return (census ? `${process.pid} ${process.ppid} ${uid}\n` : '') + '300 1 -2' + (alive ? `\n101 100 ${uid}` : '');
      }
      if (command === 'env') { assert.deepEqual(args, ['LC_ALL=C', 'TZ=UTC', 'ps', '-p', '101', '-o', 'lstart=']); return 'Mon Nov  9 22:21:02 2026'; }
      if (command === 'tmux') {
        assert.deepEqual(args, ['has-session', '-t', '=' + f.identity.target]);
        if (denied) throw Object.assign(new Error('tmux failed'), { status: 1, stderr: 'permission denied' });
        if (!pane) throw Object.assign(new Error('absent'), { status: 1, stderr: Buffer.from("can't find session: " + f.identity.target) });
        return '';
      }
      return f.run(command, args);
    };
    const inspect = () => inspectRigRecoveryProcess(run, saved, () => ({ ...original, native_pid: currentPid, settings_hash: changedSettings ? 'a'.repeat(64) : original.settings_hash }), () => boot);
    assert.equal((await inspect()).reason, 'original-process');
    pane = false;
    await assert.rejects(inspect(), /still running without its recorded pane/);
    alive = false;
    const before = processReads;
    assert.deepEqual(await inspect(), { state: 'missing', reason: 'process-loss' });
    assert.equal(processReads - before, 2);
    denied = true;
    await assert.rejects(inspect(), /pane absence cannot be verified/);
    denied = false; census = false;
    await assert.rejects(inspect(), /process absence cannot be verified/);
    census = true; alive = true; boot = 'a'.repeat(64);
    const previousReads = processReads;
    assert.deepEqual(await inspect(), { state: 'missing', reason: 'host-reboot' });
    assert.equal(processReads, previousReads, 'An old PID was reused to infer survival across a different boot');
    boot = storedBoot; pane = true; changedSettings = true;
    await assert.rejects(inspect(), /original settings changed/);
    changedSettings = false; currentPid = 102;
    await assert.rejects(inspect(), /still exists outside the selected pane/);
    alive = false;
    assert.equal((await inspect()).reason, 'verified-continuation');
    f.metadataStatus(401);
    await assert.rejects(inspect(), /cannot verify/);
    f.metadataStatus(200);
    let reads = 0;
    await assert.rejects(inspectRigRecoveryProcess(run, saved, () => original, () => ++reads === 1 ? storedBoot : 'b'.repeat(64)), /host identity changed/);
  } finally { await f.close(); }
});

test('Rig metadata captures an unchanged inherited team default independently of explicit plan mode', async () => {
  const f = await fixture();
  try {
    f.inherit();
    const saved = await observeRigRecovery(f.run, f.identity, f.directory);
    assert.equal(saved.native_source, 'rig-snapshot');
    assert.equal((await planRigRecovery(f.run, saved)).plan_only, true);
  } finally { await f.close(); }
});
