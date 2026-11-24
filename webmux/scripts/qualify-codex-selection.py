"""Actual CLI/PTY selection test; creates only isolated synthetic conversations.

This is UI selection evidence, not actual model/coding-work or survival evidence.
"""
import argparse
import base64
import binascii
import datetime
import fcntl
import hashlib
import hmac
import json
import os
from pathlib import Path
import pty
import re
import select
import signal
import socket
import struct
import subprocess
import tempfile
import termios
import time
import uuid

parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--output', required=True)
parser.add_argument('--expected-sha256', required=True, help='SHA-256 from the native build receipt')
parser.add_argument('--profile', required=True, choices=['managed', 'legacy-remote', 'remote-negative'])
parser.add_argument('--daemon-binary', help='Managed profile: exact installed daemon executable, read only')
parser.add_argument('--daemon-sha256', help='Managed profile: independently recorded executable SHA-256')
parser.add_argument('--webmux-binary', help='Exercise the actual WebMux launcher and signed recovery channel')
parser.add_argument('--webmux-sha256', help='SHA-256 of the exact WebMux launcher build')
parser.add_argument('--companion', action='store_true', help='Require independent selection in a second viewer on the same daemon')
parser.add_argument('--reconnect', action='store_true', help='Require selection to return after replacing only the isolated managed daemon')
parser.add_argument('--host-cache', action='store_true', help='Also require real authenticated host-RPC snapshots of each selected/lost state')
args = parser.parse_args()
if not re.fullmatch(r'[0-9a-f]{64}', args.expected_sha256):
    parser.error('--expected-sha256 must be 64 lowercase hexadecimal characters')
if args.profile == 'managed' and (not args.daemon_binary or not re.fullmatch(r'[0-9a-f]{64}', args.daemon_sha256 or '')):
    parser.error('managed profile requires --daemon-binary and --daemon-sha256')
if args.webmux_binary and not re.fullmatch(r'[0-9a-f]{64}', args.webmux_sha256 or ''):
    parser.error('--webmux-binary requires --webmux-sha256')
if args.companion and args.profile != 'managed':
    parser.error('--companion requires the managed profile')
if args.reconnect and args.profile != 'managed':
    parser.error('--reconnect requires the managed profile')
if args.host_cache and (not args.webmux_binary or args.profile != 'managed'):
    parser.error('--host-cache requires --webmux-binary and the managed profile')
binary = Path(args.binary).resolve()
output = Path(args.output).resolve()
assert not output.exists(), 'preserve prior receipts'
# The real coordination provisioning path has a native sockaddr_un bound.
root = Path(tempfile.mkdtemp(prefix='wm-native-selection-', dir='/tmp')).resolve()
home = root / 'home'
project = root / 'project'
home.mkdir(mode=0o700)
project.mkdir(mode=0o700)
socket_path = home / 'app-server-control' / 'app-server-control.sock' if args.profile == 'managed' else root / 'control.sock'
model_guard = socket.socket()
model_guard.bind(('127.0.0.1', 0))
model_port = model_guard.getsockname()[1]
env = {key: os.environ[key] for key in ['PATH', 'HOME', 'USER', 'LANG', 'TMPDIR'] if key in os.environ}
env.update(CODEX_HOME=str(home), TERM='xterm-256color', COLORTERM='truecolor', WEBMUX_SELECTION_TEST_KEY='synthetic-fixture-key')
(home / 'config.toml').write_text(f'''model = "gpt-5.3-codex"
model_provider = "selection-fixture"
check_for_update_on_startup = false
web_search = "disabled"
[model_providers.selection-fixture]
name = "Selection UI fixture, no model turns"
base_url = "http://127.0.0.1:{model_port}/v1"
env_key = "WEBMUX_SELECTION_TEST_KEY"
wire_api = "responses"
requires_openai_auth = false
[features]
enable_mcp_apps = false
[projects.{json.dumps(str(project))}]
trust_level = "trusted"
''')
os.chmod(home / 'config.toml', 0o600)
receipt = {'schema': 1, 'passed': False, 'fixture_root': str(root), 'selection_events': [],
           'profile': args.profile,
           'observer': 'webmux-signed-recovery' if args.webmux_binary else 'native-pipe',
           'host_cache_qualified': False,
           'active_work_qualified': False, 'four_session_survival_qualified': False,
           'fixture_kind': 'synthetic saved rollouts; actual native CLI and app-server; no model execution'}
