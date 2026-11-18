// Only the published checked HTTP stop contract may end a seat. Human CLI
// commands and WebMux launch records never authorize native lifecycle effects.
import { rigTmuxAttachment } from './rig-attachment.mts';
import { createHash } from 'node:crypto';

type Row = Record<string, unknown>;
export type RigStopRun = (command: string, args: string[]) => string;
export interface RigStopSelection {
  rigId: string; logicalId: string; sessionName: string;
  tmuxServerFlag: string; tmuxServerValue: string;
}
export interface RigStopReview extends RigStopSelection {
  contract: 'webmux-rig-stop/v1'; port: number; runtime: string;
  managerPid: number; managerIdentity: string;
  expected: { nodeId: string; sessionId: string };
}
export interface RigStopResult {
  state: 'stopped' | 'refused' | 'uncertain'; code: string; posted: boolean;
  stopPhase: 'none' | 'detach_attempted' | 'detached' | 'kill_attempted' | 'unknown';
}
const ulid = /^[0-7][0-9A-HJKMNP-TV-Z]{25}$/;
const token = /^[a-zA-Z0-9_.:@-]{1,256}$/;
const object = (value: unknown): value is Row => !!value && typeof value === 'object' && !Array.isArray(value);
const rows = (value: unknown): Row[] => {
  if (!Array.isArray(value) || value.length > 4096 || !value.every(object)) throw new Error('Rig stop inventory is unavailable.');
  return value;
};
class Refusal extends Error {
  readonly code: string;
  constructor(code: string) { super(code); this.code = code; }
}
function selection(value: RigStopSelection) {
  if (!ulid.test(value.rigId) || !token.test(value.logicalId) || !token.test(value.sessionName)
    || value.tmuxServerFlag !== '-S' || typeof value.tmuxServerValue !== 'string') throw new Refusal('invalid_selection');
  const attachment = rigTmuxAttachment({ canonicalSessionName: value.sessionName, attachmentType: 'tmux', tmuxServerArgs: [value.tmuxServerFlag, value.tmuxServerValue] }, true);
  if (!attachment) throw new Refusal('invalid_selection');
}
function connection(run: RigStopRun) {
  if (run('rig', ['--version']).trim() !== '0.7.18 (3bf87c62)') throw new Refusal('unsupported_version');
  const status = run('rig', ['daemon', 'status']);
  const matches = [...status.matchAll(/^Daemon running on port (\d{1,5})(?: \(pid \d+\))?\r?$/gm)];
  const port = Number(matches[0]?.[1]);
  if (matches.length !== 1 || !Number.isSafeInteger(port) || port < 1024 || port > 65535) throw new Refusal('manager_unavailable');
  return port;
}
async function request(fetcher: typeof fetch, port: number, route: string, body?: unknown) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 8000);
  try {
    const response = await fetcher(`http://127.0.0.1:${port}${route}`, {
      method: body === undefined ? 'GET' : 'POST', redirect: 'error', signal: controller.signal,
      ...(body === undefined ? {} : { headers: { 'content-type': 'application/json' }, body: JSON.stringify(body) }),
    });
    if (!response.body || Number(response.headers.get('content-length') || 0) > 512 * 1024) {
      await response.body?.cancel(); throw new Error('Rig stop response is unavailable.');
    }
    const reader = response.body.getReader();
    const chunks: Uint8Array[] = [];
    let length = 0;
    try {
      for (;;) {
        const part = await reader.read();
        if (part.done) break;
        length += part.value.length;
        if (length > 512 * 1024) { await reader.cancel(); throw new Error('Rig stop response exceeds its limit.'); }
        chunks.push(part.value);
      }
    } finally { reader.releaseLock(); }
    return { status: response.status, body: JSON.parse(Buffer.concat(chunks).toString('utf8')) as unknown };
  } finally { clearTimeout(timer); }
}

