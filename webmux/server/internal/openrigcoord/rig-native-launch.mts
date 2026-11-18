// Public launch preparation: native enrollment is not proof of a live CLI.
// This observes only the original member; it never replaces a lost provider.
import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { createRigNativeCodex, nativeCreateArtifacts as artifacts, validateNativeTrustRequest, type NativeCodexCreateInput } from './rig-native-create.mts';
import { validateCodexLaunchPlan } from './codex-launch-policy.mts';
import { validateCodexLaunchEnvironment } from './codex-launch-environment.mts';
import { rigTmuxAttachment, type RigTmuxAttachment } from './rig-attachment.mts';
import { reviewRigCreateManager, type RigCreateManager, type RigStopRun } from './rig-stop.mts';
import type { CodexObservationHelper } from './codex-owner-plan.mts';

const ulid = /^[0-7][0-9A-HJKMNP-TV-Z]{25}$/;
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const hash = (value: string) => createHash('sha256').update(value).digest('hex');
const refuse = (): never => { throw new Error('native_launch_original_unavailable'); };
type Run = (command: string, args: string[], timeout?: number) => string;
interface Dependencies {
  run: Run; fetcher?: typeof fetch; observationHelper?: CodexObservationHelper;
  create?: typeof createRigNativeCodex; review?: () => Promise<RigCreateManager>;
  observe?: (attachment: RigTmuxAttachment, executable: string, digest: string) => Promise<boolean>;
}

export async function observeNativeCodexPane(run: Run, attachment: RigTmuxAttachment, executable: string, expectedDigest: string) {
  try {
    const flags = [attachment.tmuxServerFlag, attachment.tmuxServerValue];
    const pane = () => Number(run('tmux', [...flags, 'display-message', '-p', '-t', '=' + attachment.tmuxTarget + ':', '#{pane_pid}'], 3000).trim());
    const root = pane(); if (!Number.isSafeInteger(root) || root < 2) return false;
    const table = run('ps', ['-axo', 'pid=,ppid=,uid='], 3000).trim().split('\n');
    if (table.length > 8192) return false;
    // macOS can report the unrelated nobody account as UID -2.
    const processes = table.map(line => /^\s*(\d+)\s+(\d+)\s+(-?\d+)\s*$/.exec(line));
    if (processes.some(row => !row)) return false;
    const descendants = new Set([root]);
    for (let depth = 0; depth < 128; depth++) {
      const before = descendants.size;
      for (const row of processes) if (descendants.has(Number(row![2]))) descendants.add(Number(row![1]));
      if (before === descendants.size) break;
    }
    const candidates: number[] = [];
    for (const row of processes) {
      const pid = Number(row![1]);
      if (!descendants.has(pid) || Number(row![3]) !== process.geteuid?.()) continue;
      let target: string;
      try { target = process.platform === 'linux' ? fs.realpathSync(`/proc/${pid}/exe`) : run('ps', ['-ww', '-p', String(pid), '-o', 'comm='], 3000).trim(); }
      catch { continue; }
      // comm may contain only a basename. The selected macOS candidate must
      // still expose the exact executable as a loaded text image below.
      if (target === executable || (process.platform === 'darwin' && target === path.basename(executable))) candidates.push(pid);
    }
    // Nested tool CLIs do not replace the native foreground root.
    const top = candidates.filter(pid => {
      let parent = Number(processes.find(row => Number(row![1]) === pid)![2]);
      for (let depth = 0; depth < 128 && descendants.has(parent); depth++) {
        if (candidates.includes(parent)) return false;
        parent = Number(processes.find(row => Number(row![1]) === parent)?.[2]);
      }
      return true;
    });
    if (top.length !== 1) return false;
    const pid = top[0], start = () => run('env', ['LC_ALL=C', 'TZ=UTC', 'ps', '-p', String(pid), '-o', 'lstart='], 3000).trim();
    const before = start(); if (!before) return false;
    if (process.platform === 'darwin' && !run('lsof', ['-a', '-p', String(pid), '-d', 'txt', '-F', 'n'], 3000).split('\n').includes('n' + executable)) return false;
    const file = process.platform === 'linux' ? `/proc/${pid}/exe` : executable;
    if (!fs.statSync(file).isFile() || fs.statSync(file).size > 512 * 1024 * 1024) return false;
    const digest = createHash('sha256');
    for await (const bytes of fs.createReadStream(file, { signal: AbortSignal.timeout(5000) })) digest.update(bytes);
    return digest.digest('hex') === expectedDigest && before === start() && root === pane();
  } catch { return false; }
}

