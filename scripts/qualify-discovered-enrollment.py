#!/usr/bin/env python3
"""Qualify real two-host mDNS/API discovery and background TLS peer migration.

Uses private disposable API fixtures and a fresh temporary grant, never an
installed service, account store or OS trust change. Raw LAN records stay private.
"""
import argparse
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import secrets
import shlex
import subprocess
import sys
import time


REMOTE = r'''
import hashlib,json,os,pathlib,select,subprocess,sys,time
os.umask(0o077)
i=json.loads(sys.stdin.readline());root=pathlib.Path(i['root'])
if i['operation']=='prepare':
 root.mkdir(mode=0o700,parents=False,exist_ok=False)
 print(json.dumps({'kernel':os.uname().sysname,'machine':os.uname().machine}));sys.exit(0)
binary=root/'httpapi-linux-arm64.test'
if hashlib.sha256(binary.read_bytes()).hexdigest()!=i['sha256']:raise RuntimeError('binary hash mismatch')
(root/'credential').write_text(i['credential']+'\n')
env={k:v for k,v in os.environ.items() if k in ('PATH','HOME','TMPDIR','LANG','TZ')}
env.update(WEBMUX_NODE_NETWORK_ROLE='server',WEBMUX_NODE_NETWORK_DIR=str(root),WEBMUX_NODE_NETWORK_BIND=i['bind'],WEBMUX_NODE_NETWORK_DISCOVERY='1')
with (root/'server.log').open('xb') as log:
 p=subprocess.Popen([str(binary),'-test.run=^TestNodeIdentityNativeNetworkServer$','-test.v','-test.timeout=100s'],env=env,stdin=subprocess.DEVNULL,stdout=log,stderr=subprocess.STDOUT)
 try:
  deadline=time.monotonic()+8
  while time.monotonic()<deadline and not (root/'manifest.json').exists() and p.poll() is None:time.sleep(.025)
  if not (root/'manifest.json').exists():raise RuntimeError('server not ready within eight seconds')
  print(json.dumps({'manifest':json.loads((root/'manifest.json').read_text()),'server_pid':p.pid}),flush=True)
  deadline=time.monotonic()+100
  while p.poll() is None and time.monotonic()<deadline:
   if select.select([sys.stdin],[],[],.1)[0]:
    sys.stdin.readline();break
  if p.poll() is None:p.terminate()
  try:p.wait(timeout=5)
  except subprocess.TimeoutExpired:p.kill();p.wait(timeout=5)
 finally:
  if p.poll() is None:
   p.terminate()
   try:p.wait(timeout=5)
   except subprocess.TimeoutExpired:p.kill();p.wait(timeout=5)
print(json.dumps({'server_exit_code':p.returncode,'server_log':(root/'server.log').read_text()}),flush=True)
sys.exit(0 if p.returncode==0 else 1)
'''


