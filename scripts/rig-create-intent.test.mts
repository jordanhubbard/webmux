import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { reserveRigCreateIntent, dispatchRigCreateIntent, inspectRigCreateIntent, type RigCreateIntent } from '../webmux/server/internal/openrigcoord/rig-create-intent.mts';

const posixTest = process.platform === 'win32' ? test.skip : test;
const key = 'a'.repeat(64);
const rigId = '01M71ZM6DE3RTCXHPJRXJ6XKPE';
const nodeId = '01M71ZM6TZ91BH689AGCFP2WCN';
function fixture(t: { after: (fn: () => void) => void }) {
  const parent = fs.mkdtempSync(path.join(os.tmpdir(), 'wm-create-intent-'));
  const directory = path.join(parent, 'journal');
  t.after(() => fs.rmSync(parent, { recursive: true, force: true }));
  const intent: RigCreateIntent = { managerIdentity: 'b'.repeat(64), port: 43210, rigId, pod: 'agents',
    requestText: '{ "member": { "id": "worker", "runtime": "terminal" } }' };
  const posts: RequestInit[] = [];
  let lost = false, supported = true, empty = false, wrong = false;
  const fetcher = (async (address: string | URL | Request, init?: RequestInit) => {
    const url = new URL(String(address));
    assert.equal(url.origin, 'http://127.0.0.1:43210');
    assert.equal(init?.redirect, 'error'); assert.ok(init?.signal);
    if (init?.method === 'GET') return Response.json({ contract: 'operation-correlation.v1', kinds: supported ? ['member.add'] : [] });
    assert.equal(url.pathname, `/api/rigs/${rigId}/pods/agents/members`);
    assert.equal(init?.body, intent.requestText);
    assert.equal(fs.statSync(path.join(directory, key + '.dispatched')).mode & 0o777, 0o600);
    const saved = JSON.parse(fs.readFileSync(path.join(directory, key + '.intent.json'), 'utf8')) as { operationId: string };
    assert.equal(new Headers(init?.headers).get('X-OpenRig-Operation-Id'), saved.operationId);
    posts.push(init!);
    if (lost) throw new Error('lost response after native effect');
    return Response.json({ operation: { contract: 'operation-correlation.v1', operationId: saved.operationId,
      rigId: wrong ? nodeId : rigId, nodeIds: empty ? [] : [nodeId] } }, { status: 201 });
  }) as typeof fetch;
  return { parent, directory, intent, fetcher, posts, lose: (v: boolean) => { lost = v; },
    unsupported: () => { supported = false; }, empty: () => { empty = true; }, wrong: () => { wrong = true; } };
}

