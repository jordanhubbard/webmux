"""Disposable subprocess diagnostics; no native account, model or sandbox claim."""
import errno
import importlib.util
import json
import os
from pathlib import Path
import shlex
import signal
import subprocess
import sys
import tempfile
import unittest

SOURCE = Path(__file__).with_name('qualify-codex-observer-startup.py')
spec = importlib.util.spec_from_file_location('observer_startup_controls', SOURCE)
controls = importlib.util.module_from_spec(spec)
spec.loader.exec_module(controls)


@unittest.skipUnless(os.name == 'posix', 'Native fixtures use POSIX process groups')
class NativeDiagnosticsTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='wm-observer-diagnostics-')
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.home = self.root / 'home'
        self.home.mkdir(mode=0o700)
        self.project = self.root / 'project'
        self.project.mkdir()

    def executable(self, name, code):
        source = self.root / (name + '.py')
        source.write_text(code)
        wrapper = self.root / name
        wrapper.write_text('#!/bin/sh\nexec ' + shlex.join([sys.executable, str(source)]) + ' "$@"\n')
        wrapper.chmod(0o700)
        return wrapper

    def native(self, code, diagnostics):
        native = controls.Native(self.executable('native', code), self.project, self.home,
                                 diagnostics=diagnostics)
        self.addCleanup(native.close)
        return native

    def test_noisy_stderr_does_not_block_rpc_and_retains_bounded_tail(self):
        diagnostic = {}
        native = self.native('''import json,sys
request=json.loads(sys.stdin.buffer.readline())
sys.stderr.buffer.write(b'x' * (1024 * 1024) + b'DIAGNOSTIC_END\\n')
sys.stderr.buffer.flush()
print(json.dumps({'id':request['id'],'result':{'ready':True}}),flush=True)
sys.stdin.buffer.read()
''', diagnostic)
        self.assertEqual(native.rpc('initialize', {}), {'ready': True})
        native.close()
        self.assertEqual(len(native.stderr_tail), 32768)
        self.assertTrue(diagnostic['stderrTail'].endswith('DIAGNOSTIC_END\n'))
        self.assertEqual(diagnostic['stderrBytesSeen'], 1024 * 1024 + len(b'DIAGNOSTIC_END\n'))
        self.assertTrue(diagnostic['stderrTruncated'])
        self.assertTrue(diagnostic['stderrComplete'])
        self.assertIsNone(diagnostic['exitBeforeCleanup'])
        self.assertTrue(diagnostic['processTerminationRequested'])
        self.assertEqual(diagnostic['returnCodeAfterCleanup'], -signal.SIGTERM)
        self.assertEqual(diagnostic['methods'], ['initialize'])
        self.assertIsNotNone(native.process.poll())

    def test_natural_failure_status_survives_cleanup(self):
        diagnostic = {}
        native = self.native('''import sys
sys.stdin.buffer.readline()
sys.stderr.write('fixture initialization failed\\n')
sys.stderr.flush()
raise SystemExit(17)
''', diagnostic)
        with self.assertRaisesRegex(RuntimeError, 'native_stdout_closed'):
            native.rpc('initialize', {})
        native.close()
        self.assertEqual(diagnostic['stderrTail'], 'fixture initialization failed\n')
        self.assertEqual(diagnostic['exitBeforeCleanup'], 17)
        self.assertEqual(diagnostic['returnCodeAfterCleanup'], 17)
        self.assertFalse(diagnostic['processTerminationRequested'])
        self.assertTrue(diagnostic['stderrComplete'])
        snapshot = dict(diagnostic)
        native.close()
        self.assertEqual(diagnostic, snapshot, 'Repeated cleanup must not overwrite the original status')

    def test_cleanup_drains_noisy_termination_handler(self):
        diagnostic = {}
        native = self.native('''import json,signal,sys
def stop(*unused):
 sys.stderr.buffer.write(b'z' * (512 * 1024));sys.stderr.buffer.flush()
 raise SystemExit(0)
signal.signal(signal.SIGTERM,stop)
request=json.loads(sys.stdin.buffer.readline())
print(json.dumps({'id':request['id'],'result':{}}),flush=True)
sys.stdin.buffer.read()
''', diagnostic)
        self.assertEqual(native.rpc('initialize', {}), {})
        native.close()
        self.assertIsNone(diagnostic['exitBeforeCleanup'])
        self.assertTrue(diagnostic['processTerminationRequested'])
        self.assertEqual(diagnostic['returnCodeAfterCleanup'], 0)
        self.assertEqual(diagnostic['stderrBytesSeen'], 512 * 1024)
        self.assertTrue(diagnostic['stderrComplete'])
        self.assertFalse(diagnostic['cleanupEscalated'])

    def test_launch_failure_is_bound_to_the_supplied_diagnostic(self):
        diagnostic = {'case': 'fixture', 'stage': 'policy'}
        with self.assertRaises(FileNotFoundError):
            controls.Native(self.root / 'missing-executable', self.project, self.home, diagnostics=diagnostic)
        self.assertEqual(diagnostic, {'case': 'fixture', 'stage': 'policy',
                                     'launchErrorType': 'FileNotFoundError', 'launchErrno': errno.ENOENT})

    def test_capture_requires_explicit_opt_in(self):
        native = self.native('''import json,sys
request=json.loads(sys.stdin.buffer.readline())
sys.stderr.write('discarded fixture text\\n')
print(json.dumps({'id':request['id'],'result':{}}),flush=True)
sys.stdin.buffer.read()
''', None)
        self.assertEqual(native.rpc('initialize', {}), {})
        native.close()
        self.assertEqual(native.stderr_bytes, 0)
        self.assertEqual(native.stderr_tail, b'')
        self.assertIsNone(native.process.stderr)

    def test_isolation_receipt_keeps_initialization_failure_case_and_stage(self):
        codex = self.executable('fake-codex', '''import sys
if '--version' in sys.argv:
 print('codex-cli 0.169.0');raise SystemExit(0)
sys.stdin.buffer.readline()
sys.stderr.write('controlled startup failure\\n');sys.stderr.flush()
raise SystemExit(17)
''')
        node = self.executable('fake-node', 'import json\nprint(json.dumps(' + repr({'executable': str(codex), 'args': []}) + '))\n')
        output = self.root / 'failure.json'
        result = subprocess.run([sys.executable, str(SOURCE.with_name('qualify-codex-observation-isolation.py')),
                                 '--codex', str(codex), '--node', str(node), '--helper', sys.executable,
                                 '--output', str(output)], env={**os.environ, 'TMPDIR': str(self.root)},
                                capture_output=True, text=True, timeout=15)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('native_stdout_closed', result.stderr)
        receipt = json.loads(output.read_text())
        self.assertFalse(receipt['passed'])
        self.assertEqual(receipt['cases'], [{'mode': 'full', 'guarded': False}])
        diagnostic, = receipt['nativeProcesses']
        self.assertEqual((diagnostic['case'], diagnostic['stage']), ('full-control', 'account'))
        self.assertEqual(diagnostic['exitBeforeCleanup'], 17)
        self.assertEqual(diagnostic['stderrTail'], 'controlled startup failure\n')
        self.assertTrue(diagnostic['stderrComplete'])
        self.assertIn('nativeSHA256', receipt)
        self.assertIn('moduleSHA256', receipt)


if __name__ == '__main__':
    unittest.main()
