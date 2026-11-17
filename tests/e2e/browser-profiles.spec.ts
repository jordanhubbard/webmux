import { test, expect } from '@playwright/test';
import http from 'node:http';
import { writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';

test('saved Chromium profile survives worker replacement while viewer control is exclusive', async ({ request }, info) => {
 test.setTimeout(90_000);
 let pressed = 0, released = 0;
 const started = performance.now();
 let phase = 'create-saved-profile', dropped = 0;
 const timeline: { ms: number; phase: string; event: string; detail: unknown }[] = [];
 const record = (event: string, detail: unknown) => {
  if (timeline.length === 128) { dropped++; return; }
  timeline.push({ ms: Math.round(performance.now() - started), phase, event, detail });
 };
 const origin = http.createServer((req, res) => {
  if (req.url === '/pressed') { pressed++; res.end(); return; }
  if (req.url === '/released') { released++; res.end(); return; }
  record('fixture-document', { path: req.url?.split('?')[0].slice(0, 128), retainedCookie: req.headers.cookie?.includes('saved=fixture') === true });
  res.setHeader('Content-Type', 'text/html; charset=utf-8');
  if (req.url === '/set') res.setHeader('Set-Cookie', 'saved=fixture; Path=/; Max-Age=3600; SameSite=Lax');
  res.end(`<title>${req.headers.cookie?.includes('saved=fixture') ? 'retained-cookie' : 'empty-profile'}</title><p>synthetic browser profile</p><script>document.addEventListener('pointerdown',()=>fetch('/pressed'));document.addEventListener('pointerup',()=>fetch('/released'))</script>`);
 });
 await new Promise<void>(resolve => origin.listen(0, '127.0.0.1', resolve));
 const url = `http://127.0.0.1:${(origin.address() as { port: number }).port}`;
 const created = await request.post('/api/sessions', { data: { transport: 'local' } });
 expect(created.ok()).toBe(true);
 const session = await created.json() as { id: string };
 const endpoint = `/api/sessions/${session.id}/browser`;
 const a = 'a'.repeat(32), b = 'b'.repeat(32);
 let token = '', saved = '';
 const action = async (body: Record<string, unknown>) => {
  record('action-request', { action: body.action, profile: body.profileId === undefined ? undefined : body.profileId === '' ? 'temporary' : 'saved' });
  const response = await request.post(endpoint, { data: { viewer: a, controlToken: token, ...body } });
  record('action-status', { action: body.action, status: response.status() });
  expect(response.ok(), `${body.action}: ${response.status()}`).toBe(true);
  const frame = await response.json();
  // Select bounded synthetic-fixture metadata; never retain the control token,
  // internal profile key, screenshots, clipboard or transfer bodies.
  record('worker-frame', {
   action: body.action, protocol: frame.protocol, target: String(frame.target ?? '').slice(0, 128),
   url: String(frame.url ?? '').slice(0, 512), error: String(frame.error ?? '').slice(0, 512),
   profileId: String(frame.profileId ?? '').slice(0, 128), owned: frame.control?.owned,
   navigationDiagnostics: frame.navigationDiagnostics,
   tabs: Array.isArray(frame.tabs) ? frame.tabs.slice(0, 4).map((tab: { id?: string; title?: string }) => ({ id: String(tab.id ?? '').slice(0, 128), title: String(tab.title ?? '').slice(0, 128) })) : [],
  });
  if (process.env.WEBMUX_BROWSER_NAVIGATION_DIAGNOSTICS === '1') expect(frame.navigationDiagnostics).toBeDefined();
  expect(frame.profileKey).toBeUndefined();
  if (frame.error) expect(frame.error).toBe('Page rendering was interrupted; retry or open the site in your browser');
  return frame;
 };
 try {
  const profile = await request.post(`${endpoint}/profiles`, { data: { name: 'Synthetic retained sign-in' } });
  expect(profile.status()).toBe(201);
  const registry = await profile.json(); saved = registry.id;
  expect(Object.keys(registry).sort()).toEqual(['id', 'name']);
  phase = 'initial-saved-worker';
  let frame = await action({ action: 'start', profileId: saved });
  token = frame.control.token;
  expect(frame.control.owned).toBe(true);
  expect(frame.profileId).toBe(saved);
  const observer = await action({ action: 'start', viewer: b, controlToken: '' });
  expect(observer.control).toMatchObject({ owned: false, available: false });
  expect(observer.control.token).toBeUndefined();
  for (const mutation of ['navigate', 'text', 'resize', 'download', 'dialog']) {
   const denied = await request.post(endpoint, { data: { action: mutation, url, viewer: b, controlToken: token } });
   expect(denied.status()).toBe(409);
  }
  const deniedEnd = await request.delete(endpoint, { data: { viewer: b, controlToken: token } });
  expect(deniedEnd.status()).toBe(409);
  await action({ action: 'navigate', url: `${url}/set` });
  // Navigation is intentionally asynchronous (including OAuth callbacks).
  // Observe the cookie-setting document before another navigation can cancel it.
  await expect.poll(async () => {
   const frame = await action({ action: 'frame' });
   return frame.url === `${url}/set` && frame.tabs.some((tab: { title: string }) => tab.title === 'empty-profile');
  }).toBe(true);
  await action({ action: 'navigate', url: `${url}/check` });
  await expect.poll(async () => (await action({ action: 'frame' })).tabs.map((tab: { title: string }) => tab.title)).toContain('retained-cookie');
  await action({ action: 'mouse_down', x: 20, y: 20, button: 'left', buttons: 1, clickCount: 1 });
  await expect.poll(() => pressed).toBe(1);
  const oldToken = token;
  frame = await action({ action: 'claim', viewer: b, controlToken: '', force: true });
  expect(frame.control.owned).toBe(true);
  const bToken = frame.control.token;
  await expect.poll(() => released).toBe(1);
  expect(bToken).not.toBe(oldToken);
  const stale = await request.post(endpoint, { data: { action: 'text', text: 'stale', viewer: a, controlToken: oldToken } });
  expect(stale.status()).toBe(409);
  const activeRemoval = await request.delete(`${endpoint}/profiles`, { data: { profileId: saved } });
  expect(activeRemoval.status()).toBe(409);
  await action({ action: 'release', viewer: b, controlToken: bToken });
  frame = await action({ action: 'claim', controlToken: '' }); token = frame.control.token;
  expect(frame.control.owned).toBe(true);
  expect((await request.delete(endpoint, { data: { viewer: a, controlToken: token } })).status()).toBe(204);
  token = '';
  phase = 'replacement-saved-worker';
  frame = await action({ action: 'start', profileId: saved }); token = frame.control.token;
  await action({ action: 'navigate', url: `${url}/check` });
  await expect.poll(async () => (await action({ action: 'frame' })).tabs.map((tab: { title: string }) => tab.title)).toContain('retained-cookie');
  expect((await request.delete(endpoint, { data: { viewer: a, controlToken: token } })).status()).toBe(204);
  token = '';
  expect((await request.delete(`${endpoint}/profiles`, { data: { profileId: saved } })).status()).toBe(204);
  saved = '';
  expect(await (await request.get(`${endpoint}/profiles`)).json()).toEqual([]);
  phase = 'replacement-temporary-worker';
  frame = await action({ action: 'start', profileId: '' }); token = frame.control.token;
  await action({ action: 'navigate', url: `${url}/check` });
  await expect.poll(async () => (await action({ action: 'frame' })).tabs.map((tab: { title: string }) => tab.title)).toContain('empty-profile');
 } finally {
  await request.delete(endpoint, { data: { viewer: a, controlToken: token } }).catch(() => {});
  if (saved) await request.delete(`${endpoint}/profiles`, { data: { profileId: saved } }).catch(() => {});
  await request.delete(`/api/sessions/${session.id}`).catch(() => {});
  if (process.platform !== 'win32') { try { execFileSync('tmux', ['kill-session', '-t', `webmux-${session.id}`]); } catch { /* already closed */ } }
  origin.closeAllConnections();
  await new Promise<void>(resolve => origin.close(() => resolve()));
  const receipt = info.outputPath('browser-profile-action-trace.json');
  writeFileSync(receipt, JSON.stringify({ timeline, dropped }));
  await info.attach('browser-profile-action-trace', { path: receipt, contentType: 'application/json' });
 }
});
