import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { projectCodexOwnerPlan, resolveCodexOwnerPlan as nativeResolveCodexOwnerPlan, codexObservationCommand } from '../webmux/server/internal/openrigcoord/codex-owner-plan.mts';


// RPC protocol fixtures do not execute the native CLI. Native qualification
// separately exercises the default OS guard; unit fixtures inject transport.
const resolveCodexOwnerPlan = (cwd: string, options: Parameters<typeof nativeResolveCodexOwnerPlan>[1]) =>
  nativeResolveCodexOwnerPlan(cwd, options, { command: (executable, args) => ({ executable, args }) });

const identity = { executable: '/bin/codex', executableSHA256: 'a'.repeat(64), home: '/account', cwd: '/project', profile: null };
function snapshot() {
  return { config: { model: 'owner-model', secret: 'NEVER_EXPORT_CONFIG' },
    thread: { thread: { ephemeral: true }, cwd: '/project', model: 'owner-model', modelProvider: 'openai',
      reasoningEffort: 'medium', approvalsReviewer: 'user', approvalPolicy: 'never', sandbox: { type: 'dangerFullAccess' } } as Record<string, unknown>,
    account: { requiresOpenaiAuth: true, userId: 'private-user-id', account: { type: 'chatgpt', email: 'NEVER_EXPORT_EMAIL' },
      workspaceRouting: { chatgptAccountId: 'private-account-id', backendOrigin: 'https://example.invalid/codex', accountRoutingOverride: 'NO_CONSTRAINT' } } };
}
const project = (s: ReturnType<typeof snapshot>) => projectCodexOwnerPlan(s.config, s.thread, s.account, identity);
test('native policy projection keeps workspace roots, network and temporary-directory choices', () => {
  const s = snapshot();
  s.thread.sandbox = { type: 'workspaceWrite', writableRoots: ['/extra root'], networkAccess: false, excludeSlashTmp: true, excludeTmpdirEnvVar: false };
  s.thread.runtimeWorkspaceRoots = ['/project'];
  const plan = project(s);
  assert.deepEqual(plan.policy, s.thread.sandbox); assert.deepEqual(plan.workspaceRoots, ['/project']);
  assert.equal(plan.approval, 'never'); assert.equal(plan.model, 'owner-model');
});
test('read-only remains read-only with its native network choice', () => {
  for (const networkAccess of [false, true]) {
    const s = snapshot(); s.thread.sandbox = { type: 'readOnly', networkAccess };
    assert.deepEqual(project(s).policy, s.thread.sandbox);
  }
});
test('fingerprint binds account, policy and project; no raw account or config data escapes', () => {
  const s = snapshot(), before = project(s);
  s.account.workspaceRouting.chatgptAccountId = 'different-account';
  assert.notEqual(project(s).fingerprint, before.fingerprint);
  const output = JSON.stringify(before);
  for (const secret of ['private-user-id', 'private-account-id', 'NEVER_EXPORT_CONFIG', 'NEVER_EXPORT_EMAIL']) assert.equal(output.includes(secret), false);
  assert.equal(before.accountFingerprint.length, 64);
});
test('legacy summary cannot erase named, inherited or unknown permission rules', () => {
  const cases: Array<(s: ReturnType<typeof snapshot>) => void> = [
    s => { Object.assign(s.config, { default_permissions: 'restricted' }); },
    s => { s.thread.activePermissionProfile = { id: 'restricted' }; },
    s => { s.thread.activePermissionProfile = { id: ':workspace', extends: 'restricted' }; },
    s => { s.thread.sandbox = { type: 'readOnly', deniedPaths: ['/private'] }; },
    s => { s.thread.sandbox = { type: 'externalSandbox' }; },
    s => { s.thread.sandbox = { type: 'workspaceWrite', writableRoots: ['relative'] }; },
  ];
  for (const mutate of cases) { const s = snapshot(); mutate(s); assert.throws(() => project(s), /native_policy_not_representable/); }
});
test('unsupported approval, reviewer, provider, project or persisted thread refuses', () => {
  for (const update of [{ approvalPolicy: 'untrusted' }, { approvalPolicy: { granular: {} } },
    { approvalsReviewer: 'auto_review' }, { modelProvider: 'other' }, { cwd: '/other' }, { thread: { ephemeral: false } }]) {
    const s = snapshot(); Object.assign(s.thread, update); assert.throws(() => project(s), /native_owner_plan_unavailable/);
  }
});
test('missing native account ids refuse instead of falling back to email or display name', () => {
  const s = snapshot(); s.account.userId = '';
  assert.throws(() => project(s), /native_owner_plan_unavailable/);
  const other = snapshot(); other.account.account.type = 'apiKey';
  assert.throws(() => project(other), /native_account_identity_unavailable/);
});

