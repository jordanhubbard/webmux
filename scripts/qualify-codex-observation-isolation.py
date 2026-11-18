#!/usr/bin/env python3
"""Qualify native macOS observation isolation, optionally with an existing account.

Default cases use private, unauthenticated homes. The optional account check
uses the existing native account in place. No model turn or credential copy.
"""
import argparse
import hashlib
import http.server
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tempfile
import threading
import time
from importlib.util import spec_from_file_location, module_from_spec


def main():
    if sys.platform != 'darwin':
        raise RuntimeError('native_observation_guard_platform_unqualified')
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--codex', required=True)
    parser.add_argument('--node', required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--authenticated-cwd', type=Path,
                        help='Also inspect the existing native account in this project; no turn or credential copy')
    args = parser.parse_args()
    codex, node = [Path(shutil.which(p) or p).resolve() for p in (args.codex, args.node)]
    if subprocess.check_output([str(codex), '--version'], text=True, timeout=5).strip() != 'codex-cli 0.169.0':
        raise RuntimeError('unqualified_native_version')
    spec = spec_from_file_location('startup_controls', Path(__file__).with_name('qualify-codex-observer-startup.py'))
    controls = module_from_spec(spec)
    spec.loader.exec_module(controls)
    module = Path(__file__).resolve().parents[1] / 'webmux/server/internal/openrigcoord/codex-owner-plan.mts'
    source_sha = controls.sha(module)
    root = Path(tempfile.mkdtemp(prefix='wm-owner-os-guard-')).resolve()
    (root / 'mcp.py').write_text(controls.MCP)
    requests = []

    class Handler(http.server.BaseHTTPRequestHandler):
        def request(self):
            if len(requests) >= 64:
                raise RuntimeError('native_http_probe_bound')
            requests.append(self.path)
            self.send_response(404)
            self.end_headers()
        do_POST = request
        do_GET = request
        def log_message(self, *unused):
            pass

    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    report = {'scope': __doc__.strip(), 'cases': [], 'passed': False, 'moduleSHA256': source_sha,
              'nativeSHA256': controls.sha(codex), 'probeSHA256': controls.sha(Path(__file__))}

    def command(executable, argv, stage):
        code = 'import {codexObservationCommand as c} from ' + json.dumps(module.as_uri()) + ';console.log(JSON.stringify(c(...JSON.parse(process.argv[1]))));'
        result = subprocess.check_output([str(node), '--input-type=module', '-e', code,
                                          json.dumps([str(executable), argv, stage])], text=True, timeout=5)
        value = json.loads(result)
        return [value['executable'], *value['args']]

    def start(home, cwd, stage):
        argv = ['-C', str(cwd)]
        for feature in ['hooks', 'plugins', 'apps', 'shell_snapshot']:
            argv.extend(['-c', 'features.' + feature + '=false'])
        argv.extend(['app-server', '--listen', 'stdio://'])
        executable = home / ('start-' + stage)
        executable.write_text('#!/bin/sh\nexec ' + shlex.join(command(codex, argv, stage)) + '\n')
        executable.chmod(0o700)
        native = controls.Native(executable, cwd, home)
        try:
            native.rpc('initialize', {'clientInfo': {'name': 'webmux-os-isolation', 'version': '1'},
                                      'capabilities': {'experimentalApi': True}})
            native.process.stdin.write(b'{"method":"initialized"}\n')
            native.process.stdin.flush()
            return native
        except BaseException:
            native.close()
            raise

    try:
        for mode in ['full', 'read', 'workspace']:
            policies = []
            for guarded in [False, True]:
                home = root / (mode + ('-guarded' if guarded else '-control'))
                home.mkdir(mode=0o700)
                cwd, extra = home / 'project', home / 'extra'
                cwd.mkdir(); extra.mkdir()
                sandbox = {'full': 'danger-full-access', 'read': 'read-only', 'workspace': 'workspace-write'}[mode]
                base = ('cli_auth_credentials_store="file"\napproval_policy="never"\nsandbox_mode=' + json.dumps(sandbox) + '\n'
                        '[sandbox_workspace_write]\nnetwork_access=false\nexclude_slash_tmp=true\nexclude_tmpdir_env_var=true\nwritable_roots=' + json.dumps([str(extra)]) + '\n'
                        '[projects.' + json.dumps(str(cwd)) + ']\ntrust_level="trusted"\n')
                marker, config = home / 'late.marker', home / 'config.toml'
                additions = ('[mcp_servers.late]\ncommand=' + json.dumps(sys.executable) + '\nargs=' + json.dumps([str(root / 'mcp.py'), str(marker)]) + '\n'
                             '[mcp_servers.http]\nurl="http://127.0.0.1:' + str(server.server_address[1]) + '/mcp"\n')
                case = {'mode': mode, 'guarded': guarded}
                report['cases'].append(case)
                # The network-capable account process must not start integrations.
                config.write_text(base + additions)
                before = len(requests)
                account = start(home, cwd, 'account')
                try:
                    account.rpc('config/read', {'cwd': str(cwd), 'includeLayers': True})
                    account.rpc('account/read', {'refreshToken': False})
                    time.sleep(.25)
                    case['accountStartsNoMCP'] = not marker.exists() and len(requests) == before
                    assert case['accountStartsNoMCP']
                    assert 'thread/start' not in account.methods
                finally:
                    account.close()
                    case['accountStopped'] = account.process.poll() is not None
                config.write_text(base)
                native = start(home, cwd, 'policy' if guarded else 'account')
                try:
                    read = native.rpc('config/read', {'cwd': str(cwd), 'includeLayers': True})
                    assert not read['config'].get('mcp_servers')
                    # New names appear after enumeration. OS enforcement must cover them.
                    config.write_text(base + additions)
                    expected_config = config.read_bytes()
                    before = len(requests)
                    thread = native.rpc('thread/start', {'ephemeral': True, 'config': {
                        'features.hooks': False, 'features.plugins': False,
                        'features.apps': False, 'features.shell_snapshot': False}})
                    time.sleep(1)
                    case['lateProcessStarted'] = marker.exists()
                    case['httpRequests'] = len(requests) - before
                    policy = {k: thread.get(k) for k in ['cwd', 'runtimeWorkspaceRoots', 'sandbox', 'approvalPolicy',
                              'approvalsReviewer', 'model', 'modelProvider', 'reasoningEffort', 'serviceTier', 'activePermissionProfile']}
                    case['policy'] = json.loads(json.dumps(policy).replace(str(home), '<fixture>'))
                    policies.append(case['policy'])
                    case['configUnchanged'] = expected_config == config.read_bytes()
                    case['noTurn'] = 'turn/start' not in native.methods
                    assert case['configUnchanged'] and case['noTurn']
                    assert case['lateProcessStarted'] == (not guarded)
                    assert (case['httpRequests'] == 0) if guarded else (case['httpRequests'] > 0)
                finally:
                    native.close()
                    case['policyStopped'] = native.process.poll() is not None
                    if marker.exists():
                        for pid in {json.loads(line)['pid'] for line in marker.read_text().splitlines()}:
                            try:
                                os.kill(pid, 0)
                            except ProcessLookupError:
                                continue
                            raise RuntimeError('owned_mcp_cleanup_unconfirmed')
            assert policies[0] == policies[1], 'native_policy_changed'
        # Initial exec is allowed, but a child using that same binary must fail.
        marker = root / 'child.marker'
        python = Path(sys.executable).resolve()
        child = 'import pathlib;pathlib.Path(' + repr(str(marker)) + ').write_text("started")'
        parent = 'import subprocess,sys;subprocess.run([sys.executable,"-c",' + repr(child) + '],check=True)'
        control = subprocess.run([str(python), '-c', parent], capture_output=True, timeout=5)
        assert control.returncode == 0 and marker.exists()
        marker.unlink()
        denied = subprocess.run(command(python, ['-c', parent], 'policy'), capture_output=True, timeout=5)
        report['sameExecutableForkDenied'] = denied.returncode != 0 and not marker.exists() and b'Operation not permitted' in denied.stderr
        assert report['sameExecutableForkDenied']
        if args.authenticated_cwd:
            marker = root / 'authenticated-mcp.marker'
            bridge = root / 'authenticated.mjs'
            overrides = ['-c', 'mcp_servers.webmux_observer_fixture.command=' + json.dumps(sys.executable),
                         '-c', 'mcp_servers.webmux_observer_fixture.args=' + json.dumps([str(root / 'mcp.py'), str(marker)]),
                         '-c', 'mcp_servers.webmux_observer_http.url="http://127.0.0.1:' + str(server.server_address[1]) + '/mcp"']
            bridge.write_text('import {resolveCodexOwnerPlan,codexObservationCommand} from ' + json.dumps(module.as_uri()) + ';'
                + 'try{const p=await resolveCodexOwnerPlan(' + json.dumps(str(args.authenticated_cwd.resolve()))
                + ',{executable:' + json.dumps(str(codex)) + '},{command:(exe,args,stage)=>codexObservationCommand(exe,['
                + '...' + json.dumps(overrides) + ',...args],stage)});'
                + 'console.log(JSON.stringify({version:p.version,policyType:p.policy.type,approval:p.approval}));}'
                + 'catch(e){console.log(JSON.stringify({error:e.message}));process.exitCode=1;}')
            before = len(requests)
            completed = subprocess.run([str(node), str(bridge)], capture_output=True, text=True, timeout=50)
            observed = json.loads(completed.stdout)
            report['authenticated'] = dict(observed, stdioStarted=marker.exists(), httpRequests=len(requests) - before,
                                           noTurn=True, credentialCopy=False, exit=completed.returncode)
            assert completed.returncode == 0 and not marker.exists() and len(requests) == before
        report['sourceUnchanged'] = controls.sha(module) == source_sha
        assert report['sourceUnchanged']
        report['passed'] = True
    finally:
        server.shutdown(); server.server_close()
        args.output.write_text(json.dumps(report, indent=2) + '\n')
        if report['passed']:
            shutil.rmtree(root)
    print(json.dumps({'passed': report['passed'], 'cases': len(report['cases'])}))


if __name__ == '__main__':
    main()
