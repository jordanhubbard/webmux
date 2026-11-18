import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { retainHttpCreateRequest, type HttpCreateSelection } from '../webmux/server/internal/openrigcoord/rig-http-create.mts';
const posixTest = process.platform === 'win32' ? test.skip : test;
const requestId = '2d438c43-c962-4b1d-b8ed-a35e07a8e964';
function fixture(t: { after(fn: () => void): void }) {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'wm-http-create-')));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const directory = path.join(root, 'owner');
  const selection: HttpCreateSelection = { mode: 'new', rigId: '', rigName: 'original', pod: '',
    name: 'coder', host: 'localhost', cwd: root, runtime: 'codex', row: 1, col: 2 };
  const file = path.join(directory, 'http-requests', requestId, 'selection.json');
  return { root, directory, selection, file,
    retain: (change: Partial<HttpCreateSelection> = {}) => retainHttpCreateRequest(directory, requestId, { ...selection, ...change }, true) };
}
posixTest('original public selection is durable before handoff and survives reconstruction', t => {
  const f = fixture(t);
  f.retain().requireNative(true);
  const bytes = fs.readFileSync(f.file);
  assert.equal(fs.statSync(f.file).mode & 0o777, 0o600);
  assert.equal(fs.statSync(path.dirname(f.file)).mode & 0o777, 0o700);
  assert.deepEqual(JSON.parse(bytes.toString()), { contract: 'webmux-native-http-create/v1', requestId, ...f.selection });
  // No in-memory state or native operation ID is needed to reopen the request.
  f.retain().requireNative(true);
  assert.deepEqual(fs.readFileSync(f.file), bytes);
  assert.equal(fs.readdirSync(f.directory).join(','), 'http-requests');
});
posixTest('changed selections refuse before native handoff, including placement and default names', t => {
  const f = fixture(t); f.retain(); const original = fs.readFileSync(f.file);
  for (const change of [{ mode: 'existing', rigId: '01JQPWV4E7QP26M799M3S0MR6E', pod: 'agents' },
    { rigName: 'next-project' }, { rigId: '01JQPWV4E7QP26M799M3S0MR6E' }, { pod: 'other' },
    { name: 'coder-two' }, { cwd: '/another' }, { row: 2 }, { col: 3 }, { row: null, col: null }] as Partial<HttpCreateSelection>[]) {
    assert.throws(() => f.retain(change), /original_request_unavailable/);
    assert.deepEqual(fs.readFileSync(f.file), original);
  }
});
posixTest('unsupported contracts and nonlocal runtimes refuse even the first tracked dispatch', t => {
  const f = fixture(t);
  assert.throws(() => retainHttpCreateRequest(f.directory, requestId, f.selection, false), /contract_unavailable/);
  for (const change of [{ host: 'worker' }, { runtime: 'claude-code' }]) assert.throws(() => f.retain(change), /contract_unavailable/);
  assert.equal(fs.existsSync(f.directory), false);
  assert.throws(() => retainHttpCreateRequest(f.directory, '../escape', f.selection, true), /invalid_request_id/);
});
posixTest('tracked dispatch cannot adopt a legacy seat, even after a matching public selection', t => {
  const f = fixture(t), request = f.retain();
  assert.throws(() => request.requireNative(false), /legacy_route_refused/);
  assert.throws(() => f.retain().requireNative(false), /legacy_route_refused/);
  assert.throws(() => f.retain().requireNative(true), /legacy_route_refused/, 'A later topology change must not convert this original into a new native launch');
});
posixTest('owner namespaces remain separate and cannot redirect another owner selection', t => {
  const f = fixture(t); f.retain(); const original = fs.readFileSync(f.file);
  const other = path.join(f.root, 'another-owner'), selection = { ...f.selection, name: 'another' };
  retainHttpCreateRequest(other, requestId, selection, true).requireNative(true);
  assert.deepEqual(fs.readFileSync(f.file), original);
  assert.throws(() => retainHttpCreateRequest(other, requestId, f.selection, true), /original_request_unavailable/);
});
posixTest('corrupt, symlinked and over-permissive records never authorize native handoff', t => {
  for (const kind of ['corrupt', 'symlink', 'mode', 'hardlink']) {
    const f = fixture(t), request = f.retain();
    if (kind === 'corrupt') fs.writeFileSync(f.file, '{}');
    if (kind === 'mode') fs.chmodSync(f.file, 0o644);
    if (kind === 'hardlink') fs.linkSync(f.file, path.join(f.root, 'linked'));
    if (kind === 'symlink') { fs.renameSync(f.file, path.join(f.root, 'original')); fs.symlinkSync(path.join(f.root, 'original'), f.file); }
    assert.throws(() => f.retain());
    assert.throws(() => request.requireNative(true));
  }
});
posixTest('runner refuses changed public selections before any topology, manager or account probe', t => {
  const f = fixture(t); f.retain();
  const bin = path.join(f.root, 'bin'); fs.mkdirSync(bin);
  const calls = path.join(f.root, 'manager-called');
  fs.writeFileSync(path.join(bin, 'tmux'), '#!/bin/sh\nprintf "tmux 3.8\\n"\n', { mode: 0o700 });
  fs.writeFileSync(path.join(bin, 'rig'), '#!' + process.execPath + '\nif(process.argv[2]==="--version"){console.log("0.7.18 (3bf87c62)")}else{require("node:fs").writeFileSync(' + JSON.stringify(calls) + ',"called");console.error("manager-probe-control");process.exitCode=77}\n', { mode: 0o700 });
  const invoke = (change: object) => spawnSync(process.execPath, [path.resolve(import.meta.dirname, '../webmux/server/internal/openrigcoord/runner.mts'), 'create', f.directory], {
    input: JSON.stringify({ ...f.selection, creationRequestId: requestId, ...change }), encoding: 'utf8', timeout: 5000,
    env: { ...process.env, PATH: bin + path.delimiter + process.env.PATH },
  });
  for (const change of [{ name: 'replacement' }, { rigName: 'replacement' }, { row: 3 }, { cwd: '/other' }]) {
    const result = invoke(change); assert.equal(result.status, 1); assert.match(result.stderr, /original_request_unavailable/);
    assert.equal(fs.existsSync(calls), false);
  }
  const original = invoke({ continueOriginal: true });
  assert.equal(original.status, 1); assert.match(original.stderr, /manager-probe-control/);
  assert.equal(fs.existsSync(calls), true);
});

