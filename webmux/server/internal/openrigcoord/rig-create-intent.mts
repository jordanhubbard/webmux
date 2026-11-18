// Durable member-add correlation only. The caller must resolve the owner launch
// plan and verify the local manager before using this protocol. A native receipt
// records enrollment; it does not prove readiness or authenticated coding.
import fs from 'node:fs';
import path from 'node:path';
import { createHash, randomUUID } from 'node:crypto';

export interface RigCreateIntent {
  managerIdentity: string; port: number; rigId: string; pod: string; requestText: string;
}
export interface RigCreateReservation {
  version: 1; operationId: string; intent: RigCreateIntent;
}
export interface RigCreateResult {
  state: 'recorded' | 'uncertain'; code: string; operationId: string;
  posted: boolean; rigId?: string; nodeIds?: string[];
}
const ulid = /^[0-7][0-9A-HJKMNP-TV-Z]{25}$/;
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/;
const hash = /^[a-f0-9]{64}$/;
const maximum = 1024 * 1024;
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const digest = (value: string) => createHash('sha256').update(value).digest('hex');

function validate(intent: RigCreateIntent) {
  if (Object.keys(intent).sort().join(',') !== 'managerIdentity,pod,port,requestText,rigId'
    || !hash.test(intent.managerIdentity) || !Number.isSafeInteger(intent.port) || intent.port < 1024 || intent.port > 65535
    || !ulid.test(intent.rigId) || !/^[a-z][a-z0-9-]{0,47}$/.test(intent.pod)
    || typeof intent.requestText !== 'string' || Buffer.byteLength(intent.requestText) > maximum) throw new Error('invalid_native_create_intent');
  const request: unknown = JSON.parse(intent.requestText);
  if (!object(request) || !object(request.member)) throw new Error('invalid_native_create_intent');
}
function names(directory: string, key: string) {
  if (process.platform === 'win32') throw new Error('native_create_journal_requires_posix');
  if (!hash.test(key)) throw new Error('invalid_native_create_key');
  fs.mkdirSync(directory, { mode: 0o700 });
}
function root(directory: string, key: string) {
  try { names(directory, key); }
  catch (error) { if ((error as NodeJS.ErrnoException).code !== 'EEXIST') throw error; }
  const stat = fs.lstatSync(directory);
  if (!stat.isDirectory() || stat.isSymbolicLink() || stat.uid !== process.geteuid?.() || (stat.mode & 0o077) !== 0) {
    throw new Error('unsafe_native_create_journal');
  }
  const base = path.join(fs.realpathSync(directory), key);
  return { intent: `${base}.intent.json`, consumed: `${base}.dispatched` };
}
function syncDirectory(file: string) {
  const fd = fs.openSync(path.dirname(file), fs.constants.O_RDONLY);
  try { fs.fsyncSync(fd); } finally { fs.closeSync(fd); }
}
function exclusive(file: string, value: unknown) {
  const fd = fs.openSync(file, fs.constants.O_WRONLY | fs.constants.O_CREAT | fs.constants.O_EXCL, 0o600);
  try { fs.writeFileSync(fd, JSON.stringify(value)); fs.fsyncSync(fd); }
  finally { fs.closeSync(fd); }
  // If either flush fails, leave the artifact in place and refuse dispatch.
  // A partial/corrupt artifact never authorizes replacement with a new UUID.
  syncDirectory(file);
}
function read(file: string): unknown {
  const fd = fs.openSync(file, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW | fs.constants.O_NONBLOCK);
  try {
    const stat = fs.fstatSync(fd);
    if (!stat.isFile() || stat.uid !== process.geteuid?.() || (stat.mode & 0o077) !== 0 || stat.size > maximum + 8192) {
      throw new Error('unsafe_native_create_record');
    }
    const value: unknown = JSON.parse(fs.readFileSync(fd, 'utf8'));
    // An earlier process may have died after writing but before flushing.
    // Re-establish durability before these bytes can authorize dispatch.
    fs.fsyncSync(fd);
    syncDirectory(file);
    return value;
  } finally { fs.closeSync(fd); }
}
const same = (a: RigCreateIntent, b: RigCreateIntent) => a.managerIdentity === b.managerIdentity && a.port === b.port
  && a.rigId === b.rigId && a.pod === b.pod && a.requestText === b.requestText;

