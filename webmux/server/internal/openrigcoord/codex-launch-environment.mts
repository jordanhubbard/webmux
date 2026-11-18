// Private launch state stores only standard path/locale values and hashes for
// other inherited values. Never copy arbitrary credentials to Rig metadata.
import path from 'node:path';
import fs from 'node:fs';
import { createHash } from 'node:crypto';

const standard = new Set(['CODEX_HOME', 'HOME', 'PATH', 'SHELL', 'USER', 'LOGNAME', 'TMPDIR', 'TMP', 'TEMP',
  'LANG', 'LC_ALL', 'LC_CTYPE', 'LC_COLLATE', 'LC_MESSAGES', 'LC_MONETARY', 'LC_NUMERIC', 'LC_TIME', 'TZ',
  'XDG_CONFIG_HOME', 'XDG_CACHE_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_RUNTIME_DIR']);
const terminal = new Set(['TMUX', 'TMUX_PANE', 'TMUX_TMPDIR', 'TERM', 'TERM_PROGRAM', 'TERM_PROGRAM_VERSION', 'COLORTERM', 'LINES', 'COLUMNS']);
const parentOnly = new Set(['PWD', 'OLDPWD', 'SHLVL', '_', 'CODEX_THREAD_ID', 'CODEX_SESSION_ID', 'CODEX_TOOL_CALL_ID']);
const fresh = (name: string) => name.startsWith('OPENRIG_') || terminal.has(name);
const omit = (name: string) => fresh(name) || parentOnly.has(name);
const hash = (value: string) => createHash('sha256').update(value).digest('hex');
const refuse = (): never => { throw new Error('codex_launch_environment_unavailable'); };
const row = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const key = (name: string) => /^[a-zA-Z_][a-zA-Z0-9_]{0,255}$/.test(name);
const text = (value: unknown): value is string => typeof value === 'string' && value.length <= 65536 && !value.includes('\0');
export interface CodexLaunchEnvironment {
  contract: 'webmux-codex-launch-environment/v1'; values: Record<string, string>;
  inheritedHashes: Record<string, string>; fingerprint: string;
}

export function captureCodexLaunchEnvironment(env: NodeJS.ProcessEnv, home: string): CodexLaunchEnvironment {
  if (!path.isAbsolute(home)) return refuse();
  const values: Array<[string, string]> = [], inherited: Array<[string, string]> = [];
  for (const name of Object.keys({ ...env, CODEX_HOME: home }).sort()) {
    const value = name === 'CODEX_HOME' ? home : env[name];
    if (value === undefined || omit(name)) continue;
    if (!key(name) || !text(value)) return refuse();
    (standard.has(name) ? values : inherited).push([name, standard.has(name) ? value : hash(value)]);
  }
  const content = { contract: 'webmux-codex-launch-environment/v1' as const,
    values: Object.fromEntries(values), inheritedHashes: Object.fromEntries(inherited) };
  const result = { ...content, fingerprint: hash(JSON.stringify(content)) };
  return validateCodexLaunchEnvironment(result, home);
}

export function validateCodexLaunchEnvironment(value: unknown, home: string): CodexLaunchEnvironment {
  if (!row(value) || value.contract !== 'webmux-codex-launch-environment/v1'
    || Object.keys(value).some(name => !['contract', 'values', 'inheritedHashes', 'fingerprint'].includes(name))
    || !row(value.values) || !row(value.inheritedHashes)) return refuse();
  const { fingerprint, ...content } = value;
  if (fingerprint !== hash(JSON.stringify(content)) || JSON.stringify(value).length > 196608
    || Object.keys(value.values).length + Object.keys(value.inheritedHashes).length > 512
    || value.values.CODEX_HOME !== home || typeof value.values.HOME !== 'string' || !path.isAbsolute(value.values.HOME)
    || typeof value.values.PATH !== 'string') return refuse();
  for (const [name, entry] of Object.entries(value.values)) if (!standard.has(name) || !text(entry)) return refuse();
  for (const [name, entry] of Object.entries(value.inheritedHashes)) {
    if (!key(name) || omit(name) || standard.has(name) || typeof entry !== 'string' || !/^[a-f0-9]{64}$/.test(entry)) return refuse();
  }
  return value as unknown as CodexLaunchEnvironment;
}

export function restoreCodexLaunchEnvironment(value: unknown, inherited: NodeJS.ProcessEnv, home: string, cwd: string, bindings = inherited): NodeJS.ProcessEnv {
  const snapshot = validateCodexLaunchEnvironment(value, home);
  const entries = new Map(Object.entries(snapshot.values));
  for (const [name, expected] of Object.entries(snapshot.inheritedHashes)) {
    const current = inherited[name];
    if (!text(current) || hash(current) !== expected) return refuse();
    entries.set(name, current);
  }
  // Manager-only variables, including credentials/routing, are excluded. These
  // live coordination and terminal bindings must identify the new native seat.
  for (const [name, entry] of Object.entries(bindings)) if (fresh(name) && entry !== undefined) entries.set(name, entry);
  entries.set('PWD', cwd);
  return Object.fromEntries(entries);
}

// Rig 0.7.18 places only its paired CLI in this first PATH directory. Retain
// that integration without inheriting the rest of the manager's tool PATH.
export function nativeRigToolDirectory(env: NodeJS.ProcessEnv): string {
  const home = env.OPENRIG_HOME, bin = env.PATH?.split(path.delimiter)[0];
  if (!home || !path.isAbsolute(home) || !bin || path.dirname(bin) !== path.join(home, 'run', 'seat-bin')
    || !/^[a-f0-9]{64}$/.test(path.basename(bin))) return refuse();
  const stat = fs.lstatSync(bin), names = fs.readdirSync(bin), link = path.join(bin, 'rig');
  if (!stat.isDirectory() || stat.uid !== process.geteuid?.() || (stat.mode & 0o022) !== 0
    || names.length !== 1 || names[0] !== 'rig' || !fs.lstatSync(link).isSymbolicLink()) return refuse();
  const cli = fs.realpathSync(link);
  if (hash(cli) !== path.basename(bin) || !fs.statSync(cli).isFile()) return refuse();
  fs.accessSync(cli, fs.constants.X_OK);
  return bin;
}
