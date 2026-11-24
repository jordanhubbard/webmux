#!/usr/bin/env python3
"""Own four real Codex coding turns across an isolated WebMux lifecycle.

Uses the supplied native build and installed authentication by reference. Creates
fresh projects and saved conversations, submits each coding request exactly once,
and never resumes/replays user work. A shell heartbeat alone cannot qualify:
progress must arrive through authenticated native command-output notifications,
matched to the same command item in actual MCP reads while the original turn
runs. All failures and fixture files remain.
"""
import argparse
import datetime
import hashlib
import json
import os
import pwd
from pathlib import Path
import re
import select
import shlex
import shutil
import signal
import socket
import subprocess
import tempfile
import time
import urllib.request
from urllib.parse import unquote, urlsplit
import uuid


class Refusal(RuntimeError):
    pass


def require(condition, code):
    if not condition:
        raise Refusal(code)


def digest(path):
    value = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            value.update(block)
    return value.hexdigest()


def verify_continuity(before, after):
    """No caller-provided pass flags; compare exact native/OS observations."""
    require(len(before) == len(after) == 4, 'exactly-four-sessions-required')
    require(len({row['thread_id'] for row in before}) == 4, 'distinct-threads-required')
    require(len({row['viewer_epoch'] for row in before}) == 4, 'distinct-viewers-required')
    require(len({row['provider_epoch'] for row in before}) == 4, 'distinct-providers-required')
    require(len({row['work_epoch'] for row in before}) == 4, 'distinct-coding-workers-required')
    require(len({row['daemon_epoch'] for row in before}) == 1, 'one-shared-native-daemon-required')
    for row in [*before, *after]:
        require(type(row['work_counter']) is int and row['work_counter'] > 0, 'native-work-counter-required')
        require(isinstance(row['work_epoch'], str) and row['work_epoch'], 'live-coding-worker-required')
        require(isinstance(row['work_script_sha256'], str) and re.fullmatch('[0-9a-f]{64}', row['work_script_sha256']), 'coding-script-digest-required')
        require(row.get('work_witness') == 'authenticated-native-command-output-stream' and isinstance(row.get('work_item_id'), str), 'native-command-stream-required')
        require(row['turn_status'] == 'inProgress', 'turn-no-longer-active')
        require(row['management_thread_id'] == row['selection_thread_id'] == row['thread_id'], 'thread-evidence-disagrees')
    for original, current in zip(before, after, strict=True):
        for key in ('session_id', 'thread_id', 'turn_id', 'viewer_epoch', 'provider_epoch', 'daemon_epoch', 'work_epoch', 'work_script_sha256', 'work_item_id'):
            require(original[key] == current[key], 'continuity-' + key)
        require(current['turn_status'] == 'inProgress', 'turn-no-longer-active')
        require(current['management_thread_id'] == current['thread_id'], 'management-thread-mismatch')
        require(current['selection_thread_id'] == current['thread_id'], 'selection-thread-mismatch')
        require(current['work_counter'] > original['work_counter'], 'work-did-not-progress')


def work_counter(data, turn_id, nonce):
    # Only actual native tool results from this turn can contribute. Code Mode
    # wraps delegated exec_command/write_stdin results as JSON text; arguments,
    # model summaries and arbitrary console dictionaries are not command output.
    pattern = re.compile(r'^' + re.escape('WM_CODING_'+nonce+'_') + r'(\d{1,6})\r?$', re.MULTILINE)
    counters = []
    def output_lines(text):
        if isinstance(text, str):
            for match in pattern.finditer(text):
                counters.append(int(match.group(1)))
    def native_result(value, depth=0):
        if depth > 8:
            return
        if isinstance(value, dict):
            if isinstance(value.get('chunk_id'), str) and isinstance(value.get('output'), str):
                # The native Code Mode adapter escapes line breaks inside its
                # delegated command-output field in addition to JSON encoding.
                output_lines(value['output'].replace('\\r\\n', '\n').replace('\\n', '\n'))
            elif value.get('type') in ('input_text', 'text'):
                native_result(value.get('text'), depth+1)
        elif isinstance(value, list):
            for item in value:
                native_result(item, depth+1)
        elif isinstance(value, str):
            for text in [value, *value.splitlines()]:
                try:
                    parsed = json.loads(text)
                except (ValueError, TypeError):
                    continue
                if parsed != value:
                    native_result(parsed, depth+1)
    for turn in data.get('turns', []):
        if turn.get('id') != turn_id:
            continue
        for item in turn.get('items', []):
            text = item.get('output', {}).get('text', '')
            if item.get('type') == 'commandExecution':
                output_lines(text)
            elif item.get('type') == 'functionCallOutput' and item.get('name') in ('exec', 'wait') and item.get('namespace') in (None, 'functions'):
                native_result(text)
    return max(counters, default=0)


def verify_viewer_migration(before, after):
    require(isinstance(before['work_epoch'], str) and before['work_epoch'] and isinstance(after['work_epoch'], str) and after['work_epoch'], 'migration-live-worker-required')
    require(before['viewer_epoch'] != after['viewer_epoch'], 'viewer-was-not-replaced')
    for key in ('session_id', 'thread_id', 'turn_id', 'provider_epoch', 'daemon_epoch', 'work_epoch', 'work_script_sha256', 'work_item_id'):
        require(before[key] == after[key], 'migration-changed-'+key)
    require(after['turn_status'] == 'inProgress', 'migration-interrupted-original-turn')
    require(after['thread_id'] == after['management_thread_id'] == after['selection_thread_id'], 'migration-thread-evidence')
    require(after['work_counter'] > before['work_counter'], 'migration-work-did-not-progress')


