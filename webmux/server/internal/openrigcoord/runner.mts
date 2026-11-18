// WebMux owns the control plane; SSH carries each remote CLI's terminal and a
// loopback-only reverse forward. No remote daemon or provider credentials copied.
import fs from 'node:fs';
import { observeRigEnrollments } from './rig-enrollment.mts';
import { rigTmuxAttachment, type RigAttachmentNode } from './rig-attachment.mts';
import type { RigRecoveryIdentity } from './rig-recovery.mts';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';

type Agent = { target: string; senderAddress?: string; project: string; rigId?: string; rigName?: string; pod?: string; name: string; host: string; cwd: string; runtime: string; tmux: string; remotePort?: number; sessionId?: string; nativeOwned?: true; nativeNodeId?: string; logicalId?: string; creationOperationId?: string };
type Input = { mode?: 'new' | 'existing'; project?: string; rigId?: string; rigName?: string; pod?: string; name?: string; host?: string; cwd?: string; runtime?: string; path?: string; inspect?: boolean; trustRequest?: string; creationRequestId?: string; continueOriginal?: boolean; row?: number; col?: number };
class BlockedCreation extends Error {
  value: { state: 'blocked'; code: 'native_project_trust_required'; requestId: string };
  constructor(value: BlockedCreation['value']) { super('native_creation_blocked'); this.value = value; }
}
class PendingCreation extends Error {
  value: { state: 'uncertain'; stage: 'project' | 'agent'; code: string; operationId: string };
  constructor(code: string, operationId: string, stage: 'project' | 'agent' = 'project') { super('native_creation_pending'); this.value = { state: 'uncertain', stage, code, operationId }; }
}
type RigRow = { rigId?: string; name?: string; rigName?: string; nodeCount?: number; runningCount?: number; status?: string; lifecycleState?: string; latestSnapshot?: string | null };
type AgentActivity = { state?: string; evidenceSource?: string; sampledAt?: string; stale?: boolean; generationProvenance?: string };
type NodeRow = RigAttachmentNode & { nodeId?: string; agentActivity?: AgentActivity; startupStatus?: string; rigId?: string; rigName?: string; logicalId?: string; podNamespace?: string; hostSelfId?: string | null; runtime?: string; sessionStatus?: string; lifecycleState?: string };
const [action, directory] = process.argv.slice(2);
if (!directory) throw new Error('Missing coordination state directory');
// Controller commands follow the configured coordinator endpoint. A user's
// CLI host-selection preference must not redirect discovery or lifecycle work.
process.env.OPENRIG_HOST_SELECTED = 'local';
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
const observationArgs = action === 'observations' ? ['--no-cleanup'] : [];
const rigRows = (archived = false): RigRow[] => entries(JSON.parse(run('rig', ['ps', ...observationArgs, ...(archived ? ['--include-archived'] : []), '--json', '--fields', 'rigId,name,rigName,nodeCount,runningCount,status,lifecycleState,latestSnapshot'])) as RigRow[] | { entries?: RigRow[] });
// Project before enrichment so neither seats nor legacy agent rows carry raw
// evidence, runtime event text, or provider secrets.
const activityFacts = (activity?: AgentActivity): AgentActivity | undefined => activity ? { state: activity.state, evidenceSource: activity.evidenceSource, sampledAt: activity.sampledAt, stale: activity.stale, generationProvenance: activity.generationProvenance } : undefined;
let structuredAttachments: boolean | undefined;
const structuredAttachmentBuild = () => {
  if (structuredAttachments === undefined) {
    try { structuredAttachments = run('rig', ['--version']).trim() === '0.7.18 (3bf87c62)'; }
    catch { structuredAttachments = false; }
  }
  return structuredAttachments;
};
const nodeRows = (): NodeRow[] => {
  structuredAttachmentBuild();
  const fields = 'rigId,rigName,logicalId,podNamespace,hostSelfId,canonicalSessionName,runtime,sessionStatus,lifecycleState,tmuxAttachCommand,startupStatus,agentActivity'
    + (structuredAttachments ? ',attachmentType,tmuxServerArgs' : '');
  // 0.7.18 emits structured attachment fields in full JSON but its CLI
  // --fields allow-list omits them. Project immediately; never export the
  // remaining full inventory (including recovery/runtime metadata).
  return entries(JSON.parse(run('rig', ['ps', '--nodes', '-A', ...observationArgs, '--json', ...(structuredAttachments ? ['--full'] : ['--fields', fields])])) as NodeRow[] | { entries?: NodeRow[] })
    .map(node => ({
      rigId: node.rigId, rigName: node.rigName, logicalId: node.logicalId, podNamespace: node.podNamespace,
      hostSelfId: node.hostSelfId, canonicalSessionName: node.canonicalSessionName, runtime: node.runtime,
      sessionStatus: node.sessionStatus, lifecycleState: node.lifecycleState, tmuxAttachCommand: node.tmuxAttachCommand,
      startupStatus: node.startupStatus, agentActivity: activityFacts(node.agentActivity),
      ...(structuredAttachments ? { nodeId: node.nodeId, attachmentType: node.attachmentType, tmuxServerArgs: node.tmuxServerArgs } : {}),
    }));
};
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
    write(path.join(directory, `${tmux}.env`), 'export OPENRIG_HOST_SELECTED=local\n' + keys.filter(key => typeof attached.env?.[key] === 'string').map(key => `export ${key}=${q(String(attached.env?.[key]))}`).join('\n') + '\n');
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
  } else if (action === 'stop-preview') {
    const { reviewRigStop } = await import('./rig-stop.mts');
    console.log(JSON.stringify(await reviewRigStop(run, JSON.parse(fs.readFileSync(0, 'utf8')) as import('./rig-stop.mts').RigStopSelection)));
  } else if (action === 'stop-execute') {
    const { executeRigStop } = await import('./rig-stop.mts');
    const input = JSON.parse(fs.readFileSync(0, 'utf8')) as { review: import('./rig-stop.mts').RigStopReview; reason: string; operator: string };
    console.log(JSON.stringify(await executeRigStop(run, input.review, input.reason, input.operator)));
  } else if (action === 'status' || action === 'status-enrollment' || action === 'observations') {
    let rigs: RigRow[];
    let nodes: NodeRow[];
    try { rigs = rigRows(true); nodes = nodeRows(); }
    catch { throw new Error('Canonical Rig discovery requires OpenRig CLI 0.6.14 or newer and a reachable daemon.'); }
    const managed = records();
    const agents = managed.map(agent => {
      const matches = nodes.filter(node => agent.logicalId ? node.rigId === agent.rigId && node.logicalId === agent.logicalId : node.canonicalSessionName === agent.target);
      const node = matches.length === 1 ? matches[0] : undefined;
      const attachment = node ? rigTmuxAttachment(node, structuredAttachments === true) : undefined;
      const connected = (() => {
        // A canonical inventory must select its exact server. Only the legacy
        // agents-only protocol may use the environment-selected namespace.
        if (matches.length && !attachment || structuredAttachments && !attachment) return false;
        try {
          run('tmux', [...(attachment?.tmuxServerFlag ? [attachment.tmuxServerFlag, attachment.tmuxServerValue] : []),
            'has-session', '-t', `=${attachment?.tmuxTarget ?? agent.tmux}`], 3000);
          return true;
        } catch { return false; }
      })();
      return { ...agent, ...(node ?? {}), connected };
    });
    const enrollments = action === 'status' ? [] : await observeRigEnrollments(run, nodes);
    const seats = nodes.map((node, index) => {
      const agent = managed.find(value => value.logicalId ? value.rigId === node.rigId && value.logicalId === node.logicalId : value.target === node.canonicalSessionName);
      const attachment = rigTmuxAttachment(node, structuredAttachments === true);
      return { ...node, ...enrollments[index], ...attachment, checkedStop: structuredAttachments === true, managed: Boolean(agent), host: agent?.host, cwd: agent?.cwd, name: agent?.name, openable: Boolean(attachment) };
    });
    const messages: { from: string; to: string; body: string; time: string; state: string }[] = [];
    for (const agent of (action === 'observations' ? [] : agents.slice(0, 16))) {
      const rows = JSON.parse(run('rig', ['queue', 'outbox-list', agent.senderAddress ?? agent.target, '--limit', '8', '--json'])) as { senderSession: string; destinationSession: string; body: string; tsDispatched: string; deliveryState: string }[];
      for (const row of rows) messages.push({ from: row.senderSession, to: row.destinationSession, body: row.body.slice(0, 2000), time: row.tsDispatched, state: row.deliveryState });
    }
    messages.sort((a, b) => b.time.localeCompare(a.time));
    console.log(JSON.stringify({ rigs, seats, agents, nativeCreationRequests: structuredAttachments === true && Number(process.versions.node.split('.')[0]) === 24 && ['darwin', 'linux'].includes(process.platform), messages: messages.slice(0, 24), contract: 'openrig-cli/0.6.14' }));
  } else if (action === 'recovery-observe') {
    const { observeRigRecovery } = await import('./rig-recovery.mts');
    const identity = JSON.parse(fs.readFileSync(0, 'utf8')) as RigRecoveryIdentity;
    console.log(JSON.stringify(await observeRigRecovery(run, identity, directory)));
  } else if (action === 'recovery-plan') {
    const { planRigRecovery } = await import('./rig-recovery.mts');
    const identity = JSON.parse(fs.readFileSync(0, 'utf8')) as RigRecoveryIdentity;
    console.log(JSON.stringify(await planRigRecovery(run, identity)));
  } else if (action === 'recovery-process') {
    const { observeRigNativeProcess } = await import('./rig-recovery.mts');
    const identity = JSON.parse(fs.readFileSync(0, 'utf8')) as RigRecoveryIdentity;
    console.log(JSON.stringify(observeRigNativeProcess(run, identity)));
  } else if (action === 'recovery-inspect') {
    const { inspectRigRecoveryProcess } = await import('./rig-recovery.mts');
    const identity = JSON.parse(fs.readFileSync(0, 'utf8')) as RigRecoveryIdentity;
    console.log(JSON.stringify(await inspectRigRecoveryProcess(run, identity)));
  } else if (action === 'recovery-prepare' || action === 'recovery-resume') {
    const { prepareRigNativeRecovery, executeRigNativeRecovery } = await import('./rig-recovery.mts');
    const identity = JSON.parse(fs.readFileSync(0, 'utf8')) as RigRecoveryIdentity;
    console.log(JSON.stringify(await (action === 'recovery-prepare' ? prepareRigNativeRecovery : executeRigNativeRecovery)(run, identity)));
  } else if (action === 'native-recovery-resume' || action === 'native-recovery-attachment') {
    const { verifyRigNativeCodexReference, observeRigNativeOriginalAttachment } = await import('./rig-native-capture.mts');
    const { resumeRigNativeCodex } = await import('./rig-native-resume.mts');
    const { nativeCreateArtifacts } = await import('./rig-native-create.mts');
    const input = JSON.parse(fs.readFileSync(0, 'utf8')) as { identity: RigRecoveryIdentity; operationId?: string; prepare?: boolean; inspect?: boolean };
    const identity = input.identity;
    if (!identity.native_rig_recovery) throw new Error('native_resume_reference_missing');
    const reference = JSON.parse(identity.native_rig_recovery) as import('./rig-native-capture.mts').NativeCodexRecoveryReference;
    const matches = records().filter(agent => agent.nativeOwned === true && agent.target === identity.target
      && agent.host === identity.host && agent.cwd === identity.workspace && agent.runtime === identity.runtime
      && agent.rigId === reference.rigId && agent.nativeNodeId === reference.nodeId && agent.logicalId === `${reference.pod}.${reference.member}`);
    if (matches.length !== 1) throw new Error('native_resume_original_agent_unavailable');
    const selection = { directory, identity, rigId: reference.rigId, nodeId: reference.nodeId, pod: reference.pod, member: reference.member, cwd: reference.cwd };
    await verifyRigNativeCodexReference(selection, reference, { run });
    const surviving = (input.prepare || action === 'native-recovery-attachment') ? await observeRigNativeOriginalAttachment(selection, reference, { run }) : undefined;
    if (surviving) console.log(JSON.stringify({ state: input.prepare ? 'surviving' : 'attached', code: 'native_original_surviving', posted: false,
      operationId: input.operationId, nativeSessionId: reference.nativeSessionId, logicalId: `${reference.pod}.${reference.member}`, attachment: surviving }));
    else if (action === 'native-recovery-attachment') throw new Error('native_resume_original_not_running');
    else {
      if (!input.operationId) throw new Error('native_resume_selection_invalid');
      const observationHelper = process.platform === 'linux' ? JSON.parse(nativeCreateArtifacts.read(path.join(directory, '.native-codex-observer.json')).toString('utf8')) as import('./codex-owner-plan.mts').CodexObservationHelper : undefined;
      const result = await resumeRigNativeCodex({ directory, rigId: reference.rigId, nodeId: reference.nodeId, pod: reference.pod, member: reference.member,
        cwd: reference.cwd, snapshotId: reference.snapshotId, historySessionId: reference.historySessionId, nativeSessionId: reference.nativeSessionId,
        sessionName: reference.sessionName, operationId: input.operationId, prepare: input.prepare, inspect: input.inspect }, { run, observationHelper });
      if (result.state !== 'confirmed') console.log(JSON.stringify(result));
      else {
        const attachment = await observeRigNativeOriginalAttachment(selection, reference, { run });
        console.log(JSON.stringify(attachment ? { ...result, attachment } : { ...result, state: 'uncertain', code: 'native_resume_attachment_unverified' }));
      }
    }
  } else if (action === 'native-recovery-observe') {
    const { captureRigNativeCodex } = await import('./rig-native-capture.mts');
    const identity = JSON.parse(fs.readFileSync(0, 'utf8')) as RigRecoveryIdentity;
    const matches = records().filter(agent => agent.nativeOwned === true && agent.target === identity.target
      && agent.host === identity.host && agent.cwd === identity.workspace && agent.runtime === identity.runtime);
    if (matches.length !== 1) throw new Error('native_recovery_original_agent_unavailable');
    const agent = matches[0], logical = agent.logicalId?.split('.');
    if (!agent.rigId || !agent.nativeNodeId || logical?.length !== 2 || agent.pod !== logical[0]) throw new Error('native_recovery_original_agent_unavailable');
    console.log(JSON.stringify(await captureRigNativeCodex({ directory, identity, rigId: agent.rigId, nodeId: agent.nativeNodeId,
      pod: logical[0], member: logical[1], cwd: agent.cwd }, { run })));
  } else if (action === 'native-project') {
    const { createRigNativeProject } = await import('./rig-native-project.mts');
    const input = JSON.parse(fs.readFileSync(0, 'utf8')) as import('./rig-native-project.mts').NativeRigProjectInput;
    console.log(JSON.stringify(await createRigNativeProject({ ...input, directory }, { run })));
  } else if (action === 'native-resume') {
    const { resumeRigNativeCodex } = await import('./rig-native-resume.mts');
    const { nativeCreateArtifacts } = await import('./rig-native-create.mts');
    const input = JSON.parse(fs.readFileSync(0, 'utf8')) as import('./rig-native-resume.mts').NativeCodexResumeInput;
    const observationHelper = process.platform === 'linux' ? JSON.parse(nativeCreateArtifacts.read(path.join(directory, '.native-codex-observer.json')).toString('utf8')) as import('./codex-owner-plan.mts').CodexObservationHelper : undefined;
    console.log(JSON.stringify(await resumeRigNativeCodex({ ...input, directory }, { run, observationHelper })));
  } else if (action === 'native-create') {
    const { createRigNativeCodex, nativeCreateArtifacts } = await import('./rig-native-create.mts');
    const input = JSON.parse(fs.readFileSync(0, 'utf8')) as import('./rig-native-create.mts').NativeCodexCreateInput;
    // The authenticated caller's owner namespace always wins over input paths.
    const observationHelper = process.platform === 'linux' ? JSON.parse(nativeCreateArtifacts.read(path.join(directory, '.native-codex-observer.json')).toString('utf8')) as import('./codex-owner-plan.mts').CodexObservationHelper : undefined;
    console.log(JSON.stringify(await createRigNativeCodex({ ...input, directory }, { run, observationHelper })));
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
    if (input.trustRequest !== undefined && (!/^[a-f0-9]{64}$/.test(input.trustRequest) || !structuredAttachmentBuild()
      || !local || runtime !== 'codex')) throw new Error('The saved native trust request is unavailable for this selection.');
    if (input.continueOriginal !== undefined && (typeof input.continueOriginal !== 'boolean' || !input.creationRequestId)) throw new Error('native_http_create_invalid_continuation');
    let publicRequest: { requireNative(native: boolean): void } | undefined;
    if (input.creationRequestId !== undefined) {
      const { retainHttpCreateRequest } = await import('./rig-http-create.mts');
      publicRequest = retainHttpCreateRequest(directory, input.creationRequestId, {
        mode, rigId: input.rigId ?? '', rigName: input.rigName ?? input.project ?? '', pod: input.pod ?? '',
        name, host, cwd, runtime, row: input.row ?? null, col: input.col ?? null,
      }, structuredAttachmentBuild());
    }
    const trustSelection = Buffer.from(JSON.stringify({ mode, rigId: input.rigId ?? '', rigName: input.rigName ?? input.project ?? '',
      pod: input.pod ?? '', name, host, cwd, runtime }));
    if (input.trustRequest) {
      // Bind the browser selection BEFORE the topology allocator. A changed
      // project/name must not allocate a namespace and only then fail the
      // member's saved-selection check.
      const { nativeCreateArtifacts: artifacts } = await import('./rig-native-create.mts');
      try {
        const requests = artifacts.ownedRoot(path.join(directory, 'trust-requests'));
        if (!artifacts.read(path.join(requests, input.trustRequest + '.json')).equals(trustSelection)) throw new Error();
      } catch { throw new Error('native_create_original_request_changed'); }
    }
    if (input.inspect && (!structuredAttachmentBuild() || (mode !== 'new' && !(mode === 'existing' && local && runtime === 'codex')))) throw new Error('This request has no supported original creation to inspect.');
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
      const nativeOriginal = path.join(directory, 'projects', crypto.createHash('sha256').update(project).digest('hex'), 'original.json');
      if (structuredAttachmentBuild()) {
        const { createRigNativeProject } = await import('./rig-native-project.mts');
        const { nativeCreateArtifacts: artifacts } = await import('./rig-native-create.mts');
        const requests = artifacts.ownedRoot(path.join(directory, 'project-requests'));
        const key = crypto.createHash('sha256').update(JSON.stringify([project, name, host])).digest('hex');
        const original = path.join(requests, key + '.json');
        if (input.inspect && !fs.existsSync(original)) throw new Error('The original project creation request is unavailable.');
        artifacts.publish(original, Buffer.from(JSON.stringify({ contract: 'webmux-project-launch/v1', project, name, host, cwd, runtime })));
        if (input.inspect || fs.existsSync(nativeOriginal) || !saved?.rigId) {
          // Namespace allocation is controller-local even for an SSH member.
          // Every member retains its own execution-host cwd below.
          const result = await createRigNativeProject({ directory, name: project, cwd: directory, inspect: input.inspect }, { run });
          if (result.state !== 'recorded' || !result.rigId) throw new PendingCreation(result.code, result.operationId);
          rigId = result.rigId;
        } else {
          // Keep earlier WebMux launch records attached to their original rig;
          // a stopped/missing namespace is not authority to allocate another.
          const originalRig = rigRows().find(value => value.rigId === saved.rigId);
          if (!originalRig || (originalRig.rigName || originalRig.name) !== rig) throw new Error('The saved project is unavailable. Recover its original Rig project or choose another project.');
          rigId = saved.rigId;
        }
      } else {
        const savedRig = saved?.rigId ? rigRows(true).find(value => value.rigId === saved.rigId) : undefined;
        if (!saved?.rigId || !savedRig || savedRig.lifecycleState === 'stopped') {
          write(spec, { version: '0.2', name: rig, summary: `Coordinated project ${project}`, pods: [{ id: pod, label: project, members: [] }], edges: [] });
          const result = JSON.parse(run('rig', ['up', spec, '--yes', '--json'])) as { rigId?: string };
          if (!result.rigId || !/^[0-9A-HJKMNP-TV-Z]{26}$/.test(result.rigId)) throw new Error('Coordinator did not create the project.');
          write(projectState, result);
        }
        rigId = (JSON.parse(fs.readFileSync(projectState, 'utf8')) as { rigId: string }).rigId;
      }
    }
    const mapping = records().find(agent => (agent.rigId === rigId || (!agent.rigId && agent.project === project)) && agent.host === host);
    if (mapping && mapping.cwd !== cwd) throw new Error(`Rig ${rig} already maps ${host} to ${mapping.cwd}. Use that directory or choose another topology.`);
    const member = `${name}-${crypto.createHash('sha256').update(host).digest('hex').slice(0, 8)}`;
    const tmux = `wm-${scope}-${rigId.slice(-8).toLowerCase()}-${member}`;
    const target = tmux;
    const recordFile = path.join(directory, `${tmux}.agent.json`);
    const existing = records().find(a => a.tmux === tmux || (a.nativeOwned === true && a.rigId === rigId && a.pod === pod && a.name === name && a.host === host));
    let alive = false;
    try { run('tmux', ['has-session', '-t', `=${tmux}`], 3000); alive = true; } catch { /* A stopped relay can be explicitly reconnected. */ }
    if (existing && (existing.cwd !== cwd || existing.runtime !== runtime || existing.rigId !== rigId)) throw new Error('This agent has a different topology, directory or runtime. Choose another name.');
    const native = existing?.nativeOwned === true || (!existing && structuredAttachmentBuild() && local && runtime === 'codex');
    publicRequest?.requireNative(native);
    if (input.inspect && mode === 'existing' && !native) throw new Error('This session has no original native creation request to inspect.');
    if (native) {
      const { launchRigNativeCodex } = await import('./rig-native-launch.mts');
      const { nativeCreateArtifacts } = await import('./rig-native-create.mts');
      const observationHelper = process.platform === 'linux' ? JSON.parse(nativeCreateArtifacts.read(path.join(directory, '.native-codex-observer.json')).toString('utf8')) as import('./codex-owner-plan.mts').CodexObservationHelper : undefined;
      const result = await launchRigNativeCodex({ directory, rigId, pod, member, cwd, inspect: input.inspect, trustRequest: input.trustRequest }, { run, observationHelper });
      if (result.state === 'blocked') {
        const requests = nativeCreateArtifacts.ownedRoot(path.join(directory, 'trust-requests'));
        nativeCreateArtifacts.publish(path.join(requests, result.requestId + '.json'), trustSelection);
        throw new BlockedCreation({ state: 'blocked', code: result.code, requestId: result.requestId });
      }
      if (result.state !== 'attached') throw new PendingCreation(result.code, result.operationId, 'agent');
      const agent: Agent = { target: result.target, senderAddress: result.target, project, rigId, rigName: rig, pod, name, host, cwd,
        runtime, tmux: result.target, sessionId: result.sessionId, nativeOwned: true, nativeNodeId: result.nodeId, logicalId: `${pod}.${member}`, creationOperationId: result.operationId };
      run('tmux', [result.tmuxServerFlag, result.tmuxServerValue, 'set-option', '-t', result.target, 'mouse', 'on'], 3000);
      write(recordFile, agent);
      console.log(JSON.stringify(agent));
    } else if (existing && alive) {
      if (existing.cwd !== cwd || existing.runtime !== runtime) throw new Error('This agent already exists with a different directory or runtime. Choose another name.');
      run('tmux', ['has-session', '-t', `=${tmux}`], 3000);
      run('tmux', ['set-option', '-t', tmux, 'mouse', 'on'], 3000);
      console.log(JSON.stringify(existing));
    } else {
      // Validate the actual execution host before allocating a central seat.
      const probe = `export PATH="$PATH:$HOME/.local/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin"; cd ${q(cwd)} && command -v ${runtime === 'codex' ? 'codex' : 'claude'} && command -v rig${local ? '' : ' && command -v tmux'}`;
      try {
        if (local) run('/bin/sh', ['-c', probe]); else run('ssh', [...sshArgs, host, probe]);
      } catch (error) {
        // Report the cause, not the probe's command line. ssh exits 255 (or is
        // killed at the timeout) when it cannot reach the host at all.
        const failure = error as { status?: number | null; stderr?: string };
        const detail = String(failure.stderr ?? '').trim().split('\n').filter(Boolean).pop() ?? '';
        if (!local && (failure.status === 255 || failure.status == null)) throw new Error(`Cannot reach ${host} over SSH${detail ? `: ${detail.slice(0, 300)}` : ''}. Nothing was started.`);
        throw new Error(`${host} is not ready for this agent: ${cwd} must exist and ${runtime === 'codex' ? 'codex' : 'claude'}, rig${local ? '' : ' and tmux'} must be on its PATH.`);
      }
      // attach-self binds the real central tmux pane. It remains the delivery
      // transport even when its foreground process is SSH to another machine.
      const attached = path.join(directory, `${tmux}.attached.json`);
      const launcher = path.join(directory, `${tmux}.sh`);
      const role = `You are ${name}, a WebMux coordinated agent in Rig ${rig} (${rigId}), executing on ${host} in ${cwd}. OpenRig is already connected through OPENRIG_URL and OPENRIG_SESSION_NAME. Run rig whoami --json and rig ps --nodes --rig ${rig} --json to discover teammates. Use rig send <canonicalSessionName> <message> for coordination; do not override sender identity. Rig identity is shared, files are host-local. Keep the owner's project instructions and permission policy. Report READY and wait for the owner or a teammate. Do not launch other agents automatically.`;
      const cli = runtime === 'codex' ? `codex ${q(role)}` : `claude --append-system-prompt ${q(role)} ${q('Verify your coordination identity, discover teammates, report READY and wait.')}`;
      const attach = `rig attach --self --rig ${q(rigId)} ${existing ? `--node ${q((existing.pod ?? pod) + '.' + member)}` : `--pod ${q(pod)} --member ${q(member)}`} --runtime ${q(runtime)} --cwd ${q(cwd)}${relayIdentity ? ` --relay-ssh-host ${q(host)} --relay-tmux-session ${q(tmux)}` : ''} --json`;
      // Get the actual daemon URL from the supported attach response, not a
      // guessed service port. Node writes a shell-quoted env file from JSON.
      const envFile = path.join(directory, `${tmux}.env`);
      const remotePort = local ? 0 : existing?.remotePort ?? Number(run('ssh', [...sshArgs, host, "python3 -c 'import socket; s=socket.socket(); s.bind((\"127.0.0.1\",0)); print(s.getsockname()[1])'"]));
      if (!local && (!Number.isInteger(remotePort) || remotePort < 1024 || remotePort > 65535)) throw new Error('Cannot allocate a private SSH reverse forward.');
      const remoteAgent = `export PATH="$PATH:$HOME/.local/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin"; export OPENRIG_HOST_SELECTED=local; export OPENRIG_URL=http://127.0.0.1:${remotePort}; export OPENRIG_SESSION_NAME=${q('__OPENRIG_SENDER_ADDRESS__')}; cd ${q(cwd)} && exec ${cli}`;
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
      write(launcher, `#!/bin/sh\nset -eu\nexport OPENRIG_HOST_SELECTED=local\nexport PATH=${q(process.env.PATH ?? '')}:"$HOME/.local/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin"\n${attach} > ${q(attached + '.tmp')} || { mv -f ${q(attached + '.tmp')} ${q(attached)}; exit 1; }\nmv -f ${q(attached + '.tmp')} ${q(attached)}\n${q(process.execPath)} ${q(process.argv[1])} environment ${q(directory)} ${q(tmux)}\n. ${q(envFile)}\n${command}\n`);
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
      const registration = JSON.parse(fs.readFileSync(attached, 'utf8')) as { ok?: boolean; error?: string; attachmentType?: string; sessionId?: string; sessionName?: string; env?: { OPENRIG_SESSION_NAME?: string } };
      if (registration.ok === false || registration.attachmentType !== 'tmux' || !registration.sessionId) throw new Error(registration.error || 'Agent transport registration failed.');
      agent.sessionId = registration.sessionId;
      if (typeof registration.env?.OPENRIG_SESSION_NAME !== 'string') throw new Error('Coordinator did not return a sender address.');
      if (!/^[a-zA-Z0-9_.:@-]{1,256}$/.test(registration.env.OPENRIG_SESSION_NAME)) throw new Error('Coordinator returned an invalid sender address.');
      agent.senderAddress = registration.env.OPENRIG_SESSION_NAME;
      if (structuredAttachmentBuild()) {
        if (registration.sessionName !== tmux) throw new Error('Coordinator did not verify the created physical tmux session.');
        agent.target = registration.sessionName;
      } else {
        agent.target = registration.env.OPENRIG_SESSION_NAME;
      }
      // Keep a registered seat discoverable even if provider startup fails.
      // The owner can fix authentication and explicitly reconnect that seat.
      write(recordFile, agent);
      await delay(300);
      run('tmux', ['has-session', '-t', `=${tmux}`], 3000);
      console.log(JSON.stringify(agent));
    }
  } else throw new Error('Unknown coordinator action');
} catch (error) {
  if (error instanceof BlockedCreation) {
    console.log(JSON.stringify({ blocked: error.value }));
  } else if (error instanceof PendingCreation) {
    console.log(JSON.stringify({ creation: error.value }));
  } else {
    console.error((action === 'native-resume' || action === 'native-recovery-resume')
      ? error instanceof Error && /^native_(?:resume|create)_[a-z_]+$/.test(error.message) ? error.message : 'native_resume_original_unavailable'
      : error instanceof Error ? error.message.slice(0, 3000) : String(error).slice(0, 3000));
    process.exitCode = 1;
  }
}
