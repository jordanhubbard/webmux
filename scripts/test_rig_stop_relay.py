import importlib.util
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


spec = importlib.util.spec_from_file_location(
    'rig_stop_relay', Path(__file__).with_name('qualify-rig-stop-relay.py'))
relay = importlib.util.module_from_spec(spec)
spec.loader.exec_module(relay)


class CommandEvidenceTests(unittest.TestCase):
    def test_deadline_keeps_both_partial_streams_and_original_timeout(self):
        log = io.StringIO()
        command = [sys.executable, '-u', '-c',
                   "import sys,time;print('BUILD_STDOUT_READY',flush=True);"
                   "print('BUILD_STDERR_READY',file=sys.stderr,flush=True);time.sleep(60)"]
        with self.assertRaises(subprocess.TimeoutExpired) as caught:
            relay.run_recorded(command, log, timeout=2)
        self.assertEqual(caught.exception.cmd, command)
        self.assertEqual(caught.exception.timeout, 2)
        events = [json.loads(line) for line in log.getvalue().splitlines()]
        self.assertEqual([e['event'] for e in events], ['command_start', 'command_timeout'])
        self.assertEqual(events[0]['command'], command)
        self.assertEqual(events[0]['commandId'], events[1]['commandId'])
        self.assertIn('BUILD_STDOUT_READY', events[1]['stdout'])
        self.assertIn('BUILD_STDERR_READY', events[1]['stderr'])
        self.assertEqual(events[1]['timeoutSeconds'], 2)
        self.assertGreaterEqual(events[1]['elapsedSeconds'], 2)

    def test_bounded_evidence_does_not_truncate_successful_command_result(self):
        log = io.StringIO()
        size = relay.LOG_STREAM_LIMIT + 100
        result = relay.run_recorded(
            [sys.executable, '-c', f"import sys;sys.stdout.write('A'*{size}+'TAIL')"], log)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, 'A' * size + 'TAIL')
        event = json.loads(log.getvalue().splitlines()[-1])
        self.assertEqual(event['event'], 'command_exit')
        self.assertEqual(event['stdoutBytes'], size + 4)
        self.assertEqual(event['stdoutOmittedBytes'], 104)
        self.assertLess(len(event['stdout']), relay.LOG_STREAM_LIMIT + 100)
        self.assertTrue(event['stdout'].endswith('TAIL'))

    def test_nonzero_exit_is_retained_and_check_false_still_returns_it(self):
        command = [sys.executable, '-c',
                   "import sys;print('FAILED_BUILD');print('REASON',file=sys.stderr);sys.exit(7)"]
        for check in [True, False]:
            with self.subTest(check=check):
                log = io.StringIO()
                if check:
                    with self.assertRaisesRegex(RuntimeError, r'failed \(7\)'):
                        relay.run_recorded(command, log)
                else:
                    self.assertEqual(relay.run_recorded(command, log, check=False).returncode, 7)
                event = json.loads(log.getvalue().splitlines()[-1])
                self.assertEqual(event['exit'], 7)
                self.assertIn('FAILED_BUILD', event['stdout'])
                self.assertIn('REASON', event['stderr'])

    def test_spawn_failure_has_a_start_record_and_preserves_os_error(self):
        log = io.StringIO()
        with tempfile.TemporaryDirectory() as root:
            with self.assertRaises(FileNotFoundError):
                relay.run_recorded([str(Path(root) / 'absent-command')], log)
        events = [json.loads(line) for line in log.getvalue().splitlines()]
        self.assertEqual([e['event'] for e in events], ['command_start', 'command_start_error'])
        self.assertEqual(events[1]['errorType'], 'FileNotFoundError')


if __name__ == '__main__':
    unittest.main()
