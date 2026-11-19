import { test } from 'node:test';
import assert from 'node:assert/strict';
import { waitForRigChange } from '../webmux/server/internal/openrigcoord/rig-changes.mts';

const run = (_command: string, args: string[]) => args[0] === '--version' ? '0.6.14 (df1c1517)' : 'Daemon running on port 4242';
const encoder = new TextEncoder();
const event = 'id: 42\ndata: {"seq":42,"text":"PRIVATE_EVENT","hookToken":"PRIVATE_TOKEN"}\n\n';
const unavailable = { available: false, changed: false };
function response(chunks: string[], headers = { 'content-type': 'text/event-stream' }) {
  return new Response(new ReadableStream({ start(c) { for (const chunk of chunks) c.enqueue(encoder.encode(chunk)); c.close(); } }), { headers });
}

for (const build of ['0.6.14 (df1c1517)', '0.7.18 (3bf87c62)'])
test(`Rig change is a payload-free invalidation across every stream split on ${build}`, async () => {
  for (let split = 0; split <= event.length; split++) {
    const result = await waitForRigChange((command, args) => args[0] === '--version' ? build : run(command, args), async (url, init) => {
      assert.equal(url, 'http://127.0.0.1:4242/api/events?replay=0');
      assert.equal(init?.method, 'GET'); assert.equal(init?.redirect, 'error');
      assert.deepEqual(init?.headers, { Accept: 'text/event-stream' });
      assert.ok(init?.signal);
      return response([event.slice(0, split), event.slice(split)]);
    });
    assert.deepEqual(result, { available: true, changed: true });
    assert.ok(!JSON.stringify(result).includes('PRIVATE'));
  }
});

test('CRLF, mixed separators and keepalive comments work without history', async () => {
  const input = ': ping\r\n\r\n' + event;
  for (let split = 0; split <= input.length; split++) {
    assert.deepEqual(await waitForRigChange(run, async () => response([input.slice(0, split), input.slice(split)])), { available: true, changed: true });
  }
  assert.deepEqual(await waitForRigChange(run, async () => response([event.replaceAll('\n', '\r\n')])), { available: true, changed: true });
});

test('unknown builds, invalid ports and command failures never open streams', async () => {
  for (const bad of [() => '0.6.15', () => '0.6.14 (other)', () => '0.7.18 (other)', () => '0.7.19 (future)', (_c: string, a: string[]) => a[0] === '--version' ? '0.6.14 (df1c1517)' : 'running on port 65536', () => { throw Error('PRIVATE'); }]) {
    assert.deepEqual(await waitForRigChange(bad, async () => { throw Error('unexpected request'); }), unavailable);
  }
});

test('malformed frames, wrong media type, EOF and size bounds degrade to polling', async () => {
  for (const input of ['id: 1\ndata: '+JSON.stringify({seq:1,private:'😀'.repeat(20000)})+'\n\n', '', 'id: 1\ndata: {"seq":2}\n\n', 'id: 0\ndata: {"seq":0}\n\n', 'id: 9007199254740992\ndata: {}\n\n', 'id: 1\ndata: notjson\n\n', 'id: 1\ndata: []\n\n', 'x'.repeat(65537), ':ping\n\n'.repeat(129), 'data: '+ 'x'.repeat(256*1024)]) {
    assert.deepEqual(await waitForRigChange(run, async () => response([input])), unavailable);
  }
  assert.deepEqual(await waitForRigChange(run, async () => response([event], { 'content-type': 'text/plain' })), unavailable);
  assert.deepEqual(await waitForRigChange(run, async () => new Response(null, { status: 503 })), unavailable);
  assert.deepEqual(await waitForRigChange(run, async () => { throw Error('PRIVATE'); }), unavailable);
});

test('quiet connected streams expire and cancel their readers', async () => {
  let canceled = false;
  const result = await waitForRigChange(run, async (_url, init) => {
    const body = new ReadableStream<Uint8Array>({
      start(c) { init?.signal?.addEventListener('abort', () => c.error(Error('expired'))); },
      cancel() { canceled = true; },
    });
    init?.signal?.addEventListener('abort', () => { canceled = true; });
    return new Response(body, { headers: { 'content-type': 'text/event-stream' } });
  }, 10);
  assert.deepEqual(result, { available: true, changed: false }); assert.equal(canceled, true);
});

test('successful first event cancels upstream instead of retaining a subscription', async () => {
  let canceled = false;
  const result = await waitForRigChange(run, async () => new Response(new ReadableStream({ start(c) { c.enqueue(encoder.encode(event)); }, cancel() { canceled = true; } }), { headers: { 'content-type': 'text/event-stream' } }));
  assert.deepEqual(result, { available: true, changed: true }); assert.ok(canceled);
});


test('an explicit other endpoint cannot redirect a local subscription', async () => {
  const original = process.env.OPENRIG_URL;
  try {
    process.env.OPENRIG_URL = 'http://example.invalid:4242';
    assert.deepEqual(await waitForRigChange(run, async () => { throw Error('unexpected network'); }), unavailable);
  } finally {
    if (original === undefined) delete process.env.OPENRIG_URL;
    else process.env.OPENRIG_URL = original;
  }
});