async function rows(fetcher: typeof fetch, port: number, route: string) {
  const response = await fetcher(`http://127.0.0.1:${port}${route}`, { redirect: 'error', signal: AbortSignal.timeout(5000) });
  if (response.status !== 200 || !response.body) return refuse();
  const reader = response.body.getReader(), chunks: Uint8Array[] = []; let size = 0;
  try {
    for (;;) {
      const part = await reader.read(); if (part.done) break;
      size += part.value.length; if (size > 2 * 1024 * 1024) { await reader.cancel(); return refuse(); }
      chunks.push(part.value);
    }
  } finally { reader.releaseLock(); }
  const value: unknown = JSON.parse(Buffer.concat(chunks).toString('utf8'));
  if (!Array.isArray(value) || value.length > 4096 || !value.every(object)) return refuse();
  return value;
}

export async function launchRigNativeCodex(input: NativeCodexCreateInput, deps: Dependencies) {
  if (!['darwin', 'linux'].includes(process.platform) || !ulid.test(input.rigId)
    || !/^[a-z][a-z0-9-]{0,47}$/.test(input.pod) || !/^[a-z][a-z0-9-]{0,63}$/.test(input.member)) return refuse();
  const base = artifacts.ownedRoot(input.directory), cwd = fs.realpathSync(input.cwd);
  const key = hash(JSON.stringify([input.rigId, input.pod, input.member]));
  const directory = artifacts.ownedRoot(path.join(artifacts.ownedRoot(path.join(base, 'requests')), key));
  if (input.trustRequest) validateNativeTrustRequest(input, directory);
  const registration = path.join(directory, 'native-registration.json');
  let saved: { contract: string; rigId: string; pod: string; member: string; cwd: string; nodeId: string; operationId: string } | undefined;
  try { saved = JSON.parse(artifacts.read(registration).toString('utf8')) as typeof saved; }
  catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
  if (saved !== undefined && !object(saved)) return refuse();
  if (!saved) {
    const result = await (deps.create ?? createRigNativeCodex)({ ...input, directory: base }, { run: deps.run, fetcher: deps.fetcher, observationHelper: deps.observationHelper });
    if (result.state === 'blocked') return result;
    if (result.state !== 'recorded') return { state: 'pending' as const, code: result.code, operationId: result.operationId };
    if (!result.nodeIds || result.nodeIds.length !== 1 || !ulid.test(result.nodeIds[0]) || !uuid.test(result.operationId)) return refuse();
    saved = { contract: 'webmux-native-codex-registration/v1', rigId: input.rigId, pod: input.pod, member: input.member,
      cwd, nodeId: result.nodeIds[0], operationId: result.operationId };
    artifacts.publish(registration, Buffer.from(JSON.stringify(saved)));
  }
  if (!object(saved) || saved.contract !== 'webmux-native-codex-registration/v1' || saved.rigId !== input.rigId
    || saved.pod !== input.pod || saved.member !== input.member || saved.cwd !== cwd || !ulid.test(saved.nodeId) || !uuid.test(saved.operationId)) return refuse();
  const pending = () => ({ state: 'pending' as const, code: 'native_provider_not_verified', operationId: saved!.operationId });
  const launch = JSON.parse(artifacts.read(path.join(directory, 'launch.json')).toString('utf8')) as unknown;
  if (!object(launch) || launch.contract !== 'webmux-native-codex-create/v2' || launch.cwd !== cwd || launch.member !== input.member
    || !object(launch.intent) || launch.intent.rigId !== input.rigId || launch.intent.pod !== input.pod || typeof launch.intent.requestText !== 'string') return refuse();
  const original = JSON.parse(artifacts.read(path.join(directory, 'journal', key + '.intent.json')).toString('utf8')) as unknown;
  if (!object(original) || original.version !== 1 || original.operationId !== saved.operationId
    || JSON.stringify(original.intent) !== JSON.stringify(launch.intent)) return refuse();
  const plan = validateCodexLaunchPlan(launch.plan), environment = validateCodexLaunchEnvironment(launch.environment, plan.home);
  const member = (JSON.parse(launch.intent.requestText) as { member: { id: string; runtime: string; launch_wrapper: unknown } }).member;
  if (plan.cwd !== cwd || !member || member.id !== input.member || member.runtime !== 'codex' || !Array.isArray(member.launch_wrapper)
    || member.launch_wrapper[2] !== path.join(directory, 'launch-manifest.json')) return refuse();
  try {
    if (!artifacts.read(path.join(directory, 'environment-consumed.json')).equals(Buffer.from(JSON.stringify({ contract: 'webmux-codex-environment-consumed/v1', fingerprint: environment.fingerprint })))) return refuse();
  } catch (error) { if ((error as NodeJS.ErrnoException).code === 'ENOENT') return pending(); throw error; }
  const fetcher = deps.fetcher ?? fetch, review = deps.review ?? (() => reviewRigCreateManager(deps.run as RigStopRun, fetcher));
  const manager = await review();
  if (manager.managerIdentity !== launch.intent.managerIdentity || manager.port !== launch.intent.port) return refuse();
  const read = await Promise.allSettled([rows(fetcher, manager.port, `/api/rigs/${input.rigId}/nodes`), rows(fetcher, manager.port, `/api/rigs/${input.rigId}/sessions`)]);
  if (read[0].status !== 'fulfilled' || read[1].status !== 'fulfilled') return pending();
  const nodes = read[0].value.filter(node => node.nodeId === saved!.nodeId);
  const sessions = read[1].value.filter(session => session.nodeId === saved!.nodeId);
  if (nodes.length !== 1 || nodes[0].rigId !== input.rigId || nodes[0].logicalId !== `${input.pod}.${input.member}`
    || nodes[0].runtime !== 'codex' || nodes[0].cwd !== cwd || JSON.stringify(nodes[0].launchWrapper) !== JSON.stringify(member.launch_wrapper)
    || !sessions.length || sessions.some(session => typeof session.id !== 'string' || !ulid.test(session.id))) return pending();
  const current = sessions.reduce((a, b) => String(a.id) > String(b.id) ? a : b);
  const node = nodes[0], target = node.canonicalSessionName;
  if (sessions.filter(session => session.id === current.id).length !== 1 || current.status !== 'running' || current.origin !== 'launched'
    || current.sessionName !== target || node.sessionStatus !== 'running' || typeof target !== 'string') return pending();
  const attachment = rigTmuxAttachment({ canonicalSessionName: target,
    attachmentType: typeof node.attachmentType === 'string' ? node.attachmentType : undefined, tmuxServerArgs: node.tmuxServerArgs }, true);
  if (!attachment || !await (deps.observe ?? ((value, executable, digest) => observeNativeCodexPane(deps.run, value, executable, digest)))(attachment, plan.executable, plan.executableSHA256)) return pending();
  const after = await review();
  if (after.managerIdentity !== manager.managerIdentity || after.processIdentity !== manager.processIdentity || after.port !== manager.port) return pending();
  return { state: 'attached' as const, operationId: saved.operationId, nodeId: saved.nodeId, sessionId: String(current.id), target,
    tmuxServerFlag: attachment.tmuxServerFlag, tmuxServerValue: attachment.tmuxServerValue };
}