server = viewer = None
retired_processes = []
companion = None
companion_reader = companion_writer = companion_master = companion_slave = None
companion_buffer = companion_tail = b''
companion_target = None
companion_transition = True
master = slave = reader = writer = None
logs = []
pipe_buffer = b''
terminal_tail = b''
packet_buffer = b''
wrapper_key = uuid.uuid4().hex
wrapper_instance = uuid.uuid4().hex
wrapper_session = str(uuid.uuid4())
output.parent.mkdir(parents=True, exist_ok=True)
receipt_file = output.open('x')
os.chmod(output, 0o600)

def checkpoint():
    receipt_file.seek(0)
    json.dump(receipt, receipt_file, indent=2)
    receipt_file.write('\n')
    receipt_file.truncate()
    receipt_file.flush()

checkpoint()

def binary_hash(path):
    digest = hashlib.sha256()
    with path.open('rb') as source:
        for chunk in iter(lambda: source.read(1024*1024), b''):
            digest.update(chunk)
    return digest.hexdigest()

def epoch(pid):
    result = subprocess.run(['ps', '-p', str(pid), '-o', 'lstart=,uid=,pgid='],
        capture_output=True, text=True, timeout=5)
    return result.stdout.strip() if result.returncode == 0 else None

def authenticated_event(item):
    item = dict(item)
    signature = item.pop('signature')
    canonical = json.dumps(item, separators=(',', ':'), ensure_ascii=False).replace('<', '\\u003c').replace('>', '\\u003e').replace('&', '\\u0026').encode()
    actual = base64.urlsafe_b64encode(hmac.new(wrapper_key.encode(), canonical, hashlib.sha256).digest()).rstrip(b'=').decode()
    assert hmac.compare_digest(signature, actual), 'invalid WebMux recovery signature'
    assert item.get('instance') == wrapper_instance
    return item

def host_rpc(operation, expect_rejection=False, **extra):
    request = {'protocol': 1, 'session_id': wrapper_session, 'key': wrapper_key,
        'operation': operation, **extra}
    result = subprocess.run([str(webmux_binary), '--recovery-rpc'], input=json.dumps(request),
        capture_output=True, text=True, cwd=project, env=env, timeout=10)
    result.check_returncode()
    value = json.loads(result.stdout)
    assert value.get('protocol') == 1
    if expect_rejection:
        assert value.get('error') and not value.get('events'), 'wrong key obtained host metadata'
        return value
    assert not value.get('error'), f'host RPC {operation} refused'
    return value

def verify_host_selection(expected):
    if not args.host_cache:
        return
    snapshot = host_rpc('snapshot')
    events = [item for item in snapshot.get('events', []) if item.get('kind') == 'cli']
    assert len(events) == 1, 'missing unambiguous host-cache CLI evidence'
    item = authenticated_event(events[0])
    assert item.get('cwd') == str(project) and item.get('cli') == 'codex'
    selected = item.get('session_id') if item.get('native_evidence') == 'native-selection' else None
    assert selected == expected
    if expected is None:
        assert item.get('native_evidence') == 'unavailable'
    observations = receipt.setdefault('host_cache_observations', [])
    revision = snapshot['revision']
    assert not observations or revision > observations[-1]['revision']
    observations.append({'thread_id': selected, 'revision': revision,
        'signature_verified': True, 'instance_verified': True})
    checkpoint()

