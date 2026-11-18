import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { projectCodexOwnerPlan, resolveCodexOwnerPlan } from '../webmux/server/internal/openrigcoord/codex-owner-plan.mts';

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
function fixture(t: { after: (fn: () => void) => void }, mode = '') {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'wm-owner-plan-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const home = path.join(root, 'home'), cwd = path.join(root, 'project'); fs.mkdirSync(home); fs.mkdirSync(cwd);
  const state = snapshot(); state.thread.cwd = fs.realpathSync(cwd);
  const data = path.join(root, 'data.json'), log = path.join(root, 'protocol.jsonl'), stopped = path.join(root, 'stopped');
  fs.writeFileSync(data, JSON.stringify(state));
  const script = path.join(root, 'native.mjs');
  fs.writeFileSync(script, `import fs from 'node:fs';import readline from 'node:readline';
    const mode=${JSON.stringify(mode)}, log=${JSON.stringify(log)};
    if(process.argv.includes('--version')){console.log('codex-cli '+(mode==='version'?'0.168.0':'0.169.0'));process.exit(0);}
    const data=JSON.parse(fs.readFileSync(${JSON.stringify(data)},'utf8'));
    fs.appendFileSync(log,JSON.stringify({pid:process.pid,args:process.argv.slice(2)})+'\\n');
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
      const result=request.method==='initialize'?{}:request.method==='config/read'?{config:data.config}:
        request.method==='account/read'?data.account:request.method==='thread/start'?data.thread:null;
      console.log(JSON.stringify({id:request.id,result}));
    });`);
  const executable = path.join(root, 'codex'); fs.writeFileSync(executable, `#!/bin/sh\nexec ${q(process.execPath)} ${q(script)} "$@"\n`, { mode: 0o700 });
  return { executable, home, cwd, log, stopped, env: { ...process.env, CODEX_HOME: home } };
}
posixTest('native RPC sequence submits no turn and reaps its private process', async t => {
  const f = fixture(t); const plan = await resolveCodexOwnerPlan(f.cwd, f);
  const events = fs.readFileSync(f.log, 'utf8').trim().split('\n').map(line => JSON.parse(line) as Record<string, unknown>);
  assert.deepEqual(events.slice(1).map(e => e.method), ['initialize', 'initialized', 'config/read', 'account/read', 'thread/start']);
  assert.deepEqual(events.at(-1)?.params, { cwd: fs.realpathSync(f.cwd), ephemeral: true });
  assert.deepEqual(events[0].args, ['-C', fs.realpathSync(f.cwd), 'app-server', '--listen', 'stdio://']);
  assert.equal(plan.profile, null); assert.equal(fs.existsSync(f.stopped), true);
  assert.equal(JSON.stringify(plan).includes('NEVER_EXPORT'), false);
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
