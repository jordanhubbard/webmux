import { test, expect } from '@playwright/test';
import http from 'node:http';
import { readFileSync } from 'node:fs';

test('browser supports interactive pages, dialogs, clipboard, files, navigation and tabs', async ({ page, request }) => {
 test.setTimeout(120_000);
 const signals: string[] = [];
 let uploaded = Buffer.alloc(0);
 const downloadBytes = Buffer.from('browser download bytes\n😀\u0000');
 const server = http.createServer((req, res) => {
  if (req.url?.startsWith('/signal?')) { signals.push(new URL(req.url, 'http://fixture').searchParams.get('value')!); res.end('ok'); return; }
  if (req.url === '/upload') {
   const chunks: Buffer[] = []; req.on('data', chunk => chunks.push(chunk)); req.on('end', () => { uploaded = Buffer.concat(chunks); res.end('ok'); }); return;
  }
  if (req.url === '/download') { res.setHeader('Content-Disposition', 'attachment; filename="browser-proof.bin"'); res.setHeader('Content-Type', 'application/octet-stream'); res.end(downloadBytes); return; }
  res.setHeader('Content-Type', 'text/html');
  if (req.url === '/second') { res.end('<title>Second page</title><h1>Second page</h1>'); return; }
  res.end(`<!doctype html><title>Browser work fixture</title><style>body{margin:0;font:16px sans-serif}button,input,a,.area{position:absolute;box-sizing:border-box;height:40px;width:190px}#drag{left:20px;top:20px;background:#bad}#hover{left:240px;top:20px;background:#bda}#confirm{left:20px;top:80px}#double{left:240px;top:80px}#files{left:20px;top:140px}#context{left:240px;top:140px}#download{left:20px;top:200px}#prompt{left:240px;top:200px}#text{left:20px;top:260px;width:400px}#popup{left:20px;top:320px}#select{position:absolute;left:240px;top:320px;height:40px;width:190px}</style>
   <div id="drag" class="area">Drag here</div><div id="hover" class="area">Hover here</div>
   <button id="confirm">Confirm</button><button id="double">Double click</button>
   <input id="files" type="file"><div id="context" class="area">Right click here</div>
   <a id="download" href="/download">Download fixture</a><button id="prompt">Prompt</button>
   <input id="text" value=""><button id="popup">Open popup</button>
   <select id="select"><option>First</option><option>Second</option></select>
   <script>
    const signal=value=>fetch('/signal?value='+encodeURIComponent(value));
    let held=false;
    drag.onmousedown=()=>{held=true}; document.onmouseup=()=>{if(held){held=false;signal('drag')}};
    hover.onmouseenter=()=>signal('hover');
    document.getElementById('confirm').onclick=()=>signal('confirm:'+window.confirm('Continue with fixture?'));
    double.ondblclick=()=>signal('double');
    context.oncontextmenu=event=>{event.preventDefault();signal('context')};
    document.getElementById('prompt').onclick=()=>signal('prompt:'+window.prompt('Your answer','initial'));
    files.onchange=()=>fetch('/upload',{method:'POST',body:files.files[0]});
    text.oninput=()=>signal('text:'+text.value);
    text.onselect=()=>signal('selection:'+text.selectionStart+':'+text.selectionEnd);
    text.onkeydown=event=>{if(event.key==='Escape')signal('escape')};
    popup.onclick=()=>window.open('/second','_blank');
    document.getElementById('select').onchange=event=>signal('select:'+event.target.value);
   </script>`);
 });
 await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
 const origin = `http://127.0.0.1:${(server.address() as { port: number }).port}`;
 const created = await request.post('/api/sessions', { data: { transport: 'local' } });
 expect(created.ok()).toBe(true); const { id } = await created.json();
 const endpoint = `/api/sessions/${id}/browser`;
 try {
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
  await page.goto('/');
  await page.getByTestId(`tile-cell-${id}`).getByRole('button', { name: 'Session tools', exact: true }).click();
  await page.getByRole('button', { name: 'Open browser', exact: true }).click();
  const image = page.getByAltText('Interactive remote browser viewport');
  await expect(image).toBeVisible({ timeout: 30_000 });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.getByRole('button', { name: 'Return to terminal', exact: true })).toBeInViewport();
  await expect(page.getByRole('textbox', { name: 'Remote browser URL' })).toBeInViewport();
  await expect.poll(async () => (await (await request.post(endpoint, { data: { action: 'frame' } })).json()).width).toBeLessThanOrEqual(390);
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.getByRole('button', { name: 'Expand browser', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Restore browser size', exact: true })).toBeVisible();
  await page.getByRole('textbox', { name: 'Remote browser URL' }).fill(origin);
  await page.getByRole('button', { name: 'Go', exact: true }).click();
  await expect.poll(async () => (await (await request.post(endpoint, { data: { action: 'frame' } })).json()).url).toBe(origin + '/');
  await expect.poll(async () => (await image.evaluate(el => (el as HTMLImageElement).naturalWidth))).toBeGreaterThan(900);
  const position = async (x: number, y: number) => image.evaluate((el, p) => {
   const img = el as HTMLImageElement; const box = img.getBoundingClientRect();
   return { x: box.x + p.x * box.width / img.naturalWidth, y: box.y + p.y * box.height / img.naturalHeight };
  }, { x, y });
  const click = async (x: number, y: number, button: 'left' | 'right' = 'left') => {
   const p = await position(x, y); await page.mouse.click(p.x, p.y, { button });
  };
  const hover = await position(280, 40); await page.mouse.move(hover.x, hover.y);
  await expect.poll(() => signals).toContain('hover');
  const start = await position(50, 40); const end = await position(150, 50);
  await page.mouse.move(start.x, start.y); await page.mouse.down(); await page.mouse.move(end.x, end.y, { steps: 8 }); await page.mouse.up();
  await expect.poll(() => signals).toContain('drag');
  const twice = await position(280, 100); await page.mouse.dblclick(twice.x, twice.y);
  await expect.poll(() => signals).toContain('double');
  await click(280, 160, 'right'); await expect.poll(() => signals).toContain('context');
  await click(50, 100);
  const confirm = page.getByRole('dialog', { name: 'Browser confirm', exact: true });
  await expect(confirm).toBeVisible();
  const wrongDialog = await (await request.post(endpoint, { data: { action: 'dialog', target: 'another-tab', accept: true } })).json();
  expect(wrongDialog.error).toContain('another tab'); expect(wrongDialog.dialog.type).toBe('confirm');
  await confirm.getByRole('button', { name: 'OK', exact: true }).click();
  await expect.poll(() => signals).toContain('confirm:true');
  await expect(confirm).not.toBeVisible();
  await click(280, 220);
  const prompt = page.getByRole('dialog', { name: 'Browser prompt', exact: true });
  await expect(prompt).toBeVisible(); await prompt.getByLabel('Browser prompt response').fill('native-dialog-proof');
  await prompt.getByRole('button', { name: 'OK', exact: true }).click();
  await expect.poll(() => signals).toContain('prompt:native-dialog-proof');
  await expect(prompt).not.toBeVisible();
  await click(50, 280);
  await page.keyboard.type('browser clipboard proof');
  await expect.poll(() => signals).toContain('text:browser clipboard proof');
  const mac = await page.evaluate(() => /Mac|iPhone|iPad/.test(navigator.platform));
  await page.keyboard.press(mac ? 'Meta+a' : 'Control+a');
  await page.getByRole('button', { name: 'Copy selection', exact: true }).click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('browser clipboard proof');
  await click(50, 280);
  await page.keyboard.press(mac ? 'Meta+a' : 'Control+a');
  await page.keyboard.insertText('日本語😀');
  await expect.poll(() => signals).toContain('text:日本語😀');
  await page.keyboard.press('Escape'); await expect.poll(() => signals).toContain('escape');
  // Native browser dropdowns need an actual control rather than a bitmap of an OS popup.
  await click(280, 340);
  await page.getByRole('dialog', { name: 'Choose a page option' }).getByLabel('Page option', { exact: true }).selectOption('Second');
  await expect.poll(() => signals).toContain('select:Second');
  await expect(page.getByRole('dialog', { name: 'Choose a page option' })).not.toBeVisible();
  await click(50, 160);
  await expect(page.getByLabel('Choose files for page')).toBeVisible();
  const invalidUpload = await (await request.post(endpoint, { data: { action: 'upload', files: [{ name: '../escape', data: 'aGVsbG8=' }] } })).json();
  expect(invalidUpload.error).toContain('Invalid upload filename');
  const invalidDownload = await (await request.post(endpoint, { data: { action: 'download', target: '../../config/auth.yaml' } })).json();
  expect(invalidDownload.error).toContain('Invalid download'); expect(invalidDownload.file).toBeUndefined();
  const file = Buffer.alloc(1100 * 1024, 0x5a); file.write('larger-than-the-old-protocol-limit');
  await page.getByLabel('Choose files for page').setInputFiles({ name: 'upload-proof.bin', mimeType: 'application/octet-stream', buffer: file });
  await expect.poll(() => uploaded.length, { timeout: 15_000 }).toBe(file.length); expect(uploaded).toEqual(file);
  await expect(page.getByRole('dialog', { name: 'Upload files to this page' })).not.toBeVisible();
  await click(60, 220);
  await page.locator('.browser-companion summary').click();
  const save = page.getByRole('button', { name: 'Save browser-proof.bin', exact: true }); await expect(save).toBeEnabled({ timeout: 15_000 });
  const received = page.waitForEvent('download'); await save.click(); const downloaded = await received;
  expect(readFileSync((await downloaded.path())!)).toEqual(downloadBytes);
  await page.locator('.browser-companion summary').click();
  const originalTab = await page.getByLabel('Remote browser page').inputValue();
  await page.getByRole('textbox', { name: 'Remote browser URL' }).fill(origin + '/second'); await page.getByRole('button', { name: 'Go', exact: true }).click();
  await expect.poll(async () => (await (await request.post(endpoint, { data: { action: 'frame' } })).json()).url).toBe(origin + '/second');
  await expect(page.getByRole('button', { name: 'Browser back', exact: true })).toBeEnabled();
  await page.getByRole('button', { name: 'Browser back', exact: true }).click();
  await expect(page.getByRole('textbox', { name: 'Remote browser URL' })).toHaveValue(origin + '/');
  await page.getByRole('button', { name: 'Browser forward', exact: true }).click();
  await expect(page.getByRole('textbox', { name: 'Remote browser URL' })).toHaveValue(origin + '/second');
  await page.getByRole('button', { name: 'New tab', exact: true }).click();
  await expect(page.getByLabel('Remote browser page').locator('option')).toHaveCount(2);
  await page.getByRole('button', { name: 'Close tab', exact: true }).click();
  await expect(page.getByLabel('Remote browser page')).toHaveValue(originalTab);
  await page.getByRole('button', { name: 'Browser back', exact: true }).click();
  await expect(page.getByRole('textbox', { name: 'Remote browser URL' })).toHaveValue(origin + '/');
  await click(50, 340);
  await expect(page.getByLabel('Remote browser page').locator('option')).toHaveCount(2);
  const popup = await page.getByLabel('Remote browser page').locator('option').evaluateAll((options, old) => options.map(option => (option as HTMLOptionElement).value).find(value => value !== old), originalTab);
  await page.getByLabel('Remote browser page').selectOption(popup!);
  await expect(page.getByRole('textbox', { name: 'Remote browser URL' })).toHaveValue(origin + '/second');
  await page.getByRole('button', { name: 'Close tab', exact: true }).click();
  await expect(page.getByLabel('Remote browser page')).toHaveValue(originalTab);
  // Closing the first/root tab must leave the other tab and browser alive.
  await page.getByRole('button', { name: 'New tab', exact: true }).click();
  await expect(page.getByLabel('Remote browser page').locator('option')).toHaveCount(2);
  const retainedTab = await page.getByLabel('Remote browser page').inputValue();
  await page.getByLabel('Remote browser page').selectOption(originalTab);
  await page.getByRole('button', { name: 'Close tab', exact: true }).click();
  await expect(page.getByLabel('Remote browser page')).toHaveValue(retainedTab);
  await expect(page.getByLabel('Remote browser page').locator(`option[value="${originalTab}"]`)).toHaveCount(0);
  await page.getByRole('textbox', { name: 'Remote browser URL' }).fill(origin + '/second');
  await page.getByRole('button', { name: 'Go', exact: true }).click();
  await expect.poll(async () => (await (await request.post(endpoint, { data: { action: 'frame' } })).json()).url).toBe(origin + '/second');
  await image.focus(); await page.keyboard.press(mac ? 'Meta+f' : 'Control+f');
  await page.getByLabel('Find on page').fill('Second');
  await page.getByRole('button', { name: 'Next match', exact: true }).click();
  await expect(page.locator('.browser-find').getByRole('status')).toHaveText('Match found.');
  await page.getByRole('button', { name: 'Copy selection', exact: true }).click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('Second');
  await page.getByRole('button', { name: 'Close find', exact: true }).click();
  await page.screenshot({ path: 'test-results/browser-workspace.png' });
  await page.getByRole('button', { name: 'Return to terminal', exact: true }).click();
  await page.getByTestId(`tile-cell-${id}`).getByRole('button', { name: 'Session tools', exact: true }).click();
  await page.getByRole('button', { name: 'Open browser', exact: true }).click(); await expect(image).toBeVisible();
  await expect(page.getByLabel('Remote browser page')).toHaveValue(retainedTab);
  await page.getByRole('button', { name: 'End browser', exact: true }).click();
  await expect(image).not.toBeVisible();
  expect((await request.post(endpoint, { data: { action: 'frame' } })).status()).toBe(503);
 } finally {
  await request.delete(`/api/sessions/${id}`);
  await new Promise<void>(resolve => server.close(() => resolve()));
 }
});
