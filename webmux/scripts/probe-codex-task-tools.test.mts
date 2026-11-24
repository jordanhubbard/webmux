import assert from 'node:assert/strict';
import test from 'node:test';
import fs from 'node:fs/promises';
import http from 'node:http';
import { createRequire } from 'node:module';
import { once } from 'node:events';
import { ProbeFailure, connect, owns, probe } from './probe-codex-task-tools.mjs';

const thread = '11111111-1111-4111-8111-111111111111';
const turn = '22222222-2222-4222-8222-222222222222';
const options = { socket: '/private/owned/socket', cwd: '/owned/project', thread, viewerPid: 42, daemonPid: 43 };

// These are refusal/receipt controls with injected inspectors, not native proof.
function fixture(overrides = {}) {
  const calls = [];
  const epochs = new Map();
  let closed = 0;
  const dependencies = {
    authorizeSocket: async socket => socket,
    realpath: async value => value,
    epoch: async pid => { epochs.set(pid, (epochs.get(pid) ?? 0) + 1); return `epoch-${pid}`; },
    owns: async () => true,
    connect: async () => ({
      call: async (method, params) => {
        calls.push({ method, params });
        if (overrides.call) return overrides.call(method, params, calls);
        return responses(method);
      },
      close: () => { closed++; },
    }),
    ...overrides.inspectors,
  };
  return { dependencies, calls, epochs, closed: () => closed };
}

function responses(method) {
  switch (method) {
    case 'thread/read': return { thread: { id: thread, cwd: options.cwd } };
    case 'thread/turns/list': return { data: [{ id: turn, status: 'inProgress' }] };
    case 'mcpServerStatus/list': return { data: [{ name: 'codex_tui', httpOrigin: 'http://127.0.0.1:12345', runtimeStatus: 'connected' }] };
    case 'mcpServer/tool/call': return { isError: false, content: [{ type: 'text', text: JSON.stringify({ schemaVersion: 1, thread: { id: thread }, preview: 'SECRET_RESPONSE_CONTENT' }) }] };
    default: throw new Error('Unexpected operation');
  }
}

test('a real management result is required; readiness never claims four-session survival', async () => {
  const f = fixture();
  const result = await probe(options, f.dependencies);
  assert.equal(result.passed, true);
  assert.equal(result.active_work_qualified, false);
  assert.equal(result.four_session_survival_qualified, false);
  assert.equal(f.closed(), 1);
  assert.ok(f.calls.some(call => call.method === 'mcpServer/tool/call' && call.params.arguments.threadId === thread));
  assert.ok(f.calls.every(call => !/resume|start|write|reload|queue/.test(call.method)));
  assert.ok(!JSON.stringify(result).includes('SECRET_RESPONSE_CONTENT'));
  assert.ok(!JSON.stringify(result).includes(options.cwd));
});

test('cached connected inventory and transport errors cannot pass', async () => {
  const f = fixture({ call: async method => {
    if (method === 'mcpServer/tool/call') throw new ProbeFailure(method, -32603);
    return responses(method);
  } });
  const result = await probe(options, f.dependencies);
  assert.equal(result.passed, false);
  assert.deepEqual(result.failure, { stage: 'mcpServer/tool/call', code: -32603 });
  assert.equal(f.closed(), 1);
});

test('a different project is refused before its management endpoint is called', async () => {
  const f = fixture({ call: async method => method === 'thread/read' ? { thread: { id: thread, cwd: '/another/project' } } : responses(method) });
  const result = await probe(options, f.dependencies);
  assert.deepEqual(result.failure, { stage: 'thread', code: 'project' });
  assert.ok(!f.calls.some(call => call.method === 'mcpServer/tool/call'));
});

test('a registered endpoint owned by a different viewer is refused', async () => {
  const f = fixture({ inspectors: { owns: async (_pid, args) => args.includes('-U') } });
  const result = await probe(options, f.dependencies);
  assert.deepEqual(result.failure, { stage: 'tools', code: 'viewer-endpoint-owner' });
  assert.ok(!f.calls.some(call => call.method === 'mcpServer/tool/call'));
});

test('the daemon socket must belong to the specified execution process', async () => {
  const f = fixture({ inspectors: { owns: async () => false } });
  const result = await probe(options, f.dependencies);
  assert.deepEqual(result.failure, { stage: 'daemon', code: 'socket-owner' });
  assert.equal(f.calls.length, 0);
});

