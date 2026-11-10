#!/usr/bin/env python3
"""Qualify real SSH/mosh recovery using only the owned disposable VM fixture.

Starts a private controller; never restarts the user's WebMux. Reboots only the
identified QEMU guest. Retains receipts, controller logs and private config.
"""

import argparse
import json
import os
from pathlib import Path
import shlex
import shutil
import socket
import subprocess
import time
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--vm-root', type=Path, required=True)
    parser.add_argument('--controller-binary', type=Path, required=True)
    parser.add_argument('--transport', choices=['ssh', 'mosh'], required=True)
    args = parser.parse_args()
    root = args.vm_root.resolve()
    launch = json.loads((root / 'launch.json').read_text())
    ssh_args = json.loads((root / 'ssh-argv.json').read_text())
    assert launch['ssh_port'] == 32222 and ssh_args[-1] == 'webmuxqa@127.0.0.1'
    process = subprocess.run(['ps', '-p', str(launch['pid']), '-o', 'command='], capture_output=True, text=True, check=True)
    assert 'qemu-system-aarch64' in process.stdout and str(root / 'disk.qcow2') in process.stdout

    def ssh(command, check=True, timeout=12):
        result = subprocess.run(ssh_args[:-1] + ['-o', 'ConnectTimeout=2', ssh_args[-1], command], capture_output=True, text=True, timeout=timeout)
        if check and result.returncode:
            raise RuntimeError('guest command failed: ' + result.stderr[-500:])
        return result

    assert ssh('hostname').stdout.strip() == 'webmux-recovery-qa'
    assert ssh('test -f /var/tmp/webmux-qualification-ready').returncode == 0
    repo = Path(__file__).resolve().parents[1]
    fixture = root / (args.transport + '-controller-' + str(uuid.uuid4()))
    fixture.mkdir(mode=0o700)
    home = fixture / 'home'
    shutil.copytree(repo / 'webmux/config.defaults', home / 'config')
    (home / 'config/auth.yaml').write_text('auth:\n  mode: none\n  users: []\n')
    wrapper = fixture / 'bin'
    wrapper.mkdir(mode=0o700)
    # Scoped executable wrappers preserve the fixture's own trust store and
    # account. They do not edit the user's ssh config or known_hosts.
    options = ['-F', '/dev/null', '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes', '-o', 'UserKnownHostsFile=' + str(root / 'known_hosts'), '-o', 'GlobalKnownHostsFile=/dev/null', '-o', 'IdentitiesOnly=yes', '-o', 'IdentityAgent=none']
    (wrapper / 'ssh').write_text('#!/usr/bin/env python3\nimport os,sys\nos.execv("/usr/bin/ssh", ' + repr(['/usr/bin/ssh'] + options) + '+sys.argv[1:])\n')
    (wrapper / 'ssh').chmod(0o700)
    mosh = shutil.which('mosh')
    if args.transport == 'mosh':
        assert mosh, 'native mosh client required'
        ports = json.loads((root / 'mosh-extra-forwarded-ports.json').read_text())
        assert ports == list(range(60001, 60011)), 'owned QEMU fixture must forward the mosh reconnect port range'
        (wrapper / 'mosh').write_text('#!/bin/sh\nexec ' + shlex.quote(mosh) + ' --port=60000:60010 "$@"\n')
        (wrapper / 'mosh').chmod(0o700)
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        port = listener.getsockname()[1]
    base = 'http://127.0.0.1:' + str(port)
    environment = dict(os.environ, WEBMUX_HOME=str(home), WEBMUX_ROOT='', WEBMUX_COORDINATION_DISABLED='1', PATH=str(wrapper) + os.pathsep + os.environ['PATH'], SHELL='/bin/bash')
    controller = None
    log = (fixture / 'controller.log').open('ab')
    session_id = None
    stages = []
    receipt = fixture / 'receipt.json'

    def start():
        nonlocal controller
        controller = subprocess.Popen([str(args.controller_binary.resolve()), '-listen', '127.0.0.1:' + str(port)], env=environment, stdin=subprocess.DEVNULL, stdout=log, stderr=log)

    def stop(kill=False):
        if controller and controller.poll() is None:
            controller.kill() if kill else controller.terminate()
            try:
                controller.wait(timeout=10)
            except subprocess.TimeoutExpired:
                controller.kill()
                controller.wait(timeout=5)

    def api(path, data=None, method=None):
        encoded = json.dumps(data).encode() if data is not None else None
        request = urllib.request.Request(base + path, data=encoded, method=method, headers={'Content-Type': 'application/json'})
        with urllib.request.urlopen(request, timeout=8) as response:
            payload = response.read()
            return json.loads(payload) if payload else None

    def wait_for(predicate, label, seconds=75):
        deadline = time.monotonic() + seconds
        last = None
        while time.monotonic() < deadline:
            try:
                value = predicate()
                if value:
                    return value
            except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
                last = str(error)
            time.sleep(.25)
        raise RuntimeError('timed out waiting for ' + label + ': ' + str(last))

    def write_receipt(passed=False):
        receipt.write_text(json.dumps({'transport': args.transport, 'passed': passed, 'session_id': session_id, 'stages': stages}, indent=2) + '\n')
        receipt.chmod(0o600)

    try:
        start()
        wait_for(lambda: api('/api/health')['status'] == 'ok', 'private controller')
        api('/api/config', {'app': {'session_logging': {'enabled': True}, 'transport': {'ssh_fallback': False}}}, 'PUT')
        key = api('/api/keys', {'private_key_path': str(root / 'id_ed25519'), 'description': 'owned recovery VM qualification'})
        session = api('/api/sessions', {'hostname': '127.0.0.1', 'port': 32222, 'username': 'webmuxqa', 'key_id': key['id'], 'transport': args.transport, 'cols': 100, 'rows': 30})
        session_id = str(uuid.UUID(session['id']))
        target = 'webmux-' + session_id
        recovery_path = '/api/session-recovery/' + session_id
        count_path = '/home/webmuxqa/qualification/executions-' + session_id
        marker = 'REMOTE_RECOVERY_' + session_id

        def view():
            return api(recovery_path)

        def executions():
            value = ssh('test ! -f ' + shlex.quote(count_path) + ' || wc -l < ' + shlex.quote(count_path)).stdout.strip()
            return int(value or '0')

        def record(stage):
            recovery = view()
            current = api('/api/sessions/' + session_id)
            assert current['transport'] == args.transport, 'transport fallback cannot qualify the requested transport'
            clients = []
            if args.transport == 'mosh':
                def native_clients():
                    tree = subprocess.run(['ps', '-axo', 'pid=,ppid=,comm='], capture_output=True, text=True, check=True)
                    processes = [parts for line in tree.stdout.splitlines() if len(parts := line.strip().split(None, 2)) == 3]
                    descendants = {controller.pid}
                    for _ in range(16):
                        children = {int(pid) for pid, parent, _ in processes if int(parent) in descendants}
                        if children.issubset(descendants):
                            break
                        descendants.update(children)
                    return [int(pid) for pid, _, name in processes if int(pid) in descendants and Path(name).name == 'mosh-client']
                clients = wait_for(native_clients, 'native mosh-client below private controller', seconds=15)
            provider, pane = ssh("tmux display-message -p -t " + target + " '#{pid}:#{pane_pid}'").stdout.strip().split(':')
            item = {'stage': stage, 'transport': current['transport'], 'controller_pid': controller.pid, 'boot_id': ssh('cat /proc/sys/kernel/random/boot_id').stdout.strip(), 'provider_pid': int(provider), 'pane_pid': int(pane), 'executions': executions(), 'waiting': recovery['waiting'], 'transcripts': recovery.get('state', {}).get('transcripts', [])}
            if clients:
                item['native_mosh_client_pids'] = clients
            stages.append(item)
            write_receipt()
            print(json.dumps(item), flush=True)
            return item

        wait_for(lambda: view()['shell_ready'], 'remote shell readiness')
        code = "require('node:fs').appendFileSync(" + json.dumps(count_path) + ", 'execution\\n');console.log(" + json.dumps(marker) + ");setInterval(()=>console.log('remote qa tick'),1000)"
        command = '/usr/bin/node -e ' + shlex.quote(code)
        bridge = """import fs from 'node:fs';import WebSocket from 'ws';
const input=JSON.parse(fs.readFileSync(0,'utf8'));const ws=new WebSocket(input.url);
const timer=setTimeout(()=>{ws.terminate();process.exitCode=1},5000);
ws.on('error',()=>{clearTimeout(timer);process.exitCode=1});
ws.on('open',()=>{ws.send(JSON.stringify({type:'input',data:input.command+'\\r'}));setTimeout(()=>{clearTimeout(timer);ws.close()},500)});
"""
        subprocess.run(['node', '--input-type=module', '-e', bridge], cwd=repo / 'webmux', input=json.dumps({'url': base.replace('http:', 'ws:') + '/api/term/' + session_id, 'command': command}), text=True, check=True, timeout=10)
        wait_for(lambda: executions() == 1, 'remote job')
        def observed():
            recovery = view()
            return marker in recovery['history'] and marker in recovery.get('state', {}).get('activity', {}).get('command', '')
        wait_for(observed, 'remote evidence and transcript')
        original = record('running')
        for stage, kill in [('controller-restart', False), ('controller-SIGKILL', True)]:
            stop(kill)
            start()
            wait_for(lambda: api('/api/health')['status'] == 'ok', stage)
            wait_for(lambda: view().get('state', {}).get('disposition') == 'reattached', stage + ' reattachment')
            observed = record(stage)
            assert observed['provider_pid'] == original['provider_pid'] and observed['pane_pid'] == original['pane_pid'] and observed['executions'] == 1
        prior_boot = original['boot_id']
        assert ssh('sudo -n systemctl reboot', check=False).returncode in (0, 255)
        wait_for(lambda: ssh('cat /proc/sys/kernel/random/boot_id').stdout.strip() != prior_boot, 'guest reboot', seconds=120)
        # Transport loss remains explicit until reconnect, with retained logs.
        api('/api/sessions/' + session_id + '/reconnect', {})
        wait_for(lambda: view()['waiting'], 'remote recovery waiting')
        booted = record('remote-host-reboot-waiting')
        assert booted['boot_id'] != prior_boot and booted['executions'] == 1
        recovery = view()
        assert marker in recovery['history'] and recovery['can_replay']
        action = {'id': str(uuid.uuid4()), 'kind': 'replay', 'revision': recovery['state']['revision'], 'host_revision': recovery['host_revision'], 'instance': recovery['instance'], 'confirm': True}
        assert api(recovery_path, action)['outcome'] == 'consumed'
        wait_for(lambda: executions() == 2, 'approved remote replay')
        assert api(recovery_path, action)['outcome'] == 'consumed'
        for _ in range(3):
            api('/api/sessions/' + session_id + '/reconnect', {})
        wait_for(lambda: api('/api/sessions/' + session_id)['state'] == 'connected', 'remote reconnect')
        final = record('approved-once-and-reconnected')
        assert final['executions'] == 2 and set(original['transcripts']).issubset(set(final['transcripts']))
        write_receipt(True)
        print(json.dumps({'passed': True, 'evidence': str(receipt)}), flush=True)
    finally:
        try:
            if session_id:
                try:
                    api('/api/sessions/' + session_id, method='DELETE')
                finally:
                    ssh('tmux kill-session -t webmux-' + session_id, check=False)
        finally:
            stop()
            log.close()


if __name__ == '__main__':
    main()
