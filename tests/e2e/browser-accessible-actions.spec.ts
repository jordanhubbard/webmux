import { test, expect } from '@playwright/test';
import http from 'node:http';
import { execFileSync } from 'node:child_process';

test('accessible controls retain live identity, refuse stale references and dispatch once', async ({ request }) => {
 test.setTimeout(90_000);
 let presses = 0, dynamicPresses = 0, linkDocuments = 0, label = 'Original action', observed = '';
 let automaticNavigation = false;
 const entered: string[] = [];
 const site = http.createServer((req, res) => {
  if (req.url === '/press') { presses++; res.end(); return; }
  if (req.url === '/dynamic-press') { dynamicPresses++; res.end(); return; }
  if (req.url === '/label') { res.end(label); return; }
  if (req.url === '/route') { res.end(automaticNavigation ? 'leave' : 'stay'); return; }
  if (req.url?.startsWith('/observed?')) { observed = new URL(req.url, 'http://fixture').searchParams.get('name')!; res.end(); return; }
  if (req.url?.startsWith('/typed?')) { entered.push(new URL(req.url, 'http://fixture').searchParams.get('value')!); res.end(); return; }
  res.setHeader('Content-Type', 'text/html; charset=utf-8');
  if (req.url === '/second') { linkDocuments++; res.end('<title>Accessible destination</title><h1>Destination</h1>'); return; }
  res.end(`<!doctype html><title>Accessible control fixture</title>
   <button id="press">Submit action</button><button id="dynamic">Original action</button>
   <input id="entry" aria-label="Private entry" value="private-native-value"><button disabled>Unavailable action</button>
   <a href="/second">Open destination</a><div id="shadow"></div>
   <script>
   press.onclick=()=>fetch('/press');dynamic.onclick=()=>fetch('/dynamic-press');
   entry.oninput=()=>fetch('/typed?value='+encodeURIComponent(entry.value));
   const shadowRoot=shadow.attachShadow({mode:'closed'});shadowRoot.innerHTML='<input aria-label="Shadow entry">';
   shadowRoot.querySelector('input').oninput=event=>fetch('/typed?value='+encodeURIComponent(event.target.value));
   setInterval(async()=>{const name=await(await fetch('/label')).text();if(dynamic.textContent!==name){dynamic.textContent=name;await fetch('/observed?name='+encodeURIComponent(name))}},100);
   setInterval(async()=>{if(await(await fetch('/route')).text()==='leave')location.href='/second'},100);
   </script>`);
 });
 await new Promise<void>(resolve => site.listen(0, '127.0.0.1', resolve));
 const origin = `http://127.0.0.1:${(site.address() as { port: number }).port}`;
 const created = await request.post('/api/sessions', { data: { transport: 'local' } });
 expect(created.ok()).toBe(true);
 const { id } = await created.json() as { id: string };
 const endpoint = `/api/sessions/${id}/browser`;
 const viewer = 'c'.repeat(32);
 let token = '';
 type Control = { name: string; role: string; element?: string; canActivate?: boolean; canFocus?: boolean };
 const call = async (data: Record<string, unknown>) => {
  const response = await request.post(endpoint, { data: { viewer, controlToken: token, ...data } });
  expect(response.ok()).toBe(true);
  const frame = await response.json();
  if (frame.control?.token) token = frame.control.token;
  return frame;
 };
 const read = async () => {
  const frame = await call({ action: 'frame' });
  const text = await call({ action: 'read_page', target: frame.target });
  expect(text.error).toBeUndefined();
  expect(JSON.stringify(text.pageText)).not.toContain('private-native-value');
  return { target: text.target as string, nodes: text.pageText.nodes as Control[] };
 };
 try {
  await call({ action: 'start' });
  await call({ action: 'navigate', url: origin });
  await expect.poll(async () => (await call({ action: 'frame' })).url).toBe(origin + '/');
  let snapshot = await read();
  const button = snapshot.nodes.find(n => n.name === 'Submit action')!;
  expect(button.canActivate).toBe(true);
  expect(button.element).toMatch(/^[a-f0-9]{32}-\d{1,3}$/);
  expect(snapshot.nodes.find(n => n.name === 'Unavailable action')?.element).toBeUndefined();
  const denied = await request.post(endpoint, { data: { action: 'activate_element', element: button.element, target: snapshot.target, viewer: 'd'.repeat(32), controlToken: token } });
  expect(denied.status()).toBe(409);
  const action = { action: 'activate_element', element: button.element, target: snapshot.target };
  expect((await call(action)).error).toBeUndefined();
  await expect.poll(() => presses).toBe(1);
  expect((await call(action)).error).toContain('changed');
  expect(presses).toBe(1);

  snapshot = await read();
  const field = snapshot.nodes.find(n => n.name === 'Private entry')!;
  expect(field.canFocus).toBe(true); expect(field.canActivate).toBeUndefined();
  expect((await call({ action: 'focus_element', target: snapshot.target, element: field.element })).error).toBeUndefined();
  await call({ action: 'key', key: 'a', code: 'KeyA', modifiers: process.platform === 'darwin' ? 4 : 2 });
  await call({ action: 'text', text: 'typed through native focus' });
  await expect.poll(() => entered).toContain('typed through native focus');
  snapshot = await read();
  const shadow = snapshot.nodes.find(n => n.name === 'Shadow entry')!;
  expect(shadow.canFocus).toBe(true);
  expect((await call({ action: 'focus_element', target: snapshot.target, element: shadow.element })).error).toBeUndefined();
  await call({ action: 'text', text: 'typed into closed shadow root' });
  await expect.poll(() => entered).toContain('typed into closed shadow root');

  snapshot = await read();
  const original = snapshot.nodes.find(n => n.name === 'Original action')!;
  label = 'Changed action';
  // The page changes itself; no WebMux mutation or read can invalidate the
  // reference before this control exercises the fresh AX identity check.
  await expect.poll(() => observed).toBe(label);
  expect((await call({ action: 'activate_element', target: snapshot.target, element: original.element })).error).toContain('changed');
  expect(dynamicPresses).toBe(0);
  snapshot = await read();
  const changed = snapshot.nodes.find(n => n.name === label)!;
  expect((await call({ action: 'activate_element', target: snapshot.target, element: changed.element })).error).toBeUndefined();
  await expect.poll(() => dynamicPresses).toBe(1);

  snapshot = await read();
  const old = snapshot.nodes.find(n => n.name === 'Submit action')!;
  await read();
  expect((await call({ action: 'activate_element', target: snapshot.target, element: old.element })).error).toContain('changed');
  expect(presses).toBe(1);
  snapshot = await read();
  const previousDocument = snapshot.nodes.find(n => n.name === 'Submit action')!;
  automaticNavigation = true;
  await expect.poll(async () => (await call({ action: 'frame' })).url).toBe(origin + '/second');
  // Only frame polls ran during this page-initiated navigation: the worker
  // must compare the captured document, not merely a cleared handle map.
  expect((await call({ action: 'activate_element', target: snapshot.target, element: previousDocument.element })).error).toContain('changed');
  expect(presses).toBe(1);
  automaticNavigation = false;
  await call({ action: 'navigate', url: origin });
  await expect.poll(async () => (await call({ action: 'frame' })).url).toBe(origin + '/');
  // A read followed only by frame polling and resize keeps semantic handles;
  // opening the text panel itself changes the viewport dimensions.
  snapshot = await read();
  await call({ action: 'resize', width: 1000, height: 700 });
  const link = snapshot.nodes.find(n => n.name === 'Open destination')!;
  await call({ action: 'activate_element', target: snapshot.target, element: link.element });
  await expect.poll(async () => (await call({ action: 'frame' })).url).toBe(origin + '/second');
  expect(linkDocuments).toBe(2);
  expect(presses).toBe(1);
 } finally {
  await request.delete(endpoint, { data: { viewer, controlToken: token } }).catch(() => {});
  await request.delete(`/api/sessions/${id}`).catch(() => {});
  if (process.platform !== 'win32') { try { execFileSync('tmux', ['kill-session', '-t', `webmux-${id}`]); } catch { /* already closed */ } }
  site.closeAllConnections();
  await new Promise<void>(resolve => site.close(() => resolve()));
 }
});
