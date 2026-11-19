#!/usr/bin/env python3
"""Exercise local Rig Stop across real SSH to an isolated Linux tmux owner.

Creates only an explicitly labelled disposable container and fixture SSH keys.
No operator credentials, source mounts, global SSH configuration or host daemon
changes are used. Retains evidence and removes only its own container/image.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import time
import uuid

BASE_IMAGE = 'ubuntu@sha256:fe81560990fae86dc4743202b2cb4d3df98d0bcf949173cecef15ce1127ef852'


def run_logged(command, log, *, check=True, timeout=30):
    log.write(json.dumps({'command': command, 'event': 'start', 'timeoutSeconds': timeout}) + '\n')
    log.flush()
    try:
        result = subprocess.run(command, capture_output=True, text=True, timeout=timeout)
    except subprocess.TimeoutExpired as error:
        # TimeoutExpired retains bytes even with text=True, possibly ending in
        # an incomplete UTF-8 sequence. Keep that evidence before propagating
        # the original failure to the qualifier's existing cleanup/report path.
        def text(value):
            return value.decode('utf-8', errors='replace') if isinstance(value, bytes) else value or ''
        log.write(json.dumps({'command': command, 'timedOut': True, 'timeoutSeconds': error.timeout})
                  + '\n' + text(error.stdout) + text(error.stderr) + '\n')
        log.flush()
        raise
    log.write(json.dumps({'command': command, 'exit': result.returncode}) + '\n' + result.stdout + result.stderr + '\n')
    log.flush()
    if check and result.returncode:
        raise RuntimeError(f'{Path(command[0]).name} failed ({result.returncode}); inspect private fixture.log')
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--cli', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--chromium', type=Path)
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[1]
    output = args.output.resolve()
    output.mkdir(parents=True, mode=0o700, exist_ok=False)
    cli = args.cli.resolve(strict=True)
    nonce = uuid.uuid4().hex
    label = 'webmux.rig-stop-relay'
    image = 'webmux-rig-stop-relay:' + nonce
    container = None
    client_key_hash = None
    report = {'scope': 'Actual native Rig/WebMux/browser/local SSH relay and an isolated Linux SSH/tmux endpoint; synthetic persistent sleep provider, no authenticated coding recovery or physical-host failure qualification.',
              'checks': {}, 'passed': False}
    log = (output / 'fixture.log').open('w')

    def run(command, *, check=True, timeout=30):
        return run_logged(command, log, check=check, timeout=timeout)

    def owned(kind, identity):
        result = run(['docker', kind, 'inspect', identity, '--format', '{{json .Config.Labels}}'])
        if json.loads(result.stdout).get(label) != nonce:
            raise RuntimeError('Fixture ownership label changed; refusing cleanup/control')

    try:
        run(['/usr/bin/ssh-keygen', '-q', '-t', 'ed25519', '-N', '', '-f', str(output / 'client')])
        client_key_hash = hashlib.sha256((output / 'client').read_bytes()).digest()
        (output / '.dockerignore').write_text('*\n!Dockerfile\n!client.pub\n!sshd_config\n')
        (output / 'sshd_config').write_text('''ListenAddress 0.0.0.0
Port 22
HostKey /etc/ssh/ssh_host_ed25519_key
AuthorizedKeysFile /etc/ssh/fixture_authorized_keys
PasswordAuthentication no
KbdInteractiveAuthentication no
UsePAM no
PermitRootLogin no
AllowUsers worker
StrictModes yes
AllowAgentForwarding no
AllowTcpForwarding no
X11Forwarding no
PermitTunnel no
PermitUserRC no
PrintMotd no
LogLevel VERBOSE
''')
        (output / 'Dockerfile').write_text('FROM ' + BASE_IMAGE + '''
RUN apt-get -o Acquire::Retries=2 -o Acquire::http::Timeout=30 update && \
    DEBIAN_FRONTEND=noninteractive apt-get -o Acquire::Retries=2 -o Acquire::http::Timeout=30 install -y --no-install-recommends openssh-server tmux && \
    useradd --create-home --shell /bin/sh --password x worker && \
    mkdir -p /run/sshd && rm -f /etc/ssh/ssh_host_*
COPY client.pub /etc/ssh/fixture_authorized_keys
COPY sshd_config /etc/ssh/sshd_config.fixture
RUN chmod 644 /etc/ssh/fixture_authorized_keys
CMD ["/bin/sh", "-c", "ssh-keygen -q -t ed25519 -N '' -f /etc/ssh/ssh_host_ed25519_key && exec /usr/sbin/sshd -D -e -f /etc/ssh/sshd_config.fixture"]
''')
        print('Building the isolated Linux SSH/tmux fixture.', flush=True)
        run(['docker', 'build', '--label', label + '=' + nonce, '--tag', image, str(output)], timeout=180)
        owned('image', image)
        report['baseImage'] = BASE_IMAGE
        report['image'] = json.loads(run(['docker', 'image', 'inspect', image]).stdout)[0]['Id']
        container = run(['docker', 'run', '--detach', '--rm', '--memory', '256m', '--cpus', '1',
                         '--label', label + '=' + nonce, '--publish', '127.0.0.1::22', image]).stdout.strip()
        if not re.fullmatch(r'[a-f0-9]{64}', container):
            raise RuntimeError('Invalid fixture container ID')
        owned('container', container)
        report['container'] = container
        details = json.loads(run(['docker', 'container', 'inspect', container]).stdout)[0]
        binding = details['NetworkSettings']['Ports']['22/tcp']
        assert len(binding) == 1 and binding[0]['HostIp'] == '127.0.0.1'
        port = int(binding[0]['HostPort'])
        deadline = time.monotonic() + 20
        while True:
            key = run(['docker', 'exec', container, 'cat', '/etc/ssh/ssh_host_ed25519_key.pub'], check=False)
            if key.returncode == 0 and key.stdout.startswith('ssh-ed25519 '):
                break
            if time.monotonic() >= deadline:
                raise RuntimeError('Private SSH host key did not become available')
            time.sleep(.2)
        (output / 'known_hosts').write_text(f'[127.0.0.1]:{port} ' + key.stdout)
        ssh = ['/usr/bin/ssh', '-F', '/dev/null', '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes',
               '-o', 'UserKnownHostsFile=' + str(output / 'known_hosts'), '-o', 'GlobalKnownHostsFile=/dev/null',
               '-o', 'IdentityAgent=none', '-o', 'IdentitiesOnly=yes', '-o', 'ConnectTimeout=5',
               '-i', str(output / 'client'), '-p', str(port), 'worker@127.0.0.1']
        while True:
            ready = run([*ssh, 'printf SSH_FIXTURE_READY'], check=False, timeout=7)
            if ready.returncode == 0 and ready.stdout == 'SSH_FIXTURE_READY':
                break
            if time.monotonic() >= deadline:
                raise RuntimeError('Private SSH endpoint did not become ready')
            time.sleep(.2)
        socket = '/tmp/webmux-remote-owner.sock'
        marker = 'WM_REMOTE_RELAY_' + nonce
        run([*ssh, shlex.join(['tmux', '-S', socket, 'new-session', '-d', '-s', 'provider', '/bin/sh', '-c', f'printf "{marker}\\n"; exec /bin/sleep 600'])])
        probe = shlex.join(['tmux', '-S', socket, 'list-panes', '-t', '=provider', '-F', '#{pane_pid}'])
        pid = run([*ssh, probe]).stdout.strip()
        assert re.fullmatch(r'[1-9][0-9]*', pid)
        run([*ssh, 'kill -0 ' + pid])
        executable = run([*ssh, 'readlink /proc/' + pid + '/exe']).stdout.strip()
        assert executable == '/usr/bin/sleep'
        report['remoteVersions'] = run([*ssh, 'uname -sm; tmux -V']).stdout.strip()
        report['remotePackageVersions'] = run([*ssh, 'dpkg-query -W openssh-server tmux']).stdout.strip()
        manifest = {'ssh': ssh, 'attach': shlex.join(['tmux', '-S', socket, 'attach-session', '-t', '=provider']),
                    'probe': probe, 'pid': pid, 'executable': executable, 'marker': marker}
        (output / 'relay.json').write_text(json.dumps(manifest))
        environment = dict(os.environ, WEBMUX_REAL_OPENRIG='1', WEBMUX_OPENRIG_CLI=str(cli),
                           WEBMUX_RIG_RELAY_MANIFEST=str(output / 'relay.json'))
        if args.chromium:
            environment['PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH'] = str(args.chromium.resolve(strict=True))
        print('Running native WebMux/browser local Stop across the SSH relay.', flush=True)
        with (output / 'browser.log').open('w') as browser_log:
            result = subprocess.run(['npm', '--prefix', 'webmux', 'run', 'test:e2e:go', '--', 'openrig-stop-relay.spec.ts',
                                     '--output=' + str(output / 'browser')], cwd=repo, env=environment,
                                    stdout=browser_log, stderr=subprocess.STDOUT, timeout=120)
        assert result.returncode == 0, 'Native browser case failed; inspect browser.log'
        receipts = list((output / 'browser').rglob('checked-stop-relay.json'))
        assert len(receipts) == 1, 'Missing native browser proof (a skipped test is not acceptance)'
        proof = json.loads(receipts[0].read_text())
        assert proof['passed'] and all(proof['checks'].values())
        report['checks'].update(proof['checks'])
        report['browserProofSHA256'] = hashlib.sha256(receipts[0].read_bytes()).hexdigest()
        report['localRuntime'] = {key: proof[key] for key in ['rig', 'node', 'platform', 'architecture', 'browser']}
        report['nativeBinarySHA256'] = hashlib.sha256((repo / 'tests/e2e/.test-home/webmux').read_bytes()).hexdigest()
        report['sourceSHA256'] = {str(name): hashlib.sha256((repo / name).read_bytes()).hexdigest() for name in [
            Path('scripts/qualify-rig-stop-relay.py'), Path('tests/e2e/openrig-stop-relay.spec.ts'),
            Path('webmux/server/internal/openrigcoord/rig-stop.mts'), Path('webmux/server/internal/httpapi/rig_stop.go')]}
        assert run([*ssh, probe]).stdout.strip() == pid
        run([*ssh, 'kill -0 ' + pid])
        assert run([*ssh, 'readlink /proc/' + pid + '/exe']).stdout.strip() == executable
        report['checks']['remotePersistentPIDSurvivesAfterBrowserTest'] = True
        report['passed'] = True
    except Exception as error:
        report['error'] = str(error)
    finally:
        try:
            if container:
                owned('container', container)
                (output / 'sshd.log').write_text(run(['docker', 'logs', container], check=False).stderr)
                run(['docker', 'rm', '--force', container])
                report['checks']['ownedContainerRemoved'] = run(['docker', 'container', 'inspect', container], check=False).returncode != 0
        except Exception as error:
            report['passed'] = False
            report['cleanupError'] = str(error)
        try:
            if run(['docker', 'image', 'inspect', image], check=False).returncode == 0:
                owned('image', image)
                run(['docker', 'image', 'rm', image])
                report['checks']['ownedImageRemoved'] = run(['docker', 'image', 'inspect', image], check=False).returncode != 0
        except Exception as error:
            report['passed'] = False
            report['imageCleanupError'] = str(error)
        try:
            key = output / 'client'
            if client_key_hash is not None:
                assert key.is_file() and not key.is_symlink()
                assert hashlib.sha256(key.read_bytes()).digest() == client_key_hash
                key.unlink()
                report['checks']['generatedClientKeyRemoved'] = not key.exists()
        except Exception as error:
            report['passed'] = False
            report['keyCleanupError'] = str(error)
        report['passed'] = bool(report['passed'] and all(report['checks'].values())
                                and report['checks'].get('ownedContainerRemoved')
                                and report['checks'].get('ownedImageRemoved')
                                and report['checks'].get('generatedClientKeyRemoved'))
        report['qualifierSHA256'] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
        (output / 'results.json').write_text(json.dumps(report, indent=2) + '\n')
        log.close()
    print(json.dumps({'passed': report['passed'], 'checks': report['checks'], 'evidence': str(output)}), flush=True)
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
