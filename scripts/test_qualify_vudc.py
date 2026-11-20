import importlib.util
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
import copy

spec = importlib.util.spec_from_file_location('vudc', Path(__file__).with_name('qualify-test-vudc.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class PersistentSourceCache(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.source = self.root / 'source'
        scripts = self.source / 'scripts'
        scripts.mkdir(parents=True)
        self.cache = self.root / 'shared-cache'
        self.cache.mkdir()
        self.body = b'public fixture source\n'
        self.manifest = {'commit': '1' * 40,
                         'files': {'vudc.c': hashlib.sha256(self.body).hexdigest()}}
        (self.source / 'fixture-source-manifest.json').write_text(json.dumps(self.manifest))
        self.cached = self.cache / (self.manifest['commit'] + '-vudc.c')
        self.cached.write_bytes(self.body)
        # A child at the native-build boundary runs the real source verifier.
        # It refuses all network access and does not pretend to build a module.
        (scripts / 'build-test-vudc.py').write_text(
            'import argparse,json,sys\nfrom pathlib import Path\n'
            f'sys.path.insert(0, {str(Path(module.__file__).parent)!r})\n'
            'from vudc_sources import acquire\n'
            'p=argparse.ArgumentParser()\n'
            "p.add_argument('--kernel');p.add_argument('--build-root',type=Path)\n"
            "p.add_argument('--source-cache',type=Path)\n"
            "p.add_argument('--unpatched',action='store_true')\n"
            "p.add_argument('--hold-replies',action='store_true')\na=p.parse_args()\n"
            "manifest=json.loads((Path(__file__).parents[1]/'fixture-source-manifest.json').read_text())\n"
            'a.build_root.mkdir(parents=True)\n'
            "def no_network(*args,**kwargs):raise AssertionError('unexpected source download')\n"
            'acquire(manifest,a.build_root,a.source_cache,opener=no_network)\n')

    def test_three_independent_builds_reuse_verified_external_cache_without_network(self):
        for name, options in [('unpatched', ['--unpatched', '--hold-replies']),
                              ('corrected-held', ['--hold-replies']), ('corrected', [])]:
            build = self.root / name / 'build'
            module.build_fixture(self.source, '6.8.0-fixture', build, self.cache, options)
            self.assertEqual((build / 'vudc.c').read_bytes(), self.body)
            receipt = json.loads((build / 'source-acquisition.json').read_text())
            self.assertTrue(receipt['passed'])
            self.assertTrue(receipt['files'][0]['from_cache'])
            self.assertEqual(receipt['files'][0]['attempts'], [])
            self.assertEqual(receipt['files'][0]['verified_sha256'],
                             self.manifest['files']['vudc.c'])
        self.assertEqual(self.cached.read_bytes(), self.body)

    def test_restored_corrupt_bytes_refuse_without_download_or_replacement(self):
        self.cached.write_bytes(b'corrupt')
        build = self.root / 'bad' / 'build'
        with self.assertRaises(subprocess.CalledProcessError):
            module.build_fixture(self.source, '6.8.0-fixture', build, self.cache, [])
        self.assertFalse((build / 'vudc.c').exists())
        self.assertEqual(self.cached.read_bytes(), b'corrupt')
        receipt = json.loads((build / 'source-acquisition.json').read_text())
        self.assertFalse(receipt['passed'])
        self.assertTrue(receipt['files'][0]['from_cache'])
        self.assertEqual(receipt['files'][0]['attempts'], [])


class NativeAdmission(unittest.TestCase):
    phases = ['Loaded gadget srcversion verified: abc',
              'Genuine native reply queued; direct read is blocked',
              'Held native read interrupted by detach',
              'HELD REPLY REATTACH: beginning replacement import',
              'Replacement import acknowledged while native TX remained held',
              'Releasing held native replies after import acknowledgment']
    loss = ['cannot find a urb of seqnum 210 max seqnum 1',
            'Timed out: USB import after held-reply disconnect']
    repaired = ['discarded 1 stale USB/IP replies',
                'PASS: held native reply discarded; replacement import and exact payload verified']

    def test_only_expected_native_reattach_failure_is_admitted(self):
        lines = self.phases + self.loss
        log = '\n'.join(lines)
        self.assertTrue(module.admits_loss(1, log))
        for status in (0, 2, 124, -9):
            self.assertFalse(module.admits_loss(status, log))
        for line in lines:
            self.assertFalse(module.admits_loss(1, log.replace(line, '')))

    def test_handshake_corruption_never_qualifies_as_data_path_loss(self):
        for error in ('usbip: error: USBIP Kernel and tool version mismatch: 0 273:',
                      'usbip: error: Attach Request for usbip-vudc.0 failed',
                      'usbip: error: recv different busid'):
            self.assertFalse(module.admits_loss(1, '\n'.join(self.phases + [error])))
            self.assertFalse(module.admits_loss(1, '\n'.join(self.phases + [error] + self.loss)))
            self.assertFalse(module.admits_repair(0, '\n'.join(self.phases + [error] + self.repaired)))

    def test_phase_order_is_required(self):
        for i in range(len(self.phases) - 1):
            phases = self.phases.copy()
            phases[i], phases[i + 1] = phases[i + 1], phases[i]
            self.assertFalse(module.admits_loss(1, '\n'.join(phases + self.loss)))
            self.assertFalse(module.admits_repair(0, '\n'.join(phases + self.repaired)))

    def test_legacy_pre_handshake_release_receipts_are_not_admitted(self):
        old = ['Loaded gadget srcversion verified: abc',
               'Genuine native reply queued; direct read is blocked',
               'HELD REPLY REATTACH: beginning replacement import']
        self.assertFalse(module.admits_loss(1, '\n'.join(old + self.loss)))
        self.assertFalse(module.admits_repair(0, '\n'.join(old + self.repaired)))

    def test_repair_requires_actual_discard_and_successful_new_import(self):
        lines = self.phases + self.repaired
        log = '\n'.join(lines)
        self.assertTrue(module.admits_repair(0, log))
        self.assertFalse(module.admits_repair(1, log))
        self.assertFalse(module.admits_repair(0, log.replace('discarded 1', 'discarded 0')))
        for line in lines:
            self.assertFalse(module.admits_repair(0, log.replace(line, '')))
        # The correction discards the old queue during teardown, before the
        # replacement handshake. Kernel console ordering is independent.
        self.assertTrue(module.admits_repair(0, '\n'.join(self.repaired[:1] + self.phases + self.repaired[1:])))

    def test_dashboard_and_data_path_both_require_completion(self):
        data = 'PASS: virtual USB enumeration, read/write, detach, interruption and worker cleanup'
        dashboard = 'PASS: dashboard pairing, controller-loss continuity, owner Stop and confirmed listener release'
        self.assertTrue(module.admits_webmux(0, data + '\\n' + dashboard))
        self.assertFalse(module.admits_webmux(0, data))
        self.assertFalse(module.admits_webmux(0, dashboard))
        self.assertFalse(module.admits_webmux(1, data + '\\n' + dashboard))
        self.assertFalse(module.admits_webmux(0, data + '\\n' + dashboard + '\\nOops: fault'))

    def test_kernel_faults_cannot_be_admitted_as_transport_failures_or_passes(self):
        for fault in ('BUG: kernel NULL pointer dereference', 'Oops: 0000',
                      'Kernel panic - not syncing', 'general protection fault'):
            self.assertFalse(module.kernel_clean('PASS: transport completed\n' + fault))
        self.assertTrue(module.kernel_clean('module verification failed: signature missing - tainting kernel'))


class HeldCounterAdmission(unittest.TestCase):
    pre = {'queued': 1, 'before': 0, 'readerBlocked': True}
    state = {'before': 0, 'counter': {'text': '0\n', 'truncated': False},
             'hold': {'text': 'Y\n', 'truncated': False},
             'prequeuePause': {'text': 'N\n', 'truncated': False},
             'queuedCompletions': {'text': '1\n', 'truncated': False}, 'readerExit': None}

    def failure(self, state=None, pre=None):
        return ('Loaded gadget srcversion verified: fixture\nPREQUEUE_CONTROL ' + json.dumps(self.pre if pre is None else pre)
                + '\nHELD_REPLY_DIAGNOSTIC ' + json.dumps(self.state if state is None else state)
                + '\nRuntimeError: Timed out: real native completion queued under TX hold\n')

    def test_counter_gap_requires_real_queued_reply_hold_and_unpaused_transmitter(self):
        log = self.failure()
        self.assertTrue(module.admits_counter_gap(1, log))
        self.assertTrue(module.admits_counter_gap(1, log.replace('\n', '\r\n')))
        for status in [0, 2, 124, -9]:
            self.assertFalse(module.admits_counter_gap(status, log))
        for key in self.state:
            state = copy.deepcopy(self.state)
            del state[key]
            self.assertFalse(module.admits_counter_gap(1, self.failure(state)))
        for key, value in [('counter', '1\n'), ('hold', 'N\n'), ('prequeuePause', 'Y\n'), ('queuedCompletions', '0\n')]:
            state = copy.deepcopy(self.state)
            state[key]['text'] = value
            self.assertFalse(module.admits_counter_gap(1, self.failure(state)))
            state = copy.deepcopy(self.state)
            state[key]['truncated'] = True
            self.assertFalse(module.admits_counter_gap(1, self.failure(state)))

    def test_unrelated_timeout_handshake_or_kernel_fault_cannot_establish_counter_gap(self):
        log = self.failure()
        for marker in ['PREQUEUE_CONTROL', 'HELD_REPLY_DIAGNOSTIC', 'Loaded gadget srcversion verified:',
                       'real native completion queued under TX hold']:
            self.assertFalse(module.admits_counter_gap(1, log.replace(marker, 'other')))
        for error in ['BUG: fault', 'Oops: fault', 'Kernel panic', 'recv different busid',
                      'Genuine native reply queued; direct read is blocked']:
            self.assertFalse(module.admits_counter_gap(1, log + error))
        state = copy.deepcopy(self.state)
        state['readerExit'] = 1
        self.assertFalse(module.admits_counter_gap(1, self.failure(state)))
        pre = copy.deepcopy(self.pre)
        pre['queued'] = 0
        self.assertFalse(module.admits_counter_gap(1, self.failure(pre=pre)))

    def test_fixed_counter_requires_original_repair_and_actual_prequeue_comparison(self):
        repair = '\n'.join(NativeAdmission.phases + NativeAdmission.repaired)
        log = repair + '\nPREQUEUE_CONTROL ' + json.dumps(self.pre) + '\nPREQUEUE_COUNTER ' + json.dumps({'held': 1, 'queued': 1})
        self.assertTrue(module.admits_counted_prequeue(0, log))
        self.assertFalse(module.admits_counted_prequeue(1, log))
        self.assertFalse(module.admits_counted_prequeue(0, repair))
        self.assertFalse(module.admits_counted_prequeue(0, log.replace('"held": 1', '"held": 2')))
        for marker in NativeAdmission.phases + NativeAdmission.repaired:
            self.assertFalse(module.admits_counted_prequeue(0, log.replace(marker, 'other')))


if __name__ == '__main__':
    unittest.main()
