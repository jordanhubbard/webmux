#!/usr/bin/env python3
"""Exercise local Rig Stop across real SSH to an isolated Linux tmux owner.

Creates only an explicitly labelled disposable container and fixture SSH keys.
No operator credentials, source mounts, global SSH configuration or host daemon
changes are used. Retains evidence and removes only its own container/image.
"""

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shlex
import ssl
import subprocess
import time
import uuid

BASE_IMAGE = 'ubuntu@sha256:fe81560990fae86dc4743202b2cb4d3df98d0bcf949173cecef15ce1127ef852'
LOG_STREAM_LIMIT = 1024 * 1024
SYSTEM_CA_BUNDLE = Path('/etc/ssl/certs/ca-certificates.crt')


def write_build_inputs(output):
    # The pinned minimal image has the Ubuntu archive keyring, but no CA
    # bundle. Export public trust anchors only; no private keys or native
    # account state enter the build context.
    if SYSTEM_CA_BUNDLE.is_file():
        if SYSTEM_CA_BUNDLE.stat().st_size > 4 * 1024 * 1024:
            raise RuntimeError('Public host CA bundle oversized')
        context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        context.load_verify_locations(cafile=str(SYSTEM_CA_BUNDLE))
        trust_source = 'system-public-pem-bundle'
    else:
        context = ssl.create_default_context()
        trust_source = 'python-default-public-store'
    certificates = context.get_ca_certs(binary_form=True)
    if not certificates:
        raise RuntimeError('Public host CA trust roots unavailable')
    if len(certificates) > 4096:
        raise RuntimeError('Public host CA trust roots oversized')
    bundle = ''.join(ssl.DER_cert_to_PEM_cert(cert) for cert in sorted(set(certificates))).encode('ascii')
    if len(bundle) > 4 * 1024 * 1024:
        raise RuntimeError('Public host CA bundle oversized')
    helper = Path(__file__).with_name('configure-ci-apt.py')
    spec = importlib.util.spec_from_file_location('webmux_ci_apt', helper)
    apt = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(apt)
    sources = apt.deb822_sources('''Types: deb
URIs: https://archive.ubuntu.com/ubuntu/
Suites: noble noble-updates noble-backports
Components: main restricted universe multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

Types: deb
URIs: https://security.ubuntu.com/ubuntu/
Suites: noble-security
Components: main restricted universe multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
''')
    settings = (apt.network_settings(Path('/etc/apt/webmux-ci-sources'))
                + 'Acquire::https::CAInfo "/etc/ssl/certs/ca-certificates.crt";\n')
    (output / 'ca-certificates.crt').write_bytes(bundle)
    (output / 'ubuntu.sources').write_text(sources)
    (output / 'apt-ci.conf').write_text(settings)
    (output / '.dockerignore').write_text('*\n!Dockerfile\n!client.pub\n!sshd_config\n!ca-certificates.crt\n!ubuntu.sources\n!apt-ci.conf\n')
    dockerfile = (
        'FROM ' + BASE_IMAGE + '\n'
        'COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt\n'
        'COPY ubuntu.sources /etc/apt/webmux-ci-sources/ubuntu.sources\n'
        'COPY apt-ci.conf /etc/apt/apt.conf.d/99-webmux-ci-network\n'
        'RUN . /etc/os-release && test "$ID" = ubuntu && test "$VERSION_CODENAME" = noble && '
        'apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends openssh-server tmux && '
        'useradd --create-home --shell /bin/sh --password x worker && '
        'mkdir -p /run/sshd && rm -f /etc/ssh/ssh_host_*\n'
        'COPY client.pub /etc/ssh/fixture_authorized_keys\n'
        'COPY sshd_config /etc/ssh/sshd_config.fixture\n'
        'RUN chmod 644 /etc/ssh/fixture_authorized_keys\n'
        """CMD ["/bin/sh", "-c", "ssh-keygen -q -t ed25519 -N '' -f /etc/ssh/ssh_host_ed25519_key && exec /usr/sbin/sshd -D -e -f /etc/ssh/sshd_config.fixture"]
""")
    (output / 'Dockerfile').write_text(dockerfile)
    return {'baseImage': BASE_IMAGE, 'publicHostCAUniqueCertificates': len(set(certificates)),
            'publicHostCATrustSource': trust_source,
            'publicHostCABundleSHA256': hashlib.sha256(bundle).hexdigest(),
            'ubuntuSourcesSHA256': hashlib.sha256(sources.encode()).hexdigest(),
            'aptSettingsSHA256': hashlib.sha256(settings.encode()).hexdigest(),
            'aptHelperSHA256': hashlib.sha256(helper.read_bytes()).hexdigest()}


