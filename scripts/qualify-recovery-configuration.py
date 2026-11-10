#!/usr/bin/env python3
"""Qualify original CLI configuration roots through real WebMux continuation.

Uses a manifest from qualify-recovery-native.py and only its synthetic projects
and conversations. Starts its own controller with deliberately different roots.
Aliases existing native directories without copying credentials. Retains private
receipts; destroys only the fixture's providers and controller.
"""

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shlex
import shutil
import socket
import sqlite3
import subprocess
import struct
import sys
import time
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--cli', choices=['codex', 'claude', 'cursor', 'opencode'], action='append')
    parser.add_argument('--codex-daemon', action='store_true', help='qualify the normal shared-daemon mode instead of the direct-process profile case')
    parser.add_argument('--claude-hooks', action='store_true', help='qualify scoped native SessionStart observations through clear/resume and recovery')
    args = parser.parse_args()
    qualifier_sha256 = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    repo = Path(__file__).resolve().parents[1]
    spec = importlib.util.spec_from_file_location('native_qualification', repo / 'scripts/qualify-recovery-native.py')
    native = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(native)
    entries = json.loads(args.manifest.read_text())
    if isinstance(entries, dict):
        entries = entries['results']
    selected = args.cli or ['codex', 'claude', 'cursor', 'opencode']
    entries = [entry for entry in entries if entry['cli'] in selected]
    assert {entry['cli'] for entry in entries} == set(selected)
    for entry in entries:
        assert entry['conversational_resume'] and entry['marker'].startswith('WM_RECOVERY_')
        assert Path(entry['project']).resolve() == (Path(entry['evidence']) / 'project').resolve()
        assert Path(entry['project']).is_dir()
    fixture = Path.home() / '.local/share/webmux/recovery-qualification' / ('configuration-' + str(uuid.uuid4()))
    fixture.mkdir(mode=0o700, parents=True)
    print(json.dumps({'fixture': str(fixture), 'codex_mode': 'shared-daemon' if args.codex_daemon else 'embedded-profile'}), flush=True)
    home = fixture / 'controller-home'
    shutil.copytree(repo / 'webmux/config.defaults', home / 'config')
    (home / 'config/auth.yaml').write_text('auth:\n  mode: none\n  users: []\n')
    aliases = fixture / 'aliases'
    aliases.mkdir(mode=0o700)
    roots = {
        'codex': {'CODEX_HOME': os.environ.get('CODEX_HOME') or str(Path.home() / '.codex')},
        'claude': {'CLAUDE_CONFIG_DIR': os.environ.get('CLAUDE_CONFIG_DIR') or str(Path.home() / '.claude')},
        'cursor': {'CURSOR_CONFIG_DIR': os.environ.get('CURSOR_CONFIG_DIR') or str(Path(os.environ['XDG_CONFIG_HOME']) / 'cursor' if os.environ.get('XDG_CONFIG_HOME') else Path.home() / '.cursor')},
        'opencode': {key: os.environ.get(key) or str(Path.home() / suffix) for key, suffix in [('XDG_CONFIG_HOME', '.config'), ('XDG_DATA_HOME', '.local/share'), ('XDG_STATE_HOME', '.local/state'), ('XDG_CACHE_HOME', '.cache')]},
    }
    claude_settings_path = Path(roots['claude']['CLAUDE_CONFIG_DIR']) / 'settings.json'
    claude_settings_before = hashlib.sha256(claude_settings_path.read_bytes()).hexdigest() if claude_settings_path.is_file() else None
    unset = {'claude': {'CLAUDE_CONFIG_DIR'} if not os.environ.get('CLAUDE_CONFIG_DIR') else set()}
    for cli in selected:
        for key, target in roots[cli].items():
            link = aliases / (cli + '-' + key)
            source = Path(target).resolve()
            if cli == 'codex' and args.codex_daemon:
                roots[cli][key] = str(target)
                continue
            if cli == 'claude':
                # Claude's macOS keychain identity also depends on its config
                # root. Preserve the already-authenticated original root (or
                # its absence), while the controller uses a different root.
                roots[cli][key] = str(target)
                continue
            if cli == 'codex':
                # These native roots also contain mutable per-root state.
                # Keep fixture-created files in the fixture, and reference the
                # existing files without copying authentication material.
                link.mkdir(mode=0o700)
                for item in source.iterdir():
                    (link / item.name).symlink_to(item, target_is_directory=item.is_dir())
                profile = link / 'webmux-qualification.config.toml'
                assert not profile.exists(), 'fixture profile name collides with an existing profile'
                profile.write_text('sandbox_mode = "read-only"\n')
            else:
                link.symlink_to(source, target_is_directory=True)
            roots[cli][key] = str(link)
    if 'opencode' in selected:
        assert not os.environ.get('OPENCODE_CONFIG_CONTENT') and not os.environ.get('OPENCODE_PERMISSION'), 'inline OpenCode settings require manual restoration'
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        port = listener.getsockname()[1]
    base = 'http://127.0.0.1:' + str(port)
    environment = dict(os.environ, WEBMUX_HOME=str(home), WEBMUX_ROOT='', WEBMUX_COORDINATION_DISABLED='1', SHELL='/bin/bash')
    environment['WEBMUX_RECOVERY_CACHE_HOME'] = str(fixture / 'parent-terminal-cache')
    for key in {key for cli in selected for key in roots[cli]}:
        empty = fixture / ('different-' + key)
        empty.mkdir(mode=0o700, exist_ok=True)
        environment[key] = str(empty)
    binary = args.binary.resolve()
    log = (fixture / 'controller.log').open('ab')
    controller = subprocess.Popen([str(binary), '-listen', '127.0.0.1:' + str(port)], env=environment, stdin=subprocess.DEVNULL, stdout=log, stderr=log)
    session_id = None
    receipts = []
    bridge = """import fs from 'node:fs';import WebSocket from 'ws';
const i=JSON.parse(fs.readFileSync(0,'utf8'));const w=new WebSocket(i.url);
const timer=setTimeout(()=>{w.terminate();process.exitCode=1},7000);
w.on('error',()=>{clearTimeout(timer);process.exitCode=1});
w.on('open',()=>{w.send(JSON.stringify({type:'input',data:i.command}));
setTimeout(()=>w.send(JSON.stringify({type:'input',data:'\\r'})),1000);
setTimeout(()=>{clearTimeout(timer);w.close()},1500)});
"""

    def api(path, data=None, method=None):
        request = urllib.request.Request(base + path, data=json.dumps(data).encode() if data is not None else None, method=method, headers={'Content-Type': 'application/json'})
        with urllib.request.urlopen(request, timeout=35) as response:
            data = response.read()
            return json.loads(data) if data else None

    def wait_for(predicate, label, seconds=90):
        deadline = time.monotonic() + seconds
        last = None
        while time.monotonic() < deadline:
            try:
                value = predicate()
                if value:
                    return value
            except (OSError, ValueError, KeyError, subprocess.SubprocessError) as error:
                last = str(error)
            time.sleep(.25)
        raise RuntimeError('timed out waiting for ' + label + ': ' + str(last))

    def send(command):
        subprocess.run(['node', '--input-type=module', '-e', bridge], cwd=repo / 'webmux', input=json.dumps({'url': base.replace('http:', 'ws:') + '/api/term/' + session_id, 'command': command}), text=True, check=True, timeout=10)

    def records(cli, entry, activity):
        ident = entry['native_session_id']
        result = {}
        if cli in ('codex', 'claude'):
            if cli == 'codex':
                path = Path(activity['native_state_file'])
            else:
                paths = list((Path(roots[cli]['CLAUDE_CONFIG_DIR']) / 'projects').glob('*/' + ident + '.jsonl'))
                if not paths:
                    return result
                assert len(paths) == 1
                path = paths[0]
            for index, line in enumerate(path.read_bytes().splitlines()):
                try:
                    record = json.loads(line)
                except ValueError:
                    continue
                payload = record.get('payload', {}) if cli == 'codex' else record.get('message', {})
                assistant = record.get('type') == 'response_item' and payload.get('role') == 'assistant' if cli == 'codex' else record.get('type') == 'assistant'
                if assistant:
                    result[str(index)] = ''.join(part.get('text', '') for part in payload.get('content', []) if part.get('type') in ('text', 'output_text'))
        elif cli == 'cursor':
            project = str(Path(entry['project']).resolve())
            path = Path(roots[cli]['CURSOR_CONFIG_DIR']) / 'chats' / hashlib.md5(project.encode()).hexdigest() / ident / 'store.db'
            with sqlite3.connect('file:' + str(path) + '?mode=ro', uri=True) as database:
                for blob_id, data in database.execute('select id,data from blobs'):
                    try:
                        record = json.loads(data)
                    except (ValueError, UnicodeError):
                        continue
                    if isinstance(record, dict) and record.get('role') == 'assistant':
                        result[blob_id] = ''.join(part.get('text', '') for part in record.get('content', []) if part.get('type') == 'text')
        else:
            exported = subprocess.run([native.native_binary('opencode'), 'export', ident], cwd=entry['project'], env=dict(os.environ, **roots[cli], PWD=entry['project']), capture_output=True, text=True, check=True, timeout=20)
            for message in json.loads(exported.stdout)['messages']:
                if message['info']['role'] == 'assistant':
                    result[message['info']['id']] = ''.join(part.get('text', '') for part in message.get('parts', []) if part.get('type') == 'text')
        return result

    def daemon_identity():
        control = Path(roots['codex']['CODEX_HOME']) / 'app-server-control/app-server-control.sock'
        with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as peer:
            peer.settimeout(3)
            peer.connect(str(control))
            if sys.platform == 'darwin':
                pid = struct.unpack('i', peer.getsockopt(0, 2, 4))[0]
            elif sys.platform.startswith('linux'):
                pid, uid, _ = struct.unpack('3i', peer.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12))
                assert uid == os.getuid()
            else:
                raise RuntimeError('This shared-daemon qualification requires native Unix peer PID support')
        assert pid > 1
        started = subprocess.check_output(['ps', '-p', str(pid), '-o', 'lstart='], text=True).strip()
        assert started
        return {'pid': pid, 'started_at': started, 'socket': str(control)}

    try:
        wait_for(lambda: api('/api/health')['status'] == 'ok', 'private controller')
        api('/api/config', {'app': {'session_logging': {'enabled': True}}}, 'PUT')
        for entry in entries:
            cli = entry['cli']
            session_id = str(uuid.UUID(api('/api/sessions', {'transport': 'local', 'cols': 120, 'rows': 40})['id']))
            target = 'webmux-' + session_id
            path = '/api/session-recovery/' + session_id
            wait_for(lambda: api(path)['shell_ready'], cli + ' shell readiness')
            ident = entry['native_session_id']
            daemon_before = daemon_identity() if cli == 'codex' and args.codex_daemon else None
            options = {'codex': ['codex', 'resume', ident, '--sandbox', 'read-only', '--no-daemon', '--no-alt-screen'], 'claude': ['claude', '--resume', ident, '--safe-mode', '--tools', ''], 'cursor': ['cursor-agent', '--resume', ident, '--mode', 'ask', '--sandbox', 'enabled'], 'opencode': ['opencode', '--session', ident, '--pure', '--agent', 'plan']}[cli]
            if cli == 'codex' and args.codex_daemon:
                options.remove('--no-daemon')
            elif cli == 'codex':
                options.extend(['--profile', 'webmux-qualification'])
            if cli == 'claude' and args.claude_hooks:
                options = ['claude', '--resume', ident, '--restricted', '--strict-mcp-config', '--tools', '', '--permission-mode', 'plan']
            settings = ['-u ' + key for key in unset.get(cli, set())]
            settings.extend(shlex.quote(key + '=' + value) for key, value in roots[cli].items() if key not in unset.get(cli, set()))
            command = 'cd ' + shlex.quote(entry['project']) + ' && env ' + ' '.join(settings) + ' ' + shlex.join(options)
            send(command)

            phase = 'initial'
            hook_choices = set()

            def running():
                if cli == 'claude' and args.claude_hooks:
                    pane = subprocess.run(['tmux', 'capture-pane', '-pt', target, '-S', '-200'], capture_output=True, text=True)
                    if 'Make auto mode your default permission mode?' in pane.stdout:
                        if 'No, keep bypass permissions' not in pane.stdout:
                            raise RuntimeError('Unknown native permission migration dialog; no choice submitted')
                        if '❯ Yes' in pane.stdout or '❯ 1.' in pane.stdout:
                            subprocess.run(['tmux', 'send-keys', '-t', target, 'Down', 'Enter'], check=True)
                        elif '❯ No' in pane.stdout or '❯ 2.' in pane.stdout:
                            subprocess.run(['tmux', 'send-keys', '-t', target, 'Enter'], check=True)
                        else:
                            raise RuntimeError('Native migration selection is unknown; no choice submitted')
                if cli == 'codex' and phase not in hook_choices:
                    pane = subprocess.run(['tmux', 'capture-pane', '-pt', target], capture_output=True, text=True)
                    if 'Hooks need review' in pane.stdout and 'Continue without trusting' in pane.stdout:
                        # The fixture's new configuration root can require a
                        # fresh native trust decision. Continue with hooks off;
                        # do not grant trust or use a bypass flag for the test.
                        subprocess.run(['tmux', 'send-keys', '-t', target, 'Down', 'Down', 'Enter'], check=True)
                        hook_choices.add(phase)
                activity = api(path).get('state', {}).get('activity', {})
                if cli == 'claude' and args.claude_hooks and activity.get('native_evidence') != 'native-hook':
                    return None
                return activity if activity.get('running') and activity.get('native_session_id') == ident and (cli == 'opencode' or activity.get('native_state_file')) else None

            switches = []
            before = wait_for(running, cli + ' native identity')
            if cli == 'claude' and args.claude_hooks:
                def native_observed():
                    activity = running()
                    return activity if activity and activity.get('native_evidence') == 'native-hook' else None
                before = wait_for(native_observed, 'Claude authenticated native callback')
                def native_history(marker):
                    running()  # Handle only the fixture's known native migration dialog.
                    pane = subprocess.run(['tmux', 'capture-pane', '-pt', target, '-S', '-200'], capture_output=True, text=True)
                    return marker in pane.stdout
                wait_for(lambda: native_history(entry['marker']), 'Claude original conversation ready')
                send('/clear')
                def changed_conversation():
                    activity = api(path).get('state', {}).get('activity', {})
                    current = activity.get('native_session_id')
                    return activity if current and current != ident and activity.get('native_evidence') == 'native-hook' else None
                cleared = wait_for(changed_conversation, 'Claude clear native callback')
                switched_id = str(uuid.UUID(cleared['native_session_id']))
                switched_marker = 'WM_RECOVERY_SWITCH_' + uuid.uuid4().hex
                switched_entry = dict(entry, native_session_id=switched_id, marker=switched_marker)
                send('Remember this exact marker for later in this conversation: ' + switched_marker + '. Reply only with this marker. Do not use tools.')
                def switched_answer():
                    return next((text for text in records(cli, switched_entry, cleared).values() if switched_marker in text), None)
                wait_for(switched_answer, 'Claude fresh cleared-conversation response')
                switches.append({'action': 'clear', 'native_session_id': switched_id, 'observation': cleared})
                (fixture / 'claude-switches.json').write_text(json.dumps(switches, indent=2) + '\n')
                send('/resume ' + ident)
                wait_for(lambda: running() if running() and running().get('native_evidence') == 'native-hook' else None, 'Claude in-process original resume callback')
                wait_for(lambda: native_history(entry['marker']), 'Claude original history after in-process resume')
                switches.append({'action': 'resume-original', 'native_session_id': ident, 'observation': running()})
                (fixture / 'claude-switches.json').write_text(json.dumps(switches, indent=2) + '\n')
                send('/resume ' + switched_id)
                entry = switched_entry
                ident = switched_id
                before = wait_for(running, 'Claude in-process switched conversation callback')
                wait_for(lambda: native_history(switched_marker), 'Claude switched history after in-process resume')
                switches.append({'action': 'resume-switched', 'native_session_id': switched_id, 'observation': before})
                (fixture / 'claude-switches.json').write_text(json.dumps(switches, indent=2) + '\n')

            expected_evidence = 'process-state' if cli == 'codex' and not args.codex_daemon else 'launch-selector'
            if cli == 'claude' and args.claude_hooks:
                expected_evidence = 'native-hook'
            if daemon_before:
                # Let both the fast first observation and the later owned-FD
                # observation run. Do not qualify from the initial selector alone.
                time.sleep(6)
                before = running()
                assert before
                assert daemon_identity() == daemon_before

            assert before.get('native_evidence') == expected_evidence, 'initial native identity provenance is missing or incorrectly promoted'
            assert all(key not in before['configuration']['roots'] if key in unset.get(cli, set()) else before['configuration']['roots'].get(key) == value for key, value in roots[cli].items())
            prior = records(cli, entry, before)
            subprocess.run(['tmux', 'kill-session', '-t', target], check=True)
            wait_for(lambda: api('/api/sessions/' + session_id)['state'] == 'disconnected', cli + ' provider exit')
            api('/api/sessions/' + session_id + '/reconnect', {})
            def waiting():
                value = api(path)
                return value if value['waiting'] else None
            preview = wait_for(waiting, cli + ' recovery waiting')
            action = {'id': str(uuid.uuid4()), 'kind': 'native', 'revision': preview['state']['revision'], 'host_revision': preview['host_revision'], 'instance': preview['instance']}
            (fixture / (cli + '-action.json')).write_text(json.dumps({'preview': preview, 'action': action}, indent=2) + '\n')
            assert api(path, action)['outcome'] == 'consumed'
            phase = 'recovered'
            def resumed_running():
                activity = running()
                return activity if activity and activity.get('shell_instance') == preview['instance'] else None
            after = wait_for(resumed_running, cli + ' resumed native identity from the new worker')
            assert after.get('native_evidence') == expected_evidence, 'native continuation lost its identity provenance'
            assert all(key not in after['configuration']['roots'] if key in unset.get(cli, set()) else after['configuration']['roots'].get(key) == value for key, value in roots[cli].items())
            expected_argv = list(before['argv'])
            if cli == 'claude' and args.claude_hooks:
                expected_argv[expected_argv.index('--resume') + 1] = ident
            assert after['argv'] == expected_argv, 'native continuation dropped original invocation restrictions or profile'
            nonce = 'AFTER_CONFIGURATION_RECOVERY_' + uuid.uuid4().hex
            # Native TUI startup is distinct from its wrapper's identity event.
            wait_for(lambda: entry['marker'] in subprocess.run(['tmux', 'capture-pane', '-pt', target, '-S', '-200'], capture_output=True, text=True, check=True).stdout, cli + ' native history rendering')
            send('What exact marker did I ask you to remember earlier in this conversation? Reply with that marker followed by ' + nonce + '. Do not use tools.')

            def new_answer():
                return next((text for key, text in records(cli, entry, after).items() if key not in prior and entry['marker'] in text and nonce in text), None)

            answer = wait_for(new_answer, cli + ' new native assistant response')
            daemon_after = daemon_identity() if daemon_before else None
            if daemon_before:
                assert daemon_after == daemon_before, 'shared daemon identity changed during continuation'
                time.sleep(6)
                after = running()
                assert after and after.get('native_evidence') == 'launch-selector', 'shared-daemon case was incorrectly promoted to observed process state'

            assert api(path, action)['outcome'] == 'consumed'
            receipt = {'cli': cli, 'passed': True, 'session_id': session_id, 'native_session_id': ident, 'before': before, 'after': after, 'nonce': nonce, 'new_assistant_answer': answer, 'controller_ambient_roots': {key: environment[key] for key in roots[cli]}, 'native_hooks_left_untrusted': sorted(hook_choices), 'in_process_switches': switches, 'shared_daemon_before': daemon_before, 'shared_daemon_after': daemon_after, 'scope': 'Explicit selected conversation continuation; interactive switches are not verified' if daemon_before else 'Original root and profile continuation'}
            receipts.append(receipt)
            (fixture / (cli + '-receipt.json')).write_text(json.dumps(receipt, indent=2) + '\n')
            print(json.dumps({'cli': cli, 'passed': True, 'fixture': str(fixture)}), flush=True)
            api('/api/sessions/' + session_id, method='DELETE')
            subprocess.run(['tmux', 'kill-session', '-t', target], capture_output=True)
            session_id = None
        if args.claude_hooks:
            claude_settings_after = hashlib.sha256(claude_settings_path.read_bytes()).hexdigest() if claude_settings_path.is_file() else None
            assert claude_settings_after == claude_settings_before, 'native Claude permission settings changed during the fixture'
        assert hashlib.sha256(Path(__file__).read_bytes()).hexdigest() == qualifier_sha256, 'qualification source changed during the native run'
        (fixture / 'manifest.json').write_text(json.dumps({'passed': True, 'qualifier_sha256': qualifier_sha256, 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'receipts': receipts}, indent=2) + '\n')
        print(json.dumps({'passed': True, 'manifest': str(fixture / 'manifest.json')}), flush=True)
    except Exception as error:
        # Keep a failing native case reviewable before cleaning up its provider.
        # Never infer success from a partially populated manifest or old output.
        activity = None
        if session_id:
            try:
                activity = api('/api/session-recovery/' + session_id).get('state', {}).get('activity')
            except (OSError, ValueError, AttributeError):
                pass
        failure = {'passed': False, 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
                   'codex_mode': 'shared-daemon' if args.codex_daemon else 'embedded-profile',
                   'error': str(error), 'error_type': type(error).__name__,
                   'session_id': session_id, 'last_activity': activity, 'receipts': receipts}
        (fixture / 'manifest.json').write_text(json.dumps(failure, indent=2) + '\n')
        print(json.dumps({'passed': False, 'manifest': str(fixture / 'manifest.json')}), flush=True)
        raise
    finally:
        try:
            if session_id:
                try:
                    api('/api/sessions/' + session_id, method='DELETE')
                finally:
                    subprocess.run(['tmux', 'kill-session', '-t', 'webmux-' + session_id], capture_output=True)
        finally:
            controller.terminate()
            try:
                controller.wait(timeout=10)
            except subprocess.TimeoutExpired:
                controller.kill()
                controller.wait(timeout=5)
            log.close()


if __name__ == '__main__':
    main()
