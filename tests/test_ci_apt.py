import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("ci_apt", Path(__file__).resolve().parents[1] / "scripts" / "configure-ci-apt.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class AptConfigurationTest(unittest.TestCase):
    def fixture(self):
        context = tempfile.TemporaryDirectory()
        self.addCleanup(context.cleanup)
        root = Path(context.name)
        (root / "sources.list.d").mkdir()
        return root

    def test_hosted_mirrors_use_azure_without_redirecting_arm64_ports(self):
        # Both runner formats and release/security pockets must avoid the
        # central endpoints; ARM64 needs the separate ports archive.
        for uri in [
            "http://azure.archive.ubuntu.com/ubuntu",
            "https://azure.archive.ubuntu.com/ubuntu/",
            "http://archive.ubuntu.com/ubuntu",
            "https://security.ubuntu.com/ubuntu/",
            "mirror+file:/etc/apt/apt-mirrors.txt",
            "mirror+file:/etc/apt/apt-mirrors-security.txt",
        ]:
            with self.subTest(uri=uri):
                self.assertEqual(module.ubuntu_uri(uri), "https://azure.archive.ubuntu.com/ubuntu/")
        self.assertEqual(module.ubuntu_uri("http://ports.ubuntu.com/ubuntu-ports"),
                         "https://ports.ubuntu.com/ubuntu-ports/")

    def test_legacy_sources_preserve_suites_keys_and_third_parties(self):
        root = self.fixture()
        original = "deb [arch=amd64 signed-by=/keys/ubuntu.gpg] http://azure.archive.ubuntu.com/ubuntu jammy main\ndeb http://security.ubuntu.com/ubuntu jammy-security main\ndeb http://127.0.0.1:9/unavailable stable main\n"
        source = root / "sources.list"
        source.write_text(original)
        module.configure(root)
        self.assertEqual(source.read_text(), original)
        selected = (root / "webmux-ci-sources/ubuntu.list").read_text()
        self.assertIn("[arch=amd64 signed-by=/keys/ubuntu.gpg] https://azure.archive.ubuntu.com/ubuntu/ jammy main", selected)
        self.assertIn("https://azure.archive.ubuntu.com/ubuntu/ jammy-security main", selected)
        self.assertNotIn("unavailable", selected)
        settings = (root / "apt.conf.d/99-webmux-ci-network").read_text()
        for setting in ['Acquire::https::Timeout "30";', 'Dpkg::Lock::Timeout "120";', 'APT::Update::Error-Mode "any";', 'Dir::Etc::sourcelist "-";']:
            self.assertIn(setting, settings)
        self.assertNotIn("AllowUnauthenticated", settings)
        self.assertNotIn("AllowInsecure", settings)

    def test_mixed_deb822_retains_keys_and_excludes_vendor_uris(self):
        root = self.fixture()
        source = root / "sources.list.d/mixed.sources"
        original = "Types: deb\nURIs: mirror+file:/etc/apt/apt-mirrors.txt\n https://packages.microsoft.com/ubuntu/22.04/prod\nSuites: noble noble-updates\nComponents: main universe\nArchitectures: amd64\nSigned-By: /keys/ubuntu.gpg\n\nTypes: deb\nURIs: https://security.ubuntu.com/ubuntu\nSuites: noble-security\nComponents: main\nSigned-By:\n -----BEGIN PGP PUBLIC KEY BLOCK-----\n .\n key-body\n -----END PGP PUBLIC KEY BLOCK-----\n"
        source.write_text(original)
        mirror = root / "apt-mirrors.txt"
        mirror.write_text("http://azure.archive.ubuntu.com/ubuntu/\n")
        module.configure(root)
        selected = (root / "webmux-ci-sources/ubuntu.sources").read_text()
        self.assertEqual(source.read_text(), original)
        self.assertEqual(mirror.read_text(), "http://azure.archive.ubuntu.com/ubuntu/\n")
        self.assertNotIn("microsoft", selected)
        for value in ["URIs: https://azure.archive.ubuntu.com/ubuntu/", "Suites: noble noble-updates", "Architectures: amd64", "Signed-By: /keys/ubuntu.gpg", "Signed-By: \n -----BEGIN PGP PUBLIC KEY BLOCK-----\n .\n key-body\n -----END PGP PUBLIC KEY BLOCK-----"]:
            self.assertIn(value, selected)

    def test_disabled_and_lookalike_sources_do_not_become_ubuntu(self):
        for uri in ["https://azure.archive.ubuntu.com.evil.invalid/ubuntu", "https://azure.archive.ubuntu.com/ubuntu-evil", "https://user@azure.archive.ubuntu.com/ubuntu", "https://archive.ubuntu.com.evil.invalid/ubuntu", "https://archive.ubuntu.com/ubuntu-evil", "https://user@archive.ubuntu.com/ubuntu", "https://archive.ubuntu.com:443/ubuntu", "https://archive.ubuntu.com/ubuntu?mirror=evil"]:
            self.assertIsNone(module.ubuntu_uri(uri), uri)
        root = self.fixture()
        (root / "sources.list.d/disabled.sources").write_text("Types: deb\nURIs: https://archive.ubuntu.com/ubuntu\nSuites: jammy\nEnabled: no\n")
        with self.assertRaisesRegex(ValueError, "No enabled"):
            module.configure(root)
        self.assertFalse((root / "apt.conf.d").exists())

    def test_signature_bypass_is_refused(self):
        for text in ["deb [trusted=yes] https://archive.ubuntu.com/ubuntu jammy main\n", "deb [allow-insecure=true] https://archive.ubuntu.com/ubuntu jammy main\n"]:
            with self.assertRaisesRegex(ValueError, "signature"):
                module.legacy_sources(text)
        for flag in ["Trusted", "Allow-Insecure", "Allow-Weak", "Allow-Downgrade-To-Insecure"]:
            with self.assertRaisesRegex(ValueError, "signature"):
                module.deb822_sources(f"Types: deb\nURIs: https://archive.ubuntu.com/ubuntu\nSuites: jammy\n{flag}: yes\n")

    def test_idempotence_and_private_symlink_refusal(self):
        root = self.fixture()
        (root / "sources.list").write_text("deb https://archive.ubuntu.com/ubuntu jammy main\n")
        module.configure(root)
        first = {p: p.read_bytes() for p in root.rglob("*") if p.is_file()}
        module.configure(root)
        self.assertEqual(first, {p: p.read_bytes() for p in root.rglob("*") if p.is_file()})
        private = root / "webmux-ci-sources/ubuntu.list"
        private.unlink()
        private.symlink_to(root / "sources.list")
        with self.assertRaisesRegex(ValueError, "symlink"):
            module.configure(root)

    def test_system_entrypoint_requires_explicit_ci_context(self):
        env = {key: value for key, value in os.environ.items() if key != "GITHUB_ACTIONS"}
        result = subprocess.run([sys.executable, str(Path(module.__file__)), "/etc/apt"],
                                env=env, text=True, capture_output=True, timeout=5)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("restricted to ephemeral GitHub CI", result.stderr)

    def test_effective_apt_selection_preserves_signed_ubuntu_and_ignores_dead_feed(self):
        if not shutil.which("apt-get"):
            self.skipTest("APT executable unavailable on this platform")
        root = self.fixture()
        (root / "sources.list").write_text("deb [signed-by=/keys/ubuntu.gpg] https://archive.ubuntu.com/ubuntu jammy main\n")
        (root / "sources.list.d/vendor.list").write_text("deb http://127.0.0.1:9/unavailable stable main\n")
        (root / "sources.list.d/security.sources").write_text("Types: deb\nURIs: https://security.ubuntu.com/ubuntu\nSuites: jammy-security\nComponents: main\nSigned-By: /keys/ubuntu.gpg\n")
        module.configure(root)
        lists = root / "lists"
        lists.mkdir()
        result = subprocess.run(["apt-get", "-c", str(root / "apt.conf.d/99-webmux-ci-network"), "-o", f"Dir::State::Lists={lists}", "-o", "Debug::NoLocking=1", "update", "--print-uris"], text=True, capture_output=True, timeout=15)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("https://azure.archive.ubuntu.com/ubuntu/dists/jammy/InRelease", result.stdout)
        self.assertIn("https://azure.archive.ubuntu.com/ubuntu/dists/jammy-security/InRelease", result.stdout)
        self.assertNotIn("127.0.0.1", result.stdout)
        self.assertNotIn("unavailable", result.stdout)


if __name__ == "__main__":
    unittest.main()
