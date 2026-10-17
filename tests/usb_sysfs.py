"""Read the disposable USB fixture's imported disk while devices come and go."""
import errno
from pathlib import Path


def imported_disk(block_root=Path('/sys/class/block')):
    for entry in block_root.iterdir():
        resolved = entry.resolve()
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
