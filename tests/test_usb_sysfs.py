import errno
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from usb_sysfs import imported_disk


class ImportedDiskTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        root = Path(self.temporary.name)
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


if __name__ == '__main__':
    unittest.main()