const posixTest = process.platform === 'win32' ? test.skip : test;
const q = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
function fixture(t: { after: (fn: () => void) => void }, mode = '', config?: Record<string, unknown>) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'wm-owner-plan-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const home = path.join(root, 'home'), cwd = path.join(root, 'project'); fs.mkdirSync(home); fs.mkdirSync(cwd);
  const state = snapshot(); if (config) Object.assign(state.config, config); state.thread.cwd = fs.realpathSync(cwd);
  const data = path.join(root, 'data.json'), log = path.join(root, 'protocol.jsonl'), stopped = path.join(root, 'stopped');
  fs.writeFileSync(data, JSON.stringify(state));
  const script = path.join(root, 'native.mjs');
  fs.writeFileSync(script, `import fs from 'node:fs';import readline from 'node:readline';
    const mode=${JSON.stringify(mode)}, log=${JSON.stringify(log)};
    if(process.argv.includes('--version')){console.log('codex-cli '+(mode==='version'?'0.168.0':'0.169.0'));process.exit(0);}
    const data=JSON.parse(fs.readFileSync(${JSON.stringify(data)},'utf8'));
    fs.appendFileSync(log,JSON.stringify({pid:process.pid,args:process.argv.slice(2),cwd:process.cwd(),priorStopped:fs.existsSync(${JSON.stringify(stopped)})})+'\\n');
    process.on('SIGTERM',()=>{if(mode==='ignore-term')return;fs.writeFileSync(${JSON.stringify(stopped)},'stopped');process.exit(0);});
    process.stdout.on('error',()=>process.exit(0));
    if(mode==='exit')process.exit(3);
    console.error('NEVER_EXPORT_NATIVE_STDERR');
    readline.createInterface({input:process.stdin}).on('line',line=>{
      const request=JSON.parse(line);fs.appendFileSync(log,JSON.stringify(request)+'\\n');
      if(request.method==='initialized')return;
      if(mode==='hang')return;
      if(mode==='request'){console.log(JSON.stringify({id:999,method:'item/commandExecution/requestApproval'}));return;}
      if(mode==='oversize'){console.log('x'.repeat(3*1024*1024));return;}
      const result=request.method==='initialize'?{}:request.method==='config/read'?{config:data.config,layers:mode==='untrusted'?[{name:{type:'project'},version:'fixture',disabledReason:'untrusted',config:{private:'NEVER_EXPORT_LAYER'}}]:[]}:
        request.method==='account/read'?data.account:request.method==='thread/start'?data.thread:null;
      if(mode==='config-change'&&request.method==='account/read')fs.writeFileSync(${JSON.stringify(data)},JSON.stringify({...data,config:{...data.config,model:'changed'}}));
      console.log(JSON.stringify({id:request.id,result}));
    });`);
  const executable = path.join(root, 'codex'); fs.writeFileSync(executable, `#!/bin/sh\nexec ${q(process.execPath)} ${q(script)} "$@"\n`, { mode: 0o700 });
  return { executable, home, cwd, log, stopped, env: { ...process.env, CODEX_HOME: home } };
}
posixTest('native RPC sequence submits no turn and reaps its private process', async t => {
  const f = fixture(t); const plan = await resolveCodexOwnerPlan(f.cwd, f);
  const events = fs.readFileSync(f.log, 'utf8').trim().split('\n').map(line => JSON.parse(line) as Record<string, unknown>);
  assert.deepEqual(events.filter(e => e.method).map(e => e.method), ['initialize', 'initialized', 'config/read', 'account/read', 'initialize', 'initialized', 'config/read', 'thread/start']);
  assert.deepEqual(events.at(-1)?.params, { ephemeral: true, config: {
    'features.hooks': false, 'features.plugins': false, 'features.apps': false, 'features.shell_snapshot': false } });
  assert.deepEqual(events[0].args, ['-C', fs.realpathSync(f.cwd), ...['hooks', 'plugins', 'apps', 'shell_snapshot'].flatMap(name => ['-c', `features.${name}=false`]), 'app-server', '--listen', 'stdio://']);
  assert.equal(events[0].cwd, fs.realpathSync(f.cwd));
  const children = events.filter(e => typeof e.pid === 'number');
  assert.equal(children.length, 2); assert.equal(children[0].priorStopped, false); assert.equal(children[1].priorStopped, true);
  assert.notEqual(children[0].pid, children[1].pid);
  assert.equal(plan.profile, null); assert.equal(fs.existsSync(f.stopped), true);
  assert.equal(JSON.stringify(plan).includes('NEVER_EXPORT'), false);
});
posixTest('observation disables configured MCP startup without overriding owner policy', async t => {
  const f = fixture(t, '', { mcp_servers: { 'with.dot': { command: 'NEVER_RUN', args: ['--fixture'], env: { PRIVATE: 'NEVER_EXPORT_CONFIG' }, tools: null }, plain: { url: 'https://example.invalid', http_headers: { Authorization: 'NEVER_EXPORT_HEADER' } } } });
  const plan = await resolveCodexOwnerPlan(f.cwd, f);
  const events = fs.readFileSync(f.log, 'utf8').trim().split('\n').map(line => JSON.parse(line));
  const overrides = events.at(-1).params.config;
  assert.deepEqual(overrides, { 'features.hooks': false, 'features.plugins': false, 'features.apps': false,
    'features.shell_snapshot': false, mcp_servers: { 'with.dot': { command: 'NEVER_RUN', args: ['--fixture'], enabled: false }, plain: { url: 'https://example.invalid', enabled: false } } });
  assert.deepEqual(plan.policy, { type: 'dangerFullAccess' });
  assert.equal(JSON.stringify(plan).includes('NEVER_RUN'), false);
  assert.equal(JSON.stringify(overrides).includes('NEVER_EXPORT'), false);
});
posixTest('malformed or unbounded startup config refuses before thread creation', async t => {
  for (const servers of [[], 'invalid', { bad: null }, { missing: {} }, { invalidArgs: { command: 'never', args: [42] } }, { ['bad\nname']: {} },
    Object.fromEntries(Array.from({ length: 129 }, (_, i) => [String(i), {}]))]) {
    const f = fixture(t, '', { mcp_servers: servers });
    await assert.rejects(resolveCodexOwnerPlan(f.cwd, f), /native_startup_config_unavailable/);
    const lines = fs.readFileSync(f.log, 'utf8');
    assert.equal(lines.includes('thread/start'), false); assert.equal(fs.existsSync(f.stopped), true);
  }
});
posixTest('native approval requests are never answered or approved', async t => {
  const f = fixture(t, 'request');
  await assert.rejects(resolveCodexOwnerPlan(f.cwd, f), /native_owner_resolution_failed/);
  const lines = fs.readFileSync(f.log, 'utf8').trim().split('\n'); assert.equal(lines.length, 2);
  assert.equal(fs.existsSync(f.stopped), true);
});
posixTest('a server ignoring graceful shutdown is killed and reaped before returning', async t => {
  const f = fixture(t, 'ignore-term'); await resolveCodexOwnerPlan(f.cwd, f);
  const first = JSON.parse(fs.readFileSync(f.log, 'utf8').split('\n')[0]) as { pid: number };
  assert.throws(() => process.kill(first.pid, 0), (error: NodeJS.ErrnoException) => error.code === 'ESRCH');
});
posixTest('native exit or oversized output cannot become a successful owner plan', async t => {
  for (const mode of ['exit', 'oversize']) {
    const f = fixture(t, mode); await assert.rejects(resolveCodexOwnerPlan(f.cwd, f), /native_owner_resolution_failed/);
  }
});
posixTest('unknown CLI versions and profile traversal refuse before starting a server', async t => {
  const f = fixture(t, 'version'); await assert.rejects(resolveCodexOwnerPlan(f.cwd, f), /qualified_codex_unavailable/);
  assert.equal(fs.existsSync(f.log), false);
  const other = fixture(t); await assert.rejects(resolveCodexOwnerPlan(other.cwd, { ...other, profile: '../other' }), /invalid_codex_profile/);
  await assert.rejects(resolveCodexOwnerPlan(other.cwd, { ...other, profile: 'owner-profile' }), /codex_profile_resolution_unsupported/);
  assert.equal(fs.existsSync(other.log), false);
});
posixTest('a silent native server hits the resolution deadline and is reaped', async t => {
  const f = fixture(t, 'hang'), start = Date.now();
  await assert.rejects(resolveCodexOwnerPlan(f.cwd, f), /native_owner_resolution_failed/);
  assert.ok(Date.now() - start < 24000);
  assert.equal(fs.existsSync(f.stopped), true);
});

