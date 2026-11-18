import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { captureRigNativeCodex, verifyRigNativeCodexReference, observeRigNativeOriginalAttachment } from '../webmux/server/internal/openrigcoord/rig-native-capture.mts';
import { nativeCreateArtifacts as a } from '../webmux/server/internal/openrigcoord/rig-native-create.mts';
import { projectCodexOwnerPlan } from '../webmux/server/internal/openrigcoord/codex-owner-plan.mts';
import type { launchRigNativeCodex } from '../webmux/server/internal/openrigcoord/rig-native-launch.mts';
const posix = process.platform === 'win32' ? test.skip : test;
const rigId = '01JQPWV4E7QP26M799M3S0MR6E', nodeId = '01JQPWV4E7QP26M799M3S0MR6F', historyId = '01JQPWV4E7QP26M799M3S0MR6G', snapshotId = '01JQPWV4E7QP26M799M3S0MR6H';
const nativeId = '01a1eff2-f58a-7082-be6b-69f9eca4a07c', generationId = '9aa1753a-1d61-4420-af42-e6b6fa591c47';
function fixture(t: { after(fn: () => void): void }) {
  const root = a.ownedRoot(fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'wm-native-capture-')))); t.after(() => fs.rmSync(root, { force: true, recursive: true }));
  const cwd = path.join(root, 'project'); fs.mkdirSync(cwd); const home = path.join(root, 'native'); fs.mkdirSync(home); fs.mkdirSync(path.join(home, 'sessions'));
  const file = path.join(home, 'sessions', 'original.jsonl'); fs.writeFileSync(file, JSON.stringify({ type: 'session_meta', payload: { id: nativeId, cwd } }) + '\n' + 'private conversation bytes must not be read by a remote metadata query\n');
  const directory = a.ownedRoot(path.join(root, 'state')), key = createHash('sha256').update(JSON.stringify([rigId, 'agents', 'alpha'])).digest('hex');
  const owned = a.ownedRoot(path.join(a.ownedRoot(path.join(directory, 'requests')), key));
  const wrapper = ['/native/node', '/owned/wrapper', path.join(owned, 'launch-manifest.json')];
  const manager = { port: 4242, managerPid: 123, managerIdentity: 'b'.repeat(64), processIdentity: 'c'.repeat(64) };
  const plan = projectCodexOwnerPlan({}, { thread: { ephemeral: true }, cwd, model: 'owner-model', modelProvider: 'openai', reasoningEffort: null, approvalPolicy: 'never', approvalsReviewer: 'user', sandbox: { type: 'dangerFullAccess' } },
    { requiresOpenaiAuth: true, userId: 'private-user', account: { type: 'chatgpt' }, workspaceRouting: { chatgptAccountId: 'private-workspace', backendOrigin: 'https://example.invalid', accountRoutingOverride: 'NO_CONSTRAINT' } },
    { cwd, home, executable: '/native/codex', executableSHA256: 'a'.repeat(64), profile: null });
  a.publish(path.join(owned, 'native-registration.json'), Buffer.from(JSON.stringify({ rigId, nodeId, pod: 'agents', member: 'alpha', cwd })));
  a.publish(path.join(owned, 'launch.json'), Buffer.from(JSON.stringify({ plan, intent: { port: manager.port, managerIdentity: manager.managerIdentity, requestText: JSON.stringify({ member: { launch_wrapper: wrapper } }) } })));
  const target = 'agents-alpha@qualified', identity = { target, project: 'qualified', name: 'alpha', host: 'localhost', runtime: 'codex', workspace: cwd, tmux_server_flag: '-S', tmux_server_value: '/owned/socket' };
  const input = { directory, identity, rigId, nodeId, pod: 'agents', member: 'alpha', cwd };
  const stat = fs.statSync(file), record = { runtime: 'codex', sessionId: nativeId, generationId: JSON.stringify([generationId, nativeId, file, stat.dev, stat.ino, stat.birthtimeMs]) };
  const snapshot = { id: snapshotId, rigId, data: { nodes: [{ id: nodeId, rigId, logicalId: 'agents.alpha', runtime: 'codex', cwd, launchWrapper: wrapper }],
    sessions: [{ id: historyId, nodeId, sessionName: target, origin: 'launched', resumeType: 'codex_id', resumeToken: nativeId }], activeSessionIdByNode: { [nodeId]: historyId } } };
  let posts = 0, records = 0, launches = 0, ready = true, switchAfterRead = false;
  const launch: typeof launchRigNativeCodex = async () => { launches++; return ready ? { state: 'attached', operationId: generationId, nodeId, sessionId: historyId, target, tmuxServerFlag: '-S', tmuxServerValue: identity.tmux_server_value } : { state: 'pending', code: 'not_ready', operationId: generationId }; };
  const deps = { run: () => { throw Error('No command may run in this fixture'); }, launch, review: async () => manager,
    fetcher: (async (url: string | URL | Request, init?: RequestInit) => {
      assert.equal(new URL(String(url)).search, '', 'No transcript suffix or arbitrary query is permitted');
      if (String(url).endsWith('/generation-record')) {
        records++; return new Response(JSON.stringify({ ...record, ...(switchAfterRead && records > 1 ? { generationId: record.generationId.replace(generationId, nativeId) } : {}) }));
      }
      if (init?.method === 'POST') posts++;
      return new Response(JSON.stringify(snapshot));
    }) as typeof fetch };
  return { input, deps, record, snapshot, owned, file, counts: () => ({ posts, records, launches }), switch: () => { switchAfterRead = true; }, unready: () => { ready = false; } };
}
posix('capture joins original registration, live attachment, native generation/header and snapshot; repeats reuse one snapshot', async t => {
  const f = fixture(t), first = await captureRigNativeCodex(f.input, f.deps);
  assert.equal(first.nativeSessionId, nativeId); assert.equal(first.historySessionId, historyId); assert.equal(first.nativeStateFile, f.file);
  assert.equal(f.counts().posts, 1); assert.equal(f.counts().records, 2);
  assert.deepEqual(await captureRigNativeCodex(f.input, f.deps), first); assert.equal(f.counts().posts, 1);
  const cache = fs.readdirSync(path.join(f.owned, 'recovery-captures')); assert.equal(cache.length, 1);
  assert.equal(fs.statSync(path.join(f.owned, 'recovery-captures', cache[0])).mode & 0o777, 0o600);
});
posix('capture refuses missing original registration and an unverified provider without creating a snapshot', async t => {
  const f = fixture(t); f.unready(); await assert.rejects(captureRigNativeCodex(f.input, f.deps)); assert.equal(f.counts().posts, 0); assert.equal(f.counts().records, 0);
  fs.unlinkSync(path.join(f.owned, 'native-registration.json')); await assert.rejects(captureRigNativeCodex(f.input, f.deps)); assert.equal(f.counts().launches, 1);
});
posix('foreign native header, file identity and owner destination refuse before snapshot creation', async t => {
  const f = fixture(t); fs.writeFileSync(f.file, JSON.stringify({ type: 'session_meta', payload: { id: nativeId, cwd: '/foreign' } }) + '\n');
  await assert.rejects(captureRigNativeCodex(f.input, f.deps)); assert.equal(f.counts().posts, 0);
  const g = fixture(t); const tuple = JSON.parse(g.record.generationId) as unknown[]; tuple[4] = 1; g.record.generationId = JSON.stringify(tuple);
  await assert.rejects(captureRigNativeCodex(g.input, g.deps)); assert.equal(g.counts().posts, 0);
  await assert.rejects(captureRigNativeCodex({ ...g.input, identity: { ...g.input.identity, host: 'remote' } }, g.deps));
});
posix('foreign snapshot conversation/wrapper and a changed generation cannot become recovery authority', async t => {
  const f = fixture(t); f.snapshot.data.sessions[0].resumeToken = generationId;
  await assert.rejects(captureRigNativeCodex(f.input, f.deps)); assert.equal(fs.readdirSync(path.join(f.owned, 'recovery-captures')).length, 0);
  const g = fixture(t); g.snapshot.data.nodes[0].launchWrapper[2] = '/foreign/manifest'; await assert.rejects(captureRigNativeCodex(g.input, g.deps));
  const h = fixture(t); h.switch(); await assert.rejects(captureRigNativeCodex(h.input, h.deps)); assert.equal(fs.readdirSync(path.join(h.owned, 'recovery-captures')).length, 0);
});

