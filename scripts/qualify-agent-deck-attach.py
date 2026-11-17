#!/usr/bin/env python3
"""Compare real Agent Deck startup/input under private Linux PTYs, without replies.

This is a transport control, not coding-conversation or reboot acceptance.
The caller supplies published binaries and a new evidence directory. No global
coordinator, default tmux socket, installed service or credentials are changed.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import pty
import select
import shlex
import signal
import struct
import subprocess
import sys
import termios
import time
import tempfile
import uuid
import fcntl


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def observe(argv, env, directory, inputs, *, timeout=7, late=None, attached=None, delivered=None):
    start = time.monotonic()
    pid, master = pty.fork()
    if pid == 0:
        os.chdir(directory)
        os.execvpe(argv[0], argv, env)
    fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack('HHHH', 24, 80, 0, 0))
    output, sent = bytearray(), []
    status = None
    try:
        while time.monotonic() - start < timeout:
            elapsed = time.monotonic() - start
            for scheduled, token in inputs:
                if elapsed >= scheduled and token not in [x['token'] for x in sent]:
                    assert elapsed < scheduled + .5, 'input schedule missed its observation window'
                    os.write(master, (token + '\r').encode())
                    sent.append({'token': token, 'seconds': elapsed})
            # termenv's old timeout is per input byte. A fixed-time late
            # control can itself prolong that timeout and never reach tmux.
            # Observe this exact PTY client's native handoff before the late
            # control; early input and the fixed version's latency oracle stay
            # unchanged. No terminal query is ever answered.
            if late and len(sent) == len(inputs) and elapsed > inputs[-1][0] + .1 and attached(pid):
                os.write(master, (late + '\r').encode())
                sent.append({'token': late, 'seconds': elapsed, 'native_client_observed': True})
            if late and len(sent) > len(inputs) and delivered(late):
                break
            readable, _, _ = select.select([master], [], [], .02)
            if readable:
                try:
                    data = os.read(master, 16384)
                except OSError:
                    data = b''
                output.extend(data)
                assert len(output) <= 128 << 10, 'unexpectedly large fixture output'
            exited, code = os.waitpid(pid, os.WNOHANG)
            if exited:
                status = code
                break
    finally:
        if status is None:
            os.killpg(pid, signal.SIGTERM)
            deadline = time.monotonic() + 3
            while time.monotonic() < deadline:
                exited, code = os.waitpid(pid, os.WNOHANG)
                if exited:
                    status = code
                    break
                time.sleep(.02)
            if status is None:
                os.killpg(pid, signal.SIGKILL)
                _, status = os.waitpid(pid, 0)
        os.close(master)
    return {'duration': time.monotonic() - start, 'sent': sent,
            'status': status, 'color_query': b'\x1b]11;?' in output,
            'cursor_query': b'\x1b[6n' in output}, bytes(output), start


def qualify(binary, version, directory):
    directory.mkdir(mode=0o700)
    env = {k: v for k, v in os.environ.items()
           if not k.startswith(('AGENTDECK_', 'AGENT_DECK_')) and k not in ['TMUX', 'TMUX_PANE', 'CLAUDE_CONFIG_DIR', 'ANTHROPIC_API_KEY']}
    env.update(HOME=str(directory / 'home'), TERM='xterm-256color', COLORTERM='truecolor', DO_NOT_TRACK='1', SHELL='/bin/bash')
    for key in ['XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME']:
        env[key] = str(directory / key.lower())
    env['TMUX_TMPDIR'] = tempfile.mkdtemp(prefix='wm-deck-', dir='/tmp')
    for key in ['HOME', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME']:
        Path(env[key]).mkdir(mode=0o700)
    socket = 'wm-deck-' + uuid.uuid4().hex[:12]
    config = Path(env['XDG_CONFIG_HOME']) / 'agent-deck'
    config.mkdir(mode=0o700)
    (config / 'config.toml').write_text('[launch]\ninject_identity = false\n[security]\nauto_trust_folders = false\n[tmux]\nsocket_name = ' + json.dumps(socket) + '\n')
    def deck(*args):
        result = subprocess.run([str(binary), *args], env=env, cwd=directory,
                                capture_output=True, text=True, timeout=25)
        assert result.returncode == 0, (result.stdout + result.stderr)[-2000:]
        return result.stdout
    assert deck('version').split()[:3] == ['Agent', 'Deck', 'v' + version]
    before = digest(binary)
    startup, raw, _ = observe([str(binary), 'version'], env, directory, [])
    (directory / 'version.pty').write_bytes(raw)
    (directory / 'startup.json').write_text(json.dumps(startup, indent=2)+'\n')
    assert os.waitstatus_to_exitcode(startup['status']) == 0
    payload = directory / 'echo.py'
    delivered = directory / 'delivered.jsonl'
    payload.write_text('import json,os,sys,time\nf=open(sys.argv[1],"a",buffering=1)\nf.write(json.dumps({"ready":True,"pid":os.getpid()})+"\\n")\nfor line in sys.stdin:\n f.write(json.dumps({"line":line.strip(),"at":time.monotonic(),"pid":os.getpid()})+"\\n")\n')
    project = directory / 'project'
    project.mkdir()
    created = json.loads(deck('add', str(project), '--cmd', shlex.join([sys.executable, str(payload), str(delivered)]),
                              '--no-identity', '--no-parent', '--tmux-socket', socket, '--title', 'WebMux attach input control', '--json'))
    ident = created['id']
    (directory / 'fixture.json').write_text(json.dumps({'id': ident, 'tmux_runtime': env['TMUX_TMPDIR'], 'socket': socket}, indent=2)+'\n')
    result = None
    try:
        deck('session', 'start', ident, '--json')
        deadline = time.monotonic() + 10
        while not delivered.exists() and time.monotonic() < deadline:
            time.sleep(.02)
        assert delivered.exists(), 'real payload did not start'
        ready = json.loads(delivered.read_text().splitlines()[0])
        trials = []
        for index, early_at in enumerate([.3, 1., 3.]):
            prior = len(delivered.read_text().splitlines())
            challenge = uuid.uuid4().hex
            early, late = challenge + '-early', challenge + '-late'
            def attached(pid):
                clients = subprocess.run(['tmux', '-L', socket, 'list-clients', '-F', '#{client_pid}'], env=env, capture_output=True, text=True, timeout=2)
                return str(pid) in clients.stdout.splitlines()
            def seen(token):
                return any(json.loads(line).get('line') == token for line in delivered.read_text().splitlines()[prior:])
            attach, raw, started = observe([str(binary), 'session', 'attach', ident], env, directory,
                                           [(early_at, early)], timeout=early_at + 7,
                                           late=late, attached=attached, delivered=seen)
            (directory / f'attach-{index}.pty').write_bytes(raw)
            rows = [json.loads(line) for line in delivered.read_text().splitlines()[prior:]]
            received = [row['line'] for row in rows if 'line' in row]
            assert len(attach['sent']) == 2 and attach['sent'][1]['native_client_observed']
            assert received.count(late) == 1, 'late control did not reach the same live native payload'
            assert all(row['pid'] == ready['pid'] for row in rows), 'payload identity changed'
            if version == '1.16.34':
                assert startup['color_query'] and startup['cursor_query'] and startup['duration'] >= 4
                assert early not in received, 'baseline did not reproduce early input loss'
            else:
                assert not startup['color_query'] and not startup['cursor_query'] and startup['duration'] < 1
                assert received == [early, late], 'fixed version lost, duplicated or reordered early input'
                assert rows[0]['at'] - started < early_at + 1, 'early input was buffered past its delivery window'
            trials.append({'early_at': early_at, 'attach': attach, 'received': rows})
        assert digest(binary) == before
        result = {'version': version, 'binary_sha256': before, 'startup': startup, 'trials': trials,
                  'same_payload_pid': ready['pid'], 'passed': True}
        return result
    finally:
        stopping = subprocess.run([str(binary), 'session', 'stop', ident, '--json'], env=env, capture_output=True, timeout=20)
        subprocess.run(['tmux', '-L', socket, 'kill-server'], env=env, capture_output=True, timeout=5)
        cleanup = {'stop_status': stopping.returncode, 'stdout': stopping.stdout.decode(errors='replace')[-2000:], 'stderr': stopping.stderr.decode(errors='replace')[-2000:]}
        if delivered.exists():
            pid = json.loads(delivered.read_text().splitlines()[0])['pid']
            deadline = time.monotonic() + 3
            def payload_live():
                try: return str(payload).encode() in Path(f'/proc/{pid}/cmdline').read_bytes()
                except FileNotFoundError: return False
            while payload_live() and time.monotonic() < deadline: time.sleep(.02)
            cleanup['payload_exited'] = not payload_live()
        else: cleanup['payload_exited'] = True
        if result is not None:
            result['cleanup'] = cleanup
            result['passed'] = stopping.returncode == 0 and cleanup['payload_exited']
            (directory / 'receipt.json').write_text(json.dumps(result, indent=2)+'\n')
        (directory / 'cleanup.json').write_text(json.dumps(cleanup, indent=2)+'\n')
        if (stopping.returncode or not cleanup['payload_exited']) and sys.exc_info()[0] is None:
            (directory / 'receipt.json').write_text(json.dumps({'passed': False, 'cleanup': cleanup}, indent=2)+'\n')
            raise RuntimeError('private manager cleanup failed; retained cleanup.json')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--baseline', required=True, type=Path)
    parser.add_argument('--candidate', required=True, type=Path)
    parser.add_argument('--evidence', required=True, type=Path)
    args = parser.parse_args()
    if sys.platform != 'linux':
        parser.error('requires a disposable Linux qualification worker')
    root = args.evidence.resolve()
    root.mkdir(mode=0o700)
    results = []
    for binary, version in [(args.baseline, '1.16.34'), (args.candidate, '1.16.39')]:
        result = qualify(binary.resolve(strict=True), version, root / version)
        results.append(result)
        print(json.dumps({'version': version, 'passed': result['passed'], 'startup_seconds': result['startup']['duration']}), flush=True)
    (root / 'receipt.json').write_text(json.dumps({'results': results, 'qualifier_sha256': digest(Path(__file__)), 'scope': 'Real native PTY startup and input delivery, not conversation or reboot acceptance.'}, indent=2) + '\n')


if __name__ == '__main__':
    main()
