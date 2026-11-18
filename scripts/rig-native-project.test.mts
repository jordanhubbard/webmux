import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createRigNativeProject } from '../webmux/server/internal/openrigcoord/rig-native-project.mts';
import { dispatchRigTopologyIntent, type RigTopologyIntent } from '../webmux/server/internal/openrigcoord/rig-create-intent.mts';
const posixTest = process.platform === 'win32' ? test.skip : test;
const rigId = '01JQPWV4E7QP26M799M3S0MR6E';
function fixture(t: { after(fn: () => void): void }) {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'wm-native-project-')));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const input = { directory: path.join(root, 'state'), name: 'fixture', cwd: root };
  let lost = false, present = true, supported = true, pod = true;
  const posts: Array<{ id: string; body: string; root: string; cwd: string }> = [];
  const manager = { port: 4242, managerPid: 123, managerIdentity: 'a'.repeat(64), processIdentity: 'b'.repeat(64) };
  const deps = { run: () => '[]', review: async () => ({ ...manager }), fetcher: (async (url: string | URL | Request, init?: RequestInit) => {
    const endpoint = new URL(String(url)); assert.equal(endpoint.origin, 'http://127.0.0.1:4242');
    assert.equal(init?.redirect, 'error'); assert.ok(init?.signal);
    if (endpoint.pathname === '/api/operations/capabilities') return Response.json({ contract: 'operation-correlation.v1', kinds: supported ? ['rig.materialize'] : ['member.add'] });
    if (endpoint.pathname === '/api/rigs') return Response.json(present ? [{ id: rigId, name: input.name, lifecycleState: 'stopped' }] : []);
    if (endpoint.pathname.endsWith('/spec.json')) return Response.json({ name: input.name, version: '0.2', pods: pod ? [{ id: 'agents', members: [] }] : [] });
    assert.equal(endpoint.pathname, '/api/rigs/import/materialize'); assert.equal(init?.method, 'POST');
    const headers = new Headers(init.headers), body = String(init.body), directory = headers.get('X-Rig-Root')!;
    const records = fs.readdirSync(path.join(directory, 'journal'));
    assert.equal(records.filter(file => file.endsWith('.dispatched')).length, 1);
    assert.equal(fs.readFileSync(path.join(directory, 'rig.json'), 'utf8'), body);
    assert.deepEqual(JSON.parse(body).pods[0].members, []);
    posts.push({ id: headers.get('X-OpenRig-Operation-Id')!, body, root: directory, cwd: headers.get('X-Cwd-Override')! });
    if (lost) throw new Error('lost reply');
    return Response.json({ operation: { contract: 'operation-correlation.v1', operationId: posts.at(-1)!.id, rigId, nodeIds: [], current: { rig: { id: rigId, exists: true, archivedAt: null } } } }, { status: 201 });
  }) as typeof fetch };
  return { root, input, manager, deps, posts, lose: (value: boolean) => { lost = value; }, missing: () => { present = false; }, noPod: () => { pod = false; }, unsupported: () => { supported = false; } };
}
posixTest('empty stopped topology is durably recorded and reused without another POST', async t => {
  const f = fixture(t), first = await createRigNativeProject(f.input, f.deps);
  assert.equal(first.state, 'recorded'); assert.equal(first.rigId, rigId); assert.deepEqual(first.nodeIds, []);
  const duplicate = await createRigNativeProject(f.input, f.deps);
  assert.equal(duplicate.state, 'recorded'); assert.equal(duplicate.posted, false); assert.equal(duplicate.operationId, first.operationId);
  assert.equal(f.posts.length, 1); assert.equal(f.posts[0].cwd, f.root);
});
posixTest('lost topology reply stays consumed; explicit inspection uses the original UUID, bytes and headers', async t => {
  const f = fixture(t); f.lose(true); const first = await createRigNativeProject(f.input, f.deps);
  assert.equal(first.state, 'uncertain');
  const duplicate = await createRigNativeProject(f.input, f.deps); assert.equal(duplicate.posted, false); assert.equal(f.posts.length, 1);
  f.lose(false); const inspected = await createRigNativeProject({ ...f.input, inspect: true }, f.deps);
  assert.equal(inspected.state, 'recorded'); assert.equal(inspected.operationId, first.operationId); assert.deepEqual(f.posts[1], f.posts[0]);
});
posixTest('missing or changed native namespace preserves the original allocation instead of replacing it', async t => {
  for (const change of ['rig', 'pod']) {
    const f = fixture(t), first = await createRigNativeProject(f.input, f.deps);
    if (change === 'rig') f.missing(); else f.noPod();
    const result = await createRigNativeProject(f.input, f.deps);
    assert.equal(result.state, 'uncertain'); assert.equal(result.code, 'native_project_namespace_unverified');
    assert.equal(result.operationId, first.operationId); assert.equal(result.rigId, first.rigId); assert.equal(f.posts.length, 1);
  }
});
posixTest('inspection without an original project and changed cwd refuse before dispatch', async t => {
  const f = fixture(t); await assert.rejects(createRigNativeProject({ ...f.input, inspect: true }, f.deps), /original_request_missing/);
  await createRigNativeProject(f.input, f.deps); const other = path.join(f.root, 'other'); fs.mkdirSync(other);
  await assert.rejects(createRigNativeProject({ ...f.input, cwd: other }, f.deps), /original_request_changed/); assert.equal(f.posts.length, 1);
});
posixTest('manager drift and unsupported materialization correlation never dispatch', async t => {
  const f = fixture(t); let n = 0;
  f.deps.review = async () => ({ ...f.manager, processIdentity: ++n === 1 ? 'c'.repeat(64) : 'd'.repeat(64) });
  await assert.rejects(createRigNativeProject(f.input, f.deps), /manager_changed/); assert.equal(f.posts.length, 0);
  const unsupported = fixture(t); unsupported.unsupported(); await assert.rejects(createRigNativeProject(unsupported.input, unsupported.deps), /correlation_unavailable/);
  assert.equal(unsupported.posts.length, 0);
});
posixTest('corrupt project receipt cannot redirect a known purpose to another rig', async t => {
  const f = fixture(t); await createRigNativeProject(f.input, f.deps);
  const directory = f.posts[0].root, file = path.join(directory, 'enrollment.json'), saved = JSON.parse(fs.readFileSync(file, 'utf8'));
  saved.rigId = '01JQPWV4E7QP26M799M3S0MR6F'; fs.writeFileSync(file, JSON.stringify(saved));
  await assert.rejects(createRigNativeProject(f.input, f.deps), /receipt_invalid/); assert.equal(f.posts.length, 1);
});
posixTest('topology journal refuses coding members or extra launch configuration', async t => {
  for (const extra of ['member', 'env']) {
    const f = fixture(t), request = { version: '0.2', name: 'fixture', summary: 'Coordinated project fixture', pods: [{ id: 'agents', label: 'fixture', members: [] as object[] }], edges: [] };
    if (extra === 'member') request.pods[0].members.push({ id: 'coding', runtime: 'codex' }); else Object.assign(request, { env: { KEY: 'never-launch' } });
    const intent: RigTopologyIntent = { kind: 'rig.materialize', managerIdentity: f.manager.managerIdentity, port: 4242, rigRoot: f.root, cwd: f.root, requestText: JSON.stringify(request) };
    await assert.rejects(dispatchRigTopologyIntent(path.join(f.root, 'journal'), 'a'.repeat(64), intent, f.deps.fetcher), /invalid_native_create_intent/);
    assert.equal(f.posts.length, 0); assert.equal(fs.existsSync(path.join(f.root, 'journal')), false);
  }
});
