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


class ObserverMetadataTests(unittest.TestCase):
    def test_only_owned_compiler_package_ids_are_retained(self):
        raw = '\n'.join([
            '11 /toolchain/compile -o /private/SECRET-OUTPUT -p net/http /private/SECRET-SOURCE.go',
            '12 /toolchain/compile -p crypto/tls',  # Unobserved PID.
            '13 /unrelated/node -p SECRET-OTHER-PROCESS',
            '14 /toolchain/compile -p ../SECRET-PATH',
            '15 /toolchain/compile -p "PRIVATE INVALID LABEL"',
            '16 /toolchain/compile -p "unterminated',
        ])
        result = observer.compiler_packages(raw, {11, 13, 14, 15, 16})
        self.assertEqual(result, [{'pid': 11, 'package': 'net/http'}])
        self.assertNotIn('SECRET', json.dumps(result))
        self.assertNotIn('private', json.dumps(result))

    def test_compiler_query_uses_only_observed_compiler_pids_and_is_bounded(self):
        processes = [{'pid': 11, 'executable': 'compile'}, {'pid': 12, 'executable': 'node'}]
        reply = subprocess.CompletedProcess([], 0, stdout='11 /toolchain/compile -p net/http\n', stderr='')
        with patch.object(observer.subprocess, 'run', return_value=reply) as query:
            self.assertEqual(observer.sample_compilers(processes), {
                'state': 'sampled', 'packages': [{'pid': 11, 'package': 'net/http'}]})
        self.assertEqual(query.call_args.args[0], ['/bin/ps', '-p', '11', '-o', 'pid=,args='])
        self.assertEqual(query.call_args.kwargs['timeout'], 3)
        with patch.object(observer.subprocess, 'run', side_effect=subprocess.TimeoutExpired('ps', 3)):
            self.assertEqual(observer.sample_compilers(processes), {'state': 'timed-out', 'packages': []})
        with patch.object(observer.subprocess, 'run') as query:
            self.assertEqual(observer.sample_compilers([{'pid': 12, 'executable': 'node'}]),
                             {'state': 'none', 'packages': []})
            self.assertEqual(observer.sample_compilers([
                {'pid': pid, 'executable': 'compile'} for pid in range(129)]),
                {'state': 'over-bound', 'packages': []})
            query.assert_not_called()

    @unittest.skipUnless(os.name == 'posix', 'Uses an owned executable fixture')
    def test_proxy_forwards_original_argv_and_status_without_logging_private_arguments(self):
        with tempfile.TemporaryDirectory(prefix='client-proxy-test-') as directory:
            output = Path(directory)
            fixture = output / 'go'
            argv_file = output / 'argv.json'
            fixture.write_text(f'#!{sys.executable}\nimport sys,json,pathlib\n'
                               f'pathlib.Path({str(argv_file)!r}).write_text(json.dumps(sys.argv[1:]))\n'
                               'raise SystemExit(7)\n')
            fixture.chmod(0o700)
            args = ['build', '-trimpath', '-o', '/private/SECRET-OUTPUT', './cmd/webmux-client']
            log = output / 'events.jsonl'
            with patch.dict(os.environ, {'CGO_ENABLED': '0', 'SECRET_TOKEN': 'PRIVATE-TOKEN'}):
                status = observer.proxy(log, str(fixture), args)
            self.assertEqual(status, 7)
            self.assertEqual(json.loads(argv_file.read_text()), args)
            events = [json.loads(line) for line in log.read_text().splitlines()]
            self.assertEqual(events[0]['action'], 'build')
            self.assertEqual(events[0]['workload'], 'client')
            self.assertTrue(events[0]['trimpath'])
            self.assertEqual(events[0]['cgo'], '0')
            self.assertEqual(events[1]['status'], 7)
            self.assertNotIn('SECRET', log.read_text())
            self.assertNotIn('PRIVATE', log.read_text())


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
