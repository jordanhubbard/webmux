// Called only by an authorized, explicit creation/inspection action. Native
// owner resolution can initialize integrations; never call it from a status GET.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { resolveCodexOwnerPlan, type CodexOwnerPlan, type CodexObservationHelper } from './codex-owner-plan.mts';
import { validateCodexLaunchPlan } from './codex-launch-policy.mts';
import { captureCodexLaunchEnvironment, validateCodexLaunchEnvironment, type CodexLaunchEnvironment } from './codex-launch-environment.mts';
import { openCodexEnvironmentChannel } from './codex-environment-channel.mts';
import { reviewRigCreateManager, type RigCreateManager, type RigStopRun } from './rig-stop.mts';
import { dispatchRigCreateIntent, inspectRigCreateIntent, type RigCreateIntent } from './rig-create-intent.mts';

const digest = (value: string | Buffer) => createHash('sha256').update(value).digest('hex');
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const refuse = (): never => { throw new Error('native_create_state_unavailable'); };
function sync(directory: string) {
  const fd = fs.openSync(directory, fs.constants.O_RDONLY);
  try { fs.fsyncSync(fd); } finally { fs.closeSync(fd); }
}
function ownedRoot(directory: string) {
  try { fs.mkdirSync(directory, { mode: 0o700 }); }
  catch (error) { if ((error as NodeJS.ErrnoException).code !== 'EEXIST') throw error; }
  const stat = fs.lstatSync(directory);
  if (!stat.isDirectory() || stat.isSymbolicLink() || stat.uid !== process.geteuid?.() || (stat.mode & 0o777) !== 0o700) return refuse();
  const canonical = fs.realpathSync(directory); sync(path.dirname(canonical)); return canonical;
}
function read(file: string, maximum = 262144): Buffer {
  const fd = fs.openSync(file, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW | fs.constants.O_NONBLOCK);
  try {
    const stat = fs.fstatSync(fd);
    if (!stat.isFile() || stat.uid !== process.geteuid?.() || stat.nlink !== 1 || (stat.mode & 0o777) !== 0o600 || stat.size > maximum) return refuse();
    const bytes = Buffer.alloc(maximum + 1), count = fs.readSync(fd, bytes, 0, bytes.length, 0), after = fs.fstatSync(fd);
    if (count !== stat.size || after.size !== stat.size || after.mtimeMs !== stat.mtimeMs || after.ctimeMs !== stat.ctimeMs) return refuse();
    fs.fsyncSync(fd); sync(path.dirname(file)); return bytes.subarray(0, count);
  } finally { fs.closeSync(fd); }
}
function publish(file: string, bytes: Buffer) {
  let fd: number;
  try { fd = fs.openSync(file, fs.constants.O_WRONLY | fs.constants.O_CREAT | fs.constants.O_EXCL, 0o600); }
  catch (error) {
    if ((error as NodeJS.ErrnoException).code !== 'EEXIST') throw error;
    if (!read(file).equals(bytes)) return refuse();
    return;
  }
  try { fs.writeFileSync(fd, bytes); fs.fsyncSync(fd); } finally { fs.closeSync(fd); }
  sync(path.dirname(file));
}
// Shared by the topology allocator so all native creation artifacts use the
// same owner, file-type, exclusive-publication and durability checks.
export const nativeCreateArtifacts = { ownedRoot, read, publish };
const json = (value: unknown) => Buffer.from(JSON.stringify(value));
const agentBytes = json({ name: 'webmux-native-codex', version: '1.0', description: 'Owner Codex with native lifecycle',
  defaults: { runtime: 'codex' }, profiles: { default: { uses: { skills: [], guidance: [], subagents: [], plugins: [], runtime_resources: [] } } } });
const bundleFiles = ['codex-owner-plan.mts', 'codex-launch-policy.mts', 'codex-launch-wrapper.mts', 'codex-launch-environment.mts', 'codex-environment-channel.mts'] as const;
export function publishCodexLaunchBundle(directory: string, source = path.dirname(fileURLToPath(import.meta.url))) {
  const base = ownedRoot(directory), bundles = ownedRoot(path.join(base, 'launchers'));
  const files = bundleFiles.map(name => {
    const file = path.join(source, name), stat = fs.lstatSync(file);
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 131072) return refuse();
    const bytes = fs.readFileSync(file); if (bytes.length !== stat.size) return refuse();
    return { name, bytes };
  });
  const identity = digest(Buffer.concat(files.flatMap(({ name, bytes }) => [Buffer.from(name + '\0'), bytes])));
  const destination = ownedRoot(path.join(bundles, identity));
  for (const { name, bytes } of files) publish(path.join(destination, name), bytes);
  sync(destination); sync(bundles);
  return { identity, wrapper: path.join(destination, 'codex-launch-wrapper.mts') };
}

