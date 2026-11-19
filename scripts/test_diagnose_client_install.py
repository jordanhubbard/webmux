"""Real owned-process controls; no Go build, install or hosted-runner admission."""
import importlib.util
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch
import uuid


spec = importlib.util.spec_from_file_location(
    'client_install_observer', Path(__file__).with_name('diagnose-client-install.py'))
observer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(observer)


@unittest.skipUnless(os.name == 'posix' and Path('/bin/ps').is_file(),
                     'Requires POSIX process groups and ps')
class ObserverCleanupTests(unittest.TestCase):
    def test_timeout_cleans_descendant_after_leader_exits_on_term(self):
        self.check_descendant_cleanup('time.sleep(60)', 0.2, -signal.SIGTERM, True)

    def test_successful_leader_exit_still_cleans_descendant(self):
        self.check_descendant_cleanup('sys.exit(0)', 10, 0, False)

    def test_nonzero_result_is_preserved(self):
        with tempfile.TemporaryDirectory(prefix='client-observer-test-') as directory:
            output = Path(directory)
            result = observer.observe([sys.executable, '-c', 'raise SystemExit(7)'],
                                      output, dict(os.environ), output, 'nonzero', 10)
            self.assertEqual(result['status'], 7)
            self.assertFalse(result['observerTimedOut'])

    def check_descendant_cleanup(self, tail, limit, expected_status, timed_out):
        with tempfile.TemporaryDirectory(prefix='client-observer-test-') as directory:
            output = Path(directory)
            identity = output / 'identities.json'
            ready = output / 'ready'
            nonce = 'client_observer_' + uuid.uuid4().hex
            descendant = (
                'import signal,time,pathlib;'
                'signal.signal(signal.SIGTERM,signal.SIG_IGN);'
                f'pathlib.Path({str(ready)!r}).write_text("ready");time.sleep(60)')
            leader = (
                'import subprocess,json,os,time,sys,pathlib;'
                f'child=subprocess.Popen([sys.executable,"-c",{descendant!r},{nonce!r}]);'
                f'pathlib.Path({str(identity)!r}).write_text('
                'json.dumps({"leader":os.getpid(),"child":child.pid}));'
                f'deadline=time.monotonic()+5\n'
                f'while not pathlib.Path({str(ready)!r}).exists() and time.monotonic()<deadline:time.sleep(.01)\n'
                f'assert pathlib.Path({str(ready)!r}).exists()\n' + tail)
            command = [sys.executable, '-c', leader]
            real_popen = subprocess.Popen
            created = []

            def wait_for_fixture(*args, **kwargs):
                child = real_popen(*args, **kwargs)
                if args[0] == command:
                    created.append(child)
                    deadline = time.monotonic() + 5
                    while not ready.exists() and time.monotonic() < deadline:
                        if child.poll() is not None:
                            break
                        time.sleep(0.01)
                    self.assertTrue(ready.exists(), 'Fixture must install its signal handler first')
                return child

            try:
                with patch.object(observer.subprocess, 'Popen', side_effect=wait_for_fixture):
                    result = observer.observe(command, output, dict(os.environ),
                                              output, 'owned-group', limit)
                owned = json.loads(identity.read_text())
                self.assertEqual(result['status'], expected_status)
                self.assertEqual(result['observerTimedOut'], timed_out)
                self.assertFalse(self.owned_descendant_alive(owned, nonce),
                                 'Observer returned with its descendant still running')
            finally:
                # Keep the failing-before control safe too. Never kill by name.
                if identity.exists():
                    owned = json.loads(identity.read_text())
                    if self.owned_descendant_alive(owned, nonce):
                        try:
                            os.killpg(owned['leader'], signal.SIGKILL)
                        except ProcessLookupError:
                            pass
                elif created and created[0].poll() is None:
                    # A fixture startup error can precede its identity receipt.
                    os.killpg(created[0].pid, signal.SIGKILL)
                    created[0].wait(timeout=3)

    @staticmethod
    def owned_descendant_alive(owned, nonce):
        probe = subprocess.run(['/bin/ps', '-p', str(owned['child']),
                                '-o', 'pgid=,stat=,command='],
                               capture_output=True, text=True, check=False)
        fields = probe.stdout.split(None, 2)
        return (probe.returncode == 0 and len(fields) == 3
                and int(fields[0]) == owned['leader'] and 'Z' not in fields[1]
                and nonce in fields[2])


if __name__ == '__main__':
    unittest.main()
