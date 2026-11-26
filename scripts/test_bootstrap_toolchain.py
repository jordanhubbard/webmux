"""Exercise cold-tool selection and refusal paths without network downloads."""
from pathlib import Path
import os
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class BootstrapTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="webmux-bootstrap-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.cache = self.root / "toolchains"
        self.mod = self.root / "go.mod"
        self.mod.write_text("module fixture\n\ngo 1.27.4\n")
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ["PATH"])
        # A failed/corrupt download is enough to prove missing tools reach provisioning,
        # and that unverified bytes never become an executable installation.
        self.executable("curl", 'echo "$*" > "$PROBE_URL"\nwhile [ "$#" -gt 0 ]; do\n'
                        'if [ "$1" = --output ]; then shift; printf corrupt > "$1"; fi\nshift\ndone\n')
        self.env["PROBE_URL"] = str(self.root / "download")
        self.executable("uname", 'case "$1" in -s) echo Linux ;; -m) echo x86_64 ;; esac\n')
        self.executable("npm", "exit 0\n")

    def executable(self, name, body):
        p = self.bin / name
        p.write_text("#!/bin/sh\n" + body)
        p.chmod(0o755)
        return str(p)

    def run_tool(self, tool, command=None):
        return subprocess.run(["sh", str(ROOT / "scripts/bootstrap-toolchain.sh"), tool,
                               command or tool, str(self.cache), str(self.mod)],
                              env=self.env, capture_output=True, text=True, timeout=10)

    def test_compatible_tools_need_no_cache_or_download(self):
        self.executable("node", "echo v24.22.3\n")
        self.executable("go", "echo 'go version go1.27.4 linux/amd64'\n")
        for tool in ["node", "go"]:
            with self.subTest(tool=tool):
                r = self.run_tool(tool)
                self.assertEqual(r.returncode, 0, r.stderr)
        self.assertFalse(self.cache.exists())
        self.assertFalse((self.root / "download").exists())

    def test_old_node_and_missing_go_download_but_reject_corrupt_archives(self):
        self.executable("node", "echo v22.22.1\n")
        self.executable("go", "exit 127\n")
        for tool, archive in [("node", "node-v24.22.3-linux-x64.tar.gz"),
                              ("go", "go1.27.4.linux-amd64.tar.gz")]:
            with self.subTest(tool=tool):
                r = self.run_tool(tool)
                self.assertNotEqual(r.returncode, 0)
                self.assertIn("Checksum mismatch", r.stderr)
                self.assertIn(archive, (self.root / "download").read_text())
                self.assertEqual(list(self.cache.iterdir()), [], "Partial install or lock leaked")

    def test_missing_node_is_provisioned(self):
        self.executable("node", "exit 127\n")
        r = self.run_tool("node")
        self.assertIn("Installing node", r.stderr)
        self.assertIn("Checksum mismatch", r.stderr)

    def test_explicit_incompatible_override_is_not_replaced(self):
        command = self.executable("custom-node", "echo v22.22.1\n")
        r = self.run_tool("node", command)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("Explicit node override", r.stderr)
        self.assertFalse(self.cache.exists())
        self.assertFalse((self.root / "download").exists())

    def test_go_pin_cannot_silently_lag_module_requirement(self):
        self.mod.write_text("module fixture\n\ngo 1.28.0\n")
        self.executable("go", "echo 'go version go1.27.4 linux/amd64'\n")
        r = self.run_tool("go")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("Update the pinned Go bootstrap", r.stderr)
        self.assertFalse((self.root / "download").exists())

    def test_make_build_uses_both_toolchains_and_locked_install(self):
        r = subprocess.run(["make", "--no-print-directory", "-n", "-j4", "build"],
                           cwd=ROOT, capture_output=True, text=True, timeout=10)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout.count("bootstrap-toolchain.sh go"), 1)
        self.assertEqual(r.stdout.count("bootstrap-toolchain.sh node"), 1)
        self.assertIn("npm ci --workspace=frontend --include-workspace-root", r.stdout)
        self.assertNotIn("npm install", r.stdout)


if __name__ == "__main__":
    unittest.main()