class ManagedDaemon:
    """A daemon managed by its real lifecycle CLI, not a waitable broker child."""
    def __init__(self, executable):
        self.executable = executable
        self.pid = None
        self.identity = None
        self.stop_exit_code = None
        self.command_count = 0

    def launch(self):
        package = home / 'packages' / 'app-server-daemon'
        (package / 'fixture' / 'bin').mkdir(parents=True)
        (package / 'fixture' / 'bin' / 'codex').symlink_to(self.executable)
        (package / 'current').symlink_to('fixture')
        settings = home / 'app-server-daemon' / 'settings.json'
        settings.parent.mkdir(mode=0o700)
        settings.write_text(json.dumps({'remoteControlEnabled': False, 'featureOverrides': {},
            'shutdownGraceSeconds': 2, 'updater': {'autoUpdateEnabled': False, 'updateIntervalMinutes': 60}}))
        os.chmod(settings, 0o600)
        self.start()

    def start(self):
        settings = home / 'app-server-daemon' / 'settings.json'
        self.stop_exit_code = None
        try:
            result = self.command('start')
        except (OSError, subprocess.SubprocessError):
            # A CLI can fail after publishing its detached child. Retain that
            # owned PID for lifecycle cleanup rather than losing the handle.
            record_path = settings.parent / 'daemon.pid'
            if record_path.exists():
                record = json.loads(record_path.read_text())
                self.pid = record['pid']
                self.identity = epoch(self.pid)
            raise
        value = json.loads(result.stdout)
        self.pid = value['pid']
        self.identity = epoch(self.pid)
        assert self.identity and value['status'] == 'started'
        assert self.identity.split()[-2:] == [str(os.getuid()), str(self.pid)]
        assert Path(value['socketPath']) == socket_path
        assert Path(value['managedCodexPath']).resolve() == self.executable
        record = json.loads((settings.parent / 'daemon.pid').read_text())
        assert record['pid'] == self.pid
        assert not (settings.parent / 'daemon-updater.pid').exists()
        receipt['managed_start'] = {'exit_code': result.returncode, 'pid': self.pid,
            'epoch': self.identity, 'socket_path': str(socket_path), 'updater_disabled': True}
        receipt.setdefault('managed_starts', []).append(receipt['managed_start'])

    def command(self, action):
        result = subprocess.run([str(self.executable), 'app-server', 'daemon', action],
            stdin=subprocess.DEVNULL, capture_output=True, text=True, cwd=project, env=env, timeout=30)
        self.command_count += 1
        log = root / f'managed-{self.command_count}-{action}.log'
        with log.open('x') as stream:
            stream.write(result.stdout + result.stderr)
        os.chmod(log, 0o600)
        result.check_returncode()
        return result

    def poll(self):
        return None if self.pid is not None and epoch(self.pid) == self.identity else 0

    def terminate(self):
        if self.poll() is None:
            self.stop_exit_code = self.command('stop').returncode

    def kill(self):
        if self.poll() is None:
            os.kill(self.pid, signal.SIGKILL)

    def wait(self, timeout):
        deadline = time.monotonic() + timeout
        while self.poll() is None:
            if time.monotonic() >= deadline:
                raise subprocess.TimeoutExpired('owned managed daemon closure', timeout)
            time.sleep(0.1)
        return 0

    def closure(self):
        return {'pid': self.pid, 'exit_code': None, 'not_running': self.poll() is not None,
            'observation': 'process epoch no longer present; detached daemon is not a waitable child',
            'lifecycle_stop_exit_code': self.stop_exit_code}

def private_log(name):
    handle = open(root / name, 'xb', buffering=0)
    os.chmod(root / name, 0o600)
    logs.append(handle)
    return handle