async function localManager(run: RigStopRun, port: number, fetcher: typeof fetch, uid: number | undefined) {
  const response = await request(fetcher, port, '/healthz');
  const value = response.body;
  if (uid === undefined || response.status !== 200 || !object(value) || !Number.isSafeInteger(value.pid) || Number(value.pid) < 1 || value.euid !== uid) throw new Refusal('local_manager_unverified');
  if (value.semver !== '0.7.18' || value.commit !== '3bf87c62a24fcb0a441334d7f8b0b1f2687ecf64' || value.dirty !== false) throw new Refusal('unsupported_manager_build');
  // A loopback SSH tunnel is not a local daemon. Corroborate the reported PID
  // against the actual listener and its OS owner/start/executable identity.
  let listeners: string;
  let observedProcess: string;
  let daemonCommand: string;
  try {
    listeners = run(process.platform === 'darwin' ? '/usr/sbin/lsof' : 'lsof', ['-nP', '-a', `-iTCP:${port}`, '-sTCP:LISTEN', '-Fp']);
    observedProcess = run('/bin/ps', ['-p', String(value.pid), '-o', 'uid=,lstart=,comm=']).trim();
    daemonCommand = run('/bin/ps', ['-p', String(value.pid), '-o', 'command=']).trim();
  } catch { throw new Refusal('local_manager_unverified'); }
  const pids = [...new Set(listeners.split(/\r?\n/).filter(line => /^p\d+$/.test(line)).map(line => Number(line.slice(1))))];
  if (pids.length !== 1 || pids[0] !== value.pid) throw new Refusal('local_manager_unverified');
  if (observedProcess.length > 2048 || Number(/^(\d+)\s+/.exec(observedProcess)?.[1]) !== uid) throw new Refusal('local_manager_unverified');
  if (!/(?:^|[\/\s])node(?:js)?$/.test(observedProcess) || daemonCommand.length > 4096
    || !/\/daemon\/dist\/index\.js(?:\s|$)/.test(daemonCommand)) throw new Refusal('local_manager_unverified');
  // Only the fingerprint is retained; process arguments never leave this probe.
  return { managerPid: Number(value.pid), managerIdentity: createHash('sha256').update(observedProcess + '\n' + daemonCommand).digest('hex') };
}

export async function reviewRigStop(run: RigStopRun, input: RigStopSelection, fetcher: typeof fetch = fetch, uid = process.geteuid?.()): Promise<RigStopReview> {
  selection(input);
  const port = connection(run);
  const manager = await localManager(run, port, fetcher, uid);
  const raw = JSON.parse(run('rig', ['ps', '--nodes', '-A', '--no-cleanup', '--json', '--full'])) as unknown;
  const inventory = rows(Array.isArray(raw) ? raw : object(raw) ? raw.entries : undefined);
  const candidates = inventory.filter(row => row.rigId === input.rigId && row.logicalId === input.logicalId);
  if (candidates.length !== 1 || inventory.filter(row => row.canonicalSessionName === input.sessionName).length !== 1) throw new Refusal('selection_changed');
  const node = candidates[0];
  const attachment = rigTmuxAttachment({
    canonicalSessionName: typeof node.canonicalSessionName === 'string' ? node.canonicalSessionName : undefined,
    attachmentType: typeof node.attachmentType === 'string' ? node.attachmentType : undefined,
    tmuxServerArgs: node.tmuxServerArgs,
  }, true);
  if (node.canonicalSessionName !== input.sessionName || attachment?.tmuxTarget !== input.sessionName
    || attachment.tmuxServerFlag !== input.tmuxServerFlag || attachment.tmuxServerValue !== input.tmuxServerValue) throw new Refusal('selection_changed');
  const reference = encodeURIComponent(input.sessionName);
  const precondition = await request(fetcher, port, `/api/seat/stop-precondition/${reference}`);
  if (precondition.status !== 200 || !object(precondition.body) || precondition.body.contract !== 'seat-stop.v1') throw new Refusal('checked_stop_unavailable');
  const expected = precondition.body.expected;
  if (!object(expected) || Object.keys(expected).sort().join(',') !== 'nodeId,sessionId'
    || typeof expected.nodeId !== 'string' || !ulid.test(expected.nodeId)
    || typeof expected.sessionId !== 'string' || !ulid.test(expected.sessionId)) throw new Refusal('no_verified_session');
  const [nodeReply, sessionReply] = await Promise.all([
    request(fetcher, port, `/api/rigs/${input.rigId}/nodes`),
    request(fetcher, port, `/api/rigs/${input.rigId}/sessions`),
  ]);
  if (nodeReply.status !== 200 || sessionReply.status !== 200) throw new Refusal('manager_unavailable');
  const nodes = rows(nodeReply.body).filter(row => row.rigId === input.rigId && row.logicalId === input.logicalId);
  const sessions = rows(sessionReply.body).filter(row => row.nodeId === expected.nodeId);
  if (nodes.length !== 1 || nodes[0].nodeId !== expected.nodeId || nodes[0].canonicalSessionName !== input.sessionName
    || !sessions.length || sessions.some(row => typeof row.id !== 'string' || !ulid.test(row.id))) throw new Refusal('selection_changed');
  const latest = sessions.reduce((left, right) => String(left.id) > String(right.id) ? left : right);
  if (sessions.filter(row => row.id === latest.id).length !== 1 || latest.id !== expected.sessionId || latest.sessionName !== input.sessionName) throw new Refusal('selection_changed');
  if (latest.origin !== 'launched') throw new Refusal('claimed_session');
  if (latest.status !== 'running') throw new Refusal('session_not_live');
  const after = await localManager(run, port, fetcher, uid);
  if (after.managerPid !== manager.managerPid || after.managerIdentity !== manager.managerIdentity) throw new Refusal('local_manager_unverified');
  return { contract: 'webmux-rig-stop/v1', ...input, ...manager, port, runtime: typeof node.runtime === 'string' ? node.runtime.slice(0, 128) : 'unknown',
    expected: { nodeId: expected.nodeId, sessionId: expected.sessionId } };
}

