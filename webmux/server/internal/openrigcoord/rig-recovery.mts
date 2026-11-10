// Native Rig recovery metadata stays scoped to one already-owned viewer. Never
// fetch generation-record with sinceBytes: that would return conversation text.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';

type ObjectValue = Record<string, unknown>;
export interface RigRecoveryIdentity {
  provider?: string; target: string; project: string; name: string;
  host: string; runtime: string; workspace?: string; manager_profile?: string;
  native_session_id?: string; native_state_file?: string; native_source?: string;
  manager_rig_id?: string; manager_node_id?: string; manager_logical_id?: string;
  manager_snapshot_id?: string; manager_snapshot_hash?: string;
  manager_policy_hash?: string; manager_record_identity?: string; manager_process?: string;
}
export type RigRecoveryRun = (command: string, args: string[]) => string;
const ulid = /^[0-9A-HJKMNP-TV-Z]{26}$/;
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i;
const maximum = 4 * 1024 * 1024;
const object = (value: unknown): ObjectValue => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Rig recovery metadata is unavailable.');
  return value as ObjectValue;
};
const rows = (value: unknown): ObjectValue[] => {
  if (!Array.isArray(value)) throw new Error('Rig recovery inventory is unavailable.');
  return value.map(object);
};
const digest = (value: unknown) => crypto.createHash('sha256').update(JSON.stringify(value)).digest('hex');
const text = (value: unknown, limit = 4096): string => {
  if (typeof value !== 'string' || !value || value.length > limit || /[\x00-\x1f\x7f]/.test(value)) throw new Error('Rig recovery identity is invalid.');
  return value;
};
const sameDirectory = (left: string, right: string) => fs.realpathSync(left) === fs.realpathSync(right);

async function manager(run: RigRecoveryRun) {
  if (run('rig', ['--version']).trim() !== '0.6.14 (9d0200f3)') throw new Error('This Rig version is not qualified for native recovery metadata.');
  const match = /running on port (\d+)/.exec(run('rig', ['daemon', 'status']));
  const port = Number(match?.[1]);
  if (!Number.isSafeInteger(port) || port < 1024 || port > 65535) throw new Error('The original Rig daemon is unavailable.');
  return async (route: string, method = 'GET'): Promise<unknown> => {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 5000);
    try {
      const response = await fetch(`http://127.0.0.1:${port}${route}`, { method, signal: controller.signal, redirect: 'error' });
      if (!response.ok || Number(response.headers.get('content-length') || 0) > maximum || !response.body) {
        throw new Error('The original Rig daemon cannot verify this recovery metadata.');
      }
      const reader = response.body.getReader();
      const chunks: Uint8Array[] = [];
      let length = 0;
      for (;;) {
        const chunk = await reader.read();
        if (chunk.done) break;
        length += chunk.value.length;
        if (length > maximum) { await reader.cancel(); throw new Error('Rig recovery metadata exceeds its limit.'); }
        chunks.push(chunk.value);
      }
      return JSON.parse(Buffer.concat(chunks).toString('utf8')) as unknown;
    } finally { clearTimeout(timer); }
  };
}

function nativeState(file: string, id: string, cwd: string) {
  const descriptor = fs.openSync(file, fs.constants.O_RDONLY | fs.constants.O_NONBLOCK);
  try {
    const stat = fs.fstatSync(descriptor);
    if (!stat.isFile()) throw new Error('The native conversation is unavailable.');
    const bytes = Buffer.alloc(1024 * 1024);
    const read = fs.readSync(descriptor, bytes, 0, bytes.length, 0);
    for (const line of bytes.subarray(0, read).toString('utf8').split('\n').slice(0, 64)) {
      try {
        const record = object(JSON.parse(line));
        if (record.sessionId === id && typeof record.cwd === 'string' && sameDirectory(record.cwd, cwd)) return digest([id, file, stat.dev, stat.ino, stat.birthtimeMs]);
      } catch { /* Incomplete/unknown records establish no identity. */ }
    }
    throw new Error('The native conversation does not verify the recorded project and session.');
  } finally { fs.closeSync(descriptor); }
}

