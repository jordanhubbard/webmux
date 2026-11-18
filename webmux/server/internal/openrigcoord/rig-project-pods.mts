// Discover one explicitly selected canonical namespace without observing an
// account or starting a provider. Seat presence is not a pod definition.
import { reviewRigCreateManager, type RigCreateManager, type RigStopRun } from './rig-stop.mts';

const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const refuse = (): never => { throw new Error('native_project_namespace_unverified'); };
export interface RigProjectPods { projectId: string; projectName: string; pods: string[]; source: 'native-spec' | 'legacy-seats' }
interface Dependencies { run: RigStopRun; fetcher?: typeof fetch; review?: () => Promise<RigCreateManager> }

export async function readRigProjectPods(projectId: string, projectName: string, deps: Dependencies): Promise<RigProjectPods> {
  if (!/^[0-7][0-9A-HJKMNP-TV-Z]{25}$/.test(projectId) || !projectName || projectName.length > 256 || /[\x00-\x1f\x7f]/.test(projectName)) return refuse();
  const fetcher = deps.fetcher ?? fetch;
  const review = deps.review ?? (() => reviewRigCreateManager(deps.run, fetcher));
  const manager = await review();
  const read = async (route: string): Promise<unknown> => {
    const response = await fetcher(`http://127.0.0.1:${manager.port}${route}`, { redirect: 'error', signal: AbortSignal.timeout(3000) });
    if (!response.ok || !response.body || Number(response.headers.get('content-length') ?? 0) > 1048576) return refuse();
    const reader = response.body.getReader(), chunks: Uint8Array[] = []; let size = 0;
    try { for (;;) {
      const part = await reader.read(); if (part.done) break;
      size += part.value.length;
      if (size > 1048576) { await reader.cancel(); return refuse(); }
      chunks.push(part.value);
    } } finally { reader.releaseLock(); }
    try { return JSON.parse(Buffer.concat(chunks).toString('utf8')) as unknown; }
    catch { return refuse(); }
  };
  // The default native list excludes archived rigs. A stopped/empty but current
  // rig remains eligible; an old local receipt cannot make an archived rig live.
  const rigs = await read('/api/rigs');
  if (!Array.isArray(rigs) || rigs.length > 4096) return refuse();
  const matches = rigs.filter(value => object(value) && value.id === projectId);
  if (matches.length !== 1 || !object(matches[0]) || matches[0].name !== projectName) return refuse();
  const spec = await read(`/api/rigs/${projectId}/spec.json`);
  if (!object(spec) || spec.name !== projectName || !Array.isArray(spec.pods) || spec.pods.length > 4096) return refuse();
  const pods: string[] = [];
  for (const pod of spec.pods) {
    if (!object(pod) || typeof pod.id !== 'string' || !/^[a-z][a-z0-9-]{0,47}$/.test(pod.id) || pods.includes(pod.id)) return refuse();
    pods.push(pod.id);
  }
  const after = await review();
  if (after.managerIdentity !== manager.managerIdentity || after.processIdentity !== manager.processIdentity || after.port !== manager.port) return refuse();
  return { projectId, projectName, pods: pods.sort(), source: 'native-spec' };
}
