#!/usr/bin/env node
// Local macOS daemon readiness only. This never restarts or resumes a session.
import fs from 'node:fs/promises';
import net from 'node:net';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { execFile } from 'node:child_process';
import { promisify, parseArgs } from 'node:util';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import type WebSocketType from 'ws';

const execute = promisify(execFile);
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const hash = (value: string | Buffer) => createHash('sha256').update(value).digest('hex');

export class ProbeFailure extends Error {
  readonly stage: string;
  readonly code: string | number;
  constructor(stage: string, code: string | number) { super(`${stage}: ${code}`); this.stage = stage; this.code = code; }
}

type JsonObject = Record<string, unknown>;
export type RpcMethod = 'thread/read' | 'thread/turns/list' | 'mcpServerStatus/list' | 'mcpServer/tool/call';
export interface RpcChannel { call(method: RpcMethod, params: JsonObject): Promise<unknown>; close(): void; }
export interface ProbeOptions { socket?: string; cwd?: string; thread?: string; viewerPid: number; daemonPid: number; turn?: string; }
export interface ProbeDependencies {
  epoch(pid: number): Promise<string>;
  owns(pid: number, args: string[], expected: string): Promise<boolean>;
  authorizeSocket(socket: string): Promise<string>;
  connect(socket: string): Promise<RpcChannel>;
  realpath(path: string): Promise<string>;
}
export interface ProbeReceipt {
  schema: string; scope: string; observed_at: string; thread_id?: string;
  passed: boolean; active_work_qualified: boolean; four_session_survival_qualified: false;
  viewer_pid: number; daemon_pid: number; process_epochs?: { viewer: string; daemon: string };
  expected_cwd_sha256?: string; endpoint_port?: number; cwd_matches?: boolean;
  management_tool_called?: string; turn_id?: string; probe_sha256?: string;
  failure?: { stage: string; code: string | number };
}
function object(value: unknown): JsonObject | undefined {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as JsonObject : undefined;
}
function objects(value: unknown): JsonObject[] {
  return Array.isArray(value) ? value.flatMap(item => { const row = object(item); return row ? [row] : []; }) : [];
}
function uid(): number {
  const value = process.getuid?.();
  if (value === undefined) throw new ProbeFailure('platform', 'unix-account-required');
  return value;
}

async function epoch(pid: number): Promise<string> {
  const { stdout } = await execute('/bin/ps', ['-p', String(pid), '-o', 'uid=', '-o', 'lstart=', '-o', 'comm='], {
    timeout: 3000, maxBuffer: 8192, env: { ...process.env, LC_ALL: 'C', TZ: 'UTC' },
  });
  const match = stdout.trim().match(/^(\d+)\s+(.{24})\s+(.+)$/u);
  if (!match || Number(match[1]) !== uid()) throw new ProbeFailure('process', 'account-or-epoch');
  const image = path.basename(match[3]);
  if (!['codex', 'codex-tui', 'codex-selection-prototype'].includes(image)) throw new ProbeFailure('process', 'unexpected-image');
  return hash(`${pid}\n${match[1]}\n${match[2]}\n${match[3]}`);
}

export async function owns(pid: number, args: string[], expected: string): Promise<boolean> {
  let stdout;
  try { ({ stdout } = await execute('/usr/sbin/lsof', ['-nP', '-a', '-p', String(pid), ...args, '-Fn'], { timeout: 3000, maxBuffer: 128 * 1024 })); }
  catch (error) { if (object(error)?.code === 1) return false; throw new ProbeFailure('process', 'inspector-unavailable'); }
  const lines = stdout.split('\n');
  return lines.includes(`p${pid}`) && lines.includes(`n${expected}`);
}

async function authorizeSocket(socket: string): Promise<string> {
  if (process.platform !== 'darwin') throw new ProbeFailure('platform', 'native-macos-required');
  const real = await fs.realpath(socket);
  const [info, directory] = await Promise.all([fs.stat(real), fs.stat(path.dirname(real))]);
  if (!info.isSocket() || info.uid !== uid() || directory.uid !== uid() ||
      (info.mode & 0o077) !== 0 || (directory.mode & 0o077) !== 0) throw new ProbeFailure('socket', 'account-or-permissions');
  return real;
}

