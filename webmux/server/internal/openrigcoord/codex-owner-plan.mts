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

export async function resolveCodexOwnerPlan(cwd: string, options: { profile?: string; executable?: string; env?: NodeJS.ProcessEnv } = {}): Promise<CodexOwnerPlan> {
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
  const child = spawn(executable, ['-C', cwd, 'app-server', '--listen', 'stdio://'], { env, detached: true, stdio: ['pipe', 'pipe', 'pipe'] });
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
  try {
    await rpc('initialize', { clientInfo: { name: 'webmux-owner-plan', version: '1' }, capabilities: { experimentalApi: true } });
    child.stdin.write(JSON.stringify({ method: 'initialized' }) + '\n');
    const configReply = await rpc('config/read', { cwd, includeLayers: false });
    if (!object(configReply)) throw new Error('native_owner_resolution_failed');
    const account = await rpc('account/read', { refreshToken: false });
    const thread = await rpc('thread/start', { cwd, ephemeral: true });
    return projectCodexOwnerPlan(configReply.config, thread, account, { executable, executableSHA256, home, cwd, profile });
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
    if (!ended) throw new Error('native_owner_child_cleanup_unconfirmed');
  }
}
