#!/usr/bin/env python3
"""Qualify Rig-owned terminal survival and loss using a private native daemon.

Requires an existing synthetic Claude conversation manifest. The installed Rig
CLI and its Node runtime are supplied explicitly: the user's Rig wrapper can
pin a shared instance and must not be used for this fixture. An empty topology
and one adopted native conversation are created on a private tmux socket. No
kernel, agent role, delegated task or provider credentials are created/copied.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shlex
import signal
import shutil
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
import urllib.parse
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--rig-cli', type=Path, required=True)
    parser.add_argument('--rig-node', type=Path, required=True)
    parser.add_argument('--snapshot-plan', action='store_true', help='capture one private Rig snapshot and inspect exact-seat native launch plans without executing them')
    parser.add_argument('--native-hooks', action='store_true', help='qualify the installed Rig SessionStart relay with per-invocation Claude settings and native snapshot identity')
    parser.add_argument('--native-resume', action='store_true', help='qualify actual private Rig-owned exact-snapshot native continuation after process loss')
    parser.add_argument('--webmux-action', action='store_true', help='perform approved native recovery through the WebMux HTTP action and verify once-only receipts across restart')
    parser.add_argument('--keep-team-default', action='store_true', help='retain Rig native team-default permissions from first launch through resume without switching to explicit plan mode')
    parser.add_argument('--expect-policy-drift', action='store_true', help='test the known first-launch/resume settings difference and require WebMux to reject it; does not qualify policy-preserving native recovery')
    args = parser.parse_args()
    if args.keep_team_default and (not args.native_resume or args.expect_policy_drift):
        parser.error('--keep-team-default requires --native-resume and cannot expect a policy change')
    if args.webmux_action and (not args.native_resume or args.expect_policy_drift):
        parser.error('--webmux-action requires unchanged-policy --native-resume')
    if args.native_resume and not args.expect_policy_drift:
        args.keep_team_default = True
    if args.expect_policy_drift and not args.native_resume:
        parser.error('--expect-policy-drift requires --native-resume')
    if args.native_resume and not args.native_hooks:
        parser.error('--native-resume requires --native-hooks and --snapshot-plan')
    if args.native_hooks and not args.snapshot_plan:
        parser.error('--native-hooks requires --snapshot-plan')
    script_hash = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    repo = Path(__file__).resolve().parents[1]
    binary, rig_cli, rig_node = [p.resolve(strict=True) for p in (args.binary, args.rig_cli, args.rig_node)]
    entries = json.loads(args.manifest.read_text())
    if isinstance(entries, dict):
        entries = entries['results']
    entry = next(e for e in entries if e['cli'] == 'claude')
    assert entry['conversational_resume'] and entry['marker'].startswith('WM_RECOVERY_')
    assert Path(entry['project']).resolve() == (Path(entry['evidence']) / 'project').resolve()
    uuid.UUID(entry['native_session_id'])
    original_path = os.environ.get('WEBMUX_BROWSER_ORIGINAL_PATH') or os.environ['PATH']
    claude = shutil.which('claude', path=original_path)
    tmux = shutil.which('tmux', path=original_path)
    assert claude and tmux
    roots = {key: os.environ.get(key) for key in ['HOME', 'CLAUDE_CONFIG_DIR', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME']}
    native_root = Path(roots['CLAUDE_CONFIG_DIR']) if roots['CLAUDE_CONFIG_DIR'] else Path(roots['HOME']) / '.claude'
    state_files = list((native_root / 'projects').glob('*/' + entry['native_session_id'] + '.jsonl'))
    assert len(state_files) == 1
    native_file = state_files[0]
    settings = native_root / 'settings.json'
    settings_hash = hashlib.sha256(settings.read_bytes()).hexdigest() if settings.is_file() else None
    fixture = Path.home() / '.local/share/webmux/recovery-qualification' / ('rig-native-' + str(uuid.uuid4()))
    fixture.mkdir(parents=True, mode=0o700)
    print(json.dumps({'fixture': str(fixture)}), flush=True)
    private_home, rig_home, home, aliases = [fixture / n for n in ['manager-home', 'rig-instance', 'webmux-home', 'bin']]
    for p in [private_home, rig_home, aliases]:
        p.mkdir(mode=0o700)
    shutil.copytree(repo / 'webmux/config.defaults', home / 'config')
    (home / 'config/auth.yaml').write_text('auth:\n  mode: none\n  users: []\n')
    (home / 'config/app.yaml').write_text('app:\n  session_logging:\n    enabled: true\n  coordination:\n    enabled_providers: [rig]\n    default_launch_provider: webmux\n    default_view: all\n')
    socket_name = 'wm-rig-' + uuid.uuid4().hex[:12]
    pane = 'agents-continuation@webmux-recovery' if args.native_resume else 'wm-recovery-' + uuid.uuid4().hex[:12]
    env = dict(os.environ)
    for key in list(env):
        if key.startswith(('WEBMUX_RECOVERY_', 'OPENRIG_', 'RIGGED_', 'AGENTDECK_', 'AGENT_DECK_')) or key in ['TMUX', 'TMUX_PANE', 'WEBMUX_COORDINATION_DISABLED']:
            env.pop(key, None)
    env.update(HOME=str(private_home), OPENRIG_HOME=str(rig_home), CODEX_HOME=str(private_home / '.codex'), CLAUDE_CONFIG_DIR=str(private_home / '.claude'), WEBMUX_HOME=str(home), WEBMUX_ROOT='', PATH=str(aliases) + os.pathsep + original_path)
    for key in ['XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME']:
        p = fixture / key.lower()
        p.mkdir(mode=0o700)
        env[key] = str(p)
    def executable(path, text):
        path.write_text(text)
        path.chmod(0o700)
    restart_block = fixture / 'block-manager-start'
    executable(aliases / 'rig', '#!/bin/sh\nif [ "$1" = daemon ] && [ "${2-}" = start ] && [ -f ' + shlex.quote(str(restart_block)) + ' ]; then echo "Private qualification fixture denies manager startup" >&2; exit 1; fi\nexec ' + shlex.join([str(rig_node), str(rig_cli)]) + ' "$@"\n')
    executable(aliases / 'tmux', '#!/bin/sh\nexec ' + shlex.join([tmux, '-L', socket_name]) + ' "$@"\n')
    executable(aliases / 'node', '#!/bin/sh\nexec ' + shlex.quote(str(rig_node)) + ' "$@"\n')
    def rig(*words, check=True):
        result = subprocess.run([str(aliases / 'rig'), *words], env=env, capture_output=True, text=True, timeout=45)
        with (fixture / 'rig-events.jsonl').open('a') as f:
            f.write(json.dumps({'args': words, 'code': result.returncode, 'stdout': result.stdout, 'stderr': result.stderr}) + '\n')
        if check and result.returncode:
            raise RuntimeError('Private Rig operation failed: ' + ' '.join(words[:2]) + ': ' + result.stderr[-500:])
        return result
    def port():
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            return listener.getsockname()[1]
    rig_port, web_port = port(), port()
    base = 'http://127.0.0.1:' + str(web_port)
    def api(path, body=None, method=None):
        req = urllib.request.Request(base + path, data=json.dumps(body).encode() if body is not None else None, method=method, headers={'Content-Type': 'application/json'})
        with urllib.request.urlopen(req, timeout=35) as response:
            data = response.read()
            return json.loads(data) if data else None
    def wait(fn, label, seconds=90):
        end = time.monotonic() + seconds
        last = None
        while time.monotonic() < end:
            try:
                value = fn()
                if value:
                    return value
            except (OSError, ValueError, KeyError) as error:
                last = str(error)
            time.sleep(.5)
        raise RuntimeError('Timed out waiting for ' + label + ': ' + str(last))
    def mux(*words, check=True):
        return subprocess.run([tmux, '-L', socket_name, *words], env=env, capture_output=True, text=True, timeout=10, check=check)
    def native():
        result = mux('list-panes', '-a', '-F', '#{session_name} #{pane_pid}', check=False)
        rows = [line.split() for line in result.stdout.splitlines()]
        assert all(row[0] == pane for row in rows) and len(rows) <= 1, 'Unexpected native process duplication'
        return {'count': len(rows), 'pane_pid': int(rows[0][1]) if rows else None}
    log = (fixture / 'controller.log').open('ab')
    controller, sid, daemon_started = None, None, False
    phases = []
    original_settings_preserved = None
    def start():
        nonlocal controller
        controller = subprocess.Popen([str(binary), '-listen', '127.0.0.1:' + str(web_port)], env=env, stdin=subprocess.DEVNULL, stdout=log, stderr=log)
        def ready():
            if controller.poll() is not None:
                raise RuntimeError('Private WebMux controller exited during startup; inspect controller.log')
            return api('/api/health')['status'] == 'ok'
        wait(ready, 'private controller')
    def stop(abrupt=False):
        nonlocal controller
        if controller and controller.poll() is None:
            controller.kill() if abrupt else controller.terminate()
            try:
                controller.wait(timeout=15)
            except subprocess.TimeoutExpired:
                controller.kill()
                controller.wait(timeout=5)
        controller = None
    def view():
        return api('/api/session-recovery/' + sid)
    def connected():
        s = api('/api/sessions/' + sid)
        clients = mux('list-clients', '-F', '#{session_name}', check=False)
        return s if s['state'] == 'connected' and clients.stdout.splitlines().count(pane) == 1 else None
    def receipt(label, **data):
        phases.append({'phase': label, **data})
        (fixture / 'lifecycle-phases.json').write_text(json.dumps(phases, indent=2) + '\n')
        print(json.dumps({'phase': label, 'passed': True}), flush=True)
    def records():
        found = {}
        for number, line in enumerate(native_file.read_bytes().splitlines()):
            try:
                row = json.loads(line)
            except ValueError:
                continue
            if row.get('type') == 'assistant':
                found[str(number)] = ''.join(c.get('text', '') for c in row.get('message', {}).get('content', []) if c.get('type') == 'text')
        return found
    bridge = "import fs from 'node:fs';import WebSocket from 'ws';const i=JSON.parse(fs.readFileSync(0,'utf8'));const w=new WebSocket(i.url);const t=setTimeout(()=>{w.terminate();process.exitCode=1},7000);w.on('error',()=>{clearTimeout(t);process.exitCode=1});w.on('open',()=>{w.send(JSON.stringify({type:'input',data:i.text}));setTimeout(()=>w.send(JSON.stringify({type:'input',data:'\\r'})),1000);setTimeout(()=>{clearTimeout(t);w.close()},1500)});"
    try:
        version = rig('--version').stdout.strip()
        assert version == '0.6.14 (9d0200f3)', 'This fixture qualifies only the exact installed Rig revision'
        rig('daemon', 'start', '--host', '127.0.0.1', '--port', str(rig_port), '--db', str(rig_home / 'openrig.sqlite'), '--no-kernel')
        daemon_started = True
        spec = fixture / 'rig.json'
        spec.write_text(json.dumps({'version': '0.2', 'name': 'webmux-recovery', 'summary': 'Private recovery qualification; no kernel or agent launch', 'pods': [{'id': 'agents', 'label': 'Recovery', 'members': []}], 'edges': []}))
        if args.native_resume:
            package = fixture / 'agent'
            package.mkdir(mode=0o700)
            (package / 'agent.yaml').write_text(json.dumps({'name': 'webmux-recovery-native', 'version': '1.0', 'description': 'Private no-tool native recovery fixture', 'profiles': {'default': {'uses': {'skills': [], 'guidance': [], 'subagents': [], 'plugins': [], 'runtime_resources': []}}}, 'startup': {'files': []}}))
            definition = json.loads(spec.read_text())
            definition['pods'][0]['members'] = [{'id': 'continuation', 'agent_ref': 'local:agent', 'runtime': 'claude-code', 'profile': 'default', 'cwd': entry['project']}]
            spec.write_text(json.dumps(definition))
            rig_id = None
        else:
            created = json.loads(rig('up', str(spec), '--yes', '--json').stdout)
            rig_id = created['rigId']
        registration_file = fixture / 'attached.json'
        launcher = fixture / 'native-launch.py'
        native_args = [claude, '--resume', entry['native_session_id'], '--restricted', '--strict-mcp-config', '--tools', '', '--permission-mode', 'plan']
        routing = ''
        relay_sha256 = None
        if args.native_hooks:
            relay_source = rig_cli.parent.parent / 'daemon/assets/plugins/openrig-core/hooks/scripts/activity-relay.cjs'
            relay = fixture / 'activity-relay.cjs'
            relay.write_bytes(relay_source.read_bytes())
            relay_sha256 = hashlib.sha256(relay.read_bytes()).hexdigest()
            hook_settings = {'hooks': {'SessionStart': [{'hooks': [{'type': 'command', 'command': shlex.join([str(rig_node), str(relay)]), 'timeout': 5}]}]}}
            native_args.remove('--restricted')
            native_args += ['--settings', json.dumps(hook_settings)]
            routing = 'env.update(json.loads(r.stdout)["env"])\nenv["OPENRIG_SEAT_PROJECT_DIR"]=' + repr(entry['project']) + '\n'
        if args.native_resume:
            # The manager owns this private executable and its fixed policy.
            # Preserve every argument it supplies; refuse a bypass/changed mode.
            wrapper = '#!' + sys.executable + '\nimport os,sys,json\nroots=' + repr(roots) + '\nenv=dict(os.environ)\nfor key,value in roots.items():\n if value is None: env.pop(key,None)\n else: env[key]=value\na=sys.argv[1:]\nrequested=list(a)\n'
            wrapper += 'with open(' + repr(str(fixture / 'native-requests.jsonl')) + ',"a") as f: f.write(json.dumps({"args":a})+chr(10))\n'
            wrapper += 'if any(x in a for x in ["--dangerously-skip-permissions","--allow-dangerously-skip-permissions"]): raise SystemExit("Fixture refuses permission bypass")\n'
            wrapper += 'if "--permission-mode" in a and a[a.index("--permission-mode")+1] not in ["acceptEdits","plan"]: raise SystemExit("Fixture refuses changed permission mode")\n'
            wrapper += 'if not any(x in a for x in ["--version","--help","-h"]):\n'
            wrapper += ' hook=' + repr(hook_settings['hooks']['SessionStart']) + '\n'
            wrapper += ' found=False\n for i,key in enumerate(a):\n  if key=="--settings":\n   config=json.loads(a[i+1])\n   assert isinstance(config,dict)\n   hooks=config.setdefault("hooks",{})\n   events=hooks.setdefault("SessionStart",[])\n   assert isinstance(events,list)\n   events.extend(hook)\n   a[i+1]=json.dumps(config,separators=(",",":"))\n   found=True\n'
            wrapper += '   deny=config.setdefault("permissions",{}).setdefault("deny",[])\n   assert isinstance(deny,list)\n   if "*" not in deny: deny.append("*")\n   a[i+1]=json.dumps(config,separators=(",",":"))\n'
            wrapper += ' if not found: a += ["--settings",' + repr(json.dumps(dict(hook_settings, permissions={'deny': ['*']}))) + ']\n'
            wrapper += 'with open(' + repr(str(fixture / 'native-executions.jsonl')) + ',"a") as f: f.write(json.dumps({"pid":os.getpid(),"requested":requested,"args":a,"cwd":os.getcwd(),"roots":{key:env.get(key) for key in roots}})+chr(10))\n'
            wrapper += 'os.execve(' + repr(claude) + ',[' + repr(claude) + ',*a],env)\n'
            compile(wrapper, str(aliases / 'claude'), 'exec')
            executable(aliases / 'claude', wrapper)
        if args.native_resume:
            started = json.loads(rig('up', str(spec), '--yes', '--json').stdout)
            rig_id = started['rigId']
            if not args.keep_team_default:
                rig('seat', 'set-permissions', pane, '--mode', 'plan', '--operator', 'webmux-recovery-fixture', '--reason', 'Match the live fixture plan mode for future continuation', '--json')
                mux('send-keys', '-t', pane, '-l', '/plan')
                mux('send-keys', '-t', pane, 'Enter')
                wait(lambda: 'plan mode' in mux('capture-pane', '-pt', pane, check=False).stdout.lower(), 'native plan mode before the first model prompt')
            with urllib.request.urlopen('http://127.0.0.1:' + str(rig_port) + '/api/seat/status/' + urllib.parse.quote(pane, safe=''), timeout=10) as response:
                initial_permissions = json.loads(response.read())
            (fixture / 'initial-seat-permissions.json').write_text(json.dumps(initial_permissions, indent=2) + '\n')
            def managed_identity():
                with urllib.request.urlopen('http://127.0.0.1:' + str(rig_port) + '/api/rigs/' + rig_id + '/sessions', timeout=10) as response:
                    rows = json.loads(response.read())
                candidates = [row for row in rows if row.get('sessionName') == pane and row.get('resumeType') == 'claude_id' and row.get('resumeToken')]
                return candidates[-1] if candidates else None
            managed = wait(managed_identity, 'native Rig-managed initial conversation identity')
            uuid.UUID(managed['resumeToken'])
            entry = dict(entry, native_session_id=managed['resumeToken'], marker='WM_RECOVERY_RIG_' + uuid.uuid4().hex)
            registration = {'attachmentType': 'tmux', 'sessionId': managed['id'], 'nodeId': managed['nodeId'], 'env': {'OPENRIG_SESSION_NAME': pane}}
            registration_file.write_text(json.dumps(registration))
            def locate_native_state():
                files = list((native_root / 'projects').glob('*/' + entry['native_session_id'] + '.jsonl'))
                return files[0] if len(files) == 1 else None
            # The first prompt creates the native transcript; use the live TTY
            # only after native Rig has confirmed the managed CLI start.
            mux('send-keys', '-t', pane, '-l', 'Remember this exact marker for later: ' + entry['marker'] + '. Reply with the marker. Do not use tools.')
            mux('send-keys', '-t', pane, 'Enter')
            native_file = wait(locate_native_state, 'fresh managed native transcript')
            wait(lambda: any(entry['marker'] in text for text in records().values()), 'fresh managed native marker response')
        else:
            attach_args = [str(aliases / 'rig'), 'attach', '--self', '--rig', rig_id]
            attach_args += ['--node', 'agents.continuation'] if args.native_resume else ['--pod', 'agents', '--member', 'continuation']
            attach_args += ['--runtime', 'claude-code', '--cwd', entry['project'], '--json']
            executable(launcher, '#!' + sys.executable + '\nimport os,sys,json,subprocess\n' +
                       'env=dict(os.environ)\nr=subprocess.run(' + repr(attach_args) + ',env=env,capture_output=True,text=True,check=True)\n' +
                       'open(' + repr(str(registration_file)) + ',"w").write(r.stdout)\n' + routing + 'roots=' + repr(roots) + '\nfor key,value in roots.items():\n if value is None: env.pop(key,None)\n else: env[key]=value\n' +
                       'os.chdir(' + repr(entry['project']) + ')\nos.execve(' + repr(claude) + ',' + repr(native_args) + ',env)\n')
            mux('new-session', '-d', '-s', pane, '-x', '120', '-y', '40', shlex.join([sys.executable, str(launcher)]))
            registration = wait(lambda: json.loads(registration_file.read_text()) if registration_file.exists() else None, 'native Rig enrollment')
            assert registration.get('attachmentType') == 'tmux' and registration.get('sessionId')
            target = registration['env']['OPENRIG_SESSION_NAME']
        target = registration['env']['OPENRIG_SESSION_NAME']
        scope = hashlib.sha256(b'anonymous').hexdigest()[:24]
        directory = home / 'data/coordination' / scope
        directory.mkdir(parents=True, mode=0o700)
        agent = {'target': target, 'project': 'webmux-recovery', 'rigId': rig_id, 'rigName': 'webmux-recovery', 'pod': 'agents', 'name': 'continuation', 'host': 'localhost', 'cwd': entry['project'], 'runtime': 'claude-code', 'tmux': pane, 'sessionId': registration['sessionId']}
        (directory / (pane + '.agent.json')).write_text(json.dumps(agent))
        def native_ready():
            text = mux('capture-pane', '-pt', pane, '-S', '-200', check=False).stdout
            if 'Make auto mode your default permission mode?' in text:
                assert 'No, keep bypass permissions' in text
                lines = text.splitlines()
                if any(line.lstrip().startswith('❯ Yes,') for line in lines):
                    mux('send-keys', '-t', pane, 'Down', 'Enter')
                elif any(line.lstrip().startswith('❯ No,') for line in lines):
                    mux('send-keys', '-t', pane, 'Enter')
                else:
                    raise RuntimeError('Unknown permission-migration menu; no input sent')
                return None
            return entry['marker'] in text
        wait(native_ready, 'native Claude history')
        before = native()
        assert before['count'] == 1
        start()
        sid = api('/api/coordination/attach', {'provider': 'rig', 'target': target})['id']
        wait(connected, 'managed Rig viewer')
        wait(lambda: entry['marker'] in view()['history'], 'retained native history')
        assert view()['manager_target'] == target and view()['lifecycle_owner'] == 'rig'
        if args.native_resume:
            captured = view()
            assert captured.get('manager_native_session') == entry['native_session_id'], 'WebMux did not capture the verified native conversation'
            assert captured.get('manager_native_state_file') == str(native_file)
            assert captured.get('manager_native_source') == 'rig-snapshot' and captured.get('manager_snapshot_id')
            api('/api/coordination/attach', {'provider': 'rig', 'target': target})
            assert view()['manager_snapshot_id'] == captured['manager_snapshot_id'], 'Unchanged metadata created another snapshot'
            captured_session = api('/api/sessions/' + sid)
            captured_identity = captured_session['rig']
            if not captured_identity.get('manager_process'):
                source = home / 'data/coordination' / hashlib.sha256(captured_session['owner'].encode()).hexdigest()[:24] / 'rig-recovery.mts'
                probe_js = "import fs from 'node:fs';import{execFileSync}from'node:child_process';import{pathToFileURL}from'node:url';const m=await import(pathToFileURL(process.argv[1]));const i=JSON.parse(fs.readFileSync(0,'utf8'));const run=(c,a)=>execFileSync(c,a,{encoding:'utf8',timeout:25000,maxBuffer:4194304}).trim();console.log(JSON.stringify({process:m.observeRigNativeProcess(run,i,true),boot:m.rigBootIdentity(run)}));"
                probe = subprocess.run([str(rig_node), '--input-type=module', '-e', probe_js, str(source)], input=json.dumps(captured_identity), env=env, capture_output=True, text=True, timeout=35)
                (fixture / 'initial-process-diagnostic.json').write_text(json.dumps({'returncode': probe.returncode, 'stdout': probe.stdout, 'stderr': probe.stderr}, indent=2) + '\n')
                own = [json.loads(line) for line in (fixture / 'native-executions.jsonl').read_text().splitlines() if '--permission-mode' in json.loads(line)['args'] and '--resume' not in json.loads(line)['args']]
                if len(own) == 1:
                    observed = subprocess.run(['ps', '-ww', '-p', str(own[0]['pid']), '-o', 'pid=,ppid=,args='], capture_output=True, text=True, timeout=5)
                    (fixture / 'initial-kernel-process.txt').write_text(observed.stdout)
                    (fixture / 'initial-kernel-process.txt').chmod(0o600)
            original_process = json.loads(captured_identity['manager_process'])
            assert original_process['pane_pid'] == before['pane_pid']
            initial_execs = [json.loads(line) for line in (fixture / 'native-executions.jsonl').read_text().splitlines()]
            initial_clis = [row for row in initial_execs if '--name' in row['args'] and row['args'][row['args'].index('--name') + 1] == pane and '--resume' not in row['args']]
            assert len(initial_clis) == 1 and original_process['native_pid'] == initial_clis[0]['pid']
            assert original_process['native_session_id'] == entry['native_session_id'] and original_process['native_identity_source'] == 'process-argv'
            assert len(original_process['boot_id']) == 64 and original_process['started_at']
            assert len(original_process['settings_hash']) == 64 and len(original_process['executable_hash']) == 64
            (fixture / 'original-native-process.json').write_text(json.dumps(original_process, indent=2) + '\n')

            owner_scope = hashlib.sha256(captured_session['owner'].encode()).hexdigest()[:24]
            runners = [home / 'data/coordination' / owner_scope / 'runner.mts']
            assert runners[0].is_file()
            def webmux_inspect(label):
                inspection = subprocess.run([str(rig_node), str(runners[0]), 'recovery-inspect', str(runners[0].parent)], input=json.dumps(captured_identity), env=env, capture_output=True, text=True, timeout=35)
                (fixture / ('webmux-process-inspection-' + label + '.json')).write_text(json.dumps({'returncode': inspection.returncode, 'stdout': inspection.stdout, 'stderr': inspection.stderr}, indent=2) + '\n')
                return inspection
            live_inspection = webmux_inspect('live')
            assert live_inspection.returncode == 0 and json.loads(live_inspection.stdout)['reason'] == 'original-process'

            def webmux_checkpoint_plan(label):
                checkpoint = subprocess.run([str(rig_node), str(runners[0]), 'recovery-plan', str(runners[0].parent)], input=json.dumps(captured_identity), env=env, capture_output=True, text=True, timeout=35)
                (fixture / ('webmux-checkpoint-plan-' + label + '.json')).write_text(json.dumps({'returncode': checkpoint.returncode, 'stdout': checkpoint.stdout, 'stderr': checkpoint.stderr}, indent=2) + '\n')
                assert checkpoint.returncode == 0, 'WebMux could not validate its recorded Rig checkpoint; inspect checkpoint plan receipt'
                planned_checkpoint = json.loads(checkpoint.stdout)
                assert planned_checkpoint['plan_only'] is True and planned_checkpoint['snapshot_id'] == captured['manager_snapshot_id']
                assert planned_checkpoint['native_session_id'] == entry['native_session_id'] and planned_checkpoint['target'] == target
                return planned_checkpoint
            assert native() == before
            webmux_checkpoint_plan('live')
            assert native() == before, 'Read-only WebMux preflight replaced the surviving process'
            if args.webmux_action:
                live_preview = view()
                live_action = {'id': str(uuid.uuid4()), 'kind': 'managed', 'confirm': True, 'revision': live_preview['state']['revision'], 'host_revision': 0, 'instance': live_preview['instance']}
                assert api('/api/session-recovery/' + sid, live_action)['outcome'] == 'consumed'
                assert api('/api/session-recovery/' + sid, live_action)['outcome'] == 'consumed'
                assert native() == before, 'Recovery action replaced the surviving provider'
                receipt('WebMux managed action reattaches survivor and deduplicates receipt', native=native())


        else:
            assert not view().get('manager_native_session'), 'Rig enrollment ID was mistaken for a provider-native conversation'
        seats = [seat for seat in api('/api/coordination')['seats'] if seat['provider'] == 'rig' and seat['id'] == target]
        assert len(seats) == 1 and not seats[0].get('nativeSessionId'), 'Coordination inventory mislabeled the Rig enrollment ID as a native CLI conversation'
        assert seats[0].get('managerSessionId') == registration['sessionId'], 'Coordination inventory lost the manager enrollment identity'
        receipt('initial native Rig attachment', native=before, preview=view(), registration_session_id=registration['sessionId'], native_session_id=entry['native_session_id'])
        for abrupt in (False, True):
            stop(abrupt)
            assert native() == before
            start()
            wait(connected, 'restored Rig viewer')
            v = wait(lambda: view() if view().get('state', {}).get('disposition') == 'reattached' else None, 'Rig restoration receipt')
            assert native() == before and entry['marker'] in v['history']
            if args.native_resume:
                assert v['manager_snapshot_id'] == captured['manager_snapshot_id'] and v['manager_native_session'] == captured['manager_native_session']
                assert api('/api/sessions/' + sid)['rig']['manager_process'] == captured_identity['manager_process'], 'Original native process proof changed across controller restart'
            receipt('controller SIGKILL' if abrupt else 'orderly controller restart', native=native(), preview=v)
        for _ in range(3):
            api('/api/sessions/' + sid + '/reconnect', {})
            wait(connected, 'repeated Rig reconnect')
            assert native() == before
        prior, nonce = records(), 'AFTER_RIG_RECOVERY_' + uuid.uuid4().hex
        text = 'What exact marker did I ask you to remember earlier? Reply with the marker followed by ' + nonce + '. Do not use tools.'
        subprocess.run([str(rig_node), '--input-type=module', '-e', bridge], cwd=repo / 'webmux', input=json.dumps({'url': base.replace('http:', 'ws:') + '/api/term/' + sid, 'text': text}), text=True, timeout=12, check=True)
        answer = wait(lambda: next((value for key, value in records().items() if key not in prior and entry['marker'] in value and nonce in value), None), 'new Rig-owned native conversation response')
        receipt('three reconnects and native continuity', native=native(), nonce=nonce, new_assistant_answer=answer)
        stop()
        def kill_private_daemon():
            state = json.loads((rig_home / 'daemon.json').read_text())
            assert state['port'] == rig_port and state['host'] == '127.0.0.1'
            assert Path(state['db']).resolve() == (rig_home / 'openrig.sqlite').resolve()
            with urllib.request.urlopen('http://127.0.0.1:' + str(rig_port) + '/healthz', timeout=5) as response:
                health = json.loads(response.read())
            assert health['pid'] == state['pid'] and state['pid'] > 1
            os.kill(state['pid'], signal.SIGKILL)
            def exited():
                status = subprocess.run(['ps', '-p', str(state['pid']), '-o', 'stat='], capture_output=True, text=True, timeout=5)
                return not status.stdout.strip() or status.stdout.lstrip().startswith('Z')
            wait(exited, 'confirmed private Rig daemon termination')
            assert native() == before
            return state['pid']
        previous_pid = kill_private_daemon()
        start()
        wait(connected, 'automatic private Rig manager recovery')
        replacement = json.loads((rig_home / 'daemon.json').read_text())
        assert replacement['pid'] != previous_pid and native() == before
        receipt('private Rig daemon SIGKILL and automatic manager recovery', native=native(), daemon_pid_before=previous_pid, daemon_pid_after=replacement['pid'], preview=view())
        stop()
        restart_block.write_text('The fixture temporarily refuses manager startup to exercise unavailable-manager fallback.\n')
        previous_pid = kill_private_daemon()
        start()
        v = wait(lambda: view() if view().get('state', {}).get('disposition') == 'managed-unavailable' else None, 'unavailable Rig manager fallback')
        assert entry['marker'] in v['history'] and native() == before
        try:
            api('/api/sessions/' + sid + '/reconnect', {})
        except urllib.error.HTTPError as error:
            assert error.code == 409
        else:
            raise AssertionError('Disconnected Rig manager was bypassed')
        assert native() == before
        receipt('private Rig daemon loss with fixture-denied startup', native=native(), daemon_pid=previous_pid, preview=v)
        stop()
        restart_block.unlink()
        rig('daemon', 'start', '--host', '127.0.0.1', '--port', str(rig_port), '--db', str(rig_home / 'openrig.sqlite'), '--no-kernel')
        replacement = json.loads((rig_home / 'daemon.json').read_text())
        assert replacement['pid'] != previous_pid
        start()
        wait(connected, 'same native conversation after Rig manager recovery')
        assert native() == before and not view().get('state', {}).get('manager_message')
        receipt('private Rig manager recovery after startup refusal', native=native(), daemon_pid=replacement['pid'], preview=view())
        snapshot_id = None
        if args.native_resume and not args.keep_team_default:
            rig('seat', 'set-permissions', target, '--mode', 'plan', '--operator', 'webmux-recovery-fixture', '--reason', 'Private native recovery qualification with tools disabled', '--json')
        if args.snapshot_plan:
            snapshot_output = rig('snapshot', rig_id, '--intended-seats', 'agents.continuation').stdout
            prefix = 'Snapshot created: '
            snapshot_id = next(line[len(prefix):].strip() for line in snapshot_output.splitlines() if line.startswith(prefix))
            assert len(snapshot_id) == 26 and snapshot_id.isalnum()
            with urllib.request.urlopen('http://127.0.0.1:' + str(rig_port) + '/api/rigs/' + rig_id + '/snapshots/' + snapshot_id, timeout=10) as response:
                snapshot = json.loads(response.read())
            (fixture / 'native-snapshot.json').write_text(json.dumps(snapshot, indent=2) + '\n')
            if args.native_resume:
                for label, route in [('generation-record', '/api/sessions/' + urllib.parse.quote(pane, safe='') + '/generation-record'), ('node-detail', '/api/rigs/' + rig_id + '/nodes/agents.continuation'), ('seat-status', '/api/seat/status/' + urllib.parse.quote(pane, safe=''))]:
                    try:
                        with urllib.request.urlopen('http://127.0.0.1:' + str(rig_port) + route, timeout=10) as response:
                            value = response.read(2 * 1024 * 1024 + 1)
                            assert len(value) <= 2 * 1024 * 1024
                            observed = {'status': response.status, 'data': json.loads(value)}
                    except urllib.error.HTTPError as error:
                        observed = {'status': error.code}
                    (fixture / (label + '-metadata.json')).write_text(json.dumps(observed, indent=2) + '\n')
            if args.native_hooks:
                selected = [row for row in snapshot['data']['sessions'] if row['id'] == registration['sessionId']]
                assert len(selected) == 1
                if args.native_resume:
                    assert snapshot['data']['nodeStartupContext'][registration['nodeId']] is not None, 'Rig did not persist its managed startup context'
                assert selected[0]['resumeType'] == 'claude_id' and selected[0]['resumeToken'] == entry['native_session_id']
                assert selected[0]['resumeProvenance'] == 'hook', 'native Rig callback did not establish the snapshot identity'
                receipt('native Rig SessionStart identity retained through manager lifecycles', native=native(), native_provenance=selected[0]['resumeProvenance'], relay_sha256=relay_sha256)
            planned = rig('launch', rig_id, 'agents.continuation', '--snapshot-id', snapshot_id, '--plan', '--json', check=False)
            (fixture / 'live-snapshot-plan.json').write_text(json.dumps({'returncode': planned.returncode, 'stdout': planned.stdout, 'stderr': planned.stderr}, indent=2) + '\n')
            plan = json.loads(planned.stdout)
            assert planned.returncode == 0 and plan.get('ok') is True and plan.get('planOnly') is True
            assert plan['snapshotSelection']['snapshotId'] == snapshot_id
            assert plan['targetNodes'] == [{'nodeId': registration['nodeId'], 'logicalId': 'agents.continuation'}]
            assert not plan['unmatchedIds'] and plan['nonTargetEffects'] == {'mode': 'unchanged', 'reason': None, 'affected': []}
            assert native() == before
            receipt('private snapshot capture and live exact-seat plan only', native=native(), snapshot_id=snapshot_id, plan_returncode=planned.returncode)
        stop()
        mux('kill-session', '-t', '=' + pane)
        assert native()['count'] == 0
        if snapshot_id:
            planned = rig('launch', rig_id, 'agents.continuation', '--snapshot-id', snapshot_id, '--plan', '--json', check=False)
            (fixture / 'lost-snapshot-plan.json').write_text(json.dumps({'returncode': planned.returncode, 'stdout': planned.stdout, 'stderr': planned.stderr}, indent=2) + '\n')
            plan = json.loads(planned.stdout)
            assert planned.returncode == 0 and plan.get('ok') is True and plan.get('planOnly') is True
            assert plan['snapshotSelection']['snapshotId'] == snapshot_id
            assert plan['targetNodes'] == [{'nodeId': registration['nodeId'], 'logicalId': 'agents.continuation'}]
            assert not plan['unmatchedIds'] and plan['nonTargetEffects'] == {'mode': 'unchanged', 'reason': None, 'affected': []}
            assert native()['count'] == 0
            receipt('lost native process exact-seat snapshot plan only', native=native(), snapshot_id=snapshot_id, plan_returncode=planned.returncode)
        start()
        v = wait(lambda: view() if view().get('state', {}).get('disposition') == 'managed-unavailable' else None, 'lost Rig provider fallback')
        assert native()['count'] == 0 and entry['marker'] in v['history'] and v['can_resume_managed'] == bool(args.native_resume)
        for _ in range(3):
            try:
                api('/api/sessions/' + sid + '/reconnect', {})
            except urllib.error.HTTPError as error:
                assert error.code == 409
            else:
                raise AssertionError('Lost Rig process silently restarted')
            assert native()['count'] == 0
        receipt('native process loss and repeated manual fallback', native=native(), preview=view())
        if args.native_resume:
            webmux_checkpoint_plan('lost')
            lost_inspection = webmux_inspect('lost')
            assert lost_inspection.returncode == 0 and json.loads(lost_inspection.stdout) == {'state': 'missing', 'reason': 'process-loss'}
            assert native()['count'] == 0, 'Read-only WebMux preflight launched a replacement'
            snapshot_id = captured['manager_snapshot_id']
            receipt('WebMux recorded checkpoint live and lost-process preflight', snapshot_id=snapshot_id, native=native())
            if args.webmux_action:
                action_preview = view()
                recovery_action = {'id': str(uuid.uuid4()), 'kind': 'managed', 'confirm': True, 'revision': action_preview['state']['revision'], 'host_revision': 0, 'instance': action_preview['instance']}
                assert api('/api/session-recovery/' + sid, recovery_action)['outcome'] == 'consumed'
                resumed_identity = native()
                assert resumed_identity['count'] == 1
                assert api('/api/session-recovery/' + sid, recovery_action)['outcome'] == 'consumed'
                assert native() == resumed_identity
                stop(True)
                start()
                wait(connected, 'recovery receipt retained across controller SIGKILL')
                assert api('/api/session-recovery/' + sid, recovery_action)['outcome'] == 'consumed'
                assert native() == resumed_identity
                receipt('WebMux approved native action and durable once-only receipt across restart', native=native())
            else:
                result = rig('launch', rig_id, 'agents.continuation', '--snapshot-id', snapshot_id, '--json', check=False)
                (fixture / 'native-resume-result.json').write_text(json.dumps({'returncode': result.returncode, 'stdout': result.stdout, 'stderr': result.stderr}, indent=2) + '\n')
                if result.returncode:
                    outcome = json.loads(result.stdout)
                    if outcome.get('code') != 'attention_required' or 'joined restore proof is incomplete' not in outcome.get('error', ''):
                        raise RuntimeError('Native Rig did not confirm exact-snapshot resume; inspect native-resume-result.json')
                    pending = native()
                    assert pending['count'] == 1
                    reconciled = rig('reconcile-session', pane, '--rig', rig_id, '--node', 'agents.continuation', '--no-launch', '--json', check=False)
                    (fixture / 'native-reconcile-result.json').write_text(json.dumps({'returncode': reconciled.returncode, 'stdout': reconciled.stdout, 'stderr': reconciled.stderr}, indent=2) + '\n')
                    assert native() == pending, 'Reconciliation replaced the native process'
                    if reconciled.returncode:
                        raise RuntimeError('Native Rig could not reconcile the existing resumed process without launch; inspect native-reconcile-result.json')
            after = wait(lambda: native() if native()['count'] == 1 else None, 'Rig-owned resumed native process')
            assert after['pane_pid'] != before['pane_pid']
            api('/api/sessions/' + sid + '/reconnect', {})
            wait(connected, 'Rig-owned resumed native viewer')
            wait(native_ready, 'resumed native history')
            prior, nonce = records(), 'AFTER_RIG_NATIVE_RESUME_' + uuid.uuid4().hex
            text = 'What exact marker did I ask you to remember earlier? Reply with the marker followed by ' + nonce + '. Do not use tools.'
            subprocess.run([str(rig_node), '--input-type=module', '-e', bridge], cwd=repo / 'webmux', input=json.dumps({'url': base.replace('http:', 'ws:') + '/api/term/' + sid, 'text': text}), text=True, timeout=12, check=True)
            answer = wait(lambda: next((value for key, value in records().items() if key not in prior and entry['marker'] in value and nonce in value), None), 'new native answer after Rig-owned snapshot resume')
            executions = [json.loads(line) for line in (fixture / 'native-executions.jsonl').read_text().splitlines()]
            resumes = [row for row in executions if '--resume' in row['args'] and row['args'][row['args'].index('--resume') + 1] == entry['native_session_id']]
            assert len(resumes) == 1, 'Manager launched more than one native resume'
            resumed = resumes[0]
            assert resumed['roots'] == roots and Path(resumed['cwd']).samefile(entry['project'])
            argv, requested = resumed['args'], resumed['requested']
            assert argv.count('--permission-mode') == 1 and argv[argv.index('--permission-mode') + 1] == ('acceptEdits' if args.keep_team_default else 'plan')
            assert '--fork-session' not in argv and '--session-id' not in argv
            configurations = [json.loads(argv[i + 1]) for i, key in enumerate(argv) if key == '--settings']
            assert len(configurations) == 1 and '*' in configurations[0]['permissions']['deny']
            configuration = configurations[0]
            for index, key in enumerate(requested):
                if key != '--settings':
                    continue
                original = json.loads(requested[index + 1])
                for name, value in original.items():
                    if name == 'hooks':
                        for event, groups in value.items():
                            assert configuration['hooks'][event][:len(groups)] == groups
                    elif name == 'permissions':
                        for permission, rules in value.items():
                            if permission == 'deny':
                                assert configuration['permissions']['deny'][:len(rules)] == rules
                            else:
                                assert configuration['permissions'][permission] == rules
                    else:
                        assert configuration[name] == value
            parents = {}
            for line in subprocess.check_output(['ps', '-axo', 'pid=,ppid='], text=True, timeout=5).splitlines():
                child, parent = line.split()
                parents[int(child)] = int(parent)
            cursor = resumed['pid']
            for _ in range(64):
                if cursor == after['pane_pid']:
                    break
                cursor = parents.get(cursor, 0)
                if not cursor:
                    raise AssertionError('Resumed CLI does not descend from the verified private pane')
            else:
                raise AssertionError('Native ancestry exceeds the qualification bound')
            if sys.platform.startswith('linux'):
                kernel_cwd = os.readlink('/proc/' + str(resumed['pid']) + '/cwd')
            else:
                output = subprocess.check_output(['lsof', '-a', '-p', str(resumed['pid']), '-d', 'cwd', '-Fn'], text=True, timeout=5)
                paths = [line[1:] for line in output.splitlines() if line.startswith('n')]
                assert len(paths) == 1
                kernel_cwd = paths[0]
            assert Path(kernel_cwd).samefile(entry['project'])
            # Observe only this fixture-owned native PID. Retain argv privately
            # to design a kernel-backed production check without reading other
            # users' process arguments or treating the pane shell as the CLI.
            native_process = subprocess.check_output(['ps', '-p', str(resumed['pid']), '-o', 'pid=,ppid=,lstart=,comm=,args='], text=True, timeout=5)
            (fixture / 'native-kernel-process.txt').write_text(native_process)
            (fixture / 'native-kernel-process.txt').chmod(0o600)
            receipt('fixture-owned native kernel identity', native_pid=resumed['pid'], native_id_in_kernel_argv=entry['native_session_id'] in native_process)
            process_proof = subprocess.run([str(rig_node), str(runners[0]), 'recovery-process', str(runners[0].parent)], input=json.dumps(captured_identity), env=env, capture_output=True, text=True, timeout=35)
            (fixture / 'webmux-native-process-proof.json').write_text(json.dumps({'returncode': process_proof.returncode, 'stdout': process_proof.stdout, 'stderr': process_proof.stderr}, indent=2) + '\n')
            assert process_proof.returncode == 0, 'WebMux could not independently prove the resumed native process'
            proof = json.loads(process_proof.stdout)
            assert proof['native_pid'] == resumed['pid'] and proof['pane_pid'] == after['pane_pid']
            assert proof['native_session_id'] == entry['native_session_id'] and proof['permission_mode'] == ('acceptEdits' if args.keep_team_default else 'plan')
            assert proof['started_at'] and len(proof['executable_hash']) == 64 and len(proof['settings_hash']) == 64
            receipt('WebMux native process, executable, ID and kernel cwd proof', proof=proof)
            original_configuration = json.loads(initial_clis[0]['args'][initial_clis[0]['args'].index('--settings') + 1])
            original_settings_preserved = original_configuration == configuration
            assert proof['executable_hash'] == original_process['executable_hash']
            inspected_resume = webmux_inspect('resumed')
            policy_comparison = {'original_settings_preserved': original_settings_preserved, 'original_hook_events': sorted(original_configuration.get('hooks', {})), 'resumed_hook_events': sorted(configuration.get('hooks', {})), 'original_permission_fields': sorted(original_configuration.get('permissions', {})), 'resumed_permission_fields': sorted(configuration.get('permissions', {}))}
            (fixture / 'original-settings-comparison.json').write_text(json.dumps(policy_comparison, indent=2) + '\n')
            if args.expect_policy_drift:
                assert not original_settings_preserved, 'Expected policy drift disappeared; qualify the strict original-policy path instead'
                assert inspected_resume.returncode != 0 and 'original settings changed' in inspected_resume.stderr
                receipt('Original native settings drift rejected by WebMux', **policy_comparison)
            else:
                assert original_settings_preserved, 'Rig continuation changed first-launch inline settings; inspect original-settings-comparison.json'
                assert inspected_resume.returncode == 0 and json.loads(inspected_resume.stdout)['reason'] == 'verified-continuation'



            for line in native_file.read_bytes().splitlines():
                try:
                    row = json.loads(line)
                except ValueError:
                    continue
                if row.get('type') == 'assistant':
                    assert not any(part.get('type') == 'tool_use' for part in row.get('message', {}).get('content', [])), 'No-tool fixture executed a model tool call'
            receipt('Rig-owned native snapshot resume and conversation continuity', native=native(), native_pid=resumed['pid'], kernel_cwd=kernel_cwd, roots_preserved=True, manager_supplied_settings_preserved_per_invocation=True, original_settings_preserved=original_settings_preserved, permission_mode='acceptEdits' if args.keep_team_default else 'plan', deny_all_tools=True, nonce=nonce, new_assistant_answer=answer)
        assert (hashlib.sha256(settings.read_bytes()).hexdigest() if settings.is_file() else None) == settings_hash
        assert hashlib.sha256(Path(__file__).read_bytes()).hexdigest() == script_hash
        scope = 'Private native Rig lifecycle, native identity and manual fallback.'
        if args.native_resume:
            scope += ' Includes Rig-owned exact-snapshot conversation continuation, no-launch reconciliation, original roots, kernel cwd and independent original-settings comparison.'
        if args.expect_policy_drift:
            scope += ' The expected original-settings mismatch was rejected by WebMux; this does not qualify policy-preserving recovery.'
        scope += ' WebMux managed action and durable duplicate receipt were exercised.' if args.webmux_action else ' No WebMux Rig native-launch action is qualified.'
        scope += ' Host reboot and remote destinations are not qualified.'
        result = {'passed': True, 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'qualifier_sha256': script_hash, 'rig_version': version, 'snapshot_plan_only': args.snapshot_plan, 'native_hooks': args.native_hooks, 'native_resume': args.native_resume, 'kept_team_default': args.keep_team_default, 'webmux_action': args.webmux_action, 'expected_policy_drift': args.expect_policy_drift, 'original_settings_preserved': original_settings_preserved, 'relay_sha256': relay_sha256, 'native_cli': subprocess.check_output([claude, '--version'], text=True).strip(), 'native_session_id': entry['native_session_id'], 'webmux_session_id': sid, 'phases': phases, 'scope': scope}
        (fixture / 'manifest.json').write_text(json.dumps(result, indent=2) + '\n')
        print(json.dumps({'passed': True, 'manifest': str(fixture / 'manifest.json')}), flush=True)
    except Exception as error:
        panes = mux('list-panes', '-a', '-F', '#{pane_id}', check=False).stdout.splitlines()
        for number, pane_id in enumerate(panes):
            (fixture / ('failure-pane-' + str(number) + '.txt')).write_text(mux('capture-pane', '-pt', pane_id, '-S', '-200', check=False).stdout)
        (fixture / 'failure-native-pane.txt').write_text(mux('capture-pane', '-pt', pane, '-S', '-200', check=False).stdout)
        if sid and controller and controller.poll() is None:
            try:
                (fixture / 'failure-preview.json').write_text(json.dumps(view(), indent=2) + '\n')
            except (OSError, ValueError):
                pass
        (fixture / 'manifest.json').write_text(json.dumps({'passed': False, 'error': str(error), 'error_type': type(error).__name__, 'phases': phases, 'qualifier_sha256': script_hash, 'expected_policy_drift': args.expect_policy_drift, 'original_settings_preserved': original_settings_preserved, 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest()}, indent=2) + '\n')
        print(json.dumps({'passed': False, 'manifest': str(fixture / 'manifest.json')}), flush=True)
        raise
    finally:
        if sid and controller and controller.poll() is None:
            try:
                api('/api/sessions/' + sid, method='DELETE')
            except (OSError, ValueError):
                pass
        stop()
        mux('kill-session', '-t', '=' + pane, check=False)
        stopped = None
        try:
            stopped = rig('daemon', 'stop', check=False) if daemon_started else None
        finally:
            # All panes on this generated socket belong to this fixture, including
            # a manager-created attention shell under a derived canonical name.
            mux('kill-server', check=False)
            remaining = mux('list-panes', '-a', '-F', '#{session_name} #{pane_pid}', check=False)
            cleanup = {'remaining_panes': remaining.stdout}
            if stopped is not None:
                cleanup.update(daemon_stop_returncode=stopped.returncode, daemon_stop_stdout=stopped.stdout, daemon_stop_stderr=stopped.stderr)
            (fixture / 'cleanup.json').write_text(json.dumps(cleanup, indent=2) + '\n')
            log.close()


if __name__ == '__main__':
    main()
