// One verified conversation for deliberate original-only recovery. No transcript
// suffix, account lookup or provider launch occurs while capturing metadata.
import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { nativeCreateArtifacts as artifacts } from './rig-native-create.mts';
import { launchRigNativeCodex } from './rig-native-launch.mts';
import { validateCodexLaunchPlan } from './codex-launch-policy.mts';
import { reviewRigCreateManager, type RigCreateManager, type RigStopRun } from './rig-stop.mts';
import type { RigRecoveryIdentity } from './rig-recovery.mts';
const hash = (value: string | Buffer) => createHash('sha256').update(value).digest('hex');
const object = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const ulid = /^[0-7][0-9A-HJKMNP-TV-Z]{25}$/;
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const refuse = (): never => { throw new Error('native_recovery_conversation_unverified'); };
export interface NativeCodexCaptureInput {
  directory: string; identity: RigRecoveryIdentity; rigId: string; nodeId: string; pod: string; member: string; cwd: string;
}
export interface NativeCodexRecoveryReference {
  contract: 'webmux-rig-native-codex-recovery/v1'; rigId: string; nodeId: string; pod: string; member: string; cwd: string;
  sessionName: string; snapshotId: string; historySessionId: string; nativeSessionId: string;
  nativeStateFile: string; nativeRecordIdentity: string; snapshotSHA256: string; planFingerprint: string;
}
interface Dependencies {
  run: RigStopRun; fetcher?: typeof fetch; launch?: typeof launchRigNativeCodex; review?: () => Promise<RigCreateManager>;
}
async function request(fetcher: typeof fetch, port: number, route: string, method = 'GET') {
  const response = await fetcher(`http://127.0.0.1:${port}${route}`, { method, redirect: 'error', signal: AbortSignal.timeout(5000) });
  if (![200, 201].includes(response.status) || !response.body) return refuse();
  const reader = response.body.getReader(), chunks: Uint8Array[] = []; let size = 0;
  try { for (;;) {
    const part = await reader.read(); if (part.done) break;
    size += part.value.length; if (size > 2 * 1024 * 1024) { await reader.cancel(); return refuse(); } chunks.push(part.value);
  } } finally { reader.releaseLock(); }
  const value: unknown = JSON.parse(Buffer.concat(chunks).toString('utf8')); if (!object(value)) return refuse(); return value;
}
function nativeRecord(record: Record<string, unknown>, home: string, cwd: string) {
  if (record.runtime !== 'codex' || typeof record.sessionId !== 'string' || !uuid.test(record.sessionId)
    || typeof record.generationId !== 'string' || record.generationId.length > 8192) return refuse();
  const generation: unknown = JSON.parse(record.generationId);
  if (!Array.isArray(generation) || generation.length !== 6 || typeof generation[0] !== 'string' || !uuid.test(generation[0])
    || generation[1] !== record.sessionId || typeof generation[2] !== 'string' || generation[2].length > 4096 || !path.isAbsolute(generation[2])) return refuse();
  const file = fs.realpathSync(generation[2]), sessions = fs.realpathSync(path.join(home, 'sessions'));
  if (file !== generation[2] || !file.startsWith(sessions + path.sep)) return refuse();
  const fd = fs.openSync(file, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW | fs.constants.O_NONBLOCK);
  try {
    const stat = fs.fstatSync(fd);
    if (!stat.isFile() || stat.uid !== process.geteuid?.() || stat.nlink !== 1
      || JSON.stringify(generation.slice(3)) !== JSON.stringify([stat.dev, stat.ino, stat.birthtimeMs])) return refuse();
    const bytes = Buffer.alloc(Math.min(stat.size, 1024 * 1024));
    const count = fs.readSync(fd, bytes, 0, bytes.length, 0), newline = bytes.subarray(0, count).indexOf(10); if (newline < 0) return refuse();
    const header: unknown = JSON.parse(bytes.subarray(0, newline).toString('utf8'));
    if (!object(header) || header.type !== 'session_meta' || !object(header.payload) || header.payload.id !== record.sessionId || header.payload.cwd !== cwd) return refuse();
    const after = fs.fstatSync(fd);
    if (after.dev !== stat.dev || after.ino !== stat.ino || after.birthtimeMs !== stat.birthtimeMs || after.size < stat.size) return refuse();
    return { file, id: record.sessionId, generationId: record.generationId, identity: hash(JSON.stringify([record.sessionId, file, stat.dev, stat.ino, stat.birthtimeMs])) };
  } finally { fs.closeSync(fd); }
}
export async function captureRigNativeCodex(input: NativeCodexCaptureInput, deps: Dependencies): Promise<NativeCodexRecoveryReference> {
  const identity = input.identity;
  if (!['darwin', 'linux'].includes(process.platform) || ![input.rigId, input.nodeId].every(id => ulid.test(id))
    || !/^[a-z][a-z0-9-]{0,47}$/.test(input.pod) || !/^[a-z][a-z0-9-]{0,63}$/.test(input.member)
    || identity.host !== 'localhost' || identity.runtime !== 'codex' || (identity.provider && identity.provider !== 'rig')) return refuse();
  const base = artifacts.ownedRoot(input.directory), cwd = fs.realpathSync(input.cwd); if (cwd !== identity.workspace) return refuse();
  const key = hash(JSON.stringify([input.rigId, input.pod, input.member]));
  const directory = artifacts.ownedRoot(path.join(artifacts.ownedRoot(path.join(base, 'requests')), key));
  const registration: unknown = JSON.parse(artifacts.read(path.join(directory, 'native-registration.json')).toString('utf8'));
  if (!object(registration) || registration.nodeId !== input.nodeId || registration.rigId !== input.rigId
    || registration.pod !== input.pod || registration.member !== input.member || registration.cwd !== cwd) return refuse();
  const launch: unknown = JSON.parse(artifacts.read(path.join(directory, 'launch.json')).toString('utf8'));
  if (!object(launch) || !object(launch.intent) || typeof launch.intent.requestText !== 'string') return refuse();
  const plan = validateCodexLaunchPlan(launch.plan), initial: unknown = JSON.parse(launch.intent.requestText);
  if (!object(initial) || !object(initial.member) || !Array.isArray(initial.member.launch_wrapper)) return refuse();
  // A missing registration must never cause the creation factory to run.
  const ready = await (deps.launch ?? launchRigNativeCodex)({ directory: base, rigId: input.rigId, pod: input.pod, member: input.member, cwd },
    { run: deps.run, fetcher: deps.fetcher, create: async () => { throw new Error('native_recovery_cannot_create'); } });
  if (ready.state !== 'attached' || ready.nodeId !== input.nodeId || identity.target !== ready.target
    || identity.tmux_server_flag !== ready.tmuxServerFlag || identity.tmux_server_value !== ready.tmuxServerValue) return refuse();
  const fetcher = deps.fetcher ?? fetch, review = deps.review ?? (() => reviewRigCreateManager(deps.run, fetcher)), manager = await review();
  if (manager.port !== launch.intent.port || manager.managerIdentity !== launch.intent.managerIdentity) return refuse();
  const recordRoute = `/api/sessions/${encodeURIComponent(ready.target)}/generation-record`;
  // Omitting sinceBytes asks for identity only, never conversation contents.
  const before = nativeRecord(await request(fetcher, manager.port, recordRoute), plan.home, cwd);
  const cacheKey = hash(JSON.stringify([input.rigId, input.nodeId, ready.sessionId, before.generationId, plan.fingerprint]));
  const captures = artifacts.ownedRoot(path.join(directory, 'recovery-captures')), cached = path.join(captures, cacheKey + '.json');
  let selected: NativeCodexRecoveryReference | undefined;
  try { selected = JSON.parse(artifacts.read(cached).toString('utf8')) as NativeCodexRecoveryReference; }
  catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
  let snapshot: Record<string, unknown>;
  if (selected !== undefined) {
    if (!object(selected) || typeof selected.snapshotId !== 'string' || !ulid.test(selected.snapshotId)) return refuse();
    snapshot = await request(fetcher, manager.port, `/api/rigs/${input.rigId}/snapshots/${selected.snapshotId}`);
  } else snapshot = await request(fetcher, manager.port, `/api/rigs/${input.rigId}/snapshots`, 'POST');
  if (typeof snapshot.id !== 'string' || !ulid.test(snapshot.id) || snapshot.rigId !== input.rigId || !object(snapshot.data)
    || !Array.isArray(snapshot.data.nodes) || !snapshot.data.nodes.every(object) || !Array.isArray(snapshot.data.sessions)
    || !snapshot.data.sessions.every(object) || !object(snapshot.data.activeSessionIdByNode)) return refuse();
  const nodes = snapshot.data.nodes.filter(node => node.id === input.nodeId), sessions = snapshot.data.sessions.filter(session => session.id === ready.sessionId && session.nodeId === input.nodeId);
  if (nodes.length !== 1 || nodes[0].rigId !== input.rigId || nodes[0].logicalId !== `${input.pod}.${input.member}`
    || nodes[0].runtime !== 'codex' || nodes[0].cwd !== cwd || JSON.stringify(nodes[0].launchWrapper) !== JSON.stringify(initial.member.launch_wrapper)
    || snapshot.data.activeSessionIdByNode[input.nodeId] !== ready.sessionId || sessions.length !== 1 || sessions[0].sessionName !== ready.target
    || sessions[0].origin !== 'launched' || sessions[0].resumeType !== 'codex_id' || sessions[0].resumeToken !== before.id) return refuse();
  const after = nativeRecord(await request(fetcher, manager.port, recordRoute), plan.home, cwd), finalManager = await review();
  if (after.generationId !== before.generationId || finalManager.managerIdentity !== manager.managerIdentity
    || finalManager.processIdentity !== manager.processIdentity || finalManager.port !== manager.port) return refuse();
  const result: NativeCodexRecoveryReference = { contract: 'webmux-rig-native-codex-recovery/v1', rigId: input.rigId, nodeId: input.nodeId,
    pod: input.pod, member: input.member, cwd, sessionName: ready.target, snapshotId: snapshot.id, historySessionId: ready.sessionId,
    nativeSessionId: before.id, nativeStateFile: before.file, nativeRecordIdentity: before.identity,
    snapshotSHA256: hash(JSON.stringify(snapshot)), planFingerprint: plan.fingerprint };
  artifacts.publish(cached, Buffer.from(JSON.stringify(result))); return result;
}

