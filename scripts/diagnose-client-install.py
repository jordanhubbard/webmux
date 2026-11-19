#!/usr/bin/env python3
"""Observe unchanged helper tests on a disposable hosted Mac; never qualify a failure."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shlex
import shutil
import signal
import subprocess
import sys
import time


def record(path, value):
    data = (json.dumps(value, separators=(',', ':')) + '\n').encode()
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
    try:
        os.write(fd, data)
    finally:
        os.close(fd)


def proxy(log, executable, args):
    started = time.monotonic()
    child = subprocess.Popen([executable, *args])
    record(log, dict(event='go-start', unix=time.time(), proxyPID=os.getpid(),
                     parentPID=os.getppid(), childPID=child.pid,
                     action=args[0] if args and args[0] in ['build', 'test', 'env', 'version', 'run'] else 'other'))
    status = child.wait()
    record(log, dict(event='go-exit', unix=time.time(), childPID=child.pid,
                     elapsedSeconds=time.monotonic() - started, status=status))
    if status < 0:
        os.kill(os.getpid(), -status)
    return status


def write_proxy(output, executable):
    directory = output / 'proxy-bin'
    directory.mkdir(mode=0o700)
    wrapper = directory / 'go'
    wrapper.write_text('#!/bin/sh\nexec ' + ' '.join(shlex.quote(x) for x in
                       [sys.executable, str(Path(__file__).resolve()), 'proxy',
                        str(output / 'go-events.jsonl'), executable]) + ' "$@"\n')
    wrapper.chmod(0o700)
    return wrapper


def observe(command, cwd, env, output, label, limit):
    """Bound the outer observer; individual test/build deadlines stay unchanged."""
    start = time.monotonic()
    unix_start = time.time()
    with (output / (label + '.log')).open('wb') as log:
        child = subprocess.Popen(command, cwd=cwd, env=env, stdout=log,
                                 stderr=subprocess.STDOUT, start_new_session=True)
        timed_out = False
        samples = 0
        dropped = 0
        sample_path = output / (label + '-processes.jsonl')
        try:
            while child.poll() is None:
                if time.monotonic() - start >= limit:
                    timed_out = True
                    break
                ps = subprocess.run(['/bin/ps', '-axo', 'pid=,ppid=,time=,comm='],
                                    capture_output=True, text=True, timeout=3, check=True)
                rows = [line.split(None, 3) for line in ps.stdout.splitlines()]
                rows = [r for r in rows if len(r) == 4]
                owned = {child.pid}
                while True:
                    found = {int(r[0]) for r in rows if int(r[1]) in owned}
                    if found <= owned:
                        break
                    owned |= found
                processes = [dict(pid=int(r[0]), parentPID=int(r[1]), cpuTime=r[2],
                                  executable=Path(r[3]).name) for r in rows if int(r[0]) in owned]
                if not sample_path.exists() or sample_path.stat().st_size < 8 * 1024**2:
                    record(sample_path, dict(elapsedSeconds=time.monotonic() - start,
                                            loadAverage=os.getloadavg(), processes=processes))
                    samples += 1
                else:
                    dropped += 1
                time.sleep(1)
        finally:
            if child.poll() is None:
                # Only this freshly created process group; never find/kill by name.
                try:
                    os.killpg(child.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
                try:
                    child.wait(timeout=3)
                except subprocess.TimeoutExpired:
                    try:
                        os.killpg(child.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                    child.wait(timeout=3)
        return dict(command=label, startedUnix=unix_start, status=child.returncode, observerTimedOut=timed_out,
                    elapsedSeconds=time.monotonic() - start, samples=samples,
                    droppedSamples=dropped)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--cache-mode', required=True, choices=['cold', 'warm'])
    args = parser.parse_args()
    repo = Path(__file__).resolve().parent.parent
    if (platform.system() != 'Darwin' or platform.machine() != 'arm64'
            or os.environ.get('GITHUB_ACTIONS') != 'true'
            or os.environ.get('RUNNER_ENVIRONMENT') != 'github-hosted'
            or Path(os.environ.get('GITHUB_WORKSPACE', '/missing')).resolve() != repo):
        raise SystemExit('Requires the clean checkout on a disposable GitHub-hosted ARM64 Mac')
    output = args.output.resolve()
    temp = Path(os.environ['RUNNER_TEMP']).resolve()
    if temp not in output.parents or output.exists():
        raise SystemExit('Output must be a new directory beneath RUNNER_TEMP')
    if subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=normal'], cwd=repo):
        raise SystemExit('Diagnostic requires a clean checkout')
    output.mkdir(mode=0o700)
    go = shutil.which('go')
    if not go:
        raise SystemExit('Go is required')
    cache = output / 'go-cache'
    cache.mkdir(mode=0o700)
    wrapper = write_proxy(output, go)
    # Makefile assigns GO := go, so an environment GO override is ineffective.
    # Resolve the real tool first, then observe PATH lookups without adding flags.
    env = dict(os.environ, GOCACHE=str(cache),
               PATH=str(wrapper.parent) + os.pathsep + os.environ.get('PATH', ''))
    go_environment = json.loads(subprocess.check_output(
        [go, 'env', '-json', 'GOOS', 'GOARCH', 'GOHOSTARCH'], cwd=repo / 'webmux/server',
        env=env, text=True, timeout=10))
    node_environment = json.loads(subprocess.check_output(
        ['node', '-p', 'JSON.stringify({platform:process.platform,architecture:process.arch,version:process.version})'],
        text=True, timeout=10))
    if (go_environment != dict(GOOS='darwin', GOARCH='arm64', GOHOSTARCH='arm64')
            or node_environment['platform'] != 'darwin' or node_environment['architecture'] != 'arm64'):
        raise SystemExit('Comparison requires native ARM64 Go and Node')
    files = ['Makefile', 'scripts/make-controls.test.mts', 'webmux/server/go.mod',
             'webmux/server/go.sum', 'scripts/diagnose-client-install.py']
    summary = dict(source=subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=repo, text=True).strip(),
                   cacheMode=args.cache_mode, sourceSHA256={p: hashlib.sha256((repo / p).read_bytes()).hexdigest() for p in files},
                   toolVersions=dict(go=subprocess.check_output([go, 'version'], text=True, timeout=10).strip(),
                                     goEnvironment=go_environment, node=node_environment,
                                     python=platform.python_version()),
                   phases=[], helperCommand=['npm', 'run', 'test:helpers'],
                   scope='Diagnostic only. Unchanged full helper suite/default concurrency/30-second Make install limit. '
                         'PATH Go proxy changes tool lookup and adds observation overhead but no Go flags. Fresh cache conditions do not prove '
                         'the historical cache state or timeout cause. No production service or authentication changes.')
    try:
        if args.cache_mode == 'warm':
            phase = observe([str(wrapper), 'build', '-o', str(output / 'warm-client'), './cmd/webmux-client'],
                            repo / 'webmux/server', env, output, 'explicit-cache-preparation', 180)
            summary['phases'].append(phase)
            if phase['status'] != 0 or phase['observerTimedOut']:
                return 1
        phase = observe(['npm', 'run', 'test:helpers'], repo / 'webmux', env, output, 'original-helpers', 900)
        summary['phases'].append(phase)
        return 0 if phase['status'] == 0 and not phase['observerTimedOut'] else 1
    finally:
        (output / 'summary.json').write_text(json.dumps(summary, indent=2) + '\n')


if __name__ == '__main__':
    if len(sys.argv) >= 4 and sys.argv[1] == 'proxy':
        raise SystemExit(proxy(sys.argv[2], sys.argv[3], sys.argv[4:]))
    raise SystemExit(main())
