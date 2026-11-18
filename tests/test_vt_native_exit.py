import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[1] / "scripts/vt-native-exit.py"


class NativeExitTest(unittest.TestCase):
    def invoke(self, command):
        with tempfile.TemporaryDirectory() as directory:
            report = Path(directory) / "native.json"
            child = subprocess.run([sys.executable, str(SCRIPT), str(report), *command],
                                   capture_output=True, timeout=15)
            return child, json.loads(report.read_text())

    def test_success_and_framed_output_pass_through(self):
        child, report = self.invoke([sys.executable, "-c",
                                    "import os; os.write(1, b'\\x16=== RUN   control\\n'); os.write(2, b'stderr-control\\n')"])
        self.assertEqual(child.returncode, 0)
        self.assertEqual(child.stdout, b"\x16=== RUN   control\n")
        self.assertEqual(child.stderr, b"stderr-control\n")
        self.assertTrue(report["started"])
        self.assertEqual(report["exit_code"], 0)
        self.assertIsNone(report["spawn_error"])

    def test_nonzero_child_is_never_green(self):
        child, report = self.invoke([sys.executable, "-c", "raise SystemExit(7)"])
        self.assertEqual(child.returncode, 1)
        self.assertTrue(report["started"])
        self.assertEqual(report["exit_code"], 7)

    def test_spawn_refusal_is_not_a_native_child_exit(self):
        with tempfile.TemporaryDirectory() as directory:
            child, report = self.invoke([str(Path(directory) / "absent.exe")])
        self.assertEqual(child.returncode, 1)
        self.assertFalse(report["started"])
        self.assertIsNone(report["exit_code"])
        self.assertIsNotNone(report["spawn_error"])

    @unittest.skipUnless(sys.platform == "win32", "native Win32 status requires Windows")
    def test_windows_high_bit_status_survives_normalized_driver_failure(self):
        command = ("import ctypes; f=ctypes.WinDLL('kernel32').ExitProcess; "
                   "f.argtypes=[ctypes.c_uint32]; f.restype=None; f(0xc0000409)")
        child, report = self.invoke([sys.executable, "-c", command])
        self.assertEqual(child.returncode, 1)
        self.assertTrue(report["started"])
        self.assertEqual(report["windows_exit_hex"], "0xc0000409")
        self.assertEqual(report["exit_code"] & 0xffffffff, 0xc0000409)


if __name__ == "__main__":
    unittest.main()