export async function connect(socket: string): Promise<RpcChannel> {
  const require = createRequire(new URL('../package.json', import.meta.url));
  const WebSocket = require('ws') as typeof WebSocketType;
  const lock: unknown = JSON.parse(await fs.readFile(new URL('../package-lock.json', import.meta.url), 'utf8'));
  const installed = object(require('ws/package.json'))?.version;
  const locked = object(object(object(lock)?.packages)?.['node_modules/ws'])?.version;
  if (typeof installed !== 'string' || installed !== locked) throw new ProbeFailure('dependency', 'lock-mismatch');
  const ws = new WebSocket('ws://localhost/rpc', {
    createConnection: () => net.createConnection(socket), maxPayload: 4 * 1024 * 1024, handshakeTimeout: 5000,
  });
  const pending = new Map<number, { method: string; timer: ReturnType<typeof setTimeout>; resolve(value: unknown): void; reject(error: ProbeFailure): void }>();
  let next = 0;
  function fail(error: ProbeFailure) {
    for (const request of pending.values()) { clearTimeout(request.timer); request.reject(error); }
    pending.clear();
    ws.terminate();
  }
  ws.on('error', () => fail(new ProbeFailure('rpc', 'transport')));
  ws.on('close', () => fail(new ProbeFailure('rpc', 'closed')));
  ws.on('message', bytes => {
    let message: JsonObject | undefined;
    try { message = object(JSON.parse(bytes.toString())); } catch { fail(new ProbeFailure('rpc', 'invalid-json')); return; }
    if (!message) { fail(new ProbeFailure('rpc', 'invalid-envelope')); return; }
    if (message.method && message.id !== undefined) { fail(new ProbeFailure('rpc', 'interactive-request')); return; }
    const request = typeof message.id === 'number' ? pending.get(message.id) : undefined;
    if (!request) return; // Never persist unrelated daemon notifications.
    pending.delete(message.id as number);
    clearTimeout(request.timer);
    if (message.error) { const code = object(message.error)?.code; request.reject(new ProbeFailure(request.method, typeof code === 'number' && Number.isInteger(code) ? code : 'rpc-error')); }
    else request.resolve(message.result);
  });
  await new Promise<void>((resolve, reject) => { ws.once('open', resolve); ws.once('error', () => reject(new ProbeFailure('rpc', 'connect'))); ws.once('close', () => reject(new ProbeFailure('rpc', 'connect-closed'))); });
  const call = (method: string, params: JsonObject): Promise<unknown> => new Promise((resolve, reject) => {
    const id = ++next;
    const timer = setTimeout(() => { fail(new ProbeFailure(method, 'deadline')); }, 10000);
    pending.set(id, { method, timer, resolve, reject });
    ws.send(JSON.stringify({ id, method, params }));
  });
  try {
    await call('initialize', { clientInfo: { name: 'webmux-continuity-probe', version: '1' }, capabilities: { experimentalApi: true } });
    ws.send(JSON.stringify({ method: 'initialized' }));
    return { call, close: () => ws.terminate() };
  } catch (error) { const failure = error instanceof ProbeFailure ? error : new ProbeFailure('rpc', 'initialize-unavailable'); fail(failure); throw failure; }
}

