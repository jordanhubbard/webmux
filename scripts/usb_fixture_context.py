"""Admission for local USB tests: an unprivileged container and a distinct VM.

These checks prevent accidental module installation/testing on the local host.
They are not an authentication boundary against a hostile root operator.
"""
import os
from pathlib import Path
import re
import uuid


def boot_id(value):
    value = value.strip()
    if str(uuid.UUID(value)) != value:
        raise ValueError('expected a canonical boot ID')
    return value


def local_container_boot_id():
    if 'GITHUB_ACTIONS' in os.environ:
        raise ValueError('local execution must not claim a GitHub Actions identity')
    if not Path('/.dockerenv').is_file():
        raise ValueError('local qualification requires a disposable Docker container')
    status = Path('/proc/self/status').read_text()
    match = re.search(r'^CapEff:\s*([0-9a-fA-F]+)$', status, re.M)
    # A subset of Docker's ordinary default capabilities is sufficient. Reject
    # added capabilities, including module loading, ptrace, mounts and reboot.
    allowed = 0xa80425fb
    if not match or int(match[1], 16) & ~allowed:
        raise ValueError('container has unknown or unsafe host capabilities')
    mounts = []
    for line in Path('/proc/self/mountinfo').read_text().splitlines():
        before, after = line.split(' - ', 1)
        mounts.append((Path(before.split()[4]), after.split()[0]))
    if (Path('/'), 'overlay') not in mounts:
        raise ValueError('container root must be a private overlay filesystem')
    modules = Path('/lib/modules').resolve()
    if any(m != Path('/') and (m == modules or m.is_relative_to(modules)
                              or modules.is_relative_to(m)) for m, _ in mounts):
        raise ValueError('refusing an externally mounted module directory')
    return boot_id(Path('/proc/sys/kernel/random/boot_id').read_text())


def local_guest(parent_boot):
    if 'GITHUB_ACTIONS' in os.environ:
        raise ValueError('local guest must not claim a GitHub Actions identity')
    parent_boot = boot_id(parent_boot)
    current = boot_id(Path('/proc/sys/kernel/random/boot_id').read_text())
    if current == parent_boot:
        raise ValueError('USB test must run in a separate booted kernel')
    if 'virtme_hostname=virtme-ng' not in Path('/proc/cmdline').read_text().split():
        raise ValueError('missing virtme guest kernel identity')
    device_tree = Path('/proc/device-tree/compatible')
    vendor = Path('/sys/devices/virtual/dmi/id/sys_vendor')
    if not ((device_tree.is_file() and b'linux,dummy-virt' in device_tree.read_bytes().split(b'\0'))
            or (vendor.is_file() and vendor.read_text().strip() == 'QEMU')):
        raise ValueError('local USB test requires a QEMU virtual machine')
