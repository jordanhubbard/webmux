// Bank one reviewed original-only recovery request before its first dispatch.
// Only explicit inspection may complete that same native operation after loss.
import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { nativeCreateArtifacts as artifacts, openRigNativeCodexRecoveryHandoff, type NativeCodexCreateInput } from './rig-native-create.mts';
import { reviewRigCreateManager, type RigCreateManager, type RigStopRun } from './rig-stop.mts';
import type { CodexObservationHelper } from './codex-owner-plan.mts';

export interface NativeCodexResumeInput extends Omit<NativeCodexCreateInput, 'inspect'> {
  operationId: string; nodeId: string; snapshotId: string; historySessionId: string;
  nativeSessionId: string; sessionName: string; inspect?: boolean;
}
export interface NativeCodexResumeResult {
  state: 'confirmed' | 'uncertain'; code: string; posted: boolean;
  operationId: string; logicalId: string; nativeSessionId: string;
  previouslyConfirmed?: boolean;
  nativeCode?: string;
}
interface Dependencies {
  run: RigStopRun; fetcher?: typeof fetch; observationHelper?: CodexObservationHelper;
  review?: () => Promise<RigCreateManager>;
  handoff?: typeof openRigNativeCodexRecoveryHandoff;
}
const ulid = /^[0-7][0-9A-HJKMNP-TV-Z]{25}$/;
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const operation = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const digest = (value: string | Buffer) => createHash('sha256').update(value).digest('hex');
const json = (value: unknown) => Buffer.from(JSON.stringify(value));
const refuse = (): never => { throw new Error('native_resume_original_unavailable'); };
function consume(file: string, bytes: Buffer, inspect: boolean) {
  let fd: number;
  try { fd = fs.openSync(file, fs.constants.O_WRONLY | fs.constants.O_CREAT | fs.constants.O_EXCL, 0o600); }
  catch (error) {
    if ((error as NodeJS.ErrnoException).code !== 'EEXIST') throw error;
    if (!artifacts.read(file).equals(bytes)) return refuse();
    return inspect;
  }
  try { fs.writeFileSync(fd, bytes); fs.fsyncSync(fd); } finally { fs.closeSync(fd); }
  const parent = fs.openSync(path.dirname(file), fs.constants.O_RDONLY);
  try { fs.fsyncSync(parent); } finally { fs.closeSync(parent); }
  return true;
}
const rows = (value: unknown) => {
  if (!Array.isArray(value) || value.length > 4096 || !value.every(object)) return refuse();
  return value;
};
async function request(fetcher: typeof fetch, port: number, route: string, body?: string) {
  const response = await fetcher(`http://127.0.0.1:${port}${route}`, {
    method: body === undefined ? 'GET' : 'POST', redirect: 'error', signal: AbortSignal.timeout(10000),
    ...(body === undefined ? {} : { headers: { 'content-type': 'application/json' }, body }),
  });
  if (!response.body) return refuse();
  const reader = response.body.getReader(), chunks: Uint8Array[] = []; let size = 0;
  try {
    for (;;) {
      const part = await reader.read(); if (part.done) break;
      size += part.value.length;
      if (size > 2 * 1024 * 1024) { await reader.cancel(); return refuse(); }
      chunks.push(part.value);
    }
  } finally { reader.releaseLock(); }
  return { status: response.status, value: JSON.parse(Buffer.concat(chunks).toString('utf8')) as unknown };
}

