import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { createRigNativeCodex, publishCodexLaunchBundle } from '../webmux/server/internal/openrigcoord/rig-native-create.mts';
import { projectCodexOwnerPlan } from '../webmux/server/internal/openrigcoord/codex-owner-plan.mts';

const posixTest = process.platform === 'win32' ? test.skip : test;
const rigId = '01JQPWV4E7QP26M799M3S0MR6E', nodeId = '01JQPWV4E7QP26M799M3S0MR6F';
function fixture(t: { after(fn: () => void): void }) {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'wm-native-create-')));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  fs.chmodSync(root, 0o700); const cwd = path.join(root, 'project'); fs.mkdirSync(cwd);
  const input = { directory: path.join(root, 'state'), cwd, rigId, pod: 'agents', member: 'alpha' };
  const plan = projectCodexOwnerPlan({}, { thread: { ephemeral: true }, cwd, model: 'owner-model', modelProvider: 'openai',
    reasoningEffort: 'medium', approvalPolicy: 'never', approvalsReviewer: 'user', sandbox: { type: 'dangerFullAccess' } },
  { requiresOpenaiAuth: true, userId: 'private-user', account: { type: 'chatgpt' }, workspaceRouting: { chatgptAccountId: 'private-workspace', backendOrigin: 'https://example.invalid', accountRoutingOverride: 'NO_CONSTRAINT' } },
  { cwd, home: root, executable: '/native/codex', executableSHA256: 'a'.repeat(64), profile: null });
  const posts: Array<{ id: string; text: string }> = []; let resolves = 0, reviews = 0, loseReply = false, changed = false;
  const manager = { port: 4242, managerPid: 123, managerIdentity: 'b'.repeat(64), processIdentity: 'c'.repeat(64) };
  const deps = { observationHelper: { contract: 'webmux-native-codex-observer/v1' as const, executable: '/observer/PRIVATE_HELPER_NEVER_EXPORT', executableSHA256: 'b'.repeat(64) }, handoffLifetime: 20, env: { HOME: root, PATH: process.env.PATH!, TOOL_API_KEY: 'PRIVATE_CREATE_VALUE_NEVER_SAVE' }, run: () => '[]', review: async () => { reviews++; return { ...manager }; },
    resolve: async () => { resolves++; if (changed && resolves > 1) { const { fingerprint: _old, ...content } = plan; content.model = 'different-owner-model'; return { ...content, fingerprint: createHash('sha256').update(JSON.stringify(content)).digest('hex') }; } return plan; },
    fetcher: (async (url: string | URL | Request, init?: RequestInit) => {
      if (String(url).endsWith('/capabilities')) return new Response(JSON.stringify({ contract: 'operation-correlation.v1', kinds: ['member.add'] }));
      const id = new Headers(init?.headers).get('X-OpenRig-Operation-Id')!; posts.push({ id, text: String(init?.body) });
      if (loseReply) throw new Error('response discarded');
      return new Response(JSON.stringify({ operation: { contract: 'operation-correlation.v1', operationId: id, rigId, nodeIds: [nodeId] } }), { status: 201 });
    }) as typeof fetch };
  return { root, input, plan, deps, manager, posts, counts: () => ({ resolves, reviews }), lose: () => { loseReply = true; }, restore: () => { loseReply = false; }, change: () => { changed = true; } };
}
posixTest('creation persists exact plan and immutable source bundle before one correlated POST', async t => {
  const f = fixture(t), result = await createRigNativeCodex(f.input, f.deps);
  assert.equal(result.state, 'recorded'); assert.equal(f.posts.length, 1); assert.deepEqual(f.counts(), { resolves: 2, reviews: 2 });
  const body = JSON.parse(f.posts[0].text), manifest = body.member.launch_wrapper[2];
  assert.equal(body.member.permission_policy, 'builtin:yolo'); assert.equal(body.member.codex_permissions, undefined);
  assert.equal(body.member.model, f.plan.model); assert.equal(body.member.effort, f.plan.effort);
  assert.deepEqual(body.member.env, { CODEX_HOME: f.plan.home });
  const envelope = JSON.parse(fs.readFileSync(manifest, 'utf8'));
  assert.equal(envelope.contract, 'webmux-codex-launch/v2'); assert.deepEqual(envelope.plan, f.plan);
  assert.equal(envelope.environment.values.HOME, f.root);
  assert.equal(fs.readFileSync(manifest, 'utf8').includes(f.deps.env.TOOL_API_KEY), false);
  assert.equal(f.posts[0].text.includes(f.deps.env.TOOL_API_KEY), false);
  assert.equal(JSON.stringify(envelope).includes('PRIVATE_HELPER_NEVER_EXPORT'), false);
  assert.equal(f.posts[0].text.includes('PRIVATE_HELPER_NEVER_EXPORT'), false);
  assert.equal(fs.statSync(manifest).mode & 0o777, 0o600); assert.equal(fs.statSync(path.dirname(manifest)).mode & 0o777, 0o700);
  const bundle = body.member.launch_wrapper[1]; assert.ok(fs.existsSync(bundle));
  assert.equal(JSON.stringify(result).includes('private-user'), false); assert.equal(JSON.stringify(result).includes(f.root), false);
});
posixTest('lost reply and duplicate creation never allocate another UUID, POST or resolver process', async t => {
  const f = fixture(t); f.lose(); const first = await createRigNativeCodex(f.input, f.deps);
  assert.equal(first.state, 'uncertain'); const before = f.counts();
  const second = await createRigNativeCodex(f.input, f.deps);
  assert.equal(second.posted, false); assert.equal(second.operationId, first.operationId); assert.equal(f.posts.length, 1);
  assert.equal(f.counts().resolves, before.resolves);
  f.restore(); const inspected = await createRigNativeCodex({ ...f.input, inspect: true }, f.deps);
  assert.equal(inspected.state, 'recorded'); assert.equal(inspected.operationId, first.operationId);
  assert.deepEqual(f.posts[1], f.posts[0]);
});
posixTest('changed owner or manager and an unrelated existing member refuse before dispatch', async t => {
  const f = fixture(t); f.change(); await assert.rejects(createRigNativeCodex(f.input, f.deps)); assert.equal(f.posts.length, 0);
  const other = fixture(t); other.deps.run = () => JSON.stringify([{ rigId, logicalId: 'agents.alpha' }]);
  await assert.rejects(createRigNativeCodex(other.input, other.deps), /native_create_member_already_exists/); assert.equal(other.posts.length, 0);
  const manager = fixture(t); let calls = 0;
  manager.deps.review = async () => ({ ...manager.manager, processIdentity: ++calls === 1 ? 'a'.repeat(64) : 'b'.repeat(64) });
  await assert.rejects(createRigNativeCodex(manager.input, manager.deps), /native_create_manager_changed/); assert.equal(manager.posts.length, 0);
});
posixTest('inspection without an original request and changed input never create a new request', async t => {
  const f = fixture(t); await assert.rejects(createRigNativeCodex({ ...f.input, inspect: true }, f.deps), /original_request_missing/);
  await createRigNativeCodex(f.input, f.deps); const other = path.join(f.root, 'other'); fs.mkdirSync(other);
  await assert.rejects(createRigNativeCodex({ ...f.input, cwd: other }, f.deps), /original_request_changed/); assert.equal(f.posts.length, 1);
});
posixTest('bundle files are reused unchanged and corruption is never overwritten', t => {
  const f = fixture(t), first = publishCodexLaunchBundle(f.input.directory), before = fs.statSync(first.wrapper).mtimeMs;
  const second = publishCodexLaunchBundle(f.input.directory); assert.deepEqual(second, first); assert.equal(fs.statSync(first.wrapper).mtimeMs, before);
  fs.appendFileSync(first.wrapper, '\n// corrupted\n'); assert.throws(() => publishCodexLaunchBundle(f.input.directory));
  assert.ok(fs.readFileSync(first.wrapper, 'utf8').endsWith('// corrupted\n'));
});
posixTest('inspection refuses changed agent configuration without replaying the original request', async t => {
  const f = fixture(t); f.lose(); await createRigNativeCodex(f.input, f.deps);
  const body = JSON.parse(f.posts[0].text), agent = body.member.agent_ref.slice('path:'.length);
  fs.appendFileSync(path.join(agent, 'agent.yaml'), '\ncorrupted: true\n');
  await assert.rejects(createRigNativeCodex({ ...f.input, inspect: true }, f.deps), /original_request_changed/);
  assert.equal(f.posts.length, 1);
});
posixTest('changed execution environment blocks inspection without a new operation or replay', async t => {
  const f = fixture(t); f.lose(); const original = await createRigNativeCodex(f.input, f.deps);
  f.deps.env.TOOL_API_KEY = 'CHANGED_PRIVATE_VALUE';
  const duplicate = await createRigNativeCodex(f.input, f.deps);
  assert.equal(duplicate.operationId, original.operationId); assert.equal(duplicate.posted, false);
  await assert.rejects(createRigNativeCodex({ ...f.input, inspect: true }, f.deps), /native_create_owner_environment_changed/);
  assert.equal(f.posts.length, 1);
});
posixTest('parent-directory flush failure prevents all native mutation', async t => {
  const f = fixture(t), original = fs.fsyncSync; let injected = false;
  fs.fsyncSync = fd => { if (!injected && fs.fstatSync(fd).isDirectory()) { injected = true; throw Object.assign(new Error('injected flush error'), { code: 'EIO' }); } return original(fd); };
  try { await assert.rejects(createRigNativeCodex(f.input, f.deps), /injected flush error/); }
  finally { fs.fsyncSync = original; }
  assert.equal(injected, true); assert.equal(f.posts.length, 0);
});
