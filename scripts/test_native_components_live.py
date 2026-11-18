"""Opt-in real Linux guacd/plugin inventory; no RDP credentials or target host."""
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import signal
import socket
import subprocess
import tempfile
import time
import unittest


@unittest.skipUnless(platform.system() == "Linux" and os.environ.get("WEBMUX_NATIVE_COMPONENTS_GUACD"),
                     "Set WEBMUX_NATIVE_COMPONENTS_GUACD for a private Linux guacd fixture")
class LiveNativeInventory(unittest.TestCase):
    def test_loaded_plugin_package_identity_and_cleanup(self):
        destination = os.environ.get("WEBMUX_NATIVE_COMPONENTS_OUTPUT")
        if destination:
            Path(destination).mkdir(parents=True, exist_ok=True)
        root = Path(tempfile.mkdtemp(prefix="native-components-", dir=destination))
        executable = Path(os.environ["WEBMUX_NATIVE_COMPONENTS_GUACD"]).resolve(strict=True)
        script = Path(__file__).with_name("native-components.py")
        with socket.socket() as reservation:
            reservation.bind(("127.0.0.1", 0))
            port = reservation.getsockname()[1]
        client = None
        summary = {"passed": False}
        with (root / "guacd.log").open("wb") as log:
            process = subprocess.Popen([str(executable), "-f", "-b", "127.0.0.1", "-l", str(port), "-L", "info"],
                                       stdout=log, stderr=log, start_new_session=True)
            try:
                until = time.monotonic() + 10
                while time.monotonic() < until:
                    self.assertIsNone(process.poll(), "Private guacd exited; inspect guacd.log")
                    try:
                        client = socket.create_connection(("127.0.0.1", port), timeout=1)
                        break
                    except OSError:
                        time.sleep(.1)
                self.assertIsNotNone(client)
                client.sendall(b"6.select,3.rdp;")
                self.assertTrue(client.recv(8192).startswith(b"4.args,"))
                # guacd forks from a connection-handling task. Inspect only
                # this fixture's tasks/children, never an unrelated system PID.
                candidates = {process.pid}
                for task in Path(f"/proc/{process.pid}/task").iterdir():
                    try:
                        candidates.update(int(pid) for pid in (task / "children").read_text().split())
                    except FileNotFoundError:
                        pass
                workers = [pid for pid in candidates if "libguac-client-rdp" in Path(f"/proc/{pid}/maps").read_text()]
                self.assertEqual(len(workers), 1)
                output = root / "components.json"
                command = ["python3", str(script), "--output", str(output), "--tool", "guacd=" + str(executable),
                           "--guacd-pid", str(workers[0])]
                with (root / "inventory.log").open("wb") as inventory_log:
                    subprocess.run(command, check=True, stdout=inventory_log, stderr=inventory_log, timeout=120)
                    original = output.read_bytes()
                    duplicate = subprocess.run(command, stdout=inventory_log, stderr=inventory_log, timeout=10)
                self.assertEqual(duplicate.returncode, 2)
                self.assertEqual(output.read_bytes(), original)
                data = json.loads(output.read_text())
                observed = data["guacdProcess"]
                self.assertEqual(observed["state"], "observed")
                self.assertTrue(observed["versionToolMatchesExecutable"])
                self.assertEqual(observed["versionProbe"]["state"], "ok")
                self.assertTrue(any("libguac-client-rdp" in path for path in observed["observedPlugins"]))
                files = observed["mappedFiles"]
                self.assertGreater(len(files), 3)
                self.assertTrue(all(item["state"] == "recorded" for item in files))
                for item in files:
                    self.assertEqual(item["file"]["sha256"], hashlib.sha256(Path(item["file"]["path"]).read_bytes()).hexdigest())
                packages = [package for item in files for package in item["package"].get("packages", [])]
                if shutil.which("dpkg-query"):
                    self.assertTrue(packages, "Live Debian package metadata must be recorded")
                    self.assertTrue(all(package.get("version") for package in packages))
                    self.assertTrue(any(package["copyright"]["state"] == "recorded" for package in packages))
                other = shutil.which("guacd")
                mismatch = None
                if other and not executable.samefile(other):
                    wrong = root / "other-path-tool.json"
                    subprocess.run(command[:3] + [str(wrong), "--tool", "guacd=" + other,
                                                 "--guacd-pid", str(workers[0])], check=True,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=120)
                    mismatch = json.loads(wrong.read_text())["guacdProcess"]
                    self.assertFalse(mismatch["versionToolMatchesExecutable"])
                    self.assertEqual(mismatch["versionProbe"]["state"], "not_observed")
                summary.update(passed=True, mappedELFFiles=len(files), packageRecords=len(packages),
                               plugins=[Path(path).name for path in observed["observedPlugins"]],
                               versionProbeMatchesExecutable=True, duplicateOutputRefused=True,
                               differentPathToolRefused=mismatch is not None,
                               scope="Private guacd/plugin inventory only; not functional RDP, authentication, vulnerability or complete-deployment acceptance")
            finally:
                if client:
                    client.close()
                if process.poll() is None:
                    os.killpg(process.pid, signal.SIGTERM)
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.wait(timeout=5)
                summary["ownedParentExited"] = process.returncode is not None
                try:
                    with socket.create_connection(("127.0.0.1", port), timeout=1):
                        self.fail("Owned guacd listener survived cleanup")
                except ConnectionRefusedError:
                    summary["ownedListenerStopped"] = True
                (root / "native-summary.json").write_text(json.dumps(summary, indent=2) + "\n")
                print(f"Native inventory evidence: {root}", flush=True)


if __name__ == "__main__":
    unittest.main()