// Dependencies are injectable for refusal tests. CLI qualification uses only
// these real OS inspectors and the authenticated native Unix-socket transport.
export async function probe(options: ProbeOptions, dependencies: Partial<ProbeDependencies> = {}): Promise<ProbeReceipt> {
  const io = { epoch, owns, authorizeSocket, connect, realpath: (value: string) => fs.realpath(value), ...dependencies };
  const receipt: ProbeReceipt = {
    schema: 'webmux.codex-task-tools-probe.v1', scope: 'one-local-daemon-thread-readiness',
    observed_at: new Date().toISOString(), thread_id: options.thread, passed: false,
    active_work_qualified: false, four_session_survival_qualified: false,
    viewer_pid: options.viewerPid, daemon_pid: options.daemonPid,
  };
  let channel: RpcChannel | undefined;
  try {
    if (typeof options.thread !== 'string' || !uuid.test(options.thread) ||
        typeof options.cwd !== 'string' || !path.isAbsolute(options.cwd) ||
        typeof options.socket !== 'string' || !path.isAbsolute(options.socket) ||
        ![options.viewerPid, options.daemonPid].every(pid => Number.isSafeInteger(pid) && pid > 1) ||
        (options.turn && !uuid.test(options.turn))) throw new ProbeFailure('arguments', 'invalid');
    const socket = await io.authorizeSocket(options.socket);
    const cwd = await io.realpath(options.cwd);
    const before = { viewer: await io.epoch(options.viewerPid), daemon: await io.epoch(options.daemonPid) };
    receipt.process_epochs = before;
    receipt.expected_cwd_sha256 = hash(cwd);
    if (!await io.owns(options.daemonPid, ['-U'], socket)) throw new ProbeFailure('daemon', 'socket-owner');
    channel = await io.connect(socket);
    const thread = object(object(await channel.call('thread/read', { threadId: options.thread, includeTurns: false }))?.thread);
    if (thread?.id !== options.thread || typeof thread.cwd !== 'string') throw new ProbeFailure('thread', 'identity');
    const actualCwd = thread.cwd.startsWith('file:') ? fileURLToPath(thread.cwd) : thread.cwd;
    if (!path.isAbsolute(actualCwd) || await io.realpath(actualCwd) !== cwd) throw new ProbeFailure('thread', 'project');
    const verifyTurn = async (expected: string) => {
      const turns = object(await channel!.call('thread/turns/list', { threadId: options.thread, limit: 1, sortDirection: 'desc' }));
      const current = objects(turns?.data)[0];
      if (current?.id !== expected || current.status !== 'inProgress') throw new ProbeFailure('turn', 'not-same-active-work');
    };
    if (options.turn) await verifyTurn(options.turn);
    const inventory = object(await channel.call('mcpServerStatus/list', { threadId: options.thread, serverName: 'codex_tui', detail: 'toolsAndAuthOnly' }));
    const servers = objects(inventory?.data).filter(server => server.name === 'codex_tui');
    if (servers?.length !== 1) throw new ProbeFailure('tools', 'ambiguous-registration');
    let origin: URL;
    try { if (typeof servers[0].httpOrigin !== 'string') throw new Error(); origin = new URL(servers[0].httpOrigin); } catch { throw new ProbeFailure('tools', 'unsupported-endpoint'); }
    const port = Number(origin.port);
    if (origin.protocol !== 'http:' || !['127.0.0.1', '[::1]'].includes(origin.hostname) ||
        !Number.isSafeInteger(port) || port < 1 || port > 65535 || origin.pathname !== '/' || origin.username || origin.password || origin.search || origin.hash) {
      throw new ProbeFailure('tools', 'unsupported-endpoint');
    }
    const address = `${origin.hostname}:${port}`;
    if (!await io.owns(options.viewerPid, [`-iTCP:${port}`, '-sTCP:LISTEN'], address)) throw new ProbeFailure('tools', 'viewer-endpoint-owner');
    // Connected inventory alone cannot pass. Make a real, same-thread, read-only
    // management call through its original native MCP runtime and authentication.
    const result = object(await channel.call('mcpServer/tool/call', {
      threadId: options.thread, server: 'codex_tui', tool: 'read_thread',
      arguments: { threadId: options.thread, turnLimit: 1, includeOutputs: false, maxOutputCharsPerItem: 0 },
    }));
    const returned = objects(result?.content).filter(item => item.type === 'text').flatMap(item => {
      if (typeof item.text !== 'string') return [];
      try { const value = object(JSON.parse(item.text)); return value ? [value] : []; } catch { return []; }
    });
    if (result?.isError === true || returned?.length !== 1 || returned[0]?.schemaVersion !== 1 || object(returned[0]?.thread)?.id !== options.thread) {
      throw new ProbeFailure('tools', 'management-result');
    }
    if (options.turn) await verifyTurn(options.turn);
    const after = { viewer: await io.epoch(options.viewerPid), daemon: await io.epoch(options.daemonPid) };
    if (before.viewer !== after.viewer || before.daemon !== after.daemon) throw new ProbeFailure('process', 'epoch-changed');
    receipt.process_epochs = before;
    receipt.viewer_pid = options.viewerPid;
    receipt.daemon_pid = options.daemonPid;
    receipt.endpoint_port = port;
    receipt.cwd_matches = true;
    receipt.management_tool_called = 'codex_tui/read_thread';
    receipt.active_work_qualified = Boolean(options.turn);
    if (options.turn) receipt.turn_id = options.turn;
    receipt.passed = true;
  } catch (error) {
    // Errors can contain source paths, raw transport headers or thread contents.
    // Retain only our fixed stage and code, never error text or response bodies.
    receipt.failure = error instanceof ProbeFailure ? { stage: error.stage, code: error.code } : { stage: 'probe', code: 'unavailable' };
  } finally { channel?.close(); }
  return receipt;
}

async function main() {
  const { values } = parseArgs({ options: {
    socket: { type: 'string' }, cwd: { type: 'string' }, thread: { type: 'string' },
    'viewer-pid': { type: 'string' }, 'daemon-pid': { type: 'string' }, turn: { type: 'string' }, output: { type: 'string' },
  } });
  if (!values.output) throw new ProbeFailure('arguments', 'new-output-required');
  const output = await fs.open(values.output, 'wx', 0o600);
  try {
    const receipt = await probe({ ...values, viewerPid: Number(values['viewer-pid']), daemonPid: Number(values['daemon-pid']) });
    receipt.probe_sha256 = hash(await fs.readFile(fileURLToPath(import.meta.url)));
    await output.writeFile(`${JSON.stringify(receipt, null, 2)}\n`);
    process.stdout.write(`${JSON.stringify({ passed: receipt.passed, failure: receipt.failure })}\n`);
    if (!receipt.passed) process.exitCode = 1;
  } finally { await output.close(); }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch(() => { process.stderr.write('Probe could not create a new private receipt or validate its arguments.\n'); process.exitCode = 1; });
}