export async function executeRigStop(run: RigStopRun, review: RigStopReview, reason: string, operator: string, fetcher: typeof fetch = fetch, uid = process.geteuid?.()): Promise<RigStopResult> {
  const refusal = (code: string): RigStopResult => ({ state: 'refused', code, posted: false, stopPhase: 'none' });
  try {
    if (review.contract !== 'webmux-rig-stop/v1' || !review.expected || !ulid.test(review.expected.nodeId) || !ulid.test(review.expected.sessionId)
      || !reason.trim() || reason.length > 1024 || !operator || operator.length > 256) return refusal('invalid_review');
    const current = await reviewRigStop(run, review, fetcher, uid);
    if (current.port !== review.port || current.managerPid !== review.managerPid || current.managerIdentity !== review.managerIdentity || current.expected.nodeId !== review.expected.nodeId || current.expected.sessionId !== review.expected.sessionId) return refusal('selection_changed');
  } catch (error) { return refusal(error instanceof Refusal ? error.code : 'preflight_unavailable'); }
  // The caller must persist a consumed/uncertain receipt BEFORE entering this
  // function. No retry or unchecked endpoint fallback exists after dispatch.
  try {
    const response = await request(fetcher, review.port, `/api/seat/stop/${encodeURIComponent(review.sessionName)}`, { expected: review.expected, reason, operator });
    const value = response.body;
    if (response.status === 200 && object(value) && value.ok === true && value.sessionId === review.expected.sessionId
      && value.sessionName === review.sessionName && object(value.seat) && value.seat.nodeId === review.expected.nodeId
      && value.seat.rigId === review.rigId && value.seat.logicalId === review.logicalId) {
      return { state: 'stopped', code: 'stopped', posted: true, stopPhase: 'kill_attempted' };
    }
    const phase = object(value) && ['none', 'detach_attempted', 'detached', 'kill_attempted'].includes(String(value.stopPhase))
      ? value.stopPhase as RigStopResult['stopPhase'] : 'unknown';
    const code = object(value) && typeof value.code === 'string' && /^[a-z_]{1,64}$/.test(value.code) ? value.code : 'unverified_response';
    const noEffect = object(value) && value.ok === false && value.partialEffect !== true
      && (value.stopPhase === undefined || phase === 'none')
      && response.status === (code === 'tmux_probe_failed' ? 502 : 409)
      && ['selection_changed', 'claimed_session', 'typing_guard_enabled', 'guard_unavailable', 'no_session', 'session_not_live', 'tmux_probe_failed'].includes(code);
    return { state: noEffect ? 'refused' : 'uncertain', code, posted: true, stopPhase: noEffect ? 'none' : phase };
  } catch { return { state: 'uncertain', code: 'response_unavailable', posted: true, stopPhase: 'unknown' }; }
}
