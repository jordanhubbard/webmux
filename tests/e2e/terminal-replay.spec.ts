import { test, expect, type WebSocketRoute } from '@playwright/test';
import path from 'node:path';
import { writeFileSync } from 'node:fs';

test('rejoining a truncated full-screen replay keeps status bars out of scrollback', async ({ page, request }) => {
  const captures: string[] = [];
  page.on('websocket', socket => socket.on('framereceived', frame => {
    const message = JSON.parse(frame.payload.toString());
    if (message.type === 'output') captures.push(message.data);
  }));
  const fixture = path.resolve(__dirname, 'replay-terminal-fixture.mts');
  const created = await request.post('/api/sessions', { data: {
    hostname: 'localhost', username: 'replay-test', transport: 'exec',
    exec_command: `"${process.execPath}" "${fixture}"`,
  } });
  expect(created.ok()).toBe(true);
  const { id } = await created.json() as { id: string };
  try {
    await page.goto('/');
    const tile = page.getByTestId(`tile-cell-${id}`);
    const rows = tile.locator('.xterm-rows');
    await expect(rows).toContainText('REPLAY READY');
    // Rejoin after the provider has emitted >64 KiB; its entry sequence is gone.
    await page.reload();
    await expect(rows).toContainText('AGENT STATUS');
    await page.setViewportSize({ width: 1000, height: 650 });
    await page.setViewportSize({ width: 1280, height: 900 });
    const screen = tile.locator('.xterm-screen');
    await screen.hover();
    await page.mouse.wheel(0, -3000);
    // Order the assertion after the browser applies its wheel/scroll events.
    await expect.poll(() => tile.locator('.xterm-viewport').evaluate(el => el.scrollTop)).toBe(0);
    await expect(rows).toContainText('REPLAY READY');
    await expect(rows).toContainText('TMUX STATUS');
    await expect(rows).not.toContainText('OLD HISTORY');
  } finally {
    if (process.platform === 'win32') {
      console.log('WINDOWS_REPLAY_VT', JSON.stringify(captures.map(data => ({ bytes: data.length, head: data.slice(0, 128), tail: data.slice(-512), alternate: data.includes('\x1b[?1049h') }))));
      await test.info().attach('terminal-replay-vt', { body: Buffer.from(JSON.stringify(captures)), contentType: 'application/json' });
    }
    expect((await request.delete(`/api/sessions/${id}`, { timeout: 10_000 })).ok()).toBe(true);
  }
});


for (const profile of ['webmux-xterm-state-v1', 'webmux-xterm-state-v2']) {
test(`terminal advertises v2 and configures ${profile} metadata before replay`, async ({ page, request }) => {
  // The real app and Go server carry ordinary PTY traffic. Inject owned
  // handshake/output frames to isolate the frontend ordering contract; native
  // owner negotiation and unmodified ConPTY streams have separate coverage.
  const fixture = path.resolve(__dirname, 'terminal-fixture.mts');
  const response = await request.post('/api/sessions', { data: {
    hostname: 'localhost', username: 'profile-test', transport: 'exec',
    exec_command: `"${process.execPath}" "${fixture}"`,
  } });
  expect(response.ok()).toBe(true);
  const { id } = await response.json() as { id: string };
  const sockets: WebSocketRoute[] = [];
  const inputs: string[] = [];
  await page.routeWebSocket(new RegExp(`/api/term/${id}`), socket => {
    expect(new URL(socket.url()).searchParams.get('snapshot_profile')).toBe('webmux-xterm-state-v2');
    sockets.push(socket); socket.connectToServer();
  });
  page.on('websocket', socket => socket.on('framesent', frame => {
    const message = JSON.parse(frame.payload.toString()) as { type: string; data: string };
    if (message.type === 'input' && message.data !== '\x1b[I' && message.data !== '\x1b[O') inputs.push(message.data);
  }));
  try {
    for (let join = 0; join < 2; join++) {
      if (join) await page.reload(); else await page.goto('/');
      const rows = page.getByTestId(`tile-cell-${id}`).locator('.xterm-rows');
      await expect(rows).toContainText('READY');
      await expect.poll(() => sockets.length).toBe(join + 1);
      const socket = sockets[join];
      const token = (join ? 'b' : 'a').repeat(32);
      socket.send(JSON.stringify({ type: 'status', session_id: id, state: 'connected', viewer_id: 'profile-test', snapshot_profile: profile, snapshot_token: token }));
      const version = profile.endsWith('v2') ? 'v2' : 'v1';
      const payload = version === 'v2' ? Buffer.from(JSON.stringify({ u: 2, r: 'Z' })).toString('base64') : 'Wg==';
      socket.send(JSON.stringify({ type: 'output', session_id: id, data: `Z\x1b]7777;webmux.replay.${version};${token};${payload}\x07\x1b[2b` }));
      await expect(rows).toContainText('ZZZ');
      // The pinned DOM renderer uses raw extended style0 for an FG-only
      // underline flag, even though the public fallback style getter says1.
      if (profile.endsWith('v2')) await expect(rows.locator('.xterm-underline-0')).toContainText('ZZ');
      // Replaying an old token after changing trusted metadata cannot seed REP.
      socket.send(JSON.stringify({ type: 'status', session_id: id, state: 'connected', viewer_id: 'profile-test', snapshot_profile: profile, snapshot_token: 'c'.repeat(32) }));
      socket.send(JSON.stringify({ type: 'output', session_id: id, data: `Z\x1b]7777;webmux.replay.${version};${token};${payload}\x07\x1b[2b` }));
      await expect(rows).toContainText('Z');
      await expect(rows).not.toContainText('ZZ');
    }
    expect(inputs).toEqual([]);
    const receipt = test.info().outputPath('snapshot-profile-application-contract.json');
    writeFileSync(receipt, JSON.stringify({ advertised: 'webmux-xterm-state-v2', required: profile, attachments: 2, staleTokenIgnored: true, generatedInput: inputs, rawUnderlineClass: profile.endsWith('v2') ? 'xterm-underline-0' : undefined, scope: 'Actual app/Go server with injected handshake/output frames; not a native producer test' }));
    await test.info().attach('snapshot-profile-application-contract', { path: receipt, contentType: 'application/json' });
  } finally { await request.delete(`/api/sessions/${id}`); }
});

}
