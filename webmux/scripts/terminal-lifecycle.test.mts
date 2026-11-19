import assert from 'node:assert/strict';
import { test } from 'node:test';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { disposeFixtureTerminal, observeFixtureBrowser } from '../../tests/e2e/terminal-lifecycle.mts';
import { prepareBrowserTestHome } from './browser-test-home.mts';

const lifecycle = { contract: 'webmux-local-provider-lifecycle/v1', state: 'running', selection: 'a'.repeat(64) };
function fixture(options: { state?: string; operation?: string; stop?: 'uncertain' | 'lost' | 'foreign'; legacy?: boolean } = {}) {
  const calls: { method: string; endpoint: string; data?: Record<string, unknown> }[] = [];
  const retained: string[] = [];
  const response = (status: number, body: unknown) => ({ status: () => status, json: async () => body });
  const request = {
    get: async (endpoint: string) => {
      calls.push({ method: 'GET', endpoint });
      return response(200, { id: 'fixture', ...(!options.legacy ? { local_lifecycle: { ...lifecycle, state: options.state || 'running', operation_id: options.operation } } : {}) });
    },
    post: async (endpoint: string, { data }: { data: Record<string, unknown> }) => {
      calls.push({ method: 'POST', endpoint, data });
      if (options.stop === 'lost') throw Error('connection lost after dispatch');
      return response(options.stop === 'uncertain' ? 202 : 200, {
        outcome: options.stop === 'uncertain' ? 'uncertain' : 'stopped',
        session: { id: options.stop === 'foreign' ? 'another-session' : 'fixture', local_lifecycle: {
          ...lifecycle, operation_id: data.id, selection: data.selection,
        } },
      });
    },
    patch: async (endpoint: string, { data }: { data: Record<string, unknown> }) => { calls.push({ method: 'PATCH', endpoint, data }); return response(200, {}); },
    delete: async (endpoint: string, options?: { data: Record<string, unknown> }) => { calls.push({ method: 'DELETE', endpoint, ...(options ? { data: options.data } : {}) }); return response(204, null); },
  } as unknown as Parameters<typeof disposeFixtureTerminal>[0];
  const info = { attach: async (_name: string, value: { body?: string | Buffer }) => { retained.push(String(value.body)); } } as Parameters<typeof disposeFixtureTerminal>[2];
  return { request, info, calls, retained };
}

test('owned fixture deletion follows confirmation of exactly its frozen Stop', async () => {
  const f = fixture(); await disposeFixtureTerminal(f.request, 'fixture', f.info);
  assert.deepEqual(f.calls.map(x => x.method), ['GET', 'POST', 'DELETE']);
  assert.equal(f.calls[1].data?.selection, lifecycle.selection);
  assert.equal(f.calls[1].data?.mode, 'stop');
  assert.match(String(f.calls[1].data?.id), /^[a-f0-9-]{36}$/);
  assert.equal(f.retained.length, 0);
});

test('reconstructed original Stop is inspected with its existing identity', async () => {
  const f = fixture({ state: 'uncertain', operation: 'original-operation' });
  await disposeFixtureTerminal(f.request, 'fixture', f.info);
  assert.deepEqual(f.calls[1].data, { id: 'original-operation', selection: lifecycle.selection, mode: 'inspect', confirm: false });
});

for (const stop of ['uncertain', 'lost'] as const) test(`${stop} Stop preserves access without deleting or sending another Stop`, async () => {
  const f = fixture({ stop }); await disposeFixtureTerminal(f.request, 'fixture', f.info);
  assert.deepEqual(f.calls.map(x => x.method), ['GET', 'POST', 'PATCH']);
  assert.deepEqual(f.calls[2].data, { minimized: true });
  assert.equal(f.retained.length, 1);
  assert.equal(JSON.parse(f.retained[0]).operation, f.calls[1].data?.id);
});

test('unavailable original owner is retained and cannot become a delete-only cleanup', async () => {
  const f = fixture({ state: 'unavailable' }); await disposeFixtureTerminal(f.request, 'fixture', f.info);
  assert.deepEqual(f.calls.map(x => x.method), ['GET', 'PATCH']);
  assert.equal(JSON.parse(f.retained[0]).savedAccessRetained, true);
});

test('a successful reply naming another session cannot authorize deletion', async () => {
  const f = fixture({ stop: 'foreign' });
  await assert.rejects(disposeFixtureTerminal(f.request, 'fixture', f.info), /another session/);
  assert.deepEqual(f.calls.map(x => x.method), ['GET', 'POST']);
});

