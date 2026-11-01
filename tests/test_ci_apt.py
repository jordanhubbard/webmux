import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("ci_apt", Path(__file__).resolve().parents[1] / "scripts" / "configure-ci-apt.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class AptConfigurationTest(unittest.TestCase):
    def test_legacy_sources_preserve_suites_keys_and_third_parties(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            original = "deb [signed-by=/keys/ubuntu.gpg] http://azure.archive.ubuntu.com/ubuntu jammy main\ndeb http://security.ubuntu.com/ubuntu jammy-security main\ndeb https://packages.microsoft.com/ubuntu/22.04/prod jammy main\n"
            (root / "sources.list").write_text(original)
            module.configure(root)
            result = (root / "sources.list").read_text()
            self.assertIn("[signed-by=/keys/ubuntu.gpg] https://archive.ubuntu.com/ubuntu/ jammy main", result)
            self.assertIn("https://security.ubuntu.com/ubuntu/ jammy-security main", result)
            self.assertIn("https://packages.microsoft.com/ubuntu/22.04/prod jammy main", result)
            settings = (root / "apt.conf.d/99-webmux-ci-network").read_text()
            self.assertIn('Acquire::https::Timeout "30";', settings)
            self.assertIn('Dpkg::Lock::Timeout "120";', settings)
            self.assertNotIn("AllowUnauthenticated", settings)
            self.assertNotIn("AllowInsecure", settings)

    def test_deb822_and_mirror_lists_keep_signature_configuration(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "sources.list.d").mkdir()
            source = root / "sources.list.d/ubuntu.sources"
            source.write_text("Types: deb\nURIs: mirror+file:/etc/apt/apt-mirrors.txt\nSuites: noble noble-updates\nSigned-By: /keys/ubuntu.gpg\n")
            (root / "apt-mirrors.txt").write_text("http://azure.archive.ubuntu.com/ubuntu/\n")
            (root / "apt-mirrors-security.txt").write_text("http://azure.archive.ubuntu.com/ubuntu/\n")
            module.configure(root)
            self.assertEqual((root / "apt-mirrors.txt").read_text(), "https://archive.ubuntu.com/ubuntu/\n")
            self.assertEqual((root / "apt-mirrors-security.txt").read_text(), "https://security.ubuntu.com/ubuntu/\n")
            self.assertIn("Signed-By: /keys/ubuntu.gpg", source.read_text())
            self.assertIn("mirror+file:/etc/apt/apt-mirrors.txt", source.read_text())

    def test_only_ubuntu_repository_files_are_changed(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "sources.list.d").mkdir()
            unrelated = root / "sources.list.d/vendor.list"
            text = "deb https://azure.archive.ubuntu.com.evil.invalid/ubuntu stable main\n"
            unrelated.write_text(text)
            module.configure(root)
            self.assertEqual(unrelated.read_text(), text)
            with self.assertRaises(ValueError):
                module.configure(root / "missing")


if __name__ == "__main__":
    unittest.main()