// Recheck banked historical evidence without requiring a surviving provider.
// This does not select a new generation, create a snapshot or observe an account.
export async function verifyRigNativeCodexReference(input: NativeCodexCaptureInput, reference: NativeCodexRecoveryReference, deps: Dependencies) {
  if (!object(reference) || reference.contract !== 'webmux-rig-native-codex-recovery/v1'
    || reference.rigId !== input.rigId || reference.nodeId !== input.nodeId || reference.pod !== input.pod || reference.member !== input.member
    || reference.cwd !== input.cwd || reference.cwd !== input.identity.workspace || reference.sessionName !== input.identity.target
    || ![reference.rigId, reference.nodeId, reference.snapshotId, reference.historySessionId].every(id => typeof id === 'string' && ulid.test(id))
    || typeof reference.nativeSessionId !== 'string' || !uuid.test(reference.nativeSessionId)
    || ![reference.nativeRecordIdentity, reference.snapshotSHA256, reference.planFingerprint].every(value => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value))) return refuse();
  const base = artifacts.ownedRoot(input.directory), key = hash(JSON.stringify([input.rigId, input.pod, input.member]));
  const directory = artifacts.ownedRoot(path.join(artifacts.ownedRoot(path.join(base, 'requests')), key));
  const registration: unknown = JSON.parse(artifacts.read(path.join(directory, 'native-registration.json')).toString('utf8'));
  const launch: unknown = JSON.parse(artifacts.read(path.join(directory, 'launch.json')).toString('utf8'));
  if (!object(registration) || registration.nodeId !== reference.nodeId || registration.rigId !== reference.rigId
    || registration.member !== reference.member || registration.pod !== reference.pod || registration.cwd !== reference.cwd
    || !object(launch) || !object(launch.intent)) return refuse();
  const plan = validateCodexLaunchPlan(launch.plan);
  if (plan.fingerprint !== reference.planFingerprint || plan.cwd !== reference.cwd) return refuse();
  // The tuple here reopens the banked file; it makes no claim about the live
  // conversation. Its immutable identity must still equal the captured hash.
  const stat = fs.statSync(reference.nativeStateFile);
  const record = nativeRecord({ runtime: 'codex', sessionId: reference.nativeSessionId,
    generationId: JSON.stringify([reference.nativeSessionId, reference.nativeSessionId, reference.nativeStateFile, stat.dev, stat.ino, stat.birthtimeMs]) }, plan.home, reference.cwd);
  if (record.identity !== reference.nativeRecordIdentity) return refuse();
  const fetcher = deps.fetcher ?? fetch, review = deps.review ?? (() => reviewRigCreateManager(deps.run, fetcher)), manager = await review();
  if (manager.managerIdentity !== launch.intent.managerIdentity || manager.port !== launch.intent.port) return refuse();
  const snapshot = await request(fetcher, manager.port, `/api/rigs/${reference.rigId}/snapshots/${reference.snapshotId}`);
  if (hash(JSON.stringify(snapshot)) !== reference.snapshotSHA256) return refuse();
}