test('confirmed and other lifecycle owners retain their existing delete contract', async () => {
  for (const options of [{ state: 'stopped' }, { legacy: true }]) {
    const f = fixture(options); await disposeFixtureTerminal(f.request, 'fixture', f.info);
    assert.deepEqual(f.calls.map(x => x.method), ['GET', 'DELETE']);
  }
});

test('browser cleanup ends only the selected viewer lease and does not claim another viewer', async () => {
  const f = fixture();
  let onResponse: (response: unknown) => void = () => {};
  const page = { on: (_event: string, listener: typeof onResponse) => { onResponse = listener; } } as unknown as Parameters<typeof observeFixtureBrowser>[0];
  const close = observeFixtureBrowser(page, f.request, 'fixture');
  const reply = (id: string, owned: boolean, token: string, viewer: string) => ({
    url: () => `http://localhost/api/sessions/${id}/browser`, ok: () => true,
    request: () => ({ method: () => 'POST', postDataJSON: () => ({ viewer }) }),
    json: async () => ({ control: { owned, token } }),
  });
  onResponse(reply('fixture', true, 'original-lease', 'original-viewer'));
  onResponse(reply('another-session', true, 'foreign-lease', 'foreign-viewer'));
  onResponse(reply('fixture', false, '', 'another-viewer'));
  await close();
  assert.deepEqual(f.calls, [{ method: 'DELETE', endpoint: '/api/sessions/fixture/browser', data: { viewer: 'original-viewer', controlToken: 'original-lease' } }]);
});

for (const available of [true, false]) test(`released browser cleanup respects current availability=${available}`, async () => {
  const calls: { method: string; data: object }[] = []; let deletes = 0;
  const response = (status: number, body: unknown) => ({ status: () => status, json: async () => body });
  const request = {
    delete: async (_endpoint: string, { data }: { data: object }) => { calls.push({ method: 'DELETE', data }); return response(++deletes === 1 ? 409 : 204, null); },
    post: async (_endpoint: string, { data }: { data: { action: string } }) => {
      calls.push({ method: 'POST', data });
      return response(200, { control: data.action === 'frame' ? { available } : { owned: true, token: 'new-lease' } });
    },
  } as unknown as Parameters<typeof observeFixtureBrowser>[1];
  let onResponse: (response: unknown) => void = () => {};
  const page = { on: (_event: string, listener: typeof onResponse) => { onResponse = listener; } } as unknown as Parameters<typeof observeFixtureBrowser>[0];
  const close = observeFixtureBrowser(page, request, 'fixture');
  onResponse({ url: () => 'http://localhost/api/sessions/fixture/browser', ok: () => true,
    request: () => ({ method: () => 'POST', postDataJSON: () => ({ viewer: 'original-viewer' }) }),
    json: async () => ({ control: { owned: true, token: 'old-lease' } }),
  });
  if (available) {
    await close();
    assert.deepEqual(calls.map(x => x.method), ['DELETE', 'POST', 'POST', 'DELETE']);
    assert.deepEqual(calls[2].data, { action: 'claim', viewer: 'original-viewer', controlToken: 'old-lease' });
    assert.deepEqual(calls[3].data, { viewer: 'original-viewer', controlToken: 'new-lease' });
  } else {
    await assert.rejects(close(), /another viewer owns/);
    assert.deepEqual(calls.map(x => x.method), ['DELETE', 'POST']);
  }
});

test('a new Windows fixture preserves the complete previous home and its recovery authority', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-browser-home-'));
  try {
    const home = path.join(root, '.test-home'); fs.mkdirSync(path.join(home, 'data'), { recursive: true });
    fs.writeFileSync(path.join(home, 'data', 'sessions.yaml'), 'synthetic-original-recovery-authority');
    fs.writeFileSync(path.join(home, 'webmux.exe'), 'synthetic-original-image');
    const retained = prepareBrowserTestHome(home, true)!;
    assert.equal(fs.existsSync(home), false);
    assert.equal(fs.readFileSync(path.join(retained, 'data', 'sessions.yaml'), 'utf8'), 'synthetic-original-recovery-authority');
    assert.equal(fs.readFileSync(path.join(retained, 'webmux.exe'), 'utf8'), 'synthetic-original-image');
    assert.equal(prepareBrowserTestHome(home, true), undefined);
    fs.mkdirSync(home); fs.writeFileSync(path.join(home, 'new-run'), 'new');
    const next = prepareBrowserTestHome(home, true)!; assert.notEqual(next, retained);
    assert.equal(fs.readFileSync(path.join(retained, 'webmux.exe'), 'utf8'), 'synthetic-original-image');
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});