export async function resumeRigNativeCodex(input: NativeCodexResumeInput, deps: Dependencies): Promise<NativeCodexResumeResult> {
  if (!['darwin', 'linux'].includes(process.platform) || Number(process.versions.node.split('.')[0]) !== 24
    || ![input.rigId, input.nodeId, input.snapshotId, input.historySessionId].every(id => ulid.test(id))
    || !operation.test(input.operationId) || !uuid.test(input.nativeSessionId)
    || !/^[a-z][a-z0-9-]{0,47}$/.test(input.pod) || !/^[a-z][a-z0-9-]{0,63}$/.test(input.member)
    || !/^[a-zA-Z0-9_.:@-]{1,256}$/.test(input.sessionName) || !path.isAbsolute(input.cwd)) return refuse();
  const cwd = fs.realpathSync(input.cwd), base = artifacts.ownedRoot(input.directory);
  const key = digest(JSON.stringify([input.rigId, input.pod, input.member]));
  const original = artifacts.ownedRoot(path.join(artifacts.ownedRoot(path.join(base, 'requests')), key));
  const directory = artifacts.ownedRoot(path.join(original, 'recovery-operations'));
  const file = path.join(directory, input.operationId + '.json'), dispatched = path.join(directory, input.operationId + '.dispatched');
  const confirmed = path.join(directory, input.operationId + '.confirmed.json');
  const selection = { rigId: input.rigId, nodeId: input.nodeId, pod: input.pod, member: input.member, cwd,
    snapshotId: input.snapshotId, historySessionId: input.historySessionId, nativeSessionId: input.nativeSessionId,
    sessionName: input.sessionName, operationId: input.operationId };
  const route = `/api/startup/${input.rigId}/nodes/${input.nodeId}/original`;
  let saved: { contract: string; selection: typeof selection; port: number; managerIdentity: string; requestText: string } | undefined;
  try { saved = JSON.parse(artifacts.read(file).toString('utf8')) as typeof saved; }
  catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
  if (saved !== undefined && (!object(saved) || saved.contract !== 'webmux-native-codex-resume/v1'
    || JSON.stringify(saved.selection) !== JSON.stringify(selection) || typeof saved.requestText !== 'string')) return refuse();
  if (!saved && (input.inspect || fs.existsSync(dispatched))) return refuse();
  const summary = { operationId: input.operationId, logicalId: `${input.pod}.${input.member}`, nativeSessionId: input.nativeSessionId };
  // A normal duplicate cannot re-observe an account or accidentally turn a
  // previous uncertainty into new launch authority, even after process loss.
  if (saved && fs.existsSync(dispatched) && !input.inspect) {
    if (!artifacts.read(dispatched).equals(json({ operationId: input.operationId, requestSHA256: digest(saved.requestText) }))) return refuse();
    let previouslyConfirmed = false;
    try {
      const proof = JSON.parse(artifacts.read(confirmed).toString('utf8')) as unknown;
      if (!object(proof) || proof.contract !== 'webmux-native-codex-resume-confirmation/v1' || proof.operationId !== input.operationId
        || proof.requestSHA256 !== digest(saved.requestText) || !['resumed', 'already_running_original'].includes(String(proof.code))) return refuse();
      previouslyConfirmed = true;
    } catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
    // A historical confirmation cannot establish that the provider is live now.
    return { ...summary, state: 'uncertain', code: 'native_resume_already_dispatched', posted: false, previouslyConfirmed };
  }
  const fetcher = deps.fetcher ?? fetch;
  const reviewManager = deps.review ?? (() => reviewRigCreateManager(deps.run, fetcher));
  const manager = await reviewManager();
  if (saved && (saved.port !== manager.port || saved.managerIdentity !== manager.managerIdentity)) return refuse();
  if (!saved) {
    const created = JSON.parse(artifacts.read(path.join(original, 'launch.json')).toString('utf8')) as unknown;
    if (!object(created) || !object(created.intent) || created.cwd !== cwd || created.member !== input.member
      || created.intent.rigId !== input.rigId || created.intent.pod !== input.pod || created.intent.managerIdentity !== manager.managerIdentity
      || created.intent.port !== manager.port || typeof created.intent.requestText !== 'string') return refuse();
    const initial = JSON.parse(created.intent.requestText) as unknown;
    if (!object(initial) || !object(initial.member) || initial.member.runtime !== 'codex') return refuse();
    const snapshot = await request(fetcher, manager.port, `/api/rigs/${input.rigId}/snapshots/${input.snapshotId}`);
    if (snapshot.status !== 200 || !object(snapshot.value) || snapshot.value.id !== input.snapshotId
      || snapshot.value.rigId !== input.rigId || !object(snapshot.value.data)) return refuse();
    const nodes = rows(snapshot.value.data.nodes).filter(node => node.id === input.nodeId);
    const sessions = rows(snapshot.value.data.sessions).filter(session => session.id === input.historySessionId);
    if (nodes.length !== 1 || nodes[0].rigId !== input.rigId || nodes[0].logicalId !== summary.logicalId || nodes[0].runtime !== 'codex'
      || nodes[0].cwd !== cwd || JSON.stringify(nodes[0].launchWrapper) !== JSON.stringify(initial.member.launch_wrapper)
      || !Array.isArray(nodes[0].launchWrapper) || nodes[0].launchWrapper[2] !== path.join(original, 'launch-manifest.json')
      || sessions.length !== 1 || sessions[0].nodeId !== input.nodeId || sessions[0].sessionName !== input.sessionName
      || sessions[0].resumeType !== 'codex_id' || sessions[0].resumeToken !== input.nativeSessionId || sessions[0].origin !== 'launched') return refuse();
    const query = new URLSearchParams({ snapshotId: input.snapshotId, historySessionId: input.historySessionId });
    const review = await request(fetcher, manager.port, route + '?' + query);
    const value = review.value;
    if (review.status !== 200 || !object(value) || value.ok !== true || value.contract !== 'startup-history.v1' || value.intent !== 'resume-original'
      || value.rigId !== input.rigId || value.nodeId !== input.nodeId || value.logicalId !== summary.logicalId
      || value.snapshotId !== input.snapshotId || value.historySessionId !== input.historySessionId
      || typeof value.revision !== 'string' || !/^[a-f0-9]{64}$/.test(value.revision)
      || !object(value.observed) || value.observed.state !== 'stopped') return refuse();
    const requestText = JSON.stringify({ action: 'resume', snapshotId: input.snapshotId, historySessionId: input.historySessionId,
      revision: value.revision, operationId: input.operationId,
      expectedOriginal: { nodeId: input.nodeId, sessionName: input.sessionName, runtime: 'codex', nativeId: input.nativeSessionId } });
    saved = { contract: 'webmux-native-codex-resume/v1', selection, port: manager.port, managerIdentity: manager.managerIdentity, requestText };
    artifacts.publish(file, json(saved));
  }
  // Even explicit completion sends the original exact body and UUID. Never
  // refresh a review, change a home or substitute another conversation here.
  const body = JSON.parse(saved.requestText) as Record<string, unknown>;
  if (Object.keys(body).sort().join(',') !== 'action,expectedOriginal,historySessionId,operationId,revision,snapshotId'
    || body.action !== 'resume' || body.snapshotId !== input.snapshotId || body.historySessionId !== input.historySessionId
    || body.operationId !== input.operationId || typeof body.revision !== 'string' || !/^[a-f0-9]{64}$/.test(body.revision)
    || JSON.stringify(body.expectedOriginal) !== JSON.stringify({ nodeId: input.nodeId, sessionName: input.sessionName, runtime: 'codex', nativeId: input.nativeSessionId })) return refuse();
  const handoff = await (deps.handoff ?? openRigNativeCodexRecoveryHandoff)(input, { run: deps.run, fetcher, observationHelper: deps.observationHelper });
  try {
    if (handoff.manager.managerIdentity !== manager.managerIdentity || handoff.manager.port !== manager.port
      || handoff.manager.processIdentity !== manager.processIdentity) return refuse();
    if (!consume(dispatched, json({ operationId: input.operationId, requestSHA256: digest(saved.requestText) }), input.inspect === true)) {
      return { ...summary, state: 'uncertain', code: 'native_resume_already_dispatched', posted: false };
    }
    try {
      const response = await request(fetcher, manager.port, route, saved.requestText), value = response.value;
      const acknowledged = await handoff.wait();
      if (response.status === 200 && object(value) && value.ok === true && value.operationId === input.operationId
        && value.rigId === input.rigId && value.nodeId === input.nodeId
        && ((value.code === 'already_running_original' && value.nativeEffect === false)
          || (value.code === 'resumed' && acknowledged && value.snapshotId === input.snapshotId
            && value.historySessionId === input.historySessionId && value.sourceSessionId === input.historySessionId
            && typeof value.resultSessionId === 'string' && ulid.test(value.resultSessionId)))) {
        try {
          const proof = JSON.parse(artifacts.read(confirmed).toString('utf8')) as unknown;
          if (!object(proof) || proof.contract !== 'webmux-native-codex-resume-confirmation/v1' || proof.operationId !== input.operationId
            || proof.requestSHA256 !== digest(saved.requestText) || !['resumed', 'already_running_original'].includes(String(proof.code))) return refuse();
        } catch (error) {
          if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error;
          artifacts.publish(confirmed, json({ contract: 'webmux-native-codex-resume-confirmation/v1', operationId: input.operationId,
            requestSHA256: digest(saved.requestText), code: value.code }));
        }
        return { ...summary, state: 'confirmed', code: value.code, posted: true };
      }
      return { ...summary, state: 'uncertain', code: 'native_resume_unconfirmed', posted: true,
        ...(object(value) && typeof value.code === 'string' && /^[a-z_]{1,64}$/.test(value.code) ? { nativeCode: value.code } : {}) };
    } catch {
      await handoff.wait();
      return { ...summary, state: 'uncertain', code: 'native_resume_response_unavailable', posted: true };
    }
  } finally { await handoff.close(); }
}
