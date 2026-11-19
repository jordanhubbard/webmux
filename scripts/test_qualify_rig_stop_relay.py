import importlib.util
import io
import json
from pathlib import Path
import subprocess
import sys
import unittest


spec = importlib.util.spec_from_file_location('rig_stop_relay', Path(__file__).with_name('qualify-rig-stop-relay.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class RelayCommandEvidence(unittest.TestCase):
    def test_success_retains_both_streams_and_result(self):
        log = io.StringIO()
        result = module.run_logged([sys.executable, '-c',
                                    'import sys; print("ready"); print("diagnostic", file=sys.stderr)'], log)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, 'ready\n')
        self.assertEqual(result.stderr, 'diagnostic\n')
        self.assertIn('ready\ndiagnostic\n', log.getvalue())

    def test_nonzero_exit_remains_failure_after_output_is_logged(self):
        log = io.StringIO()
        with self.assertRaisesRegex(RuntimeError, r'failed \(7\)'):
            module.run_logged([sys.executable, '-c', 'print("before exit"); raise SystemExit(7)'], log)
        self.assertIn('before exit', log.getvalue())
        self.assertIn('"exit": 7', log.getvalue())

    def test_nonzero_inspection_can_still_return_its_result(self):
        log = io.StringIO()
        result = module.run_logged([sys.executable, '-c', 'raise SystemExit(7)'], log, check=False)
        self.assertEqual(result.returncode, 7)
        self.assertIn('"exit": 7', log.getvalue())

    def test_timeout_keeps_partial_streams_and_original_timeout(self):
        log = io.StringIO()
        command = [sys.executable, '-c',
                   'import os,time; os.write(1,b"build-started\\n"); '
                   'os.write(2,b"package-progress\\n\\xe2\\x82"); time.sleep(60)']
        with self.assertRaises(subprocess.TimeoutExpired) as raised:
            module.run_logged(command, log, timeout=2)
        self.assertEqual(raised.exception.cmd, command)
        self.assertEqual(raised.exception.timeout, 2)
        self.assertEqual(raised.exception.stdout, b'build-started\n')
        self.assertEqual(raised.exception.stderr, b'package-progress\n\xe2\x82')
        self.assertIn('build-started\npackage-progress\n\ufffd', log.getvalue())
        records = [json.loads(line) for line in log.getvalue().splitlines() if line.startswith('{')]
        self.assertEqual(records[-1], {'command': command, 'timedOut': True, 'timeoutSeconds': 2})
        self.assertNotIn('"exit": 0', log.getvalue())


if __name__ == '__main__':
    unittest.main()