posix('a banked capture remains verifiable after provider loss without selecting or launching a current generation', async t => {
  const f = fixture(t), reference = await captureRigNativeCodex(f.input, f.deps), before = f.counts(); f.unready();
  await verifyRigNativeCodexReference(f.input, reference, f.deps);
  assert.deepEqual(f.counts(), before, 'Historical revalidation must not probe a new live generation or create a snapshot');
});
posix('banked reference rejects replaced native files, changed snapshots and a foreign selection', async t => {
  const f = fixture(t), reference = await captureRigNativeCodex(f.input, f.deps);
  await assert.rejects(verifyRigNativeCodexReference(f.input, { ...reference, nodeId: snapshotId }, f.deps));
  f.snapshot.data.nodes[0].cwd = '/foreign'; await assert.rejects(verifyRigNativeCodexReference(f.input, reference, f.deps));
  const g = fixture(t), other = await captureRigNativeCodex(g.input, g.deps);
  const old = g.file + '.retained'; fs.renameSync(g.file, old); fs.writeFileSync(g.file, fs.readFileSync(old));
  await assert.rejects(verifyRigNativeCodexReference(g.input, other, g.deps));
});

posix('survivor attachment verifies the banked native conversation and never becomes a new snapshot or launch', async t => {
  const f = fixture(t), reference = await captureRigNativeCodex(f.input, f.deps);
  assert.equal((await observeRigNativeOriginalAttachment(f.input, reference, f.deps))?.state, 'attached');
  assert.equal(f.counts().posts, 1); f.unready();
  assert.equal(await observeRigNativeOriginalAttachment(f.input, reference, f.deps), undefined); assert.equal(f.counts().posts, 1);
  const g = fixture(t), original = await captureRigNativeCodex(g.input, g.deps);
  g.record.sessionId = generationId;
  await assert.rejects(observeRigNativeOriginalAttachment(g.input, original, g.deps)); assert.equal(g.counts().posts, 1);
});
