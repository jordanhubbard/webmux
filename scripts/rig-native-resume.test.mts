import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { resumeRigNativeCodex } from '../webmux/server/internal/openrigcoord/rig-native-resume.mts';
import { nativeCreateArtifacts as artifacts } from '../webmux/server/internal/openrigcoord/rig-native-create.mts';

const posixTest = process.platform === 'win32' ? test.skip : test;
const rigId = '01JQPWV4E7QP26M799M3S0MR6E', nodeId = '01JQPWV4E7QP26M799M3S0MR6F';
const snapshotId = '01JQPWV4E7QP26M799M3S0MR6G', historySessionId = '01JQPWV4E7QP26M799M3S0MR6H';
const operationId = '9aa1753a-1d61-4420-af42-e6b6fa591c47', nativeSessionId = '01a1eff2-f58a-7082-be6b-69f9eca4a07c';
function fixture(t: { after(fn: () => void): void }) {
  const root = artifacts.ownedRoot(fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'wm-native-resume-'))));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const cwd = path.join(root, 'project'); fs.mkdirSync(cwd);
  const input = { directory: artifacts.ownedRoot(path.join(root, 'state')), cwd, rigId, pod: 'agents', member: 'alpha',
    operationId, nodeId, snapshotId, historySessionId, nativeSessionId, sessionName: 'agents-alpha@qualified' };
  const key = createHash('sha256').update(JSON.stringify([rigId, input.pod, input.member])).digest('hex');
  const original = artifacts.ownedRoot(path.join(artifacts.ownedRoot(path.join(input.directory, 'requests')), key));
  const wrapper = [process.execPath, path.join(input.directory, 'launchers', 'original', 'codex-launch-wrapper.mts'), path.join(original, 'launch-manifest.json')];
  const manager = { port: 4242, managerPid: 123, managerIdentity: 'b'.repeat(64), processIdentity: 'c'.repeat(64) };
  artifacts.publish(path.join(original, 'launch.json'), Buffer.from(JSON.stringify({ cwd, member: input.member,
    intent: { rigId, pod: input.pod, managerIdentity: manager.managerIdentity, port: manager.port,
      requestText: JSON.stringify({ member: { runtime: 'codex', launch_wrapper: wrapper } }) } })));
  const snapshot = { id: snapshotId, rigId, data: {
    nodes: [{ id: nodeId, rigId, logicalId: 'agents.alpha', runtime: 'codex', cwd, launchWrapper: wrapper }],
    sessions: [{ id: historySessionId, nodeId, sessionName: input.sessionName, resumeType: 'codex_id', resumeToken: nativeSessionId, origin: 'launched' }],
  } };
  const review = { ok: true, contract: 'startup-history.v1', intent: 'resume-original', rigId, nodeId, logicalId: 'agents.alpha',
    snapshotId, historySessionId, revision: 'a'.repeat(64), observed: { state: 'stopped' } };
  const posts: string[] = []; let gets = 0, handoffs = 0, closed = 0, lose = false, replay = false, foreignReply = false, busy = false;
  const operationFile = path.join(original, 'recovery-operations', operationId + '.json');
  const marker = path.join(original, 'recovery-operations', operationId + '.dispatched');
  const deps = {
    run: () => { throw new Error('No arbitrary command may be replayed'); }, review: async () => manager,
    handoff: async () => { handoffs++; return { alreadyDelivered: false, manager, wait: async () => true, close: async () => { closed++; } }; },
    fetcher: (async (url: string | URL | Request, init?: RequestInit) => {
      if (init?.method !== 'POST') { gets++; return new Response(JSON.stringify(String(url).includes('/snapshots/') ? snapshot : review)); }
      const text = String(init.body), body = JSON.parse(text); posts.push(text);
      assert.equal(JSON.parse(artifacts.read(operationFile).toString()).requestText, text);
      assert.deepEqual(JSON.parse(artifacts.read(marker).toString()), { operationId, requestSHA256: createHash('sha256').update(text).digest('hex') });
      assert.equal(body.expectedOriginal.nativeId, nativeSessionId);
      if (lose) throw new Error('response lost');
      if (busy) return new Response(JSON.stringify({ ok: false, partial: true, operationId, rigId, nodeId, code: 'operation_in_progress' }), { status: 409 });
      return new Response(JSON.stringify({ ok: true, rigId, nodeId: foreignReply ? snapshotId : nodeId, operationId,
        ...(replay ? { code: 'already_running_original', nativeEffect: false }
          : { code: 'resumed', snapshotId, historySessionId, sourceSessionId: historySessionId, resultSessionId: '01JQPWV4E7QP26M799M3S0MR6J' }) }));
    }) as typeof fetch,
  };
  return { input, deps, review, snapshot, posts, operationFile, marker, manager,
    lose: () => { lose = true; }, replay: () => { lose = false; replay = true; }, foreign: () => { foreignReply = true; }, busy: () => { busy = true; },
    counts: () => ({ gets, handoffs, closed }) };
}
posixTest('native resume banks exact identity/revision/body and consumes authority before dispatch', async t => {
  const f = fixture(t), result = await resumeRigNativeCodex(f.input, f.deps);
  assert.equal(result.state, 'confirmed'); assert.equal(f.posts.length, 1);
  assert.equal(fs.statSync(f.operationFile).mode & 0o777, 0o600);
  assert.deepEqual(JSON.parse(f.posts[0]), { action: 'resume', snapshotId, historySessionId, revision: 'a'.repeat(64), operationId,
    expectedOriginal: { nodeId, sessionName: f.input.sessionName, runtime: 'codex', nativeId: nativeSessionId } });
  assert.equal(f.counts().closed, 1);
  const duplicate = await resumeRigNativeCodex(f.input, f.deps);
  assert.equal(duplicate.posted, false); assert.equal(duplicate.previouslyConfirmed, true);
  assert.equal(duplicate.state, 'uncertain', 'Historical confirmation is not current provider liveness');
});
posixTest('lost response survives invocation loss; duplicates do nothing and explicit inspection reuses exact bytes', async t => {
  const f = fixture(t); f.lose();
  const first = await resumeRigNativeCodex(f.input, f.deps); assert.equal(first.state, 'uncertain');
  const before = f.counts();
  const duplicate = await resumeRigNativeCodex({ ...f.input }, f.deps);
  assert.equal(duplicate.posted, false); assert.equal(duplicate.previouslyConfirmed, false); assert.deepEqual(f.counts(), before); assert.equal(f.posts.length, 1);
  f.review.revision = 'd'.repeat(64); f.replay();
  const inspected = await resumeRigNativeCodex({ ...f.input, inspect: true }, f.deps);
  assert.equal(inspected.state, 'confirmed'); assert.equal(inspected.code, 'already_running_original');
  assert.equal(f.posts[1], f.posts[0]); assert.equal(f.counts().gets, before.gets);
});
posixTest('changed conversation and foreign snapshot wrapper cannot select a new native original', async t => {
  const f = fixture(t); await resumeRigNativeCodex(f.input, f.deps);
  await assert.rejects(resumeRigNativeCodex({ ...f.input, nativeSessionId: operationId, inspect: true }, f.deps));
  assert.equal(f.posts.length, 1);
  const other = fixture(t); other.snapshot.data.nodes[0].launchWrapper[2] = '/foreign/manifest.json';
  await assert.rejects(resumeRigNativeCodex(other.input, other.deps)); assert.equal(other.posts.length, 0); assert.equal(other.counts().handoffs, 0);
});
posixTest('inspection without original and review of a live occupant cannot dispatch', async t => {
  const f = fixture(t); await assert.rejects(resumeRigNativeCodex({ ...f.input, inspect: true }, f.deps));
  assert.equal(f.counts().gets, 0);
  f.review.observed.state = 'running'; await assert.rejects(resumeRigNativeCodex(f.input, f.deps));
  assert.equal(f.posts.length, 0); assert.equal(f.counts().handoffs, 0);
});
posixTest('concurrent ordinary callers consume one dispatch even when both handoffs become ready', async t => {
  const f = fixture(t); let arrived = 0, ready!: () => void;
  const gate = new Promise<void>(resolve => { ready = resolve; });
  f.deps.handoff = async () => { if (++arrived === 2) ready(); await gate;
    return { alreadyDelivered: false, manager: f.manager, wait: async () => true, close: async () => {} }; };
  const results = await Promise.all([resumeRigNativeCodex(f.input, f.deps), resumeRigNativeCodex({ ...f.input }, f.deps)]);
  assert.equal(f.posts.length, 1); assert.equal(results.filter(result => result.posted).length, 1);
});
posixTest('unverified native success remains uncertain without fallback or another dispatch', async t => {
  const f = fixture(t); f.foreign(); assert.equal((await resumeRigNativeCodex(f.input, f.deps)).state, 'uncertain');
  assert.equal((await resumeRigNativeCodex(f.input, f.deps)).posted, false); assert.equal(f.posts.length, 1);
});
posixTest('native operation-in-progress remains uncertain and never triggers an automatic repost', async t => {
  const f = fixture(t); f.busy();
  const result = await resumeRigNativeCodex(f.input, f.deps);
  assert.equal(result.state, 'uncertain'); assert.equal(result.nativeCode, 'operation_in_progress');
  assert.equal((await resumeRigNativeCodex(f.input, f.deps)).posted, false); assert.equal(f.posts.length, 1);
});
posixTest('consumption flush failure leaves the original operation uncertain and never dispatches', async t => {
  const f = fixture(t), originalSync = fs.fsyncSync;
  t.after(() => { fs.fsyncSync = originalSync; });
  f.deps.handoff = async () => {
    fs.fsyncSync = () => { throw new Error('fixture flush failed'); };
    return { alreadyDelivered: false, manager: f.manager, wait: async () => true, close: async () => {} };
  };
  await assert.rejects(resumeRigNativeCodex(f.input, f.deps), /fixture flush failed/);
  fs.fsyncSync = originalSync;
  assert.equal(f.posts.length, 0); assert.equal((await resumeRigNativeCodex(f.input, f.deps)).posted, false);
});