posixTest('server cancellation reaps the private app-server before the observer exits', async t => {
  const f = fixture(t, 'hang');
  const parentScript = path.join(path.dirname(f.home), 'parent.mjs');
  const module = new URL('../webmux/server/internal/openrigcoord/codex-owner-plan.mts', import.meta.url).href;
  fs.writeFileSync(parentScript, `import {resolveCodexOwnerPlan} from ${JSON.stringify(module)};try{await resolveCodexOwnerPlan(${JSON.stringify(f.cwd)},${JSON.stringify({ executable: f.executable })},{command:(executable,args)=>({executable,args})});}catch(e){console.log(e.message);process.exitCode=1;}`);
  const parent = spawn(process.execPath, [parentScript], { env: f.env, stdio: ['ignore', 'pipe', 'pipe'] });
  let nativePid = 0, output = ''; parent.stdout.on('data', data => { output += String(data); }); parent.stderr.resume();
  t.after(() => { parent.kill('SIGKILL'); if (nativePid && !fs.existsSync(f.stopped)) { try { process.kill(-nativePid, 'SIGKILL'); } catch { /* Already exited. */ } } });
  const exited = new Promise<number | null>(resolve => parent.once('close', resolve));
  const deadline = Date.now() + 5000;
  while (Date.now() < deadline) {
    const lines = fs.existsSync(f.log) ? fs.readFileSync(f.log, 'utf8') : '';
    if (lines.includes('"method":"initialize"')) { nativePid = JSON.parse(lines.split('\n')[0]).pid; break; }
    await new Promise(resolve => setTimeout(resolve, 20));
  }
  assert.ok(nativePid); parent.kill('SIGTERM');
  let timer: ReturnType<typeof setTimeout>;
  const status = await Promise.race([exited, new Promise<never>((_, reject) => { timer = setTimeout(() => reject(new Error('observer cancellation timed out')), 6000); })]).finally(() => clearTimeout(timer));
  assert.equal(status, 1); assert.match(output, /native_owner_resolution_cancelled/);
  assert.equal(fs.existsSync(f.stopped), true);
  assert.throws(() => process.kill(nativePid, 0), (error: NodeJS.ErrnoException) => error.code === 'ESRCH');
});

