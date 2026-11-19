// Controller-local public request identity, saved before topology allocation.
// Native operation UUIDs and their dispatch/inspection journal remain separate.
import path from 'node:path';
import { nativeCreateArtifacts as artifacts } from './rig-native-create.mts';

export interface HttpCreateSelection {
  mode: 'new' | 'existing'; rigId: string; rigName: string; pod: string;
  name: string; host: string; cwd: string; runtime: string;
  row: number | null; col: number | null;
}
export const httpCreationRequestID = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

// All tracked requests require native creation, including their FIRST dispatch.
// A previous caller's lost response must never turn into a legacy member launch.
export function retainHttpCreateRequest(directory: string, requestId: string, selection: HttpCreateSelection, nativeContract: boolean) {
  if (!httpCreationRequestID.test(requestId)) throw new Error('native_http_create_invalid_request_id');
  if (!nativeContract || !['darwin', 'linux'].includes(process.platform) || Number(process.versions.node.split('.')[0]) !== 24
    || selection.host !== 'localhost' || selection.runtime !== 'codex') throw new Error('native_http_create_contract_unavailable');
  if (!['new', 'existing'].includes(selection.mode)
    || !/^[a-z][a-z0-9-]{0,47}$/.test(selection.name)
    || (selection.mode === 'new' ? !/^[a-z][a-z0-9-]{0,47}$/.test(selection.rigName)
      : !/^[0-7][0-9A-HJKMNP-TV-Z]{25}$/.test(selection.rigId) || !/^[a-z][a-z0-9-]{0,47}$/.test(selection.pod))
    || !path.isAbsolute(selection.cwd) || /[\x00\r\n]/.test(selection.cwd) || selection.cwd.length > 2048
    || (selection.row === null) !== (selection.col === null)
    || [selection.row, selection.col].some(value => value !== null && (!Number.isSafeInteger(value) || value < 0))) {
    throw new Error('native_http_create_invalid_selection');
  }
  // Build explicitly: continuation flags, trust tokens and object property order
  // do not alter the original selection. Do not persist the process environment.
  const bytes = Buffer.from(JSON.stringify({ contract: 'webmux-native-http-create/v1', requestId,
    mode: selection.mode, rigId: selection.rigId, rigName: selection.rigName, pod: selection.pod,
    name: selection.name, host: selection.host, cwd: selection.cwd, runtime: selection.runtime,
    row: selection.row, col: selection.col }));
  const root = artifacts.ownedRoot(directory);
  const requests = artifacts.ownedRoot(path.join(root, 'http-requests'));
  const request = artifacts.ownedRoot(path.join(requests, requestId));
  const file = path.join(request, 'selection.json');
  try { artifacts.publish(file, bytes); }
  catch { throw new Error('native_http_create_original_request_unavailable'); }
  const refused = path.join(request, 'legacy-refused.json');
  const legacy = Buffer.from(JSON.stringify({ contract: 'webmux-native-http-legacy-refusal/v1', requestId }));
  const refuseLegacy = () => {
    // A later inventory change cannot silently convert an adopted legacy seat
    // into a new native launch under the same original public request.
    try { artifacts.read(refused); }
    catch (error) { if ((error as NodeJS.ErrnoException).code === 'ENOENT') return; throw error; }
    throw new Error('native_http_create_legacy_route_refused');
  };
  refuseLegacy();
  return {
    // The chosen existing/adopted member can only be classified after looking
    // up its topology. Refuse it before any legacy member effect.
    requireNative(native: boolean) {
      if (!native) {
        artifacts.publish(refused, legacy);
        throw new Error('native_http_create_legacy_route_refused');
      }
      refuseLegacy();
      // Recheck the durable selection immediately before handing off to the
      // native journal. Corruption or replacement cannot authorize a launch.
      if (!artifacts.read(file).equals(bytes)) throw new Error('native_http_create_original_request_unavailable');
    },
  };
}