function boundedFile(file: string, maximumBytes: number): Buffer {
  const descriptor = fs.openSync(file, fs.constants.O_RDONLY | fs.constants.O_NONBLOCK);
  try {
    const stat = fs.fstatSync(descriptor);
    if (!stat.isFile() || stat.size > maximumBytes) throw new Error('Native recovery metadata exceeds its limit.');
    const bytes = Buffer.alloc(maximumBytes + 1);
    const length = fs.readSync(descriptor, bytes, 0, bytes.length, 0);
    if (length > maximumBytes) throw new Error('Native recovery metadata exceeds its limit.');
    return bytes.subarray(0, length);
  } finally { fs.closeSync(descriptor); }
}

function policyFiles(cwd: string, file: string) {
  const root = path.dirname(path.dirname(path.dirname(file)));
  const files = [path.join(root, 'settings.json'), path.join(cwd, '.claude/settings.json'), path.join(cwd, '.claude/settings.local.json')];
  return files.map(name => {
    try {
      return [name, crypto.createHash('sha256').update(boundedFile(name, 1024 * 1024)).digest('hex')];
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'ENOENT') return [name, null];
      throw error;
    }
  });
}

async function destination(run: RigRecoveryRun, identity: RigRecoveryIdentity) {
  if ((identity.provider && identity.provider !== 'rig') || identity.host !== 'localhost' || identity.runtime !== 'claude-code') {
    throw new Error('Native Rig recovery metadata is unsupported for this destination.');
  }
  const target = text(identity.target, 256);
  const cwd = text(identity.workspace);
  if (!path.isAbsolute(cwd)) throw new Error('The original Rig project is unavailable.');
  const get = await manager(run);
  // Resolve only the already-selected canonical seat through the stable CLI projection.
  const inventory = JSON.parse(run('rig', ['ps', '--nodes', '-A', '--json', '--fields', 'rigId,logicalId,canonicalSessionName,runtime'])) as unknown;
  const candidates = rows(Array.isArray(inventory) ? inventory : object(inventory).entries).filter(row => row.canonicalSessionName === target);
  if (candidates.length !== 1) throw new Error('The recorded Rig seat is missing or ambiguous.');
  const rigId = text(candidates[0].rigId), logicalId = text(candidates[0].logicalId, 256);
  if (!ulid.test(rigId)) throw new Error('The recorded Rig identity is invalid.');
  const route = `/api/rigs/${encodeURIComponent(rigId)}`;
  const node = object(await get(`${route}/nodes/${encodeURIComponent(logicalId)}`));
  const nodeId = text(node.nodeId);
  if (!ulid.test(nodeId) || node.rigId !== rigId || node.logicalId !== logicalId || node.canonicalSessionName !== target || node.runtime !== identity.runtime || !sameDirectory(text(node.cwd), cwd)) {
    throw new Error('The Rig seat destination changed.');
  }
  const permissions = object(object(await get(`/api/seat/status/${encodeURIComponent(target)}`)).permissions);
  const effective = object(permissions.effective);
  const desired = object(permissions.desired);
  if (desired.runtime !== 'claude-code' || desired.mode !== 'plan' || effective.permissionMode !== 'plan') {
    throw new Error('This native Rig recovery policy has not been qualified.');
  }
  const profile = text(node.profile, 128);
  if (profile !== 'default') throw new Error('This Rig profile is not qualified for native recovery.');
  const policy = (file: string) => digest({ rigId, nodeId, logicalId, cwd, runtime: node.runtime, profile, agentRef: node.agentRef, spec: node.resolvedSpecHash, model: node.model, permissions: { desired: { runtime: desired.runtime, mode: desired.mode }, effective }, files: policyFiles(cwd, file) });
  return { get, target, cwd, rigId, logicalId, route, nodeId, node, profile, policy };
}

