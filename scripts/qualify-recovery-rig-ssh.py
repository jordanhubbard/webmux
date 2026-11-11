#!/usr/bin/env python3
"""Qualify fallback for a WebMux-coordinated Rig seat on an SSH execution host.

The seat is a local tmux session whose pane runs ssh; the coding CLI (Claude
Code) runs in a tmux session on the retained disposable QEMU guest. This runs
a private WebMux controller, Rig daemon, tmux server (TMUX_TMPDIR) and HOME on
this machine, and an ssh wrapper that reaches only the guest alias.

Phases: controller restart and SIGKILL with the guest healthy; the guest hung
(paused through QMP) past the seat's SSH keepalive, then resumed; loss of the
local seat with the guest untouched (what a reboot of this machine does to the
seat); an actual guest reboot; and the guest powered off. Each phase requires
truthful fallback, retained transcripts, bounded startup, refused automatic
reconnection when the seat is gone, and no duplicate coding-CLI process. The
explicit way back - starting the agent again with the same settings - must
reattach a surviving CLI, start exactly one new conversation when it is gone,
and start nothing when the host is unreachable. Only the validated guest is
paused, rebooted and powered off. The guest uses its own Claude login.
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
RIG_VERSION = '0.6.14 (9d0200f3)'
GUEST_RIG = GUEST_HOME + '/qualification/openrig-9d0200f3/node_modules/@openrig/cli/dist/bin-wrapper.js'
GUEST_NODE = GUEST_HOME + '/qualification/node-22.23.3/bin/node'
STARTUP_LIMIT = 40
SSH_LOST = 'local SSH connection for this Rig seat on '


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--vm-root', type=Path, required=True)
    parser.add_argument('--binary', type=Path, required=True, help='WebMux build for this machine')
    parser.add_argument('--rig-cli', type=Path, required=True, help='bin-wrapper.js of the qualified Rig release on this machine')
    parser.add_argument('--rig-node', type=Path, required=True, help='node executable for that Rig release')
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[1]
    root = args.vm_root.resolve()
    binary = args.binary.resolve(strict=True)
    launch = json.loads((root / 'launch.json').read_text())
    ssh_args = json.loads((root / 'ssh-argv.json').read_text())
    assert ssh_args[-1] == 'webmuxqa@127.0.0.1' and launch['ssh_port'] == 32222
    process = subprocess.run(['ps', '-p', str(launch['pid']), '-o', 'command='], capture_output=True, text=True)
    assert 'qemu-system-aarch64' in process.stdout and str(root / 'disk.qcow2') in process.stdout, 'start the disposable guest first'
    # Never use an installed `rig` command: WebMux's wrapper pins HOME and
    # OPENRIG_HOME to the installed instance, which would defeat isolation.
    rig_cli, rig_node = args.rig_cli.resolve(strict=True), args.rig_node.resolve(strict=True)

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
    assert guest('tmux -V') == 'tmux 3.4'
    assert json.loads(guest(GUEST_HOME + '/.local/bin/claude auth status --json'))['loggedIn'], 'Authenticate the guest Claude first'
    # WebMux requires `rig` on the execution host; expose the qualified build.
    assert guest(shlex.join([GUEST_NODE, GUEST_RIG, '--version'])) == RIG_VERSION
    guest('test -x ~/.local/bin/rig || { printf "#!/bin/sh\\nexec %s %s \\"\\$@\\"\\n" > ~/.local/bin/rig && chmod 755 ~/.local/bin/rig; }' % (GUEST_NODE, GUEST_RIG))
    assert guest('~/.local/bin/rig --version') == RIG_VERSION

    tag = uuid.uuid4().hex[:12]
    p = Path.home() / '.local/share/webmux/recovery-qualification' / ('rig-ssh-' + tag)
    p.mkdir(mode=0o700, parents=True)
    g = GUEST_HOME + '/qualification/rig-ssh-' + tag
    workspace = g + '/project'
    alias = 'wmqa-' + tag
    for name in ('home', 'bin', 'rig', 'tmux'):
        (p / name).mkdir(mode=0o700)
    env = {k: v for k, v in os.environ.items() if not k.startswith(('WEBMUX_', 'OPENRIG_', 'RIGGED_', 'AGENTDECK_', 'AGENT_DECK_')) and k not in ('TMUX', 'TMUX_PANE')}
    env.update(HOME=str(p / 'home'), OPENRIG_HOME=str(p / 'rig'), TMUX_TMPDIR=str(p / 'tmux'), PATH=str(p / 'bin') + os.pathsep + os.environ['PATH'])
    # WebMux runs `ssh` from PATH. This wrapper reaches only the guest alias.
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
    (p / 'ssh_config').write_text('\n'.join(config) + '\n')
    (p / 'bin/ssh').write_text('#!/bin/sh\nexec /usr/bin/ssh -F ' + shlex.quote(str(p / 'ssh_config')) + ' "$@"\n')
    (p / 'bin/ssh').chmod(0o700)
    rig_binary = str(p / 'bin/rig')
    Path(rig_binary).write_text('#!/bin/sh\nexec ' + shlex.quote(str(rig_node)) + ' ' + shlex.quote(str(rig_cli)) + ' "$@"\n')
    Path(rig_binary).chmod(0o700)
    assert subprocess.run([rig_binary, '--version'], env=env, capture_output=True, text=True).stdout.strip() == RIG_VERSION

    record_path = p / 'receipt.json'
    record = {'passed': False, 'scope': __doc__.strip().splitlines()[0], 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'qualifier_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'rig': RIG_VERSION,
              'guest_claude': guest(GUEST_HOME + '/.local/bin/claude --version'), 'fixture': str(p), 'guest_fixture': g, 'phases': []}

    def save():
        record_path.write_text(json.dumps(record, indent=2) + '\n')
        record_path.chmod(0o600)

    def phase(name, **data):
        record['phases'].append({'phase': name, **data})
        save()
        print(json.dumps({'phase': name, 'passed': True}), flush=True)

    def tmux(*a):
        return subprocess.run(['tmux', *a], env=env, capture_output=True, text=True)

    def rig(*a):
        return subprocess.run([rig_binary, *a], env=env, capture_output=True, text=True, timeout=60)

    def clis():
        """Claude processes running in this fixture's workspace on the guest."""
        out = guest('for pid in $(pgrep -x claude); do [ "$(readlink /proc/$pid/cwd)" = ' + shlex.quote(workspace) + ' ] && echo $pid; done; true')
        return sorted(int(x) for x in out.split())

    def remote_seats():
        return guest('tmux has-session -t ' + shlex.quote('=' + target) + ' 2>/dev/null && echo 1 || echo 0')

    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
        port = s.getsockname()[1]
    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
        rig_port = s.getsockname()[1]
    base = 'http://127.0.0.1:' + str(port)
    home = p / 'webmux-home'
    shutil.copytree(repo / 'webmux/config.defaults', home / 'config')
    (home / 'config/auth.yaml').write_text('auth:\n  mode: none\n  users: []\n')
    (home / 'config/app.yaml').write_text('app:\n  session_logging:\n    enabled: true\n  coordination:\n    enabled_providers: [rig]\n    default_launch_provider: webmux\n    default_view: all\n')
    controller_env = dict(env, WEBMUX_HOME=str(home), WEBMUX_COORDINATION_DISABLED='0')
    controller = None
    log = (p / 'controller.log').open('ab')

    def api(path, data=None, timeout=90):
        request = urllib.request.Request(base + path, data=json.dumps(data).encode() if data is not None else None, headers={'Content-Type': 'application/json'})
        with urllib.request.urlopen(request, timeout=timeout) as response:
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

    spec = {'provider': 'rig', 'mode': 'new', 'projectName': 'wmqa-' + tag[:8], 'name': 'guest', 'host': alias, 'workspace': workspace, 'runtime': 'claude-code'}
    sid = target = None

    def view():
        return api('/api/session-recovery/' + sid)

    def disposition():
        v = view()
        state = v.get('state') or {}
        return state.get('disposition'), state.get('manager_message') or '', v.get('message') or ''

    def connected():
        return api('/api/sessions/' + sid)['state'] == 'connected'

    def seat_alive():
        return tmux('has-session', '-t', '=' + target).returncode == 0

    def refused_reconnect():
        began = time.monotonic()
        try:
            api('/api/sessions/' + sid + '/reconnect', {})
        except urllib.error.HTTPError as error:
            body = json.loads(error.read() or b'{}')
            assert error.code == 409, error.code
            assert SSH_LOST + alias in body.get('error', ''), body
            return {'status': 409, 'error': body.get('error', ''), 'seconds': round(time.monotonic() - began, 1)}
        raise AssertionError('reconnected a seat whose SSH transport is gone')

    def relaunch():
        began = time.monotonic()
        try:
            value = api('/api/coordination/agents', spec)
        except urllib.error.HTTPError as error:
            return {'status': error.code, 'error': json.loads(error.read() or b'{}').get('error', ''), 'seconds': round(time.monotonic() - began, 1)}
        return {'status': 200, 'session': value['session']['id'], 'target': value['target'], 'seconds': round(time.monotonic() - began, 1)}

    def history():
        if workspace not in (view().get('history') or ''):
            raise RuntimeError('transcript lost its pre-interruption output')
        return True

    def boot_id():
        return guest('cat /proc/sys/kernel/random/boot_id')

    try:
        guest('mkdir -p ' + shlex.quote(workspace))
        started = rig('daemon', 'start', '--host', '127.0.0.1', '--port', str(rig_port), '--db', str(p / 'rig/openrig.sqlite'), '--no-kernel')
        assert started.returncode == 0, started.stderr
        assert (p / 'rig/daemon.log').is_file(), 'the Rig daemon did not use the private OPENRIG_HOME'
        record['boot_ids'] = [boot_id()]
        startup = start()
        launched = api('/api/coordination/agents', spec)
        sid, target = launched['session']['id'], launched['target']
        record.update(webmux_session=sid, seat=target)
        wait(connected, 'initial viewer')
        wait(lambda: len(clis()) == 1, 'coding CLI on the guest')
        wait(history, 'initial transcript')
        original = clis()
        phase('initial seat', startup_seconds=startup, cli_pids=original, remote_seats=remote_seats())

        for abrupt in (False, True):
            stop(kill=abrupt)
            startup = start()
            wait(lambda: disposition()[0] == 'reattached' and connected(), 'reattachment')
            history()
            assert clis() == original, clis()
            phase('healthy guest, controller ' + ('SIGKILL' if abrupt else 'restart'), startup_seconds=startup, cli_pids=clis(), message=disposition()[2])

        # Hung host past the seat's SSH keepalive (15 s x 3).
        stop()
        qmp('stop')
        hung_at = time.monotonic()
        try:
            startup = start()
            assert startup <= STARTUP_LIMIT, 'startup waited %ss for a hung execution host' % startup
            early = disposition()
            wait(lambda: not seat_alive(), 'seat SSH to give up on the hung host', 120)
            seat_lost = round(time.monotonic() - hung_at, 1)
            wait(lambda: not connected(), 'viewer to observe the lost seat', 15)
            later = disposition()
            assert later[0] == 'reattached' and 'has since ended' in later[2] and 'work was preserved' not in later[2], later
            refused = refused_reconnect()
            history()
        finally:
            qmp('cont')
        wait(lambda: guest('true') == '', 'guest resumed')
        assert clis() == original, 'the coding CLI did not survive the hang'
        back = relaunch()
        assert back['status'] == 200 and back['session'] == sid and back['target'] == target, back
        wait(connected, 'viewer after starting the agent again')
        assert clis() == original, 'starting the agent again duplicated a surviving CLI'
        phase('hung guest past SSH keepalive, then resumed', startup_seconds=startup, while_hung=early, seat_lost_after_seconds=seat_lost,
              after_seat_loss=later, reconnect=refused, start_again=back, cli_pids=clis())

        # Local seat lost, guest untouched: what a reboot of this machine does.
        stop()
        tmux('kill-server')
        startup = start()
        state, manager, message = disposition()
        assert state == 'managed-unavailable' and SSH_LOST + alias in manager, (state, manager)
        history()
        refusals = [refused_reconnect() for _ in range(3)]
        assert clis() == original, 'a refused reconnect touched the coding CLI'
        back = relaunch()
        assert back['status'] == 200 and back['session'] == sid, back
        wait(connected, 'viewer after starting the agent again')
        assert clis() == original, 'starting the agent again duplicated a surviving CLI'
        phase('local seat lost, guest untouched', startup_seconds=startup, disposition=state, manager_message=manager,
              repeated_reconnects=refusals, start_again=back, cli_pids=clis())

        # Actual reboot of the execution host while the controller runs.
        previous = record['boot_ids'][0]
        guest('sudo -n systemctl reboot', check=False)
        wait(lambda: not seat_alive(), 'seat to end with the rebooting guest', 120)
        wait(lambda: not connected(), 'viewer to observe the lost seat', 15)
        live_loss = disposition()
        assert 'work was preserved' not in live_loss[2], live_loss
        wait(lambda: boot_id() != previous, 'actual guest reboot', 240)
        record['boot_ids'].append(boot_id())
        assert clis() == [] and remote_seats() == '0', 'a coding CLI or remote seat survived the reboot'
        stop()
        startup = start()
        state, manager, message = disposition()
        assert state == 'managed-unavailable' and SSH_LOST + alias in manager, (state, manager)
        history()
        refusals = [refused_reconnect() for _ in range(3)]
        assert clis() == [], 'a refused reconnect started the coding CLI'
        # The explicit way back starts exactly one new conversation, as the message says.
        back = relaunch()
        assert back['status'] == 200 and back['session'] == sid, back
        wait(connected, 'viewer for the new conversation')
        fresh = wait(lambda: (lambda c: c if len(c) == 1 else None)(clis()), 'one new coding CLI')
        assert fresh != original
        time.sleep(3)
        assert clis() == fresh and remote_seats() == '1', 'starting the agent again started more than one CLI'
        phase('actual guest reboot', startup_seconds=startup, live_loss=live_loss, disposition=state, manager_message=manager,
              repeated_reconnects=refusals, start_again=back, cli_pids_after_start_again=fresh)

        # Host powered off: connections are refused.
        guest('sudo -n systemctl poweroff', check=False)
        wait(lambda: subprocess.run(['ps', '-p', str(launch['pid'])], capture_output=True).returncode != 0, 'guest power-off', 120)
        wait(lambda: not seat_alive(), 'seat to end with the powered-off guest', 120)
        stop()
        startup = start()
        assert startup <= STARTUP_LIMIT, startup
        state, manager, message = disposition()
        assert state == 'managed-unavailable' and SSH_LOST + alias in manager, (state, manager)
        history()
        refused = refused_reconnect()
        back = relaunch()
        assert back['status'] == 502 and back['seconds'] <= 30 and 'Cannot reach ' + alias + ' over SSH' in back['error'] and 'Nothing was started' in back['error'], back
        assert not seat_alive(), 'starting the agent against an unreachable host left a local seat'
        phase('guest powered off', startup_seconds=startup, disposition=state, manager_message=manager, reconnect=refused, start_again=back)
        assert hashlib.sha256(Path(__file__).read_bytes()).hexdigest() == record['qualifier_sha256']
        record['passed'] = True
    except Exception as error:
        record['error'] = repr(error)
        if sid:
            try:
                record['failure_view'] = disposition()
            except (OSError, ValueError):
                pass
        raise
    finally:
        stop()
        tmux('kill-server')
        rig('daemon', 'stop')
        if target:
            guest('tmux kill-session -t ' + shlex.quote('=' + target) + ' 2>/dev/null; true', check=False, timeout=10) if qmp_running(qmp) else None
        log.close()
        save()
        print(json.dumps({'passed': record['passed'], 'evidence': str(record_path)}), flush=True)
    return 0


def qmp_running(qmp):
    try:
        return qmp('query-status')['running']
    except OSError:
        return False


if __name__ == '__main__':
    raise SystemExit(main())