def group_members(groups):
    result = subprocess.run(['ps', '-axo', 'pid=,pgid=,uid=,stat='],
        capture_output=True, text=True, timeout=5, check=True)
    return [
        {'pid': int(parts[0]), 'pgid': int(parts[1]), 'uid': int(parts[2]), 'state': parts[3]}
        for line in result.stdout.splitlines()
        if len(parts := line.split()) == 4 and int(parts[1]) in groups
    ]

def settle_groups(groups):
    if os.getpgrp() in groups:
        raise RuntimeError('refusing to signal the qualification broker process group')
    for action in [signal.SIGTERM, signal.SIGKILL]:
        members = group_members(groups)
        if not members:
            return []
        live = [item for item in members if not item['state'].startswith('Z')]
        for item in live:
            if item['uid'] != os.getuid():
                raise RuntimeError('fixture process group contains a different user')
            result = subprocess.run(['lsof', '-a', '-p', str(item['pid']), '-d', 'cwd', '-Fn'],
                capture_output=True, text=True, timeout=5)
            paths = [Path(line[1:]).resolve() for line in result.stdout.splitlines() if line.startswith('n')]
            if not paths and not any(row['pid'] == item['pid'] for row in group_members(groups)):
                continue
            if not paths or not all(path == root or root in path.parents for path in paths):
                raise RuntimeError('fixture helper ownership could not be verified from its working directory')
        for group in {item['pgid'] for item in live}:
            try:
                os.killpg(group, action)
                receipt.setdefault('settlement_actions', []).append({
                    'group': group, 'signal': action.name,
                    'member_pids': [item['pid'] for item in live if item['pgid'] == group],
                })
            except ProcessLookupError:
                pass
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            members = group_members(groups)
            if not members:
                return []
            time.sleep(0.05)
    return members

