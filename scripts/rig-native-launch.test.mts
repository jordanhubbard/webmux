// Native protocol fixtures; actual coding is qualified separately.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { launchRigNativeCodex, observeNativeCodexPane } from '../webmux/server/internal/openrigcoord/rig-native-launch.mts';
import { nativeCreateArtifacts as artifacts, type createRigNativeCodex } from '../webmux/server/internal/openrigcoord/rig-native-create.mts';
import { projectCodexOwnerPlan } from '../webmux/server/internal/openrigcoord/codex-owner-plan.mts';
import { captureCodexLaunchEnvironment } from '../webmux/server/internal/openrigcoord/codex-launch-environment.mts';

const posixTest = process.platform === 'win32' ? test.skip : test;
const rigId = '01JQPWV4E7QP26M799M3S0MR6E', nodeId = '01JQPWV4E7QP26M799M3S0MR6F';
const sessionId = '01JQPWV4E7QP26M799M3S0MR6G', operationId = '9aa1753a-1d61-4420-af42-e6b6fa591c47';
function fixture(t: { after(fn: () => void): void }) {
  const root = artifacts.ownedRoot(fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'wm-native-launch-'))));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const cwd = path.join(root, 'project'); fs.mkdirSync(cwd);
  const input = { directory: artifacts.ownedRoot(path.join(root, 'state')), cwd, rigId, pod: 'agents', member: 'alpha' };
  const key = createHash('sha256').update(JSON.stringify([rigId, input.pod, input.member])).digest('hex');
  const directory = artifacts.ownedRoot(path.join(artifacts.ownedRoot(path.join(input.directory, 'requests')), key));
  const wrapper = [process.execPath, path.join(input.directory, 'launchers', 'original', 'codex-launch-wrapper.mts'), path.join(directory, 'launch-manifest.json')];
  const manager = { port: 4242, managerPid: 123, managerIdentity: 'b'.repeat(64), processIdentity: 'c'.repeat(64) };
  const plan = projectCodexOwnerPlan({}, { thread: { ephemeral: true }, cwd, model: 'owner-model', modelProvider: 'openai',
    reasoningEffort: null, approvalPolicy: 'never', approvalsReviewer: 'user', sandbox: { type: 'dangerFullAccess' } },
  { requiresOpenaiAuth: true, userId: 'private-user', account: { type: 'chatgpt' }, workspaceRouting: { chatgptAccountId: 'private-workspace', backendOrigin: 'https://example.invalid', accountRoutingOverride: 'NO_CONSTRAINT' } },
  { cwd, home: root, executable: '/native/codex', executableSHA256: 'a'.repeat(64), profile: null });
  const environment = captureCodexLaunchEnvironment({ HOME: root, PATH: process.env.PATH }, root);
  let creates = 0, ready = true, lose = false, observations = 0;
  const node = { nodeId, rigId, logicalId: 'agents.alpha', runtime: 'codex', cwd, canonicalSessionName: 'agents-alpha@qualified',
    sessionStatus: 'running', attachmentType: 'tmux', tmuxServerArgs: ['-S', '/private/fixture/socket'], launchWrapper: wrapper };
  const session = { id: sessionId, nodeId, sessionName: node.canonicalSessionName, status: 'running', origin: 'launched' };
  const create: typeof createRigNativeCodex = async () => {
    creates++;
    if (lose) return { state: 'uncertain', code: 'native_create_response_unavailable', operationId, posted: true, logicalId: node.logicalId, planFingerprint: plan.fingerprint };
    const intent = { managerIdentity: manager.managerIdentity, port: manager.port, rigId, pod: input.pod,
      requestText: JSON.stringify({ member: { id: input.member, runtime: 'codex', launch_wrapper: wrapper } }) };
    artifacts.publish(path.join(directory, 'launch.json'), Buffer.from(JSON.stringify({ contract: 'webmux-native-codex-create/v2', cwd, member: input.member, plan, environment, intent })));
    const journal = artifacts.ownedRoot(path.join(directory, 'journal'));
    artifacts.publish(path.join(journal, key + '.intent.json'), Buffer.from(JSON.stringify({ version: 1, operationId, intent })));
    artifacts.publish(path.join(directory, 'environment-consumed.json'), Buffer.from(JSON.stringify({ contract: 'webmux-codex-environment-consumed/v1', fingerprint: environment.fingerprint })));
    return { state: 'recorded', code: 'native_enrollment_recorded', rigId, nodeIds: [nodeId], operationId, posted: true, logicalId: node.logicalId, planFingerprint: plan.fingerprint };
  };
  const deps = { run: () => '', create, review: async () => manager,
    observe: async () => { observations++; return ready; },
    fetcher: (async (url: string | URL | Request) => new Response(JSON.stringify(String(url).endsWith('/nodes') ? [node] : [session]))) as typeof fetch };
  return { input, deps, node, session, directory, key, counts: () => ({ creates, observations }), notReady: () => { ready = false; }, ready: () => { ready = true; }, lose: () => { lose = true; } };
}
posixTest('native enrollment remains pending until its actual process is verified; duplicates only observe', async t => {
  const f = fixture(t); f.notReady();
  assert.equal((await launchRigNativeCodex(f.input, f.deps)).state, 'pending');
  assert.equal(f.counts().creates, 1);
  f.ready(); const ready = await launchRigNativeCodex({ ...f.input, inspect: true }, f.deps);
  assert.equal(ready.state, 'attached');
  if (ready.state === 'attached') { assert.equal(ready.nodeId, nodeId); assert.equal(ready.sessionId, sessionId); assert.equal(ready.tmuxServerValue, '/private/fixture/socket'); }
  assert.equal(f.counts().creates, 1);
  f.notReady(); assert.equal((await launchRigNativeCodex(f.input, f.deps)).state, 'pending');
  assert.equal(f.counts().creates, 1, 'Lost provider must not become a replacement launch');
});
posixTest('lost native enrollment reply creates no attachment and does not infer a node', async t => {
  const f = fixture(t); f.lose(); const result = await launchRigNativeCodex(f.input, f.deps);
  assert.equal(result.state, 'pending'); assert.equal(result.operationId, operationId); assert.equal(f.counts().observations, 0);
});
posixTest('claimed and stale original registrations cannot become native ready attachments', async t => {
  const f = fixture(t); f.session.origin = 'claimed';
  assert.equal((await launchRigNativeCodex(f.input, f.deps)).state, 'pending'); assert.equal(f.counts().observations, 0);
  f.session.origin = 'launched'; f.node.nodeId = sessionId;
  assert.equal((await launchRigNativeCodex(f.input, f.deps)).state, 'pending'); assert.equal(f.counts().creates, 1);
});
posixTest('missing or changed original creation ledger refuses without creating again', async t => {
  const f = fixture(t); await launchRigNativeCodex(f.input, f.deps);
  fs.unlinkSync(path.join(f.directory, 'journal', f.key + '.intent.json'));
  await assert.rejects(launchRigNativeCodex(f.input, f.deps)); assert.equal(f.counts().creates, 1);
});
posixTest('process observer verifies a real loaded executable and rejects a changed digest', async () => {
  const executable = fs.realpathSync(process.execPath), digest = createHash('sha256').update(fs.readFileSync(executable)).digest('hex');
  const run = (command: string, args: string[]) => command === 'tmux' ? String(process.pid)
    : execFileSync(command, args, { encoding: 'utf8', timeout: 5000, maxBuffer: 4 * 1024 * 1024 });
  const attachment = { tmuxTarget: 'owned-test-pane', tmuxServerFlag: '-S', tmuxServerValue: '/private/fixture/socket' };
  assert.equal(await observeNativeCodexPane(run, attachment, executable, digest), true);
  assert.equal(await observeNativeCodexPane(run, attachment, executable, '0'.repeat(64)), false);
});