function snapshotIdentity(snapshot: ObjectValue, context: Awaited<ReturnType<typeof destination>>, snapshotId: string, id: string) {
  const { rigId, nodeId, logicalId, cwd, profile } = context;
  const data = object(snapshot.data);
  const nodes = rows(data.nodes).filter(row => row.id === nodeId);
  const active = object(data.activeSessionIdByNode)[nodeId];
  const selected = rows(data.sessions).filter(row => row.id === active && row.nodeId === nodeId);
  const startup = object(data.nodeStartupContext)[nodeId];
  if (snapshot.id !== snapshotId || snapshot.rigId !== rigId || nodes.length !== 1 || nodes[0].rigId !== rigId || nodes[0].logicalId !== logicalId || nodes[0].runtime !== 'claude-code' || nodes[0].profile !== profile || !sameDirectory(text(nodes[0].cwd), cwd) || selected.length !== 1 || selected[0].sessionName !== context.target || selected[0].status !== 'running' || selected[0].resumeType !== 'claude_id' || selected[0].resumeToken !== id || !['hook', 'operator'].includes(String(selected[0].resumeProvenance)) || !startup) {
    throw new Error('The native Rig snapshot does not verify this managed conversation.');
  }
  return String(selected[0].resumeProvenance);
}

// Called only while the broker holds its logging-operation lock. A first
// observation may create one native snapshot; matching observations reuse it.
export async function observeRigRecovery(run: RigRecoveryRun, identity: RigRecoveryIdentity, directory: string, processObserver = observeRigNativeProcess, bootObserver = rigBootIdentity): Promise<RigRecoveryIdentity> {
  const context = await destination(run, identity);
  const { get, target, cwd, rigId, logicalId, route, nodeId, profile, policy } = context;
  const record = object(await get(`/api/sessions/${encodeURIComponent(target)}/generation-record`));
  const id = text(record.sessionId);
  const generation = JSON.parse(text(record.generationId)) as unknown;
  if (!uuid.test(id) || record.runtime !== 'claude-code' || !Array.isArray(generation) || generation.length !== 6 || typeof generation[0] !== 'string' || !uuid.test(generation[0]) || generation[1] !== id) {
    throw new Error('The native Rig conversation identity is unavailable.');
  }
  const file = text(generation[2]);
  if (!path.isAbsolute(file)) throw new Error('The native Rig conversation path is unavailable.');
  const stateIdentity = nativeState(file, id, cwd);
  if (stateIdentity !== digest(generation.slice(1))) throw new Error('The native conversation file identity changed.');
  const policyHash = policy(file);
  const key = digest([rigId, nodeId, id, stateIdentity, policyHash]);
  const cache = path.join(directory, 'recovery-' + crypto.createHash('sha256').update(target).digest('hex') + '.json');
  let snapshotId = '';
  try {
    const saved = object(JSON.parse(boundedFile(cache, 32768).toString('utf8')));
    if (saved.key === key && typeof saved.snapshotId === 'string' && ulid.test(saved.snapshotId)) snapshotId = saved.snapshotId;
  } catch { /* A missing/corrupt private observation is not reusable. */ }
  if (!snapshotId) {
    const snapshot = object(await get(`${route}/snapshots`, 'POST'));
    snapshotId = text(snapshot.id);
  }
  if (!ulid.test(snapshotId)) throw new Error('The native Rig snapshot identity is unavailable.');
  const snapshot = object(await get(`${route}/snapshots/${encodeURIComponent(snapshotId)}`));
  const provenance = snapshotIdentity(snapshot, context, snapshotId, id);
  let processRecord = '';
  try {
    const observed = processObserver(run, { ...identity, native_session_id: id }, true);
    if (!observed.native_session_id && provenance !== 'hook') throw new Error('The initial native identity is not hook-verified.');
    processRecord = JSON.stringify({ ...observed, native_session_id: id, native_identity_source: observed.native_session_id ? 'process-argv' : 'manager-hook', boot_id: bootObserver(run) });
  } catch { /* Retain checkpoint evidence, but leave unverified process state manual. */ }
  const temporary = cache + '.' + crypto.randomUUID() + '.tmp';
  try { fs.writeFileSync(temporary, JSON.stringify({ key, snapshotId }), { flag: 'wx', mode: 0o600 }); fs.renameSync(temporary, cache); }
  finally { fs.rmSync(temporary, { force: true }); }
  return { ...identity, manager_profile: profile, native_session_id: id, native_state_file: file, native_source: 'rig-snapshot', manager_rig_id: rigId, manager_node_id: nodeId, manager_logical_id: logicalId, manager_snapshot_id: snapshotId, manager_snapshot_hash: digest(snapshot), manager_policy_hash: policyHash, manager_record_identity: stateIdentity, manager_process: processRecord };
}