def _log_stream(value):
    data = value.encode('utf-8') if isinstance(value, str) else (value or b'')
    observed = len(data)
    omitted = max(0, observed - LOG_STREAM_LIMIT)
    if omitted:
        half = LOG_STREAM_LIMIT // 2
        data = data[:half] + f'\n[omitted {omitted} bytes]\n'.encode() + data[-half:]
    return data.decode('utf-8', errors='replace'), observed, omitted


def run_recorded(command, log, *, check=True, timeout=30):
    """Keep bounded output even when subprocess.run raises on its deadline.

    Capture and command return semantics remain subprocess.run's. In particular,
    a Docker client timeout does not establish that its daemon finished building.
    """
    identity = uuid.uuid4().hex
    started = time.monotonic()

    def record(event, **fields):
        log.write(json.dumps({'event': event, 'commandId': identity,
                              'elapsedSeconds': time.monotonic() - started, **fields}) + '\n')
        log.flush()

    def streams(stdout, stderr):
        result = {}
        for name, value in [('stdout', stdout), ('stderr', stderr)]:
            text, observed, omitted = _log_stream(value)
            result.update({name: text, name + 'Bytes': observed, name + 'OmittedBytes': omitted})
        return result

    record('command_start', command=command, timeoutSeconds=timeout)
    try:
        result = subprocess.run(command, capture_output=True, text=True, timeout=timeout)
    except subprocess.TimeoutExpired as error:
        record('command_timeout', timeoutSeconds=timeout, **streams(error.stdout, error.stderr))
        raise
    except OSError as error:
        record('command_start_error', errorType=type(error).__name__)
        raise
    record('command_exit', exit=result.returncode, **streams(result.stdout, result.stderr))
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
              'checks': {}, 'passed': False, 'stage': 'preparation', 'nativeBrowserAttempted': False}
    log = (output / 'fixture.log').open('w')

    def run(command, *, check=True, timeout=30):
        return run_recorded(command, log, check=check, timeout=timeout)

    def owned(kind, identity):
        result = run(['docker', kind, 'inspect', identity, '--format', '{{json .Config.Labels}}'])
        if json.loads(result.stdout).get(label) != nonce:
            raise RuntimeError('Fixture ownership label changed; refusing cleanup/control')

    try:
        run(['/usr/bin/ssh-keygen', '-q', '-t', 'ed25519', '-N', '', '-f', str(output / 'client')])
        client_key_hash = hashlib.sha256((output / 'client').read_bytes()).digest()
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
        report['buildInputs'] = write_build_inputs(output)
        print('Building the isolated Linux SSH/tmux fixture.', flush=True)
        report['stage'] = 'image-build'
        run(['docker', 'build', '--label', label + '=' + nonce, '--tag', image, str(output)], timeout=180)
        owned('image', image)
        report['stage'] = 'container-start'
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
        report['stage'] = 'ssh-readiness'
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
        report['stage'] = 'remote-owner'
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
        report['stage'] = 'browser'
        report['nativeBrowserAttempted'] = True
        with (output / 'browser.log').open('w') as browser_log:
            result = subprocess.run(['npm', '--prefix', 'webmux', 'run', 'test:e2e:go', '--', 'openrig-stop-relay.spec.ts',
                                     '--output=' + str(output / 'browser')], cwd=repo, env=environment,
                                    stdout=browser_log, stderr=subprocess.STDOUT, timeout=120)
        assert result.returncode == 0, 'Native browser case failed; inspect browser.log'
        report['stage'] = 'verification'
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
        report['stage'] = 'complete'
    except Exception as error:
        report['error'] = str(error)
        report['errorType'] = type(error).__name__
        if report['stage'] == 'image-build' and isinstance(error, subprocess.TimeoutExpired):
            report['daemonBuildCompletion'] = 'unconfirmed after Docker client timeout'
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
