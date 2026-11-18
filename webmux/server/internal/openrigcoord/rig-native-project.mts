// Allocate an empty native topology, never a coding process. The caller must
// authorize the owner and use this only for an explicit create/inspect action.
import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { nativeCreateArtifacts as artifacts } from './rig-native-create.mts';
import { dispatchRigTopologyIntent, inspectRigTopologyIntent, reserveRigTopologyIntent, type RigTopologyIntent, type RigCreateResult } from './rig-create-intent.mts';
import { reviewRigCreateManager, type RigCreateManager, type RigStopRun } from './rig-stop.mts';

const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const digest = (value: string) => createHash('sha256').update(value).digest('hex');
const json = (value: unknown) => Buffer.from(JSON.stringify(value));
export interface NativeRigProjectInput { directory: string; name: string; cwd: string; inspect?: boolean }
interface Dependencies { run: RigStopRun; fetcher?: typeof fetch; review?: () => Promise<RigCreateManager> }
export async function createRigNativeProject(input: NativeRigProjectInput, deps: Dependencies) {
  if (!['darwin', 'linux'].includes(process.platform) || !/^[a-z][a-z0-9-]{0,47}$/.test(input.name)
    || !path.isAbsolute(input.cwd)) throw new Error('native_project_request_invalid');
  const cwd = fs.realpathSync(input.cwd);
  if (!fs.statSync(cwd).isDirectory()) throw new Error('native_project_request_invalid');
  const base = artifacts.ownedRoot(input.directory), projects = artifacts.ownedRoot(path.join(base, 'projects'));
  const key = digest(input.name), directory = artifacts.ownedRoot(path.join(projects, key));
  const requestText = JSON.stringify({ version: '0.2', name: input.name, summary: `Coordinated project ${input.name}`,
    pods: [{ id: 'agents', label: input.name, members: [] }], edges: [] });
  const review = deps.review ?? (() => reviewRigCreateManager(deps.run, deps.fetcher));
  const manager = await review();
  const file = path.join(directory, 'original.json');
  const intent: RigTopologyIntent = { kind: 'rig.materialize', managerIdentity: manager.managerIdentity,
    port: manager.port, rigRoot: directory, cwd, requestText };
  let saved: unknown;
  try { saved = JSON.parse(artifacts.read(file).toString('utf8')); }
  catch (error) {
    if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error;
    if (input.inspect) throw new Error('native_project_original_request_missing');
    artifacts.publish(path.join(directory, 'rig.json'), Buffer.from(requestText));
    artifacts.publish(file, json(intent)); saved = intent;
  }
  if (!json(saved).equals(json(intent)) || !artifacts.read(path.join(directory, 'rig.json')).equals(Buffer.from(requestText))) {
    throw new Error('native_project_original_request_changed');
  }
  const journal = artifacts.ownedRoot(path.join(directory, 'journal'));
  const consumed = fs.existsSync(path.join(journal, key + '.dispatched'));
  const confirmedFile = path.join(directory, 'enrollment.json');
  let confirmed: Record<string, unknown> | undefined;
  try {
    const value: unknown = JSON.parse(artifacts.read(confirmedFile).toString('utf8'));
    if (!object(value)) throw new Error('native_project_receipt_invalid');
    const { fingerprint, ...content } = value;
    if (Object.keys(value).sort().join(',') !== 'contract,fingerprint,operationId,rigId'
      || content.contract !== 'webmux-native-rig-project/v1' || fingerprint !== digest(JSON.stringify(content))
      || typeof content.rigId !== 'string' || !/^[0-7][0-9A-HJKMNP-TV-Z]{25}$/.test(content.rigId)
      || !consumed || content.operationId !== reserveRigTopologyIntent(journal, key, intent).operationId) throw new Error('native_project_receipt_invalid');
    confirmed = content;
  } catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
  if (!consumed || input.inspect) {
    const after = await review();
    if (after.managerIdentity !== manager.managerIdentity || after.processIdentity !== manager.processIdentity || after.port !== manager.port) throw new Error('native_project_manager_changed');
  }
  const result: RigCreateResult = confirmed && !input.inspect
    ? { state: 'recorded', code: 'native_topology_recorded', operationId: confirmed.operationId as string, rigId: confirmed.rigId as string, nodeIds: [], posted: false }
    : await (input.inspect && consumed ? inspectRigTopologyIntent : dispatchRigTopologyIntent)(journal, key, intent, deps.fetcher);
  if (result.state === 'recorded' && result.rigId) {
    // Receipt proves the original rig still exists, not that its namespace is
    // usable. Inspect its actual current pod rather than creating a replacement
    // when an empty/stopped topology is absent from active CLI inventory.
    try {
      const readNative = async (suffix: string): Promise<unknown> => {
        const response = await (deps.fetcher ?? fetch)(`http://127.0.0.1:${manager.port}` + suffix, { redirect: 'error', signal: AbortSignal.timeout(5000) });
        const declared = Number(response.headers.get('content-length') ?? 0);
        if (!response.ok || declared > 1048576 || !response.body) throw new Error();
        const reader = response.body.getReader(); let bytes = 0; const chunks: Uint8Array[] = [];
        try { for (;;) { const part = await reader.read(); if (part.done) break; bytes += part.value.length;
          if (bytes > 1048576) { await reader.cancel(); throw new Error(); } chunks.push(part.value); } }
        finally { reader.releaseLock(); }
        return JSON.parse(Buffer.concat(chunks).toString('utf8')) as unknown;
      };
      const current = await readNative('/api/rigs');
      if (!Array.isArray(current) || current.length > 4096 || current.filter(rig => object(rig) && rig.id === result.rigId && rig.name === input.name).length !== 1) throw new Error();
      const spec = await readNative(`/api/rigs/${result.rigId}/spec.json`);
      if (!object(spec) || spec.name !== input.name || !Array.isArray(spec.pods) || spec.pods.length > 128
        || spec.pods.filter(pod => object(pod) && pod.id === 'agents').length !== 1) throw new Error();
    } catch { return { ...result, state: 'uncertain' as const, code: 'native_project_namespace_unverified', projectName: input.name, pod: 'agents' }; }
    const receipt = { contract: 'webmux-native-rig-project/v1', operationId: result.operationId, rigId: result.rigId };
    artifacts.publish(confirmedFile, json({ ...receipt, fingerprint: digest(JSON.stringify(receipt)) }));
  }
  return { ...result, projectName: input.name, pod: 'agents' };
}
