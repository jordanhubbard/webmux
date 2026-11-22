#!/usr/bin/env python3
"""Hold the shell between opening a PID marker and writing its contents."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

results = []
for atomic in (False, True):
    with tempfile.TemporaryDirectory() as root:
        marker = Path(root) / 'child.pid'
        target = str(marker) + ('.pending' if atomic else '')
        publish = '/bin/mv -f "$TARGET" "$MARKER";' if atomic else ''
        script = ('exec 3> "$TARGET"; printf "opened\\n"; read release; '
                  'echo $$ >&3; exec 3>&-; ' + publish +
                  ' printf "published\\n"; read finish')
        child = subprocess.Popen(['/bin/sh', '-c', script],
                                 env=dict(os.environ, TARGET=target, MARKER=str(marker)),
                                 stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
        try:
            assert child.stdout.readline() == 'opened\n'
            try:
                visible = marker.read_text()
                legacy_breaks = True
            except FileNotFoundError:
                visible = None
                legacy_breaks = False
            child.stdin.write('release\n')
            child.stdin.flush()
            assert child.stdout.readline() == 'published\n'
            assert marker.read_text().strip() == str(child.pid)
            results.append({
                'publication': 'atomic-rename' if atomic else 'legacy-direct-redirection',
                'while_writer_paused': {
                    'read_succeeds': legacy_breaks,
                    'bytes': len(visible) if visible is not None else None,
                    'legacy_loop_breaks_without_pid': legacy_breaks and visible.strip() == '',
                },
                'after_publication': 'complete exact writer PID',
            })
            child.stdin.write('finish\n')
            child.stdin.flush()
            assert child.wait(timeout=3) == 0
        finally:
            if child.poll() is None:
                child.terminate()
                child.wait(timeout=3)
assert results[0]['while_writer_paused']['legacy_loop_breaks_without_pid']
assert not results[1]['while_writer_paused']['read_succeeds']
print(json.dumps(results, indent=2))
