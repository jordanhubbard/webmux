// Runs only after the owner explicitly chooses Run live test. No provider
// settings, daemon setup, existing teams, or normal terminal sessions are changed.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { spawn } from 'node:child_process';
import assert from 'node:assert/strict';

const [action, directory, team] = process.argv.slice(2) as [string, string, string];
type Proof = { run_id: string; nonce_a: string; nonce_b: string; digest: string; verified: boolean; runtime: string; peer: string };
type State = { team: string; alpha: string; beta: string; phase: string; run_id: string; nonce_a: string; digest: string; message: string; nonce_b?: string; updated_at?: string };
const messageOf = (error: unknown) => error instanceof Error ? error.message : String(error);
assert(['run', 'stop'].includes(action) && path.isAbsolute(directory));
assert(/^webmux-proof-[a-f0-9]{24}$/.test(team));
const alpha = `proof-alpha@${team}`;
const beta = `proof-beta@${team}`;
const statusFile = path.join(directory, 'status.json');
const stopFile = path.join(directory, 'stop-requested');
const lockFile = path.join(directory, 'run.lock');
let state: State = { team, alpha, beta, phase: 'preparing', run_id: '', nonce_a: '', digest: '', message: '' };
function publish(phase: string, message: string, extra: Partial<State> = {}) {
  state = { ...state, ...extra, phase, message, updated_at: new Date().toISOString() };
  const file = `${statusFile}.${process.pid}.tmp`;
  fs.writeFileSync(file, JSON.stringify(state, null, 2) + '\n', { mode: 0o600 });
  fs.renameSync(file, statusFile);
  console.log(`${phase.toUpperCase()}: ${message}`);
}
function command(program: string, args: string[], timeout = 120000): Promise<string> {
  return new Promise((resolve, reject) => {
    const child = spawn(program, args, { cwd: directory, env: process.env, stdio: ['ignore', 'pipe', 'pipe'] });
    let output = '';
    const collect = (data: Buffer) => { output = (output + data.toString()).slice(-16000); };
    child.stdout.on('data', collect);
    child.stderr.on('data', collect);
    const timer = setTimeout(() => child.kill('SIGTERM'), timeout);
    const force = setTimeout(() => child.kill('SIGKILL'), timeout + 5000);
    child.on('error', error => { clearTimeout(timer); clearTimeout(force); reject(error); });
    child.on('close', code => {
      clearTimeout(timer); clearTimeout(force);
      if (code === 0) resolve(output);
      else reject(new Error(`${program} ${args[0]} failed: ${output || `exit ${code}`}`));
    });
  });
}
async function down() { return command('rig', ['down', team], 30000); }
async function pauseUntilChange() { await new Promise(resolve => setTimeout(resolve, 500)); }
function readJSON<T>(file: string): T | null { try { return JSON.parse(fs.readFileSync(file, 'utf8')) as T; } catch { return null; } }
function write(file: string, data: string) { fs.writeFileSync(path.join(directory, file), data, { mode: 0o600 }); }