def pump(duration, expected='unmatched'):
    global pipe_buffer, terminal_tail, packet_buffer, companion_buffer, companion_tail, companion_transition
    until = time.monotonic() + duration
    while time.monotonic() < until:
        events, _, _ = select.select([fd for fd in [reader, master, companion_reader, companion_master] if fd is not None], [], [], min(0.1, max(0, until-time.monotonic())))
        for fd in events:
            try:
                data = os.read(fd, 65536)
            except OSError:
                data = b''
            if fd == companion_reader:
                if not data:
                    raise RuntimeError('companion observation channel closed unexpectedly')
                companion_buffer += data
                while b'\n' in companion_buffer:
                    line, companion_buffer = companion_buffer.split(b'\n', 1)
                    assert len(line) <= 512
                    item = json.loads(line)
                    assert item['schema'] == 'codex.tui.selection.v1' and item['pid'] == companion.pid
                    previous = receipt['companion_events']
                    assert not previous or item['sequence'] > previous[-1]['sequence']
                    assert item['thread_id'] == companion_target or (companion_transition and item['thread_id'] is None), 'another viewer changed companion selection'
                    if item['thread_id'] == companion_target:
                        companion_transition = False
                    previous.append(item)
                    checkpoint()
                continue
            if fd == reader:
                if not data:
                    if expected != 'unmatched':
                        raise RuntimeError('observer pipe closed before expected selection')
                    continue
                pipe_buffer += data
                while b'\n' in pipe_buffer:
                    line, pipe_buffer = pipe_buffer.split(b'\n', 1)
                    item = json.loads(line)
                    assert item['schema'] == 'codex.tui.selection.v1'
                    assert item['pid'] == viewer.pid
                    previous = receipt['selection_events']
                    assert not previous or item['sequence'] > previous[-1]['sequence']
                    previous.append(item)
                    checkpoint()
                    if item['thread_id'] == expected:
                        return item
            elif data:
                is_companion = fd == companion_master
                (companion_log if is_companion else terminal_log).write(data)
                matched = False
                if args.webmux_binary and not is_companion:
                    packet_buffer += data
                    prefix = b'\x1b]777;webmux-recovery;'
                    while prefix in packet_buffer:
                        start = packet_buffer.index(prefix)
                        end = packet_buffer.find(b'\x07', start)
                        if end < 0:
                            packet_buffer = packet_buffer[start:]
                            assert len(packet_buffer) <= 64*1024, 'oversized recovery packet'
                            break
                        encoded = packet_buffer[start+len(prefix):end]
                        packet_buffer = packet_buffer[end+1:]
                        try:
                            item = json.loads(base64.b64decode(encoded + b'=' * (-len(encoded) % 4), altchars=b'-_', validate=True))
                        except (binascii.Error, json.JSONDecodeError, UnicodeDecodeError):
                            # Match recovery.Decoder.Feed: discard malformed PTY
                            # frames. They never establish a selection or satisfy
                            # a wait; a fresh signed event is still required within
                            # the original deadline. Preserve transport loss counts.
                            receipt['malformed_metadata_frames_discarded'] = receipt.get('malformed_metadata_frames_discarded', 0) + 1
                            checkpoint()
                            continue
                        item = authenticated_event(item)
                        if item.get('kind') != 'cli':
                            continue
                        evidence = item.get('native_evidence')
                        if evidence not in ['native-selection', 'unavailable']:
                            continue
                        thread = item.get('session_id') if evidence == 'native-selection' else None
                        assert item.get('cwd') == str(project) and item.get('cli') == 'codex'
                        receipt['selection_events'].append({'thread_id': thread, 'native_evidence': evidence,
                            'signature_verified': True, 'instance_verified': True})
                        checkpoint()
                        matched = matched or thread == expected
                    if prefix not in packet_buffer:
                        packet_buffer = packet_buffer[-len(prefix):]
                prior_tail = companion_tail if is_companion else terminal_tail
                scan = prior_tail + data
                responses = [(b'\x1b[6n', b'\x1b[1;1R'), (b'\x1b[c', b'\x1b[?1;2c'),
                    (b'\x1b[>c', b'\x1b[>0;1;0c'), (b'\x1b[?u', b'\x1b[?0u'),
                    (b'\x1b]11;?\x07', b'\x1b]11;rgb:0000/0000/0000\x07')]
                for query, reply in responses:
                    for match in re.finditer(re.escape(query), scan):
                        if match.end() > len(prior_tail):
                            os.write(companion_master if is_companion else master, reply)
                if is_companion:
                    companion_tail = scan[-64:]
                else:
                    terminal_tail = scan[-64:]
                if matched:
                    return receipt['selection_events'][-1]
        if viewer.poll() is not None and expected != 'unmatched':
            raise RuntimeError(f'viewer exited {viewer.returncode} before expected selection')
        if companion is not None and companion.poll() is not None:
            raise RuntimeError('companion viewer exited unexpectedly')
    if expected != 'unmatched':
        raise TimeoutError(f'no observer selection {expected} within {duration}s')

