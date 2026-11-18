import { test } from 'node:test';
import assert from 'node:assert/strict';
import { reviewRigStop, executeRigStop, type RigStopResult } from '../webmux/server/internal/openrigcoord/rig-stop.mts';

const rigId = '01M71ZM6DE3RTCXHPJRXJ6XKPE';
const nodeId = '01M71ZM6TZ91BH689AGCFP2WCN';
const first = '01M71ZM6QSB0G9GNTTXX0242KJ';
const second = '01M71ZM6QSB0G9GNTTXX0242KK';
function fixture() {
  const input = { rigId, logicalId: 'agents.worker', sessionName: 'worker@project', tmuxServerFlag: '-S', tmuxServerValue: '/tmp/private socket' };
  let session = first, origin = 'launched', version = '0.7.18 (3bf87c62)', socket = input.tmuxServerValue;
  let reply: unknown = null, status = 409, lost = false, malformed = false;
  let listener = 123, managerUID = 501, managerCommit = '3bf87c62a24fcb0a441334d7f8b0b1f2687ecf64';
  let processRecord = '501 Tue Nov 17 12:00:00 2026 node';
  let daemonCommand = 'node /fixture/@openrig/cli/daemon/dist/index.js';
  let executable = '/fixture/bin/node';
  const calls: { method: string; path: string; body?: unknown }[] = [];
  const run = (command: string, args: string[]) => {
    if (command.endsWith('lsof')) return 'p' + listener;
    if (command === '/bin/ps') return args.includes('command=') ? daemonCommand : processRecord;
    if (command === '/usr/bin/readlink') {
      assert.deepEqual(args, ['--', '/proc/123/exe']);
      return executable;
    }
    assert.equal(command, 'rig');
    if (args.join(' ') === '--version') return version;
    if (args.join(' ') === 'daemon status') return 'Daemon running on port 43210 (pid 123)';
    assert.deepEqual(args, ['ps', '--nodes', '-A', '--no-cleanup', '--json', '--full']);
    return JSON.stringify([{ ...input, canonicalSessionName: input.sessionName, attachmentType: 'tmux', tmuxServerArgs: ['-S', socket], runtime: 'terminal' }]);
  };
  const fetcher = (async (address: string | URL | Request, init?: RequestInit) => {
    const url = new URL(String(address));
    assert.equal(url.origin, 'http://127.0.0.1:43210'); assert.equal(init?.redirect, 'error');
    assert.ok(init?.signal);
    const call = { method: init?.method ?? 'GET', path: url.pathname, ...(init?.body ? { body: JSON.parse(String(init.body)) as unknown } : {}) };
    calls.push(call);
    if (call.method === 'POST') {
      assert.equal(call.path, '/api/seat/stop/' + encodeURIComponent(input.sessionName));
      if (lost) throw new Error('Lost after dispatch; native effect unknown');
      return Response.json(reply ?? { ok: true, seat: { rigId, nodeId, logicalId: input.logicalId }, sessionName: input.sessionName, sessionId: session }, { status: reply ? status : 200 });
    }
    if (call.path === '/healthz') return Response.json({pid:123,euid:managerUID,semver:'0.7.18',commit:managerCommit,dirty:false});
    if (call.path.includes('stop-precondition')) return Response.json({ contract: malformed ? 'unknown' : 'seat-stop.v1', expected: { nodeId, sessionId: session } });
    if (call.path.endsWith('/nodes')) return Response.json([{ rigId, nodeId, logicalId: input.logicalId, canonicalSessionName: input.sessionName }]);
    if (call.path.endsWith('/sessions')) return Response.json([{ nodeId, id: session, sessionName: input.sessionName, origin, status: 'running' }]);
    assert.fail('Unexpected native route: ' + call.path);
  }) as typeof fetch;
  return { input, run, fetcher, calls, replace: () => { session = second; }, adopt: () => { origin = 'claimed'; },
    version: (v: string) => { version = v; }, socket: () => { socket = '/tmp/another socket'; }, malformed: () => { malformed = true; },
    reply: (body: unknown, code = 409) => { reply = body; status = code; }, lose: () => { lost = true; },
    tunnel: () => { listener = 999; }, foreign: () => { managerUID = 502; }, unstamped: () => { managerCommit = 'unknown'; },
    restart: () => { processRecord = '501 Wed Nov 18 12:00:00 2026 node'; },
    proxy: () => { daemonCommand = 'node /fixture/forwarder.js'; },
    processName: (name: string) => { processRecord = `501 Tue Nov 17 12:00:00 2026 ${name}`; },
    executable: (name: string) => { executable = name; } };
}
test('Linux Node MainThread uses the kernel executable identity and still dispatches only once', async () => {
  const f = fixture();
  f.processName('MainThread');
  const review = await reviewRigStop(f.run, f.input, f.fetcher, 501, 'linux');
  const result = await executeRigStop(f.run, review, 'Reviewed Linux native Stop', 'owner', f.fetcher, 501, 'linux');
  assert.equal(result.state, 'stopped');
  assert.equal(f.calls.filter(call => call.method === 'POST').length, 1);
});
test('Linux rejects a non-Node, deleted, relative or malformed executable even with a Node process name', async () => {
  for (const executable of ['/usr/bin/ssh', '/fixture/bin/node (deleted)', 'node', '/fixture/bin/node\n/other/node']) {
    const f = fixture();
    f.executable(executable);
    await assert.rejects(reviewRigStop(f.run, f.input, f.fetcher, 501, 'linux'), /local_manager_unverified/, executable);
    assert(f.calls.every(call => call.method === 'GET'));
  }
});
test('Linux executable replacement after review refuses without dispatch', async () => {
  const f = fixture();
  f.processName('MainThread');
  const review = await reviewRigStop(f.run, f.input, f.fetcher, 501, 'linux');
  f.executable('/another/bin/node');
  const result = await executeRigStop(f.run, review, 'reviewed', 'owner', f.fetcher, 501, 'linux');
  assert.equal(result.state, 'refused');
  assert.equal(result.posted, false);
  assert(f.calls.every(call => call.method === 'GET'));
});
test('review is read-only; execution sends the original checked identity exactly once', async () => {
  const f = fixture();
  const review = await reviewRigStop(f.run, f.input, f.fetcher, 501);
  assert(f.calls.every(call => call.method === 'GET'));
  assert.deepEqual(review.expected, { nodeId, sessionId: first });
  assert.deepEqual(await executeRigStop(f.run, review, 'Owner reviewed local seat stop', 'owner', f.fetcher, 501), {
    state: 'stopped', code: 'stopped', posted: true, stopPhase: 'kill_attempted',
  });
  const posts = f.calls.filter(call => call.method === 'POST');
  assert.equal(posts.length, 1);
  assert.deepEqual(posts[0].body, { expected: review.expected, reason: 'Owner reviewed local seat stop', operator: 'owner' });
});
for (const [kind, mutate] of [
  ['replacement', (f: ReturnType<typeof fixture>) => f.replace()],
  ['socket', (f: ReturnType<typeof fixture>) => f.socket()],
  ['adopted', (f: ReturnType<typeof fixture>) => f.adopt()],
  ['unsupported build', (f: ReturnType<typeof fixture>) => f.version('0.7.19 (unknown)')],
  ['missing checked contract', (f: ReturnType<typeof fixture>) => f.malformed()],
  ['loopback tunnel', (f: ReturnType<typeof fixture>) => f.tunnel()],
  ['foreign daemon owner', (f: ReturnType<typeof fixture>) => f.foreign()],
  ['unqualified daemon build', (f: ReturnType<typeof fixture>) => f.unstamped()],
  ['restarted manager process', (f: ReturnType<typeof fixture>) => f.restart()],
  ['local proxy instead of native daemon', (f: ReturnType<typeof fixture>) => f.proxy()],
] as const) test(`${kind} refuses without a lifecycle POST`, async () => {
  const f = fixture();
  const review = await reviewRigStop(f.run, f.input, f.fetcher, 501);
  mutate(f);
  const result = await executeRigStop(f.run, review, 'reviewed', 'owner', f.fetcher, 501);
  assert.equal(result.state, 'refused'); assert.equal(result.posted, false);
  assert(f.calls.every(call => call.method === 'GET'));
});
for (const [phase, state] of [[undefined, 'refused'], ['none', 'refused'], ['detach_attempted', 'uncertain'], ['detached', 'uncertain'], ['kill_attempted', 'uncertain']] as const) {
  test(`native rebind race preserves terminal effect phase ${phase}`, async () => {
    const f = fixture();
    const review = await reviewRigStop(f.run, f.input, f.fetcher, 501);
    f.reply({ ok: false, code: 'selection_changed', ...(phase ? { stopPhase: phase, partialEffect: phase !== 'none' } : {}) });
    const result = await executeRigStop(f.run, review, 'reviewed', 'owner', f.fetcher, 501);
    assert.equal(result.state, state); assert.equal(result.stopPhase, phase ?? 'none');
    assert.equal(f.calls.filter(call => call.method === 'POST').length, 1);
  });
}
test('lost, malformed and inconsistent stop responses never cause an automatic retry or false no-effect claim', async () => {
  for (const variant of ['lost', 'partial-without-phase', 'wrong-success', 'unknown-error', 'wrong-http-status']) {
    const f = fixture();
    const review = await reviewRigStop(f.run, f.input, f.fetcher, 501);
    if (variant === 'lost') f.lose();
    else if (variant === 'partial-without-phase') f.reply({ ok: false, code: 'selection_changed', partialEffect: true });
    else if (variant === 'wrong-success') f.reply({ ok: true, sessionId: second }, 200);
    else if (variant === 'wrong-http-status') f.reply({ ok: false, code: 'selection_changed' }, 500);
    else f.reply({ ok: false, code: 'unknown_effect' });
    const result: RigStopResult = await executeRigStop(f.run, review, 'reviewed', 'owner', f.fetcher, 501);
    assert.equal(result.state, 'uncertain', variant); assert.equal(result.posted, true);
    assert.equal(f.calls.filter(call => call.method === 'POST').length, 1);
  }
});
