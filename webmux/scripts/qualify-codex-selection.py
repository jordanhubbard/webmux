"""Actual CLI/PTY selection test; creates only isolated synthetic conversations.

This is UI selection evidence, not actual model/coding-work or survival evidence.
"""
import argparse
import datetime
import fcntl
import hashlib
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
args = parser.parse_args()
if not re.fullmatch(r'[0-9a-f]{64}', args.expected_sha256):
    parser.error('--expected-sha256 must be 64 lowercase hexadecimal characters')
binary = Path(args.binary).resolve()
output = Path(args.output).resolve()
assert not output.exists(), 'preserve prior receipts'
root = Path(tempfile.mkdtemp(prefix='wm-native-selection-')).resolve()
home = root / 'home'
project = root / 'project'
home.mkdir(mode=0o700)
project.mkdir(mode=0o700)
socket_path = root / 'control.sock'
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
           'active_work_qualified': False, 'four_session_survival_qualified': False,
           'fixture_kind': 'synthetic saved rollouts; actual native CLI and app-server; no model execution'}
server = viewer = None
master = slave = reader = writer = None
logs = []
pipe_buffer = b''
terminal_tail = b''
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
    global pipe_buffer, terminal_tail
    until = time.monotonic() + duration
    while time.monotonic() < until:
        events, _, _ = select.select([reader, master], [], [], min(0.1, max(0, until-time.monotonic())))
        for fd in events:
            try:
                data = os.read(fd, 65536)
            except OSError:
                data = b''
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
                terminal_log.write(data)
                scan = terminal_tail + data
                responses = [(b'\x1b[6n', b'\x1b[1;1R'), (b'\x1b[c', b'\x1b[?1;2c'),
                    (b'\x1b[>c', b'\x1b[>0;1;0c'), (b'\x1b[?u', b'\x1b[?0u'),
                    (b'\x1b]11;?\x07', b'\x1b]11;rgb:0000/0000/0000\x07')]
                for query, reply in responses:
                    for match in re.finditer(re.escape(query), scan):
                        if match.end() > len(terminal_tail):
                            os.write(master, reply)
                terminal_tail = scan[-64:]
        if viewer.poll() is not None and expected != 'unmatched':
            raise RuntimeError(f'viewer exited {viewer.returncode} before expected selection')
    if expected != 'unmatched':
        raise TimeoutError(f'no observer selection {expected} within {duration}s')

try:
    digest = hashlib.sha256()
    with binary.open('rb') as source:
        for chunk in iter(lambda: source.read(1024*1024), b''):
            digest.update(chunk)
    receipt['binary_sha256'] = digest.hexdigest()
    if receipt['binary_sha256'] != args.expected_sha256:
        raise RuntimeError('native binary does not match the expected build receipt hash')
    ids = [str(uuid.uuid4()) for _ in range(2)]
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
    server_log = private_log('server.log')
    server = subprocess.Popen([str(binary), 'app-server', '--listen', f'unix://{socket_path}'],
        stdin=subprocess.DEVNULL, stdout=server_log, stderr=subprocess.STDOUT, cwd=project, env=env, start_new_session=True)
    receipt['daemon_pid'] = server.pid
    until = time.monotonic()+30
    while not socket_path.exists():
        if server.poll() is not None:
            raise RuntimeError(f'isolated server exited {server.returncode}')
        if time.monotonic() > until:
            raise TimeoutError('isolated server socket not ready')
        time.sleep(0.1)
    receipt['synthetic_threads'] = ids
    reader, writer = os.pipe()
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 120, 0, 0))
    def own_terminal():
        os.setsid()
        fcntl.ioctl(0, termios.TIOCSCTTY, 0)
    terminal_log = private_log('terminal.log')
    viewer = subprocess.Popen([str(binary), '--remote', f'unix://{socket_path}', '--selection-observer-fd', str(writer),
        '--no-alt-screen', 'resume', ids[0]], stdin=slave, stdout=slave, stderr=slave,
        cwd=project, env=env, pass_fds=(writer,), preexec_fn=own_terminal)
    receipt['viewer_pid'] = viewer.pid
    os.close(writer)
    writer = None
    os.close(slave)
    slave = None
    pump(40, ids[0])
    for thread_id in [ids[1], ids[0]]:
        pump(0.3)
        os.write(master, b'\x1b[200~' + f'/resume {thread_id}'.encode() + b'\x1b[201~')
        pump(0.5)
        os.write(master, b'\r')
        pump(30, thread_id)
    assert viewer.poll() is None and server.poll() is None
    receipt['interactive_switch_qualified'] = True
    checkpoint()
    # Only the owned isolated server is terminated to verify selection demotion.
    server.terminate()
    server.wait(timeout=10)
    previous_count = len(receipt['selection_events'])
    pump(20, None)
    pump(2)
    assert len(receipt['selection_events']) > previous_count
    assert all(item['thread_id'] is None for item in receipt['selection_events'][previous_count:])
    receipt['loss_demoted_to_unavailable'] = True
except Exception as error:
    receipt['error'] = f'{type(error).__name__}: {error}'
finally:
    checkpoint()
    # Release the terminal before waiting. A reconnecting TUI can be blocked
    # writing to a full PTY when its broker stops draining during cleanup.
    if master is not None:
        os.close(master)
        master = None
    for process in [viewer, server]:
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
    for fd in [reader, writer, master, slave]:
        if fd is not None:
            os.close(fd)
    for handle in logs:
        handle.close()
    model_guard.close()
    receipt['owned_processes_closed'] = [
        {'pid': process.pid, 'exit_code': process.poll()}
        for process in [viewer, server] if process is not None
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
            item['exit_code'] is not None for item in receipt['owned_processes_closed']
        )
    )
    receipt['passed'] = bool(
        receipt.get('interactive_switch_qualified')
        and receipt.get('loss_demoted_to_unavailable')
        and receipt['cleanup_qualified']
        and not receipt.get('error')
    )
    checkpoint()
    receipt_file.close()
print(json.dumps(receipt))
raise SystemExit(0 if receipt['passed'] else 1)
