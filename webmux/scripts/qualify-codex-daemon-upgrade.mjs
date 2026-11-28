#!/usr/bin/env node
// Private mixed-version protocol qualification. Never accepts a live CODEX_HOME/socket/thread.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import fsp from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import crypto from 'node:crypto';
import { spawn, execFile, execFileSync } from 'node:child_process';
import { parseArgs, promisify } from 'node:util';
const execute = promisify(execFile);

const { values: args } = parseArgs({ options: {
  old: { type: 'string' }, 'old-sha256': { type: 'string' },
  candidate: { type: 'string' }, 'candidate-sha256': { type: 'string' },
  output: { type: 'string' },
} });
for (const k of ['old', 'old-sha256', 'candidate', 'candidate-sha256', 'output']) assert(args[k]);
assert.equal(process.platform, 'darwin');
for (const key of ['old-sha256', 'candidate-sha256']) assert.match(args[key], /^[a-f0-9]{64}$/);
for (const key of ['old', 'candidate']) {
  args[key] = await fsp.realpath(args[key]);
  await fsp.access(args[key], fs.constants.X_OK);
}
const output = path.resolve(args.output);
await fsp.mkdir(output, { mode: 0o700 }); // Exclusive: never replay an earlier fixture.
const root = await fsp.mkdtemp('/private/tmp/wmup-');
await fsp.chmod(root, 0o700);
const home = path.join(root, 'home'), project = path.join(root, 'project');
await fsp.mkdir(home, { mode: 0o700 }); await fsp.mkdir(project, { mode: 0o700 });
const control = path.join(home, 'control'); await fsp.mkdir(control, { mode: 0o700 });
const socket = path.join(control, 's.sock');
const { authorizeSocket, connect } = await import('./probe-codex-task-tools.mts');
const children = [], requests = [], heldResponses = [], descriptors = [];
let channel, provider;
const receipt = { schema: 'webmux.private-daemon-upgrade.v1', startedUtc: new Date().toISOString(),
  status: 'running', fixtureRoot: root, actors: [], stages: [],
  scope: 'Private mixed-version native managed restart with synthetic local model responses. No real coding inference, original thread/request replay, live home/socket/credentials, installed process replacement or four-session continuity claim.',
  turnStartRequests: 0, successorClientConnectedAfterInference: false };
async function save() { await fsp.writeFile(path.join(output, 'receipt.json'), JSON.stringify(receipt, null, 2) + '\n', { mode: 0o600 }); }
async function stage(name, detail = {}) { receipt.stages.push({ name, utc: new Date().toISOString(), ...detail }); await save(); console.log(JSON.stringify({ stage: name, ...detail })); }
async function digest(file) { const h = crypto.createHash('sha256'); for await (const b of fs.createReadStream(file)) h.update(b); return h.digest('hex'); }
async function waitFor(predicate, label, timeout = 10000) {
  const until = Date.now() + timeout;
  while (Date.now() < until) { if (await predicate()) return; await new Promise(r => setTimeout(r, 25)); }
  throw new Error(`Deadline: ${label}`);
}
function settled(c) { return c.exitCode !== null || c.signalCode !== null; }
async function exit(c, label) { await waitFor(() => settled(c), label); assert.equal(c.exitCode, 0, `${label}: ${c.signalCode}`); }
function reply(response, body) { response.writeHead(200, { 'content-type': 'application/json' }); response.end(JSON.stringify(body)); }
function complete(index, text) {
  const response = heldResponses[index]; assert(response && !response.writableEnded);
  const events = [
    { type: 'response.created', response: { id: `r${index}` } },
    { type: 'response.output_item.done', item: { type: 'message', id: `m${index}`, role: 'assistant', content: [{ type: 'output_text', text }] } },
    { type: 'response.completed', response: { id: `r${index}`, usage: { input_tokens: 17, input_tokens_details: null, output_tokens: 3, output_tokens_details: null, total_tokens: 20 } } },
  ];
  response.end(events.map(e => `event: ${e.type}\ndata: ${JSON.stringify(e)}\n\n`).join(''));
}
async function start(binary, name, env) {
  const stdout = fs.openSync(path.join(output, name + '.stdout'), 'wx', 0o600);
  const stderr = fs.openSync(path.join(output, name + '.stderr'), 'wx', 0o600);
  descriptors.push(stdout, stderr);
  const c = spawn(binary, ['app-server', '--listen', 'unix://' + socket, '--managed-daemon'], { cwd: project, env, stdio: ['ignore', stdout, stderr] });
  c.on('error', error => { receipt.spawnError = { code: error.code, message: error.message }; });
  assert(c.pid); children.push(c);
  receipt.actors.push({ name, pid: c.pid, binary, startedUtc: new Date().toISOString() }); await save();
  const startupBegan = Date.now(); let sampled = false;
  await waitFor(async () => {
    if (!sampled && Date.now() - startupBegan > 2000) {
      sampled = true;
      try { await execute('/usr/bin/sample', [String(c.pid), '1', '1', '-file', path.join(output, name + '.sample.txt')], { timeout: 5000, maxBuffer: 8192 }); } catch {}
    }
    if (settled(c)) throw new Error(name + ' exited before private socket');
    try {
      if (!(await fsp.stat(socket)).isSocket()) return false;
      const peer = await execute('python3', ['-c', 'import socket,struct,sys; s=socket.socket(socket.AF_UNIX); s.settimeout(1); s.connect(sys.argv[1]); print(struct.unpack("=i",s.getsockopt(0,0x002,4))[0]); s.close()', socket], { timeout: 2000, maxBuffer: 1024 });
      const verified = Number(peer.stdout.trim());
      receipt.actors.find(a => a.pid === c.pid).socketPeerPid = verified;
      return verified === c.pid;
    } catch (error) { receipt.lastListenerProbeError = error.code ?? error.name; return false; }
  }, name + ' private listener');
  return c;
}
async function attach() { return connect(await authorizeSocket(socket)); }

