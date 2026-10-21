// WebMux owns the control plane; SSH carries each remote CLI's terminal and a
// loopback-only reverse forward. No remote daemon or provider credentials copied.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';

type Agent = { target: string; project: string; name: string; host: string; cwd: string; runtime: string; tmux: string; remotePort?: number };
type Input = { project?: string; name?: string; host?: string; cwd?: string; runtime?: string };
const [action, directory] = process.argv.slice(2);
if (!directory) throw new Error('Missing coordination state directory');
fs.mkdirSync(directory, { recursive: true, mode: 0o700 });
const q = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
const run = (cmd: string, args: string[], timeout = 25000) => execFileSync(cmd, args, { encoding: 'utf8', timeout, stdio: ['pipe', 'pipe', 'pipe'], maxBuffer: 4 * 1024 * 1024 }).trim();
const write = (file: string, data: unknown) => {
  const temporary = `${file}.${process.pid}.tmp`;
  fs.writeFileSync(temporary, typeof data === 'string' ? data : JSON.stringify(data), { mode: 0o600 });
  fs.renameSync(temporary, file);
};
const records = (): Agent[] => fs.readdirSync(directory).filter(f => f.endsWith('.agent.json')).map(f => JSON.parse(fs.readFileSync(path.join(directory, f), 'utf8')) as Agent);
const identifier = (value: string, field: string) => {
  if (!/^[a-z][a-z0-9-]{0,47}$/.test(value)) throw new Error(`${field}: use a letter followed by letters, digits or hyphens (48 characters maximum).`);
  return value;
};
const sshArgs = ['-o', 'BatchMode=yes', '-o', 'ConnectTimeout=8', '-o', 'ServerAliveInterval=15', '-o', 'ServerAliveCountMax=3'];
try {
  if (action === 'ensure') {
    // start is idempotent and leaves an existing daemon and its teams intact.
    const state = path.join(directory, 'coordinator.json');
    let port = fs.existsSync(state) ? (JSON.parse(fs.readFileSync(state, 'utf8')) as { port: number }).port : 4242;
    let healthy = false;
    try { const match = run('rig', ['daemon', 'status']).match(/running on port (\d+)/); if (match) { port = Number(match[1]); healthy = (await fetch(`http://127.0.0.1:${port}/healthz`, { signal: AbortSignal.timeout(3000) })).ok; } } catch { /* Restart at the saved port. */ }
    if (!Number.isInteger(port) || port < 1024 || port > 65535) throw new Error('Invalid saved coordinator port');
    write(state, { port });
    if (healthy) console.log(JSON.stringify({ running: true, port }));
    else console.log(run('rig', ['daemon', 'start', '--host', '127.0.0.1', '--port', String(port), '--no-kernel']));
  } else if (action === 'environment') {
    const tmux = process.argv[4] ?? '';
    if (!/^wm-[a-z0-9-]+$/.test(tmux)) throw new Error('Invalid managed transport');
    const attached = JSON.parse(fs.readFileSync(path.join(directory, `${tmux}.attached.json`), 'utf8')) as { attachmentType?: string; env?: Record<string, unknown> };
    if (attached.attachmentType !== 'tmux' || typeof attached.env?.OPENRIG_URL !== 'string' || attached.env?.OPENRIG_SESSION_NAME !== tmux) throw new Error('Rig did not bind the expected terminal');
    const keys = ['OPENRIG_NODE_ID', 'OPENRIG_SESSION_NAME', 'OPENRIG_RUNTIME', 'OPENRIG_URL', 'OPENRIG_ACTIVITY_HOOK_TOKEN'];
    write(path.join(directory, `${tmux}.env`), keys.filter(key => typeof attached.env?.[key] === 'string').map(key => `export ${key}=${q(String(attached.env?.[key]))}`).join('\n') + '\n');
  } else if (action === 'status') {
    const raw = JSON.parse(run('rig', ['ps', '--nodes', '-A', '--fields', 'canonicalSessionName,runtime,rigName,sessionStatus', '--json'])) as Record<string, unknown>[] | { entries?: Record<string, unknown>[] };
    const entries = Array.isArray(raw) ? raw : raw.entries ?? [];
    const agents = records().map(agent => ({ ...agent, ...(entries.find(e => e.canonicalSessionName === agent.target) ?? {}), connected: (() => { try { run('tmux', ['has-session', '-t', `=${agent.tmux}`], 3000); return true; } catch { return false; } })() }));
    const messages: { from: string; to: string; body: string; time: string; state: string }[] = [];
    for (const agent of agents.slice(0, 16)) {
      const rows = JSON.parse(run('rig', ['queue', 'outbox-list', agent.target, '--limit', '8', '--json'])) as { senderSession: string; destinationSession: string; body: string; tsDispatched: string; deliveryState: string }[];
      for (const row of rows) messages.push({ from: row.senderSession, to: row.destinationSession, body: row.body.slice(0, 2000), time: row.tsDispatched, state: row.deliveryState });
    }
    messages.sort((a, b) => b.time.localeCompare(a.time));
    console.log(JSON.stringify({ agents, messages: messages.slice(0, 24) }));
  } else if (action === 'create') {
    const input = JSON.parse(fs.readFileSync(0, 'utf8')) as Input;
    const project = identifier(input.project ?? '', 'Project');
    const name = identifier(input.name ?? '', 'Agent name');
    const host = input.host?.trim() || 'localhost';
    if (!/^(?:[a-zA-Z0-9_][a-zA-Z0-9_.-]*@)?[a-zA-Z0-9_][a-zA-Z0-9_.-]*$/.test(host)) throw new Error('Host must be localhost or a configured OpenSSH host name (optionally user@host).');
    const cwd = input.cwd ?? '';
    if (!cwd.startsWith('/') || /[\x00\r\n]/.test(cwd) || cwd.length > 2048) throw new Error('Choose an absolute project directory on the execution host.');
    const mapping = records().find(agent => agent.project === project && agent.host === host);
    if (mapping && mapping.cwd !== cwd) throw new Error(`Project ${project} already maps ${host} to ${mapping.cwd}. Use that directory or a different project ID.`);
    const runtime = input.runtime ?? 'codex';
    if (!['codex', 'claude-code'].includes(runtime)) throw new Error('Unsupported agent runtime.');
    const local = host === 'localhost';
    const identityFile = path.join(directory, 'identity.json');
    const scope = fs.existsSync(identityFile) ? (JSON.parse(fs.readFileSync(identityFile, 'utf8')) as { scope: string }).scope : crypto.createHash('sha256').update(directory).digest('hex').slice(0, 10);
    if (!/^[a-f0-9]{10}$/.test(scope)) throw new Error('Invalid saved coordinator identity');
    write(identityFile, { scope });
    const rig = `webmux-${scope}-${project}`;
    const member = `${name}-${crypto.createHash('sha256').update(host).digest('hex').slice(0, 8)}`;
    const tmux = `wm-${scope}-${project}-${member}`;
    const target = tmux;
    const recordFile = path.join(directory, `${tmux}.agent.json`);
    const existing = records().find(a => a.target === target);
    let alive = false;
    try { run('tmux', ['has-session', '-t', `=${tmux}`], 3000); alive = true; } catch { /* A stopped relay can be explicitly reconnected. */ }
    if (existing && (existing.cwd !== cwd || existing.runtime !== runtime)) throw new Error('This agent has a different directory or runtime. Choose another name.');
    if (existing && alive) {
      if (existing.cwd !== cwd || existing.runtime !== runtime) throw new Error('This agent already exists with a different directory or runtime. Choose another name.');
      run('tmux', ['has-session', '-t', `=${tmux}`], 3000);
      console.log(JSON.stringify(existing));
    } else {
      // Validate the actual execution host before allocating a central seat.
      const probe = `export PATH="$HOME/.local/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin:$PATH"; cd ${q(cwd)} && command -v ${runtime === 'codex' ? 'codex' : 'claude'} && command -v rig${local ? '' : ' && command -v tmux'}`;
      if (local) run('/bin/sh', ['-c', probe]); else run('ssh', [...sshArgs, host, probe]);
      const projectDir = path.join(directory, project);
      fs.mkdirSync(projectDir, { recursive: true, mode: 0o700 });
      const spec = path.join(projectDir, 'rig.yaml');
      const projectState = path.join(projectDir, 'project.json');
      if (!fs.existsSync(projectState)) {
        write(spec, { version: '0.2', name: rig, summary: `WebMux project ${project}`, pods: [{ id: 'agents', label: project, members: [] }], edges: [] });
        const result = JSON.parse(run('rig', ['up', spec, '--yes', '--json'])) as { rigId?: string };
        if (!result.rigId) throw new Error('Coordinator did not create the project.');
        write(projectState, result);
      }
      // attach-self binds the real central tmux pane. It remains the delivery
      // transport even when its foreground process is SSH to another machine.
      const attached = path.join(directory, `${tmux}.attached.json`);
      const launcher = path.join(directory, `${tmux}.sh`);
      const role = `You are ${name}, a WebMux coordinated agent for project ${project}, executing on ${host} in ${cwd}. OpenRig is already connected through OPENRIG_URL and OPENRIG_SESSION_NAME. Run rig whoami --json and rig ps --nodes -A --json to discover teammates. Use rig send <canonicalSessionName> <message> for coordination; do not override sender identity. Project identity is shared, files are host-local. Keep the owner's project instructions and permission policy. Report READY and wait for the owner or a teammate. Do not launch other agents automatically.`;
      const cli = runtime === 'codex' ? `codex -c sandbox_workspace_write.network_access=true ${q(role)}` : `claude --append-system-prompt ${q(role)} ${q('Verify your coordination identity, discover teammates, report READY and wait.')}`;
      const rigId = (JSON.parse(fs.readFileSync(projectState, 'utf8')) as { rigId: string }).rigId;
      const attach = `rig attach --self --rig ${q(rigId)} ${existing ? `--node ${q('agents.' + member)}` : `--pod agents --member ${q(member)}`} --runtime ${q(runtime)} --cwd ${q(cwd)} --json`;
      // Get the actual daemon URL from the supported attach response, not a
      // guessed service port. Node writes a shell-quoted env file from JSON.
      const envFile = path.join(directory, `${tmux}.env`);
      const remotePort = local ? 0 : existing?.remotePort ?? Number(run('ssh', [...sshArgs, host, "python3 -c 'import socket; s=socket.socket(); s.bind((\"127.0.0.1\",0)); print(s.getsockname()[1])'"]));
      if (!local && (!Number.isInteger(remotePort) || remotePort < 1024 || remotePort > 65535)) throw new Error('Cannot allocate a private SSH reverse forward.');
      const remoteAgent = `export PATH="$HOME/.local/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin:$PATH"; export OPENRIG_URL=http://127.0.0.1:${remotePort}; export OPENRIG_SESSION_NAME=${q(target)}; cd ${q(cwd)} && exec ${cli}`;
      const remoteRun = `export PATH="$HOME/.local/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin:$PATH"; exec tmux new-session -A -s ${q(tmux)} ${q('/bin/sh -c ' + q(remoteAgent))}`;
      // Pass only routing identity through the SSH channel; credentials and
      // provider homes stay on the execution host. Never forward auth tokens.
      const endpoint = `node -e 'const u=new URL(process.env.OPENRIG_URL); if(u.hostname!=="127.0.0.1"&&u.hostname!=="localhost") throw Error("Coordinator must be loopback-only"); console.log(u.port||80)'`;
      const command = local ? `cd ${q(cwd)}\nexec ${cli}` : `PORT=$(${endpoint})\nexec ssh ${sshArgs.map(q).join(' ')} -o ExitOnForwardFailure=yes -tt -R ${q(`127.0.0.1:${remotePort}:127.0.0.1:`)}"$PORT" ${q(host)} ${q(remoteRun)}`;
      write(launcher, `#!/bin/sh\nset -eu\nexport PATH="$HOME/.local/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin:$PATH"\n${attach} > ${q(attached + '.tmp')} || { mv -f ${q(attached + '.tmp')} ${q(attached)}; exit 1; }\nmv -f ${q(attached + '.tmp')} ${q(attached)}\n${q(process.execPath)} ${q(process.argv[1])} environment ${q(directory)} ${q(tmux)}\n. ${q(envFile)}\n${command}\n`);
      fs.chmodSync(launcher, 0o700);
      if (existing) run('rig', ['unclaim', target]);
      fs.rmSync(attached, { force: true });
      run('tmux', ['new-session', '-d', '-s', tmux, '-x', '120', '-y', '36', `/bin/sh ${q(launcher)}`]);
      const agent: Agent = { target, project, name, host, cwd, runtime, tmux, remotePort };
      const deadline = Date.now() + 10000;
      while (!fs.existsSync(attached) && Date.now() < deadline) await delay(100);
      if (!fs.existsSync(attached)) throw new Error('Agent did not register with the coordinator.');
      const registration = JSON.parse(fs.readFileSync(attached, 'utf8')) as { ok?: boolean; error?: string; attachmentType?: string };
      if (registration.ok === false || registration.attachmentType !== 'tmux') throw new Error(registration.error || 'Agent transport registration failed.');
      await delay(300);
      run('tmux', ['has-session', '-t', `=${tmux}`], 3000);
      write(recordFile, agent);
      console.log(JSON.stringify(agent));
    }
  } else throw new Error('Unknown coordinator action');
} catch (error) {
  console.error(error instanceof Error ? error.message.slice(0, 3000) : String(error).slice(0, 3000));
  process.exitCode = 1;
}
