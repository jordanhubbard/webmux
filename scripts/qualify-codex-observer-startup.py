#!/usr/bin/env python3
"""Native Codex 0.169 startup controls in disposable homes; never submits a turn."""
import hashlib
import json
import os
import selectors
import signal
import subprocess
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
    raise SystemExit('The OpenRig owner-plan comparison was removed. Native process diagnostics remain in this module for scripts/test_codex_observer_process.py.')


if __name__ == '__main__':
    main()
