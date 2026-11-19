#!/usr/bin/env python3
"""Native Codex 0.169 startup controls in disposable homes; never submits a turn."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import selectors
import shutil
import signal
import subprocess
import tempfile
import time


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


class Native:
    def __init__(self, executable, cwd, home, *, diagnostics=None):
        # A fresh HOME and CODEX_HOME keep all account/configuration state private.
        env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"), "HOME": str(home),
               "CODEX_HOME": str(home), "LANG": "en_US.UTF-8"}
        # Opt in only for the disposable, unauthenticated fixture homes.
        # Other callers retain the old discard behavior.
        self.diagnostics = diagnostics
        self.stderr_tail = bytearray()
        self.stderr_bytes = 0
        self.stderr_panic = False
        self.stderr_eof = False
        self.stdout_closed = False
        self.closed = False
        try:
            self.process = subprocess.Popen(
                [str(executable), "-C", str(cwd), "app-server", "--listen", "stdio://"],
                cwd=cwd, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                stderr=subprocess.PIPE if diagnostics is not None else subprocess.DEVNULL, start_new_session=True)
        except OSError as error:
            if diagnostics is not None:
                diagnostics.update({'launchErrorType': type(error).__name__, 'launchErrno': error.errno})
            raise
        self.selector = selectors.DefaultSelector()
        self.selector.register(self.process.stdout, selectors.EVENT_READ, 'stdout')
        if diagnostics is not None:
            os.set_blocking(self.process.stderr.fileno(), False)
            self.selector.register(self.process.stderr, selectors.EVENT_READ, 'stderr')
        self.buffer = b""
        self.sequence = 0
        self.methods = []

    def drain_stderr(self):
        if self.diagnostics is None or self.stderr_eof:
            return
        # Bound each drain so a continuously noisy child cannot starve RPC's
        # existing deadline; retain only a 32 KiB tail while draining the pipe.
        for _ in range(8):
            try:
                data = os.read(self.process.stderr.fileno(), 65536)
            except BlockingIOError:
                return
            if not data:
                self.stderr_eof = True
                self.selector.unregister(self.process.stderr)
                return
            self.stderr_bytes += len(data)
            self.stderr_tail.extend(data)
            self.stderr_panic |= b'panicked at' in self.stderr_tail.lower()
            del self.stderr_tail[:-32768]

    def rpc(self, method, params):
        self.sequence += 1
        request_id = self.sequence
        self.methods.append(method)
        self.process.stdin.write((json.dumps({"id": request_id, "method": method,
                                             "params": params}) + "\n").encode())
        self.process.stdin.flush()
        deadline = time.monotonic() + 12
        while time.monotonic() < deadline:
            while b"\n" in self.buffer:
                line, self.buffer = self.buffer.split(b"\n", 1)
                value = json.loads(line)
                if value.get("id") == request_id:
                    if "error" in value:
                        raise RuntimeError("native_rpc_refused_" + method)
                    return value["result"]
                if "id" in value and "method" in value:
                    raise RuntimeError("native_server_request_unanswered")
            for key, _ in self.selector.select(.1):
                if key.data == 'stderr':
                    self.drain_stderr()
                    continue
                data = os.read(self.process.stdout.fileno(), 65536)
                if not data:
                    self.stdout_closed = True
                    self.drain_stderr()
                    raise RuntimeError("native_stdout_closed")
                self.buffer += data
                if len(self.buffer) > 2 * 1024 * 1024:
                    raise RuntimeError("native_response_bound")
        raise TimeoutError("native_rpc_deadline_" + method)

    def close(self):
        if self.closed:
            return
        original_exit = self.process.poll()
        # After stdout EOF, briefly reap a naturally exiting child before
        # cleanup signals can obscure its original status. No RPC is retried
        # and its twelve-second deadline is unchanged.
        if self.diagnostics is not None and self.stdout_closed and original_exit is None:
            try:
                original_exit = self.process.wait(timeout=.05)
            except subprocess.TimeoutExpired:
                pass
        term_sent = killed = False
        try:
            try:
                os.killpg(self.process.pid, signal.SIGTERM)
                term_sent = True
            except ProcessLookupError:
                pass
            deadline = time.monotonic() + 3
            while self.process.poll() is None and time.monotonic() < deadline:
                self.drain_stderr()
                time.sleep(.01)
            if self.process.poll() is None:
                os.killpg(self.process.pid, signal.SIGKILL)
                killed = True
            self.process.wait(timeout=3)
            deadline = time.monotonic() + .1
            while self.diagnostics is not None and not self.stderr_eof:
                self.drain_stderr()
                if self.stderr_eof or time.monotonic() >= deadline:
                    break
                time.sleep(.005)
        finally:
            if self.diagnostics is not None:
                self.diagnostics.update({
                    'exitBeforeCleanup': original_exit,
                    'returnCodeAfterCleanup': self.process.poll(),
                    'groupTerminationSent': term_sent,
                    'processTerminationRequested': original_exit is None and term_sent,
                    'cleanupEscalated': killed,
                    'stdoutClosed': self.stdout_closed,
                    'stderrTail': self.stderr_tail.decode('utf-8', errors='replace'),
                    'stderrBytesSeen': self.stderr_bytes,
                    'stderrTruncated': self.stderr_bytes > 32768,
                    'stderrPanicObserved': self.stderr_panic,
                    'stderrComplete': self.stderr_eof,
                    'methods': list(self.methods),
                })
            self.selector.close()
            for stream in [self.process.stdin, self.process.stdout, self.process.stderr]:
                if stream is not None:
                    stream.close()
            self.closed = True


def validate_native_diagnostics(processes):
    for value in processes:
        if value.get('stderrPanicObserved') or 'panicked at' in value.get('stderrTail', ''):
            raise RuntimeError('native_diagnostic_panic')
        if value.get('cleanupEscalated'):
            raise RuntimeError('native_diagnostic_forced_cleanup')
        if not value.get('stderrComplete'):
            raise RuntimeError('native_diagnostic_incomplete')
        if value.get('exitBeforeCleanup') not in [None, 0] or value.get('returnCodeAfterCleanup') not in [0, -signal.SIGTERM]:
            raise RuntimeError('native_diagnostic_exit')


MCP = r'''import sys,json,pathlib,os
marker=pathlib.Path(sys.argv[1])
with marker.open('a') as f:f.write(json.dumps({'pid':os.getpid(),'event':'started'})+'\n')
for line in sys.stdin:
 r=json.loads(line)
 with marker.open('a') as f:f.write(json.dumps({'pid':os.getpid(),'event':r.get('method')})+'\n')
 if 'id' not in r:continue
 if r['method']=='initialize':result={'protocolVersion':r['params']['protocolVersion'],'capabilities':{'tools':{}},'serverInfo':{'name':'webmux-marker','version':'1'}}
 elif r['method']=='tools/list':result={'tools':[]}
 else:result={}
 print(json.dumps({'jsonrpc':'2.0','id':r['id'],'result':result}),flush=True)
'''


def main():
    if os.name != "posix":
        raise RuntimeError("native_observer_platform_unqualified")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--codex", required=True)
    parser.add_argument("--node", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    codex, node = [Path(shutil.which(p) or p).resolve() for p in [args.codex, args.node]]
    version = subprocess.check_output([str(codex), "--version"], timeout=5, text=True).strip()
    if version != "codex-cli 0.169.0":
        raise RuntimeError("unqualified_native_version")
    module = Path(__file__).resolve().parents[1] / "webmux/server/internal/openrigcoord/codex-owner-plan.mts"
    report = {"scope": "Disposable native homes, no auth or coding turn; configured MCP startup suppression and policy equivalence only",
              "version": version, "moduleSHA256": sha(module), "probeSHA256": sha(Path(__file__).resolve()),
              "cliSHA256": sha(codex), "cases": [], "passed": False}
    root = Path(tempfile.mkdtemp(prefix="wm-observer-startup-")).resolve()
    (root / "mcp.py").write_text(MCP)
    try:
        for mode in ["full", "read", "workspace"]:
            policies = []
            for isolated in [False, True]:
                home = root / (mode + ("-isolated" if isolated else "-control"))
                home.mkdir(mode=0o700)
                cwd, extra = home / "project", home / "extra root"
                cwd.mkdir()
                extra.mkdir()
                marker, hook = home / "mcp.marker", home / "hook.marker"
                command = "/usr/bin/touch " + str(hook)
                identity = {"event_name": "session_start", "hooks": [
                    {"type": "command", "command": command, "timeout": 5, "async": False}]}
                trust = "sha256:" + hashlib.sha256(json.dumps(identity, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
                config_path = home / "config.toml"
                key = str(config_path) + ":session_start:0:0"
                sandbox = {"full": "danger-full-access", "read": "read-only", "workspace": "workspace-write"}[mode]
                config_path.write_text(
                    'cli_auth_credentials_store = "file"\napproval_policy = "never"\nsandbox_mode = ' + json.dumps(sandbox) + '\n'
                    '[sandbox_workspace_write]\nnetwork_access = false\nexclude_slash_tmp = true\nexclude_tmpdir_env_var = true\nwritable_roots = ' + json.dumps([str(extra)]) + '\n'
                    '[features]\nhooks = true\n[projects.' + json.dumps(str(cwd)) + ']\ntrust_level = "trusted"\n'
                    '[[hooks.SessionStart]]\n[[hooks.SessionStart.hooks]]\ntype = "command"\ncommand = ' + json.dumps(command) + '\ntimeout = 5\n'
                    '[hooks.state.' + json.dumps(key) + ']\ntrusted_hash = ' + json.dumps(trust) + '\n'
                    '[mcp_servers."webmux.marker"]\ncommand = ' + json.dumps(os.sys.executable) + '\nargs = ' + json.dumps([str(root / "mcp.py"), str(marker)]) + '\nstartup_timeout_sec = 5\n')
                before = config_path.read_bytes()
                case = {"mode": mode, "isolated": isolated, "nativeProcess": {}}
                report["cases"].append(case)
                native = Native(codex, cwd, home, diagnostics=case["nativeProcess"])
                try:
                    native.rpc("initialize", {"clientInfo": {"name": "webmux-startup-control", "version": "1"},
                                              "capabilities": {"experimentalApi": True}})
                    native.process.stdin.write(b'{"method":"initialized"}\n')
                    native.process.stdin.flush()
                    config = native.rpc("config/read", {"cwd": str(cwd), "includeLayers": False})["config"]
                    assert not marker.exists(), "MCP started before thread"
                    params = {"cwd": str(cwd), "ephemeral": True}
                    if isolated:
                        code = 'import fs from "node:fs";import {codexObservationConfig} from ' + json.dumps(module.as_uri()) + ';console.log(JSON.stringify(codexObservationConfig(JSON.parse(fs.readFileSync(0,"utf8")))));'
                        params["config"] = json.loads(subprocess.check_output(
                            [str(node), "--input-type=module", "-e", code], input=json.dumps(config), text=True, timeout=5))
                    thread = native.rpc("thread/start", params)
                    hooks = native.rpc("hooks/list", {"threadId": thread["thread"]["id"], "cwds": []})
                    time.sleep(1)
                    assert thread["thread"]["ephemeral"] is True
                    events = [json.loads(line)["event"] for line in marker.read_text().splitlines()] if marker.exists() else []
                    if isolated:
                        assert not events, "MCP started during observation"
                        assert all(not row["hooks"] and not row["errors"] for row in hooks["data"])
                    else:
                        assert "initialize" in events and "tools/list" in events, "MCP positive control not active"
                        assert any(h["enabled"] and h["trustStatus"] == "trusted" for row in hooks["data"] for h in row["hooks"])
                    policy = {k: thread.get(k) for k in ["cwd", "runtimeWorkspaceRoots", "sandbox", "approvalPolicy", "approvalsReviewer", "model", "modelProvider", "reasoningEffort", "serviceTier", "activePermissionProfile"]}
                    policy = json.loads(json.dumps(policy).replace(str(home), "<fixture>"))
                    policies.append(policy)
                    assert before == config_path.read_bytes(), "owner config changed"
                    case.update({"mcpEvents": events, "hookMarker": hook.exists(), "policy": policy,
                                 "configUnchanged": True, "noCodingTurn": "turn/start" not in native.methods})
                finally:
                    native.close()
                    case["ownedAppServerStopped"] = native.process.poll() is not None
                    if marker.exists():
                        for pid in {json.loads(line)["pid"] for line in marker.read_text().splitlines()}:
                            try:
                                os.kill(pid, 0)
                            except ProcessLookupError:
                                continue
                            raise RuntimeError("owned_mcp_cleanup_unconfirmed")
            assert policies[0] == policies[1], "observation changed owner policy"
        validate_native_diagnostics([case["nativeProcess"] for case in report["cases"]])
        report["passed"] = True
    finally:
        args.output.write_text(json.dumps(report, indent=2) + "\n")
        # Retain fixture evidence on failure; remove only this script's own homes on success.
        if report["passed"]:
            shutil.rmtree(root)
    print(json.dumps({"passed": report["passed"], "cases": len(report["cases"])}))


if __name__ == "__main__":
    main()
