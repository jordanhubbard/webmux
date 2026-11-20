import errno
from io import BytesIO
from pathlib import Path
import unittest
from unittest.mock import Mock, patch

from usb_hold_diagnostics import held_reply_diagnostic


class HeldReplyDiagnostic(unittest.TestCase):
    def test_reads_are_bounded_and_process_arguments_are_not_read(self):
        paths = []
        def opened(path, *args):
            paths.append(str(path))
            return BytesIO(b'x' * 10000)
        reader = Mock(pid=123)
        reader.poll.return_value = None
        with patch.object(Path, 'open', opened), patch.object(Path, 'iterdir', return_value=iter([])):
            d = held_reply_diagnostic(Path('/parameters'), 0, reader, Path('/dev/sda'))
        self.assertTrue(d['counter']['truncated'])
        self.assertEqual(len(d['counter']['text']), 2048)
        self.assertIsNone(d['readerExit'])
        self.assertFalse(any(p.endswith(('cmdline', 'environ')) for p in paths))

    def test_failed_reads_remain_observations(self):
        reader = Mock(pid=123)
        reader.poll.return_value = 1
        with patch.object(Path, 'open', side_effect=PermissionError(errno.EACCES, 'private message')), \
                patch.object(Path, 'iterdir', side_effect=FileNotFoundError(errno.ENOENT, 'private message')):
            d = held_reply_diagnostic(Path('/parameters'), 3, reader, Path('/dev/sda'))
        self.assertEqual(d['counter'], {'errno': errno.EACCES})
        self.assertEqual(d['taskScanErrno'], errno.ENOENT)
        self.assertNotIn('private message', str(d))


if __name__ == '__main__':
    unittest.main()