export interface RigRecoveryPlan {
  snapshot_id: string; rig_id: string; node_id: string; logical_id: string;
  target: string; native_session_id: string; plan_only: true;
}

// Read-only preflight. A valid manager plan is necessary, but cannot by itself
// authorize a launch or establish that the original process is gone.
export async function planRigRecovery(run: RigRecoveryRun, saved: RigRecoveryIdentity): Promise<RigRecoveryPlan> {
  const context = await destination(run, saved);
  const { get, route, rigId, nodeId, logicalId, cwd, policy } = context;
  const snapshotId = text(saved.manager_snapshot_id), nativeId = text(saved.native_session_id);
  const file = text(saved.native_state_file);
  if (saved.native_source !== 'rig-snapshot' || !ulid.test(snapshotId) || !uuid.test(nativeId) || !path.isAbsolute(file) || saved.manager_rig_id !== rigId || saved.manager_node_id !== nodeId || saved.manager_logical_id !== logicalId || saved.manager_profile !== context.profile) {
    throw new Error('The original Rig recovery destination changed.');
  }
  if (nativeState(file, nativeId, cwd) !== saved.manager_record_identity || policy(file) !== saved.manager_policy_hash) {
    throw new Error('The original native state or Rig recovery policy changed.');
  }
  const snapshot = object(await get(`${route}/snapshots/${encodeURIComponent(snapshotId)}`));
  snapshotIdentity(snapshot, context, snapshotId, nativeId);
  if (digest(snapshot) !== saved.manager_snapshot_hash) throw new Error('The recorded Rig checkpoint changed.');
  const plan = object(JSON.parse(run('rig', ['launch', rigId, logicalId, '--snapshot-id', snapshotId, '--plan', '--json'])));
  const targets = rows(plan.targetNodes), effects = object(plan.nonTargetEffects);
  if (plan.ok !== true || plan.planOnly !== true || object(plan.snapshotSelection).snapshotId !== snapshotId || targets.length !== 1 || targets[0].nodeId !== nodeId || targets[0].logicalId !== logicalId || !Array.isArray(plan.unmatchedIds) || plan.unmatchedIds.length !== 0 || effects.mode !== 'unchanged' || !Array.isArray(effects.affected) || effects.affected.length !== 0 || effects.reason !== null) {
    throw new Error('Rig cannot plan the exact recorded seat without other effects.');
  }
  return { snapshot_id: snapshotId, rig_id: rigId, node_id: nodeId, logical_id: logicalId, target: context.target, native_session_id: nativeId, plan_only: true };
}

function executableDigest(file: string): string {
  const fd = fs.openSync(file, fs.constants.O_RDONLY | fs.constants.O_NONBLOCK);
  try {
    const before = fs.fstatSync(fd);
    const limit = 512 * 1024 * 1024;
    if (!before.isFile() || before.size > limit) throw new Error('The native executable cannot be verified.');
    const hash = crypto.createHash('sha256'), block = Buffer.alloc(64 * 1024);
    let offset = 0;
    for (;;) {
      const n = fs.readSync(fd, block, 0, Math.min(block.length, limit - offset + 1), offset);
      if (!n) break;
      offset += n;
      if (offset > limit) throw new Error('The native executable exceeds its limit.');
      hash.update(block.subarray(0, n));
    }
    const after = fs.fstatSync(fd);
    if (after.size !== before.size || after.mtimeMs !== before.mtimeMs || offset !== before.size) throw new Error('The native executable changed during verification.');
    return hash.digest('hex');
  } finally { fs.closeSync(fd); }
}

function rigProcessStart(run: RigRecoveryRun, pid: string): string {
  return text(run('env', ['LC_ALL=C', 'TZ=UTC', 'ps', '-p', pid, '-o', 'lstart=']).trim(), 120) + ' UTC';
}