posixTest('untrusted project configuration refuses before account/thread start without granting trust', async t => {
  const f = fixture(t, 'untrusted');
  await assert.rejects(resolveCodexOwnerPlan(f.cwd, f), /native_project_trust_required/);
  const events = fs.readFileSync(f.log, 'utf8').trim().split('\n').map(line => JSON.parse(line));
  assert.equal(events.at(-1).method, 'config/read'); assert.equal(events.at(-1).params.includeLayers, true);
  assert.equal(events.some(e => e.method === 'thread/start' || e.method === 'account/read'), false);
  assert.equal(fs.existsSync(f.stopped), true);
});

posixTest('config changes between account and isolated policy observation refuse before a thread', async t => {
  const f = fixture(t, 'config-change');
  await assert.rejects(resolveCodexOwnerPlan(f.cwd, f), /native_owner_config_changed/);
  const events = fs.readFileSync(f.log, 'utf8');
  assert.equal(events.includes('thread/start'), false);
  for (const row of events.trim().split('\n').map(line => JSON.parse(line)).filter(row => row.pid)) {
    assert.throws(() => process.kill(row.pid, 0), (error: NodeJS.ErrnoException) => error.code === 'ESRCH');
  }
});

test('unqualified observation platforms refuse instead of silently starting an unguarded thread', () => {
  for (const platform of ['linux', 'win32', 'freebsd'] as const) {
    assert.throws(() => codexObservationCommand('/bin/codex', [], 'policy', platform), /isolation_unqualified/);
    assert.throws(() => codexObservationCommand('/bin/codex', [], 'account', platform), /isolation_unqualified/);
  }
});
const macTest = process.platform === 'darwin' ? test : test.skip;
macTest('macOS guards only policy startup and keeps the actual CLI arguments intact', () => {
  const executable = '/a/native "codex"', args = ['-C', '/project', 'app-server', '--listen', 'stdio://'];
  assert.deepEqual(codexObservationCommand(executable, args, 'account'), { executable, args });
  const guarded = codexObservationCommand(executable, args, 'policy');
  assert.equal(guarded.executable, '/usr/bin/sandbox-exec');
  assert.deepEqual(guarded.args.slice(2), [executable, ...args]);
  assert.match(guarded.args[1], /deny network\*/); assert.match(guarded.args[1], /deny process-fork/);
  assert.ok(guarded.args[1].includes('(literal ' + JSON.stringify(executable) + ')'));
  assert.throws(() => codexObservationCommand('/a/line\nbreak', args, 'policy'), /native_owner_plan_unavailable/);
});

posixTest('Linux helper provenance is pinned before either account or policy startup', t => {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'wm-linux-observer-')));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const executable = path.join(root, 'helper'); fs.writeFileSync(executable, 'synthetic helper metadata fixture', { mode: 0o700 });
  const helper = { contract: 'webmux-native-codex-observer/v1' as const, executable,
    executableSHA256: createHash('sha256').update(fs.readFileSync(executable)).digest('hex') };
  const args = ['-C', '/project', 'app-server', '--listen', 'stdio://'];
  assert.deepEqual(codexObservationCommand('/native/codex', args, 'policy', 'linux', helper, 'a'.repeat(64)), {
    executable, args: ['--native-codex-policy-observer', helper.executableSHA256, '/native/codex', 'a'.repeat(64), ...args],
  });
  assert.deepEqual(codexObservationCommand('/native/codex', args, 'account', 'linux', helper), { executable: '/native/codex', args });
  fs.appendFileSync(executable, 'changed');
  for (const stage of ['account', 'policy'] as const) assert.throws(() => codexObservationCommand('/native/codex', args, stage, 'linux', helper), /helper_changed/);
});
