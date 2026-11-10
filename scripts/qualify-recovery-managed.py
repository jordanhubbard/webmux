#!/usr/bin/env python3
"""Qualify real Agent Deck / Claude continuity through private WebMux lifecycles.

Requires an existing synthetic manifest from qualify-recovery-native.py and
Agent Deck 1.16.34. Uses a private manager home/profile and tmux socket, restores
only native CLI path variables in its child, and references only the synthetic
conversation log. Authentication material is never copied. Starts no other
agents; cleanup stops only the fixture provider and controller.

This checks native provider-owned continuation, not a WebMux managed-resume
action or host reboot. Failed phases remain negative qualification evidence.
"""
import argparse
import sys
import hashlib
import json
import os
import pathlib
import shutil
import socket
import subprocess
import time
import urllib.error
import urllib.request
import uuid


def prepare(manifest):
    entries = json.loads(manifest.read_text())
    if isinstance(entries, dict):
        entries = entries['results']
    entry = next((e for e in entries if e['cli'] == 'claude'))
    assert entry['conversational_resume'] and entry['marker'].startswith('WM_RECOVERY_')
    assert pathlib.Path(entry['project']).resolve() == (pathlib.Path(entry['evidence']) / 'project').resolve()
    uuid.UUID(entry['native_session_id'])
    deck_binary = shutil.which('agent-deck')
    claude_binary = shutil.which('claude', path=os.environ.get('WEBMUX_BROWSER_ORIGINAL_PATH') or os.environ.get('PATH'))
    assert deck_binary and claude_binary
    version = subprocess.run([deck_binary, '--help'], capture_output=True, text=True, timeout=15)
    assert 'Agent Deck v1.16.34' in version.stdout, 'This qualification harness requires Agent Deck 1.16.34'
    native_version = subprocess.check_output([claude_binary, '--version'], text=True, timeout=15).strip()
    p = pathlib.Path.home() / '.local/share/webmux/recovery-qualification' / ('agent-deck-native-' + str(uuid.uuid4()))
    p.mkdir(mode=0o700, parents=True)
    fixture_home = p / 'home'
    fixture_home.mkdir(mode=0o700)
    roots = {k: os.environ.get(k) for k in ['HOME', 'CLAUDE_CONFIG_DIR', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME']}
    env = dict(os.environ)
    for k in list(env):
        if k.startswith(('WEBMUX_RECOVERY_', 'AGENTDECK_', 'AGENT_DECK_')) or k in ('CLAUDE_CONFIG_DIR', 'TMUX', 'TMUX_PANE'):
            env.pop(k, None)
    env.update(HOME=str(fixture_home), SHELL='/bin/bash', AGENTDECK_PROFILE='default', DO_NOT_TRACK='1')
    for k in ['XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME']:
        path = p / k.lower()
        path.mkdir(mode=0o700)
        env[k] = str(path)
    wrapper = p / 'claude-native'
    wrapper.write_text('#!' + sys.executable + '\nimport os,sys\nroots=' + repr(roots) + '\nfor k,v in roots.items():\n if v is None: os.environ.pop(k,None)\n else: os.environ[k]=v\nos.execv(' + repr(claude_binary) + ',[' + repr(claude_binary) + ',*sys.argv[1:]])\n')
    wrapper.chmod(0o700)
    config = pathlib.Path(env['XDG_CONFIG_HOME']) / 'agent-deck'
    config.mkdir(mode=0o700)
    socket_name = 'wm-recovery-' + uuid.uuid4().hex[:12]
    (config / 'config.toml').write_text('[claude]\ncommand = ' + json.dumps(str(wrapper)) + '\ndangerous_mode = false\nhooks_enabled = false\n[security]\nauto_trust_folders = false\n[launch]\ninject_identity = false\n[tmux]\nsocket_name = ' + json.dumps(socket_name) + '\n')
    settings = pathlib.Path.home() / '.claude/settings.json'
    settings_sha = hashlib.sha256(settings.read_bytes()).hexdigest() if settings.exists() else None
    command = [deck_binary, 'add', entry['project'], '-c', 'claude', '--resume-session', entry['native_session_id'], '--no-identity', '--no-parent', '--tmux-socket', socket_name, '--extra-arg=--safe-mode', '--extra-arg=--tools', '--extra-arg=', '--extra-arg=--permission-mode', '--extra-arg=plan', '--title', 'WebMux recovery native fixture', '--json']
    created = subprocess.run(command, env=env, capture_output=True, text=True, timeout=30)
    (p / 'add.stdout').write_text(created.stdout)
    (p / 'add.stderr').write_text(created.stderr)
    if created.returncode:
        raise RuntimeError('Cannot create private Agent Deck fixture: ' + created.stderr[:300])
    provider = json.loads(created.stdout)
    ident = provider['id']
    changed = subprocess.run([deck_binary, 'session', 'set', ident, 'command', str(wrapper), '--json'], env=env, capture_output=True, text=True, timeout=20)
    assert changed.returncode == 0, changed.stderr
    shown = subprocess.run([deck_binary, 'session', 'show', ident, '--json'], env=env, capture_output=True, text=True, timeout=20)
    assert shown.returncode == 0, shown.stderr
    (p / 'before-start.json').write_text(shown.stdout)
    native_root = pathlib.Path(roots['CLAUDE_CONFIG_DIR']) if roots['CLAUDE_CONFIG_DIR'] else pathlib.Path.home() / '.claude'
    files = list((native_root / 'projects').glob('*/' + entry['native_session_id'] + '.jsonl'))
    assert len(files) == 1
    target = fixture_home / '.claude/projects' / files[0].parent.name
    target.mkdir(mode=0o700, parents=True)
    (target / files[0].name).symlink_to(files[0])
    data = {'fixture': str(p), 'environment_roots': {k: env[k] for k in ['HOME', 'SHELL', 'AGENTDECK_PROFILE', 'XDG_CONFIG_HOME', 'XDG_DATA_HOME', 'XDG_STATE_HOME', 'XDG_CACHE_HOME']}, 'entry': entry, 'socket': socket_name, 'settings_sha256_before': settings_sha, 'provider_session': provider, 'deck_binary': deck_binary, 'native_version': native_version, 'native_state_file': str(files[0])}
    (p / 'fixture.json').write_text(json.dumps(data, indent=2) + '\n')
    (p / 'fixture.json').chmod(0o600)
    print(json.dumps({'fixture': str(p), 'provider': 'agent-deck', 'native_cli': native_version}), flush=True)
    return (p, data)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=pathlib.Path, required=True)
    parser.add_argument('--manifest', type=pathlib.Path, required=True)
    args = parser.parse_args()
    qualifier_sha256 = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
    assert args.binary.is_file()
    repo = pathlib.Path(__file__).resolve().parents[1]
    p, d = prepare(args.manifest)
    entry = d['entry']
    ident = d['provider_session']['id']
    pane = json.loads((p / 'before-start.json').read_text())['tmux_session']
    env = dict(os.environ, **d['environment_roots'], DO_NOT_TRACK='1', WEBMUX_COORDINATION_DISABLED='0', WEBMUX_ROOT='')
    for k in list(env):
        if k.startswith('WEBMUX_RECOVERY_') or k in ('AGENTDECK_ACCOUNT', 'AGENTDECK_INSTANCE_ID', 'AGENT_DECK_SESSION_ID', 'CLAUDE_CONFIG_DIR'):
            env.pop(k, None)
    home = p / 'webmux-home'
    shutil.copytree(repo / 'webmux/config.defaults', home / 'config', dirs_exist_ok=True)
    (home / 'config/auth.yaml').write_text('auth:\n  mode: none\n  users: []\n')
    (home / 'config/app.yaml').write_text('app:\n  session_logging:\n    enabled: true\n  coordination:\n    enabled_providers: [agent-deck]\n    default_launch_provider: webmux\n    default_view: all\n')
    env['WEBMUX_HOME'] = str(home)
    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
        port = s.getsockname()[1]
    base = 'http://127.0.0.1:' + str(port)
    binary = args.binary.resolve()
    controller = None
    sid = None
    phases = []
    log = (p / 'controller.log').open('ab')
    bridge = "import fs from 'node:fs';import WebSocket from 'ws';const i=JSON.parse(fs.readFileSync(0,'utf8'));const w=new WebSocket(i.url);const t=setTimeout(()=>{w.terminate();process.exitCode=1},7000);w.on('error',()=>{clearTimeout(t);process.exitCode=1});w.on('open',()=>{w.send(JSON.stringify({type:'input',data:i.text}));setTimeout(()=>w.send(JSON.stringify({type:'input',data:'\\r'})),1000);setTimeout(()=>{clearTimeout(t);w.close()},1500)});"

    def api(path, data=None, method=None):
        r = urllib.request.Request(base + path, data=json.dumps(data).encode() if data is not None else None, method=method, headers={'Content-Type': 'application/json'})
        with urllib.request.urlopen(r, timeout=40) as response:
            b = response.read()
            return json.loads(b) if b else None

    def wait(fn, label, seconds=60):
        end = time.monotonic() + seconds
        last = None
        while time.monotonic() < end:
            try:
                result = fn()
                if result:
                    return result
            except (OSError, ValueError, KeyError) as e:
                last = str(e)
            time.sleep(0.25)
        raise RuntimeError('timeout ' + label + ': ' + str(last))

    def deck(*args, check=True):
        r = subprocess.run([d['deck_binary'], *args], env=env, capture_output=True, text=True, timeout=35)
        with (p / 'provider-events.jsonl').open('a') as f:
            f.write(json.dumps({'args': args, 'returncode': r.returncode, 'stdout': r.stdout, 'stderr': r.stderr}) + '\n')
        if check and r.returncode:
            raise RuntimeError('Agent Deck ' + args[0] + ' failed: ' + r.stderr[:200])
        return json.loads(r.stdout) if r.returncode == 0 and r.stdout.strip().startswith('{') else r.returncode

    def native():
        r = subprocess.run(['tmux', '-L', d['socket'], 'list-panes', '-a', '-F', '#{session_name} #{pane_pid} #{pane_current_command}'], capture_output=True, text=True)
        rows = [x.split() for x in r.stdout.splitlines() if x.startswith(pane + ' ')]
        assert len(r.stdout.splitlines()) == len(rows), 'an unexpected additional managed pane was created'
        assert len(rows) <= 1
        return {'count': len(rows), 'pane_pid': int(rows[0][1]) if rows else None}

    def start():
        nonlocal controller
        controller = subprocess.Popen([str(binary), '-listen', '127.0.0.1:' + str(port)], env=env, stdin=subprocess.DEVNULL, stdout=log, stderr=log)
        wait(lambda: api('/api/health')['status'] == 'ok', 'controller ready')

    def stop(kill=False):
        nonlocal controller
        if controller is None:
            return
        if controller.poll() is None:
            controller.kill() if kill else controller.terminate()
            try:
                controller.wait(timeout=15)
            except subprocess.TimeoutExpired:
                controller.kill()
                controller.wait(timeout=5)
        controller = None

    def view():
        return api('/api/session-recovery/' + sid)

    def connected():
        v = api('/api/sessions/' + sid)
        clients = subprocess.run(['tmux', '-L', d['socket'], 'list-clients', '-F', '#{session_name}'], capture_output=True, text=True)
        return v if v['state'] == 'connected' and clients.stdout.splitlines().count(pane) == 1 else None

    def native_ready():
        capture = subprocess.run(['tmux', '-L', d['socket'], 'capture-pane', '-pt', pane, '-S', '-200'], capture_output=True, text=True)
        text = capture.stdout
        if 'Make auto mode your default permission mode?' in text:
            # Decline a global-default migration in this synthetic fixture.
            # The native child already has explicit plan mode and no tools.
            if 'No, keep bypass permissions' not in text:
                raise RuntimeError('Unrecognized native permission-default dialog; no choice was submitted')
            if any(line.lstrip().startswith('❯ Yes,') for line in text.splitlines()):
                keys = ['Down', 'Enter']
            elif any(line.lstrip().startswith('❯ No,') for line in text.splitlines()):
                keys = ['Enter']
            else:
                raise RuntimeError('Cannot verify native dialog selection; no choice was submitted')
            subprocess.run(['tmux', '-L', d['socket'], 'send-keys', '-t', pane, *keys], check=True)
            with (p / 'native-ui-decisions.jsonl').open('a') as f:
                f.write(json.dumps({'decision': 'decline global permission-mode migration'}) + '\n')
            return None
        return entry['marker'] in text

    def records():
        f = pathlib.Path(d['native_state_file'])
        result = {}
        for idx, line in enumerate(f.read_bytes().splitlines()):
            try:
                r = json.loads(line)
            except ValueError:
                continue
            if r.get('type') == 'assistant':
                result[str(idx)] = ''.join((c.get('text', '') for c in r.get('message', {}).get('content', []) if c.get('type') == 'text'))
        return result

    def answer(label):
        wait(native_ready, 'native composer readiness')
        prior = records()
        nonce = 'AFTER_MANAGED_RECOVERY_' + uuid.uuid4().hex
        text = 'What exact marker did I ask you to remember earlier in this conversation? Reply with that marker followed by ' + nonce + '. Do not use tools.'
        subprocess.run(['node', '--input-type=module', '-e', bridge], cwd=repo / 'webmux', input=json.dumps({'url': base.replace('http:', 'ws:') + '/api/term/' + sid, 'text': text}), text=True, timeout=12, check=True)
        fresh = wait(lambda: next((s for k, s in records().items() if k not in prior and entry['marker'] in s and (nonce in s)), None), label + ' new native answer', 90)
        return {'nonce': nonce, 'new_assistant_answer': fresh}

    def receipt(label, **kw):
        row = {'phase': label, **kw}
        phases.append(row)
        (p / 'lifecycle-phases.json').write_text(json.dumps(phases, indent=2) + '\n')
        print(json.dumps({'phase': label, 'passed': True}), flush=True)
    try:
        assert native()['count'] == 0
        deck('session', 'start', ident, '--json')
        before = wait(lambda: native() if native()['count'] == 1 else None, 'native provider start')
        wait(native_ready, 'original native conversation rendering')
        start()
        s = api('/api/coordination/attach', {'provider': 'agent-deck', 'target': 'local:' + ident})
        sid = s['id']
        wait(connected, 'initial viewer')
        wait(lambda: entry['marker'] in view()['history'], 'original native history')
        assert view()['manager_target'] == 'agent-deck:local:' + ident
        assert view()['manager_native_session'] == entry['native_session_id']
        assert view()['manager_native_state_file'] and view()['manager_native_source'] == 'agent-deck-inventory'
        assert view()['manager_native_observed_at'] and view()['manager_profile'] == 'default'
        receipt('initial managed attachment', native=before, webmux_session_id=sid, preview=view())
        for abrupt in (False, True):
            stop(kill=abrupt)
            assert native() == before
            start()
            wait(connected, 'restored viewer')
            v = wait(lambda: view() if view().get('state', {}).get('disposition') == 'reattached' else None, 'managed restoration receipt')
            assert native() == before and entry['marker'] in v['history']
            assert v['manager_native_session'] == entry['native_session_id'] and v['manager_native_state_file']
            receipt('SIGKILL' if abrupt else 'orderly restart', native=native(), preview=v)
        for _ in range(3):
            api('/api/sessions/' + sid + '/reconnect', {})
            wait(connected, 'explicit reconnect')
            assert native() == before
        receipt('repeated reconnects', native=native(), conversation=answer('surviving managed conversation'))
        stop()
        deck('session', 'stop', ident, '--json')
        assert native()['count'] == 0
        start()
        v = wait(lambda: view() if view().get('state', {}).get('disposition') == 'managed-unavailable' else None, 'lost provider fallback')
        assert native()['count'] == 0 and entry['marker'] in v['history']
        try:
            api('/api/sessions/' + sid + '/reconnect', {})
        except urllib.error.HTTPError as error:
            assert error.code == 409
        else:
            raise AssertionError('missing managed process was silently restarted')
        receipt('native process loss fallback', native=native(), preview=v)
        assert v['can_resume_managed'] and v['instance']
        action = {'id': str(uuid.uuid4()), 'kind': 'managed', 'revision': v['state']['revision'],
                  'host_revision': 0, 'instance': v['instance'], 'confirm': True}
        assert api('/api/session-recovery/' + sid, action)['outcome'] == 'consumed'
        after = wait(lambda: native() if native()['count'] == 1 else None, 'provider-owned native start')
        assert after['pane_pid'] != before['pane_pid']
        wait(connected, 'new native process attachment')
        assert api('/api/session-recovery/' + sid, action)['outcome'] == 'consumed'
        assert native() == after
        stop()
        start()
        wait(connected, 'consumed managed recovery receipt after restart')
        assert api('/api/session-recovery/' + sid, action)['outcome'] == 'consumed'
        assert native() == after
        wait(lambda: entry['marker'] in view()['history'], 'native resumed history')
        native_show = deck('session', 'show', ident, '--json')
        assert native_show['claude_session_id'] == entry['native_session_id']
        receipt('WebMux managed native recovery and durable duplicate receipt', native=after, conversation=answer('provider resumed native conversation'))
        settings = pathlib.Path.home() / '.claude/settings.json'
        assert (hashlib.sha256(settings.read_bytes()).hexdigest() if settings.exists() else None) == d['settings_sha256_before']
        assert hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest() == qualifier_sha256, 'qualification source changed during the native run'
        result = {'passed': True, 'qualifier_sha256': qualifier_sha256, 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'provider': 'agent-deck', 'provider_version': '1.16.34', 'native_cli': 'claude', 'native_cli_version': d['native_version'], 'native_session_id': entry['native_session_id'], 'provider_session_id': ident, 'webmux_session_id': sid, 'phases': phases, 'scope': 'Real controller restart, SIGKILL, repeated reconnect, native process loss fallback and explicit provider-owned native continuation. Includes WebMux managed-resume action and duplicate request after restart. Not host reboot.'}
        (p / 'lifecycle-manifest.json').write_text(json.dumps(result, indent=2) + '\n')
        print(json.dumps({'passed': True, 'manifest': str(p / 'lifecycle-manifest.json')}), flush=True)
    except Exception as e:
        capture = subprocess.run(['tmux', '-L', d['socket'], 'capture-pane', '-pt', pane, '-S', '-200'], capture_output=True, text=True)
        (p / 'failure-native-pane.txt').write_text(capture.stdout)
        if sid:
            try:
                (p / 'failure-preview.json').write_text(json.dumps(view(), indent=2) + '\n')
            except (OSError, ValueError):
                pass
        (p / 'lifecycle-failure.json').write_text(json.dumps({'passed': False, 'error': str(e), 'phases': phases}, indent=2) + '\n')
        print(json.dumps({'passed': False, 'fixture': str(p), 'error': str(e)}), flush=True)
        raise
    finally:
        stop()
        deck('session', 'stop', ident, '--json', check=False)
        subprocess.run(['tmux', '-L', d['socket'], 'kill-session', '-t', pane], capture_output=True)
        log.close()
if __name__ == '__main__':
    main()