export interface RigNativeProcess {
  pane_pid: number; native_pid: number; started_at: string; execution_uid: number;
  native_session_id?: string; permission_mode: string;
  executable_hash: string; settings_hash: string;
}

// A tmux pane may be a shell wrapper. Only its native Claude descendant,
// independently joined to the OS executable and kernel cwd, is process proof.
// Read arguments only for this owned pane's bounded descendant set.
export function observeRigNativeProcess(run: RigRecoveryRun, saved: RigRecoveryIdentity, allowInitial = false, platform = process.platform): RigNativeProcess {
  if (!['darwin', 'linux'].includes(platform) || saved.host !== 'localhost' || saved.runtime !== 'claude-code' || saved.provider && saved.provider !== 'rig') throw new Error('Native Rig process proof is unsupported.');
  const target = text(saved.target, 256), cwd = text(saved.workspace);
  if (!/^[a-zA-Z0-9_@-]+$/.test(target) || !path.isAbsolute(cwd)) throw new Error('Native Rig process destination is invalid.');
  const pane = Number(run('tmux', ['display-message', '-p', '-t', '=' + target + ':', '#{pane_pid}']).trim());
  if (!Number.isSafeInteger(pane) || pane < 1) throw new Error('The original Rig pane is unavailable.');
  const table = run('ps', ['-axo', 'pid=,ppid=,uid=']);
  const ownerUID = process.getuid?.();
  if (ownerUID === undefined) throw new Error('The native process owner cannot be verified.');
  const owners = new Map<number, number>();
  if (table.length > maximum) throw new Error('Native process inventory exceeds its limit.');
  const parents = new Map<number, number>();
  for (const line of table.split('\n')) {
    if (!line.trim()) continue;
    const row = /^\s*(\d+)\s+(\d+)\s+(-?\d+)\s*$/.exec(line);
    if (!row) throw new Error('Native process inventory cannot be verified.');
    const pid = Number(row[1]), parent = Number(row[2]);
    if (!Number.isSafeInteger(pid) || !Number.isSafeInteger(parent) || parents.has(pid)) throw new Error('Native process inventory is ambiguous.');
    parents.set(pid, parent); owners.set(pid, Number(row[3]));
  }
  if (!parents.has(pane)) throw new Error('The original Rig pane process disappeared.');
  const descendants = new Set([pane]);
  for (let pass = 0; pass < 64; pass++) {
    const before = descendants.size;
    for (const [pid, parent] of parents) if (descendants.has(parent)) descendants.add(pid);
    if (descendants.size > 64) throw new Error('The native Rig process tree is too large to verify.');
    if (before === descendants.size) break;
    if (pass === 63) throw new Error('The native Rig process tree is too deep to verify.');
  }
  if ([...descendants].some(pid => owners.get(pid) !== ownerUID)) throw new Error('The native process tree crosses an execution-account boundary.');
  const selected = [...descendants].join(',');
  const commands = run('ps', ['-ww', '-p', selected, '-o', 'pid=,ppid=,args=']);
  if (commands.length > maximum) throw new Error('Native process arguments exceed their limit.');
  const candidates: { pid: number; parent: number; command: string; executable: string; id?: string; mode: string; settings: ObjectValue }[] = [];
  for (const line of commands.split('\n')) {
    if (!line.trim()) continue;
    const row = /^\s*(\d+)\s+(\d+)\s+(.+)$/.exec(line);
    if (!row || !descendants.has(Number(row[1])) || parents.get(Number(row[1])) !== Number(row[2])) throw new Error('The owned process tree changed.');
    // Published Rig uses settings last for resume, but appends the fresh
    // --session-id/name after settings on first launch. Parse only those exact
    // forms; JSON remains one opaque argument and cannot introduce options.
    const resumed = /^(\/\S*\/claude) --permission-mode (plan|acceptEdits) --resume ([a-f0-9-]{36}) --name ([a-zA-Z0-9_.:@-]+) --settings (\{.*\})$/.exec(row[3]);
    const initial = allowInitial ? /^(\/\S*\/claude) --permission-mode (plan|acceptEdits) --settings (\{.*\}) --session-id ([a-f0-9-]{36}) --name ([a-zA-Z0-9_.:@-]+)$/.exec(row[3]) : null;
    const native = resumed ? { executable: resumed[1], mode: resumed[2], id: resumed[3], target: resumed[4], settings: resumed[5] }
      : initial ? { executable: initial[1], mode: initial[2], id: initial[4], target: initial[5], settings: initial[3] } : undefined;
    if (!native) {
      if (/^\/\S*\/claude(?:\s|$)/.test(row[3])) throw new Error('The exact native Rig conversation process is missing or ambiguous.');
      continue;
    }
    if (native.target !== target || !uuid.test(native.id) || native.id !== saved.native_session_id || !allowInitial && native.mode !== 'plan') throw new Error('The exact native Rig conversation process is missing or ambiguous.');
    const settings = object(JSON.parse(native.settings));
    candidates.push({ pid: Number(row[1]), parent: Number(row[2]), command: row[3], executable: native.executable, id: native.id, mode: native.mode, settings });
  }
  if (candidates.length !== 1) throw new Error('The exact native Rig conversation process is missing or ambiguous.');
  const native = candidates[0], pid = String(native.pid);
  const started = rigProcessStart(run, pid);
  const executable = fs.realpathSync(native.executable);
  const executableHash = executableDigest(executable);
  if (platform === 'linux') {
    if (fs.realpathSync(`/proc/${pid}/exe`) !== executable || !sameDirectory(`/proc/${pid}/cwd`, cwd)) throw new Error('The native executable or kernel project changed.');
  } else {
    const executableFiles = run('lsof', ['-a', '-p', pid, '-d', 'txt', '-Fn']).split('\n').filter(line => line.startsWith('n')).map(line => line.slice(1));
    if (!executableFiles.some(file => { try { return fs.realpathSync(file) === executable; } catch { return false; } })) throw new Error('The native OS executable cannot be verified.');
    const directories = run('lsof', ['-a', '-p', pid, '-d', 'cwd', '-Fn']).split('\n').filter(line => line.startsWith('n')).map(line => line.slice(1));
    if (directories.length !== 1 || !sameDirectory(directories[0], cwd)) throw new Error('The native kernel project changed.');
  }
  // Verify the OS executable before invoking its version command; a forged
  // argv[0] must never cause an unrelated path to be executed.
  if (run(native.executable, ['--version']).trim() !== '2.1.322 (Claude Code)') throw new Error('This native Claude version is not qualified for Rig recovery.');
  const current = /^\s*(\d+)\s+(\d+)\s+(.+)$/.exec(run('ps', ['-ww', '-p', pid, '-o', 'pid=,ppid=,args=']).trim());
  if (!current || Number(current[1]) !== native.pid || Number(current[2]) !== native.parent || current[3] !== native.command) throw new Error('The native process arguments changed during verification.');
  if (rigProcessStart(run, pid) !== started || Number(run('tmux', ['display-message', '-p', '-t', '=' + target + ':', '#{pane_pid}']).trim()) !== pane) throw new Error('The native process changed during verification.');
  return { pane_pid: pane, native_pid: native.pid, started_at: started, execution_uid: ownerUID, native_session_id: native.id, permission_mode: native.mode, executable_hash: executableHash, settings_hash: digest(native.settings) };
}

// Hash a stable kernel boot identity, not wall-clock uptime. Process IDs can be
// reused after reboot; a WebMux or manager restart keeps this identity stable.
export function rigBootIdentity(run: RigRecoveryRun, platform = process.platform): string {
  if (platform === 'darwin') {
    const id = run('sysctl', ['-n', 'kern.bootsessionuuid']).trim();
    if (!uuid.test(id)) throw new Error('The execution host boot identity is unavailable.');
    return digest([platform, id.toLowerCase()]);
  }
  if (platform === 'linux') {
    const id = boundedFile('/proc/sys/kernel/random/boot_id', 256).toString('utf8').trim();
    if (!uuid.test(id)) throw new Error('The execution host boot identity is unavailable.');
    return digest([platform, id]);
  }
  throw new Error('Native Rig boot identity is unsupported.');
}