class Fixture:
    def __init__(self, args):
        self.args = args
        self.repo = Path(__file__).resolve().parents[1]
        self.root = Path(tempfile.mkdtemp(prefix='wm-four-', dir='/tmp')).resolve()
        self.home = self.root / 'native'
        self.webhome = self.root / 'webmux'
        self.home.mkdir(mode=0o700)
        self.webhome.mkdir(mode=0o700)
        self.tmuxdir = self.root / 'tmux'
        self.tmuxdir.mkdir(mode=0o700)
        self.tmuxsocket = self.tmuxdir / f'tmux-{os.getuid()}' / 'default'
        self.env = {key: os.environ[key] for key in ('PATH', 'USER', 'LANG', 'TMPDIR', 'SSH_AUTH_SOCK') if key in os.environ}
        self.env.update(HOME=str(self.root), CODEX_HOME=str(self.home), TMUX_TMPDIR=str(self.tmuxdir),
            TERM='xterm-256color', COLORTERM='truecolor', SHELL='/bin/bash',
            WEBMUX_HOME=str(self.webhome), WEBMUX_COORDINATION_DISABLED='1', WEBMUX_ROOT='')
        nativebin = self.root / 'bin'
        nativebin.mkdir()
        (nativebin / 'codex').symlink_to(args.native)
        launcher = self.root / 'viewer-launch'
        launcher.mkdir(mode=0o700)
        (launcher / 'codex').symlink_to(args.webmux)
        self.env['PATH'] = str(nativebin) + os.pathsep + self.env.get('PATH', '')
        self.env['WEBMUX_BROWSER_ORIGINAL_PATH'] = self.env['PATH']
        self.env['WEBMUX_RECOVERY_CACHE_HOME'] = str(self.root / 'host-cache')
        if args.ssh_host:
            self.configure_private_ssh(nativebin)
        self.controller = self.rpc = None
        self.daemon_start_attempted = False
        self.daemon_pid = None
        self.daemon_epoch = None
        self.streams = []
        self.sessions = []
        self.counter = 0
        self.receipt = {'schema': 'webmux.four-codex-continuity.v1', 'passed': False,
            'four_session_survival_qualified': False, 'active_work_qualified': False,
            'profile': getattr(args, 'profile', 'full'),
            'fixture_root': str(self.root), 'source': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=self.repo, text=True).strip(),
            'native_sha256': args.native_sha256, 'webmux_sha256': args.webmux_sha256,
            'code_mode_host_sha256': getattr(args, 'code_mode_host_sha256', None),
            'qualifier_sha256': digest(Path(__file__)), 'phases': [],
            'worker_start': {'load': list(os.getloadavg()), 'cpus': os.cpu_count(),
                'free_disk_bytes': shutil.disk_usage(self.root).free, 'machine': os.uname().machine},
            'model_requests_attempted': 0, 'model_requests_submitted': 0, 'coding_requests_replayed': 0, 'native_daemon_restarts': 0,
            'ssh_host': args.ssh_host, 'ssh_execution_boot_sha256': args.ssh_boot_sha256,
            'scope': ('four isolated actual SSH durable providers on the verified same-kernel macOS account' if args.ssh_host else 'four isolated local durable providers') + '; actual native model coding turns; installed/legacy viewer migration and other execution hosts unqualified'}
        self.output = args.output.open('x')
        os.chmod(args.output, 0o600)
        self.save()

    def configure_private_ssh(self, nativebin):
        # Real OpenSSH connects to the verified local execution host. Scope its
        # remote helper search and tmux/cache roots without installing anything
        # or joining the operator's SSH control master or tmux server.
        helpers = self.root / 'ssh-tools'
        helpers.mkdir(mode=0o700)
        (helpers / 'webmux').symlink_to(self.args.webmux)
        original_path = str(helpers) + os.pathsep + str(nativebin) + os.pathsep + self.env.get('PATH', '')
        remote = dict(HOME=str(self.root), CODEX_HOME=str(self.home), TMUX_TMPDIR=str(self.tmuxdir),
            WEBMUX_RECOVERY_CACHE_HOME=str(self.root / 'host-cache'), PATH=original_path,
            WEBMUX_BROWSER_ORIGINAL_PATH=original_path, SHELL='/bin/bash')
        prefix = 'cd ' + shlex.quote(str(self.root)) + ' || exit 125; export ' + ' '.join(key+'='+shlex.quote(value) for key, value in remote.items()) + '; '
        shimdir = self.root / 'ssh-launch'
        shimdir.mkdir(mode=0o700)
        shim = shimdir / 'ssh'
        shim.write_text('#!/usr/bin/env python3\nimport os, sys\nargs = sys.argv[1:]\n'
            + 'host = '+repr(self.args.ssh_host)+'\n'
            + 'if len(args) < 2 or not any(value == host or value.endswith("@"+host) for value in args) or args[-1] == host or args[-1].endswith("@"+host):\n    raise SystemExit("Fixture SSH destination or remote command is missing")\n'
            + 'args[-1] = '+repr(prefix)+' + args[-1]\n'
            + 'os.execv("/usr/bin/ssh", ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "-o", "ControlMaster=no", "-o", "ControlPath=none", "-o", "ControlPersist=no", *args])\n')
        os.chmod(shim, 0o700)
        self.env['PATH'] = str(shimdir) + os.pathsep + self.env['PATH']

    def save(self):
        self.output.seek(0)
        json.dump(self.receipt, self.output, indent=2)
        self.output.write('\n')
        self.output.truncate()
        self.output.flush()

    def record_process(self, role, pid):
        self.receipt.setdefault('owned_processes', []).append({'role': role, 'pid': pid, 'epoch': self.epoch(pid)})
        self.save()

    def log(self, name):
        stream = (self.root / name).open('xb', buffering=0)
        os.chmod(stream.name, 0o600)
        self.streams.append(stream)
        return stream

    def run(self, argv, **kw):
        try:
            return subprocess.run(argv, env=self.env, cwd=self.root, stdin=subprocess.DEVNULL,
                capture_output=True, text=True, timeout=kw.pop('timeout', 15), check=True, **kw)
        except subprocess.CalledProcessError as error:
            # Retain diagnostics privately; command arguments can contain secrets.
            rows = self.receipt.setdefault('failed_subprocesses', [])
            name = f'failed-subprocess-{len(rows)+1}.log'
            self.log(name).write(((error.stdout or '')+(error.stderr or '')).encode())
            rows.append({'executable': Path(argv[0]).name, 'exit_code': error.returncode, 'log': name})
            self.save()
            raise

    def lifecycle(self, action):
        # The unoptimized producer CLI fingerprints its entire executable before
        # spawning: measured 525 MB took about 50 seconds on the qualification host.
        # Reserve time for the ~954 MB original without changing native's own
        # readiness deadline, any RPC deadline, or production source deadlines.
        timeout = 120 if action == 'start' else 30
        rows = self.receipt.setdefault('native_lifecycle', [])
        row = {'action': action, 'wrapper_timeout_seconds': timeout}
        rows.append(row)
        self.save()
        started = time.monotonic()
        log = self.log(f'native-lifecycle-{len(rows)}-{action}.log')
        def retain(value):
            if value:
                log.write(value.encode() if isinstance(value, str) else value)
        try:
            result = self.run([str(self.args.native), 'app-server', 'daemon', action], timeout=timeout)
            retain(result.stdout)
            retain(result.stderr)
            row['exit_code'] = result.returncode
            return result
        except subprocess.SubprocessError as error:
            retain(getattr(error, 'stdout', None))
            retain(getattr(error, 'stderr', None))
            row['failure'] = type(error).__name__
            raise
        finally:
            row['elapsed_ms'] = (time.monotonic()-started)*1000
            self.save()

    def epoch(self, pid):
        data = self.run(['/bin/ps', '-p', str(pid), '-o', 'uid=,lstart=,comm=']).stdout.strip()
        require(data.split()[0] == str(os.getuid()), 'process-account')
        return hashlib.sha256(f'{pid}\n{data}'.encode()).hexdigest()

    def wait(self, predicate, code, seconds=45):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            value = predicate()
            if value:
                return value
            time.sleep(.2)
        raise Refusal(code)

    def api(self, route, data=None, method=None):
        request = urllib.request.Request(self.base + route, method=method,
            data=json.dumps(data).encode() if data is not None else None,
            headers={'Content-Type': 'application/json'})
        with urllib.request.urlopen(request, timeout=15) as response:
            value = response.read()
            return json.loads(value) if value else None

    def ready(self):
        require(self.controller is not None and self.controller.poll() is None, 'controller-exited-before-ready')
        try:
            return self.api('/api/health').get('status') == 'ok'
        except OSError:
            return False

    def start_controller(self):
        require(self.controller is None or self.controller.poll() is not None, 'controller-already-live')
        self.controller = subprocess.Popen([str(self.args.webmux), '-root', str(self.repo / 'webmux'), '-listen', self.address],
            env=self.env, cwd=self.root, stdin=subprocess.DEVNULL,
            stdout=self.log(f'controller-{len(self.receipt["phases"])}.log'), stderr=subprocess.STDOUT,
            start_new_session=True)
        self.record_process('controller', self.controller.pid)
        self.wait(self.ready, 'controller-startup')
        return self.epoch(self.controller.pid)

    def stop_controller(self, abrupt=False):
        require(self.controller is not None and self.controller.poll() is None, 'owned-controller-not-live')
        self.controller.kill() if abrupt else self.controller.terminate()
        return self.controller.wait(timeout=20)

    def native_rpc(self, method, params):
        require(self.rpc is not None and self.rpc.poll() is None, 'native-client-unavailable')
        self.counter += 1
        self.rpc.stdin.write(json.dumps({'id': self.counter, 'method': method, 'params': params}) + '\n')
        self.rpc.stdin.flush()
        require(select.select([self.rpc.stdout], [], [], 15)[0], 'native-client-deadline')
        response = json.loads(self.rpc.stdout.readline())
        require(response.get('id') == self.counter and not response.get('error'), 'native-rpc-failed')
        return response['result']

    def prepare_native_fixture(self):
        # Only the authentication file is referenced. Mutable native state and
        # all configuration, packages, updater settings and conversations are private.
        auth = self.args.auth_home / 'auth.json'
        require(auth.is_file() and auth.stat().st_uid == os.getuid(), 'owned-auth-file-required')
        (self.home / 'auth.json').symlink_to(auth)
        projects = []
        for index in range(4):
            project = self.root / f'project-{index}'
            project.mkdir(mode=0o700)
            projects.append(project)
            self.sessions.append({'project': project, 'thread': str(uuid.uuid4()), 'nonce': uuid.uuid4().hex})
        config = f'model = {json.dumps(self.args.model)}\ncheck_for_update_on_startup = false\nweb_search = "disabled"\napproval_policy = "never"\nsandbox_mode = "workspace-write"\n[features]\nenable_mcp_apps = false\n'
        for project in projects:
            config += f'[projects.{json.dumps(str(project))}]\ntrust_level = "trusted"\n'
        (self.home / 'config.toml').write_text(config)
        os.chmod(self.home / 'config.toml', 0o600)
        self.receipt['native_config_sha256_before'] = digest(self.home / 'config.toml')
        package = self.home / 'packages/app-server-daemon'
        (package / 'fixture/bin').mkdir(parents=True)
        (package / 'fixture/bin/codex').symlink_to(self.args.native)
        (package / 'current').symlink_to('fixture')
        settings = self.home / 'app-server-daemon/settings.json'
        settings.parent.mkdir(mode=0o700)
        settings.write_text(json.dumps({'remoteControlEnabled': False, 'featureOverrides': {},
            'shutdownGraceSeconds': 2, 'updater': {'autoUpdateEnabled': False, 'updateIntervalMinutes': 60}}))
        os.chmod(settings, 0o600)
        # Synthetic saved history lets each actual TUI install its own native
        # management server when the daemon first loads this fixture thread.
        now = datetime.datetime.now(datetime.timezone.utc)
        folder = self.home / 'sessions' / now.strftime('%Y/%m/%d')
        folder.mkdir(parents=True)
        for session in self.sessions:
            stamp = now.isoformat().replace('+00:00', 'Z')
            meta = {'id': session['thread'], 'session_id': session['thread'], 'timestamp': stamp,
                'cwd': str(session['project']), 'originator': 'codex', 'cli_version': '0.172.0',
                'source': 'cli', 'account_scoped': False, 'attached_thread': False,
                'model_provider': 'openai', 'history_mode': 'legacy', 'selected_capability_roots': []}
            records = [{'type': 'session_meta', 'payload': meta},
                {'type': 'response_item', 'payload': {'type': 'message', 'role': 'user', 'content': [{'type': 'input_text', 'text': 'Owned continuity fixture; no prior work.'}]}},
                {'type': 'event_msg', 'payload': {'type': 'user_message', 'message': 'Owned continuity fixture; no prior work.', 'kind': 'plain'}}]
            rollout = folder / f'rollout-{now.strftime("%Y-%m-%dT%H-%M-%S")}-{session["thread"]}.jsonl'
            rollout.write_text(''.join(json.dumps(dict(timestamp=stamp, **row))+'\n' for row in records))
            os.chmod(rollout, 0o600)

    def setup(self):
        self.prepare_native_fixture()
        self.daemon_start_attempted = True
        started = json.loads(self.lifecycle('start').stdout)
        self.daemon_pid = started['pid']
        self.daemon_epoch = self.epoch(self.daemon_pid)
        self.record_process('native-daemon', self.daemon_pid)
        require(started['status'] == 'started', 'managed-native-start')
        require(Path(started['managedCodexPath']).resolve() == self.args.native, 'managed-native-image')
        self.socket = self.home / 'app-server-control/app-server-control.sock'
        require(Path(started['socketPath']) == self.socket, 'managed-native-socket')
        watches = self.root / 'output-watches.json'
        watches.write_text(json.dumps([{'thread': row['thread'], 'nonce': row['nonce']} for row in self.sessions]))
        os.chmod(watches, 0o600)
        self.output_witness = self.root / 'command-output-witness.json'
        self.rpc = subprocess.Popen([str(self.args.node), str(self.repo / 'webmux/scripts/codex-fixture-rpc.mts'), str(self.socket), str(watches), str(self.output_witness)],
            cwd=self.root, env=self.env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=self.log('native-rpc.log'), text=True, bufsize=1, start_new_session=True)
        self.record_process('native-rpc-client', self.rpc.pid)
        shutil.copytree(self.repo / 'webmux/config.defaults', self.webhome / 'config')
        (self.webhome / 'config/auth.yaml').write_text('auth:\n  mode: none\n  users: []\n')
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            port = listener.getsockname()[1]
        self.address = '127.0.0.1:' + str(port)
        self.base = 'http://' + self.address
        self.controller_epoch = self.start_controller()
        self.api('/api/config', {'app': {'session_logging': {'enabled': True}}}, 'PUT')
        for session in self.sessions:
            request = {'transport': 'local', 'cols': 140, 'rows': 45}
            if self.args.ssh_host:
                request.update(transport='ssh', hostname=self.args.ssh_host, username=pwd.getpwuid(os.getuid()).pw_name)
            session['id'] = str(uuid.UUID(self.api('/api/sessions', request)['id']))
            session['target'] = 'webmux-' + session['id']
            self.wait(lambda: self.api('/api/session-recovery/'+session['id']).get('shell_ready'), 'provider-shell-ready')
            self.tmux('send-keys', '-t', '='+session['target']+':', '-l', f'cd {shlex.quote(str(session["project"]))} && ' + self.viewer_command(session))
            self.tmux('send-keys', '-t', '='+session['target']+':', 'Enter')
            self.wait(lambda: self.selection(session) == session['thread'], 'native-selection-ready')
            session['provider'] = int(self.tmux('display-message', '-p', '-t', '='+session['target']+':', '#{pane_pid}').strip())
            session['provider_epoch'] = self.epoch(session['provider'])
            self.record_process('provider', session['provider'])
            session['viewer'], session['management_port'] = self.management_owner(session)
            owners = [str(session['viewer'])]
            images = self.run(['/usr/sbin/lsof', '-a', '-p', owners[0], '-d', 'txt', '-Fn']).stdout.splitlines()
            require('n'+str(self.args.native) in images, 'viewer-native-image')
            session['viewer_epoch'] = self.epoch(session['viewer'])
            self.record_process('native-viewer', session['viewer'])
            # Fail before submitting model work if its actual authenticated
            # management registration is already broken.
            self.read_management(session)
            session['worktree_read_before'] = self.read_worktrees(session)
            self.receipt.setdefault('setup_viewers', []).append({
                'session_id': session['id'], 'thread_id': session['thread'],
                'provider_pid': session['provider'], 'provider_epoch': session['provider_epoch'],
                'viewer_pid': session['viewer'], 'viewer_epoch': session['viewer_epoch'],
                'management_port': session['management_port'], 'actual_management_read': True,
                'native_selection_verified': True, 'native_image_verified': True,
                'project_sha256': hashlib.sha256(str(session['project']).encode()).hexdigest(),
                'worktree_read_sha256': session['worktree_read_before']})
            self.save()
        self.receipt['fixtures_ready'] = True
        self.save()

    def viewer_command(self, session):
        # Login profiles can replace PATH. Invoke the exact Go launcher alias
        # and pin its pre-integration lookup to the private native executable.
        return shlex.join(['/usr/bin/env', 'WEBMUX_BROWSER_ORIGINAL_PATH='+str(self.root / 'bin'),
            str(self.root / 'viewer-launch/codex'), '--remote', 'unix://'+str(self.socket),
            '--no-alt-screen', 'resume', session['thread']])

    def tmux(self, *args):
        require(self.root in self.tmuxsocket.parents, 'private-tmux-boundary')
        return self.run(['tmux', '-S', str(self.tmuxsocket), *args]).stdout

    def selection(self, session):
        activity = self.api('/api/session-recovery/'+session['id']).get('state', {}).get('activity', {})
        require(not activity.get('native_session_id') or activity['native_session_id'] == session['thread'], 'unexpected-selected-thread')
        return activity.get('native_session_id') if activity.get('native_evidence') == 'native-selection' else None

    def management_owner(self, session):
        inventory = self.native_rpc('mcpServerStatus/list', {'threadId': session['thread'], 'serverName': 'codex_tui', 'detail': 'toolsAndAuthOnly'})
        servers = [row for row in inventory['data'] if row['name'] == 'codex_tui']
        require(len(servers) == 1, 'one-viewer-management-server')
        origin = urlsplit(servers[0]['httpOrigin'])
        require(origin.scheme == 'http' and origin.hostname == '127.0.0.1' and origin.port and
            not origin.username and not origin.password and origin.path in ('', '/') and not origin.query and not origin.fragment,
            'loopback-management-origin')
        owners = self.run(['/usr/sbin/lsof', '-nP', '-tiTCP:'+str(origin.port), '-sTCP:LISTEN']).stdout.split()
        require(len(owners) == 1, 'one-management-listener-owner')
        return int(owners[0]), origin.port

    def read_management(self, session):
        if 'viewer' in session:
            require(self.management_owner(session) == (session['viewer'], session['management_port']), 'management-owner-changed')
        result = self.native_rpc('mcpServer/tool/call', {'threadId': session['thread'], 'server': 'codex_tui', 'tool': 'read_thread',
            'arguments': {'threadId': session['thread'], 'turnLimit': 1, 'includeOutputs': True, 'maxOutputCharsPerItem': 20000}})
        require(not result.get('isError'), 'management-tool-error')
        texts = [json.loads(row['text']) for row in result.get('content', []) if row.get('type') == 'text']
        require(len(texts) == 1 and texts[0].get('thread', {}).get('id') == session['thread'], 'management-thread-result')
        raw_cwd = texts[0]['thread'].get('cwd', '')
        parsed = urlsplit(raw_cwd)
        cwd = Path(unquote(parsed.path)) if parsed.scheme == 'file' else Path(raw_cwd)
        require(cwd.is_absolute() and cwd.resolve() == session['project'], 'management-project-result')
        return texts[0]

    def read_worktrees(self, session):
        inventory = self.native_rpc('mcpServerStatus/list', {'threadId': session['thread'], 'serverName': 'codex_tui', 'detail': 'toolsAndAuthOnly'})
        servers = [row for row in inventory['data'] if row['name'] == 'codex_tui']
        require(len(servers) == 1, 'worktree-management-registration')
        tools = servers[0].get('tools', {})
        names = set(tools) if isinstance(tools, dict) else {tool['name'] for tool in tools}
        require({'create_worktree', 'list_worktrees', 'get_worktree_creation_status'} <= names, 'worktree-tools-unavailable')
        result = self.native_rpc('mcpServer/tool/call', {'threadId': session['thread'], 'server': 'codex_tui', 'tool': 'list_worktrees', 'arguments': {}})
        require(not result.get('isError'), 'worktree-read-failed')
        texts = [json.loads(row['text']) for row in result.get('content', []) if row.get('type') == 'text']
        require(len(texts) == 1 and isinstance(texts[0].get('data'), list) and texts[0].get('nextCursor') is None, 'worktree-read-shape')
        return hashlib.sha256(json.dumps(texts[0], sort_keys=True, separators=(',', ':')).encode()).hexdigest()

    def migrate_viewers(self):
        self.receipt['viewer_migrations'] = []
        for session in self.sessions:
            before = self.snapshot(session)
            require(self.epoch(session['viewer']) == before['viewer_epoch'], 'owned-viewer-epoch-before-migration')
            # Only this freshly-created fixture viewer is killed. Its model turn
            # belongs to the separate managed daemon and must continue unchanged.
            os.kill(session['viewer'], signal.SIGKILL)
            self.wait(lambda: self.api('/api/session-recovery/'+session['id']).get('shell_ready'), 'fixture-viewer-shell-return')
            self.tmux('send-keys', '-t', '='+session['target']+':', '-l', self.viewer_command(session))
            self.tmux('send-keys', '-t', '='+session['target']+':', 'Enter')
            old_viewer = session['viewer']
            def replacement():
                try:
                    owner = self.management_owner(session)
                except subprocess.CalledProcessError:
                    return None
                return owner if owner[0] != old_viewer else None
            viewer, port = self.wait(replacement, 'replacement-management-owner')
            images = self.run(['/usr/sbin/lsof', '-a', '-p', str(viewer), '-d', 'txt', '-Fn']).stdout.splitlines()
            require('n'+str(self.args.native) in images, 'replacement-native-image')
            session['viewer'], session['management_port'] = viewer, port
            session['viewer_epoch'] = self.epoch(viewer)
            self.record_process('replacement-native-viewer', viewer)
            self.wait(lambda: self.selection(session) == session['thread'], 'replacement-selected-thread')
            def advanced():
                current = self.snapshot(session)
                return current if current['work_counter'] > before['work_counter'] else None
            after = self.wait(advanced, 'replacement-original-work-progress', seconds=40)
            verify_viewer_migration(before, after)
            worktrees = self.read_worktrees(session)
            require(worktrees == session['worktree_read_before'], 'replacement-worktree-read-parity')
            self.receipt['viewer_migrations'].append({'before': before, 'after': after,
                'worktree_read_sha256': worktrees, 'actual_management_read': True})
            self.save()

    def start_work(self):
        for session in self.sessions:
            # Attach this fixture-only observer before submitting any model work.
            # No overrides and no turn input: subscribe to the already loaded actor.
            attached = self.native_rpc('thread/resume', {'threadId': session['thread']})
            require(attached['thread']['id'] == session['thread'], 'output-observer-thread')
            self.read_management(session)
            require(self.read_worktrees(session) == session['worktree_read_before'], 'observer-worktree-parity')
            marker = 'WM_CODING_'+session['nonce']+'_'
            prompt = f'''This is an isolated continuity coding test. Work only in this project. Write proof.py implementing a small correct prime-number predicate with assertions for 1,2,9,17, then run it. In that same Python foreground process, after the assertions, iterate i from 1 through 300: print "{marker}" followed by i with flush=True, sleep one second, and stop early if a file named STOP exists in this project. Keep that foreground command running and wait for it; do not background it, shorten the loop, request another agent, or finish your turn while it runs. Do not create STOP yourself or modify proof.py while it runs. When polling exec_command/write_stdin in Code Mode, forward the raw result with text(result), including stdout; do not summarize it. Use short polling/yield intervals so output remains visible. Once STOP appears, verify the file and report the assertions. No network or other projects.'''
            self.receipt['model_requests_attempted'] += 1
            self.save()
            response = self.native_rpc('turn/start', {'threadId': session['thread'], 'input': [{'type': 'text', 'text': prompt}]})
            session['turn'] = response['turn']['id']
            self.receipt['model_requests_submitted'] += 1
            self.save()

    def coding_worker(self, session, counter):
        if 'work_pid' not in session:
            if not counter:
                return None, None
            result = self.run(['/usr/sbin/lsof', '-nP', '-a', '-u', str(os.getuid()), '-d', 'cwd', '-Fpcn'])
            workers, pid, name = [], None, ''
            for line in result.stdout.splitlines():
                if line.startswith('p'):
                    pid, name = int(line[1:]), ''
                elif line.startswith('c'):
                    name = line[1:]
                elif line.startswith('n') and pid and name.lower().startswith('python') and Path(line[1:]).resolve() == session['project']:
                    workers.append(pid)
            require(len(workers) <= 1, 'ambiguous-coding-worker')
            if not workers:
                return None, None
            pid = workers[0]
            command = self.run(['/bin/ps', '-ww', '-p', str(pid), '-o', 'command=']).stdout
            require('proof.py' in command, 'coding-worker-program')
            script = session['project'] / 'proof.py'
            require(script.is_file() and not script.is_symlink() and session['nonce'] in script.read_text(), 'coding-worker-script')
            session.update(work_pid=pid, work_epoch=self.epoch(pid), work_script_sha256=digest(script))
            self.record_process('coding-worker', pid)
        require(self.epoch(session['work_pid']) == session['work_epoch'], 'coding-worker-replaced')
        require(digest(session['project'] / 'proof.py') == session['work_script_sha256'], 'running-coding-script-changed')
        return session['work_epoch'], session['work_script_sha256']

    def snapshot(self, session):
        turns = self.native_rpc('thread/turns/list', {'threadId': session['thread'], 'limit': 1, 'sortDirection': 'desc'})['data']
        require(len(turns) == 1 and turns[0]['id'] == session['turn'] and turns[0]['status'] == 'inProgress', 'exact-coding-turn-not-active')
        data = self.read_management(session)
        management_turns = data.get('turns', [])
        require(len(management_turns) == 1 and management_turns[0]['id'] == session['turn'] and management_turns[0]['status'] == 'inProgress', 'management-active-turn-result')
        # Preserve the last real management response for diagnosis before any
        # cleanup can retire the viewer. Fixture history only, never user tasks.
        name = 'work-read-' + session['thread'] + '.json'
        fd = os.open(self.root / name, os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, 'w') as record:
            json.dump(data, record)
        self.receipt.setdefault('work_read_receipts', {})[session['thread']] = name
        self.save()
        commands = [item for item in management_turns[0].get('items', [])
            if item.get('type') == 'commandExecution' and 'proof.py' in item.get('command', '') and item.get('status') == 'inProgress']
        require(len(commands) <= 1, 'ambiguous-active-coding-command')
        counter = 0
        if commands:
            item_id = commands[0]['id']
            require(session.get('work_item_id', item_id) == item_id, 'coding-command-item-replaced')
            session['work_item_id'] = item_id
            rows = json.loads(self.output_witness.read_text()) if self.output_witness.exists() else []
            matching = [row for row in rows if row['threadId'] == session['thread'] and row['turnId'] == session['turn'] and row['itemId'] == item_id]
            require(len(matching) <= 1, 'ambiguous-output-witness')
            counter = matching[0]['counter'] if matching else 0
        work_epoch, script_sha = self.coding_worker(session, counter)
        return {'viewer_pid': session['viewer'], 'provider_pid': session['provider'], 'daemon_pid': self.daemon_pid,
            'session_id': session['id'], 'thread_id': session['thread'], 'turn_id': session['turn'],
            'turn_status': turns[0]['status'], 'management_thread_id': data['thread']['id'],
            'selection_thread_id': self.selection(session), 'work_counter': counter,
            'work_item_id': session.get('work_item_id'), 'work_witness': 'authenticated-native-command-output-stream',
            'work_pid': session.get('work_pid'), 'work_epoch': work_epoch, 'work_script_sha256': script_sha,
            'viewer_epoch': self.epoch(session['viewer']), 'provider_epoch': self.epoch(session['provider']),
            'daemon_epoch': self.epoch(self.daemon_pid)}

    def phase(self, name, before=None):
        def collect():
            rows = [self.snapshot(session) for session in self.sessions]
            if before and any(row['work_counter'] <= old['work_counter'] for old, row in zip(before, rows, strict=True)):
                return None
            return rows if all(row['work_counter'] > 0 and row['work_epoch'] for row in rows) else None
        rows = self.wait(collect, 'actual-coding-progress-'+name, seconds=120 if before is None else 40)
        if before:
            verify_continuity(before, rows)
        self.receipt['phases'].append({'name': name, 'controller_epoch': self.epoch(self.controller.pid), 'sessions': rows})
        self.save()
        return rows

    def qualify(self):
        self.setup()
        self.start_work()
        baseline = self.phase('baseline')
        previous = baseline
        for name, abrupt in [('orderly-restart', False), ('controller-crash', True)]:
            old = self.epoch(self.controller.pid)
            code = self.stop_controller(abrupt)
            require(code == (-signal.SIGKILL if abrupt else 0), 'controller-exit-kind')
            self.start_controller()
            require(self.epoch(self.controller.pid) != old, 'controller-epoch-did-not-change')
            self.receipt.setdefault('controller_exits', []).append({'phase': name, 'exit_code': code})
            previous = self.phase(name, previous)
        for session in self.sessions:
            self.api('/api/sessions/'+session['id']+'/reconnect', {})
        previous = self.phase('provider-reconnect', previous)
        verify_continuity(baseline, previous)
        if self.args.profile == 'full':
            self.migrate_viewers()
        # Let the original four model requests finish their own coding checks.
        for session in self.sessions:
            (session['project'] / 'STOP').touch(exist_ok=False)
        for session in self.sessions:
            def complete():
                turns = self.native_rpc('thread/turns/list', {'threadId': session['thread'], 'limit': 1, 'sortDirection': 'desc'})['data']
                require(turns and turns[0]['id'] == session['turn'], 'original-turn-replaced')
                if turns[0]['status'] == 'inProgress':
                    return False
                require(turns[0]['status'] == 'completed', 'original-turn-failed')
                proof = session['project'] / 'proof.py'
                require(proof.is_file() and not proof.is_symlink() and proof.stat().st_size > 0, 'coding-artifact-missing')
                self.receipt.setdefault('coding_artifacts', {})[session['thread']] = digest(proof)
                return True
            self.wait(complete, 'original-model-turn-completion', seconds=90)
        self.receipt['original_turns_completed'] = 4
        self.receipt['native_config_sha256_after'] = digest(self.home / 'config.toml')
        require(self.receipt['native_config_sha256_after'] == self.receipt['native_config_sha256_before'], 'native-config-file-changed')

    def fixture_processes(self):
        # Retain only same-account processes whose actual cwd is in our fresh
        # private root. This also finds detached execution helpers, not just TUIs.
        result = subprocess.run(['/usr/sbin/lsof', '-nP', '-a', '-u', str(os.getuid()), '-d', 'cwd', '-Fpn'],
            capture_output=True, text=True, timeout=10)
        require(result.returncode in (0, 1), 'fixture-process-inspector')
        owned, pid = {}, None
        for line in result.stdout.splitlines():
            if line.startswith('p'):
                pid = int(line[1:])
            elif line.startswith('n') and pid and pid != os.getpid():
                path = Path(line[1:])
                if path == self.root or self.root in path.parents:
                    try:
                        owned[pid] = self.epoch(pid)
                    except subprocess.CalledProcessError:
                        pass  # Gone between two kernel observations.
        return owned

    def cleanup(self):
        errors = []
        try:
            owned = self.fixture_processes()
        except (Refusal, OSError, subprocess.SubprocessError):
            owned = {}
            errors.append('fixture-process-inventory')
        for session in self.sessions:
            try:
                if session['project'].is_dir():
                    (session['project'] / 'STOP').touch(exist_ok=True)
            except OSError:
                errors.append('stop-file')
        if self.rpc:
            try:
                self.rpc.stdin.close()
                self.rpc.wait(timeout=5)
            except (OSError, subprocess.TimeoutExpired):
                self.rpc.terminate()
                try:
                    self.rpc.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    self.rpc.kill()
                    self.rpc.wait(timeout=5)
        if self.controller and self.controller.poll() is None:
            try:
                self.stop_controller()
            except (OSError, subprocess.SubprocessError):
                self.controller.kill()
                self.controller.wait(timeout=5)
                errors.append('controller-forced-settlement')
        if self.tmuxsocket.exists():
            try:
                require(self.tmuxsocket.is_socket() and self.tmuxsocket.stat().st_uid == os.getuid(), 'private-tmux-owner')
                try:
                    self.tmux('kill-server')
                except subprocess.CalledProcessError:
                    pass  # A controller stop may already have closed it.
                def provider_closed():
                    probe = subprocess.run(['tmux', '-S', str(self.tmuxsocket), 'list-sessions'],
                        env=self.env, cwd=self.root, capture_output=True, timeout=5)
                    owners = subprocess.run(['/usr/sbin/lsof', '-nP', str(self.tmuxsocket)],
                        capture_output=True, timeout=5)
                    # tmux can leave a stale socket inode after kernel closure.
                    # Neither a failed client nor missing owners alone suffices.
                    closed = probe.returncode == 1 and owners.returncode == 1 and not owners.stdout.strip()
                    if closed:
                        self.receipt['provider_socket_settlement'] = {
                            'private_client_exit': probe.returncode, 'kernel_owners_exit': owners.returncode,
                            'socket_inode_retained': self.tmuxsocket.exists()}
                    return closed
                self.wait(provider_closed, 'private-tmux-settlement', seconds=10)
            except (OSError, subprocess.SubprocessError, Refusal):
                errors.append('provider-settlement')
        try:
            if self.daemon_start_attempted and self.daemon_pid is None:
                # A failed lifecycle start may already have published a child.
                record = self.home / 'app-server-daemon/daemon.pid'
                if record.is_file() and record.read_text().strip():
                    self.daemon_pid = json.loads(record.read_text())['pid']
                    image = self.run(['/usr/sbin/lsof', '-a', '-p', str(self.daemon_pid), '-d', 'txt', '-Fn']).stdout.splitlines()
                    require('n'+str(self.args.native) in image, 'partial-start-native-image')
                    require(self.daemon_pid in owned, 'partial-start-native-cwd')
                    self.daemon_epoch = self.epoch(self.daemon_pid)
            if self.daemon_pid:
                require(self.epoch(self.daemon_pid) == self.daemon_epoch, 'native-daemon-epoch-before-stop')
            if self.daemon_start_attempted:
                stopped = json.loads(self.lifecycle('stop').stdout)
                require(stopped.get('status') in ('stopped', 'notRunning'), 'native-lifecycle-stop-result')
        except (OSError, ValueError, KeyError, subprocess.SubprocessError, Refusal):
            errors.append('native-lifecycle-settlement')
        # Drain all captured process epochs. Only a still-owned, same-epoch
        # fixture process may be signalled; never use a remembered PID alone.
        remaining = dict(owned)
        for action in (None, signal.SIGTERM, signal.SIGKILL):
            try:
                current = self.fixture_processes()
                # Include late-born helpers that appeared during provider stop.
                for pid, identity in current.items():
                    owned.setdefault(pid, identity)
                remaining = {}
                for pid, identity in owned.items():
                    try:
                        if self.epoch(pid) == identity:
                            remaining[pid] = identity
                    except subprocess.CalledProcessError:
                        pass
                if not remaining:
                    break
                if action is not None:
                    signalled = []
                    for pid, identity in remaining.items():
                        if current.get(pid) != identity:
                            errors.append('live-process-left-private-cwd')
                            continue
                        if self.epoch(pid) == identity:
                            try:
                                os.kill(pid, action)
                                signalled.append(pid)
                            except ProcessLookupError:
                                pass
                    self.receipt.setdefault('settlement_actions', []).append({'signal': action.name, 'pids': signalled})
                deadline = time.monotonic()+5
                while time.monotonic() < deadline:
                    alive = {}
                    for pid, identity in remaining.items():
                        try:
                            if self.epoch(pid) == identity:
                                alive[pid] = identity
                        except subprocess.CalledProcessError:
                            pass
                    remaining = alive
                    if not remaining:
                        break
                    time.sleep(.1)
            except (Refusal, OSError, subprocess.SubprocessError):
                errors.append('owned-process-settlement-inspector')
                break
        if remaining:
            errors.append('owned-process-epochs-remain')
        # A failed start can reserve an empty PID file before spawning. Never
        # infer closure from that file alone: also inspect the exact native image.
        if self.daemon_start_attempted:
            image = subprocess.run(['/usr/sbin/lsof', '-nP', str(self.args.native)], capture_output=True, text=True, timeout=10)
            if image.returncode != 1 or image.stdout.strip():
                errors.append('native-image-owners-remain-or-inspector-unavailable')
        self.receipt['cleanup'] = {'passed': not errors, 'failures': errors,
            'captured_pids': list(owned), 'remaining_pids': list(remaining),
            'private_tmux_socket': str(self.tmuxsocket)}
        for stream in self.streams:
            stream.close()
        return not errors


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('native', 'webmux', 'node', 'auth-home', 'output'):
        parser.add_argument('--'+name, type=Path, required=True)
    for name in ('native-sha256', 'webmux-sha256', 'code-mode-host-sha256', 'model'):
        parser.add_argument('--'+name, required=True)
    parser.add_argument('--profile', choices=('full', 'controller'), default='full',
        help='Controller profile preserves all original viewers; full additionally qualifies viewer replacement')
    parser.add_argument('--ssh-host', help='Real SSH profile; must resolve to this kernel and account; uses private helpers/tmux')
    args = parser.parse_args()
    args.ssh_boot_sha256 = None
    require(os.uname().sysname == 'Darwin', 'native-macos-required')
    require(not args.output.exists(), 'exclusive-output-required')
    for name in ('native', 'webmux'):
        binary = getattr(args, name).resolve(strict=True)
        expected = getattr(args, name+'_sha256')
        require(re.fullmatch('[0-9a-f]{64}', expected) and digest(binary) == expected, name+'-hash-mismatch')
        setattr(args, name, binary)
    host = args.native.with_name('codex-code-mode-host')
    require(host.is_file() and os.access(host, os.X_OK), 'adjacent-code-mode-host-required')
    require(re.fullmatch('[0-9a-f]{64}', args.code_mode_host_sha256) and
        digest(host) == args.code_mode_host_sha256, 'code-mode-host-hash-mismatch')
    args.auth_home = args.auth_home.resolve(strict=True)
    args.node = args.node.resolve(strict=True)
    if args.ssh_host:
        require(re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9.-]{0,252}', args.ssh_host), 'ssh-host-name-required')
        local_boot = subprocess.check_output(['/usr/sbin/sysctl', '-n', 'kern.bootsessionuuid'], text=True, timeout=5).strip()
        code = 'import json,os,subprocess;print(json.dumps({"uid":os.getuid(),"boot":subprocess.check_output(["/usr/sbin/sysctl","-n","kern.bootsessionuuid"],text=True).strip()}))'
        peer = subprocess.run(['/usr/bin/ssh', '-o', 'BatchMode=yes', '-o', 'ConnectTimeout=5', '-o', 'ControlMaster=no', '-o', 'ControlPath=none', args.ssh_host,
            shlex.join(['/usr/bin/python3', '-c', code])], capture_output=True, text=True, timeout=15, check=True)
        identity = json.loads(peer.stdout)
        require(identity.get('uid') == os.getuid() and identity.get('boot') == local_boot, 'ssh-profile-requires-this-kernel-and-account')
        args.ssh_boot_sha256 = hashlib.sha256(local_boot.encode()).hexdigest()
    fixture = Fixture(args)
    passed = False
    def interrupted(_signal, _frame):
        raise Refusal('qualification-interrupted')
    signal.signal(signal.SIGINT, interrupted)
    signal.signal(signal.SIGTERM, interrupted)
    try:
        fixture.qualify()
        passed = True
    except (Refusal, OSError, ValueError, TypeError, KeyError, subprocess.SubprocessError) as error:
        # Never put native error bodies, raw model text or capabilities in receipts.
        fixture.receipt['failure'] = str(error) if isinstance(error, Refusal) else type(error).__name__
        fixture.receipt['model_request_outcome_uncertain'] = fixture.receipt['model_requests_attempted'] != fixture.receipt['model_requests_submitted']
    finally:
        try:
            settled = fixture.cleanup()
        except (Refusal, OSError, ValueError, subprocess.SubprocessError) as error:
            settled = False
            fixture.receipt['cleanup'] = {'passed': False, 'failure': type(error).__name__}
        fixture.receipt['worker_end'] = {'load': list(os.getloadavg()), 'free_disk_bytes': shutil.disk_usage(fixture.root).free}
        fixture.receipt['passed'] = passed and settled
        fixture.receipt['active_work_qualified'] = passed and settled
        fixture.receipt['four_session_survival_qualified'] = passed and settled
        fixture.receipt['active_viewer_handoff_qualified'] = passed and settled and args.profile == 'full'
        fixture.receipt['worktree_read_parity_qualified'] = passed and settled and args.profile == 'full'
        fixture.receipt['worktree_creation_qualified'] = False
        fixture.save()
        fixture.output.close()
    print(json.dumps({'passed': fixture.receipt['passed'], 'fixture_root': str(fixture.root), 'failure': fixture.receipt.get('failure')}))
    return 0 if fixture.receipt['passed'] else 1


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except Refusal as error:
        print(json.dumps({'passed': False, 'preflight_failure': str(error)}))
        raise SystemExit(1)
