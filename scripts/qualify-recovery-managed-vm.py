#!/usr/bin/env python3
"""Qualify real Agent Deck ownership across a disposable Linux guest reboot.

Uses the retained QEMU fixture only. The default payload is a synthetic shell
job. --native-claude uses the guest's own authenticated Claude installation and
requires fresh conversational responses before and after reboot. No credentials
are copied. The fixture's original user service is restored during cleanup.
"""

import argparse
import hashlib
import json
from pathlib import Path
import shlex
import socket
import subprocess
import time
import urllib.error
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--vm-root', type=Path, required=True)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--native-claude', action='store_true', help='qualify guest-native Claude conversation continuation after actual reboot; requires guest login')
    args = parser.parse_args()
    root = args.vm_root.resolve()
    launch = json.loads((root / 'launch.json').read_text())
    ssh_args = json.loads((root / 'ssh-argv.json').read_text())
    assert ssh_args[-1] == 'webmuxqa@127.0.0.1' and launch['ssh_port'] == 32222
    process = subprocess.run(['ps', '-p', str(launch['pid']), '-o', 'command='], capture_output=True, text=True, check=True)
    assert 'qemu-system-aarch64' in process.stdout and str(root / 'disk.qcow2') in process.stdout
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as connection:
        connection.settimeout(3)
        connection.connect(str(root / 'qmp.sock'))
        stream = connection.makefile('rwb', buffering=0)
        json.loads(stream.readline())
        stream.write(b'{"execute":"qmp_capabilities"}\n')
        json.loads(stream.readline())
        stream.write(b'{"execute":"query-status"}\n')
        assert json.loads(stream.readline())['return']['running']
    fixture = '/home/webmuxqa/qualification/managed-' + uuid.uuid4().hex[:12]
    record_path = root / ('managed-lifecycle-' + uuid.uuid4().hex + '.json')
    binary = args.binary.resolve()
    digest = hashlib.sha256(binary.read_bytes()).hexdigest()
    manifest = {'passed': False, 'scope': 'Agent Deck shell-job ownership and safe fallback across actual guest reboot; not native conversation recovery',
                'fixture': fixture, 'binary_sha256': digest, 'qualifier_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'phases': []}
    if args.native_claude:
        manifest['scope'] = 'Guest-native Agent Deck/Claude conversation continuity across actual host reboot and once-only WebMux recovery'
    native_id = str(uuid.uuid4()) if args.native_claude else None
    native_file = None
    marker = 'MANAGED_REBOOT_' + uuid.uuid4().hex
    base = 'http://127.0.0.1:' + str(launch['http_port'])
    control = 'env XDG_RUNTIME_DIR=/run/user/1000 DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus systemctl --user '
    unit = 'webmux-managed-qualification.service'
    original_unit = 'webmux-qualification.service'
    command_path = '/home/webmuxqa/qualification:/usr/bin:/bin' + (':/home/webmuxqa/.local/bin' if args.native_claude else '')
    deck_env = 'env HOME=' + fixture + '/home XDG_CONFIG_HOME=' + fixture + '/home/.config PATH=' + command_path + ' '
    ident = sid = target = None
    original_stopped = False
    original_enabled = False
    installed_unit = False
    bridge = "import fs from 'node:fs';import WebSocket from 'ws';const i=JSON.parse(fs.readFileSync(0,'utf8'));const w=new WebSocket(i.url);const t=setTimeout(()=>{w.terminate();process.exitCode=1},7000);w.on('error',()=>{clearTimeout(t);process.exitCode=1});w.on('open',()=>{w.send(JSON.stringify({type:'input',data:i.text}));setTimeout(()=>w.send(JSON.stringify({type:'input',data:'\\r'})),1000);setTimeout(()=>{clearTimeout(t);w.close()},1500)});"

    def save():
        record_path.write_text(json.dumps(manifest, indent=2) + '\n')
        record_path.chmod(0o600)

    def ssh(command, data=None, check=True, timeout=20):
        result = subprocess.run(ssh_args[:-1] + ['-o', 'ConnectTimeout=2', ssh_args[-1], command], input=data, capture_output=True, timeout=timeout)
        if check and result.returncode:
            raise RuntimeError('guest command failed: ' + result.stderr.decode(errors='replace')[-600:])
        return result

    def text(command):
        return ssh(command).stdout.decode().strip()

    def put(path, data):
        ssh('cat > ' + shlex.quote(path) + ' && chmod 600 ' + shlex.quote(path), data.encode() if isinstance(data, str) else data)

    def api(path, data=None, method=None):
        request = urllib.request.Request(base + path, data=json.dumps(data).encode() if data is not None else None, method=method, headers={'Content-Type': 'application/json'})
        with urllib.request.urlopen(request, timeout=40 if args.native_claude else 10) as response:
            data = response.read()
            return json.loads(data) if data else None

    def wait(fn, label, seconds=75):
        deadline, last = time.monotonic() + seconds, None
        while time.monotonic() < deadline:
            try:
                value = fn()
                if value:
                    return value
            except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError) as error:
                last = str(error)
            time.sleep(.3)
        raise RuntimeError('timed out waiting for ' + label + ': ' + str(last))

    def deck(*values):
        return json.loads(text(deck_env + 'agent-deck ' + shlex.join(values)))

    def executions():
        return len(text('cat ' + fixture + '/executions').splitlines())

    def boot_id():
        return str(uuid.UUID(text('cat /proc/sys/kernel/random/boot_id')))

    def pane():
        result = ssh(deck_env + 'tmux -L wm-managed-qa list-panes -a -F ' + shlex.quote('#{session_name} #{pane_pid}'), check=False)
        if result.returncode:
            detail = result.stderr.decode().strip()
            assert result.returncode == 1 and (detail.startswith('no server running on /tmp/tmux-1000/') or
                   detail.startswith('error connecting to /tmp/tmux-1000/') and detail.endswith('(No such file or directory)')), detail
            return None
        rows = [row.split() for row in result.stdout.decode().splitlines()]
        assert all(row[0] == target for row in rows), 'unexpected managed pane'
        assert len(rows) <= 1
        return int(rows[0][1]) if rows else None

    def view():
        return api('/api/session-recovery/' + sid)

    def receipt(phase):
        server_pid = int(text(control + 'show ' + unit + ' -p MainPID --value'))
        assert server_pid > 0 and text('readlink /proc/' + str(server_pid) + '/exe') == fixture + '/webmux'
        value = {'phase': phase, 'boot_id': boot_id(), 'server_pid': server_pid,
                 'pane_pid': pane(), 'executions': executions(), 'viewer': api('/api/sessions/' + sid), 'recovery': view()}
        manifest['phases'].append(value)
        save()
        print(json.dumps({'phase': phase, 'passed': True}), flush=True)
        return value

    def native_ready():
        capture = text(deck_env + 'tmux -L wm-managed-qa capture-pane -pt ' + shlex.quote(target) + ' -S -200')
        # Retained only in the failure manifest so an unrecognized native
        # screen is visible as negative evidence. The fixture is synthetic.
        manifest['last_native_pane'] = capture[-4000:]
        # Choose only known native onboarding controls in this new guest-owned
        # project. Never turn terminal output into a recovery command.
        if 'Choose the text style' in capture or 'Choose the text style that looks best' in capture:
            ssh(deck_env + 'tmux -L wm-managed-qa send-keys -t ' + shlex.quote(target) + ' Enter')
            return False
        if 'Security notes:' in capture and 'Press Enter to continue' in capture:
            ssh(deck_env + 'tmux -L wm-managed-qa send-keys -t ' + shlex.quote(target) + ' Enter')
            return False
        if 'Do you trust the files in this folder?' in capture or 'Is this a project you created or one you trust?' in capture:
            assert fixture + '/project' in capture
            # The native default is "No, exit"; select the trust option by
            # its rendered cursor position rather than pressing Enter blindly.
            lines = [line.lstrip() for line in capture.splitlines()]
            if any(line.startswith('❯ Yes, I trust this folder') for line in lines):
                keys = 'Enter'
            elif any(line.startswith('❯ No, exit') for line in lines):
                keys = 'Down Enter'
            else:
                raise RuntimeError('Unknown native folder-trust selection')
            ssh(deck_env + 'tmux -L wm-managed-qa send-keys -t ' + shlex.quote(target) + ' ' + keys)
            return False
        if 'Make auto mode your default permission mode?' in capture:
            assert 'No, keep bypass permissions' in capture
            if any(line.lstrip().startswith('❯ Yes,') for line in capture.splitlines()):
                keys = 'Down Enter'
            elif any(line.lstrip().startswith('❯ No,') for line in capture.splitlines()):
                keys = 'Enter'
            else:
                raise RuntimeError('Unknown native default-mode selection')
            ssh(deck_env + 'tmux -L wm-managed-qa send-keys -t ' + shlex.quote(target) + ' ' + keys)
            return False
        return marker in capture

    def native_records(kind='assistant'):
        code = 'import json,pathlib\nr={}\nfor n,line in enumerate(pathlib.Path(' + repr(native_file) + ').read_bytes().splitlines()):\n try: d=json.loads(line)\n except ValueError: continue\n if d.get("type")==' + repr(kind) + ':\n  c=d.get("message",{}).get("content",[])\n  r[str(n)]=c if isinstance(c,str) else "".join(x.get("text","") for x in c if isinstance(x,dict) and x.get("type")=="text")\nprint(json.dumps(r))\n'
        return json.loads(ssh('python3 -c ' + shlex.quote(code)).stdout)

    def capture_pane():
        return text(deck_env + 'tmux -L wm-managed-qa capture-pane -pt ' + shlex.quote(target))

    def composer_idle():
        # A resumed client repaints restored history (including the marker)
        # before it accepts input. Require an empty composer prompt and an
        # unchanged screen across two polls before sending a question.
        first = capture_pane()
        prompts = [line.strip() for line in first.splitlines() if line.lstrip().startswith('❯')]
        if not prompts or prompts[-1] != '❯' or 'plan mode on' not in first:
            return False
        time.sleep(1)
        return capture_pane() == first

    def send(data):
        subprocess.run(['node', '--input-type=module', '-e', bridge], cwd=Path(__file__).resolve().parents[1] / 'webmux',
                       input=json.dumps({'url': base.replace('http:', 'ws:') + '/api/term/' + sid, 'text': data}), text=True, check=True, timeout=12)

    def answer(label):
        wait(native_ready, 'native Claude composer')
        prior = native_records()
        prior_user = native_records('user')
        nonce = 'AFTER_GUEST_RECOVERY_' + uuid.uuid4().hex
        prompt = 'What exact marker did I ask you to remember earlier in this conversation? Reply with that marker followed by ' + nonce + '. Do not use tools.'

        def submitted():
            return any(key not in prior_user and nonce in value for key, value in native_records('user').items())

        # Delivery is verified against the native file, never assumed. Resend
        # only when the nonce is neither recorded nor visible in the composer,
        # so a question cannot be submitted twice. The oracle is unchanged.
        attempts = []
        for attempt in range(3):
            wait(composer_idle, 'idle native composer')
            if nonce in capture_pane():
                send('\r')
                attempts.append('enter')
            else:
                send(prompt)
                attempts.append('prompt')
            try:
                wait(submitted, 'native prompt record', seconds=20)
                break
            except RuntimeError:
                if attempt == 2:
                    manifest['native_delivery_attempts'] = attempts
                    raise
        fresh = wait(lambda: next((value for key, value in native_records().items() if key not in prior and marker in value and nonce in value), None), label, seconds=120)
        manifest.setdefault('native_answers', []).append({'phase': label, 'nonce': nonce, 'answer': fresh, 'delivery_attempts': attempts})
        save()

    try:
        assert text('hostname') == 'webmux-recovery-qa'
        assert ssh('test -f /var/tmp/webmux-qualification-ready').returncode == 0
        assert 'Agent Deck v1.16.34' in text('/home/webmuxqa/qualification/agent-deck --help')
        manifest['manager_sha256'] = text('sha256sum /home/webmuxqa/qualification/agent-deck').split()[0]
        ssh('mkdir -p ' + fixture + '/home/.config/agent-deck ' + fixture + '/project ' + fixture + '/server/config')
        put(fixture + '/webmux', binary.read_bytes())
        ssh('chmod 700 ' + fixture + '/webmux')
        put(fixture + '/home/.config/agent-deck/config.toml', ('[claude]\nhooks_enabled = false\ndangerous_mode = false\n' if args.native_claude else '') + '[security]\nauto_trust_folders = false\n[launch]\ninject_identity = false\n[tmux]\nsocket_name = "wm-managed-qa"\n')
        if args.native_claude:
            claude = '/home/webmuxqa/.local/bin/claude'
            assert text(claude + ' --version') == '2.1.322 (Claude Code)'
            manifest['native_executable_sha256'] = text('sha256sum ' + claude).split()[0]
            status = ssh(claude + ' auth status --json')
            assert json.loads(status.stdout)['loggedIn'], 'Authenticate Claude in the disposable guest first'
            prompt = 'Remember this exact marker for later in this conversation: ' + marker + '. Reply only with this marker. Do not use tools.'
            command = 'cd ' + fixture + '/project && DISABLE_AUTOUPDATER=1 ' + shlex.join([claude, '-p', prompt, '--session-id', native_id, '--safe-mode', '--tools', '', '--permission-mode', 'plan', '--output-format', 'json'])
            first = ssh(command, timeout=150)
            first_data = json.loads(first.stdout)
            assert marker in first_data.get('result', '') and first_data.get('session_id') == native_id
            put(fixture + '/first-native-answer.json', first.stdout)
            find = 'import pathlib,json\nf=list(pathlib.Path("/home/webmuxqa/.claude/projects").glob("*/' + native_id + '.jsonl"))\nassert len(f)==1\nprint(json.dumps(str(f[0])))'
            native_file = json.loads(text('python3 -c ' + shlex.quote(find)))
            manifest.update(native_session_id=native_id, native_state_file=native_file, native_cli_version='2.1.322')
            wrapper = '#!/usr/bin/python3\nimport os,sys,json\na=sys.argv[1:]\nenv=dict(os.environ)\nenv["HOME"]="/home/webmuxqa"\nenv["DISABLE_AUTOUPDATER"]="1"\nfor k in ["CLAUDE_CONFIG_DIR","XDG_CONFIG_HOME","XDG_DATA_HOME","XDG_STATE_HOME","XDG_CACHE_HOME"]: env.pop(k,None)\nif "--version" not in a:\n with open(' + repr(fixture + '/executions') + ',"a") as f: f.write(json.dumps({"pid":os.getpid(),"args":a,"cwd":os.getcwd(),"roots":{k:env.get(k) for k in ["HOME","CLAUDE_CONFIG_DIR","XDG_CONFIG_HOME"]}})+"\\n");f.flush();os.fsync(f.fileno())\nos.execve(' + repr(claude) + ',[' + repr(claude) + ',*a],env)\n'
            put(fixture + '/claude-native', wrapper)
            ssh('chmod 700 ' + fixture + '/claude-native')
            entry = deck('add', fixture + '/project', '-c', 'claude', '--resume-session', native_id, '--no-parent', '--no-identity', '--title', 'managed-reboot-qualification', '--extra-arg=--safe-mode', '--extra-arg=--tools', '--extra-arg=', '--extra-arg=--permission-mode', '--extra-arg=plan', '--json')
            deck('session', 'set', entry['id'], 'command', fixture + '/claude-native', '--json')
            link = fixture + '/home/.claude/projects/' + Path(native_file).parent.name
            ssh('mkdir -p ' + shlex.quote(link) + ' && ln -s ' + shlex.quote(native_file) + ' ' + shlex.quote(link + '/' + native_id + '.jsonl'))
        else:
            put(fixture + '/project/job.py', 'import pathlib,time,os\np=pathlib.Path(' + repr(fixture + '/executions') + ')\nwith p.open("a") as f: f.write(str(os.getpid())+"\\n"); f.flush(); os.fsync(f.fileno())\nprint(' + repr(marker) + ',flush=True)\nwhile True: print("managed tick",flush=True); time.sleep(1)\n')
            entry = deck('add', fixture + '/project', '--cmd', '/usr/bin/python3 ' + fixture + '/project/job.py', '--no-parent', '--no-identity', '--title', 'managed-reboot-qualification', '--json')
        ident = entry['id']
        assert entry['tool'] == ('claude' if args.native_claude else 'shell') and entry['started'] is False
        started = deck('session', 'start', ident, '--json')
        target = started['tmux']
        wait(lambda: pane() is not None and executions() == 1, 'initial manager job')
        if args.native_claude:
            wait(native_ready, 'original native conversation')
        put(fixture + '/server/config/auth.yaml', 'auth:\n  mode: none\n  users: []\n')
        put(fixture + '/server/config/app.yaml', 'app:\n  session_logging:\n    enabled: true\n  coordination:\n    enabled_providers: [agent-deck]\n    default_launch_provider: webmux\n    default_view: all\n')
        unit_path = '/home/webmuxqa/.config/systemd/user/' + unit
        put(unit_path, '[Unit]\nDescription=WebMux private managed reboot qualification\n[Service]\nType=simple\nWorkingDirectory=' + fixture + '/home\nEnvironment=HOME=' + fixture + '/home\nEnvironment=XDG_CONFIG_HOME=' + fixture + '/home/.config\nEnvironment=WEBMUX_HOME=' + fixture + '/server\nEnvironment=WEBMUX_COORDINATION_DISABLED=0\nEnvironment=SHELL=/bin/bash\nEnvironment=PATH=' + command_path + '\nExecStart=' + fixture + '/webmux -listen 0.0.0.0:18080\nRestart=on-failure\nRestartSec=1\nKillMode=mixed\n[Install]\nWantedBy=default.target\n')
        installed_unit = True
        original_enabled = text(control + 'is-enabled ' + original_unit) == 'enabled'
        ssh(control + 'disable --now ' + original_unit)
        original_stopped = True
        ssh(control + 'daemon-reload && ' + control + 'enable --now ' + unit)
        wait(lambda: api('/api/health')['status'] == 'ok', 'private service')
        session = api('/api/coordination/attach', {'provider': 'agent-deck', 'target': 'local:' + ident})
        sid = str(uuid.UUID(session['id']))
        wait(lambda: marker in view()['history'], 'logged managed history')
        original = receipt('running')
        assert original['recovery']['manager_target'] == 'agent-deck:local:' + ident
        if args.native_claude:
            assert original['recovery']['manager_native_session'] == native_id
            answer('before actual guest reboot')
        for abrupt in (False, True):
            before = int(text(control + 'show ' + unit + ' -p MainPID --value'))
            ssh('kill -KILL ' + str(before) if abrupt else control + 'restart ' + unit)
            wait(lambda: api('/api/health')['status'] == 'ok' and int(text(control + 'show ' + unit + ' -p MainPID --value')) != before, 'controller reconstruction')
            wait(lambda: api('/api/sessions/' + sid)['state'] == 'connected' and view().get('state', {}).get('disposition') == 'reattached', 'survivor attachment')
            current = receipt('SIGKILL' if abrupt else 'orderly-restart')
            assert current['pane_pid'] == original['pane_pid'] and current['executions'] == 1
        previous_boot = boot_id()
        # Only the already-validated disposable guest is rebooted.
        result = ssh('sudo -n systemctl reboot', check=False)
        assert result.returncode in (0, 255)
        wait(lambda: boot_id() != previous_boot, 'actual guest reboot', seconds=120)
        wait(lambda: api('/api/health')['status'] == 'ok', 'service after reboot')
        wait(lambda: api('/api/sessions/' + sid)['state'] != 'connected', 'missing managed provider fallback')
        rebooted = receipt('host-reboot')
        assert rebooted['pane_pid'] is None and rebooted['executions'] == 1
        assert marker in rebooted['recovery']['history']
        assert rebooted['recovery']['state']['disposition'] == 'managed-unavailable'
        assert rebooted['recovery']['state']['pending'] is True
        assert rebooted['recovery']['can_replay'] is False
        assert rebooted['recovery']['can_resume_managed'] is args.native_claude
        assert rebooted['recovery']['can_resume_cli'] is False
        assert rebooted['recovery']['manager_target'] == 'agent-deck:local:' + ident
        assert rebooted['recovery']['lifecycle_owner'] == 'agent-deck'
        assert rebooted['recovery']['manager_directory'] == fixture + '/project'
        assert set(original['recovery']['state']['transcripts']).issubset(rebooted['recovery']['state']['transcripts'])
        for _ in range(3):
            try:
                api('/api/sessions/' + sid + '/reconnect', {})
            except urllib.error.HTTPError as error:
                assert error.code in (400, 409)
        # Allow an accidentally started payload time to reach its durable count.
        time.sleep(2)
        repeated = receipt('repeated-reconnect')
        assert repeated['executions'] == 1 and repeated['pane_pid'] is None
        assert marker in repeated['recovery']['history']
        assert repeated['recovery']['state']['disposition'] == 'managed-unavailable'
        if args.native_claude:
            before_action = view()
            assert before_action['manager_native_session'] == native_id
            action = {'id': str(uuid.uuid4()), 'kind': 'managed', 'revision': before_action['state']['revision'], 'host_revision': 0, 'instance': before_action['instance'], 'confirm': True}
            assert api('/api/session-recovery/' + sid, action)['outcome'] == 'consumed'
            wait(lambda: pane() is not None and executions() == 2, 'native conversation resumed through lifecycle owner')
            wait(lambda: api('/api/sessions/' + sid)['state'] == 'connected', 'recovered native viewer')
            resumed = receipt('approved native continuation after actual reboot')
            assert resumed['recovery']['manager_native_session'] == native_id
            assert resumed['recovery']['manager_native_state_file'] == original['recovery']['manager_native_state_file']
            assert api('/api/session-recovery/' + sid, action)['outcome'] == 'consumed'
            ssh(control + 'restart ' + unit)
            wait(lambda: api('/api/health')['status'] == 'ok', 'reconstruct consumed native receipt')
            wait(lambda: api('/api/sessions/' + sid)['state'] == 'connected', 'reattach restored native process')
            assert api('/api/session-recovery/' + sid, action)['outcome'] == 'consumed'
            assert executions() == 2 and pane() == resumed['pane_pid']
            answer('native conversation after actual guest reboot')
            native_show = deck('session', 'show', ident, '--json')
            assert native_show['claude_session_id'] == native_id
            wrappers = [json.loads(row) for row in text('cat ' + fixture + '/executions').splitlines()]
            assert len(wrappers) == 2 and wrappers[0]['roots'] == wrappers[1]['roots']
            assert all(row['cwd'] == fixture + '/project' and native_id in row['args'] and '--safe-mode' in row['args'] and '--permission-mode' in row['args'] and row['args'][row['args'].index('--permission-mode') + 1] == 'plan' and '--tools' in row['args'] and row['args'][row['args'].index('--tools') + 1] == '' for row in wrappers)
            manifest['original_native_configuration_preserved'] = True
            native_pid = str(wrappers[-1]['pid'])
            assert text('readlink /proc/' + native_pid + '/cwd') == fixture + '/project'
            assert text('readlink /proc/' + native_pid + '/exe') == text('readlink -f /home/webmuxqa/.local/bin/claude')
            assert text('sha256sum /home/webmuxqa/.local/bin/claude').split()[0] == manifest['native_executable_sha256']
            receipt('native duplicate receipt retained across reconstruction')
        manifest['passed'] = True
        manifest.pop('last_native_pane', None)
    finally:
        cleanup_errors = []
        if sid:
            try:
                api('/api/sessions/' + sid, method='DELETE')
            except (OSError, ValueError):
                pass
        if ident:
            # Agent Deck returns INVALID_OPERATION when reboot already removed
            # the pane. Verify native absence rather than treating that as a
            # failed cleanup or accepting arbitrary manager errors.
            if target and pane() is None:
                manifest['manager_cleanup'] = 'already absent after verified pane loss'
            elif ssh(deck_env + 'agent-deck session stop ' + shlex.quote(ident) + ' --json', check=False).returncode:
                cleanup_errors.append('cannot stop private manager session')
        if installed_unit:
            if ssh(control + 'disable --now ' + unit, check=False).returncode:
                cleanup_errors.append('cannot stop private controller')
        if original_stopped:
            if ssh(control + ('enable --now ' if original_enabled else 'start ') + original_unit, check=False).returncode:
                cleanup_errors.append('cannot restore original fixture controller')
            if ssh(control + 'is-active --quiet ' + original_unit, check=False).returncode:
                cleanup_errors.append('original fixture controller is not active')
        manifest['cleanup_errors'] = cleanup_errors
        manifest['passed'] = manifest['passed'] and not cleanup_errors
        save()
        print(json.dumps({'passed': manifest['passed'], 'evidence': str(record_path)}), flush=True)
        if cleanup_errors:
            raise RuntimeError('; '.join(cleanup_errors))


if __name__ == '__main__':
    main()