fs.mkdirSync(directory, { recursive: true, mode: 0o700 });
if (action === 'stop') {
  write('stop-requested', 'stop\n');
  state = { ...state, ...(readJSON<State>(statusFile) ?? {}) };
  try { console.log(await down()); publish('stopped', 'The demo pair is stopped. Other teams and SSH/tmux sessions are unchanged.'); }
  catch (error) { publish('failed', `Could not confirm demo shutdown: ${messageOf(error)}`); process.exitCode = 1; }
} else {
  let ownsLock = false;
  try {
    if (fs.existsSync(lockFile)) {
      const pid = Number(fs.readFileSync(lockFile, 'utf8'));
      let alive = true;
      try { process.kill(pid, 0); } catch (error) { if ((error as NodeJS.ErrnoException).code === 'ESRCH') alive = false; }
      if (alive) throw new Error('A live test is already running. Return to its console or stop the demo pair first.');
      fs.unlinkSync(lockFile);
    }
    fs.writeFileSync(lockFile, String(process.pid), { flag: 'wx', mode: 0o600 });
    ownsLock = true;
    fs.rmSync(stopFile, { force: true });
    const run = `webmux-live-${crypto.randomBytes(8).toString('hex')}`;
    const nonce = crypto.randomBytes(24).toString('hex');
    state = { ...state, run_id: run, nonce_a: nonce };
    console.log('WebMux owner-visible OpenRig acceptance test');
    console.log(`Claude: ${alpha}\nCodex:  ${beta}\nRun: ${run}`);
    console.log('Watch both agent tabs. Closing a browser view does not stop the pair. Use Stop test agents when finished.');
    for (const seat of ['alpha', 'beta']) {
      fs.mkdirSync(path.join(directory, seat), { recursive: true, mode: 0o700 });
      if (!fs.existsSync(path.join(directory, seat, '.git'))) await command('git', ['init', '-q', path.join(directory, seat)]);
      fs.rmSync(path.join(directory, seat, `${seat}-proof.json`), { force: true });
    }
    fs.mkdirSync(path.join(directory, 'agent'), { recursive: true, mode: 0o700 });
    write('agent/agent.yaml', JSON.stringify({ name: 'webmux-owner-proof', version: '1.0', description: 'Bounded owner-initiated coordination acceptance', profiles: { default: { uses: { skills: [], guidance: [], subagents: [], plugins: [], runtime_resources: [] } } }, startup: { files: [{ path: 'role.md', delivery_hint: 'send_text', required: true }] } }));
    write('agent/role.md', 'You are in an owner-initiated WebMux coordination test. Reply READY, then wait for the challenge. Only use rig whoami, rig send, Python and files in your current working directory. Do not read peer files or processes, launch agents, change settings or do unrelated work. Follow the bounded challenge/response/ACK protocol and stay idle afterward.\n');
    write('rig.yaml', JSON.stringify({ version: '0.2', name: team, summary: 'Owner-visible Claude/Codex coordination test', pods: [{ id: 'proof', label: 'WebMux live test', members: [{ id: 'alpha', agent_ref: 'local:agent', profile: 'default', runtime: 'claude-code', cwd: 'alpha' }, { id: 'beta', agent_ref: 'local:agent', profile: 'default', runtime: 'codex', cwd: 'beta' }], edges: [{ kind: 'collaborates_with', from: 'alpha', to: 'beta' }] }], edges: [] }));
    publish('preparing', 'Checking the named demo pair. Provider authentication and local daemon networking must already be configured.');
    let existing = false;
    try {
      await command('tmux', ['has-session', '-t', `=${alpha}`], 3000);
      await command('tmux', ['has-session', '-t', `=${beta}`], 3000);
      existing = true;
    } catch { /* A fresh pair is started only on this explicit owner action. */ }
    if (!existing) console.log(await command('rig', ['up', path.join(directory, 'rig.yaml'), '--yes', '--json']));
    if (fs.existsSync(stopFile)) { await down(); publish('stopped', 'Demo stopped before sending a challenge.'); }
    else {
      const task = `Owner-authorized bounded coordination test. You are ALPHA (Claude Code). Verify identity with rig whoami --json. Run id ${run}; nonce_a ${nonce}. Send only through rig send to ${beta} these instructions: Generate a fresh nonce_b using Python secrets.token_hex(24), compute SHA256 hex of UTF-8 "webmux-openrig-v1:${run}:${nonce}:<nonce_b>", then reply through rig send to ${alpha} with JSON type=response, run_id, nonce_a, nonce_b, digest. Await ALPHA's matching ACK; independently verify the digest and then write beta-proof.json in your own cwd with those fields, verified=true, runtime="codex", peer="${alpha}". Stay idle. Do not inspect ALPHA files/processes. ALPHA: when BETA responds, independently verify the hash and matching run/nonce using Python. Write alpha-proof.json in your own cwd with run_id, nonce_a, nonce_b, digest, verified=true, runtime="claude-code", peer="${beta}". Then send BETA an ACK through rig send with JSON type=ack, run_id, digest and stay idle. Do not use shared files, --from, direct HTTP or tmux injection as a messaging substitute. Report a blocker instead of a pass. The owner is watching both terminals.`;
      write('controller-task.txt', task);
      console.log(await command('rig', ['send', alpha, task, '--json'], 30000));
      publish('waiting', 'Challenge sent only to Claude. Waiting for Codex response and Claude ACK; inspect both agent tabs.');
      const deadline = Date.now() + 180000;
      let completed = false;
      while (Date.now() < deadline) {
        if (fs.existsSync(stopFile)) { await down(); publish('stopped', 'The owner stopped the demo pair.'); completed = true; break; }
        const a = readJSON<Proof>(path.join(directory, 'alpha/alpha-proof.json'));
        const b = readJSON<Proof>(path.join(directory, 'beta/beta-proof.json'));
        if (a && b) {
          const digest = crypto.createHash('sha256').update(`webmux-openrig-v1:${run}:${nonce}:${b.nonce_b}`).digest('hex');
          for (const proof of [a, b]) {
            assert.equal(proof.run_id, run); assert.equal(proof.nonce_a, nonce); assert.equal(proof.nonce_b, b.nonce_b);
            assert.match(proof.nonce_b, /^[a-f0-9]{48}$/); assert.notEqual(proof.nonce_b, nonce);
            assert.equal(proof.digest, digest); assert.equal(proof.verified, true);
          }
          assert.equal(a.runtime, 'claude-code'); assert.equal(a.peer, beta);
          assert.equal(b.runtime, 'codex'); assert.equal(b.peer, alpha);
          for (const [seat, target] of [['alpha', alpha], ['beta', beta]]) {
            write(`${seat}-transcript.txt`, await command('rig', ['capture', target], 10000));
          }
          const result = { run_id: run, nonce_a: nonce, nonce_b: b.nonce_b, digest, alpha: a, beta: b };
          write('result.json', JSON.stringify(result, null, 2));
          fs.mkdirSync(path.join(directory, 'runs'), { recursive: true, mode: 0o700 });
          write(`runs/${run}.json`, JSON.stringify(result, null, 2));
          publish('passed', 'PASS: Claude → Codex challenge, Codex → Claude response, Claude → Codex ACK; both digests independently match.', { digest, nonce_b: b.nonce_b });
          console.log(`SHA-256: ${digest}\nRun fresh challenge to repeat. The demo pair remains visible until Stop test agents.`);
          completed = true; break;
        }
        await pauseUntilChange();
      }
      if (!completed) throw new Error('No verified response and ACK within three minutes. Inspect both agent tabs for authentication, permission or networking blockers. No PASS was recorded.');
    }
  } catch (error) {
    if (ownsLock) publish('failed', messageOf(error));
    else console.error(messageOf(error));
    process.exitCode = 1;
  } finally { if (ownsLock) fs.rmSync(lockFile, { force: true }); }
}
