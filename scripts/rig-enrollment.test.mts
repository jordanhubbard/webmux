import { test } from 'node:test';
import assert from 'node:assert/strict';
import { observeRigEnrollments } from '../webmux/server/internal/openrigcoord/rig-enrollment.mts';

const rigId = '01M7NQ0XV3KPXQDNNC18KY64M5';
const nodeId = '01M7NQ0XV3KPXQDNNC18KY64M6';
const sessionId = '01M7NQ0XV3KPXQDNNC18KY64M7';
const successorId = '01M7NQ0XV3KPXQDNNC18KY64M8';
function fixture(origin = 'claimed') {
  const f = {
    seats: [{ rigId, logicalId: 'agents.one', canonicalSessionName: 'native' }],
    nodes: [{ rigId, nodeId, logicalId: 'agents.one', canonicalSessionName: 'native', storedSessionStatus: 'running' }],
    sessions: [{ id: sessionId, nodeId, sessionName: 'native', status: 'running', origin, resumeToken: 'PRIVATE_TOKEN', evidence: 'PRIVATE_PROMPT' }],
    commands: [] as string[][],
    calls: [] as { url: string; init?: RequestInit }[],
    run: (_command: string, args: string[]) => { f.commands.push(args); if (args[0] === '--version') return '0.6.14 (df1c1517)'; if (args.join(' ') === 'daemon status') return 'Daemon running on port 48484'; throw new Error('unexpected command'); },
    fetcher: (async (url, init) => { f.calls.push({ url: String(url), init }); return new Response(JSON.stringify(String(url).endsWith('/nodes') ? f.nodes : f.sessions)); }) as typeof fetch,
  };
  return f;
}
for (const origin of ['launched', 'claimed']) test(`projects manager-reported ${origin} origin without private runtime metadata`, async () => {
  const f = fixture(origin);
  const [result] = await observeRigEnrollments(f.run, f.seats, f.fetcher);
  assert.equal(result.enrollmentOrigin, origin); assert.equal(result.enrollmentId, sessionId);
  assert.equal(result.enrollmentSource, 'rig-session-registry'); assert(Number.isFinite(Date.parse(result.enrollmentObservedAt!)));
  assert(!JSON.stringify(result).includes('PRIVATE_'));
  assert.deepEqual(f.commands, [['--version'], ['daemon', 'status']]);
  assert.equal(f.calls.length, 2);
  for (const call of f.calls) { assert(call.url.startsWith(`http://127.0.0.1:48484/api/rigs/${rigId}/`)); assert.equal(call.init?.method, 'GET'); assert.equal(call.init?.redirect, 'error'); }
});
test('follows the qualified inventory greatest-session-ULID rule, not registry array order or historical name reuse', async () => {
  const f = fixture(); f.sessions.unshift({ ...f.sessions[0], id: successorId, origin: 'launched' });
  assert.equal((await observeRigEnrollments(f.run, f.seats, f.fetcher))[0].enrollmentOrigin, 'launched');
  f.sessions[0].sessionName = 'replacement';
  assert.equal((await observeRigEnrollments(f.run, f.seats, f.fetcher))[0].enrollmentOrigin, 'unknown');
});
for (const scenario of ['duplicate_seat', 'duplicate_address', 'duplicate_node', 'duplicate_session', 'malformed_session', 'different_rig', 'different_binding', 'different_status', 'missing_origin', 'invalid_node']) test(`rejects ambiguous or inconsistent enrollment: ${scenario}`, async () => {
  const f = fixture();
  if (scenario === 'duplicate_seat') f.seats.push({ ...f.seats[0], canonicalSessionName: 'other' });
  if (scenario === 'duplicate_address') f.seats.push({ ...f.seats[0], logicalId: 'agents.other' });
  if (scenario === 'duplicate_node') f.nodes.push({ ...f.nodes[0] });
  if (scenario === 'duplicate_session') f.sessions.push({ ...f.sessions[0] });
  if (scenario === 'malformed_session') f.sessions.push({ ...f.sessions[0], id: 'invalid' });
  if (scenario === 'different_rig') f.nodes[0].rigId = successorId;
  if (scenario === 'different_binding') f.nodes[0].canonicalSessionName = 'other';
  if (scenario === 'different_status') f.nodes[0].storedSessionStatus = 'exited';
  if (scenario === 'missing_origin') f.sessions[0].origin = '';
  if (scenario === 'invalid_node') f.nodes[0].nodeId = 'invalid';
  assert.equal((await observeRigEnrollments(f.run, f.seats, f.fetcher))[0].enrollmentOrigin, 'unknown');
});
test('unsupported builds and invalid daemon endpoints never initiate network reads', async () => {
  const f = fixture();
  for (const run of [() => '0.6.14 (unqualified)', (_c: string, a: string[]) => a[0] === '--version' ? '0.6.14 (df1c1517)' : 'Daemon running on port 65536']) {
    assert.equal((await observeRigEnrollments(run, f.seats, f.fetcher))[0].enrollmentSource, 'unavailable');
  }
  assert.equal(f.calls.length, 0);
});
for (const scenario of ['status', 'invalid_json', 'nonarray', 'huge_header', 'huge_stream']) test(`degrades safely on a failed or oversized registry response: ${scenario}`, async () => {
  const f = fixture();
  const fetcher = (async () => {
    if (scenario === 'status') return new Response('private', { status: 403 });
    if (scenario === 'invalid_json') return new Response('PRIVATE_INVALID_JSON');
    if (scenario === 'nonarray') return new Response('{}');
    if (scenario === 'huge_header') return new Response('[]', { headers: { 'content-length': String(1024 * 1024) } });
    return new Response('x'.repeat(512 * 1024 + 1));
  }) as typeof fetch;
  const result = await observeRigEnrollments(f.run, f.seats, fetcher);
  assert.deepEqual(result, [{ enrollmentOrigin: 'unknown', enrollmentSource: 'unavailable' }]);
});
test('cancels a stalled response at the shared deadline without inventing enrollment', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const f = fixture();
  const fetcher = (async (_url, init) => new Promise<Response>((_resolve, reject) => { init?.signal?.addEventListener('abort', () => reject(new Error('aborted')), { once: true }); })) as typeof fetch;
  const pending = observeRigEnrollments(f.run, f.seats, fetcher);
  await Promise.resolve();
  t.mock.timers.tick(8000);
  assert.equal((await pending)[0].enrollmentOrigin, 'unknown');
});

test('one failed rig does not erase another rig enrollment or attribute it across rigs', async () => {
  const f = fixture();
  const otherRig = successorId;
  const seats = [...f.seats, { ...f.seats[0], rigId: otherRig, canonicalSessionName: 'other-native' }];
  const fetcher = (async url => {
    if (String(url).includes(rigId)) return new Response('unavailable', { status: 503 });
    return new Response(JSON.stringify(String(url).endsWith('/nodes') ? [{ ...f.nodes[0], rigId: otherRig, canonicalSessionName: 'other-native' }] : [{ ...f.sessions[0], sessionName: 'other-native' }]));
  }) as typeof fetch;
  const result = await observeRigEnrollments(f.run, seats, fetcher);
  assert.equal(result[0].enrollmentOrigin, 'unknown'); assert.equal(result[1].enrollmentOrigin, 'claimed');
});
