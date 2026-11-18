// Resolve through the native CLI, never by treating a config file or a legacy
// sandbox summary as a complete permission graph. No turn is submitted.
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { spawn, execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';

type Row = Record<string, unknown>;
export type CodexOwnerPolicy = { type: 'dangerFullAccess' } | { type: 'readOnly'; networkAccess: boolean }
  | { type: 'workspaceWrite'; networkAccess: boolean; writableRoots: string[]; excludeSlashTmp: boolean; excludeTmpdirEnvVar: boolean };
export interface CodexOwnerPlan {
  contract: 'webmux-codex-owner-plan/v1'; version: '0.169.0'; executable: string; executableSHA256: string;
  home: string; cwd: string; profile: string | null; model: string; provider: 'openai';
  workspaceRoots: string[];
  effort: string | null; serviceTier: string | null; approval: 'never' | 'on-request';
  policy: CodexOwnerPolicy; accountFingerprint: string; fingerprint: string;
}
const object = (value: unknown): value is Row => !!value && typeof value === 'object' && !Array.isArray(value);
const digest = (value: string) => createHash('sha256').update(value).digest('hex');
const text = (value: unknown, max = 256): string => {
  if (typeof value !== 'string' || !value || value.length > max || /[\x00-\x1f\x7f]/.test(value)) throw new Error('native_owner_plan_unavailable');
  return value;
};
const keys = (value: Row, allowed: string[]) => {
  if (Object.keys(value).some(key => !allowed.includes(key))) throw new Error('native_policy_not_representable');
};
const boolean = (value: unknown) => {
  if (value !== undefined && typeof value !== 'boolean') throw new Error('native_policy_not_representable');
  return value === true;
};
const absolutePaths = (value: unknown): string[] => {
  if (!Array.isArray(value) || value.length > 128) throw new Error('native_policy_not_representable');
  return value.map(root => { const name = text(root, 4096); if (!path.isAbsolute(name)) throw new Error('native_policy_not_representable'); return name; });
};

export function projectCodexOwnerPlan(config: unknown, thread: unknown, account: unknown, identity: {
  executable: string; executableSHA256: string; home: string; cwd: string; profile: string | null;
}): CodexOwnerPlan {
  if (identity.profile !== null) throw new Error('codex_profile_resolution_unsupported');
  if (!object(config) || !object(thread) || !object(account) || !object(thread.thread) || thread.thread.ephemeral !== true
    || thread.cwd !== identity.cwd || thread.modelProvider !== 'openai' || thread.approvalsReviewer !== 'user'
    || !['never', 'on-request'].includes(String(thread.approvalPolicy))) throw new Error('native_owner_plan_unavailable');
  // Named permission profiles can carry denies/inheritance which the legacy
  // thread sandbox field cannot express. Do not erase them during conversion.
  const active = thread.activePermissionProfile;
  if (object(active)) keys(active, ['id', 'extends']);
  if ((config.default_permissions !== undefined && config.default_permissions !== null)
    || (object(active) && (typeof active.id !== 'string' || !active.id.startsWith(':') || active.extends != null))
    || (active != null && !object(active))) throw new Error('native_policy_not_representable');
  if (!object(thread.sandbox)) throw new Error('native_policy_not_representable');
  const sandbox = thread.sandbox;
  let policy: CodexOwnerPolicy;
  if (sandbox.type === 'dangerFullAccess') {
    keys(sandbox, ['type']); policy = { type: 'dangerFullAccess' };
    if (thread.approvalPolicy !== 'never') throw new Error('native_policy_not_representable');
  } else if (sandbox.type === 'readOnly') {
    keys(sandbox, ['type', 'networkAccess']); policy = { type: 'readOnly', networkAccess: boolean(sandbox.networkAccess) };
  } else if (sandbox.type === 'workspaceWrite') {
    keys(sandbox, ['type', 'networkAccess', 'writableRoots', 'excludeSlashTmp', 'excludeTmpdirEnvVar']);
    policy = { type: 'workspaceWrite', networkAccess: boolean(sandbox.networkAccess),
      writableRoots: absolutePaths(sandbox.writableRoots ?? []),
      excludeSlashTmp: boolean(sandbox.excludeSlashTmp), excludeTmpdirEnvVar: boolean(sandbox.excludeTmpdirEnvVar) };
  } else throw new Error('native_policy_not_representable');
  // Stable native account/workspace ids, not email, token files or display names.
  if (account.requiresOpenaiAuth !== true || !object(account.account) || account.account.type !== 'chatgpt' || !object(account.workspaceRouting)) throw new Error('native_account_identity_unavailable');
  const routing = account.workspaceRouting;
  const accountFingerprint = digest(JSON.stringify([text(account.userId), text(routing.chatgptAccountId),
    text(routing.backendOrigin, 2048), text(routing.accountRoutingOverride)]));
  const base = { contract: 'webmux-codex-owner-plan/v1' as const, version: '0.169.0' as const, ...identity,
    model: text(thread.model), provider: 'openai' as const, effort: thread.reasoningEffort == null ? null : text(thread.reasoningEffort, 32),
    workspaceRoots: absolutePaths(thread.runtimeWorkspaceRoots ?? []),
    serviceTier: thread.serviceTier == null ? null : text(thread.serviceTier, 32),
    approval: thread.approvalPolicy as 'never' | 'on-request', policy, accountFingerprint };
  return { ...base, fingerprint: digest(JSON.stringify(base)) };
}

// These overrides belong only to the disposable observation thread. They must
// never become launch settings: the actual provider retains its integrations.
// No-turn/ephemeral alone does not prevent native MCP process startup.
export function codexObservationConfig(config: unknown): Row {
  if (!object(config)) throw new Error('native_owner_resolution_failed');
  const servers = config.mcp_servers ?? {};
  if (!object(servers) || Object.keys(servers).length > 128) throw new Error('native_startup_config_unavailable');
  const overrides: Row = { 'features.hooks': false, 'features.plugins': false,
    'features.apps': false, 'features.shell_snapshot': false };
  const disabled: Array<[string, Row & { enabled: false }]> = [];
  for (const name of Object.keys(servers)) {
    if (!name || name.length > 256 || /[\x00-\x1f\x7f]/.test(name) || !object(servers[name])) throw new Error('native_startup_config_unavailable');
    const server = servers[name];
    // A request table containing enabled:false alone loses a transport supplied
    // by native CLI overrides. Retain only its transport fields; normalized
    // config/read entries contain other null/default fields that cannot be
    // round-tripped as thread TOML overrides. Environment and headers stay native.
    let transport: Row;
    if (typeof server.command === 'string' && server.command) {
      if (server.args != null && (!Array.isArray(server.args) || server.args.length > 1024
        || !server.args.every(arg => typeof arg === 'string'))) throw new Error('native_startup_config_unavailable');
      transport = { command: server.command, ...(Array.isArray(server.args) ? { args: server.args } : {}) };
    } else if (typeof server.url === 'string' && server.url) transport = { url: server.url };
    else throw new Error('native_startup_config_unavailable');
    disabled.push([name, { ...transport, enabled: false }]);
  }
  // Supply a table, not a dotted override key: native request keys do not
  // interpret TOML quotes, and a server name may itself contain a dot.
  if (disabled.length) overrides.mcp_servers = Object.fromEntries(disabled);
  return overrides;
}

function executableAt(name: string, env: NodeJS.ProcessEnv) {
  if (!path.isAbsolute(name) && !/^[a-zA-Z0-9_.-]+$/.test(name)) throw new Error('qualified_codex_unavailable');
  const candidates = path.isAbsolute(name) ? [name] : (env.PATH ?? '').split(path.delimiter)
    .filter(entry => path.isAbsolute(entry)).map(entry => path.join(entry, name));
  for (const candidate of candidates) {
    try {
      const resolved = fs.realpathSync(candidate);
      fs.accessSync(resolved, fs.constants.X_OK);
      if (fs.statSync(resolved).isFile()) return resolved;
    } catch { /* Next executable on the declared service PATH. */ }
  }
  throw new Error('qualified_codex_unavailable');
}

export type CodexObservationStage = 'account' | 'policy';
export type CodexObservationCommand = (executable: string, args: string[], stage: CodexObservationStage) => { executable: string; args: string[] };
// Only the disposable policy observer is restricted. The actual coding process
// keeps its original owner policy and environment. Never fall back unguarded.
export function codexObservationCommand(executable: string, args: string[], stage: CodexObservationStage,
  platform: NodeJS.Platform = process.platform) {
  if (platform !== 'darwin') throw new Error('native_owner_observation_isolation_unqualified');
  text(executable, 4096);
  if (!path.isAbsolute(executable)) throw new Error('qualified_codex_unavailable');
  if (stage === 'account') return { executable, args };
  const guard = '/usr/bin/sandbox-exec';
  fs.accessSync(guard, fs.constants.X_OK);
  const profile = '(version 1) (allow default) (deny network*) (deny process-fork) (deny process-exec) (allow process-exec (literal '
    + JSON.stringify(executable) + '))';
  return { executable: guard, args: ['-p', profile, executable, ...args] };
}
function nativeConfig(reply: unknown) {
  if (!object(reply) || !Array.isArray(reply.layers) || reply.layers.length > 128
    || !reply.layers.every(layer => object(layer) && object(layer.name) && typeof layer.name.type === 'string')) throw new Error('native_config_layers_unavailable');
  if (reply.layers.some(layer => object(layer) && object(layer.name) && layer.name.type === 'project' && layer.disabledReason != null)) {
    throw new Error('native_project_trust_required');
  }
  codexObservationConfig(reply.config);
  return reply.config;
}
function stableConfig(value: unknown): string {
  if (Array.isArray(value)) return '[' + value.map(stableConfig).join(',') + ']';
  if (object(value)) return '{' + Object.keys(value).sort().map(key => JSON.stringify(key) + ':' + stableConfig(value[key])).join(',') + '}';
  return JSON.stringify(value);
}
type NativeRPC = (method: string, params: unknown) => Promise<unknown>;
async function observeNativeCodex<T>(command: { executable: string; args: string[] }, cwd: string, env: NodeJS.ProcessEnv,
  action: (rpc: NativeRPC) => Promise<T>): Promise<T> {
  const child = spawn(command.executable, command.args, { cwd, env, detached: true, stdio: ['pipe', 'pipe', 'pipe'] });
  let ended = false, failed: Error | null = null, nextId = 0, buffer = '', total = 0;
  const pending = new Map<number, { resolve: (value: unknown) => void; reject: (error: Error) => void }>();
  const fail = () => { failed = new Error('native_owner_resolution_failed'); for (const item of pending.values()) item.reject(failed); pending.clear(); };
  const exited = new Promise<void>(resolve => { child.once('exit', () => { ended = true; fail(); resolve(); }); child.once('error', () => { ended = true; fail(); resolve(); }); });
  child.stdin.on('error', fail); child.stdout.on('error', fail); child.stderr.on('error', fail);
  child.stderr.resume();
  child.stdout.setEncoding('utf8');
  child.stdout.on('data', (chunk: string) => {
    total += Buffer.byteLength(chunk); buffer += chunk;
    if (total > 8 * 1024 * 1024 || Buffer.byteLength(buffer) > 2 * 1024 * 1024) { fail(); child.stdout.destroy(); return; }
    for (;;) {
      const at = buffer.indexOf('\n'); if (at < 0) break;
      const line = buffer.slice(0, at); buffer = buffer.slice(at + 1);
      try {
        const value: unknown = JSON.parse(line);
        if (!object(value)) throw new Error();
        if (value.id === undefined && typeof value.method === 'string') continue;
        if (typeof value.id !== 'number' || typeof value.method === 'string') throw new Error();
        const item = pending.get(value.id); if (!item) throw new Error();
        pending.delete(value.id);
        if (value.error !== undefined) item.reject(new Error('native_owner_resolution_failed'));
        else if (value.result === undefined) item.reject(new Error('native_owner_resolution_failed'));
        else item.resolve(value.result);
      } catch { fail(); }
    }
  });
  const rpc = (method: string, params: unknown) => new Promise<unknown>((resolve, reject) => {
    if (failed || ended) { reject(new Error('native_owner_resolution_failed')); return; }
    const id = ++nextId; pending.set(id, { resolve, reject });
    child.stdin.write(JSON.stringify({ id, method, params }) + '\n', error => { if (error) fail(); });
  });
  const deadline = setTimeout(fail, 20000);
  // The owning server cancels this helper with SIGTERM. Reject the pending RPC
  // so finally reaps the detached private app-server before this process exits.
  let interrupted = false;
  const cancel = () => { interrupted = true; fail(); };
  process.once('SIGTERM', cancel);
  process.once('SIGINT', cancel);
  try {
    await rpc('initialize', { clientInfo: { name: 'webmux-owner-plan', version: '1' }, capabilities: { experimentalApi: true } });
    child.stdin.write(JSON.stringify({ method: 'initialized' }) + '\n');
    return await action(rpc);
  } finally {
    clearTimeout(deadline);
    const kill = (signal: NodeJS.Signals) => { if (!ended && child.pid) { try { process.kill(-child.pid, signal); } catch { /* Child already exited. */ } } };
    kill('SIGTERM');
    let timer: ReturnType<typeof setTimeout> | undefined;
    await Promise.race([exited, new Promise<void>(resolve => { timer = setTimeout(() => { kill('SIGKILL'); resolve(); }, 2000); })]);
    if (timer) clearTimeout(timer);
    if (!ended) {
      await Promise.race([exited, new Promise<void>(resolve => { timer = setTimeout(resolve, 1000); })]);
      if (timer) clearTimeout(timer);
    }
    child.stdin.destroy(); child.stdout.destroy(); child.stderr.destroy();
    process.removeListener('SIGTERM', cancel);
    process.removeListener('SIGINT', cancel);
    if (!ended) throw new Error('native_owner_child_cleanup_unconfirmed');
    if (interrupted) throw new Error('native_owner_resolution_cancelled');
  }
}

export async function resolveCodexOwnerPlan(cwd: string, options: { profile?: string; executable?: string; env?: NodeJS.ProcessEnv } = {},
  dependencies: { command?: CodexObservationCommand } = {}): Promise<CodexOwnerPlan> {
  if (process.platform !== 'darwin' && process.platform !== 'linux') throw new Error('native_owner_plan_platform_unqualified');
  const env = { ...(options.env ?? process.env) };
  const profile = options.profile ?? null;
  if (profile !== null && !/^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$/.test(profile)) throw new Error('invalid_codex_profile');
  // 0.169 runtime commands accept -p, but app-server explicitly refuses it.
  // Do not claim that reading the default config resolves a selected profile.
  if (profile !== null) throw new Error('codex_profile_resolution_unsupported');
  cwd = fs.realpathSync(text(cwd, 4096));
  const home = fs.realpathSync(text(env.CODEX_HOME || path.join(env.HOME || os.homedir(), '.codex'), 4096));
  if (!fs.statSync(cwd).isDirectory() || !fs.statSync(home).isDirectory()) throw new Error('native_owner_plan_unavailable');
  const executable = executableAt(options.executable ?? 'codex', env);
  try {
    if (execFileSync(executable, ['--version'], { env, encoding: 'utf8', timeout: 5000, maxBuffer: 65536, stdio: ['ignore', 'pipe', 'ignore'] }).trim() !== 'codex-cli 0.169.0') throw new Error();
  } catch { throw new Error('qualified_codex_unavailable'); }
  const hash = createHash('sha256');
  if (fs.statSync(executable).size > 512 * 1024 * 1024) throw new Error('qualified_codex_unavailable');
  for await (const bytes of fs.createReadStream(executable, { signal: AbortSignal.timeout(5000) })) hash.update(bytes);
  const executableSHA256 = hash.digest('hex');
  env.CODEX_HOME = home;
  // Never join a caller's active Codex conversation or Rig enrollment.
  for (const name of Object.keys(env)) if (name.startsWith('OPENRIG_') || ['CODEX_THREAD_ID', 'TMUX', 'TMUX_PANE'].includes(name)) delete env[name];
  const command = dependencies.command ?? codexObservationCommand;
  // Disable startup integrations at process creation, including the no-thread
  // account stage; these feature flags never become the coding launch policy.
  const features = ['hooks', 'plugins', 'apps', 'shell_snapshot'];
  const args = ['-C', cwd, ...features.flatMap(name => ['-c', `features.${name}=false`]), 'app-server', '--listen', 'stdio://'];
  const policyCommand = command(executable, args, 'policy');
  const accountCommand = command(executable, args, 'account');
  // Workspace routing needs network even with refreshToken:false. This process
  // never starts a thread. Reap it before the guarded policy observer starts.
  const observed = await observeNativeCodex(accountCommand, cwd, env, async rpc => {
    const config = nativeConfig(await rpc('config/read', { cwd, includeLayers: true }));
    const account = await rpc('account/read', { refreshToken: false });
    return { configFingerprint: digest(stableConfig(config)), account };
  });
  return observeNativeCodex(policyCommand, cwd, env, async rpc => {
    const config = nativeConfig(await rpc('config/read', { cwd, includeLayers: true }));
    if (digest(stableConfig(config)) !== observed.configFingerprint) throw new Error('native_owner_config_changed');
    // Omitting cwd avoids the native explicit-cwd trust grant. The process has
    // the physical project cwd. OS restrictions cover integrations added after
    // this read, including names which enumeration could not disable.
    const thread = await rpc('thread/start', { ephemeral: true, config: codexObservationConfig(config) });
    return projectCodexOwnerPlan(config, thread, observed.account, { executable, executableSHA256, home, cwd, profile });
  });
}
