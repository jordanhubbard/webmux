#!/usr/bin/env python3
"""Keep native stock/reserve failures visible and verify the broken control."""
from collections import Counter
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import struct


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def events(root, name):
    return [json.loads(line) for line in (root / name).read_text(encoding='utf-8-sig').splitlines()
            if line.strip()]


def verify(root):
    root = Path(root)
    result = {'scope': 'Diagnostic comparison only; no supported release runtime or original failure repair claim',
              'originalFailureRepairProven': False, 'passed': False}
    try:
        worker = read(root / 'worker.json')
        expected_source = os.environ.get('VT_RESERVE_SOURCE') or os.environ.get('GITHUB_SHA')
        if expected_source and worker['source'].strip() != expected_source:
            raise ValueError('worker source does not match the workflow head')
        result['worker'] = worker
        source = read(root / 'overlay/source-report.json')
        result['upstreamRevision'] = source['upstreamRevision']
        for name, identity in source['files'].items():
            if hashlib.sha256((Path(source['stockRoot']) / name).read_bytes()).hexdigest() != identity['beforeSHA256']:
                raise ValueError('installed SDK source changed: ' + name)
        result['stockSourcesUnchanged'] = True
        if hashlib.sha256((Path(source['stockRoot']) / 'bin/go.exe').read_bytes()).hexdigest() != worker['compilerSHA256']:
            raise ValueError('installed Go executable changed')
        result['nativeImages'] = {}
        for filename in ['stock-vt.test.exe', 'reserve-vt.test.exe', 'runtime-reserve.test.exe', 'runtime-broken-shrink.test.exe']:
            data = (root / filename).read_bytes()
            offset = struct.unpack_from('<I', data, 0x3c)[0]
            if data[:2] != b'MZ' or data[offset:offset + 4] != b'PE\0\0' or struct.unpack_from('<H', data, offset + 4)[0] != 0x8664:
                raise ValueError('expected actual AMD64 PE executable: ' + filename)
            marker = b'runtime: InitializeContext failed; errno='
            if (marker in data) != (filename != 'stock-vt.test.exe'):
                raise ValueError('compiled reserve marker differs: ' + filename)
            result['nativeImages'][filename] = {'machine': '0x8664', 'SHA256': hashlib.sha256(data).hexdigest()}
        phases = {name: read(root / (name + '-process.json')) for name in ['stock', 'reserve', 'runtime', 'negative']}
        result['nativeProcesses'] = phases
        for name, process in phases.items():
            if process['started'] is not True:
                raise ValueError(name + ' native executable did not start')
        runtime = events(root, 'runtime.jsonl')
        expected = {'TestExceptionStackReserve', 'TestExceptionAfterStackGrowShrink', 'TestExceptionReserveSurvivesShrink'}
        if phases['runtime']['exit_code'] != 0 or any(e['Action'] in ['fail', 'skip'] for e in runtime):
            raise ValueError('proposed runtime regression tests failed or skipped')
        if {e.get('Test') for e in runtime if e['Action'] == 'pass' and e.get('Test')} != expected:
            raise ValueError('runtime regression inventory differs')
        negative = events(root, 'negative.jsonl')
        text = ''.join(e.get('Output', '') for e in negative)
        match = re.search(r'shrinkstack left (\d+) bytes below SP, want at least the (30000)-byte reserve', text)
        shrink = re.search(r'stack (\d+) bytes when parked, (\d+) after resume', text)
        if phases['negative']['exit_code'] != 1 or not match or int(match[1]) >= int(match[2]):
            raise ValueError('broken shrink-accounting control did not fail for the required reason')
        if not shrink or int(shrink[2]) >= int(shrink[1]):
            raise ValueError('broken control did not exercise stack shrinking')
        if not any(e['Action'] == 'fail' and e.get('Test') == 'TestExceptionReserveSurvivesShrink' for e in negative):
            raise ValueError('broken control failed outside the intended regression test')
        result['upstreamRegressionsPassed'] = sorted(expected)
        result['brokenShrinkControlDetected'] = {'headroom': int(match[1]), 'required': int(match[2]),
                                                 'parked': int(shrink[1]), 'resumed': int(shrink[2])}
        inventories = {}
        for name in ['stock', 'reserve']:
            rows = events(root, name + '.jsonl')
            passes = Counter(e['Test'] for e in rows if e['Action'] == 'pass' and e.get('Test'))
            skips = Counter(e['Test'] for e in rows if e['Action'] == 'skip' and e.get('Test'))
            result[name] = {'exitCode': phases[name]['exit_code'], 'testPasses': sum(passes.values()),
                            'skips': dict(skips), 'failedEvents': sum(e['Action'] == 'fail' for e in rows)}
            inventories[name] = passes
            if phases[name]['exit_code'] == 0:
                if sum(passes.values()) != 21860 or skips != Counter({
                    'TestDepDiffVT': 10, 'TestDepDiffResize': 10, 'TestGhosttyParkedCursorJoinsLines': 10}):
                    raise ValueError(name + ' complete test inventory differs')
                if result[name]['failedEvents']:
                    raise ValueError(name + ' contains failed events despite native zero exit')
        stock_ok, reserve_ok = [phases[name]['exit_code'] == 0 for name in ['stock', 'reserve']]
        result['comparison'] = ('both-passed-no-attribution' if stock_ok and reserve_ok else
                                'stock-failed-reserve-passed' if reserve_ok else 'reserve-failed')
        if stock_ok and reserve_ok and inventories['stock'] != inventories['reserve']:
            raise ValueError('stock/reserve passing test multisets differ')
        if not stock_ok or not reserve_ok:
            raise ValueError('a native VT phase failed; retain the failure')
        result['passed'] = True
    except (OSError, ValueError, KeyError, struct.error) as error:
        result['verificationError'] = str(error)
    (root / 'comparison.json').write_text(json.dumps(result, indent=2) + '\n')
    return result


if __name__ == '__main__':
    report = verify(Path(sys.argv[1]))
    print(json.dumps(report, indent=2))
    raise SystemExit(0 if report['passed'] else 1)