function nativeRequest(plan: CodexOwnerPlan, member: string, agent: string, wrapper: string, manifest: string, directory: string) {
  const permissions = plan.policy.type === 'dangerFullAccess' ? { permission_policy: 'builtin:yolo' }
      : { codex_permissions: plan.policy.type === 'readOnly' ? { sandbox: 'read-only', approval: plan.approval }
        : { sandbox: 'workspace-write', approval: plan.approval, network_access: plan.policy.networkAccess,
          writable_roots: plan.policy.writableRoots, exclude_slash_tmp: plan.policy.excludeSlashTmp, exclude_tmpdir_env_var: plan.policy.excludeTmpdirEnvVar } };
  return JSON.stringify({ member: { id: member, runtime: 'codex', agent_ref: 'path:' + agent, profile: 'default',
      cwd: plan.cwd, model: plan.model, ...(plan.effort ? { effort: plan.effort } : {}), env: { CODEX_HOME: plan.home },
      launch_wrapper: [process.execPath, wrapper, manifest], ...permissions }, rigRoot: directory });
}

export interface NativeCodexCreateInput {
  directory: string; rigId: string; pod: string; member: string; cwd: string; inspect?: boolean;
}
interface SavedCreate {
  contract: 'webmux-native-codex-create/v2'; cwd: string; member: string; bundle: string;
  plan: CodexOwnerPlan; environment: CodexLaunchEnvironment; intent: RigCreateIntent;
}
interface Dependencies {
  observationHelper?: CodexObservationHelper;
  run: RigStopRun; fetcher?: typeof fetch; source?: string; env?: NodeJS.ProcessEnv; handoffLifetime?: number;
  review?: () => Promise<RigCreateManager>;
  resolve?: (cwd: string, env: NodeJS.ProcessEnv) => Promise<CodexOwnerPlan>;
}
export async function createRigNativeCodex(input: NativeCodexCreateInput, deps: Dependencies) {
  if (!['darwin', 'linux'].includes(process.platform) || Number(process.versions.node.split('.')[0]) !== 24
    || !/^[0-7][0-9A-HJKMNP-TV-Z]{25}$/.test(input.rigId) || !/^[a-z][a-z0-9-]{0,47}$/.test(input.pod)
    || !/^[a-z][a-z0-9-]{0,63}$/.test(input.member) || !path.isAbsolute(input.cwd)) return refuse();
  const cwd = fs.realpathSync(input.cwd), base = ownedRoot(input.directory);
  if (!fs.statSync(cwd).isDirectory()) return refuse();
  const key = digest(JSON.stringify([input.rigId, input.pod, input.member]));
  const requests = ownedRoot(path.join(base, 'requests')), directory = ownedRoot(path.join(requests, key));
  // Unix socket paths have a much smaller bound than durable profile paths.
  // A deterministic owner-private runtime directory also permits stale-socket
  // refusal/reclamation after controller failure without moving the journal.
  const channelRoot = path.join(fs.realpathSync('/tmp'), `wm-env-${process.geteuid!()}-${digest(base).slice(0, 16)}`);
  const environmentSocket = path.join(channelRoot, key.slice(0, 24) + '.sock');
  const savedFile = path.join(directory, 'launch.json');
  const review = deps.review ?? (() => reviewRigCreateManager(deps.run, deps.fetcher));
  const ownerEnvironment = { ...(deps.env ?? process.env) };
  const resolve = deps.resolve ?? ((value: string, env: NodeJS.ProcessEnv) => resolveCodexOwnerPlan(value, { env, observationHelper: deps.observationHelper }));
  const manager = await review();
  let saved: SavedCreate;
  try { saved = JSON.parse(read(savedFile).toString('utf8')) as SavedCreate; }
  catch (error) {
    if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error;
    if (input.inspect) throw new Error('native_create_original_request_missing');
    // An unrelated current member is not evidence of our original operation.
    const raw: unknown = JSON.parse(deps.run('rig', ['ps', '--nodes', '-A', '--no-cleanup', '--json', '--full']));
    const rows = Array.isArray(raw) ? raw : object(raw) ? raw.entries : undefined;
    if (!Array.isArray(rows) || rows.length > 4096 || !rows.every(object)) return refuse();
    if (rows.some(row => row.rigId === input.rigId && row.logicalId === `${input.pod}.${input.member}`)) throw new Error('native_create_member_already_exists');
    const plan = validateCodexLaunchPlan(await resolve(cwd, ownerEnvironment));
    if (plan.cwd !== cwd) return refuse();
    const environment = captureCodexLaunchEnvironment(ownerEnvironment, plan.home);
    const bundle = publishCodexLaunchBundle(base, deps.source);
    const agent = ownedRoot(path.join(directory, 'agent'));
    publish(path.join(agent, 'agent.yaml'), agentBytes);
    const manifest = path.join(directory, 'launch-manifest.json');
    publish(manifest, json({ contract: 'webmux-codex-launch/v2', plan, environment, environmentSocket }));
    const requestText = nativeRequest(plan, input.member, agent, bundle.wrapper, manifest, directory);
    saved = { contract: 'webmux-native-codex-create/v2', cwd, member: input.member, bundle: bundle.identity, plan, environment,
      intent: { managerIdentity: manager.managerIdentity, port: manager.port, rigId: input.rigId, pod: input.pod, requestText } };
    publish(savedFile, json(saved));
  }
  if (!object(saved) || saved.contract !== 'webmux-native-codex-create/v2' || saved.cwd !== cwd || saved.member !== input.member
    || saved.intent?.rigId !== input.rigId || saved.intent.pod !== input.pod || saved.intent.managerIdentity !== manager.managerIdentity
    || saved.intent.port !== manager.port) throw new Error('native_create_original_request_changed');
  validateCodexLaunchPlan(saved.plan);
  validateCodexLaunchEnvironment(saved.environment, saved.plan.home);
  if (!/^[a-f0-9]{64}$/.test(saved.bundle) || saved.intent.requestText !== nativeRequest(saved.plan, input.member,
    path.join(directory, 'agent'), path.join(base, 'launchers', saved.bundle, 'codex-launch-wrapper.mts'), path.join(directory, 'launch-manifest.json'), directory)
    || !read(path.join(directory, 'launch-manifest.json')).equals(json({ contract: 'webmux-codex-launch/v2', plan: saved.plan, environment: saved.environment, environmentSocket }))
    || !read(path.join(directory, 'agent', 'agent.yaml')).equals(agentBytes)) throw new Error('native_create_original_request_changed');
  const journal = ownedRoot(path.join(directory, 'journal'));
  // Ordinary duplicates go straight to the consumed journal. They never start
  // another native policy-observation process or rebuild the original bytes.
  let channel: Awaited<ReturnType<typeof openCodexEnvironmentChannel>> | undefined;
  if (!fs.existsSync(path.join(journal, key + '.dispatched')) || input.inspect) {
    const currentEnvironment = { ...(deps.env ?? process.env) };
    if (captureCodexLaunchEnvironment(currentEnvironment, saved.plan.home).fingerprint !== saved.environment.fingerprint) throw new Error('native_create_owner_environment_changed');
    const current = validateCodexLaunchPlan(await resolve(cwd, currentEnvironment));
    if (current.fingerprint !== saved.plan.fingerprint) throw new Error('native_create_owner_plan_changed');
    const bundle = publishCodexLaunchBundle(base, deps.source);
    if (bundle.identity !== saved.bundle) throw new Error('native_create_launch_bundle_changed');
    const after = await review();
    if (after.managerIdentity !== manager.managerIdentity || after.processIdentity !== manager.processIdentity || after.port !== manager.port) throw new Error('native_create_manager_changed');
    const receipt = path.join(directory, 'environment-consumed.json');
    const consumed = json({ contract: 'webmux-codex-environment-consumed/v1', fingerprint: saved.environment.fingerprint });
    let needsHandoff = true;
    try { if (!read(receipt).equals(consumed)) return refuse(); needsHandoff = false; }
    catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
    if (needsHandoff) {
      ownedRoot(channelRoot);
      channel = await openCodexEnvironmentChannel(environmentSocket, saved.environment, currentEnvironment,
        () => publish(receipt, consumed), deps.handoffLifetime);
    }
  }
  try {
    const result = await (input.inspect ? inspectRigCreateIntent : dispatchRigCreateIntent)(journal, key, saved.intent, deps.fetcher);
    if (channel && result.posted) await channel.wait();
    return { ...result, logicalId: `${input.pod}.${input.member}`, planFingerprint: saved.plan.fingerprint };
  } finally {
    await channel?.close();
    if (channel) { try { fs.rmdirSync(channelRoot); } catch (error) {
      if (!['ENOENT', 'ENOTEMPTY'].includes((error as NodeJS.ErrnoException).code ?? '')) throw error;
    } }
  }
}
