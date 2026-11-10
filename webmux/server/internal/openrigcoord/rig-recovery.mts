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
  manager_policy_hash?: string; manager_record_identity?: string;
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
    if (!fs.fstatSync(descriptor).isFile()) throw new Error('The native conversation is unavailable.');
    const bytes = Buffer.alloc(1024 * 1024);
    const read = fs.readSync(descriptor, bytes, 0, bytes.length, 0);
    for (const line of bytes.subarray(0, read).toString('utf8').split('\n').slice(0, 64)) {
      try {
        const record = object(JSON.parse(line));
        if (record.sessionId === id && typeof record.cwd === 'string' && sameDirectory(record.cwd, cwd)) return;
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

// Called only while the broker holds its logging-operation lock. A first
// observation may create one native snapshot; matching observations reuse it.
export async function observeRigRecovery(run: RigRecoveryRun, identity: RigRecoveryIdentity, directory: string): Promise<RigRecoveryIdentity> {
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
  const record = object(await get(`/api/sessions/${encodeURIComponent(target)}/generation-record`));
  const id = text(record.sessionId);
  const generation = JSON.parse(text(record.generationId)) as unknown;
  if (!uuid.test(id) || record.runtime !== 'claude-code' || !Array.isArray(generation) || generation.length !== 6 || typeof generation[0] !== 'string' || !uuid.test(generation[0]) || generation[1] !== id) {
    throw new Error('The native Rig conversation identity is unavailable.');
  }
  const file = text(generation[2]);
  if (!path.isAbsolute(file)) throw new Error('The native Rig conversation path is unavailable.');
  nativeState(file, id, cwd);
  const profile = text(node.profile, 128);
  if (profile !== 'default') throw new Error('This Rig profile is not qualified for native recovery.');
  const stateIdentity = digest(generation.slice(1));
  const policy = digest({ rigId, nodeId, logicalId, cwd, runtime: node.runtime, profile, agentRef: node.agentRef, spec: node.resolvedSpecHash, model: node.model, permissions: { desired: { runtime: desired.runtime, mode: desired.mode }, effective }, files: policyFiles(cwd, file) });
  const key = digest([rigId, nodeId, id, stateIdentity, policy]);
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
  const data = object(snapshot.data);
  const savedNodes = rows(data.nodes).filter(row => row.id === nodeId);
  const sessions = rows(data.sessions);
  const active = object(data.activeSessionIdByNode)[nodeId];
  const selected = sessions.filter(row => row.id === active && row.nodeId === nodeId);
  const contexts = object(data.nodeStartupContext);
  if (snapshot.rigId !== rigId || savedNodes.length !== 1 || savedNodes[0].logicalId !== logicalId || selected.length !== 1 || selected[0].status !== 'running' || selected[0].resumeType !== 'claude_id' || selected[0].resumeToken !== id || !['hook', 'operator'].includes(String(selected[0].resumeProvenance)) || !contexts[nodeId]) {
    throw new Error('The native Rig snapshot does not verify this managed conversation.');
  }
  const temporary = cache + '.' + crypto.randomUUID() + '.tmp';
  try { fs.writeFileSync(temporary, JSON.stringify({ key, snapshotId }), { flag: 'wx', mode: 0o600 }); fs.renameSync(temporary, cache); }
  finally { fs.rmSync(temporary, { force: true }); }
  return { ...identity, manager_profile: profile, native_session_id: id, native_state_file: file, native_source: 'rig-snapshot', manager_rig_id: rigId, manager_node_id: nodeId, manager_logical_id: logicalId, manager_snapshot_id: snapshotId, manager_snapshot_hash: digest(snapshot), manager_policy_hash: policy, manager_record_identity: stateIdentity };
}