try:
    receipt['binary_sha256'] = binary_hash(binary)
    if receipt['binary_sha256'] != args.expected_sha256:
        raise RuntimeError('native binary does not match the expected build receipt hash')
    webmux_binary = Path(args.webmux_binary).resolve() if args.webmux_binary else None
    if webmux_binary is not None:
        receipt['webmux_binary_sha256'] = binary_hash(webmux_binary)
        if receipt['webmux_binary_sha256'] != args.webmux_sha256:
            raise RuntimeError('WebMux binary does not match its expected build hash')
    daemon_binary = Path(args.daemon_binary).resolve() if args.profile == 'managed' else None
    if daemon_binary is not None:
        receipt['daemon_binary_sha256'] = binary_hash(daemon_binary)
        if receipt['daemon_binary_sha256'] != args.daemon_sha256:
            raise RuntimeError('managed daemon binary does not match its expected hash')
    ids = [str(uuid.uuid4()) for _ in range(3 if args.companion else 2)]
    # Use the upstream app_test_support rollout fixture format. Empty thread/start
    # sessions have no persisted rollout and cannot exercise a saved-chat resume.
    for index, thread_id in enumerate(ids):
        timestamp = datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(seconds=2-index)
        instant = timestamp.isoformat().replace('+00:00', 'Z')
        folder = home / 'sessions' / timestamp.strftime('%Y/%m/%d')
        folder.mkdir(parents=True, exist_ok=True)
        path = folder / f'rollout-{timestamp.strftime("%Y-%m-%dT%H-%M-%S")}-{thread_id}.jsonl'
        meta = {
            'id': thread_id, 'session_id': thread_id, 'timestamp': instant, 'cwd': str(project),
            'originator': 'codex', 'cli_version': '0.172.0', 'source': 'cli',
            'account_scoped': False, 'attached_thread': False, 'model_provider': 'selection-fixture',
            'history_mode': 'legacy', 'selected_capability_roots': [],
        }
        preview = f'Synthetic selection fixture {index}; no model execution.'
        lines = [
            {'timestamp': instant, 'type': 'session_meta', 'payload': meta},
            {'timestamp': instant, 'type': 'response_item', 'payload': {'type': 'message', 'role': 'user',
                'content': [{'type': 'input_text', 'text': preview}]}},
            {'timestamp': instant, 'type': 'event_msg', 'payload': {'type': 'user_message', 'message': preview, 'kind': 'plain'}},
        ]
        path.write_text(''.join(json.dumps(line)+'\n' for line in lines))
        os.chmod(path, 0o600)
    if daemon_binary is not None:
        server = ManagedDaemon(daemon_binary)
        server.launch()
    else:
        server_log = private_log('server.log')
        server = subprocess.Popen([str(binary), 'app-server', '--listen', f'unix://{socket_path}'],
            stdin=subprocess.DEVNULL, stdout=server_log, stderr=subprocess.STDOUT, cwd=project, env=env, start_new_session=True)
    receipt['daemon_pid'] = server.pid
    until = time.monotonic()+30
    while not socket_path.exists():
        if server.poll() is not None:
            raise RuntimeError('isolated server exited before publishing its socket')
        if time.monotonic() > until:
            raise TimeoutError('isolated server socket not ready')
        time.sleep(0.1)
    receipt['synthetic_threads'] = ids
    if webmux_binary is None:
        reader, writer = os.pipe()
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 120, 0, 0))
    def own_terminal():
        os.setsid()
        fcntl.ioctl(0, termios.TIOCSCTTY, 0)
    terminal_log = private_log('terminal.log')
    launch = [str(binary)]
    observer_flags = ['--selection-observer-fd', str(writer)]
    if webmux_binary is not None:
        native_dir = root / 'native-bin'
        wrapper_dir = root / 'wrapper-bin'
        native_dir.mkdir()
        wrapper_dir.mkdir()
        (native_dir / 'codex').symlink_to(binary)
        (wrapper_dir / 'codex').symlink_to(webmux_binary)
        env.update(WEBMUX_BROWSER_ORIGINAL_PATH=str(native_dir) + os.pathsep + env.get('PATH', ''),
            WEBMUX_RECOVERY_INSTANCE=wrapper_instance, WEBMUX_RECOVERY_SESSION=wrapper_session,
            WEBMUX_RECOVERY_KEY=wrapper_key, WEBMUX_RECOVERY_HELPER=str(webmux_binary),
            WEBMUX_RECOVERY_CACHE_HOME=str(root / 'recovery-cache'),
            WEBMUX_COORDINATION_CACHE_HOME=str(root / 'coordination-cache'))
        launch = [str(wrapper_dir / 'codex')]
        observer_flags = []  # The actual Go launcher must create and own its native pipe.
        if args.host_cache:
            # This real provisioning operation initializes only private host state
            # and an unused private directory; PrepareSocket starts no listener or
            # durable provider. The policy call enables capture in that fixture.
            prepared = host_rpc('coordination-prepare', coordination_channel=uuid.uuid4().hex)
            assert root in Path(prepared['coordination_socket']).parents
            assert not Path(prepared['coordination_socket']).exists()
            host_rpc('snapshot', expect_rejection=True, key='0'*32)
            receipt['host_cache_wrong_key_refused'] = True
            assert host_rpc('policy', recording=True)['recording'] is True
    viewer = subprocess.Popen(launch + ['--remote', f'unix://{socket_path}'] + observer_flags +
        ['--no-alt-screen', 'resume', ids[0]], stdin=slave, stdout=slave, stderr=slave,
        cwd=project, env=env, pass_fds=(writer,) if writer is not None else (), preexec_fn=own_terminal)
    receipt['viewer_pid'] = viewer.pid
    viewer_epoch = epoch(viewer.pid)
    if writer is not None:
        os.close(writer)
    writer = None
    os.close(slave)
    slave = None
    if args.companion:
        companion_target = ids[2]
        receipt['companion_events'] = []
        companion_reader, companion_writer = os.pipe()
        companion_master, companion_slave = pty.openpty()
        fcntl.ioctl(companion_slave, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 120, 0, 0))
        companion_log = private_log('companion-terminal.log')
        companion = subprocess.Popen([str(binary), '--remote', f'unix://{socket_path}',
            '--selection-observer-fd', str(companion_writer), '--no-alt-screen', 'resume', companion_target],
            stdin=companion_slave, stdout=companion_slave, stderr=companion_slave,
            cwd=project, env=env, pass_fds=(companion_writer,), preexec_fn=own_terminal)
        receipt['companion_pid'] = companion.pid
        os.close(companion_writer)
        companion_writer = None
        os.close(companion_slave)
        companion_slave = None
    if args.profile == 'remote-negative':
        pump(20)
        assert receipt['selection_events'] and all(item['thread_id'] is None for item in receipt['selection_events'])
        receipt['remote_selection_unavailable'] = True
    else:
        pump(40, ids[0])
        verify_host_selection(ids[0])
        if args.companion:
            until = time.monotonic() + 40
            while not receipt['companion_events'] or receipt['companion_events'][-1]['thread_id'] != ids[2]:
                if time.monotonic() >= until:
                    raise TimeoutError('companion did not settle on its own conversation')
                pump(0.2)
        for thread_id in [ids[1], ids[0]]:
            pump(0.3)
            os.write(master, b'\x1b[200~' + f'/resume {thread_id}'.encode() + b'\x1b[201~')
            pump(0.5)
            os.write(master, b'\r')
            pump(30, thread_id)
            verify_host_selection(thread_id)
            pump(1)  # Drain the other viewer before assessing isolation.
        if args.companion:
            assert receipt['companion_events'][-1]['thread_id'] == ids[2]
            primary_count = len(receipt['selection_events'])
            companion_target = ids[1]
            companion_transition = True
            os.write(companion_master, b'\x1b[200~' + f'/resume {companion_target}'.encode() + b'\x1b[201~')
            pump(0.5)
            os.write(companion_master, b'\r')
            until = time.monotonic() + 30
            while receipt['companion_events'][-1]['thread_id'] != companion_target:
                if time.monotonic() >= until:
                    raise TimeoutError('companion did not switch independently')
                pump(0.2)
            pump(1)
            assert receipt['selection_events'][-1]['thread_id'] == ids[0]
            assert all(item['thread_id'] == ids[0] for item in receipt['selection_events'][primary_count:])
            receipt['concurrent_viewer_isolation_qualified'] = True
    assert viewer.poll() is None and server.poll() is None
    receipt['interactive_switch_qualified'] = args.profile != 'remote-negative'
    checkpoint()
    # Only the owned isolated server is terminated to verify selection demotion.
    companion_target = None
    companion_transition = True
    server.terminate()
    server.wait(timeout=10)
    previous_count = len(receipt['selection_events'])
    pump(20, None) if args.profile != 'remote-negative' else pump(2)
    pump(2)
    if args.companion:
        assert receipt['companion_events'][-1]['thread_id'] is None
    assert args.profile == 'remote-negative' or len(receipt['selection_events']) > previous_count
    assert all(item['thread_id'] is None for item in receipt['selection_events'][previous_count:])
    receipt['loss_demoted_to_unavailable'] = True
    verify_host_selection(None)
    if args.reconnect:
        retired_processes.append(server.closure())
        assert retired_processes[-1]['not_running']
        old_pid = server.pid
        assert settle_groups({old_pid}) == []
        companion_target = ids[1] if args.companion else None
        companion_transition = True
        server.start()
        assert server.pid != old_pid
        checkpoint()
        pump(40, ids[0])
        verify_host_selection(ids[0])
        if args.companion:
            until = time.monotonic() + 40
            while receipt['companion_events'][-1]['thread_id'] != companion_target:
                if time.monotonic() >= until:
                    raise TimeoutError('companion did not recover its selected conversation')
                pump(0.2)
        assert viewer.poll() is None and epoch(viewer.pid) == viewer_epoch
        receipt['reconnect_selection_qualified'] = True
        receipt['native_daemon_continuity_qualified'] = False
    receipt['host_cache_qualified'] = args.host_cache
