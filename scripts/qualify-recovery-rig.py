#!/usr/bin/env python3
"""Qualify Rig-owned terminal survival and loss using a private native daemon.

Requires an existing synthetic Claude conversation manifest. The installed Rig
CLI and its Node runtime are supplied explicitly: the user's Rig wrapper can
pin a shared instance and must not be used for this fixture. An empty topology
and one adopted native conversation are created on a private tmux socket. No
kernel, agent role, delegated task or provider credentials are created/copied.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shlex
import signal
import shutil
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--rig-cli', type=Path, required=True)
    parser.add_argument('--rig-node', type=Path, required=True)
    args = parser.parse_args()
    script_hash = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    repo = Path(__file__).resolve().parents[1]
    binary, rig_cli, rig_node = [p.resolve(strict=True) for p in (args.binary, args.rig_cli, args.rig_node)]
    entries = json.loads(args.manifest.read_text())
    if isinstance(entries, dict):
        entries = entries['results']
    entry = next(e for e in entries if e['cli'] == 'claude')
    assert entry['conversational_resume'] and entry['marker'].startswith('WM_RECOVERY_')
    assert Path(entry['project']).resolve() == (Path(entry['evidence']) / 'project').resolve()
    uuid.UUID(entry['native_session_id'])
    original_path = os.environ.get('WEBMUX_BROWSER_ORIGINAL_PATH') or os.environ['PATH']
    claude = shutil.which('claude', path=original_path)
    tmux = shutil.which('tmux', path=original_path)
    assert claude and tmux
    roots = {key: os.environ.get(key) for key in ['HOME', 'CLAUDE_CONFIG_DIR', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME']}
    native_root = Path(roots['CLAUDE_CONFIG_DIR']) if roots['CLAUDE_CONFIG_DIR'] else Path(roots['HOME']) / '.claude'
    state_files = list((native_root / 'projects').glob('*/' + entry['native_session_id'] + '.jsonl'))
    assert len(state_files) == 1
    native_file = state_files[0]
    settings = native_root / 'settings.json'
    settings_hash = hashlib.sha256(settings.read_bytes()).hexdigest() if settings.is_file() else None
    fixture = Path.home() / '.local/share/webmux/recovery-qualification' / ('rig-native-' + str(uuid.uuid4()))
    fixture.mkdir(parents=True, mode=0o700)
    print(json.dumps({'fixture': str(fixture)}), flush=True)
    private_home, rig_home, home, aliases = [fixture / n for n in ['manager-home', 'rig-instance', 'webmux-home', 'bin']]
    for p in [private_home, rig_home, aliases]:
        p.mkdir(mode=0o700)
    shutil.copytree(repo / 'webmux/config.defaults', home / 'config')
    (home / 'config/auth.yaml').write_text('auth:\n  mode: none\n  users: []\n')
    (home / 'config/app.yaml').write_text('app:\n  session_logging:\n    enabled: true\n  coordination:\n    enabled_providers: [rig]\n    default_launch_provider: webmux\n    default_view: all\n')
    socket_name = 'wm-rig-' + uuid.uuid4().hex[:12]
    pane = 'wm-recovery-' + uuid.uuid4().hex[:12]
    env = dict(os.environ)
    for key in list(env):
        if key.startswith(('WEBMUX_RECOVERY_', 'OPENRIG_', 'RIGGED_', 'AGENTDECK_', 'AGENT_DECK_')) or key in ['TMUX', 'TMUX_PANE', 'WEBMUX_COORDINATION_DISABLED']:
            env.pop(key, None)
    env.update(HOME=str(private_home), OPENRIG_HOME=str(rig_home), CODEX_HOME=str(private_home / '.codex'), CLAUDE_CONFIG_DIR=str(private_home / '.claude'), WEBMUX_HOME=str(home), WEBMUX_ROOT='', PATH=str(aliases) + os.pathsep + original_path)
    for key in ['XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME']:
        p = fixture / key.lower()
        p.mkdir(mode=0o700)
        env[key] = str(p)
    def executable(path, text):
        path.write_text(text)
        path.chmod(0o700)
    restart_block = fixture / 'block-manager-start'
    executable(aliases / 'rig', '#!/bin/sh\nif [ "$1" = daemon ] && [ "${2-}" = start ] && [ -f ' + shlex.quote(str(restart_block)) + ' ]; then echo "Private qualification fixture denies manager startup" >&2; exit 1; fi\nexec ' + shlex.join([str(rig_node), str(rig_cli)]) + ' "$@"\n')
    executable(aliases / 'tmux', '#!/bin/sh\nexec ' + shlex.join([tmux, '-L', socket_name]) + ' "$@"\n')
    executable(aliases / 'node', '#!/bin/sh\nexec ' + shlex.quote(str(rig_node)) + ' "$@"\n')
    def rig(*words, check=True):
        result = subprocess.run([str(aliases / 'rig'), *words], env=env, capture_output=True, text=True, timeout=45)
        with (fixture / 'rig-events.jsonl').open('a') as f:
            f.write(json.dumps({'args': words, 'code': result.returncode, 'stdout': result.stdout, 'stderr': result.stderr}) + '\n')
        if check and result.returncode:
            raise RuntimeError('Private Rig operation failed: ' + ' '.join(words[:2]) + ': ' + result.stderr[-500:])
        return result
    def port():
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            return listener.getsockname()[1]
    rig_port, web_port = port(), port()
    base = 'http://127.0.0.1:' + str(web_port)
    def api(path, body=None, method=None):
        req = urllib.request.Request(base + path, data=json.dumps(body).encode() if body is not None else None, method=method, headers={'Content-Type': 'application/json'})
        with urllib.request.urlopen(req, timeout=35) as response:
            data = response.read()
            return json.loads(data) if data else None
    def wait(fn, label, seconds=90):
        end = time.monotonic() + seconds
        last = None
        while time.monotonic() < end:
            try:
                value = fn()
                if value:
                    return value
            except (OSError, ValueError, KeyError) as error:
                last = str(error)
            time.sleep(.5)
        raise RuntimeError('Timed out waiting for ' + label + ': ' + str(last))
    def mux(*words, check=True):
        return subprocess.run([tmux, '-L', socket_name, *words], env=env, capture_output=True, text=True, timeout=10, check=check)
    def native():
        result = mux('list-panes', '-a', '-F', '#{session_name} #{pane_pid}', check=False)
        rows = [line.split() for line in result.stdout.splitlines()]
        assert all(row[0] == pane for row in rows) and len(rows) <= 1, 'Unexpected native process duplication'
        return {'count': len(rows), 'pane_pid': int(rows[0][1]) if rows else None}
    log = (fixture / 'controller.log').open('ab')
    controller, sid, daemon_started = None, None, False
    phases = []
    def start():
        nonlocal controller
        controller = subprocess.Popen([str(binary), '-listen', '127.0.0.1:' + str(web_port)], env=env, stdin=subprocess.DEVNULL, stdout=log, stderr=log)
        def ready():
            if controller.poll() is not None:
                raise RuntimeError('Private WebMux controller exited during startup; inspect controller.log')
            return api('/api/health')['status'] == 'ok'
        wait(ready, 'private controller')
    def stop(abrupt=False):
        nonlocal controller
        if controller and controller.poll() is None:
            controller.kill() if abrupt else controller.terminate()
            try:
                controller.wait(timeout=15)
            except subprocess.TimeoutExpired:
                controller.kill()
                controller.wait(timeout=5)
        controller = None
    def view():
        return api('/api/session-recovery/' + sid)
    def connected():
        s = api('/api/sessions/' + sid)
        clients = mux('list-clients', '-F', '#{session_name}', check=False)
        return s if s['state'] == 'connected' and clients.stdout.splitlines().count(pane) == 1 else None
    def receipt(label, **data):
        phases.append({'phase': label, **data})
        (fixture / 'lifecycle-phases.json').write_text(json.dumps(phases, indent=2) + '\n')
        print(json.dumps({'phase': label, 'passed': True}), flush=True)
    def records():
        found = {}
        for number, line in enumerate(native_file.read_bytes().splitlines()):
            try:
                row = json.loads(line)
            except ValueError:
                continue
            if row.get('type') == 'assistant':
                found[str(number)] = ''.join(c.get('text', '') for c in row.get('message', {}).get('content', []) if c.get('type') == 'text')
        return found
    bridge = "import fs from 'node:fs';import WebSocket from 'ws';const i=JSON.parse(fs.readFileSync(0,'utf8'));const w=new WebSocket(i.url);const t=setTimeout(()=>{w.terminate();process.exitCode=1},7000);w.on('error',()=>{clearTimeout(t);process.exitCode=1});w.on('open',()=>{w.send(JSON.stringify({type:'input',data:i.text}));setTimeout(()=>w.send(JSON.stringify({type:'input',data:'\\r'})),1000);setTimeout(()=>{clearTimeout(t);w.close()},1500)});"
    try:
        version = rig('--version').stdout.strip()
        assert version == '0.6.14 (9d0200f3)', 'This fixture qualifies only the exact installed Rig revision'
        rig('daemon', 'start', '--host', '127.0.0.1', '--port', str(rig_port), '--db', str(rig_home / 'openrig.sqlite'), '--no-kernel')
        daemon_started = True
        spec = fixture / 'rig.json'
        spec.write_text(json.dumps({'version': '0.2', 'name': 'webmux-recovery', 'summary': 'Private recovery qualification; no kernel or agent launch', 'pods': [{'id': 'agents', 'label': 'Recovery', 'members': []}], 'edges': []}))
        created = json.loads(rig('up', str(spec), '--yes', '--json').stdout)
        rig_id = created['rigId']
        registration_file = fixture / 'attached.json'
        launcher = fixture / 'native-launch.py'
        executable(launcher, '#!' + sys.executable + '\nimport os,sys,json,subprocess\n' +
                   'env=dict(os.environ)\nr=subprocess.run(' + repr([str(aliases / 'rig'), 'attach', '--self', '--rig', rig_id, '--pod', 'agents', '--member', 'continuation', '--runtime', 'claude-code', '--cwd', entry['project'], '--json']) + ',env=env,capture_output=True,text=True,check=True)\n' +
                   'open(' + repr(str(registration_file)) + ',"w").write(r.stdout)\nroots=' + repr(roots) + '\nfor key,value in roots.items():\n if value is None: env.pop(key,None)\n else: env[key]=value\n' +
                   'os.chdir(' + repr(entry['project']) + ')\nos.execve(' + repr(claude) + ',' + repr([claude, '--resume', entry['native_session_id'], '--restricted', '--strict-mcp-config', '--tools', '', '--permission-mode', 'plan']) + ',env)\n')
        mux('new-session', '-d', '-s', pane, '-x', '120', '-y', '40', shlex.join([sys.executable, str(launcher)]))
        registration = wait(lambda: json.loads(registration_file.read_text()) if registration_file.exists() else None, 'native Rig enrollment')
        assert registration.get('attachmentType') == 'tmux' and registration.get('sessionId')
        target = registration['env']['OPENRIG_SESSION_NAME']
        scope = hashlib.sha256(b'anonymous').hexdigest()[:24]
        directory = home / 'data/coordination' / scope
        directory.mkdir(parents=True, mode=0o700)
        agent = {'target': target, 'project': 'webmux-recovery', 'rigId': rig_id, 'rigName': 'webmux-recovery', 'pod': 'agents', 'name': 'continuation', 'host': 'localhost', 'cwd': entry['project'], 'runtime': 'claude-code', 'tmux': pane, 'sessionId': registration['sessionId']}
        (directory / (pane + '.agent.json')).write_text(json.dumps(agent))
        def native_ready():
            text = mux('capture-pane', '-pt', pane, '-S', '-200', check=False).stdout
            if 'Make auto mode your default permission mode?' in text:
                assert 'No, keep bypass permissions' in text
                lines = text.splitlines()
                if any(line.lstrip().startswith('❯ Yes,') for line in lines):
                    mux('send-keys', '-t', pane, 'Down', 'Enter')
                elif any(line.lstrip().startswith('❯ No,') for line in lines):
                    mux('send-keys', '-t', pane, 'Enter')
                else:
                    raise RuntimeError('Unknown permission-migration menu; no input sent')
                return None
            return entry['marker'] in text
        wait(native_ready, 'native Claude history')
        before = native()
        assert before['count'] == 1
        start()
        sid = api('/api/coordination/attach', {'provider': 'rig', 'target': target})['id']
        wait(connected, 'managed Rig viewer')
        wait(lambda: entry['marker'] in view()['history'], 'retained native history')
        assert view()['manager_target'] == target and view()['lifecycle_owner'] == 'rig'
        assert not view().get('manager_native_session'), 'Rig enrollment ID was mistaken for a provider-native conversation'
        seats = [seat for seat in api('/api/coordination')['seats'] if seat['provider'] == 'rig' and seat['id'] == target]
        assert len(seats) == 1 and not seats[0].get('nativeSessionId'), 'Coordination inventory mislabeled the Rig enrollment ID as a native CLI conversation'
        receipt('initial native Rig attachment', native=before, preview=view(), registration_session_id=registration['sessionId'], native_session_id=entry['native_session_id'])
        for abrupt in (False, True):
            stop(abrupt)
            assert native() == before
            start()
            wait(connected, 'restored Rig viewer')
            v = wait(lambda: view() if view().get('state', {}).get('disposition') == 'reattached' else None, 'Rig restoration receipt')
            assert native() == before and entry['marker'] in v['history']
            receipt('controller SIGKILL' if abrupt else 'orderly controller restart', native=native(), preview=v)
        for _ in range(3):
            api('/api/sessions/' + sid + '/reconnect', {})
            wait(connected, 'repeated Rig reconnect')
            assert native() == before
        prior, nonce = records(), 'AFTER_RIG_RECOVERY_' + uuid.uuid4().hex
        text = 'What exact marker did I ask you to remember earlier? Reply with the marker followed by ' + nonce + '. Do not use tools.'
        subprocess.run([str(rig_node), '--input-type=module', '-e', bridge], cwd=repo / 'webmux', input=json.dumps({'url': base.replace('http:', 'ws:') + '/api/term/' + sid, 'text': text}), text=True, timeout=12, check=True)
        answer = wait(lambda: next((value for key, value in records().items() if key not in prior and entry['marker'] in value and nonce in value), None), 'new Rig-owned native conversation response')
        receipt('three reconnects and native continuity', native=native(), nonce=nonce, new_assistant_answer=answer)
        stop()
        def kill_private_daemon():
            state = json.loads((rig_home / 'daemon.json').read_text())
            assert state['port'] == rig_port and state['host'] == '127.0.0.1'
            assert Path(state['db']).resolve() == (rig_home / 'openrig.sqlite').resolve()
            with urllib.request.urlopen('http://127.0.0.1:' + str(rig_port) + '/healthz', timeout=5) as response:
                health = json.loads(response.read())
            assert health['pid'] == state['pid'] and state['pid'] > 1
            os.kill(state['pid'], signal.SIGKILL)
            def exited():
                status = subprocess.run(['ps', '-p', str(state['pid']), '-o', 'stat='], capture_output=True, text=True, timeout=5)
                return not status.stdout.strip() or status.stdout.lstrip().startswith('Z')
            wait(exited, 'confirmed private Rig daemon termination')
            assert native() == before
            return state['pid']
        previous_pid = kill_private_daemon()
        start()
        wait(connected, 'automatic private Rig manager recovery')
        replacement = json.loads((rig_home / 'daemon.json').read_text())
        assert replacement['pid'] != previous_pid and native() == before
        receipt('private Rig daemon SIGKILL and automatic manager recovery', native=native(), daemon_pid_before=previous_pid, daemon_pid_after=replacement['pid'], preview=view())
        stop()
        restart_block.write_text('The fixture temporarily refuses manager startup to exercise unavailable-manager fallback.\n')
        previous_pid = kill_private_daemon()
        start()
        v = wait(lambda: view() if view().get('state', {}).get('disposition') == 'managed-unavailable' else None, 'unavailable Rig manager fallback')
        assert entry['marker'] in v['history'] and native() == before
        try:
            api('/api/sessions/' + sid + '/reconnect', {})
        except urllib.error.HTTPError as error:
            assert error.code == 409
        else:
            raise AssertionError('Disconnected Rig manager was bypassed')
        assert native() == before
        receipt('private Rig daemon loss with fixture-denied startup', native=native(), daemon_pid=previous_pid, preview=v)
        stop()
        restart_block.unlink()
        rig('daemon', 'start', '--host', '127.0.0.1', '--port', str(rig_port), '--db', str(rig_home / 'openrig.sqlite'), '--no-kernel')
        replacement = json.loads((rig_home / 'daemon.json').read_text())
        assert replacement['pid'] != previous_pid
        start()
        wait(connected, 'same native conversation after Rig manager recovery')
        assert native() == before and not view().get('state', {}).get('manager_message')
        receipt('private Rig manager recovery after startup refusal', native=native(), daemon_pid=replacement['pid'], preview=view())
        stop()
        mux('kill-session', '-t', '=' + pane)
        assert native()['count'] == 0
        start()
        v = wait(lambda: view() if view().get('state', {}).get('disposition') == 'managed-unavailable' else None, 'lost Rig provider fallback')
        assert native()['count'] == 0 and entry['marker'] in v['history'] and not v['can_resume_managed']
        for _ in range(3):
            try:
                api('/api/sessions/' + sid + '/reconnect', {})
            except urllib.error.HTTPError as error:
                assert error.code == 409
            else:
                raise AssertionError('Lost Rig process silently restarted')
            assert native()['count'] == 0
        receipt('native process loss and repeated manual fallback', native=native(), preview=view())
        assert (hashlib.sha256(settings.read_bytes()).hexdigest() if settings.is_file() else None) == settings_hash
        assert hashlib.sha256(Path(__file__).read_bytes()).hexdigest() == script_hash
        result = {'passed': True, 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'qualifier_sha256': script_hash, 'rig_version': version, 'native_cli': subprocess.check_output([claude, '--version'], text=True).strip(), 'native_session_id': entry['native_session_id'], 'webmux_session_id': sid, 'phases': phases, 'scope': 'Private native Rig adoption, WebMux restart/SIGKILL, repeated reconnect, native conversation continuity, private Rig daemon SIGKILL/restart and lost-provider manual fallback. Not Rig native-resume action, host reboot or a remote destination.'}
        (fixture / 'manifest.json').write_text(json.dumps(result, indent=2) + '\n')
        print(json.dumps({'passed': True, 'manifest': str(fixture / 'manifest.json')}), flush=True)
    except Exception as error:
        (fixture / 'failure-native-pane.txt').write_text(mux('capture-pane', '-pt', pane, '-S', '-200', check=False).stdout)
        if sid and controller and controller.poll() is None:
            try:
                (fixture / 'failure-preview.json').write_text(json.dumps(view(), indent=2) + '\n')
            except (OSError, ValueError):
                pass
        (fixture / 'manifest.json').write_text(json.dumps({'passed': False, 'error': str(error), 'error_type': type(error).__name__, 'phases': phases, 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest()}, indent=2) + '\n')
        print(json.dumps({'passed': False, 'manifest': str(fixture / 'manifest.json')}), flush=True)
        raise
    finally:
        if sid and controller and controller.poll() is None:
            try:
                api('/api/sessions/' + sid, method='DELETE')
            except (OSError, ValueError):
                pass
        stop()
        mux('kill-session', '-t', '=' + pane, check=False)
        if daemon_started:
            stopped = rig('daemon', 'stop', check=False)
            (fixture / 'cleanup.json').write_text(json.dumps({'native': native(), 'daemon_stop_returncode': stopped.returncode, 'daemon_stop_stdout': stopped.stdout, 'daemon_stop_stderr': stopped.stderr}, indent=2) + '\n')
        log.close()


if __name__ == '__main__':
    main()
