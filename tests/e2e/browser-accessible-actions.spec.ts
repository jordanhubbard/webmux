import { test, expect } from '@playwright/test';
import http from 'node:http';
import { execFileSync } from 'node:child_process';

test('accessible controls retain live identity, refuse stale references and dispatch once', async ({ request }) => {
 test.setTimeout(90_000);
 let presses = 0, dynamicPresses = 0, linkDocuments = 0, label = 'Original action', observed = '';
 let automaticNavigation = false;
 let viewport: { width: number; height: number } | undefined, confirmed = false, releases = 0;
 const entered: string[] = [];
 const framePresses: Record<string, number> = {};
 const frameTyped: Record<string, string> = {};
 const site = http.createServer((req, res) => {
  if (req.url === '/press') { presses++; res.end(); return; }
  if (req.url === '/dynamic-press') { dynamicPresses++; res.end(); return; }
  if (req.url === '/label') { res.end(label); return; }
  if (req.url === '/route') { res.end(automaticNavigation ? 'leave' : 'stay'); return; }
  if (req.url === '/confirmed') { confirmed = true; res.end(); return; }
  if (req.url === '/released') { releases++; res.end(); return; }
  if (req.url?.startsWith('/viewport?')) { const query = new URL(req.url, 'http://fixture').searchParams; viewport = { width: Number(query.get('width')), height: Number(query.get('height')) }; res.end(); return; }
  if (req.url?.startsWith('/observed?')) { observed = new URL(req.url, 'http://fixture').searchParams.get('name')!; res.end(); return; }
  if (req.url?.startsWith('/typed?')) { entered.push(new URL(req.url, 'http://fixture').searchParams.get('value')!); res.end(); return; }
  if (req.url?.startsWith('/frame-press?')) { const frame = new URL(req.url, 'http://fixture').searchParams.get('frame')!; framePresses[frame] = (framePresses[frame] || 0) + 1; res.end(); return; }
  if (req.url?.startsWith('/frame-typed?')) { const query = new URL(req.url, 'http://fixture').searchParams; frameTyped[query.get('frame')!] = query.get('value')!; res.end(); return; }
  res.setHeader('Content-Type', 'text/html; charset=utf-8');
  if (req.url?.startsWith('/embedded?')) {
   const frame = new URL(req.url, 'http://fixture').searchParams.get('frame')!;
   res.end(`<title>Embedded fixture</title><button onclick="fetch('/frame-press?frame=${frame}')">${frame} frame action</button>
    <input aria-label="${frame} frame field" value="private-native-value" oninput="fetch('/frame-typed?frame=${frame}&amp;value='+encodeURIComponent(this.value))">
    ${frame === 'cross' ? `<button onclick="if(confirm('Confirm accessible action?'))fetch('/confirmed')">Cross confirm action</button><iframe src="/embedded?frame=nested"></iframe>` : ''}`);
   return;
  }

  if (req.url === '/second') { linkDocuments++; res.end('<title>Accessible destination</title><h1>Destination</h1>'); return; }
  res.end(`<!doctype html><title>Accessible control fixture</title>
   <button id="press">Submit action</button><button id="dynamic">Original action</button><button id="confirm">Confirm action</button>
   <input id="entry" aria-label="Private entry" value="private-native-value"><button disabled>Unavailable action</button>
   <a href="/second">Open destination</a><div id="shadow"></div>
   <iframe src="/embedded?frame=same"></iframe><iframe src="http://localhost:${(site.address() as { port: number }).port}/embedded?frame=cross"></iframe>
   <script>
   press.onclick=()=>fetch('/press');dynamic.onclick=()=>fetch('/dynamic-press');
   document.getElementById('confirm').onclick=()=>{if(window.confirm('Confirm accessible action?'))fetch('/confirmed')};
   addEventListener('resize',()=>fetch('/viewport?width='+innerWidth+'&height='+innerHeight));
   addEventListener('mouseup',()=>fetch('/released'));
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

  for (const frame of ['same', 'cross', 'nested']) {
   snapshot = await read();
   const control = snapshot.nodes.find(n => n.name === `${frame} frame action`)!;
   expect(control.canActivate).toBe(true);
   const refused = await request.post(endpoint, { data: { action: 'activate_element', target: snapshot.target, element: control.element, viewer: 'd'.repeat(32), controlToken: token } });
   expect(refused.status()).toBe(409);
   expect(framePresses[frame] || 0).toBe(0);
   const activate = { action: 'activate_element', target: snapshot.target, element: control.element };
   expect((await call(activate)).error).toBeUndefined();
   await expect.poll(() => framePresses[frame]).toBe(1);
   expect((await call(activate)).error).toContain('changed');
   snapshot = await read();
   const entry = snapshot.nodes.find(n => n.name === `${frame} frame field`)!;
   expect((await call({ action: 'focus_element', target: snapshot.target, element: entry.element })).error).toBeUndefined();
   await call({ action: 'key', key: 'a', code: 'KeyA', modifiers: process.platform === 'darwin' ? 4 : 2 });
   await call({ action: 'text', text: `typed into ${frame} frame` });
   await expect.poll(() => frameTyped[frame]).toBe(`typed into ${frame} frame`);
   expect(framePresses[frame]).toBe(1);
  }

  await call({ action: 'mouse_down', x: 800, y: 500, button: 'left', buttons: 1, clickCount: 1 });
  snapshot = await read();
  const confirm = snapshot.nodes.find(n => n.name === 'Cross confirm action')!;
  const opened = await call({ action: 'activate_element', target: snapshot.target, element: confirm.element });
  expect(opened.dialog?.type).toBe('confirm');
  const began = performance.now();
  const deferred = await call({ action: 'resize', width: 900, height: 650 });
  expect(performance.now() - began).toBeLessThan(1000);
  expect(deferred.error).toBeUndefined();
  expect([deferred.width, deferred.height]).toEqual([opened.width, opened.height]);
  const inputBegan = performance.now();
  const blocked = await call({ action: 'text', text: 'must-not-reach-page-under-modal' });
  expect(performance.now() - inputBegan).toBeLessThan(1000);
  expect(blocked.error).toContain('Close the current browser dialog');
  expect(blocked.dialog?.type).toBe('confirm');
  const previousToken = token, claimBegan = performance.now();
  const claimed = await call({ action: 'claim', force: true });
  expect(performance.now() - claimBegan).toBeLessThan(1000);
  expect(token).not.toBe(previousToken);
  expect(claimed.dialog?.type).toBe('confirm');
  const handled = await call({ action: 'dialog', target: snapshot.target, accept: true });
  expect(handled.error).toBeUndefined();
  expect([handled.width, handled.height]).toEqual([900, 650]);
  await expect.poll(() => viewport).toEqual({ width: 900, height: 650 });
  await expect.poll(() => confirmed).toBe(true);
  await expect.poll(() => releases).toBe(1);
  expect(presses).toBe(1);

  // Preserve the original root-dialog control alongside the OOPIF case.
  snapshot = await read();
  const rootConfirm = snapshot.nodes.find(n => n.name === 'Confirm action')!;
  confirmed = false;
  expect((await call({ action: 'activate_element', target: snapshot.target, element: rootConfirm.element })).dialog?.type).toBe('confirm');
  expect((await call({ action: 'dialog', target: snapshot.target, accept: true })).error).toBeUndefined();
  await expect.poll(() => confirmed).toBe(true);

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
