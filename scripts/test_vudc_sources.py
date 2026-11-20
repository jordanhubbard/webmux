import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import urllib.error

from vudc_sources import acquire, retry_delay, BUNDLED_SOURCES, LIMIT


class Acquisition(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.cache = self.root / 'cache'
        self.manifest = {'commit': 'a' * 40, 'files': {'vudc.c': hashlib.sha256(b'pinned').hexdigest()}}

    def build(self, name, responses, **kwargs):
        root = self.root / name
        root.mkdir()
        calls = []

        def opener(url, timeout):
            calls.append((url, timeout))
            value = responses.pop(0)
            if isinstance(value, Exception):
                raise value
            return io.BytesIO(value)

        acquire(self.manifest, root, self.cache, opener=opener, **kwargs)
        return json.loads((root / 'source-acquisition.json').read_text()), calls

    def error(self, status, retry=None):
        return urllib.error.HTTPError('https://fixture.invalid/source', status, 'fixture',
                                      {} if retry is None else {'Retry-After': retry}, None)

    def test_transient_retry_then_reuse_verified_cache(self):
        waits = []
        receipt, calls = self.build('first', [self.error(429, '2'), b'pinned'], sleep=waits.append)
        self.assertTrue(receipt['passed'])
        self.assertEqual(waits, [2])
        self.assertEqual(len(calls), 2)
        second, calls = self.build('second', [])
        self.assertTrue(second['files'][0]['from_cache'])
        self.assertEqual(calls, [])

    def test_exhausted_retry_retains_failure_and_does_not_cache(self):
        with self.assertRaises(urllib.error.HTTPError):
            self.build('exhausted', [self.error(503)] * 3, sleep=lambda _: None)
        receipt = json.loads((self.root / 'exhausted/source-acquisition.json').read_text())
        self.assertFalse(receipt['passed'])
        self.assertEqual(len(receipt['files'][0]['attempts']), 3)
        self.assertEqual(list(self.cache.iterdir()), [])

    def test_checksum_mismatch_is_never_retried_or_cached(self):
        responses = [b'wrong', b'pinned']
        with self.assertRaisesRegex(RuntimeError, 'checksum/size mismatch'):
            self.build('mismatch', responses)
        self.assertEqual(responses, [b'pinned'])
        self.assertEqual(list(self.cache.iterdir()), [])

    def test_nontransient_and_excessive_retry_after_fail_without_sleep(self):
        for name, error in [('missing', self.error(404)), ('long', self.error(429, '3600'))]:
            responses = [error, b'pinned']
            with self.assertRaises((urllib.error.HTTPError, RuntimeError)):
                self.build(name, responses, sleep=lambda _: self.fail('unexpected retry'))
            self.assertEqual(responses, [b'pinned'])
            self.assertFalse(json.loads((self.root / name / 'source-acquisition.json').read_text())['passed'])

    def test_corrupt_cache_refuses_without_network_or_overwrite(self):
        self.cache.mkdir()
        cached = self.cache / (self.manifest['commit'] + '-vudc.c')
        cached.write_bytes(b'corrupt')
        with self.assertRaisesRegex(RuntimeError, 'checksum/size mismatch'):
            self.build('corrupt', [])
        self.assertEqual(cached.read_bytes(), b'corrupt')
        receipt = json.loads((self.root / 'corrupt/source-acquisition.json').read_text())
        self.assertTrue(receipt['files'][0]['from_cache'])
        self.assertFalse(receipt['passed'])

    def test_timeout_can_recover_and_response_size_is_bounded(self):
        receipt, _ = self.build('timeout', [TimeoutError(), b'pinned'], sleep=lambda _: None)
        self.assertTrue(receipt['passed'])
        # Use a different source identity so this control cannot use the cache.
        self.manifest['commit'] = 'b' * 40
        with self.assertRaisesRegex(RuntimeError, 'checksum/size mismatch'):
            self.build('oversize', [b'x' * ((1 << 20) + 1)])

    @unittest.skipUnless(hasattr(os, 'mkfifo'), 'requires Unix FIFO support')
    def test_cached_fifo_is_refused_without_blocking_or_network(self):
        self.cache.mkdir()
        cached = self.cache / (self.manifest['commit'] + '-vudc.c')
        os.mkfifo(cached)
        root = self.root / 'fifo'
        root.mkdir()
        # Bound the child itself so the old blocking cache read fails this
        # control without hanging the test runner or needing a FIFO writer.
        command = [sys.executable, '-c',
                   'import json,sys; from pathlib import Path; from vudc_sources import acquire; '
                   'acquire(json.loads(sys.argv[1]),Path(sys.argv[2]),Path(sys.argv[3]),'
                   'opener=lambda *a,**k: (_ for _ in ()).throw(AssertionError("network used")))',
                   json.dumps(self.manifest), str(root), str(self.cache)]
        result = subprocess.run(command, cwd=Path(__file__).resolve().parent,
                                capture_output=True, text=True, timeout=5)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('cache entry is not a regular file', result.stderr)
        receipt = json.loads((root / 'source-acquisition.json').read_text())
        self.assertFalse(receipt['passed'])
        self.assertEqual(receipt['error_type'], 'RuntimeError')
        self.assertEqual(receipt['files'][0]['attempts'], [])
        self.assertFalse((root / 'vudc.c').exists())

    def test_linked_directory_and_oversized_cache_entries_refuse_without_fallback(self):
        self.cache.mkdir()
        cached = self.cache / (self.manifest['commit'] + '-vudc.c')
        original = self.root / 'pinned-cache'
        original.write_bytes(b'pinned')
        for kind in ['linked', 'directory', 'oversize']:
            with self.subTest(kind=kind):
                if kind == 'linked':
                    cached.symlink_to(original)
                elif kind == 'directory':
                    cached.mkdir()
                else:
                    cached.write_bytes(b'x' * (LIMIT + 1))
                try:
                    with self.assertRaisesRegex(RuntimeError, 'linked|regular file|checksum/size mismatch'):
                        self.build('cache-' + kind, [])
                    self.assertEqual(original.read_bytes(), b'pinned')
                    receipt = json.loads((self.root / ('cache-' + kind) / 'source-acquisition.json').read_text())
                    self.assertFalse(receipt['passed'])
                    self.assertEqual(receipt['files'][0]['attempts'], [])
                finally:
                    if kind == 'directory':
                        cached.rmdir()
                    else:
                        cached.unlink()

    def test_global_budget_applies_across_retries(self):
        now = [0]
        def clock():
            now[0] += 31
            return now[0]
        with self.assertRaisesRegex(RuntimeError, 'acquisition budget'):
            self.build('budget', [self.error(503)], clock=clock, sleep=lambda _: None)

    def test_retry_after_date_and_invalid_values(self):
        self.assertEqual(retry_delay({'Retry-After': 'Wed, 01 Jan 2020 00:00:00 GMT'}, 1), 0)
        with self.assertRaisesRegex(RuntimeError, 'invalid'):
            retry_delay({'Retry-After': 'garbage'}, 1)

    def test_repository_bundle_builds_a_cold_cache_without_network(self):
        manifest = json.loads((Path(__file__).resolve().parents[1] / 'tests/usb-vudc-sources.json').read_text())
        first = self.root / 'offline'
        first.mkdir()
        def no_network(*args, **kwargs):
            self.fail('bundled fixture sources must not use the network')
        acquire(manifest, first, self.cache, opener=no_network)
        receipt = json.loads((first / 'source-acquisition.json').read_text())
        self.assertTrue(receipt['passed'])
        self.assertEqual(len(receipt['files']), 8)
        self.assertTrue(all(f['from_bundle'] and not f['from_cache'] and not f['attempts'] for f in receipt['files']))
        for name, expected in manifest['files'].items():
            self.assertEqual(hashlib.sha256((first / name).read_bytes()).hexdigest(), expected)
            self.assertEqual((first / name).read_bytes(), (BUNDLED_SOURCES / manifest['commit'] / name).read_bytes())
        second = self.root / 'offline-cache'
        second.mkdir()
        acquire(manifest, second, self.cache, opener=no_network, bundle=None)
        reused = json.loads((second / 'source-acquisition.json').read_text())
        self.assertTrue(all(f['from_cache'] and not f['from_bundle'] for f in reused['files']))

    def test_corrupt_and_oversize_bundles_fail_without_network_or_cache(self):
        for name, data in [('corrupt', b'wrong'), ('oversize', b'x' * (LIMIT + 1))]:
            with self.subTest(name=name):
                bundle = self.root / ('bundle-' + name)
                directory = bundle / self.manifest['commit']
                directory.mkdir(parents=True)
                (directory / 'vudc.c').write_bytes(data)
                target = self.root / name
                target.mkdir()
                with self.assertRaisesRegex(RuntimeError, 'checksum/size mismatch'):
                    acquire(self.manifest, target, self.cache, bundle=bundle,
                            opener=lambda *a, **k: self.fail('corrupt bundle must not fall back to network'))
                self.assertEqual(list(self.cache.iterdir()), [])
                failed = json.loads((target / 'source-acquisition.json').read_text())
                self.assertFalse(failed['passed'])
                self.assertTrue(failed['files'][0]['from_bundle'])

    def test_linked_and_nonregular_bundle_entries_are_refused(self):
        bundle = self.root / 'unsafe-bundle'
        directory = bundle / self.manifest['commit']
        directory.mkdir(parents=True)
        source = directory / 'vudc.c'
        original = self.root / 'pinned'
        original.write_bytes(b'pinned')
        source.symlink_to(original)
        def no_network(*args, **kwargs):
            self.fail('unsafe bundle must not fall back to network')
        for kind in ['linked', 'directory', 'missing']:
            target = self.root / kind
            target.mkdir()
            with self.assertRaisesRegex(RuntimeError, 'linked|regular file|incomplete'):
                acquire(self.manifest, target, self.cache, bundle=bundle, opener=no_network)
            self.assertEqual(list(self.cache.iterdir()), [])
            if kind == 'linked':
                source.unlink()
                source.mkdir()
            elif kind == 'directory':
                source.rmdir()


if __name__ == '__main__':
    unittest.main()