except Exception as error:
    receipt['error'] = f'{type(error).__name__}: {error}'
finally:
    checkpoint()
    # Release the terminal before waiting. A reconnecting TUI can be blocked
    # writing to a full PTY when its broker stops draining during cleanup.
    if master is not None:
        os.close(master)
        master = None
    if companion_master is not None:
        os.close(companion_master)
        companion_master = None
    for process in [viewer, companion, server]:
        if process is not None and process.poll() is None:
            try:
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    if process.poll() is None:
                        process.kill()
                    process.wait(timeout=5)
            except (OSError, subprocess.SubprocessError) as error:
                receipt.setdefault('cleanup_errors', []).append({'pid': process.pid, 'error': str(error)})
    for fd in [reader, writer, master, slave, companion_reader, companion_writer, companion_master, companion_slave]:
        if fd is not None:
            os.close(fd)
    for handle in logs:
        handle.close()
    model_guard.close()
    receipt['owned_processes_closed'] = retired_processes + [
        process.closure() if isinstance(process, ManagedDaemon) else
        {'pid': process.pid, 'exit_code': process.poll(), 'not_running': process.poll() is not None}
        for process in [viewer, companion, server] if process is not None and process.pid is not None
    ]
    try:
        groups = {item['pid'] for item in receipt['owned_processes_closed']}
        receipt['remaining_owned_group_members'] = [
            item['pid'] for item in settle_groups(groups)
        ] if groups else []
    except (OSError, subprocess.SubprocessError, ValueError) as error:
        receipt.setdefault('cleanup_errors', []).append({'error': str(error)})
        receipt['remaining_owned_group_members'] = None
    receipt['cleanup_qualified'] = (
        not receipt.get('cleanup_errors') and receipt['remaining_owned_group_members'] == [] and all(
            item['not_running'] for item in receipt['owned_processes_closed']
        )
    )
    receipt['framing_integrity_qualified'] = receipt.get('malformed_metadata_frames_discarded', 0) == 0
    receipt['passed'] = bool(
        (receipt.get('interactive_switch_qualified') or receipt.get('remote_selection_unavailable'))
        and receipt.get('loss_demoted_to_unavailable')
        and receipt['cleanup_qualified']
        and (not args.companion or receipt.get('concurrent_viewer_isolation_qualified'))
        and (not args.reconnect or receipt.get('reconnect_selection_qualified'))
        and (not args.host_cache or receipt.get('host_cache_qualified'))
        and not receipt.get('error')
    )
    checkpoint()
    receipt_file.close()
print(json.dumps(receipt))
raise SystemExit(0 if receipt['passed'] else 1)
