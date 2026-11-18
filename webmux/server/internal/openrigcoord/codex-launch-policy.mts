// Scoped to fresh WebMux-created Codex seats on the qualified Rig 0.7.18 path.
// This transforms launch arguments; it grants no creation or recovery authority.
import path from 'node:path';
import { createHash } from 'node:crypto';
import type { CodexOwnerPlan } from './codex-owner-plan.mts';

const fail = (): never => { throw new Error('codex_launch_policy_mismatch'); };
const hash = (value: unknown) => createHash('sha256').update(JSON.stringify(value)).digest('hex');
const absolute = (value: unknown): value is string => typeof value === 'string' && path.isAbsolute(value) && !/[\x00-\x1f\x7f]/.test(value);

export function validateCodexLaunchPlan(value: unknown): CodexOwnerPlan {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return fail();
  const plan = value as CodexOwnerPlan;
  const { fingerprint, ...content } = plan;
  if (plan.contract !== 'webmux-codex-owner-plan/v1' || plan.version !== '0.169.0'
    || fingerprint !== hash(content) || !absolute(plan.executable) || !absolute(plan.home) || !absolute(plan.cwd)
    || !/^[a-f0-9]{64}$/.test(plan.executableSHA256) || !/^[a-f0-9]{64}$/.test(plan.accountFingerprint)
    || plan.profile !== null || plan.provider !== 'openai' || typeof plan.model !== 'string' || !plan.model
    || plan.model.length > 512 || /[\x00-\x1f\x7f]/.test(plan.model)
    || (plan.effort !== null && (typeof plan.effort !== 'string' || !/^[a-z0-9_-]{1,32}$/.test(plan.effort)))
    || (plan.serviceTier !== null && (typeof plan.serviceTier !== 'string' || !/^[a-z0-9_-]{1,32}$/.test(plan.serviceTier)))
    || !['never', 'on-request'].includes(plan.approval) || !Array.isArray(plan.workspaceRoots)
    || !plan.workspaceRoots.every(absolute)) return fail();
  if (!plan.policy || typeof plan.policy !== 'object') return fail();
  if (plan.policy.type === 'dangerFullAccess') {
    if (plan.approval !== 'never' || Object.keys(plan.policy).length !== 1) return fail();
  } else if (plan.policy.type === 'readOnly') {
    // Rig's legacy read-only member schema cannot carry network-enabled policy.
    if (plan.policy.networkAccess !== false || Object.keys(plan.policy).length !== 2) return fail();
  } else if (plan.policy.type === 'workspaceWrite') {
    if (typeof plan.policy.networkAccess !== 'boolean' || typeof plan.policy.excludeSlashTmp !== 'boolean'
      || typeof plan.policy.excludeTmpdirEnvVar !== 'boolean' || !Array.isArray(plan.policy.writableRoots)
      || !plan.policy.writableRoots.every(absolute) || Object.keys(plan.policy).length !== 5) return fail();
  } else return fail();
  return plan;
}

export function filterRigCodexLaunch(value: unknown, args: string[]) {
  const plan = validateCodexLaunchPlan(value);
  if (!Array.isArray(args) || args.length > 128 || args.some(arg => typeof arg !== 'string' || arg.length > 65536 || /[\0\r\n]/.test(arg))) return fail();
  const expected = new Map<string, unknown>();
  if (plan.policy.type === 'workspaceWrite') {
    expected.set('sandbox_workspace_write.writable_roots', plan.policy.writableRoots);
    expected.set('sandbox_workspace_write.network_access', plan.policy.networkAccess);
    expected.set('sandbox_workspace_write.exclude_tmpdir_env_var', plan.policy.excludeTmpdirEnvVar);
    expected.set('sandbox_workspace_write.exclude_slash_tmp', plan.policy.excludeSlashTmp);
  }
  const sandbox = { dangerFullAccess: 'danger-full-access', readOnly: 'read-only', workspaceWrite: 'workspace-write' }[plan.policy.type];
  const seen = new Set<string>(), kept: string[] = [], removedDirectories: string[] = [];
  let removedLoginShellOverride = false, removedManagedInstructions = false;
  const once = (key: string) => { if (seen.has(key)) fail(); seen.add(key); };
  for (let i = 0; i < args.length; i++) {
    const flag = args[i];
    if (flag === '--no-daemon') { once(flag); kept.push(flag); continue; }
    const next = args[++i];
    if (next === undefined) return fail();
    if (flag === '--add-dir') {
      // The qualified member path receives owner roots in the explicit legacy
      // envelope, not --add-dir. Every fresh --add-dir here is Rig-generated.
      if (!absolute(next) || removedDirectories.includes(next)) return fail();
      removedDirectories.push(next); continue;
    }
    if (flag === '-s' || flag === '-a' || flag === '-C' || flag === '--model') {
      once(flag);
      const wanted = { '-s': sandbox, '-a': plan.approval, '-C': plan.cwd, '--model': plan.model }[flag];
      if (next !== wanted) return fail();
      kept.push(flag, next); continue;
    }
    if (flag !== '-c') return fail();
    const at = next.indexOf('=');
    if (at < 1) return fail();
    const key = next.slice(0, at), raw = next.slice(at + 1); once(key);
    if (key === 'allow_login_shell' && raw === 'false') { removedLoginShellOverride = true; continue; }
    if (key === 'developer_instructions') {
      let instructions: unknown;
      try { instructions = JSON.parse(raw); } catch { return fail(); }
      if (typeof instructions !== 'string' || !instructions.startsWith('OpenRig seat guidance for this session follows.')) return fail();
      // WebMux is using Rig for ownership. Preserve the owner's native developer
      // instructions instead of replacing them with an unrelated team role.
      removedManagedInstructions = true; continue;
    }
    if (key === 'check_for_update_on_startup' && raw === 'false') { kept.push(flag, next); continue; }
    let parsed: unknown;
    try { parsed = JSON.parse(raw); } catch { return fail(); }
    if (key === 'model_reasoning_effort' && parsed === plan.effort && typeof parsed === 'string') {
      kept.push(flag, next); continue;
    }
    if (!expected.has(key) || JSON.stringify(expected.get(key)) !== JSON.stringify(parsed)) return fail();
    kept.push(flag, next);
  }
  if (!['-s', '-a', '-C'].every(key => seen.has(key)) || [...expected.keys()].some(key => !seen.has(key))) return fail();
  // Freeze native model choices even when Rig omitted an argument and would
  // otherwise let a subsequently edited default configuration choose it.
  if (!seen.has('--model')) kept.push('--model', plan.model);
  if (!seen.has('model_reasoning_effort') && plan.effort !== null) kept.push('-c', `model_reasoning_effort=${JSON.stringify(plan.effort)}`);
  if (plan.serviceTier !== null) kept.push('-c', `service_tier=${JSON.stringify(plan.serviceTier)}`);
  return { executable: plan.executable, args: kept, removedDirectories, removedLoginShellOverride, removedManagedInstructions };
}
