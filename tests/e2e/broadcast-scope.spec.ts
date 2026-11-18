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
    expect(sent.get(a)!.join('')).toBe('xy');
    expect(sent.get(b)!.join('')).toBe('b');
    expect(sent.get(c)!.join('')).toBe('y');
    const receipt = {
      selectedOriginalHex: readFileSync(files.get(a)!).toString('hex'),
      explicitlyIncludedLaterHex: readFileSync(files.get(c)!).toString('hex'),
      ordinaryAfterModeOffHex: readFileSync(files.get(b)!).toString('hex'),
      receiptSource: 'Actual PTY child append-only byte files, not terminal screen text',
    };
    mkdirSync(info.outputDir, { recursive: true });
    const receiptPath = info.outputPath('native-broadcast-receipts.json');
    writeFileSync(receiptPath, JSON.stringify(receipt, null, 2) + '\n');
    await info.attach('native-broadcast-receipts', { path: receiptPath, contentType: 'application/json' });
  } finally {
    for (const id of ids) await request.delete(`/api/sessions/${id}`);
  }
});
