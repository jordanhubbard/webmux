#!/usr/bin/env python3
"""Qualify Rig-owned native Claude recovery across an actual guest reboot.

Drives scripts/qualify-recovery-rig.py inside the retained disposable QEMU
guest in two stages: --reboot-prepare leaves the native process, private Rig
manager and controller running; the guest is then actually rebooted; and
--resume-after-reboot performs WebMux's approved Rig action and requires a
fresh remembered-marker answer. Uses the guest's own Claude login; no
credentials are copied. Only the validated disposable guest is rebooted.

The exact qualified Rig build (0.6.14, 9d0200f3) and a Linux arm64 Node
22.23.3 runtime are installed into the guest from local files when absent.
"""

import argparse
import hashlib
import io
import json
from pathlib import Path
import shlex
import socket
import subprocess
import tarfile
import time
import uuid

RIG_VERSION = '0.6.14 (9d0200f3)'
NODE_VERSION = 'v22.23.3'
GUEST_HOME = '/home/webmuxqa'
RIG_DIR = GUEST_HOME + '/qualification/openrig-9d0200f3'
NODE = GUEST_HOME + '/qualification/node-22.23.3/bin/node'
RIG_CLI = RIG_DIR + '/node_modules/@openrig/cli/dist/bin-wrapper.js'
GUEST_PATH = GUEST_HOME + '/.local/bin:/usr/local/bin:/usr/bin:/bin'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--vm-root', type=Path, required=True)
    parser.add_argument('--binary', type=Path, required=True, help='Linux arm64 standalone WebMux build')
    parser.add_argument('--rig-release', type=Path, help='tar.gz of the qualified Rig release directory (node_modules, package.json)')
    parser.add_argument('--node-binary', type=Path, help='Linux arm64 Node 22.23.3 executable')
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[1]
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

    record_path = root / ('rig-reboot-' + uuid.uuid4().hex + '.json')
    binary = args.binary.resolve(strict=True)
    record = {'passed': False, 'scope': 'Rig-owned native Claude continuation across an actual guest reboot through the WebMux action',
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'orchestrator_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'rig_harness_sha256': hashlib.sha256((repo / 'scripts/qualify-recovery-rig.py').read_bytes()).hexdigest(), 'stages': {}}

    def save():
        record_path.write_text(json.dumps(record, indent=2) + '\n')
        record_path.chmod(0o600)

    def ssh(command, data=None, check=True, timeout=30):
        result = subprocess.run(ssh_args[:-1] + ['-o', 'ConnectTimeout=3', ssh_args[-1], command], input=data, capture_output=True, timeout=timeout)
        if check and result.returncode:
            raise RuntimeError('guest command failed: ' + result.stderr.decode(errors='replace')[-1500:])
        return result

    def text(command, **kw):
        return ssh(command, **kw).stdout.decode().strip()

    def boot_id():
        return str(uuid.UUID(text('cat /proc/sys/kernel/random/boot_id')))

    def wait(fn, label, seconds):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            try:
                if fn():
                    return
            except (OSError, RuntimeError, subprocess.SubprocessError):
                pass
            time.sleep(1)
        raise RuntimeError('timed out waiting for ' + label)

    def last_json(output, key):
        for line in reversed(output.splitlines()):
            try:
                value = json.loads(line)
            except ValueError:
                continue
            if isinstance(value, dict) and key in value:
                return value
        return None

    try:
        assert text('hostname') == 'webmux-recovery-qa', 'Refusing a host that is not the disposable guest'
        assert ssh('test -f /var/tmp/webmux-qualification-ready', check=False).returncode == 0
        assert text('tmux -V') == 'tmux 3.4'
        claude = GUEST_HOME + '/.local/bin/claude'
        assert json.loads(text(claude + ' auth status --json'))['loggedIn'], 'Authenticate the guest Claude first'
        record['native_cli'] = text(claude + ' --version')

        # Provision the exact qualified Rig build and Node runtime when absent.
        if ssh(shlex.quote(NODE) + ' --version', check=False).stdout.decode().strip() != NODE_VERSION:
            assert args.node_binary, '--node-binary is required to provision the guest Node runtime'
            ssh('mkdir -p ' + shlex.quote(str(Path(NODE).parent)) + ' && cat > ' + shlex.quote(NODE) + ' && chmod 700 ' + shlex.quote(NODE), args.node_binary.read_bytes(), timeout=120)
        if ssh(shlex.join([NODE, RIG_CLI, '--version']), check=False).stdout.decode().strip() != RIG_VERSION:
            assert args.rig_release, '--rig-release is required to provision the qualified Rig build'
            ssh('rm -rf ' + shlex.quote(RIG_DIR) + ' && mkdir -p ' + shlex.quote(RIG_DIR) + ' && tar -C ' + shlex.quote(RIG_DIR) + ' -xzf -', args.rig_release.read_bytes(), timeout=300)
        assert text(shlex.quote(NODE) + ' --version') == NODE_VERSION
        assert text(shlex.join([NODE, RIG_CLI, '--version'])) == RIG_VERSION
        record['guest_node_sha256'] = text('sha256sum ' + shlex.quote(NODE)).split()[0]

        # Stage only what the in-guest harness reads from its repository.
        work = GUEST_HOME + '/qualification/rig-reboot-' + uuid.uuid4().hex[:12]
        record['guest_work'] = work
        bundle = io.BytesIO()
        with tarfile.open(fileobj=bundle, mode='w:gz') as archive:
            for name in ['scripts/qualify-recovery-rig.py', 'scripts/qualify-recovery-native.py', 'webmux/config.defaults', 'webmux/node_modules/ws']:
                archive.add(repo / name, arcname='repo/' + name)
            archive.add(binary, arcname='webmux')
        ssh('mkdir -p ' + shlex.quote(work) + ' && chmod 700 ' + shlex.quote(work) + ' && tar -C ' + shlex.quote(work) + ' -xzf -', bundle.getvalue(), timeout=120)
        env = 'env PATH=' + shlex.quote(GUEST_PATH) + ' WEBMUX_BROWSER_ORIGINAL_PATH=' + shlex.quote(GUEST_PATH) + ' '

        native = ssh(env + shlex.join(['python3', work + '/repo/scripts/qualify-recovery-native.py', '--cli', 'claude', '--output', work + '/native']), check=False, timeout=400)
        record['stages']['native-manifest'] = {'returncode': native.returncode, 'tail': native.stdout.decode(errors='replace')[-1500:]}
        save()
        assert native.returncode == 0, 'synthetic native conversation failed'
        manifest = last_json(native.stdout.decode(), 'manifest')['manifest']

        harness = ['python3', work + '/repo/scripts/qualify-recovery-rig.py', '--binary', work + '/webmux', '--manifest', manifest,
                   '--rig-cli', RIG_CLI, '--rig-node', NODE, '--snapshot-plan', '--native-hooks', '--native-resume', '--webmux-action']
        prepared = ssh(env + shlex.join(harness + ['--reboot-prepare']), check=False, timeout=1200)
        output = prepared.stdout.decode(errors='replace')
        record['stages']['prepare'] = {'returncode': prepared.returncode, 'tail': output[-3000:], 'stderr': prepared.stderr.decode(errors='replace')[-3000:]}
        save()
        ready = last_json(output, 'prepared')
        assert prepared.returncode == 0 and ready, 'prepare stage failed; inspect the guest fixture'
        fixture = ready['prepared']
        record['fixture'] = fixture

        previous = boot_id()
        result = ssh('sudo -n systemctl reboot', check=False)
        assert result.returncode in (0, 255)
        wait(lambda: boot_id() != previous, 'actual guest reboot', 180)
        record['boot_ids'] = [previous, boot_id()]
        save()

        resumed = ssh(env + shlex.join(harness + ['--resume-after-reboot', fixture]), check=False, timeout=1200)
        output = resumed.stdout.decode(errors='replace')
        record['stages']['resume'] = {'returncode': resumed.returncode, 'tail': output[-3000:], 'stderr': resumed.stderr.decode(errors='replace')[-3000:]}
        save()
        final = json.loads(text('cat ' + shlex.quote(fixture + '/manifest.json')))
        record['guest_manifest'] = final
        record['passed'] = resumed.returncode == 0 and final.get('passed') is True and final.get('actual_host_reboot') is True
    finally:
        save()
        print(json.dumps({'passed': record['passed'], 'evidence': str(record_path)}), flush=True)
    return 0 if record['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