await save();
try {
  receipt.qualifierSha256 = await digest(new URL(import.meta.url));
  receipt.oldSha256 = await digest(args.old); assert.equal(receipt.oldSha256, args['old-sha256']);
  receipt.candidateSha256 = await digest(args.candidate); assert.equal(receipt.candidateSha256, args['candidate-sha256']);
  receipt.rpcModuleSha256 = await digest(new URL('./probe-codex-task-tools.mts', import.meta.url));
  for (const f of [args.old, args.candidate]) execFileSync('/usr/bin/codesign', ['--verify', '--strict', f], { stdio: 'pipe' });
  await stage('exact-signed-images-verified');
  provider = http.createServer(async (request, response) => {
    if (request.method === 'GET' && request.url === '/v1/models') return reply(response, { data: [], object: 'list' });
    if (request.method === 'GET' && request.url === '/v1/responses') { response.writeHead(426); return response.end(); }
    if (request.method !== 'POST' || request.url !== '/v1/responses') { response.writeHead(404); return response.end(); }
    try {
      const chunks = []; let bytes = 0;
      for await (const b of request) { bytes += b.length; assert(bytes < 4 * 1024 * 1024); chunks.push(b); }
      const body = JSON.parse(Buffer.concat(chunks).toString()); requests.push(body);
      await fsp.writeFile(path.join(output, `synthetic-request-${requests.length}.json`), JSON.stringify(body, null, 2), { mode: 0o600 });
      if (requests.length > 2) { response.writeHead(500); return response.end('unexpected extra inference'); }
      response.writeHead(200, { 'content-type': 'text/event-stream', connection: 'close' }); response.flushHeaders();
      heldResponses.push(response);
    } catch (e) { receipt.providerError = String(e); response.destroy(); }
  });
  provider.listen(0, '127.0.0.1'); await new Promise(r => provider.once('listening', r));
  const config = 'model = "mock-model"\napproval_policy = "never"\nsandbox_mode = "read-only"\ncli_auth_credentials_store = "file"\ncheck_for_update_on_startup = false\nmodel_provider = "mock_provider"\n\n[model_providers.mock_provider]\nname = "Private upgrade fixture"\nbase_url = "http://127.0.0.1:' + provider.address().port + '/v1"\nwire_api = "responses"\nrequires_openai_auth = false\nrequest_max_retries = 0\nstream_max_retries = 0\n\n[features]\nenable_mcp_apps = false\ntool_suggest = false\nrecommended_plugins = false\n';
  await fsp.writeFile(path.join(home, 'config.toml'), config, { mode: 0o600 });
  receipt.configSha256Before = await digest(path.join(home, 'config.toml'));
  const env = { ...process.env, HOME: root, XDG_CONFIG_HOME: path.join(root, '.config'), CODEX_HOME: home,
    CODEX_DAEMON_SHUTDOWN_SOCKET: socket, CODEX_APP_SERVER_MANAGED_CONFIG_PATH: path.join(home, 'managed_config.toml') };
  for (const key of ['OPENAI_API_KEY', 'CODEX_API_KEY', 'CODEX_ACCESS_TOKEN', 'OPENAI_FEDERATION_RULE_ID', 'OPENAI_IDENTITY_TOKEN_FILE', 'OPENAI_WORKLOAD_IDENTITY_CONTEXT']) delete env[key];
  const old = await start(args.old, 'old', env); channel = await attach();
  const started = await channel.call('thread/start', { model: 'gpt-5.2', cwd: project,
    developerInstructions: 'Private session-only upgrade qualification.',
    dynamicTools: [{ name: 'lookup_ticket', description: 'Synthetic lookup', inputSchema: { type: 'object', properties: {} } }] });
  assert.equal(started.thread.cwd, project); assert.equal(started.approvalPolicy, 'never');
  const threadId = started.thread.id;
  const idle = await channel.call('thread/start', { cwd: project });
  const turn = await channel.call('turn/start', { threadId, input: [{ type: 'text', text: 'Private synthetic upgrade fixture.' }] });
  receipt.turnStartRequests++; receipt.threadId = threadId; receipt.initialTurnId = turn.turn.id; receipt.idleThreadId = idle.thread.id;
  await waitFor(() => requests.length === 1 && heldResponses.length === 1, 'initial synthetic inference', 5000);
  const goal = await channel.call('thread/goal/set', { threadId, objective: 'Continue private qualification after restart.', tokenBudget: 10000 });
  assert.equal(goal.goal.status, 'active'); assert.equal(goal.goal.tokenBudget, 10000);
  receipt.goalBeforeRestart = goal.goal;
  await stage('old-active-turn-and-goal-established');
  old.kill('SIGTERM');
  await new Promise(r => setTimeout(r, 250)); assert(!settled(old), 'Graceful stop must drain active synthetic turn');
  complete(0, 'Done'); await exit(old, 'old graceful drain'); channel.close(); channel = undefined;
  const recovery = JSON.parse(await fsp.readFile(path.join(home, 'app-server-daemon/loaded-threads.json'), 'utf8'));
  assert(recovery.includes(threadId) && recovery.includes(idle.thread.id));
  receipt.recoverySnapshot = recovery;
  receipt.oldSnapshotHasInterruptedTurn = recovery.some(x => typeof x === 'string' && x.startsWith('codex-interrupted-v1:'));
  receipt.oldTurnCompletionBeforeStopNotClaimed = receipt.oldSnapshotHasInterruptedTurn;
  await stage('old-exited-and-loaded-identities-persisted');
  const next = await start(args.candidate, 'successor', env);
  await waitFor(() => requests.length === 2 && heldResponses.length === 2, 'goal inference without successor client', 5000);
  assert.equal(requests[1].model, 'gpt-5.2');
  assert(JSON.stringify(requests[1].input).includes('Done'));
  assert(JSON.stringify(requests[1].tools).includes('lookup_ticket'));
  receipt.successorClientConnectedAfterInference = true;
  channel = await attach();
  const resumed = await channel.call('thread/resume', { threadId, excludeTurns: true });
  assert.equal(resumed.thread.id, threadId); assert.equal(resumed.thread.cwd, project);
  assert.equal(resumed.model, 'gpt-5.2'); assert.equal(resumed.approvalPolicy, 'never');
  const recoveredGoal = await channel.call('thread/goal/get', { threadId });
  assert.equal(recoveredGoal.goal.objective, goal.goal.objective); assert.equal(recoveredGoal.goal.status, 'active');
  receipt.goalAfterRestart = recoveredGoal.goal; await save();
  assert.equal(recoveredGoal.goal.tokenBudget, goal.goal.tokenBudget);
  assert.equal(recoveredGoal.goal.createdAt, goal.goal.createdAt);
  assert(recoveredGoal.goal.tokensUsed >= goal.goal.tokensUsed + 20, 'Initial synthetic usage must survive restart');
  const loaded = await channel.call('thread/loaded/list', {}); assert(loaded.data.includes(idle.thread.id));
  await channel.call('thread/goal/clear', { threadId });
  complete(1, 'Recovered');
  next.kill('SIGTERM'); await exit(next, 'successor graceful drain'); channel.close(); channel = undefined;
  assert.equal(requests.length, 2); assert.equal(receipt.turnStartRequests, 1);
  receipt.configSha256After = await digest(path.join(home, 'config.toml'));
  assert.equal(receipt.configSha256After, receipt.configSha256Before);
  receipt.status = 'passed'; await stage('mixed-version-goal-recovery-passed');
} catch (e) {
  receipt.status = 'failed'; receipt.error = { name: e.name, message: e.message };
} finally {
  channel?.close();
  for (const response of heldResponses) if (!response.writableEnded) response.destroy();
  for (const c of children) {
    if (!settled(c)) { c.kill('SIGKILL'); await waitFor(() => settled(c), 'owned fixture cleanup', 5000).catch(() => {}); }
    const a = receipt.actors.find(a => a.pid === c.pid); a.exitCode = c.exitCode; a.signalCode = c.signalCode; a.reaped = settled(c);
  }
  if (provider) { provider.closeAllConnections(); await new Promise(r => provider.close(r)); }
  for (const fd of descriptors) fs.closeSync(fd);
  receipt.finishedUtc = new Date().toISOString(); receipt.syntheticInferenceRequests = requests.length;
  receipt.allOwnedServerProcessesReaped = receipt.actors.every(a => a.reaped);
  await save(); console.log(JSON.stringify({ status: receipt.status, error: receipt.error, output, allOwnedServerProcessesReaped: receipt.allOwnedServerProcessesReaped }));
}
process.exitCode = receipt.status === 'passed' && receipt.allOwnedServerProcessesReaped ? 0 : 1;