def require(value, message):
    if not value:
        raise RuntimeError(message)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--ssh-host', required=True)
    parser.add_argument('--remote-root', type=Path, required=True)
    parser.add_argument('--remote-bind', required=True, help='Verified non-loopback Linux LAN address')
    args = parser.parse_args()
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.@-]*', args.ssh_host):
        parser.error('Use an already authorized plain SSH target')
    if not args.output.is_absolute() or not re.fullmatch(r'/[A-Za-z0-9_./-]+', str(args.remote_root)) or '..' in args.remote_root.parts:
        parser.error('Use fresh absolute private directories without parent traversal')
    address = ipaddress.ip_address(args.remote_bind)
    if address.is_loopback or address.is_unspecified or address.is_multicast:
        parser.error('An explicit non-loopback listener address is required')
    require(sys.platform == 'darwin' and os.uname().machine == 'arm64', 'Primary must be native Darwin ARM64')
    os.umask(0o077)
    args.output.mkdir(mode=0o700, parents=False, exist_ok=False)
    root = Path(__file__).resolve().parent.parent
    server_root = root / 'webmux/server'
    def sources():
        paths = [Path(__file__).resolve(), server_root/'go.mod', server_root/'go.sum']
        paths += sorted((server_root/'internal').rglob('*.go'))
        return {str(p.relative_to(root)): digest(p) for p in paths}
    receipt = {'passed': False, 'sources_before': sources(), 'commands': {}, 'scope': 'Native API fixtures, actual mDNS observations and background reconciliation; not installed application, OS trust or agent-workflow qualification'}
    ssh = ['ssh', '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes', '-o', 'ConnectTimeout=5', args.ssh_host, 'python3 -c '+shlex.quote(REMOTE)]
    environment = {k:v for k,v in os.environ.items() if k in ('PATH','HOME','TMPDIR','LANG','TZ','GOPATH','GOMODCACHE','GOCACHE','GOTOOLCHAIN')}
    remote = None
    def command(name, argv, timeout=120, **kwargs):
        result = subprocess.run(argv, capture_output=True, timeout=timeout, **kwargs)
        (args.output/(name+'.log')).write_bytes(result.stdout+result.stderr)
        receipt['commands'][name] = {'exit_code':result.returncode}
        require(result.returncode == 0, name+' failed; retain private output')
        return result.stdout
    try:
        for target in ('darwin','linux'):
            command('build-'+target, ['go','test','-c','-o',str(args.output/('httpapi-'+target+'-arm64.test')),'./internal/httpapi'], cwd=server_root, env=dict(environment, GOOS=target, GOARCH='arm64', CGO_ENABLED='0'))
        receipt['binary_sha256'] = {target:digest(args.output/('httpapi-'+target+'-arm64.test')) for target in ('darwin','linux')}
        prepared = command('remote-prepare', ssh, timeout=20, input=(json.dumps({'operation':'prepare','root':str(args.remote_root)})+'\n').encode())
        runtime = json.loads(prepared)
        require(runtime == {'kernel':'Linux','machine':'aarch64'}, 'Remote must be native Linux ARM64')
        receipt['remote'] = runtime
        command('transfer', ['scp','-B','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=5',str(args.output/'httpapi-linux-arm64.test'),args.ssh_host+':'+str(args.remote_root)+'/httpapi-linux-arm64.test'])
        credential = secrets.token_hex(32)
        (args.output/'credential').write_text(credential+'\n')
        remote = subprocess.Popen(ssh, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        remote.stdin.write((json.dumps({'operation':'run','root':str(args.remote_root),'bind':str(address),'credential':credential,'sha256':receipt['binary_sha256']['linux']})+'\n').encode())
        remote.stdin.flush()
        # select bounds startup independently of SSH connection and native fixture deadlines.
        import select
        require(bool(select.select([remote.stdout], [], [], 12)[0]), 'Remote readiness response timed out')
        ready = json.loads(remote.stdout.readline())
        manifest = ready['manifest']
        (args.output/'manifest.json').write_text(json.dumps(manifest)+'\n')
        client_env = dict(environment, WEBMUX_NODE_NETWORK_ROLE='client', WEBMUX_NODE_NETWORK_DIR=str(args.output), WEBMUX_NODE_NETWORK_DISCOVERY='1')
        command('client', [str(args.output/'httpapi-darwin-arm64.test'),'-test.run=^TestNodeIdentityNativeNetworkClient$','-test.v','-test.timeout=60s'], timeout=65, env=client_env)
        # Success requires the independent receiver witness, not only client exit.
        out, error = remote.communicate(input=b'stop\n', timeout=15)
        (args.output/'remote-result.log').write_bytes(out+error)
        receipt['server_exit_code'] = remote.returncode
        require(remote.returncode == 0, 'Independent remote witness failed')
        result = json.loads(out)
        require(result['server_exit_code'] == 0, 'Native server did not exit successfully')
        (args.output/'server.log').write_text(result['server_log'])
        receipt['passed'] = True
    except Exception as error:
        receipt['error'] = str(error)
        raise
    finally:
        if remote is not None and remote.poll() is None:
            try:
                out, error = remote.communicate(input=b'stop\n', timeout=15)
                (args.output/'remote-cleanup.log').write_bytes(out+error)
            except subprocess.TimeoutExpired:
                # Do not infer remote child exit from a client-side SSH timeout.
                receipt['remote_cleanup_unverified'] = True
                remote.terminate()
                try: remote.communicate(timeout=5)
                except subprocess.TimeoutExpired:
                    remote.kill()
                    remote.communicate(timeout=5)
        receipt['sources_after'] = sources()
        receipt['sources_unchanged'] = receipt['sources_before'] == receipt['sources_after']
        receipt['passed'] = receipt['passed'] and receipt['sources_unchanged']
        (args.output/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n')
    return 0 if receipt['passed'] else 1


if __name__ == '__main__':
    sys.exit(main())
