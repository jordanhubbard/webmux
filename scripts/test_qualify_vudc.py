import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('vudc', Path(__file__).with_name('qualify-test-vudc.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class NativeAdmission(unittest.TestCase):
    def test_only_expected_native_reattach_failure_is_admitted(self):
        log = '\n'.join(['Loaded gadget srcversion verified: abc',
                         'Genuine native reply queued; direct read is blocked',
                         'HELD REPLY REATTACH: beginning replacement import',
                         'cannot find a urb of seqnum 210 max seqnum 1',
                         'Timed out: USB import after held-reply disconnect'])
        self.assertTrue(module.admits_loss(1, log))
        for status in (0, 2, 124, -9):
            self.assertFalse(module.admits_loss(status, log))
        for line in log.splitlines():
            self.assertFalse(module.admits_loss(1, log.replace(line, '')))

    def test_old_reply_may_arrive_before_import_handshake(self):
        log = '\n'.join(['Loaded gadget srcversion verified: abc',
                         'Genuine native reply queued; direct read is blocked',
                         'HELD REPLY REATTACH: beginning replacement import',
                         'usbip: error: USBIP Kernel and tool version mismatch: 0 273:',
                         'usbip: error: Attach Request for usbip-vudc.0 failed'])
        self.assertTrue(module.admits_loss(1, log))
        self.assertFalse(module.admits_loss(124, log))
        self.assertFalse(module.admits_loss(1, log.replace('Genuine native reply queued;', '')))
        self.assertFalse(module.admits_loss(1, log.replace('HELD REPLY REATTACH:', '')))

    def test_repair_requires_actual_discard_and_successful_new_import(self):
        log = '\n'.join(['Loaded gadget srcversion verified: abc',
                         'Genuine native reply queued; direct read is blocked',
                         'discarded 1 stale USB/IP replies',
                         'PASS: held native reply discarded; replacement import and exact payload verified'])
        self.assertTrue(module.admits_repair(0, log))
        self.assertFalse(module.admits_repair(1, log))
        self.assertFalse(module.admits_repair(0, log.replace('discarded 1', 'discarded 0')))
        for line in log.splitlines():
            self.assertFalse(module.admits_repair(0, log.replace(line, '')))


if __name__ == '__main__':
    unittest.main()
