import os
from pathlib import Path
import unittest
from unittest.mock import patch

import usb_fixture_context as context


class LocalUSBContext(unittest.TestCase):
    parent = '978640f0-d806-4a77-8d8f-de61ba82d690'
    guest = '61006fbc-01c1-4e15-bdf7-994ba6bc0457'

    def setUp(self):
        self.files = {
            '/.dockerenv': '',
            '/proc/self/status': 'CapEff:\t00000000a80425fb\n',
            '/proc/self/mountinfo': '1 0 0:1 / / rw - overlay overlay rw\n',
            '/proc/sys/kernel/random/boot_id': self.guest + '\n',
            '/proc/cmdline': 'console=ttyAMA0 virtme_hostname=virtme-ng',
            '/proc/device-tree/compatible': b'linux,dummy-virt\0',
        }
        def read(path, *args, **kwargs):
            try:
                return self.files[str(path)]
            except KeyError:
                raise FileNotFoundError(str(path)) from None
        for target, action in [('read_text', read), ('read_bytes', read),
                               ('is_file', lambda p: str(p) in self.files),
                               ('resolve', lambda p: Path('/usr/lib/modules'))]:
            item = patch.object(Path, target, action)
            item.start()
            self.addCleanup(item.stop)
        item = patch.dict(os.environ, {}, clear=True)
        item.start()
        self.addCleanup(item.stop)

    def test_unprivileged_private_overlay_admitted(self):
        self.assertEqual(context.local_container_boot_id(), self.guest)

    def test_hosted_identity_never_accepted_as_local(self):
        for value in ['true', 'false', '']:
            with self.subTest(value=value), patch.dict(os.environ, {'GITHUB_ACTIONS': value}):
                with self.assertRaises(ValueError):
                    context.local_container_boot_id()
                with self.assertRaises(ValueError):
                    context.local_guest(self.parent)

    def test_unknown_or_dangerous_capabilities_refuse(self):
        for value in ['', 'CapEff: unknown', *[f'CapEff: {1 << bit:x}' for bit in [16, 19, 21, 22, 40]]]:
            with self.subTest(value=value):
                self.files['/proc/self/status'] = value
                with self.assertRaises(ValueError):
                    context.local_container_boot_id()

    def test_host_root_and_missing_container_marker_refuse(self):
        self.files['/proc/self/mountinfo'] = '1 0 0:1 / / rw - ext4 /dev/root rw\n'
        with self.assertRaises(ValueError):
            context.local_container_boot_id()
        del self.files['/.dockerenv']
        with self.assertRaises(ValueError):
            context.local_container_boot_id()

    def test_module_bind_mount_or_ancestor_or_descendant_refuses(self):
        original = self.files['/proc/self/mountinfo']
        for mount in ['/usr', '/usr/lib', '/usr/lib/modules', '/usr/lib/modules/fixture']:
            with self.subTest(mount=mount):
                self.files['/proc/self/mountinfo'] = original + f'2 1 0:2 / {mount} rw - ext4 device rw\n'
                with self.assertRaises(ValueError):
                    context.local_container_boot_id()

    def test_qemu_guest_with_different_boot_is_admitted(self):
        context.local_guest(self.parent)

    def test_same_kernel_boot_refuses(self):
        with self.assertRaises(ValueError):
            context.local_guest(self.guest)

    def test_bad_parent_or_guest_boot_id_refuses(self):
        for value in ['', 'not-a-boot-id', self.parent.upper(), self.parent.replace('-', '')]:
            with self.subTest(value=value), self.assertRaises(ValueError):
                context.local_guest(value)
        self.files['/proc/sys/kernel/random/boot_id'] = 'unknown'
        with self.assertRaises(ValueError):
            context.local_guest(self.parent)

    def test_hostname_flag_alone_cannot_admit_non_qemu_host(self):
        del self.files['/proc/device-tree/compatible']
        with self.assertRaises(ValueError):
            context.local_guest(self.parent)
        self.files['/sys/devices/virtual/dmi/id/sys_vendor'] = 'physical-machine'
        with self.assertRaises(ValueError):
            context.local_guest(self.parent)

    def test_virtual_hardware_without_virtme_boot_refuses(self):
        self.files['/proc/cmdline'] = 'console=ttyAMA0 unrelated=virtme_hostname=virtme-ng'
        with self.assertRaises(ValueError):
            context.local_guest(self.parent)

    def test_qemu_dmi_alternative(self):
        del self.files['/proc/device-tree/compatible']
        self.files['/sys/devices/virtual/dmi/id/sys_vendor'] = 'QEMU\n'
        context.local_guest(self.parent)


if __name__ == '__main__':
    unittest.main()
