#!/usr/bin/env python3
"""Qualify the test gadget's real stale-reply path, then ordinary USB over SSH."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shlex
import shutil
import signal
import subprocess
import sys


def kernel_clean(log):
    return re.search(r'BUG:|Oops:|Kernel panic|general protection fault', log) is None


def stale_phases_proven(log):
    # These progress records share the fixture's flushed userspace stream.
    # Kernel messages can arrive on another console channel, so their textual
    # order relative to these markers is not used as a clock.
    phases = ('Loaded gadget srcversion verified:',
              'Genuine native reply queued; direct read is blocked',
              'Held native read interrupted by detach',
              'HELD REPLY REATTACH: beginning replacement import',
              'Replacement import acknowledged while native TX remained held',
              'Releasing held native replies after import acknowledgment')
    offset = 0
    for phase in phases:
        found = log.find(phase, offset)
        if found < 0:
            return False
        offset = found + len(phase)
    return True


def handshake_clean(log):
    return re.search(r'USBIP Kernel and tool version mismatch|recv different busid|Attach Request for .* failed', log) is None


def admits_loss(status, log):
    return (status == 1 and kernel_clean(log) and handshake_clean(log)
            and stale_phases_proven(log)
            and 'cannot find a urb of seqnum' in log
            and 'Timed out: USB import after held-reply disconnect' in log
            and 'PASS: held native reply discarded;' not in log)


def admits_repair(status, log):
    return (status == 0 and kernel_clean(log) and handshake_clean(log)
            and stale_phases_proven(log)
            and re.search(r'discarded [1-9][0-9]* stale USB/IP replies', log) is not None
            and 'PASS: held native reply discarded; replacement import and exact payload verified' in log)


def build_fixture(source, kernel, build, cache, options):
    # The cache may outlive one disposable build, but acquire() verifies every
    # restored byte against the source manifest before compilation.
    subprocess.run([sys.executable, str(source / 'scripts/build-test-vudc.py'),
                    '--kernel', kernel, '--build-root', str(build),
                    '--source-cache', str(cache), *options], check=True, timeout=450)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--kernel', required=True)
    parser.add_argument('--vm-runner', required=True, type=Path)
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--build-root', required=True, type=Path)
    parser.add_argument('--source-cache', type=Path, help='optional persistent checksum-verified public source cache')
    args = parser.parse_args()
    if (platform.system() != 'Linux' or os.geteuid() != 0
            or os.environ.get('GITHUB_ACTIONS') != 'true'
            or not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9.+_-]*', args.kernel)
            or args.kernel == platform.release()):
        parser.error('requires a disposable Linux GitHub runner and a separate guest kernel')
    root = args.build_root.resolve()
    if root.exists() and (not root.is_dir() or any(root.iterdir())):
        parser.error('build-root must be new or empty; preserve previous evidence')
    runner, binary = args.vm_runner.resolve(strict=True), args.binary.resolve(strict=True)
    source = Path(__file__).resolve().parents[1]
    target = Path('/lib/modules', args.kernel, 'updates/webmux/usbip-vudc.ko')
    if target.exists():
        parser.error('refusing to replace an existing private module')
    root.mkdir(parents=True, exist_ok=True)
    target.parent.mkdir(parents=True, exist_ok=True)
    results = []
    owned_digest = None
    env = {**os.environ, 'PATH': str(runner.parent) + os.pathsep + os.environ['PATH'],
           'PYTHONUNBUFFERED': '1'}

    def install(module):
        nonlocal owned_digest
        if target.exists() and hashlib.sha256(target.read_bytes()).hexdigest() != owned_digest:
            raise RuntimeError('installed private module changed outside this qualification')
        stage = target.with_suffix('.stage')
        shutil.copyfile(module, stage)
        stage.chmod(0o644)
        os.replace(stage, target)
        owned_digest = hashlib.sha256(target.read_bytes()).hexdigest()
        subprocess.run(['depmod', '-a', args.kernel], check=True, timeout=30)
        selected = subprocess.check_output(['modinfo', '-k', args.kernel, '-F', 'filename', 'usbip-vudc'], text=True, timeout=10).strip()
        if Path(selected).resolve() != target.resolve():
            raise RuntimeError('guest kernel will select a different module')

    def guest(name, receipt, mode, admit):
        log_path = root / (name + '.log')
        guest_command = shlex.join(['env', 'GITHUB_ACTIONS=true',
                                   'WEBMUX_USB_EXPECTED_SRCVERSION=' + receipt['srcversion'],
                                   'python3', str(source / 'tests/usb-kernel.py'), str(binary), *mode])
        command = ['timeout', '--kill-after=15s', '180s', str(runner), '--run', args.kernel,
                   '--user', 'root', '--empty-passwords', '--memory', '1G', '--cpus', '2',
                   '--verbose', '--exec', guest_command]
        print(f'Running {name}; log: {log_path}', flush=True)
        # vng exposes stdout/stderr to the guest independently. A seekable
        # regular file can have its prefix overwritten by the other stream;
        # a pipe preserves both streams and the loaded-module identity.
        child = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                 env=env, start_new_session=True)
        try:
            captured, _ = child.communicate(timeout=210)
        except subprocess.TimeoutExpired:
            os.killpg(child.pid, signal.SIGKILL)
            captured, _ = child.communicate(timeout=10)
            log_path.write_bytes(captured)
            raise RuntimeError(f'{name}: outer observation deadline expired')
        status = child.returncode
        log_path.write_bytes(captured)
        output = log_path.read_text(errors='replace')
        admitted = kernel_clean(output) and bool(admit(status, output))
        results.append({'control': name, 'exit': status, 'admitted': admitted,
                        'module_sha256': receipt['module_sha256'], 'srcversion': receipt['srcversion']})
        (root / 'receipt.json').write_text(json.dumps({'results': results}, indent=2) + '\n')
        print(output, end='', flush=True)
        if not admitted:
            raise RuntimeError(f'{name}: unexpected native outcome; retained {log_path}')
        print(f'{name}: expected native outcome verified', flush=True)

    for name, options in [('unpatched', ['--unpatched', '--hold-replies']),
                          ('corrected-held', ['--hold-replies']), ('corrected', [])]:
        build = root / name
        build_fixture(source, args.kernel, build,
                      args.source_cache.resolve() if args.source_cache else root / 'source-cache', options)
        receipt = json.loads((build / 'receipt.json').read_text())
        install(build / 'usbip-vudc.ko')
        if name == 'unpatched':
            guest(name, receipt, ['--stale-control'], admits_loss)
        elif name == 'corrected-held':
            guest(name, receipt, ['--stale-control'], admits_repair)
        else:
            guest('direct-20', receipt, ['--direct-control'], lambda status, log:
                  status == 0 and 'PASS: 20 direct USB/IP enumeration, read/write and detach cycles' in log)
            guest('webmux-ssh', receipt, [], lambda status, log:
                  status == 0 and 'PASS: virtual USB enumeration, read/write, detach, interruption and worker cleanup' in log)


if __name__ == '__main__':
    main()
