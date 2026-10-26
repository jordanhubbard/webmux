// WebMux owns the control plane; SSH carries each remote CLI's terminal and a
// loopback-only reverse forward. No remote daemon or provider credentials copied.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';

type Agent = { target: string; project: string; rigId?: string; rigName?: string; pod?: string; name: string; host: string; cwd: string; runtime: string; tmux: string; remotePort?: number; sessionId?: string };
type Input = { mode?: 'new' | 'existing'; project?: string; rigId?: string; rigName?: string; pod?: string; name?: string; host?: string; cwd?: string; runtime?: string; path?: string };
type RigRow = { rigId?: string; name?: string; rigName?: string; nodeCount?: number; runningCount?: number; status?: string; lifecycleState?: string; latestSnapshot?: string | null };
type NodeRow = { rigId?: string; rigName?: string; logicalId?: string; podNamespace?: string; canonicalSessionName?: string; runtime?: string; sessionStatus?: string; lifecycleState?: string; tmuxAttachCommand?: string | null };
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
const entries = <T,>(value: T[] | { entries?: T[] }): T[] => Array.isArray(value) ? value : value.entries ?? [];
const rigRows = (archived = false): RigRow[] => entries(JSON.parse(run('rig', ['ps', ...(archived ? ['--include-archived'] : []), '--json', '--fields', 'rigId,name,rigName,nodeCount,runningCount,status,lifecycleState,latestSnapshot'])) as RigRow[] | { entries?: RigRow[] });
const nodeRows = (): NodeRow[] => entries(JSON.parse(run('rig', ['ps', '--nodes', '-A', '--json', '--fields', 'rigId,rigName,logicalId,podNamespace,canonicalSessionName,runtime,sessionStatus,lifecycleState,tmuxAttachCommand'])) as NodeRow[] | { entries?: NodeRow[] });
const identifier = (value: string, field: string) => {
  if (!/^[a-z][a-z0-9-]{0,47}$/.test(value)) throw new Error(`${field}: use a letter followed by letters, digits or hyphens (48 characters maximum).`);
  return value;
};
const sshArgs = ['-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes', '-o', 'ConnectTimeout=8', '-o', 'ServerAliveInterval=15', '-o', 'ServerAliveCountMax=3'];
try {
  if (action !== 'environment' && action !== 'browse') {
    // Rig's activity probe can crash tmux 3.2a after a seat disappears,
    // taking every session on that server with it. Fail before calling Rig.
    const version = run('tmux', ['-V']);
    const match = /^tmux (?:next-)?(\d+)\.(\d+)/.exec(version);
    if (!match || Number(match[1]) < 3 || (Number(match[1]) === 3 && Number(match[2]) < 4)) {
      throw new Error('Rig coordination requires tmux 3.4 or newer on the WebMux server. Upgrade tmux on the service PATH before enabling coordination.');
    }
  }
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
    const attached = JSON.parse(fs.readFileSync(path.join(directory, `${tmux}.attached.json`), 'utf8')) as { attachmentType?: string; sessionName?: string; senderAddress?: string; env?: Record<string, unknown> };
    if (attached.attachmentType !== 'tmux' || typeof attached.env?.OPENRIG_URL !== 'string' || attached.sessionName !== tmux || typeof attached.env?.OPENRIG_SESSION_NAME !== 'string' || attached.env.OPENRIG_SESSION_NAME !== (attached.senderAddress ?? attached.sessionName)) throw new Error('Rig did not bind the expected terminal');
    const keys = ['OPENRIG_NODE_ID', 'OPENRIG_SESSION_NAME', 'OPENRIG_RUNTIME', 'OPENRIG_URL', 'OPENRIG_ACTIVITY_HOOK_TOKEN'];
    const remoteFile = path.join(directory, `${tmux}.remote.json`);
    if (fs.existsSync(remoteFile)) {
      const remote = JSON.parse(fs.readFileSync(remoteFile, 'utf8')) as { setup: string; run: string };
      const sender = q(String(attached.env.OPENRIG_SESSION_NAME));
      attached.env.OPENRIG_RELAY_SETUP = remote.setup.replaceAll(q('__OPENRIG_SENDER_ADDRESS__'), sender);
      attached.env.OPENRIG_RELAY_RUN = remote.run.replaceAll(q('__OPENRIG_SENDER_ADDRESS__'), sender);
      keys.push('OPENRIG_RELAY_SETUP', 'OPENRIG_RELAY_RUN');
    }
    write(path.join(directory, `${tmux}.env`), keys.filter(key => typeof attached.env?.[key] === 'string').map(key => `export ${key}=${q(String(attached.env?.[key]))}`).join('\n') + '\n');
  } else if (action === 'browse') {
    const input = JSON.parse(fs.readFileSync(0, 'utf8')) as Input;
    const host = input.host?.trim() || 'localhost';
    if (!/^(?:[a-zA-Z0-9_][a-zA-Z0-9_.-]*@)?[a-zA-Z0-9_][a-zA-Z0-9_.-]*$/.test(host)) throw new Error('Host must be localhost or a configured OpenSSH host name (optionally user@host).');
    const requested = input.path?.trim() || '';
    if (requested && (!requested.startsWith('/') || /[\x00\r\n]/.test(requested) || requested.length > 2048)) throw new Error('Choose an absolute directory on the execution host.');
    const browser = String.raw`import json, os, sys
p = os.path.realpath(sys.argv[1] if len(sys.argv) > 1 and sys.argv[1] else os.path.expanduser('~'))
if not os.path.isdir(p): raise SystemExit('Directory does not exist on this host')
rows = []
for name in sorted(os.listdir(p), key=str.casefold):
    child = os.path.join(p, name)
    if os.path.isdir(child): rows.append({'name': name, 'path': child, 'repository': os.path.exists(os.path.join(child, '.git'))})
    if len(rows) >= 200: break
print(json.dumps({'host': sys.argv[2], 'path': p, 'parent': os.path.dirname(p) if os.path.dirname(p) != p else None, 'repository': os.path.exists(os.path.join(p, '.git')), 'entries': rows}))`;
    const args = ['-c', browser, requested, host];
    const output = host === 'localhost' ? run('python3', args) : run('ssh', [...sshArgs, host, `python3 -c ${q(browser)} ${q(requested)} ${q(host)}`]);
    console.log(output);
  } else if (action === 'status') {
    let rigs: RigRow[];
    let nodes: NodeRow[];
    try { rigs = rigRows(true); nodes = nodeRows(); }
    catch { throw new Error('Canonical Rig discovery requires OpenRig CLI 0.6.14 or newer and a reachable daemon.'); }
    const managed = records();
    const agents = managed.map(agent => ({ ...agent, ...(nodes.find(e => e.canonicalSessionName === agent.target) ?? {}), connected: (() => { try { run('tmux', ['has-session', '-t', `=${agent.tmux}`], 3000); return true; } catch { return false; } })() }));
    const seats = nodes.map(node => {
      const agent = managed.find(value => value.target === node.canonicalSessionName);
      const match = typeof node.tmuxAttachCommand === 'string' ? /^tmux attach(?:-session)? -t ([a-zA-Z0-9_.:@-]{1,256})$/.exec(node.tmuxAttachCommand) : null;
      return { ...node, managed: Boolean(agent), host: agent?.host, cwd: agent?.cwd, name: agent?.name, openable: Boolean(match), tmuxTarget: match?.[1] };
    });
    const messages: { from: string; to: string; body: string; time: string; state: string }[] = [];
    for (const agent of agents.slice(0, 16)) {
      const rows = JSON.parse(run('rig', ['queue', 'outbox-list', agent.target, '--limit', '8', '--json'])) as { senderSession: string; destinationSession: string; body: string; tsDispatched: string; deliveryState: string }[];
      for (const row of rows) messages.push({ from: row.senderSession, to: row.destinationSession, body: row.body.slice(0, 2000), time: row.tsDispatched, state: row.deliveryState });
    }
    messages.sort((a, b) => b.time.localeCompare(a.time));
    console.log(JSON.stringify({ rigs, seats, agents, messages: messages.slice(0, 24), contract: 'openrig-cli/0.6.14' }));
  } else if (action === 'create') {
    const input = JSON.parse(fs.readFileSync(0, 'utf8')) as Input;
    const mode = input.mode ?? 'new';
    const name = identifier(input.name ?? '', 'Agent name');
    const host = input.host?.trim() || 'localhost';
    if (!/^(?:[a-zA-Z0-9_][a-zA-Z0-9_.-]*@)?[a-zA-Z0-9_][a-zA-Z0-9_.-]*$/.test(host)) throw new Error('Host must be localhost or a configured OpenSSH host name (optionally user@host).');
    const cwd = input.cwd ?? '';
    if (!cwd.startsWith('/') || /[\x00\r\n]/.test(cwd) || cwd.length > 2048) throw new Error('Choose an absolute project directory on the execution host.');
    const runtime = input.runtime ?? 'codex';
    if (!['codex', 'claude-code'].includes(runtime)) throw new Error('Unsupported agent runtime.');
    const local = host === 'localhost';
    // Older published coordinators retain their existing transport, with no
    // remote identity claim. Explicit relay enrollment needs both new options.
    const attachHelp = !local && runtime === 'codex' ? run('rig', ['attach', '--help']) : '';
    const relayIdentity = attachHelp.includes('--relay-ssh-host') && attachHelp.includes('--relay-tmux-session');
    const identityFile = path.join(directory, 'identity.json');
    const scope = fs.existsSync(identityFile) ? (JSON.parse(fs.readFileSync(identityFile, 'utf8')) as { scope: string }).scope : crypto.createHash('sha256').update(directory).digest('hex').slice(0, 10);
    if (!/^[a-f0-9]{10}$/.test(scope)) throw new Error('Invalid saved coordinator identity');
    write(identityFile, { scope });
    let project: string;
    let rig: string;
    let rigId: string;
    let pod: string;
    if (mode === 'existing') {
      rigId = input.rigId ?? '';
      if (!/^[0-9A-HJKMNP-TV-Z]{26}$/.test(rigId)) throw new Error('Choose a canonical Rig project.');
      const canonical = rigRows().find(value => value.rigId === rigId);
      if (!canonical) throw new Error('The selected Rig project is no longer available. Refresh and choose again.');
      rig = canonical.rigName || canonical.name || '';
      if (!rig || (input.rigName && input.rigName !== rig)) throw new Error('The selected Rig identity changed. Refresh and choose again.');
      pod = identifier(input.pod ?? '', 'Rig pod');
      if (!nodeRows().some(value => value.rigId === rigId && value.podNamespace === pod)) throw new Error('The selected Rig pod is no longer available. Refresh and choose again.');
      project = rig;
    } else {
      project = identifier(input.rigName ?? input.project ?? '', 'Rig name');
      rig = project;
      pod = 'agents';
      const projectDir = path.join(directory, project);
      fs.mkdirSync(projectDir, { recursive: true, mode: 0o700 });
      const spec = path.join(projectDir, 'rig.yaml');
      const projectState = path.join(projectDir, 'project.json');
      const saved = fs.existsSync(projectState) ? (JSON.parse(fs.readFileSync(projectState, 'utf8')) as { rigId?: string }) : undefined;
      const savedRig = saved?.rigId ? rigRows(true).find(value => value.rigId === saved.rigId) : undefined;
      if (!saved?.rigId || !savedRig || savedRig.lifecycleState === 'stopped') {
        write(spec, { version: '0.2', name: rig, summary: `Coordinated project ${project}`, pods: [{ id: pod, label: project, members: [] }], edges: [] });
        const result = JSON.parse(run('rig', ['up', spec, '--yes', '--json'])) as { rigId?: string };
        if (!result.rigId || !/^[0-9A-HJKMNP-TV-Z]{26}$/.test(result.rigId)) throw new Error('Coordinator did not create the project.');
        write(projectState, result);
      }
      rigId = (JSON.parse(fs.readFileSync(projectState, 'utf8')) as { rigId: string }).rigId;
    }
    const mapping = records().find(agent => (agent.rigId === rigId || (!agent.rigId && agent.project === project)) && agent.host === host);
    if (mapping && mapping.cwd !== cwd) throw new Error(`Rig ${rig} already maps ${host} to ${mapping.cwd}. Use that directory or choose another topology.`);
    const member = `${name}-${crypto.createHash('sha256').update(host).digest('hex').slice(0, 8)}`;
    const tmux = `wm-${scope}-${rigId.slice(-8).toLowerCase()}-${member}`;
    const target = tmux;
    const recordFile = path.join(directory, `${tmux}.agent.json`);
    const existing = records().find(a => a.tmux === tmux);
    let alive = false;
    try { run('tmux', ['has-session', '-t', `=${tmux}`], 3000); alive = true; } catch { /* A stopped relay can be explicitly reconnected. */ }
    if (existing && (existing.cwd !== cwd || existing.runtime !== runtime || existing.rigId !== rigId)) throw new Error('This agent has a different topology, directory or runtime. Choose another name.');
    if (existing && alive) {
      if (existing.cwd !== cwd || existing.runtime !== runtime) throw new Error('This agent already exists with a different directory or runtime. Choose another name.');
      run('tmux', ['has-session', '-t', `=${tmux}`], 3000);
      run('tmux', ['set-option', '-t', tmux, 'mouse', 'on'], 3000);
      console.log(JSON.stringify(existing));
    } else {
      // Validate the actual execution host before allocating a central seat.
      const probe = `export PATH="$PATH:$HOME/.local/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin"; cd ${q(cwd)} && command -v ${runtime === 'codex' ? 'codex' : 'claude'} && command -v rig${local ? '' : ' && command -v tmux'}`;
      if (local) run('/bin/sh', ['-c', probe]); else run('ssh', [...sshArgs, host, probe]);
      // attach-self binds the real central tmux pane. It remains the delivery
      // transport even when its foreground process is SSH to another machine.
      const attached = path.join(directory, `${tmux}.attached.json`);
      const launcher = path.join(directory, `${tmux}.sh`);
      const role = `You are ${name}, a WebMux coordinated agent in Rig ${rig} (${rigId}), executing on ${host} in ${cwd}. OpenRig is already connected through OPENRIG_URL and OPENRIG_SESSION_NAME. Run rig whoami --json and rig ps --nodes --rig ${rig} --json to discover teammates. Use rig send <canonicalSessionName> <message> for coordination; do not override sender identity. Rig identity is shared, files are host-local. Keep the owner's project instructions and permission policy. Report READY and wait for the owner or a teammate. Do not launch other agents automatically.`;
      const cli = runtime === 'codex' ? `codex -c sandbox_workspace_write.network_access=true ${q(role)}` : `claude --append-system-prompt ${q(role)} ${q('Verify your coordination identity, discover teammates, report READY and wait.')}`;
      const attach = `rig attach --self --rig ${q(rigId)} ${existing ? `--node ${q((existing.pod ?? pod) + '.' + member)}` : `--pod ${q(pod)} --member ${q(member)}`} --runtime ${q(runtime)} --cwd ${q(cwd)}${relayIdentity ? ` --relay-ssh-host ${q(host)} --relay-tmux-session ${q(tmux)}` : ''} --json`;
      // Get the actual daemon URL from the supported attach response, not a
      // guessed service port. Node writes a shell-quoted env file from JSON.
      const envFile = path.join(directory, `${tmux}.env`);
      const remotePort = local ? 0 : existing?.remotePort ?? Number(run('ssh', [...sshArgs, host, "python3 -c 'import socket; s=socket.socket(); s.bind((\"127.0.0.1\",0)); print(s.getsockname()[1])'"]));
      if (!local && (!Number.isInteger(remotePort) || remotePort < 1024 || remotePort > 65535)) throw new Error('Cannot allocate a private SSH reverse forward.');
      const remoteAgent = `export PATH="$PATH:$HOME/.local/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin"; export OPENRIG_URL=http://127.0.0.1:${remotePort}; export OPENRIG_SESSION_NAME=${q('__OPENRIG_SENDER_ADDRESS__')}; cd ${q(cwd)} && exec ${cli}`;
      // The patched coordinator proves this exact attach command against the local
      // SSH process, then independently observes the remote pane. Agent prompts
      // and session creation are kept out of the identity-bearing attach argv.
      const relayPath = 'export PATH="$PATH:$HOME/.local/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin"; ';
      const remoteSeat = q('=' + tmux);
      const remoteMouse = `tmux set-option -t ${q(tmux)} mouse on`;
      const remotePrepare = `(tmux has-session -t ${remoteSeat} 2>/dev/null || tmux new-session -d -s ${q(tmux)} ${q('/bin/sh -c ' + q(remoteAgent))}) && ${remoteMouse}`;
      const remoteCreate = `${relayPath}${remotePrepare}`;
      const remoteRun = `${remoteCreate} && exec tmux attach-session -t ${remoteSeat}`;
      const remoteAttach = `${relayPath}${remoteMouse} && exec tmux attach-session -t ${remoteSeat}`;
      if (!local) write(path.join(directory, `${tmux}.remote.json`), { setup: remoteCreate, run: remoteRun });
      // Pass only routing identity through the SSH channel; credentials and
      // provider homes stay on the execution host. Never forward auth tokens.
      const endpoint = `node -e 'const u=new URL(process.env.OPENRIG_URL); if(u.hostname!=="127.0.0.1"&&u.hostname!=="localhost") throw Error("Coordinator must be loopback-only"); console.log(u.port||80)'`;
      const command = local ? `cd ${q(cwd)}\nexec ${cli}` : `${relayIdentity ? `ssh ${sshArgs.map(q).join(' ')} ${q(host)} "$OPENRIG_RELAY_SETUP"\n` : ''}PORT=$(${endpoint})\nexec ssh ${sshArgs.map(q).join(' ')} -o ExitOnForwardFailure=yes -tt -R ${q(`127.0.0.1:${remotePort}:127.0.0.1:`)}"$PORT" ${q(host)} ${relayIdentity ? q(remoteAttach) : '"$OPENRIG_RELAY_RUN"'}`;
      // An existing tmux server retains its original environment. Bind the
      // service PATH explicitly rather than inheriting that server's stale PATH.
      write(launcher, `#!/bin/sh\nset -eu\nexport PATH=${q(process.env.PATH ?? '')}:"$HOME/.local/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin"\n${attach} > ${q(attached + '.tmp')} || { mv -f ${q(attached + '.tmp')} ${q(attached)}; exit 1; }\nmv -f ${q(attached + '.tmp')} ${q(attached)}\n${q(process.execPath)} ${q(process.argv[1])} environment ${q(directory)} ${q(tmux)}\n. ${q(envFile)}\n${command}\n`);
      fs.chmodSync(launcher, 0o700);
      if (existing) {
        // Reattachments retain historical records with the same session name.
        // Use the exact enrolled session ID; names become ambiguous on retry.
        const previous = existing.sessionId ?? (fs.existsSync(attached) ? (JSON.parse(fs.readFileSync(attached, 'utf8')) as { sessionId?: string }).sessionId : undefined);
        if (!previous) throw new Error('Cannot resolve the previous Rig session ID. Restore the agent registration record before reconnecting.');
        run('rig', ['unclaim', previous]);
      }
      fs.rmSync(attached, { force: true });
      // Enable wheel scrollback only for this seat. Leave the user's global
      // tmux setting and unrelated sessions alone.
      run('tmux', ['new-session', '-d', '-s', tmux, '-x', '120', '-y', '36', `/bin/sh ${q(launcher)}`]);
      run('tmux', ['set-option', '-t', tmux, 'mouse', 'on'], 3000);
      const agent: Agent = { target, project, rigId, rigName: rig, pod, name, host, cwd, runtime, tmux, remotePort };
      const deadline = Date.now() + 10000;
      while (!fs.existsSync(attached) && Date.now() < deadline) await delay(100);
      if (!fs.existsSync(attached)) throw new Error('Agent did not register with the coordinator.');
      const registration = JSON.parse(fs.readFileSync(attached, 'utf8')) as { ok?: boolean; error?: string; attachmentType?: string; sessionId?: string; env?: { OPENRIG_SESSION_NAME?: string } };
      if (registration.ok === false || registration.attachmentType !== 'tmux' || !registration.sessionId) throw new Error(registration.error || 'Agent transport registration failed.');
      agent.sessionId = registration.sessionId;
      if (typeof registration.env?.OPENRIG_SESSION_NAME !== 'string') throw new Error('Coordinator did not return a sender address.');
      agent.target = registration.env.OPENRIG_SESSION_NAME;
      // Keep a registered seat discoverable even if provider startup fails.
      // The owner can fix authentication and explicitly reconnect that seat.
      write(recordFile, agent);
      await delay(300);
      run('tmux', ['has-session', '-t', `=${tmux}`], 3000);
      console.log(JSON.stringify(agent));
    }
  } else throw new Error('Unknown coordinator action');
} catch (error) {
  console.error(error instanceof Error ? error.message.slice(0, 3000) : String(error).slice(0, 3000));
  process.exitCode = 1;
}
