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
  const posts: string[] = []; let gets = 0, handoffs = 0, closed = 0, lose = false, replay = false, foreignReply = false, busy = false, ownerReady = true, now = 0;
  const operationFile = path.join(original, 'recovery-operations', operationId + '.json');
  const marker = path.join(original, 'recovery-operations', operationId + '.dispatched');
  const deps = {
    verifyPolicy: () => {},
    readinessNow: () => now,
    readinessWait: async (milliseconds: number) => { now += milliseconds; },
    run: () => { throw new Error('No arbitrary command may be replayed'); }, review: async () => manager,
    handoff: async () => { handoffs++; return { alreadyDelivered: false, manager, wait: async () => { await new Promise<void>(resolve => setImmediate(resolve)); return true; }, close: async () => { closed++; } }; },
    fetcher: (async (url: string | URL | Request, init?: RequestInit) => {
      if (init?.method !== 'POST') {
        gets++;
        const sessions = [{ id: '01JQPWV4E7QP26M799M3S0MR6J', nodeId, status: 'running', sessionName: input.sessionName,
          resumeType: ownerReady ? 'codex_id' : null, resumeToken: ownerReady ? nativeSessionId : null,
          startupStatus: ownerReady ? 'ready' : 'pending' }];
        return new Response(JSON.stringify(String(url).endsWith('/sessions') ? sessions : String(url).includes('/snapshots/') ? snapshot : review));
      }
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
    ownerReady: (value: boolean) => { ownerReady = value; }, clock: () => now, counts: () => ({ gets, handoffs, closed }) };
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
  assert.equal(f.posts[1], f.posts[0]); assert.equal(f.counts().gets, before.gets + 1, 'one read-only readiness query before confirming the original keyed request');
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

posixTest('preparation banks the exact native review without handoff or dispatch; explicit completion can be first POST', async t => {
  const f = fixture(t);
  const prepared = await resumeRigNativeCodex({ ...f.input, prepare: true }, f.deps);
  assert.equal(prepared.state, 'prepared'); assert.equal(prepared.posted, false);
  assert.equal(f.posts.length, 0); assert.equal(f.counts().handoffs, 0);
  const before = f.counts().gets;
  // A later invocation must use the banked review, even if a fresh review
  // would refuse the current state. No replacement review or UUID is allowed.
  f.review.observed.state = 'running';
  const again = await resumeRigNativeCodex({ ...f.input, prepare: true }, f.deps);
  assert.equal(again.state, 'prepared'); assert.equal(f.counts().gets, before);
  const result = await resumeRigNativeCodex({ ...f.input, inspect: true }, f.deps);
  assert.equal(result.state, 'confirmed'); assert.equal(f.posts.length, 1);
  assert.equal(f.counts().gets, before);
  const body = JSON.parse(f.posts[0]);
  assert.equal(body.operationId, f.input.operationId);
  assert.equal(body.snapshotId, f.input.snapshotId);
});
posixTest('preparation cannot reset a consumed operation or change a banked selection', async t => {
  const f = fixture(t); f.lose();
  await resumeRigNativeCodex(f.input, f.deps);
  const before = f.counts();
  const duplicate = await resumeRigNativeCodex({ ...f.input, prepare: true }, f.deps);
  assert.equal(duplicate.state, 'uncertain'); assert.equal(duplicate.posted, false);
  assert.deepEqual(f.counts(), before); assert.equal(f.posts.length, 1);
  await assert.rejects(resumeRigNativeCodex({ ...f.input, prepare: true, nativeSessionId: operationId }, f.deps));
  await assert.rejects(resumeRigNativeCodex({ ...f.input, prepare: true, inspect: true }, f.deps));
  assert.equal(f.posts.length, 1);
});
posixTest('an unqualified Node runtime refuses before native metadata, owner observation or dispatch', async t => {
  const f = fixture(t), descriptor = Object.getOwnPropertyDescriptor(process.versions, 'node');
  if (!descriptor?.configurable) throw new Error('Native runtime version cannot be isolated for this control');
  Object.defineProperty(process.versions, 'node', { ...descriptor, value: '26.0.0' });
  try {
    await assert.rejects(resumeRigNativeCodex({ ...f.input, prepare: true }, f.deps), /native_resume_runtime_unqualified/);
    assert.deepEqual(f.counts(), { gets: 0, handoffs: 0, closed: 0 }); assert.equal(f.posts.length, 0);
  } finally { Object.defineProperty(process.versions, 'node', descriptor); }
});

posixTest('policy refusal precedes preparation and dispatch; a handoff-time change closes the channel', async t => {
  const f = fixture(t); f.deps.verifyPolicy = () => { throw Error('native_resume_policy_changed'); };
  await assert.rejects(resumeRigNativeCodex({ ...f.input, prepare: true }, f.deps), /native_resume_policy_changed/);
  assert.equal(f.posts.length, 0); assert.equal(f.counts().handoffs, 0); assert.equal(fs.existsSync(f.marker), false);
  const g = fixture(t); let checks = 0;
  g.deps.verifyPolicy = () => { if (++checks === 2) throw Error('native_resume_policy_changed'); };
  await assert.rejects(resumeRigNativeCodex(g.input, g.deps), /native_resume_policy_changed/);
  assert.equal(g.posts.length, 0); assert.equal(g.counts().closed, 1); assert.equal(fs.existsSync(g.marker), false);
});
posixTest('ordinary duplicate never rechecks policy or reopens a handoff', async t => {
  const f = fixture(t); await resumeRigNativeCodex(f.input, f.deps);
  f.deps.verifyPolicy = () => { throw Error('must not inspect an ordinary duplicate'); };
  const counts = f.counts(); assert.equal((await resumeRigNativeCodex(f.input, f.deps)).posted, false);
  assert.deepEqual(f.counts(), counts); assert.equal(f.posts.length, 1);
});

posixTest('new worker handoff returns pending before owner reply, then unready inspection neither posts nor observes the account', async t => {
  const f = fixture(t); let acknowledged!: (value: boolean) => void, posted!: () => void, aborted = false;
  const ack = new Promise<boolean>(resolve => { acknowledged = resolve; });
  const dispatch = new Promise<void>(resolve => { posted = resolve; });
  f.deps.handoff = async () => ({ alreadyDelivered: false, manager: f.manager, wait: () => ack, close: async () => {} });
  const normalFetch = f.deps.fetcher;
  f.deps.fetcher = (async (url: string | URL | Request, init?: RequestInit) => {
    if (init?.method !== 'POST') return normalFetch(url, init);
    // The owner has reserved this exact operation before requesting its one-use
    // environment. It deliberately has not finished its joined native proof.
    const text = String(init.body); f.posts.push(text);
    assert.equal(JSON.parse(artifacts.read(f.operationFile).toString()).requestText, text);
    assert.equal(fs.existsSync(f.marker), true);
    posted();
    return new Promise<Response>((_resolve, reject) => init.signal!.addEventListener('abort', () => { aborted = true; reject(Error('response observation cancelled')); }, { once: true }));
  }) as typeof fetch;
  const first = resumeRigNativeCodex(f.input, f.deps);
  await dispatch; acknowledged(true);
  const pending = await first;
  assert.equal(pending.state, 'uncertain'); assert.equal(pending.code, 'native_resume_pending'); assert.equal(pending.posted, true);
  assert.equal(aborted, true); assert.equal(f.posts.length, 1);
  f.ownerReady(false); f.deps.fetcher = normalFetch;
  f.deps.handoff = async () => { throw Error('An unjoined original must not reopen the owner account'); };
  const beforeWait = f.counts();
  const check = await resumeRigNativeCodex({ ...f.input, inspect: true }, f.deps);
  assert.equal(check.state, 'uncertain'); assert.equal(check.code, 'native_resume_pending'); assert.equal(check.posted, false);
  assert.equal(f.posts.length, 1);
  assert.equal(f.clock(), 3000, 'read-only readiness observation stays bounded');
  assert.equal(f.counts().gets - beforeWait.gets, 13);
  // Readiness permits only confirmation using the immutable body/UUID. The
  // owner's replay guard still supplies the decisive exact-original proof.
  f.ownerReady(true); f.replay();
  f.deps.handoff = async () => ({ alreadyDelivered: true, manager: f.manager, wait: async () => true, close: async () => {} });
  const complete = await resumeRigNativeCodex({ ...f.input, inspect: true }, f.deps);
  assert.equal(complete.state, 'confirmed'); assert.equal(complete.code, 'already_running_original');
  assert.equal(f.posts.length, 2); assert.equal(f.posts[1], f.posts[0]);
});

posixTest('explicit Check observes joining readiness before confirming only the original operation', async t => {
  const f = fixture(t); f.lose(); await resumeRigNativeCodex(f.input, f.deps);
  f.ownerReady(false); f.replay();
  const advance = f.deps.readinessWait, before = f.counts();
  f.deps.readinessWait = async milliseconds => {
    assert.equal(f.posts.length, 1, 'readiness observation dispatched another POST');
    assert.equal(f.counts().handoffs, before.handoffs, 'readiness observation reopened the account');
    await advance(milliseconds);
    if (f.clock() >= 1500) f.ownerReady(true);
  };
  const complete = await resumeRigNativeCodex({ ...f.input, inspect: true }, f.deps);
  assert.equal(complete.state, 'confirmed'); assert.equal(f.clock(), 1500);
  assert.equal(f.posts.length, 2); assert.equal(f.posts[1], f.posts[0]);
  assert.equal(f.counts().gets - before.gets, 7);
});

posixTest('readiness cannot confirm a foreign current conversation or skip original owner confirmation', async t => {
  const f = fixture(t); f.lose(); await resumeRigNativeCodex(f.input, f.deps);
  const normalFetch = f.deps.fetcher;
  f.deps.fetcher = (async (url: string | URL | Request, init?: RequestInit) => String(url).endsWith('/sessions')
    ? new Response(JSON.stringify([{ id: snapshotId, nodeId, status: 'running', sessionName: f.input.sessionName,
      resumeType: 'codex_id', resumeToken: operationId, startupStatus: 'ready' }])) : normalFetch(url, init)) as typeof fetch;
  const before = f.counts().handoffs;
  assert.equal((await resumeRigNativeCodex({ ...f.input, inspect: true }, f.deps)).state, 'uncertain');
  assert.equal(f.counts().handoffs, before); assert.equal(f.posts.length, 1);
  f.deps.fetcher = normalFetch; f.replay(); f.foreign();
  const result = await resumeRigNativeCodex({ ...f.input, inspect: true }, f.deps);
  assert.equal(result.state, 'uncertain', 'a ready token alone is never a confirmation'); assert.equal(f.posts.length, 2);
});

posixTest('explicit completion can deliver the first owner POST after a crash between local claim and HTTP dispatch', async t => {
  const f = fixture(t);
  await resumeRigNativeCodex({ ...f.input, prepare: true }, f.deps);
  const saved = JSON.parse(artifacts.read(f.operationFile).toString());
  artifacts.publish(f.marker, Buffer.from(JSON.stringify({ operationId, requestSHA256: createHash('sha256').update(saved.requestText).digest('hex') })));
  const originalFetch = f.deps.fetcher;
  f.deps.fetcher = (async (url: string | URL | Request, init?: RequestInit) => String(url).endsWith('/sessions')
    ? new Response('[]') : originalFetch(url, init)) as typeof fetch;
  const result = await resumeRigNativeCodex({ ...f.input, inspect: true }, f.deps);
  assert.equal(result.state, 'confirmed'); assert.equal(f.posts.length, 1);
  assert.equal(f.posts[0], saved.requestText); assert.equal(JSON.parse(f.posts[0]).operationId, operationId);
});
