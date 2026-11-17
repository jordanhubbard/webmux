#!/usr/bin/env python3
"""Build a pinned, corrected virtual USB gadget for disposable kernel tests only."""
import argparse
import hashlib
import json
from pathlib import Path
import platform
import re
import subprocess
import urllib.request


def digest(data):
    return hashlib.sha256(data).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--kernel', required=True)
    parser.add_argument('--build-root', required=True, type=Path)
    args = parser.parse_args()
    if platform.system() != 'Linux' or not re.fullmatch(r'[A-Za-z0-9.+_-]+', args.kernel):
        parser.error('requires Linux and a kernel release name')
    root = args.build_root.resolve()
    if root.exists() and (not root.is_dir() or any(root.iterdir())):
        parser.error('build-root must be new or empty; preserve previous evidence')
    headers = Path('/lib/modules', args.kernel, 'build')
    if not headers.is_dir():
        parser.error('matching target kernel headers are required')
    tests = Path(__file__).resolve().parents[1] / 'tests'
    manifest = json.loads((tests / 'usb-vudc-sources.json').read_text())
    root.mkdir(parents=True, exist_ok=True)
    for name, expected in manifest['files'].items():
        url = f"https://raw.githubusercontent.com/torvalds/linux/{manifest['commit']}/drivers/usb/usbip/{name}"
        with urllib.request.urlopen(url, timeout=20) as response:
            data = response.read(1 << 20)
        if digest(data) != expected:
            raise RuntimeError(f'kernel fixture source checksum mismatch: {name}')
        (root / name).write_bytes(data)
    patch = (tests / 'usb-vudc.patch').read_bytes()
    subprocess.run(['patch', '--batch', '--fuzz=0', '-p1'], cwd=root, input=patch, check=True, timeout=10)
    (root / 'Makefile').write_text('obj-m := usbip-vudc.o\nusbip-vudc-y := vudc_main.o vudc_dev.o vudc_sysfs.o vudc_rx.o vudc_tx.o vudc_transfer.o\n')
    with (root / 'build.log').open('wb') as log:
        subprocess.run(['make', '-C', str(headers), 'M=' + str(root), '-j2', 'modules'],
                       stdout=log, stderr=log, check=True, timeout=180)
    module = root / 'usbip-vudc.ko'
    vermagic = subprocess.check_output(['modinfo', '-F', 'vermagic', str(module)], text=True, timeout=10).strip()
    if vermagic.split()[0] != args.kernel:
        raise RuntimeError('built module targets a different kernel')
    receipt = {'kernel': args.kernel, 'source': manifest, 'patch_sha256': digest(patch),
               'module_sha256': digest(module.read_bytes()), 'vermagic': vermagic,
               'scope': 'disposable virtual gadget; not a WebMux runtime dependency'}
    (root / 'receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
    print(f"Built corrected virtual gadget: {module}; receipt: {root / 'receipt.json'}")


if __name__ == '__main__':
    main()