posixTest('exact bytes and operation survive a separate client process before dispatch', t => {
  const f = fixture(t);
  const saved = reserveRigCreateIntent(f.directory, key, f.intent);
  const source = new URL('../webmux/server/internal/openrigcoord/rig-create-intent.mts', import.meta.url).href;
  const child = spawnSync(process.execPath, ['--input-type=module', '-e',
    `import {reserveRigCreateIntent} from ${JSON.stringify(source)}; console.log(JSON.stringify(reserveRigCreateIntent(process.argv[1],process.argv[2],JSON.parse(process.argv[3]))));`,
    f.directory, key, JSON.stringify(f.intent)], { encoding: 'utf8' });
  assert.equal(child.status, 0, child.stderr);
  assert.deepEqual(JSON.parse(child.stdout), saved);
  assert.equal(fs.statSync(f.directory).mode & 0o777, 0o700);
});
posixTest('first dispatch is recorded before HTTP; ordinary duplicate requests never post again', async t => {
  const f = fixture(t);
  const first = await dispatchRigCreateIntent(f.directory, key, f.intent, f.fetcher);
  assert.equal(first.state, 'recorded'); assert.deepEqual(first.nodeIds, [nodeId]);
  const duplicate = await dispatchRigCreateIntent(f.directory, key, f.intent, f.fetcher);
  assert.equal(duplicate.posted, false); assert.equal(duplicate.operationId, first.operationId);
  assert.equal(f.posts.length, 1);
});
posixTest('a lost original intent after dispatch never allocates a replacement operation', async t => {
  const f = fixture(t); await dispatchRigCreateIntent(f.directory, key, f.intent, f.fetcher);
  const file = path.join(f.directory, key + '.intent.json'); fs.unlinkSync(file);
  await assert.rejects(dispatchRigCreateIntent(f.directory, key, f.intent, f.fetcher), /invalid_native_create_record/);
  assert.equal(fs.existsSync(file), false); assert.equal(f.posts.length, 1);
});
posixTest('a failed journal-parent flush prevents every native mutation', t => {
  for (const preexisting of [false, true]) {
    const f = fixture(t);
    if (preexisting) fs.mkdirSync(f.directory, { mode: 0o700 });
    const source = new URL('../webmux/server/internal/openrigcoord/rig-create-intent.mts', import.meta.url).href;
    const child = spawnSync(process.execPath, ['--input-type=module', '-e', `
      import fs from 'node:fs';
      import {dispatchRigCreateIntent} from ${JSON.stringify(source)};
      const [directory,key,raw,parent]=process.argv.slice(1);
      const expected=fs.statSync(parent), original=fs.fsyncSync;
      fs.fsyncSync=fd=>{
        const stat=fs.fstatSync(fd);
        if(stat.isDirectory() && stat.ino===expected.ino && stat.dev===expected.dev)
          throw Object.assign(new Error('owned fixture parent flush failure'),{code:'EIO'});
        return original(fd);
      };
      let posts=0, refused=false;
      const fetcher=async (_address,init)=>{
        if(init.method==='GET')return Response.json({contract:'operation-correlation.v1',kinds:['member.add']});
        posts++;
        return Response.json({operation:{contract:'operation-correlation.v1',
          operationId:new Headers(init.headers).get('X-OpenRig-Operation-Id'),
          rigId:JSON.parse(raw).rigId,nodeIds:[${JSON.stringify(nodeId)}]}},{status:201});
      };
      try {await dispatchRigCreateIntent(directory,key,JSON.parse(raw),fetcher);}
      catch(error){refused=error.code==='EIO';}
      console.log(JSON.stringify({posts,refused}));`,
      f.directory, key, JSON.stringify(f.intent), f.parent], { encoding: 'utf8' });
    assert.equal(child.status, 0, child.stderr);
    assert.deepEqual(JSON.parse(child.stdout), { posts: 0, refused: true });
  }
});
posixTest('lost response stays consumed; explicit inspection uses the identical operation and bytes', async t => {
  const f = fixture(t); f.lose(true);
  const first = await dispatchRigCreateIntent(f.directory, key, f.intent, f.fetcher);
  assert.equal(first.state, 'uncertain');
  assert.equal((await dispatchRigCreateIntent(f.directory, key, f.intent, f.fetcher)).posted, false);
  f.lose(false);
  const inspected = await inspectRigCreateIntent(f.directory, key, f.intent, f.fetcher);
  assert.equal(inspected.operationId, first.operationId); assert.equal(inspected.state, 'recorded');
  assert.equal(f.posts.length, 2);
  assert.deepEqual(f.posts[0], f.posts[1]);
});
posixTest('concurrent controller attempts admit only one first dispatch', async t => {
  const f = fixture(t);
  const replies = await Promise.all([dispatchRigCreateIntent(f.directory, key, f.intent, f.fetcher), dispatchRigCreateIntent(f.directory, key, f.intent, f.fetcher)]);
  assert.equal(replies.filter(r => r.posted).length, 1); assert.equal(f.posts.length, 1);
});
posixTest('changed policy bytes, key order, manager, route or port refuse without HTTP', async t => {
  const f = fixture(t); reserveRigCreateIntent(f.directory, key, f.intent);
  for (const changed of [{ requestText: JSON.stringify(JSON.parse(f.intent.requestText)) },
    { requestText: '{ "member": { "runtime": "terminal", "id": "worker" } }' }, { managerIdentity: 'c'.repeat(64) },
    { rigId: nodeId }, { pod: 'other' }, { port: 43211 }]) {
    await assert.rejects(dispatchRigCreateIntent(f.directory, key, { ...f.intent, ...changed }, f.fetcher), /native_create_intent_changed/);
  }
  assert.equal(f.posts.length, 0);
});
posixTest('unsupported native correlation never consumes dispatch authority', async t => {
  const f = fixture(t); f.unsupported();
  await assert.rejects(dispatchRigCreateIntent(f.directory, key, f.intent, f.fetcher), /correlation_unavailable/);
  assert.equal(fs.existsSync(path.join(f.directory, key + '.dispatched')), false); assert.equal(f.posts.length, 0);
});
posixTest('empty or wrong-rig native receipts stay uncertain and consumed', async t => {
  for (const mode of ['empty', 'wrong'] as const) {
    const f = fixture(t); f[mode]();
    assert.equal((await dispatchRigCreateIntent(f.directory, key, f.intent, f.fetcher)).state, 'uncertain');
    assert.equal((await dispatchRigCreateIntent(f.directory, key, f.intent, f.fetcher)).posted, false);
    assert.equal(f.posts.length, 1);
  }
});
posixTest('a partial intent is never replaced with a new operation', t => {
  const f = fixture(t); fs.mkdirSync(f.directory, { mode: 0o700 });
  const file = path.join(f.directory, key + '.intent.json'); fs.writeFileSync(file, '{"version":', { mode: 0o600 });
  assert.throws(() => reserveRigCreateIntent(f.directory, key, f.intent));
  assert.equal(fs.readFileSync(file, 'utf8'), '{"version":');
});
posixTest('inspection cannot manufacture dispatch authority', async t => {
  const f = fixture(t);
  await assert.rejects(inspectRigCreateIntent(f.directory, key, f.intent, f.fetcher));
  assert.equal(f.posts.length, 0);
});
posixTest('symlinked or publicly readable journals refuse', t => {
  for (const mode of ['link', 'public']) {
    const f = fixture(t); fs.mkdirSync(path.join(f.parent, 'other'), { mode: 0o700 });
    if (mode === 'link') fs.symlinkSync(path.join(f.parent, 'other'), f.directory);
    else {
      fs.mkdirSync(f.directory, { mode: 0o755 });
      // Creation respects umask; establish the unsafe precondition explicitly.
      fs.chmodSync(f.directory, 0o755);
      assert.equal(fs.statSync(f.directory).mode & 0o777, 0o755);
    }
    assert.throws(() => reserveRigCreateIntent(f.directory, key, f.intent), /unsafe_native_create_journal/);
  }
});
