import importlib.util
import io
import hashlib
import json
import os
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
        self.assertEqual(sources.count('URIs: https://azure.archive.ubuntu.com/ubuntu/'), 2)
        self.assertIn('Suites: noble-security', sources)
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
                with patch.object(module, 'SYSTEM_CA_BUNDLE', root / 'absent-system-bundle'), \
                        patch.object(module.ssl, 'create_default_context', return_value=context):
                    with self.assertRaisesRegex(RuntimeError, 'unavailable|oversized'):
                        module.write_build_inputs(root)
                self.assertEqual(list(root.iterdir()), [])
        for content, error in [(b'x' * (4 * 1024 * 1024 + 1), RuntimeError),
                               (b'not a public certificate', ssl.SSLError)]:
            with self.subTest(system_bundle_bytes=len(content)):
                root = self.fixture()
                public_bundle = root / 'system.pem'
                public_bundle.write_bytes(content)
                with patch.object(module, 'SYSTEM_CA_BUNDLE', public_bundle):
                    with self.assertRaises(error):
                        module.write_build_inputs(root)
                self.assertFalse((root / 'Dockerfile').exists())

    def test_system_bundle_is_used_when_python_default_store_is_empty(self):
        original = self.fixture()
        module.write_build_inputs(original)
        public_bundle = original / 'ca-certificates.crt'
        empty_context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        self.assertEqual(empty_context.get_ca_certs(), [])
        root = self.fixture()
        with patch.object(module, 'SYSTEM_CA_BUNDLE', public_bundle, create=True), \
                patch.object(module.ssl, 'create_default_context', return_value=empty_context):
            receipt = module.write_build_inputs(root)
        self.assertGreater(receipt['publicHostCAUniqueCertificates'], 0)
        self.assertEqual((root / 'ca-certificates.crt').read_bytes(), public_bundle.read_bytes())
        self.assertEqual(receipt['publicHostCATrustSource'], 'system-public-pem-bundle')

    def test_actual_unavailable_default_paths_use_exact_public_system_root(self):
        root = self.fixture()
        context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        if module.SYSTEM_CA_BUNDLE.is_file():
            context.load_verify_locations(cafile=str(module.SYSTEM_CA_BUNDLE))
        else:
            context = ssl.create_default_context()
        certificates = context.get_ca_certs(binary_form=True)
        self.assertTrue(certificates)
        public_bundle = root / 'system.pem'
        public_bundle.write_text(ssl.DER_cert_to_PEM_cert(certificates[0]))
        output = root / 'build'
        output.mkdir()
        with patch.dict(os.environ, {'SSL_CERT_FILE': str(root / 'absent.pem'),
                                     'SSL_CERT_DIR': str(root / 'absent-directory')}):
            self.assertEqual(ssl.create_default_context().get_ca_certs(binary_form=True), [])
            with patch.object(module, 'SYSTEM_CA_BUNDLE', public_bundle):
                receipt = module.write_build_inputs(output)
        self.assertEqual(receipt['publicHostCAUniqueCertificates'], 1)
        self.assertEqual(receipt['publicHostCATrustSource'], 'system-public-pem-bundle')
        verifier = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        verifier.load_verify_locations(cafile=str(output / 'ca-certificates.crt'))
        self.assertEqual(verifier.get_ca_certs(binary_form=True), [certificates[0]])
        self.assertEqual(verifier.verify_mode, ssl.CERT_REQUIRED)
        self.assertTrue(verifier.check_hostname)

    def test_actual_missing_system_and_default_stores_refuse_before_writes(self):
        root = self.fixture()
        with patch.dict(os.environ, {'SSL_CERT_FILE': str(root / 'absent.pem'),
                                     'SSL_CERT_DIR': str(root / 'absent-directory')}), \
                patch.object(module, 'SYSTEM_CA_BUNDLE', root / 'absent-system.pem'):
            with self.assertRaisesRegex(RuntimeError, 'unavailable'):
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
            self.assertIn(f'https://azure.archive.ubuntu.com/ubuntu/dists/{suite}/InRelease', result.stdout)
        self.assertIn('https://azure.archive.ubuntu.com/ubuntu/dists/noble-security/InRelease', result.stdout)
        self.assertNotIn('http://', result.stdout)


if __name__ == '__main__':
    unittest.main()
