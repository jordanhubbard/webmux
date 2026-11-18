import importlib.util
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch

spec = importlib.util.spec_from_file_location("native_components", Path(__file__).with_name("native-components.py"))
inventory = importlib.util.module_from_spec(spec)
spec.loader.exec_module(inventory)


class NativeInventory(unittest.TestCase):
    def test_maps_deduplicate_segments_but_keep_deleted_identity(self):
        line = "1000-2000 r-xp 00000000 08:02 17 /tmp/libguac-client-rdp.so"
        data = inventory.parse_maps(line + "\n" + line.replace("r-xp", "r--p") + "\n" + line + " (deleted)\n"
                                    "3000-4000 rw-p 00000000 00:00 0 [heap]\n")
        self.assertEqual(len(data), 2)
        self.assertEqual({item["deleted"] for item in data}, {False, True})
        self.assertEqual({item["mappedInode"] for item in data}, {17})
        for text in ["broken", line.replace("08:02", "broken"), line.replace("rdp.so", r"rdp\012.so")]:
            with self.assertRaises(ValueError):
                inventory.parse_maps(text)

    @unittest.skipUnless(hasattr(os, "getuid"), "Linux process-file fixture uses POSIX ownership")
    def test_live_file_inode_must_match_and_deleted_files_are_not_rehashed(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            proc = root / "77"
            proc.mkdir()
            exe = root / "guacd"
            exe.write_bytes(b"\x7fELFfixture")
            (proc / "exe").symlink_to(exe)
            (proc / "stat").write_text("77 (guacd worker) " + " ".join(["S"] + ["0"] * 18 + ["123"]))
            info = exe.stat()
            row = f"1000-2000 r-xp 0 {os.major(info.st_dev):x}:{os.minor(info.st_dev):x} {info.st_ino} {exe}"
            (proc / "maps").write_text(row + "\n" + row.replace(str(exe), str(root / "deleted.so")) + " (deleted)\n")
            packages = Mock()
            packages.lookup.return_value = {"state": "unowned"}
            result = inventory.observe_guacd(77, packages, root)
            states = {Path(item["path"]).name: item["state"] for item in result["mappedFiles"]}
            self.assertEqual(states, {"guacd": "recorded", "deleted.so": "deleted"})
            packages.lookup.assert_called_once()
            (proc / "maps").write_text(row.replace(f" {info.st_ino} ", f" {info.st_ino + 1} "))
            result = inventory.observe_guacd(77, packages, root)
            self.assertEqual(result["mappedFiles"][0]["state"], "replaced_on_disk")
            self.assertNotIn("file", result["mappedFiles"][0])

    def test_reused_process_id_refuses_a_successful_observation(self):
        with tempfile.TemporaryDirectory() as directory:
            proc = Path(directory) / "77"
            proc.mkdir()
            (proc / "exe").symlink_to(Path(directory) / "guacd")
            (proc / "maps").write_text("1000-2000 rw-p 0 00:00 0 [heap]\n")
            with patch.object(inventory, "process_identity", side_effect=[("123", 1, 2), ("456", 1, 2)]):
                with self.assertRaisesRegex(RuntimeError, "identity changed"):
                    inventory.observe_guacd(77, Mock(), Path(directory))

    def test_package_query_failures_are_not_unowned_or_complete(self):
        packages = inventory.DebianPackages()
        packages.tool = "/fixture/dpkg-query"
        for result in [{"state": "timeout"}, {"state": "unavailable"}, {"state": "error", "exit": 2},
                       {"state": "ok", "exit": 0, "truncated": True}]:
            with patch.object(inventory, "command", return_value=result):
                self.assertEqual(packages.lookup(Path("/fixture/library.so"))["state"], "unavailable")
        with patch.object(inventory, "command", return_value={"state": "error", "exit": 1, "output": ""}):
            self.assertEqual(packages.lookup(Path("/fixture/library.so"))["state"], "unowned")

    def test_exact_package_owner_version_and_declared_license_evidence(self):
        packages = inventory.DebianPackages()
        packages.tool = "/fixture/dpkg-query"
        replies = [{"state": "ok", "exit": 0, "output": "unrelated: /other.so\nlibdemo:arm64: /fixture/library.so\n"},
                   {"state": "ok", "exit": 0, "output": "libdemo:arm64\t1.2-3ubuntu4\tarm64\tdemo\n"}]
        with patch.object(inventory, "command", side_effect=replies), patch.object(
                inventory, "copyright_evidence", return_value={"state": "recorded", "declaredLicenseFields": ["MIT"]}):
            result = packages.lookup(Path("/fixture/library.so"))
        self.assertEqual(result["packages"][0]["version"], "1.2-3ubuntu4")
        self.assertEqual(result["packages"][0]["sourcePackage"], "demo")
        self.assertEqual(result["packages"][0]["copyright"]["declaredLicenseFields"], ["MIT"])

    def test_missing_tools_and_no_process_are_explicit_and_output_is_exclusive(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "inventory.json"
            with patch("sys.argv", ["native-components.py", "--output", str(output)]), patch.object(
                    inventory.shutil, "which", return_value=None), patch("builtins.print"):
                inventory.main()
                data = json.loads(output.read_text())
                self.assertEqual(data["guacdProcess"]["state"], "not_observed")
                self.assertEqual({item["state"] for item in data["tools"].values()}, {"not_found"})
                before = output.read_bytes()
                with patch("sys.stderr", io.StringIO()), self.assertRaises(SystemExit):
                    inventory.main()
                self.assertEqual(output.read_bytes(), before)
            if os.name == "posix":
                self.assertEqual(output.stat().st_mode & 0o777, 0o600)

    def test_lost_process_does_not_write_a_success_report(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "inventory.json"
            with patch("sys.argv", ["native-components.py", "--output", str(output), "--guacd-pid", "77"]), patch.object(
                    inventory.shutil, "which", return_value=None), patch.object(inventory.platform, "system", return_value="Linux"), patch.object(
                    inventory, "observe_guacd", side_effect=ProcessLookupError("Fixture process exited")), patch("sys.stderr", io.StringIO()):
                with self.assertRaises(SystemExit) as error:
                    inventory.main()
                self.assertEqual(error.exception.code, 1)
            self.assertFalse(output.exists())

    def test_tool_replacement_during_version_probe_cannot_supply_identity(self):
        with tempfile.TemporaryDirectory() as directory:
            executable = Path(directory) / "ssh"
            executable.write_bytes(b"original tool")
            output = Path(directory) / "inventory.json"

            def replace_tool(_command):
                executable.write_bytes(b"replacement tool")
                return {"state": "ok", "exit": 0, "output": "replacement version"}

            with patch("sys.argv", ["native-components.py", "--output", str(output)]), patch.object(
                    inventory.shutil, "which", side_effect=lambda name: str(executable) if name == "ssh" else None), patch.object(
                    inventory, "command", side_effect=replace_tool), patch("builtins.print"):
                inventory.main()
            data = json.loads(output.read_text())
            self.assertEqual(data["tools"]["ssh"]["state"], "unavailable")
            self.assertNotIn("versionProbe", data["tools"]["ssh"])

    @unittest.skipUnless(hasattr(os, "getuid"), "Linux account isolation uses POSIX ownership")
    def test_other_account_is_refused_before_reading_process_files(self):
        process = Mock()
        process.stat.return_value.st_uid = os.getuid() + 1
        with self.assertRaises(PermissionError):
            inventory.process_identity(process)


if __name__ == "__main__":
    unittest.main()
