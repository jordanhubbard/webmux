import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('vudc', Path(__file__).with_name('qualify-test-vudc.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


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


if __name__ == '__main__':
    unittest.main()
