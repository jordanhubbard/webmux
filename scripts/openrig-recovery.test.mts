import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createServer } from 'node:http';
import { observeRigRecovery, planRigRecovery, type RigRecoveryIdentity } from '../webmux/server/internal/openrigcoord/rig-recovery.mts';

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
  let created = 0, metadataStatus = 200, corruptSnapshot = false;
  const plan: Record<string, unknown> = { ok: true, planOnly: true, snapshotSelection: { snapshotId: snapshotIds[0] }, targetNodes: [{ nodeId, logicalId: 'agents.coder' }], unmatchedIds: [], nonTargetEffects: { mode: 'unchanged', reason: null, affected: [] } };
  const server = createServer((req, res) => {
    calls.push(`${req.method} ${req.url}`); res.setHeader('content-type', 'application/json');
    const send = (body: unknown) => res.end(JSON.stringify(body));
    if (req.url === `/api/rigs/${rig}/nodes/agents.coder`) return send(node);
    if (req.url === `/api/seat/status/${encodeURIComponent(target)}`) return send({ permissions: policy });
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
  return { root, cwd, directory, file, run, identity, node, policy, calls, plan, snapshots, created: () => created, metadataStatus: (value: number) => { metadataStatus = value; }, corruptSnapshot: () => { corruptSnapshot = true; }, close: async () => { server.closeAllConnections(); await new Promise<void>(resolve => server.close(() => resolve())); fs.rmSync(root, { recursive: true, force: true }); } };
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
    assert.equal(fs.statSync(path.join(f.directory, cache)).mode & 0o777, 0o600);
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
