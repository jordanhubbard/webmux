import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("ci_apt", Path(__file__).resolve().parents[1] / "scripts" / "configure-ci-apt.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class AptConfigurationTest(unittest.TestCase):
    def test_ci_scope_preserves_mixed_formats_and_signing_policy(self):
        with tempfile.TemporaryDirectory(prefix="apt scope ") as directory:
            root = Path(directory)
            parts = root / "sources.list.d"
            parts.mkdir()
            legacy = "deb [arch=amd64 signed-by=/keys/ubuntu.gpg] http://archive.ubuntu.com/ubuntu jammy main universe\n"
            vendor = "deb https://packages.microsoft.com/ubuntu/22.04/prod jammy main\n"
            (root / "sources.list").write_text(legacy + vendor)
            key = "Signed-By:\n -----BEGIN PGP PUBLIC KEY BLOCK-----\n .\n fixture-key\n -----END PGP PUBLIC KEY BLOCK-----\n"
            (parts / "mixed.sources").write_text("Types: deb deb-src\nURIs: http://ports.ubuntu.com/ubuntu-ports\n# alternate mirror\n https://vendor.invalid/packages\nSuites: noble noble-updates\nComponents: main universe\nArchitectures: arm64\nEnabled: no\n" + key + "\nTypes: deb\nURIs: https://archive.ubuntu.com.evil.invalid/ubuntu\nSuites: stable\nComponents: main\n")
            module.configure(root, ubuntu_only=True)
            selected = root / "webmux-ci-sources"
            self.assertEqual((selected / "ubuntu.list").read_text(), legacy.replace("http://archive.ubuntu.com/ubuntu", "https://archive.ubuntu.com/ubuntu/"))
            result = (selected / "ubuntu.sources").read_text()
            self.assertIn("URIs: https://ports.ubuntu.com/ubuntu-ports/\n", result)
            self.assertIn("Suites: noble noble-updates\n", result)
            self.assertIn("Architectures: arm64\n", result)
            self.assertIn("Enabled: no\n", result)
            self.assertIn(key, result)
            self.assertNotIn("vendor.invalid", result)
            self.assertNotIn("evil.invalid", result)
            self.assertIn(vendor, (root / "sources.list").read_text())
            self.assertIn("https://vendor.invalid/packages", (parts / "mixed.sources").read_text())
            settings = (root / "apt.conf.d/99-webmux-ci-network").read_text()
            self.assertIn('Dir::Etc::sourcelist "/dev/null";', settings)
            self.assertIn(str(selected.resolve()), settings)
            self.assertIn('APT::Update::Error-Mode "any";', settings)
            self.assertNotIn("AllowInsecure", settings)

    def test_ci_snapshot_removes_stale_formats_and_is_idempotent(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "sources.list").write_text("deb https://archive.ubuntu.com/ubuntu jammy main\n")
            module.configure(root, ubuntu_only=True)
            (root / "sources.list").unlink()
            (root / "sources.list.d").mkdir()
            (root / "sources.list.d/ubuntu.sources").write_text("Types: deb\nURIs: https://security.ubuntu.com/ubuntu\nSuites: jammy-security\nComponents: main\nSigned-By: /keys/ubuntu.gpg\n")
            module.configure(root, ubuntu_only=True)
            selected = root / "webmux-ci-sources"
            before = {p.name: p.read_bytes() for p in selected.iterdir()}
            self.assertEqual(before['ubuntu.list'], b"")
            module.configure(root, ubuntu_only=True)
            self.assertEqual(before, {p.name: p.read_bytes() for p in selected.iterdir()})

    def test_ci_scope_fails_closed_without_ubuntu_or_with_ambiguous_uris(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            parts = root / "sources.list.d"
            parts.mkdir()
            source = parts / "vendor.sources"
            source.write_text("Types: deb\nURIs: https://vendor.invalid/ubuntu\nSuites: jammy\nComponents: main\n")
            with self.assertRaisesRegex(ValueError, "No configured official Ubuntu"):
                module.configure(root, ubuntu_only=True)
            self.assertFalse((root / "webmux-ci-sources").exists())
            source.write_text("Types: deb\nURIs: https://archive.ubuntu.com/ubuntu\nURIs: https://vendor.invalid/ubuntu\nSuites: jammy\n")
            with self.assertRaisesRegex(ValueError, "Duplicate Deb822"):
                module.configure(root, ubuntu_only=True)

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
            module.configure(root, ubuntu_only=True)
            selected = (root / "webmux-ci-sources/ubuntu.sources").read_text()
            self.assertIn("URIs: mirror+file:/etc/apt/apt-mirrors.txt", selected)
            self.assertIn("Signed-By: /keys/ubuntu.gpg", selected)

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
