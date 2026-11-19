#!/usr/bin/env python3
"""Prepare pinned diagnostic Go source overlays without modifying the SDK."""
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import shutil
import stat
import subprocess

DOCUMENTS = Path(__file__).resolve().parents[1] / 'docs/validation/vt-stack-reserve-2026-11-18'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def prepare(goroot, output, documents=DOCUMENTS):
    goroot, output = Path(goroot).absolute(), Path(output).absolute()
    lock = json.loads((documents / 'overlay-lock.json').read_text())
    if lock['schema'] != 'webmux-go-reserve-overlay/v1' or len(lock['files']) != 14:
        raise ValueError('incomplete diagnostic source inventory')
    if (goroot / 'VERSION').read_text().splitlines()[0] != lock['goVersion']:
        raise ValueError('stock SDK version differs')
    if output.resolve().is_relative_to(goroot.resolve()):
        raise ValueError('output must be outside the stock SDK')
    if output.exists():
        raise ValueError('output already exists; preserve prior evidence')
    patch = documents / 'go-reserve.patch'
    if sha(patch) != lock['patchSHA256']:
        raise ValueError('upstream patch identity differs')
    if sha(documents / 'GO-LICENSE.txt') != lock['licenseSHA256']:
        raise ValueError('Go license identity differs')
    # Admit every input before writing an overlay. No missing-file or changed-
    # version fallback can silently produce a partial runtime modification.
    for name, identity in lock['files'].items():
        relative = PurePosixPath(name)
        if relative.is_absolute() or '..' in relative.parts or relative.parts[0] != 'src' or '\\' in name:
            raise ValueError('invalid diagnostic source path')
        source = goroot / name
        if not stat.S_ISREG(source.lstat().st_mode) or sha(source) != identity['beforeSHA256']:
            raise ValueError('stock SDK source identity differs: ' + name)
    output.mkdir(parents=True)
    source_root = output / 'source'
    for name in lock['files']:
        target = source_root / name
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(goroot / name, target)
    # An output under a source checkout must not discover its enclosing Git
    # repository and apply paths relative to that repository instead.
    environment = {k: v for k, v in os.environ.items() if not k.upper().startswith('GIT_')}
    environment['GIT_CEILING_DIRECTORIES'] = str(output)
    for flags in (['--check'], []):
        result = subprocess.run(['git', '-c', 'core.autocrlf=false', '-C', str(source_root),
                                 'apply', *flags, str(patch.resolve())], env=environment,
                                capture_output=True, text=True, timeout=20)
        if result.returncode:
            (output / 'patch-error.txt').write_text(result.stdout + result.stderr)
            raise ValueError('pinned patch does not apply exactly; retain patch-error.txt')
    replacement = {}
    for name, identity in lock['files'].items():
        patched = source_root / name
        if sha(patched) != identity['afterSHA256']:
            raise ValueError('patched SDK source identity differs: ' + name)
        replacement[str(goroot / name)] = str(patched)
    (output / 'reserve-overlay.json').write_text(json.dumps({'Replace': replacement}, indent=2) + '\n')
    control = lock['negativeControl']
    # Source includes UTF-8 comments. Never decode through the Windows locale
    # or translate newlines while constructing a hash-pinned byte control.
    original = (source_root / control['path']).read_bytes()
    anchor = control['removeExactly'].encode('utf-8')
    if original.count(anchor) != 1:
        raise ValueError('negative control shrink-accounting anchor differs')
    negative = output / 'negative-source' / control['path']
    negative.parent.mkdir(parents=True)
    negative.write_bytes(original.replace(anchor, b'', 1))
    if sha(negative) != control['afterSHA256']:
        raise ValueError('negative control source identity differs')
    broken = dict(replacement)
    broken[str(goroot / control['path'])] = str(negative)
    (output / 'negative-overlay.json').write_text(json.dumps({'Replace': broken}, indent=2) + '\n')
    # Read back all installed source bytes after preparation too.
    if any(sha(goroot / name) != identity['beforeSHA256'] for name, identity in lock['files'].items()):
        raise ValueError('stock SDK source changed')
    report = {**lock, 'stockRoot': str(goroot), 'stockSourcesUnchanged': True,
              'reserveOverlay': str(output / 'reserve-overlay.json'),
              'negativeOverlay': str(output / 'negative-overlay.json')}
    shutil.copyfile(documents / 'GO-LICENSE.txt', output / 'GO-LICENSE.txt')
    (output / 'source-report.json').write_text(json.dumps(report, indent=2) + '\n')
    return report


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--goroot', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    arguments = parser.parse_args()
    result = prepare(arguments.goroot, arguments.output)
    print(json.dumps({'role': result['role'], 'upstreamRevision': result['upstreamRevision'],
                      'stockSourcesUnchanged': result['stockSourcesUnchanged'],
                      'sourceFiles': len(result['files'])}))
