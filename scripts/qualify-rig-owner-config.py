#!/usr/bin/env python3
"""Observe native-owner configuration effects of a real private Rig launch.

Uses a disposable unauthenticated Codex home and a synthetic launch wrapper.
It never uses the operator's home, account, daemon, or tmux server. This is a
manager configuration boundary control, not authenticated WebMux recovery.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shlex
import socket
import subprocess
import time
import tomllib
import urllib.request
import uuid


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--node', required=True, type=Path)
    parser.add_argument('--cli', required=True, type=Path)
    parser.add_argument('--codex', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--hooks', choices=['enabled', 'disabled'], default='disabled')
    parser.add_argument('--expect', choices=['preserved', 'changed'], default='preserved',
                        help='Use changed only to reproduce the known uncorrected defect')
    args = parser.parse_args()
    node, cli, codex = [p.resolve(strict=True) for p in (args.node, args.cli, args.codex)]
    args.output.mkdir(mode=0o700, parents=True, exist_ok=True)
    root = args.output / ('run-' + uuid.uuid4().hex)
    root.mkdir(mode=0o700)
    homes = {key: root / name for key, name in [('HOME', 'manager-home'), ('OPENRIG_HOME', 'manager-state'), ('CODEX_HOME', 'owner-home'), ('XDG_CONFIG_HOME', 'config'), ('XDG_DATA_HOME', 'data'), ('XDG_STATE_HOME', 'state'), ('XDG_CACHE_HOME', 'cache')]}
    for p in homes.values():
        p.mkdir(mode=0o700)
    project, bins, topology, agent = [root / n for n in ('project', 'bin', 'topology', 'agent')]
    for p in (project, bins, topology, agent):
        p.mkdir(mode=0o700)
    subprocess.run(['git', 'init', '-q', str(project)], check=True, timeout=10)
    owner = homes['CODEX_HOME']
    config = owner / 'config.toml'
    # Trust is present before startup, so the fixture is not asking Rig to
    # approve a new project. The parent trust covers this private project.
    original = 'approval_policy="never"\nsandbox_mode="read-only"\n[projects.' + json.dumps(str(root)) + ']\ntrust_level="trusted"\n'
    config.write_text(original)
    config.chmod(0o600)
    token = 'wm-owner-' + uuid.uuid4().hex[:12]
    tmux = subprocess.check_output(['sh', '-c', 'command -v tmux'], text=True).strip()
    def executable(p, content):
        p.write_text(content)
        p.chmod(0o700)
    executable(bins / 'tmux', '#!/bin/sh\nexec ' + shlex.join([tmux, '-L', token]) + ' "$@"\n')
    executable(bins / 'rig', '#!/bin/sh\nexec ' + shlex.join([str(node), str(cli)]) + ' "$@"\n')
    (bins / 'codex').symlink_to(codex)
    (bins / 'node').symlink_to(node)
    marker = root / 'wrapper-started'
    wrapper = root / 'provider.sh'
    executable(wrapper, '#!/bin/sh\nprintf ready > ' + shlex.quote(str(marker)) + '\nexec /bin/sleep 120\n')
    (agent / 'agent.yaml').write_text('name: owner-boundary-control\nversion: "1.0"\ndescription: disposable synthetic wrapper\ndefaults:\n  runtime: codex\nprofiles:\n  default:\n    uses:\n      skills: []\n      guidance: []\n      subagents: []\n      plugins: []\n      runtime_resources: []\n')
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        port = listener.getsockname()[1]
    env = {**{k: str(v) for k, v in homes.items()}, 'PATH': str(bins) + ':/usr/local/bin:/usr/bin:/bin', 'LANG': 'C.UTF-8', 'OPENRIG_URL': 'http://127.0.0.1:' + str(port), 'OPENRIG_RUNTIME_CODEX_HOOKS_ENABLED': str(args.hooks == 'enabled').lower()}
    (root / 'probe-source.py').write_bytes(Path(__file__).read_bytes())
    report = {'passed': False, 'expected_owner_config': args.expect, 'scope': __doc__.strip(), 'fixture': str(root), 'hooks': args.hooks, 'operator_home_used': False, 'credentials_copied': False, 'native_cli_sha256': sha(codex), 'node_sha256': sha(node), 'cli_sha256': sha(cli), 'harness_sha256': sha(Path(__file__)), 'config_before_sha256': sha(config)}
    started = False
    def rig(*words, check=True):
        p = subprocess.run([str(node), str(cli), *words], env=env, capture_output=True, text=True, timeout=35)
        with (root / 'rig.log').open('a') as f:
            f.write(json.dumps({'args': words, 'exit_code': p.returncode, 'stdout': p.stdout, 'stderr': p.stderr}) + '\n')
        if check and p.returncode:
            raise RuntimeError('private_rig_command_failed:' + ' '.join(words[:2]))
        return p
    def api(route, body=None, headers=None):
        req = urllib.request.Request(env['OPENRIG_URL'] + route, data=None if body is None else json.dumps(body).encode(), headers={'Content-Type': 'application/json', **(headers or {})})
        with urllib.request.urlopen(req, timeout=8) as response:
            return json.load(response)
    try:
        report['rig_version'] = rig('--version').stdout.strip()
        assert report['rig_version'] == '0.7.18 (3bf87c62)', 'unqualified_manager'
        started = True
        rig('daemon', 'start', '--host', '127.0.0.1', '--port', str(port), '--no-kernel')
        health = api('/healthz')
        assert health['semver'] == '0.7.18' and health['commit'] == '3bf87c62a24fcb0a441334d7f8b0b1f2687ecf64' and not health['dirty']
        report['manager_pid'] = health['pid']
        spec = {'version': '0.2', 'name': 'owner-boundary-control', 'summary': 'Disposable configuration boundary control', 'pods': [{'id': 'agents', 'label': 'control', 'members': []}], 'edges': []}
        operation = api('/api/rigs/import/materialize', spec, {'X-OpenRig-Operation-Id': str(uuid.uuid4()), 'X-Rig-Root': str(topology), 'X-Cwd-Override': str(project)})
        rig_id = operation['operation']['rigId']
        member = {'id': 'probe', 'runtime': 'codex', 'agent_ref': 'path:' + str(agent), 'profile': 'default', 'cwd': str(project), 'permission_policy': 'builtin:yolo', 'env': {'CODEX_HOME': str(owner)}, 'launch_wrapper': ['/bin/sh', str(wrapper)]}
        report['member_operation_id'] = str(uuid.uuid4())
        try:
            reply = api('/api/rigs/' + rig_id + '/pods/agents/members', {'member': member, 'rigRoot': str(topology)}, {'X-OpenRig-Operation-Id': report['member_operation_id']})
            (root / 'member-reply.json').write_text(json.dumps(reply, indent=2) + '\n')
            report['member_reply'] = 'received'
        except TimeoutError:
            # A launch may be admitted while the native readiness response is
            # still pending. Never repost it or treat timeout as confirmation.
            report['member_reply'] = 'uncertain_after_original_eight_second_deadline'
        deadline = time.monotonic() + 25
        while time.monotonic() < deadline and not marker.exists():
            time.sleep(.2)
        report.update(wrapper_started=marker.exists(), config_after_sha256=sha(config), owner_config_changed=config.read_text() != original, manager_native_account_created=(owner / 'auth.json').exists())
        report['owner_preservation_passed'] = not report['owner_config_changed']
        report['defect_reproduced'] = report['owner_config_changed']
        # Explicit defect reproduction is separate from the default acceptance
        # assertion. Neither mode proves authenticated WebMux recovery.
        assert report['owner_config_changed'] == (args.expect == 'changed'), 'unexpected_owner_configuration_effect'
        assert report['wrapper_started'], 'custom_wrapper_not_observed'
        assert not report['manager_native_account_created'], 'unexpected_fixture_account'
        report['passed'] = True
    except Exception as error:
        report['error'] = type(error).__name__ + ':' + str(error)
    finally:
        # Stop private resources before inspecting possibly damaged output.
        if started:
            try:
                report['daemon_stop_exit'] = rig('daemon', 'stop', check=False).returncode
            except (OSError, subprocess.SubprocessError) as error:
                report['daemon_stop_error'] = type(error).__name__
        try:
            subprocess.run([tmux, '-L', token, 'kill-server'], env=env, capture_output=True, timeout=10)
            check = subprocess.run([tmux, '-L', token, 'list-sessions'], env=env, capture_output=True, timeout=10)
            report['private_tmux_stopped'] = check.returncode != 0
        except (OSError, subprocess.SubprocessError) as error:
            report['private_tmux_stopped'] = False
            report['tmux_cleanup_error'] = type(error).__name__
        try:
            with socket.create_connection(('127.0.0.1', port), timeout=1):
                report['private_daemon_port_closed'] = False
        except OSError:
            report['private_daemon_port_closed'] = True
        try:
            report['owner_config_changed'] = config.read_text() != original
            report['owner_preservation_passed'] = not report['owner_config_changed']
            parsed = tomllib.loads(config.read_text())
            report['original_approval_and_sandbox_retained'] = parsed.get('approval_policy') == 'never' and parsed.get('sandbox_mode') == 'read-only'
            report['project_trust_entries'] = len(parsed.get('projects', {}))
            report['managed_hooks_present'] = 'hooks' in parsed
            report['config_final_sha256'] = sha(config)
        except (OSError, tomllib.TOMLDecodeError) as error:
            report['passed'] = False
            report['config_inspection_error'] = type(error).__name__
        (root / 'receipt.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))
    return 0 if report['passed'] and report['private_tmux_stopped'] and report['private_daemon_port_closed'] and report.get('daemon_stop_exit') == 0 else 1


if __name__ == '__main__':
    raise SystemExit(main())
