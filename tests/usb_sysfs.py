"""Read the disposable USB fixture's imported disk while devices come and go."""
import errno
from pathlib import Path


def imported_disk(block_root=Path('/sys/class/block')):
    for entry in block_root.iterdir():
        try:
            resolved = entry.resolve()
        except OSError as error:
            # Detach can remove a link after iterdir yielded its name.
            if error.errno in (errno.ENOENT, errno.ENODEV):
                continue
            raise
        if 'vhci_hcd' not in str(resolved) or (entry / 'partition').exists():
            continue
        for parent in resolved.parents:
            try:
                serial = (parent / 'serial').read_text().strip()
            except OSError as error:
                # sysfs entries can disappear after enumeration during detach.
                if error.errno in (errno.ENOENT, errno.ENODEV):
                    continue
                raise
            if serial == 'webmux-ci-storage':
                return Path('/dev', entry.name)
    return None


def imported_disk_with_payload(expected, discover=imported_disk):
    """Ready only after the current fixture disk opens and returns exact bytes.

    sysfs registration can precede /dev creation, or disappear during detach.
    Re-discover on the next existing poll; do not retry bad bytes or other I/O.
    """
    if not expected:
        raise ValueError('A nonempty fixture payload is required')
    disk = discover()
    if disk is None:
        return None
    try:
        opened = disk.open('rb', buffering=0)
    except OSError as error:
        # Linux can expose the node before its block device is live. ENXIO
        # from open is absence; ENXIO from an established read remains fatal.
        if error.errno in (errno.ENOENT, errno.ENODEV, errno.ENXIO):
            return None
        raise
    try:
        with opened as source:
            actual = source.read(len(expected))
    except OSError as error:
        if error.errno in (errno.ENOENT, errno.ENODEV):
            return None
        raise
    if actual != expected:
        raise RuntimeError('Imported fixture payload mismatch')
    return disk
