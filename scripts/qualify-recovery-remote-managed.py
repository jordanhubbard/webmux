#!/usr/bin/env python3
"""Qualify managed-session fallback when a remote execution host fails.

Runs a private WebMux controller and an isolated Agent Deck (private HOME, XDG
roots and tmux socket) on this machine against the retained disposable QEMU
guest. Three shell jobs record every execution:

  remote  - owned by the guest's own Agent Deck, registered here as a remote
  ssh     - owned by the local Agent Deck, executing on the guest over --ssh
  local   - owned and executed locally (control: must be unaffected)

Phases: controller restart and SIGKILL with the guest healthy, the guest hung
(paused through QMP) and resumed, an actual guest reboot, and the guest powered
off. Every phase requires clear fallback, retained transcripts, bounded startup
and no duplicate execution. Native coding-CLI resume on a remote host is not
offered and is not tested. Only the validated guest is paused, rebooted and
powered off; the user's WebMux and Agent Deck configuration are not touched.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import socket
import subprocess
import time
import urllib.error
import urllib.request
import uuid

GUEST_HOME = '/home/webmuxqa'
GUEST_DECK = GUEST_HOME + '/qualification/agent-deck'
STARTUP_LIMIT = 40


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--vm-root', type=Path, required=True)
    parser.add_argument('--binary', type=Path, required=True, help='WebMux build for this machine')
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[1]
    root = args.vm_root.resolve()
    binary = args.binary.resolve(strict=True)
    launch = json.loads((root / 'launch.json').read_text())
    ssh_args = json.loads((root / 'ssh-argv.json').read_text())
    assert ssh_args[-1] == 'webmuxqa@127.0.0.1' and launch['ssh_port'] == 32222
    process = subprocess.run(['ps', '-p', str(launch['pid']), '-o', 'command='], capture_output=True, text=True)
    assert 'qemu-system-aarch64' in process.stdout and str(root / 'disk.qcow2') in process.stdout, 'start the disposable guest first'
    deck_binary = shutil.which('agent-deck')
    assert deck_binary and 'Agent Deck v1.16.34' in subprocess.run([deck_binary, 'version'], capture_output=True, text=True).stdout

    def qmp(command):
        with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as connection:
            connection.settimeout(5)
            connection.connect(str(root / 'qmp.sock'))
            stream = connection.makefile('rwb', buffering=0)
            json.loads(stream.readline())
            for name in ('qmp_capabilities', command):
                stream.write(json.dumps({'execute': name}).encode() + b'\n')
                reply = json.loads(stream.readline())
                while 'event' in reply:
                    reply = json.loads(stream.readline())
            return reply['return']

    def guest(command, check=True, timeout=30):
        result = subprocess.run(ssh_args[:-1] + ['-o', 'ConnectTimeout=3', ssh_args[-1], command], capture_output=True, text=True, timeout=timeout)
        if check and result.returncode:
            raise RuntimeError('guest command failed: ' + result.stderr[-800:])
        return result.stdout.strip()

    assert qmp('query-status')['running']
    assert guest('hostname') == 'webmux-recovery-qa', 'Refusing a host that is not the disposable guest'
    assert guest('test -f /var/tmp/webmux-qualification-ready && echo ok') == 'ok'
    assert guest(GUEST_DECK + ' version').startswith('Agent Deck v1.16.34')

    tag = uuid.uuid4().hex[:12]
    p = Path.home() / '.local/share/webmux/recovery-qualification' / ('remote-managed-' + tag)
    p.mkdir(mode=0o700, parents=True)
    g = GUEST_HOME + '/qualification/remote-managed-' + tag
    alias = 'wmqa-' + tag
    env = {k: v for k, v in os.environ.items() if not k.startswith(('WEBMUX_', 'AGENTDECK_', 'AGENT_DECK_')) and k not in ('TMUX', 'TMUX_PANE')}
    for name in ('home', 'bin', 'xdg_config_home', 'xdg_data_home', 'xdg_state_home', 'xdg_cache_home', 'local-project'):
        (p / name).mkdir(mode=0o700)
    env.update(HOME=str(p / 'home'), SHELL='/bin/bash', AGENTDECK_PROFILE='default', DO_NOT_TRACK='1',
               PATH=str(p / 'bin') + os.pathsep + os.environ['PATH'],
               **{k.upper(): str(p / k) for k in ('xdg_config_home', 'xdg_data_home', 'xdg_state_home', 'xdg_cache_home')})
    # Agent Deck runs `ssh` from PATH. This wrapper reaches only the guest alias.
    options = ssh_args[1:-1]
    config = ['Host ' + alias, '  HostName 127.0.0.1', '  User webmuxqa']
    for i in range(0, len(options), 2):
        flag, value = options[i], options[i + 1]
        if flag == '-o':
            key, val = value.split('=', 1)
            config.append('  ' + key + ' ' + val)
        elif flag == '-i':
            config.append('  IdentityFile ' + value)
        elif flag == '-p':
            config.append('  Port ' + value)
    config += ['  ServerAliveInterval 5', '  ServerAliveCountMax 2']
    (p / 'ssh_config').write_text('\n'.join(config) + '\n')
    (p / 'bin/ssh').write_text('#!/bin/sh\nexec /usr/bin/ssh -F ' + shlex.quote(str(p / 'ssh_config')) + ' "$@"\n')
    (p / 'bin/ssh').chmod(0o700)
    socket_name = 'wm-remote-managed-' + tag
    (p / 'xdg_config_home/agent-deck').mkdir(mode=0o700)
    (p / 'xdg_config_home/agent-deck/config.toml').write_text('[tmux]\nsocket_name = ' + json.dumps(socket_name) + '\n[security]\nauto_trust_folders = false\n[launch]\ninject_identity = false\n')

    record_path = p / 'receipt.json'
    record = {'passed': False, 'scope': __doc__.strip().splitlines()[0], 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'qualifier_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'agent_deck': '1.16.34', 'fixture': str(p), 'guest_fixture': g, 'phases': []}

    def save():
        record_path.write_text(json.dumps(record, indent=2) + '\n')
        record_path.chmod(0o600)

    def phase(name, **data):
        record['phases'].append({'phase': name, **data})
        save()
        print(json.dumps({'phase': name, 'passed': True}), flush=True)

    def deck(*a, check=True):
        result = subprocess.run([deck_binary, *a], env=env, capture_output=True, text=True, timeout=60)
        with (p / 'agent-deck-calls.jsonl').open('a') as f:
            f.write(json.dumps({'args': a, 'returncode': result.returncode, 'stdout': result.stdout[-2000:], 'stderr': result.stderr[-2000:]}) + '\n')
        if check and result.returncode:
            raise RuntimeError('agent-deck ' + ' '.join(a[:3]) + ' failed: ' + result.stderr[-400:])
        return json.loads(result.stdout) if result.stdout.strip().startswith('{') else result.stdout

    job = ('import pathlib,time,os,sys\np=pathlib.Path(sys.argv[1])\nwith p.open("a") as f: f.write(str(os.getpid())+"\\n"); f.flush(); os.fsync(f.fileno())\n'
           'while True: print("managed tick", sys.argv[2], flush=True); time.sleep(2)\n')
    markers = {name: 'WM_REMOTE_MANAGED_' + name.upper() + '_' + uuid.uuid4().hex[:8] for name in ('remote', 'ssh', 'local')}
    guest('mkdir -p ' + g + '/remote ' + g + '/ssh && cat > ' + g + '/job.py <<"EOF"\n' + job + 'EOF')
    (p / 'local-project/job.py').write_text(job)

    def executions():
        counts = {'local': len((p / 'local-executions').read_text().splitlines()) if (p / 'local-executions').exists() else 0}
        try:
            out = guest('for f in remote ssh; do test -f ' + g + '/$f-executions && wc -l < ' + g + '/$f-executions || echo 0; done')
            counts['remote'], counts['ssh'] = (int(x) for x in out.split())
        except (RuntimeError, subprocess.TimeoutExpired, ValueError):
            counts['remote'] = counts['ssh'] = None
        return counts

    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
        port = s.getsockname()[1]
    base = 'http://127.0.0.1:' + str(port)
    home = p / 'webmux-home'
    shutil.copytree(repo / 'webmux/config.defaults', home / 'config')
    (home / 'config/auth.yaml').write_text('auth:\n  mode: none\n  users: []\n')
    (home / 'config/app.yaml').write_text('app:\n  session_logging:\n    enabled: true\n  coordination:\n    enabled_providers: [agent-deck]\n    default_launch_provider: webmux\n    default_view: all\n')
    controller_env = dict(env, WEBMUX_HOME=str(home), WEBMUX_COORDINATION_DISABLED='0')
    controller = None
    log = (p / 'controller.log').open('ab')

    def api(path, data=None):
        request = urllib.request.Request(base + path, data=json.dumps(data).encode() if data is not None else None, headers={'Content-Type': 'application/json'})
        with urllib.request.urlopen(request, timeout=60) as response:
            body = response.read()
            return json.loads(body) if body else None

    def wait(fn, label, seconds=60):
        end = time.monotonic() + seconds
        last = None
        while time.monotonic() < end:
            try:
                value = fn()
                if value:
                    return value
            except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError) as error:
                last = error
            time.sleep(0.5)
        raise RuntimeError('timed out waiting for ' + label + ': ' + str(last))

    def start():
        nonlocal controller
        began = time.monotonic()
        controller = subprocess.Popen([str(binary), '-listen', '127.0.0.1:' + str(port)], env=controller_env, stdin=subprocess.DEVNULL, stdout=log, stderr=log, start_new_session=True)
        wait(lambda: api('/api/health')['status'] == 'ok', 'controller ready', STARTUP_LIMIT + 20)
        return round(time.monotonic() - began, 1)

    def stop(kill=False):
        nonlocal controller
        if controller and controller.poll() is None:
            controller.kill() if kill else controller.terminate()
            controller.wait(timeout=20)
        controller = None

    def view(name):
        return api('/api/session-recovery/' + sids[name])

    def disposition(name):
        state = view(name).get('state') or {}
        return state.get('disposition'), state.get('manager_message') or ''

    def connected(name):
        return api('/api/sessions/' + sids[name])['state'] == 'connected'

    def refused_reconnect(name):
        began = time.monotonic()
        try:
            api('/api/sessions/' + sids[name] + '/reconnect', {})
        except urllib.error.HTTPError as error:
            body = json.loads(error.read() or b'{}')
            assert error.code == 409, error.code
            return {'status': 409, 'error': body.get('error', ''), 'seconds': round(time.monotonic() - began, 1)}
        raise AssertionError(name + ' reconnected to a process that is not confirmed running')

    def history(name):
        if markers[name] not in (view(name).get('history') or ''):
            raise RuntimeError(name + ' transcript lost its pre-interruption output')
        return True

    def boot_id():
        return guest('cat /proc/sys/kernel/random/boot_id')

    sids = {}
    try:
        # Lifecycle owners: the guest's own Agent Deck, local --ssh, local.
        deck('remote', 'add', 'qa', alias, '--agent-deck-path', GUEST_DECK)
        remote = deck('remote', 'qa', 'add', g + '/remote', '--cmd', '/usr/bin/python3 ' + g + '/job.py ' + g + '/remote-executions ' + markers['remote'], '--no-parent', '--no-identity', '--title', 'wm-remote-' + tag, '--json')
        ssh_job = deck('add', str(p / 'local-project'), '--ssh', alias, '--remote-path', g + '/ssh', '--cmd', '/usr/bin/python3 ' + g + '/job.py ' + g + '/ssh-executions ' + markers['ssh'], '--no-parent', '--no-identity', '--title', 'wm-ssh-' + tag, '--json')
        local = deck('add', str(p / 'local-project'), '--cmd', '/usr/bin/python3 ' + str(p / 'local-project/job.py') + ' ' + str(p / 'local-executions') + ' ' + markers['local'], '--no-parent', '--no-identity', '--title', 'wm-local-' + tag, '--json')
        targets = {'remote': 'qa:' + remote['id'], 'ssh': 'local:' + ssh_job['id'], 'local': 'local:' + local['id']}
        deck('remote', 'qa', 'session', 'start', remote['id'], '--json')
        deck('session', 'start', ssh_job['id'], '--json')
        deck('session', 'start', local['id'], '--json')
        wait(lambda: executions() == {'local': 1, 'remote': 1, 'ssh': 1}, 'all three jobs started once')
        record['targets'] = targets
        record['boot_ids'] = [boot_id()]
        startup = start()
        for name, target in targets.items():
            sids[name] = api('/api/coordination/attach', {'provider': 'agent-deck', 'target': target})['id']
        for name in targets:
            wait(lambda: connected(name), name + ' initial viewer')
            wait(lambda: history(name), name + ' initial transcript')
        phase('initial managed viewers', startup_seconds=startup, executions=executions(), webmux_sessions=sids)

        for abrupt in (False, True):
            stop(kill=abrupt)
            startup = start()
            for name in targets:
                wait(lambda: disposition(name)[0] == 'reattached' and connected(name), name + ' reattachment')
                history(name)
            assert executions() == {'local': 1, 'remote': 1, 'ssh': 1}
            phase('healthy guest, controller ' + ('SIGKILL' if abrupt else 'restart'), startup_seconds=startup, executions=executions())

        # Hung host: connections stall rather than fail.
        stop()
        qmp('stop')
        try:
            startup = start()
            assert startup <= STARTUP_LIMIT, 'startup waited %ss for a hung remote host' % startup
            state, message = disposition('remote')
            assert state == 'managed-unavailable' and 'remote "qa" did not respond' in message, (state, message)
            wait(lambda: disposition('local')[0] == 'reattached' and connected('local'), 'local session isolated from the hung host', 10)
            refused = refused_reconnect('remote')
            assert refused['seconds'] <= 30 and 'did not respond' in refused['error']
            history('remote')
            hung = {'startup_seconds': startup, 'remote': disposition('remote'), 'ssh': disposition('ssh'), 'local': disposition('local'), 'remote_reconnect': refused}
        finally:
            qmp('cont')
        wait(lambda: guest('true') == '', 'guest resumed')
        counts = executions()
        assert counts['remote'] == 1 and counts['ssh'] == 1 and counts['local'] == 1, counts
        def reattached_after_resume():
            try:
                api('/api/sessions/' + sids['remote'] + '/reconnect', {})
            except urllib.error.HTTPError:
                time.sleep(2)
                return False
            return wait(lambda: connected('remote'), 'remote viewer', 20)
        wait(reattached_after_resume, 'remote reattachment after the host resumed', 120)
        phase('hung guest, then resumed', **hung, executions=counts)

        # Actual reboot of the execution host while the controller runs.
        previous = record['boot_ids'][0]
        guest('sudo -n systemctl reboot', check=False)
        wait(lambda: boot_id() != previous, 'actual guest reboot', 240)
        record['boot_ids'].append(boot_id())
        stop()
        startup = start()
        for name in ('remote', 'ssh'):
            state, message = wait(lambda: (lambda d: d if d[0] == 'managed-unavailable' else None)(disposition(name)), name + ' fallback after reboot')
            assert 'not confirmed running' in message, (name, message)
            history(name)
        wait(lambda: disposition('local')[0] == 'reattached' and connected('local'), 'local session after guest reboot')
        refusals = {name: [refused_reconnect(name) for _ in range(3)] for name in ('remote', 'ssh')}
        shown = deck('remote', 'qa', 'show', remote['id'], '--json')
        assert shown['status'] not in ('running', 'waiting', 'idle'), shown['status']
        # The bracket keeps the pattern from matching this probe's own shell.
        assert guest('pgrep -f ' + shlex.quote(g + '/jo[b].py') + ' || true') == '', 'a job was restarted on the guest'
        counts = executions()
        assert counts == {'local': 1, 'remote': 1, 'ssh': 1}, counts
        phase('actual guest reboot', startup_seconds=startup, remote=disposition('remote'), ssh=disposition('ssh'), local=disposition('local'),
              repeated_reconnects=refusals, remote_manager_status=shown['status'], executions=counts)

        # Host powered off: connections are refused.
        guest('sudo -n systemctl poweroff', check=False)
        wait(lambda: subprocess.run(['ps', '-p', str(launch['pid'])], capture_output=True).returncode != 0, 'guest power-off', 120)
        stop()
        startup = start()
        assert startup <= STARTUP_LIMIT, startup
        state, message = disposition('remote')
        assert state == 'managed-unavailable' and 'remote "qa" did not respond' in message, (state, message)
        assert disposition('ssh')[0] == 'managed-unavailable'
        wait(lambda: disposition('local')[0] == 'reattached' and connected('local'), 'local session with the guest powered off')
        refused = refused_reconnect('remote')
        for name in targets:
            history(name)
        assert executions()['local'] == 1
        phase('guest powered off', startup_seconds=startup, remote=disposition('remote'), ssh=disposition('ssh'), local=disposition('local'), remote_reconnect=refused)
        assert hashlib.sha256(Path(__file__).read_bytes()).hexdigest() == record['qualifier_sha256']
        record['passed'] = True
    except Exception as error:
        record['error'] = repr(error)
        for name in sids:
            try:
                record.setdefault('failure_views', {})[name] = view(name).get('state')
            except (OSError, ValueError):
                pass
        raise
    finally:
        stop()
        for target in ('ssh', 'local'):
            if record.get('targets'):
                deck('session', 'stop', record['targets'][target].split(':', 1)[1], '--json', check=False)
        subprocess.run(['tmux', '-L', socket_name, 'kill-server'], capture_output=True)
        log.close()
        save()
        print(json.dumps({'passed': record['passed'], 'evidence': str(record_path)}), flush=True)
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
