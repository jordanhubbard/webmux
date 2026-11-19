import importlib.util
import io
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import shutil
import ssl
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location('rig_stop_relay', Path(__file__).with_name('qualify-rig-stop-relay.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class RelayCommandEvidence(unittest.TestCase):
    def test_success_retains_both_streams_and_result(self):
        log = io.StringIO()
        result = module.run_recorded([sys.executable, '-c',
                                    'import sys; print("ready"); print("diagnostic", file=sys.stderr)'], log)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, 'ready\n')
        self.assertEqual(result.stderr, 'diagnostic\n')
        event = json.loads(log.getvalue().splitlines()[-1])
        self.assertEqual(event['stdout'], 'ready\n')
        self.assertEqual(event['stderr'], 'diagnostic\n')

    def test_nonzero_exit_remains_failure_after_output_is_logged(self):
        log = io.StringIO()
        with self.assertRaisesRegex(RuntimeError, r'failed \(7\)'):
            module.run_recorded([sys.executable, '-c', 'print("before exit"); raise SystemExit(7)'], log)
        self.assertIn('before exit', log.getvalue())
        self.assertIn('"exit": 7', log.getvalue())

    def test_nonzero_inspection_can_still_return_its_result(self):
        log = io.StringIO()
        result = module.run_recorded([sys.executable, '-c', 'raise SystemExit(7)'], log, check=False)
        self.assertEqual(result.returncode, 7)
        self.assertIn('"exit": 7', log.getvalue())

    def test_timeout_keeps_partial_streams_and_original_timeout(self):
        log = io.StringIO()
        command = [sys.executable, '-c',
                   'import os,time; os.write(1,b"build-started\\n"); '
                   'os.write(2,b"package-progress\\n\\xe2\\x82"); time.sleep(60)']
        with self.assertRaises(subprocess.TimeoutExpired) as raised:
            module.run_recorded(command, log, timeout=2)
        self.assertEqual(raised.exception.cmd, command)
        self.assertEqual(raised.exception.timeout, 2)
        self.assertEqual(raised.exception.stdout, b'build-started\n')
        self.assertEqual(raised.exception.stderr, b'package-progress\n\xe2\x82')
        event = json.loads(log.getvalue().splitlines()[-1])
        self.assertEqual(event['stdout'], 'build-started\n')
        self.assertEqual(event['stderr'], 'package-progress\n\ufffd')
        records = [json.loads(line) for line in log.getvalue().splitlines() if line.startswith('{')]
        self.assertEqual(records[-1]['event'], 'command_timeout')
        self.assertEqual(records[-1]['timeoutSeconds'], 2)
        self.assertEqual(records[-1]['commandId'], records[0]['commandId'])
        self.assertNotIn('"exit": 0', log.getvalue())


class RelayBuildInputs(unittest.TestCase):
    def fixture(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        return Path(directory.name)

    def test_https_bootstrap_retains_public_roots_and_ubuntu_signing_key(self):
        root = self.fixture()
        receipt = module.write_build_inputs(root)
        bundle = (root / 'ca-certificates.crt').read_bytes()
        verifier = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        verifier.load_verify_locations(cadata=bundle.decode('ascii'))
        self.assertEqual(len(verifier.get_ca_certs()), receipt['publicHostCAUniqueCertificates'])
        self.assertEqual(hashlib.sha256(bundle).hexdigest(), receipt['publicHostCABundleSHA256'])
        self.assertNotIn(b'PRIVATE KEY', bundle)
        sources = (root / 'ubuntu.sources').read_text()
        self.assertIn('URIs: https://archive.ubuntu.com/ubuntu/', sources)
        self.assertIn('URIs: https://security.ubuntu.com/ubuntu/', sources)
        self.assertEqual(sources.count('Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg'), 2)
        self.assertNotIn('http://', sources)
        settings = (root / 'apt-ci.conf').read_text()
        for value in ['Acquire::https::Timeout "30";', 'Acquire::Retries "2";',
                      'APT::Update::Error-Mode "any";', 'Dir::Etc::sourcelist "-";',
                      'Acquire::https::CAInfo "/etc/ssl/certs/ca-certificates.crt";']:
            self.assertIn(value, settings)
        for value in ['Verify-Peer', 'Verify-Host', 'AllowUnauthenticated', 'AllowInsecure', 'trusted=yes']:
            self.assertNotIn(value, sources + settings)
        dockerfile = (root / 'Dockerfile').read_text()
        self.assertTrue(dockerfile.startswith('FROM ' + module.BASE_IMAGE + '\n'))
        self.assertIn('test "$VERSION_CODENAME" = noble', dockerfile)
        self.assertIn('--no-install-recommends openssh-server tmux', dockerfile)

    def test_empty_or_oversized_trust_refuses_before_writing_build_inputs(self):
        for certificates in [[], [b'x'] * 4097, [b'x' * (4 * 1024 * 1024)]]:
            with self.subTest(count=len(certificates)):
                root = self.fixture()
                context = SimpleNamespace(get_ca_certs=lambda **kwargs: certificates)
                with patch.object(module.ssl, 'create_default_context', return_value=context):
                    with self.assertRaisesRegex(RuntimeError, 'unavailable|oversized'):
                        module.write_build_inputs(root)
                self.assertEqual(list(root.iterdir()), [])

    def test_context_does_not_admit_generated_private_key_or_other_files(self):
        root = self.fixture()
        module.write_build_inputs(root)
        ignore = (root / '.dockerignore').read_text().splitlines()
        self.assertEqual(ignore[0], '*')
        self.assertEqual(set(ignore[1:]), {'!Dockerfile', '!client.pub', '!sshd_config',
                                          '!ca-certificates.crt', '!ubuntu.sources', '!apt-ci.conf'})
        self.assertNotIn('COPY .', (root / 'Dockerfile').read_text())

    def test_effective_apt_selection_uses_https_noble_and_ignores_original_feeds(self):
        if not shutil.which('apt-get'):
            self.skipTest('APT executable unavailable on this platform')
        root = self.fixture()
        module.write_build_inputs(root)
        sources = root / 'sources'
        sources.mkdir()
        (sources / 'ubuntu.sources').write_bytes((root / 'ubuntu.sources').read_bytes())
        lists = root / 'lists'
        lists.mkdir()
        result = subprocess.run(['apt-get', '-c', str(root / 'apt-ci.conf'),
                                 '-o', f'Dir::Etc::sourceparts={sources}',
                                 '-o', f'Dir::State::Lists={lists}', '-o', 'Debug::NoLocking=1',
                                 'update', '--print-uris'], capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode, 0, result.stderr)
        for suite in ['noble', 'noble-updates', 'noble-backports']:
            self.assertIn(f'https://archive.ubuntu.com/ubuntu/dists/{suite}/InRelease', result.stdout)
        self.assertIn('https://security.ubuntu.com/ubuntu/dists/noble-security/InRelease', result.stdout)
        self.assertNotIn('http://', result.stdout)


if __name__ == '__main__':
    unittest.main()
