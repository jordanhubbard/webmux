// Called by the isolated WinSW lifecycle test with its private home and port.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { randomBytes } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { stripVTControlCharacters } from 'node:util';

assert.equal(process.platform, 'win32');
const [home, rawPort, installer] = process.argv.slice(2);
assert(home && path.isAbsolute(home) && installer && path.isAbsolute(installer));
assert(rawPort && /^\d+$/.test(rawPort));
const port = Number(rawPort); assert(port > 0 && port <= 65535);
const base = `http://127.0.0.1:${port}`;
const fixture = path.join(import.meta.dirname, 'service-terminal-fixture.mts');
assert(!process.execPath.includes('"') && !fixture.includes('"'));
const marker = randomBytes(8).toString('hex');
async function waitFor(check: () => Promise<boolean>, description: string): Promise<void> {
  const deadline = Date.now() + 15000;
  while (!await check()) {
    assert(Date.now() < deadline, `Timed out: ${description}`);
    await new Promise(resolve => setTimeout(resolve, 50));
  }
}
const response = await fetch(`${base}/api/sessions`, {
  method: 'POST', headers: { 'Content-Type': 'application/json' }, signal: AbortSignal.timeout(5000),
  body: JSON.stringify({ hostname: 'localhost', username: 'fixture', transport: 'exec',
    exec_command: `"${process.execPath}" "${fixture}" ${marker}`, cols: 120, rows: 30 }),
});
assert(response.ok, `Create PTY: ${response.status}`);
const session: unknown = await response.json();
assert(session && typeof session === 'object' && 'id' in session && typeof session.id === 'string');
let output = '';
let socketError = false;
const socket = new WebSocket(`ws://127.0.0.1:${port}/api/term/${session.id}`);
socket.addEventListener('error', () => { socketError = true; });
socket.addEventListener('message', event => {
  const value: unknown = JSON.parse(String(event.data));
  if (value && typeof value === 'object' && 'type' in value && value.type === 'output'
    && 'data' in value && typeof value.data === 'string') output += value.data;
});
const expected = Array.from({ length: 128 }, (_, index) => `${marker}:${index}:done`);
try {
  await waitFor(async () => {
    assert(!socketError, 'Terminal WebSocket failed');
    const plain = stripVTControlCharacters(output);
    return expected.every(line => plain.includes(line));
  }, 'complete PTY output');
  const childPID = Number(stripVTControlCharacters(output).match(/child:([1-9][0-9]*):ready/)?.[1]);
  assert(Number.isSafeInteger(childPID) && childPID > 1);
  process.kill(childPID, 0);
  const stopped = spawnSync('powershell.exe', ['-NoProfile', '-NonInteractive', '-File', installer, 'stop'], {
    encoding: 'utf8', timeout: 45000,
  });
  assert.ifError(stopped.error); assert.equal(stopped.status, 0, stopped.stdout + stopped.stderr);
  await waitFor(async () => socket.readyState === WebSocket.CLOSED, 'service terminal socket close');
  await waitFor(async () => {
    try { process.kill(childPID, 0); return false; }
    catch (error) { assert(error instanceof Error && 'code' in error && error.code === 'ESRCH'); return true; }
  }, 'service PTY child exit');
  const directory = path.join(home, 'logs/sessions');
  const files = (await fs.readdir(directory)).filter(name => name.startsWith(`session-${session.id}-`));
  assert.equal(files.length, 1);
  const transcript = stripVTControlCharacters(await fs.readFile(path.join(directory, files[0]!), 'utf8'));
  for (const line of expected) assert(transcript.includes(line), `Transcript lost ${line}`);
  assert.match(transcript, /\[webmux transcript stopped .* reason=shutdown\]\r?\n$/);
  console.log('WinSW stop closed the terminal, terminated its child and drained all acknowledged transcript output.');
} finally { socket.close(); }