posixTest('runner permanently refuses an adopted legacy seat for the tracked original', t => {
  const f = fixture(t), rigId = '01JQPWV4E7QP26M799M3S0MR6E';
  const selection = { ...f.selection, mode: 'existing' as const, rigId, rigName: 'original', pod: 'agents' };
  fs.mkdirSync(f.directory, { mode: 0o700 });
  const bin = path.join(f.root, 'bin'); fs.mkdirSync(bin);
  const calls = path.join(f.root, 'calls');
  const scope = createHash('sha256').update(f.directory).digest('hex').slice(0, 10);
  const member = selection.name + '-' + createHash('sha256').update('localhost').digest('hex').slice(0, 8);
  const tmux = 'wm-' + scope + '-' + rigId.slice(-8).toLowerCase() + '-' + member;
  const record = path.join(f.directory, 'legacy.agent.json');
  const agent = { ...selection, project: selection.rigName, tmux, target: tmux };
  fs.writeFileSync(record, JSON.stringify(agent));
  fs.writeFileSync(path.join(bin, 'tmux'), '#!/bin/sh\nif [ "$1" = "-V" ]; then printf "tmux 3.8\\n"; fi\n', { mode: 0o700 });
  const stub = '#!' + process.execPath + '\nconst fs=require("node:fs"),a=process.argv.slice(2);fs.appendFileSync(' + JSON.stringify(calls) + ',JSON.stringify(a)+"\\n");if(a[0]==="--version"){console.log("0.7.18 (3bf87c62)")}else if(a[0]==="ps"){console.log(JSON.stringify(a.includes("--nodes")?[{rigId:' + JSON.stringify(rigId) + ',podNamespace:"agents"}]:[{rigId:' + JSON.stringify(rigId) + ',rigName:"original"}]))}else{process.exitCode=77}\n';
  fs.writeFileSync(path.join(bin, 'rig'), stub, { mode: 0o700 });
  const invoke = () => spawnSync(process.execPath, [path.resolve(import.meta.dirname, '../webmux/server/internal/openrigcoord/runner.mts'), 'create', f.directory], {
    input: JSON.stringify({ ...selection, creationRequestId: requestId }), encoding: 'utf8', timeout: 5000,
    env: { ...process.env, PATH: bin + path.delimiter + process.env.PATH },
  });
  const first = invoke(); assert.equal(first.status, 1); assert.match(first.stderr, /legacy_route_refused/);
  const observed = fs.readFileSync(calls, 'utf8').trim().split('\n').map(line => JSON.parse(line) as string[]);
  assert.ok(observed.every(args => args[0] === '--version' || args[0] === 'ps'));
  fs.writeFileSync(record, JSON.stringify({ ...agent, nativeOwned: true }));
  fs.writeFileSync(calls, '');
  const changed = invoke(); assert.equal(changed.status, 1); assert.match(changed.stderr, /legacy_route_refused/);
  const retried = fs.readFileSync(calls, 'utf8').trim().split('\n').map(line => JSON.parse(line) as string[]);
  assert.deepEqual(retried, [['--version']], 'A refused legacy original must not consult topology again');
});