test('replacement of a viewer epoch after a successful tool call fails qualification', async () => {
  let viewerReads = 0;
  const f = fixture({ inspectors: { epoch: async pid => pid === options.viewerPid ? `viewer-${++viewerReads}` : 'daemon' } });
  const result = await probe(options, f.dependencies);
  assert.deepEqual(result.failure, { stage: 'process', code: 'epoch-changed' });
});

test('active-work scope requires the exact same in-progress turn on both sides', async () => {
  const good = fixture();
  assert.equal((await probe({ ...options, turn }, good.dependencies)).active_work_qualified, true);
  for (const replacement of [{ id: turn, status: 'completed' }, { id: thread, status: 'inProgress' }]) {
    let reads = 0;
    const f = fixture({ call: async method => method === 'thread/turns/list' && ++reads === 2 ? { data: [replacement] } : responses(method) });
    const result = await probe({ ...options, turn }, f.dependencies);
    assert.equal(result.passed, false);
    assert.deepEqual(result.failure, { stage: 'turn', code: 'not-same-active-work' });
    assert.equal(result.active_work_qualified, false);
  }
});

test('tool-level errors and another conversation in the response are refused', async () => {
  for (const result of [{ isError: true, content: responses('mcpServer/tool/call').content }, { content: [{ type: 'text', text: JSON.stringify({ schemaVersion: 1, thread: { id: turn } }) }] }]) {
    const f = fixture({ call: async method => method === 'mcpServer/tool/call' ? result : responses(method) });
    assert.equal((await probe(options, f.dependencies)).passed, false);
  }
});

test('unclassified errors cannot leak transport messages or credentials', async () => {
  const f = fixture({ call: async () => { throw new Error('SECRET_BEARER_TOKEN_AND_SOURCE_PATH'); } });
  const result = await probe(options, f.dependencies);
  assert.equal(result.passed, false);
  assert.ok(!JSON.stringify(result).includes('SECRET_BEARER'));
  assert.equal(f.closed(), 1);
});

test('failed or malformed initialize closes the actual Unix WebSocket connection', { skip: process.platform === 'win32' }, async () => {
  const require = createRequire(new URL('../webmux/package.json', import.meta.url));
  const { WebSocketServer } = require('ws');
  for (const malformed of [false, true]) {
    const directory = await fs.mkdtemp('/tmp/wm-codex-probe-');
    const socket = `${directory}/s`;
    const httpServer = http.createServer();
    const server = new WebSocketServer({ server: httpServer });
    let closed;
    server.once('connection', ws => {
      closed = once(ws, 'close');
      ws.once('message', bytes => {
        const request = JSON.parse(bytes.toString());
        ws.send(JSON.stringify(malformed ? null : { id: request.id, error: { code: -32603, message: 'PRIVATE_ERROR_DETAILS' } }));
      });
    });
    try {
      httpServer.listen(socket);
      await once(httpServer, 'listening');
      await assert.rejects(connect(socket), error => error instanceof ProbeFailure && error.code === (malformed ? 'invalid-envelope' : -32603));
      assert.ok(closed);
      let timer;
      try { await Promise.race([closed, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('client connection leaked')), 2000); })]); }
      finally { clearTimeout(timer); }
    } finally {
      for (const ws of server.clients) ws.terminate();
      await new Promise(resolve => server.close(resolve));
      await new Promise(resolve => httpServer.close(() => resolve()));
      await fs.rm(directory, { recursive: true, force: true });
    }
  }
});


test('native macOS listener ownership uses numeric loopback addresses', { skip: process.platform !== 'darwin' }, async () => {
  const server = http.createServer();
  try {
    server.listen(0, '127.0.0.1');
    await once(server, 'listening');
    const port = server.address().port;
    assert.equal(await owns(process.pid, [`-iTCP:${port}`, '-sTCP:LISTEN'], `127.0.0.1:${port}`), true);
    assert.equal(await owns(process.ppid, [`-iTCP:${port}`, '-sTCP:LISTEN'], `127.0.0.1:${port}`), false);
  } finally { await new Promise(resolve => server.close(resolve)); }
});
