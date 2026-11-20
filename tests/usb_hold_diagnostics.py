"""Read-only, bounded observations after the native held-reply assertion fails."""
from pathlib import Path


def held_reply_diagnostic(parameters, before, reader, disk):
    def read(path):
        try:
            with path.open('rb') as source:
                data = source.read(2049)
            return {'text': data[:2048].decode(errors='replace'), 'truncated': len(data) > 2048}
        except OSError as error:
            return {'errno': error.errno}

    result = {
        'before': before,
        'counter': read(parameters / 'webmux_test_held_replies'),
        'hold': read(parameters / 'webmux_test_hold_replies'),
        'readerExit': reader.poll(),
        'readerWait': read(Path('/proc', str(reader.pid), 'wchan')),
        'readerStack': read(Path('/proc', str(reader.pid), 'stack')),
        'diskInflight': read(Path('/sys/class/block', disk.name, 'inflight')),
        'diskStat': read(Path('/sys/class/block', disk.name, 'stat')),
    }
    # No process arguments, environments, output or unrelated process names.
    result['kernelTasks'] = []
    try:
        count = 0
        for task in Path('/proc').iterdir():
            if not task.name.isdecimal():
                continue
            count += 1
            if count > 256 or len(result['kernelTasks']) >= 8:
                result['taskScanTruncated'] = True
                break
            name = read(task / 'comm').get('text', '').strip()
            if name not in {'vudc_tx', 'vudc_rx', 'usb-storage', 'usbip_event'}:
                continue
            result['kernelTasks'].append({'name': name, 'wait': read(task / 'wchan'),
                                          'stack': read(task / 'stack')})
    except OSError as error:
        result['taskScanErrno'] = error.errno
    return result
