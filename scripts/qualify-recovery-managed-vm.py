#!/usr/bin/env python3
"""Qualify real Agent Deck ownership across a disposable Linux guest reboot.

Uses the retained QEMU fixture only. The managed payload is a synthetic shell
job, not a coding CLI or a native-conversation substitute. No credentials are
copied. The fixture's original user service is restored during cleanup.
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
    base = 'http://127.0.0.1:' + str(launch['http_port'])
    control = 'env XDG_RUNTIME_DIR=/run/user/1000 DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus systemctl --user '
    unit = 'webmux-managed-qualification.service'
    original_unit = 'webmux-qualification.service'
    deck_env = 'env HOME=' + fixture + '/home XDG_CONFIG_HOME=' + fixture + '/home/.config PATH=/home/webmuxqa/qualification:/usr/bin:/bin '
    ident = sid = target = None
    original_stopped = False
    original_enabled = False
    installed_unit = False

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
        with urllib.request.urlopen(request, timeout=10) as response:
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

    try:
        assert text('hostname') == 'webmux-recovery-qa'
        assert ssh('test -f /var/tmp/webmux-qualification-ready').returncode == 0
        assert 'Agent Deck v1.16.34' in text('/home/webmuxqa/qualification/agent-deck --help')
        manifest['manager_sha256'] = text('sha256sum /home/webmuxqa/qualification/agent-deck').split()[0]
        ssh('mkdir -p ' + fixture + '/home/.config/agent-deck ' + fixture + '/project ' + fixture + '/server/config')
        put(fixture + '/webmux', binary.read_bytes())
        ssh('chmod 700 ' + fixture + '/webmux')
        put(fixture + '/home/.config/agent-deck/config.toml', '[security]\nauto_trust_folders = false\n[launch]\ninject_identity = false\n[tmux]\nsocket_name = "wm-managed-qa"\n')
        marker = 'MANAGED_REBOOT_' + uuid.uuid4().hex
        put(fixture + '/project/job.py', 'import pathlib,time,os\np=pathlib.Path(' + repr(fixture + '/executions') + ')\nwith p.open("a") as f: f.write(str(os.getpid())+"\\n"); f.flush(); os.fsync(f.fileno())\nprint(' + repr(marker) + ',flush=True)\nwhile True: print("managed tick",flush=True); time.sleep(1)\n')
        entry = deck('add', fixture + '/project', '--cmd', '/usr/bin/python3 ' + fixture + '/project/job.py', '--no-parent', '--no-identity', '--title', 'managed-reboot-qualification', '--json')
        ident = entry['id']
        assert entry['tool'] == 'shell' and entry['started'] is False
        started = deck('session', 'start', ident, '--json')
        target = started['tmux']
        wait(lambda: pane() is not None and executions() == 1, 'initial manager job')
        put(fixture + '/server/config/auth.yaml', 'auth:\n  mode: none\n  users: []\n')
        put(fixture + '/server/config/app.yaml', 'app:\n  session_logging:\n    enabled: true\n  coordination:\n    enabled_providers: [agent-deck]\n    default_launch_provider: webmux\n    default_view: all\n')
        unit_path = '/home/webmuxqa/.config/systemd/user/' + unit
        put(unit_path, '[Unit]\nDescription=WebMux private managed reboot qualification\n[Service]\nType=simple\nWorkingDirectory=' + fixture + '/home\nEnvironment=HOME=' + fixture + '/home\nEnvironment=XDG_CONFIG_HOME=' + fixture + '/home/.config\nEnvironment=WEBMUX_HOME=' + fixture + '/server\nEnvironment=WEBMUX_COORDINATION_DISABLED=0\nEnvironment=SHELL=/bin/bash\nEnvironment=PATH=/home/webmuxqa/qualification:/usr/bin:/bin\nExecStart=' + fixture + '/webmux -listen 0.0.0.0:18080\nRestart=on-failure\nRestartSec=1\nKillMode=mixed\n[Install]\nWantedBy=default.target\n')
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
        assert rebooted['recovery']['can_resume_managed'] is False and rebooted['recovery']['can_resume_cli'] is False
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
        manifest['passed'] = True
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
