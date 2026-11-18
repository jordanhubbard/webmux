import errno
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from usb_sysfs import imported_disk, imported_disk_with_payload


class ImportedDiskTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        root = Path(self.temporary.name).resolve()
        self.blocks = root / 'block'
        self.blocks.mkdir()
        self.device = root / 'vhci_hcd' / 'usb1' / '1-1'
        self.disk = self.device / 'block' / 'sda'
        self.disk.mkdir(parents=True)
        (self.blocks / 'sda').symlink_to(self.disk, target_is_directory=True)
        self.serial = self.device / 'serial'
        self.serial.write_text('webmux-ci-storage\n')

    def test_identifies_only_the_fixture_whole_disk(self):
        self.assertEqual(imported_disk(self.blocks), Path('/dev/sda'))
        (self.disk / 'partition').write_text('1')
        self.assertIsNone(imported_disk(self.blocks))
        (self.disk / 'partition').unlink()
        self.serial.write_text('another-device')
        self.assertIsNone(imported_disk(self.blocks))

    def test_detach_during_serial_read_is_not_a_failure(self):
        original = Path.read_text
        for code in (errno.ENOENT, errno.ENODEV):
            with self.subTest(errno=code):
                def read(path, *args, **kwargs):
                    if path == self.serial:
                        raise OSError(code, 'device disappeared')
                    return original(path, *args, **kwargs)
                with patch.object(Path, 'read_text', read):
                    self.assertIsNone(imported_disk(self.blocks))

    def test_unexpected_io_errors_remain_failures(self):
        original = Path.read_text
        for code in (errno.EACCES, errno.EIO):
            with self.subTest(errno=code):
                def read(path, *args, **kwargs):
                    if path == self.serial:
                        raise OSError(code, 'unexpected error')
                    return original(path, *args, **kwargs)
                with patch.object(Path, 'read_text', read):
                    with self.assertRaises(OSError) as raised:
                        imported_disk(self.blocks)
                    self.assertEqual(raised.exception.errno, code)


class ReadableDiskTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.disk = self.root / 'sda'
        self.payload = bytes(range(256))

    def test_sysfs_before_devnode_is_not_ready_then_exact_bytes_are_ready(self):
        # Reproduce the old select-then-open failure using an actual absent node.
        with self.assertRaises(FileNotFoundError):
            with self.disk.open('rb', buffering=0):
                pass
        self.assertIsNone(imported_disk_with_payload(self.payload, lambda: self.disk))
        self.disk.write_bytes(self.payload)
        self.assertEqual(imported_disk_with_payload(self.payload, lambda: self.disk), self.disk)

    def test_each_poll_rediscovers_the_current_fixture(self):
        replacement = self.root / 'sdb'
        replacement.write_bytes(self.payload)
        discover = iter([self.disk, replacement])
        self.assertIsNone(imported_disk_with_payload(self.payload, lambda: next(discover)))
        self.assertEqual(imported_disk_with_payload(self.payload, lambda: next(discover)), replacement)

    def test_absent_or_unmatched_fixture_does_not_open_a_device(self):
        with patch.object(Path, 'open', side_effect=AssertionError('must not open')):
            self.assertIsNone(imported_disk_with_payload(self.payload, lambda: None))

    def test_disappearance_during_open_or_read_is_not_readiness(self):
        for code in (errno.ENOENT, errno.ENODEV):
            with self.subTest(errno=code, phase='open'):
                with patch.object(Path, 'open', side_effect=OSError(code, 'disappeared')):
                    self.assertIsNone(imported_disk_with_payload(self.payload, lambda: self.disk))
            with self.subTest(errno=code, phase='read'):
                with patch.object(Path, 'open') as opened:
                    opened.return_value.__enter__.return_value.read.side_effect = OSError(code, 'disappeared')
                    self.assertIsNone(imported_disk_with_payload(self.payload, lambda: self.disk))

    def test_permission_and_unexpected_io_errors_fail_immediately(self):
        for code in (errno.EACCES, errno.EIO, errno.ENXIO, None):
            with self.subTest(errno=code):
                with patch.object(Path, 'open', side_effect=OSError(code, 'unexpected')):
                    with self.assertRaises(OSError) as raised:
                        imported_disk_with_payload(self.payload, lambda: self.disk)
                    self.assertEqual(raised.exception.errno, code)

    def test_short_or_wrong_payload_is_a_failure_not_another_poll(self):
        for actual in (self.payload[:-1], b'x' * len(self.payload)):
            with self.subTest(length=len(actual)):
                self.disk.write_bytes(actual)
                with self.assertRaisesRegex(RuntimeError, 'payload mismatch'):
                    imported_disk_with_payload(self.payload, lambda: self.disk)

    def test_empty_expected_bytes_cannot_establish_readiness(self):
        with self.assertRaises(ValueError):
            imported_disk_with_payload(b'', lambda: self.disk)


if __name__ == '__main__':
    unittest.main()
