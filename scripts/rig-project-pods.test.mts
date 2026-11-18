import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readRigProjectPods } from '../webmux/server/internal/openrigcoord/rig-project-pods.mts';

const id = '01JQPWV4E7QP26M799M3S0MR6E';
function fixture() {
  const manager = { port: 4242, managerPid: 123, managerIdentity: 'a'.repeat(64), processIdentity: 'b'.repeat(64) };
  const state: { rigs: unknown; spec: unknown; changed: boolean; reads: string[]; reviews: number; invalidJSON: boolean; oversized: boolean } = {
    rigs: [{ id, name: 'empty-project' }], spec: { name: 'empty-project', pods: [{ id: 'workers', members: [{ id: 'existing' }] }, { id: 'research', members: [] }] },
    changed: false, reads: [], reviews: 0, invalidJSON: false, oversized: false,
  };
  const deps = {
    run: () => { throw new Error('Namespace discovery must not execute an account observer or provider'); },
    review: async () => ({ ...manager, ...(state.changed && state.reviews++ ? { processIdentity: 'c'.repeat(64) } : {}) }),
    fetcher: (async (url: string | URL | Request, init?: RequestInit) => {
      const u = new URL(String(url)); assert.equal(u.origin, 'http://127.0.0.1:4242'); assert.equal(u.search, '');
      assert.equal(init?.method ?? 'GET', 'GET'); assert.equal(init?.redirect, 'error'); state.reads.push(u.pathname);
      return new Response(state.oversized ? 'x'.repeat(1048577) : state.invalidJSON ? 'PRIVATE-MALFORMED-SPEC' : JSON.stringify(u.pathname === '/api/rigs' ? state.rigs : state.spec));
    }) as typeof fetch,
  };
  return { state, deps };
}
test('canonical empty and occupied pods are read without requiring a seat or invoking a provider', async () => {
  const f = fixture();
  assert.deepEqual(await readRigProjectPods(id, 'empty-project', f.deps), { projectId: id, projectName: 'empty-project', pods: ['research', 'workers'], source: 'native-spec' });
  assert.deepEqual(f.state.reads, ['/api/rigs', `/api/rigs/${id}/spec.json`]);
  f.state.spec = { name: 'empty-project', pods: [] };
  assert.deepEqual((await readRigProjectPods(id, 'empty-project', f.deps)).pods, [], 'No default pod may be invented');
});
test('archived/missing, duplicated and renamed project identities refuse before reading their spec', async () => {
  for (const rigs of [[], [{ id, name: 'renamed' }], [{ id, name: 'empty-project' }, { id, name: 'empty-project' }]]) {
    const f = fixture(); f.state.rigs = rigs;
    await assert.rejects(readRigProjectPods(id, 'empty-project', f.deps), /namespace_unverified/);
    assert.deepEqual(f.state.reads, ['/api/rigs']);
  }
});
test('foreign spec, ambiguous/invalid pod and changed manager cannot supply launch choices', async () => {
  for (const spec of [{ name: 'foreign', pods: [] }, { name: 'empty-project', pods: [{ id: 'research' }, { id: 'research' }] }, { name: 'empty-project', pods: [{ id: '../foreign' }] }]) {
    const f = fixture(); f.state.spec = spec;
    await assert.rejects(readRigProjectPods(id, 'empty-project', f.deps), /namespace_unverified/);
  }
  const f = fixture(); f.state.changed = true;
  await assert.rejects(readRigProjectPods(id, 'empty-project', f.deps), /namespace_unverified/);
});
test('unbounded or malformed native replies refuse without returning private spec text', async () => {
  for (const mode of ['invalidJSON', 'oversized'] as const) {
    const f = fixture(); f.state[mode] = true;
    await assert.rejects(readRigProjectPods(id, 'empty-project', f.deps), error => error instanceof Error && error.message === 'native_project_namespace_unverified');
  }
  const f = fixture();
  await assert.rejects(readRigProjectPods('../foreign', 'empty-project', f.deps), /namespace_unverified/);
  assert.equal(f.state.reads.length, 0);
});


test('a larger canonical topology retains every pod within the bounded response', async () => {
  const f = fixture();
  f.state.spec = { name: 'empty-project', pods: Array.from({ length: 130 }, (_, i) => ({ id: `pod-${i}`, members: [] })) };
  const result = await readRigProjectPods(id, 'empty-project', f.deps);
  assert.equal(result.pods.length, 130);
  assert(result.pods.includes('pod-129'));
});
