import { test, expect } from '@playwright/test';
import path from 'node:path';
import { randomUUID } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';

test('explicit recipients survive new registration and deliver bytes only after selection', async ({ page, request }, info) => {
  test.setTimeout(60_000);
  const ids: string[] = [];
  const files = new Map<string, string>();
  const sent = new Map<string, string[]>();
  const title = new Map<string, string>();
  page.on('websocket', socket => {
    const id = ids.find(id => socket.url().includes(`/api/term/${id}`));
    if (!id) return;
    socket.on('framesent', frame => {
      const message = JSON.parse(frame.payload.toString()) as { type: string; data: string };
      if (message.type === 'input') sent.get(id)!.push(message.data);
    });
  });
  const create = async (label: string, row: number, col: number) => {
    const file = path.resolve(__dirname, '.test-home', `broadcast-${randomUUID()}.bin`);
    writeFileSync(file, Buffer.alloc(0), { flag: 'wx', mode: 0o600 });
    const fixture = path.resolve(__dirname, 'broadcast-terminal-fixture.mts');
    const response = await request.post('/api/sessions', { data: {
      transport: 'exec', hostname: 'localhost', username: `broadcast-${label}`, row, col,
      exec_command: `"${process.execPath}" "${fixture}" "${file}"`,
    } });
    expect(response.ok()).toBe(true);
    const { id } = await response.json() as { id: string };
    ids.push(id); files.set(id, file); sent.set(id, []); title.set(id, `Broadcast ${label}`);
    expect((await request.patch(`/api/sessions/${id}`, { data: { title: title.get(id) } })).ok()).toBe(true);
    return id;
  };
  const bytes = (id: string) => readFileSync(files.get(id)!).toString('utf8');
  const tile = (id: string) => page.getByTestId(`tile-cell-${id}`);
  try {
    const a = await create('A', 0, 0), b = await create('B', 0, 1);
    await page.setViewportSize({ width: 1280, height: 1000 });
    await page.goto('/');
    for (const id of ids) await expect(tile(id).locator('.xterm-rows')).toContainText('BROADCAST READY');
    await page.getByRole('button', { name: 'Type to All', exact: true }).click();
    const recipients = page.getByRole('group', { name: 'Type to All recipients', exact: true });
    await recipients.getByRole('button', { name: 'None', exact: true }).click();
    await recipients.getByRole('checkbox', { name: title.get(a), exact: true }).check();
    const c = await create('C', 1, 0);
    // Use the production inventory notification without reloading the provider.
    await page.evaluate(() => window.dispatchEvent(new CustomEvent('webmux-sessions-changed')));
    await expect(tile(c).locator('.xterm-rows')).toContainText('BROADCAST READY');
    await expect(recipients.getByRole('checkbox', { name: title.get(c), exact: true })).not.toBeChecked();
    await tile(a).locator('.xterm-helper-textarea').focus(); await page.keyboard.type('x');
    await expect.poll(() => bytes(a)).toBe('x');
    expect(bytes(b)).toBe(''); expect(bytes(c)).toBe('');
    await recipients.getByRole('checkbox', { name: title.get(c), exact: true }).check();
    await tile(a).locator('.xterm-helper-textarea').focus(); await page.keyboard.type('y');
    await expect.poll(() => bytes(a)).toBe('xy');
    // The positive receipt on C is an ordered barrier for any earlier C input.
    await expect.poll(() => bytes(c)).toBe('y');
    await page.getByRole('button', { name: 'Type to All: ON', exact: true }).click();
    await tile(b).locator('.xterm-helper-textarea').focus(); await page.keyboard.type('b');
    await expect.poll(() => bytes(b)).toBe('b');
    expect(bytes(a)).toBe('xy'); expect(bytes(c)).toBe('y');
    // ConPTY enables terminal-local focus reports. They are consumed by the
    // console, not typed into this child. Keep all raw frames in the receipt;
    // exclude only these exact two protocol frames from the typing assertion.
    const typedHex = (id: string) => Buffer.from(sent.get(id)!
      .filter(data => data !== '\x1b[I' && data !== '\x1b[O').join('')).toString('hex');
    // Child-file writes and Playwright frame notifications arrive independently.
    // Observe both receipts before checking the exact input stream.
    await expect.poll(() => typedHex(a)).toBe('7879');
    await expect.poll(() => typedHex(b)).toBe('62');
    await expect.poll(() => typedHex(c)).toBe('79');
  } finally {
    const received = (index: number) => {
      const file = files.get(ids[index]);
      return file ? readFileSync(file).toString('hex') : null;
    };
    const receipt = {
      selectedOriginalHex: received(0),
      explicitlyIncludedLaterHex: received(2),
      ordinaryAfterModeOffHex: received(1),
      outboundFramesHex: ids.map(id => ({ fixture: title.get(id), frames: sent.get(id)!.map(data => Buffer.from(data).toString('hex')) })),
      receiptSource: 'Actual PTY child append-only byte files, not terminal screen text',
    };
    mkdirSync(info.outputDir, { recursive: true });
    const receiptPath = info.outputPath('native-broadcast-receipts.json');
    writeFileSync(receiptPath, JSON.stringify(receipt, null, 2) + '\n');
    try {
      await info.attach('native-broadcast-receipts', { path: receiptPath, contentType: 'application/json' });
    } finally {
      for (const id of ids) await request.delete(`/api/sessions/${id}`);
    }
  }
});