// A surviving original conversation can be reattached without any Resume POST.
export async function observeRigNativeOriginalAttachment(input: NativeCodexCaptureInput, reference: NativeCodexRecoveryReference, deps: Dependencies) {
  await verifyRigNativeCodexReference(input, reference, deps);
  const ready = await (deps.launch ?? launchRigNativeCodex)(input, { run: deps.run, fetcher: deps.fetcher,
    create: async () => { throw new Error('native_recovery_cannot_create'); } });
  if (ready.state !== 'attached') return undefined;
  if (ready.nodeId !== reference.nodeId || ready.target !== reference.sessionName || ready.tmuxServerFlag !== input.identity.tmux_server_flag
    || ready.tmuxServerValue !== input.identity.tmux_server_value) return refuse();
  const key = hash(JSON.stringify([input.rigId, input.pod, input.member]));
  const launch = JSON.parse(artifacts.read(path.join(input.directory, 'requests', key, 'launch.json')).toString('utf8')) as { plan: unknown };
  const plan = validateCodexLaunchPlan(launch.plan), fetcher = deps.fetcher ?? fetch;
  const review = deps.review ?? (() => reviewRigCreateManager(deps.run, fetcher)), manager = await review();
  const route = `/api/sessions/${encodeURIComponent(ready.target)}/generation-record`;
  const before = nativeRecord(await request(fetcher, manager.port, route), plan.home, input.cwd);
  if (before.id !== reference.nativeSessionId || before.identity !== reference.nativeRecordIdentity) return refuse();
  const after = nativeRecord(await request(fetcher, manager.port, route), plan.home, input.cwd), finalManager = await review();
  if (after.generationId !== before.generationId || finalManager.managerIdentity !== manager.managerIdentity
    || finalManager.processIdentity !== manager.processIdentity || finalManager.port !== manager.port) return refuse();
  return ready;
}