export function reserveRigCreateIntent(directory: string, key: string, intent: RigCreateIntent): RigCreateReservation {
  validate(intent);
  const files = root(directory, key);
  try { exclusive(files.intent, { version: 1, operationId: randomUUID(), intent }); }
  catch (error) { if ((error as NodeJS.ErrnoException).code !== 'EEXIST') throw error; }
  const saved = read(files.intent);
  if (!object(saved) || saved.version !== 1 || typeof saved.operationId !== 'string' || !uuid.test(saved.operationId)
    || !object(saved.intent)) throw new Error('invalid_native_create_record');
  const original = saved.intent as unknown as RigCreateIntent;
  validate(original);
  if (!same(original, intent)) throw new Error('native_create_intent_changed');
  return { version: 1, operationId: saved.operationId, intent: original };
}

async function request(fetcher: typeof fetch, intent: RigCreateIntent, operationId?: string) {
  const response = await fetcher(`http://127.0.0.1:${intent.port}` + (operationId
    ? `/api/rigs/${intent.rigId}/pods/${intent.pod}/members` : '/api/operations/capabilities'), {
    method: operationId ? 'POST' : 'GET', redirect: 'error', signal: AbortSignal.timeout(8000),
    ...(operationId ? { headers: { 'content-type': 'application/json', 'X-OpenRig-Operation-Id': operationId }, body: intent.requestText } : {}),
  });
  if (!response.body) throw new Error('native_response_unavailable');
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  try {
    for (;;) {
      const part = await reader.read();
      if (part.done) break;
      size += part.value.length;
      if (size > maximum) { await reader.cancel(); throw new Error('native_response_exceeds_limit'); }
      chunks.push(part.value);
    }
  } finally { reader.releaseLock(); }
  return { status: response.status, value: JSON.parse(Buffer.concat(chunks).toString('utf8')) as unknown };
}
async function capability(fetcher: typeof fetch, intent: RigCreateIntent) {
  const { status, value } = await request(fetcher, intent);
  if (status !== 200 || !object(value) || value.contract !== 'operation-correlation.v1'
    || !Array.isArray(value.kinds) || !value.kinds.includes('member.add')) throw new Error('native_create_correlation_unavailable');
}
async function post(reservation: RigCreateReservation, fetcher: typeof fetch): Promise<RigCreateResult> {
  const { operationId, intent } = reservation;
  const uncertain = (code: string): RigCreateResult => ({ state: 'uncertain', code, operationId, posted: true });
  try {
    const { status, value } = await request(fetcher, intent, operationId);
    const operation = object(value) ? value.operation : null;
    if (![200, 201].includes(status) || !object(operation) || operation.contract !== 'operation-correlation.v1'
      || operation.operationId !== operationId || operation.rigId !== intent.rigId || !Array.isArray(operation.nodeIds)
      || operation.nodeIds.length !== 1 || typeof operation.nodeIds[0] !== 'string' || !ulid.test(operation.nodeIds[0])) {
      return uncertain('native_create_unverified_receipt');
    }
    return { state: 'recorded', code: 'native_enrollment_recorded', operationId, posted: true,
      rigId: intent.rigId, nodeIds: [operation.nodeIds[0]] };
  } catch { return uncertain('native_create_response_unavailable'); }
}

export async function dispatchRigCreateIntent(directory: string, key: string, intent: RigCreateIntent, fetcher: typeof fetch = fetch): Promise<RigCreateResult> {
  const saved = reserveRigCreateIntent(directory, key, intent);
  await capability(fetcher, saved.intent);
  try { exclusive(root(directory, key).consumed, { operationId: saved.operationId, intentSHA256: digest(JSON.stringify(saved.intent)) }); }
  catch (error) {
    if ((error as NodeJS.ErrnoException).code !== 'EEXIST') throw error;
    return { state: 'uncertain', code: 'native_create_already_dispatched', posted: false, operationId: saved.operationId };
  }
  return post(saved, fetcher);
}

// Explicit inspection uses the SAME original native operation, route and exact
// bytes. Native correlation replays an existing reservation without executing it
// again. If the original POST never arrived, this can be its first execution;
// it never invents a replacement UUID or a different launch intent.
export async function inspectRigCreateIntent(directory: string, key: string, intent: RigCreateIntent, fetcher: typeof fetch = fetch): Promise<RigCreateResult> {
  const saved = reserveRigCreateIntent(directory, key, intent);
  const consumed = read(root(directory, key).consumed);
  if (!object(consumed) || consumed.operationId !== saved.operationId || consumed.intentSHA256 !== digest(JSON.stringify(saved.intent))) {
    throw new Error('invalid_native_create_dispatch_record');
  }
  await capability(fetcher, saved.intent);
  return post(saved, fetcher);
}