// Synthetic canonical metadata isolates frontend selection. The terminals and
// byte receipts are actual owned Go/PTY children; no remote/Rig coding is claimed.
test('project selection separates same-named destinations and stays frozen across metadata refresh', async ({ page, request }, info) => {
  test.setTimeout(60_000);
  const ids: string[] = [];
  const files = new Map<string, string>();
  const metadata = new Map<string, { project_id: string; project: string; name: string; host: string; target: string; runtime: string; provider?: string }>();
  const create = async (label: string, row: number, col: number, host: string, provider = 'rig') => {
    const file = path.resolve(__dirname, '.test-home', `broadcast-project-${randomUUID()}.bin`);
    writeFileSync(file, Buffer.alloc(0), { flag: 'wx', mode: 0o600 });
    const fixture = path.resolve(__dirname, 'broadcast-terminal-fixture.mts');
    const response = await request.post('/api/sessions', { data: { transport: 'exec', hostname: 'localhost', username: `project-${label}`, row, col,
      exec_command: `"${process.execPath}" "${fixture}" "${file}"` } });
    expect(response.ok()).toBe(true);
    const { id } = await response.json() as { id: string };
    ids.push(id); files.set(id, file);
    metadata.set(id, { project_id: provider === 'rig' ? `rig-${host}` : 'local:lab', project: 'lab', name: label === 'D' ? 'new worker' : 'worker', host, target: label, runtime: 'terminal', provider });
    return id;
  };
  const bytes = (id: string) => readFileSync(files.get(id)!).toString('utf8');
  const tile = (id: string) => page.getByTestId(`tile-cell-${id}`);
  try {
    const a = await create('A', 0, 0, 'foo');
    const b = await create('B', 0, 1, 'bar');
    const c = await create('C', 1, 0, 'foo', 'agent-deck');
    await page.route('**/api/sessions', async route => {
      if (route.request().method() !== 'GET') return route.continue();
      const response = await route.fetch();
      const rows = await response.json() as { id: string }[];
      await route.fulfill({ response, json: rows.map(row => metadata.has(row.id) ? { ...row, rig: metadata.get(row.id) } : row) });
    });
    await page.setViewportSize({ width: 1440, height: 1100 });
    await page.goto('/');
    for (const id of ids) await expect(tile(id).locator('.xterm-rows')).toContainText('BROADCAST READY');
    await page.getByRole('button', { name: 'Type to All', exact: true }).click();
    const recipients = page.getByRole('group', { name: 'Type to All recipients', exact: true });
    await recipients.getByRole('button', { name: 'Project: lab · Rig · foo', exact: true }).click();
    await expect(recipients.getByRole('checkbox', { name: 'lab/worker · Rig · foo', exact: true })).toBeChecked();
    await expect(recipients.getByRole('checkbox', { name: 'lab/worker · Rig · bar', exact: true })).not.toBeChecked();
    await expect(recipients.getByRole('checkbox', { name: 'lab/worker · Agent Deck · foo', exact: true })).not.toBeChecked();
    await tile(a).locator('.xterm-helper-textarea').focus(); await page.keyboard.type('x');
    await expect.poll(() => bytes(a)).toBe('x');
    metadata.set(a, { ...metadata.get(a)!, project: 'renamed' });
    const d = await create('D', 1, 1, 'foo');
    await page.evaluate(() => window.dispatchEvent(new CustomEvent('webmux-sessions-changed')));
    await expect(tile(d).locator('.xterm-rows')).toContainText('BROADCAST READY');
    await expect(recipients.getByRole('checkbox', { name: 'renamed/worker · Rig · foo', exact: true })).toBeChecked();
    await expect(recipients.getByRole('checkbox', { name: 'lab/new worker · Rig · foo', exact: true })).not.toBeChecked();
    await tile(a).locator('.xterm-helper-textarea').focus(); await page.keyboard.type('y');
    await expect.poll(() => bytes(a)).toBe('xy');
    await page.getByRole('button', { name: 'Type to All: ON', exact: true }).click();
    for (const [id, value] of [[b, 'b'], [c, 'c'], [d, 'd']]) {
      await tile(id).locator('.xterm-helper-textarea').focus(); await page.keyboard.type(value);
      // Each positive child write orders its earlier empty-stream assertion.
      await expect.poll(() => bytes(id)).toBe(value);
    }
    expect(bytes(a)).toBe('xy');
  } finally {
    mkdirSync(info.outputDir, { recursive: true });
    const receipt = info.outputPath('native-project-broadcast-receipts.json');
    writeFileSync(receipt, JSON.stringify({ source: 'Actual Go/PTY append-only files; canonical metadata is an explicit browser fixture',
      received: ids.map(id => ({ id, destination: metadata.get(id), hex: readFileSync(files.get(id)!).toString('hex') })) }, null, 2) + '\n');
    try { await info.attach('project broadcast byte receipts', { path: receipt, contentType: 'application/json' }); }
    finally { for (const id of ids) await request.delete(`/api/sessions/${id}`); }
  }
});
