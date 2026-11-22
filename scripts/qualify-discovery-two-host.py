#!/usr/bin/env python3
"""Run bounded production DNS-SD endpoints on a Mac and an SSH Linux ARM64 host.

Outputs contain private network addresses. Use a fresh private artifact directory.
This opens no HTTP listeners, changes no policy and starts no installed service.
A successful endpoint advertisement alone never establishes discovery success.
"""
import argparse
import asyncio
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import platform
import re
import secrets
import shlex
import signal
import subprocess
import sys


REMOTE = r'''
import hashlib,json,os,pathlib,platform,subprocess,sys,time
os.umask(0o077)
i=json.load(sys.stdin);root=pathlib.Path(i['root'])
if i['operation']=='prepare':
 root.mkdir(mode=0o700,parents=False,exist_ok=False)
 print(json.dumps({'kernel':platform.system(),'machine':platform.machine()}));sys.exit(0)
binary=root/'discovery-linux-arm64.test'
actual=hashlib.sha256(binary.read_bytes()).hexdigest()
if actual!=i['sha256']:raise RuntimeError('transferred binary hash mismatch')
log=root/(i['phase']+'.log');output=root/i['phase']
if output.exists() or log.exists():raise RuntimeError('fixture output already exists')
env={k:v for k,v in os.environ.items() if k in ('PATH','TMPDIR','LANG','TZ')}
env.update(WEBMUX_TEST_NODE_ROLE=i['role'],WEBMUX_TEST_NODE_ID=i['id'],WEBMUX_TEST_NODE_EXPECTED=i['expected'],WEBMUX_TEST_NODE_OUTPUT=str(output))
with log.open('xb') as f:
 p=subprocess.Popen([str(binary),'-test.run=^TestNodeNativeEndpoint$','-test.v','-test.timeout=20s'],env=env,stdout=f,stderr=subprocess.STDOUT,stdin=subprocess.DEVNULL)
 try:
  deadline=time.monotonic()+5
  while time.monotonic()<deadline and not (output/'ready.json').exists() and p.poll() is None:time.sleep(.025)
  ready=json.loads((output/'ready.json').read_text()) if (output/'ready.json').exists() else None
  print(json.dumps({'ready':ready,'kernel':platform.system(),'machine':platform.machine(),'binary_sha256':actual}),flush=True)
  result=p.wait(timeout=23)
 finally:
  if p.poll() is None:
   p.terminate()
   try:p.wait(timeout=3)
   except subprocess.TimeoutExpired:p.kill();p.wait()
print(log.read_text(),end='',flush=True)
records={name:json.loads((output/name).read_text()) for name in ('observed.json','observed-all.json','failed-snapshot.json') if (output/name).exists()}
print('NATIVE_DISCOVERY_END='+json.dumps({'exit_code':result,'records':records}),flush=True)
sys.exit(result)
'''


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def private_environment():
    return {k: v for k, v in os.environ.items() if k in ('PATH', 'TMPDIR', 'LANG', 'TZ')}


class Qualification:
    def __init__(self, args):
        self.args = args
        self.root = Path(__file__).resolve().parent.parent
        self.output = args.output
        self.output.mkdir(mode=0o700, parents=False, exist_ok=False)
        self.receipt = {'passed': False, 'phases': []}
        self.sources = [Path(__file__).resolve()]
        self.sources += sorted((self.root / 'webmux/server/internal/discovery').glob('*.go'))
        self.sources += [self.root / 'webmux/server' / name for name in ('go.mod', 'go.sum')]
        self.receipt['sources_before'] = self.source_hashes()
        self.linux_hash = None

    def source_hashes(self):
        return {str(p.relative_to(self.root)): digest(p) for p in self.sources}

    def command(self, name, argv, **kwargs):
        result = subprocess.run(argv, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                stderr=subprocess.STDOUT, timeout=120, **kwargs)
        (self.output / (name + '.log')).write_bytes(result.stdout)
        self.receipt.setdefault('commands', {})[name] = {'exit_code': result.returncode}
        require(result.returncode == 0, name + ' failed; inspect retained log')
        return result

    def ssh(self):
        return ['ssh', '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes',
                '-o', 'ConnectTimeout=5', self.args.ssh_host, 'python3 -c ' + shlex.quote(REMOTE)]

    async def remote(self, request):
        process = await asyncio.create_subprocess_exec(
            *self.ssh(), stdin=asyncio.subprocess.PIPE, stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.STDOUT)
        request.update(root=str(self.args.remote_root))
        process.stdin.write(json.dumps(request).encode())
        await process.stdin.drain()
        process.stdin.close()
        return process

    async def prepare(self):
        require(platform.system() == 'Darwin' and platform.machine() == 'arm64',
                'primary target must be native Darwin ARM64')
        self.receipt['primary'] = {'kernel': platform.system(), 'machine': platform.machine()}
        self.command('go-version', ['go', 'version'])
        for target in ('darwin', 'linux'):
            env = private_environment()
            env.update({key: os.environ[key] for key in
                        ('HOME', 'GOPATH', 'GOCACHE', 'GOMODCACHE', 'GOTOOLCHAIN')
                        if key in os.environ})
            env.update(GOOS=target, GOARCH='arm64', CGO_ENABLED='0')
            self.command('build-' + target, ['go', 'test', '-c', '-o',
                         str(self.output / ('discovery-' + target + '-arm64.test')),
                         './internal/discovery'], cwd=self.root / 'webmux/server', env=env)
        self.receipt['binary_sha256'] = {
            target: digest(self.output / ('discovery-' + target + '-arm64.test'))
            for target in ('darwin', 'linux')}
        self.linux_hash = self.receipt['binary_sha256']['linux']
        process = await self.remote({'operation': 'prepare'})
        try:
            out, _ = await asyncio.wait_for(process.communicate(), 20)
        finally:
            if process.returncode is None:
                process.terminate()
                await process.wait()
        (self.output / 'remote-prepare.log').write_bytes(out)
        require(process.returncode == 0, 'remote private directory preparation failed')
        runtime = json.loads(out)
        require(runtime['kernel'] == 'Linux' and runtime['machine'] == 'aarch64',
                'SSH target must be native Linux ARM64')
        self.receipt['remote'] = runtime
        self.command('transfer', ['scp', '-B', '-o', 'StrictHostKeyChecking=yes',
                     '-o', 'ConnectTimeout=5', str(self.output / 'discovery-linux-arm64.test'),
                     self.args.ssh_host + ':' + str(self.args.remote_root) + '/discovery-linux-arm64.test'])

    async def phase(self, name, linux_role, mac_role):
        own, peer = secrets.token_hex(16), secrets.token_hex(16)
        process = await self.remote({'operation': 'run', 'phase': name, 'role': linux_role,
                                    'id': peer, 'expected': own, 'sha256': self.linux_hash})
        first, rest = b'', b''
        mac = None
        result = {'name': name, 'passed': False, 'mac_role': mac_role, 'linux_role': linux_role,
                  'mac_id': own, 'linux_id': peer}
        self.receipt['phases'].append(result)
        try:
            first = await asyncio.wait_for(process.stdout.readline(), 12)
            ready = json.loads(first)
            require(ready['ready'] and ready['ready']['id'] == peer and
                    ready['ready']['role'] == linux_role and ready['kernel'] == 'Linux' and
                    ready['machine'] == 'aarch64' and ready['binary_sha256'] == self.linux_hash,
                    'remote endpoint not ready with the expected native binary and identity')
            result['remote_ready'] = ready
            with (self.output / (name + '-mac.log')).open('xb') as log:
                if mac_role == 'bonjour-advertise':
                    argv = ['/usr/bin/dns-sd', '-R', 'WebMux-' + own, '_webmux-node._tcp',
                            'local', '19311', 'v=1', 'id=' + own, 'tls=1']
                    env = private_environment()
                else:
                    env = private_environment()
                    env.update(WEBMUX_TEST_NODE_ROLE=mac_role, WEBMUX_TEST_NODE_ID=own,
                               WEBMUX_TEST_NODE_EXPECTED=peer,
                               WEBMUX_TEST_NODE_OUTPUT=str(self.output / (name + '-mac')))
                    argv = [str(self.output / 'discovery-darwin-arm64.test'),
                            '-test.run=^TestNodeNativeEndpoint$', '-test.v', '-test.timeout=20s']
                mac = await asyncio.create_subprocess_exec(
                    *argv, env=env, stdin=asyncio.subprocess.DEVNULL, stdout=log,
                    stderr=asyncio.subprocess.STDOUT)
                if mac_role == 'bonjour-advertise':
                    rest, _ = await asyncio.wait_for(process.communicate(), 28)
                    result['bonjour_running_until_cleanup'] = mac.returncode is None
                    if mac.returncode is None:
                        mac.send_signal(signal.SIGINT)
                    await asyncio.wait_for(mac.wait(), 5)
                else:
                    await asyncio.wait_for(mac.wait(), 25)
                    rest, _ = await asyncio.wait_for(process.communicate(), 28)
            end = json.loads(next(line.split('=', 1)[1] for line in rest.decode().splitlines()
                                  if line.startswith('NATIVE_DISCOVERY_END=')))
            local = {p.name: json.loads(p.read_text())
                     for p in (self.output / (name + '-mac')).glob('*.json')}
            result.update(mac_exit_code=mac.returncode, linux_exit_code=process.returncode,
                          linux_result=end, mac_records=local)
            mac_observed = mac_role in ('advertise', 'bonjour-advertise') or local.get(
                'observed.json', {}).get('claimed_id') == peer
            linux_observed = linux_role == 'advertise' or end['records'].get(
                'observed.json', {}).get('claimed_id') == own
            mac_ok = mac.returncode == 0 if mac_role != 'bonjour-advertise' else (
                result['bonjour_running_until_cleanup'] and mac.returncode == -signal.SIGINT)
            result['passed'] = mac_ok and process.returncode == end['exit_code'] == 0 and mac_observed and linux_observed
            result['candidate_address_families'] = {}
            for label, records in [('mac', local), ('linux', end['records'])]:
                addresses = [ipaddress.ip_address(c['address'].split('%')[0])
                             for c in records.get('observed-all.json', [])]
                result['candidate_address_families'][label] = sorted({
                    4 if getattr(a, 'ipv4_mapped', None) else a.version for a in addresses})
            print(json.dumps({k: result[k] for k in ('name', 'passed', 'mac_exit_code',
                             'linux_exit_code', 'candidate_address_families')}), flush=True)
        finally:
            if mac is not None and mac.returncode is None:
                mac.terminate()
                try:
                    await asyncio.wait_for(mac.wait(), 5)
                except asyncio.TimeoutError:
                    mac.kill()
                    await mac.wait()
            if process.returncode is None:
                try:
                    rest, _ = await asyncio.wait_for(process.communicate(), 30)
                except asyncio.TimeoutError:
                    process.terminate()
                    await process.wait()
            (self.output / (name + '-linux.log')).write_bytes(first + rest)
            self.save()

    def save(self):
        self.receipt['sources_after'] = self.source_hashes()
        self.receipt['sources_unchanged'] = self.receipt['sources_before'] == self.receipt['sources_after']
        self.receipt['passed'] = self.receipt['passed'] and self.receipt['sources_unchanged']
        (self.output / 'receipt.json').write_text(json.dumps(self.receipt, indent=2) + '\n')

    async def run(self):
        try:
            await self.prepare()
            for args in [('forward', 'advertise', 'browse'), ('reverse', 'browse', 'advertise'),
                         ('duplex', 'both', 'both')]:
                await self.phase(*args)
            if self.args.bonjour_control:
                await self.phase('bonjour-control', 'browse', 'bonjour-advertise')
            self.save()
            self.receipt['passed'] = self.receipt['sources_unchanged'] and all(
                phase['passed'] for phase in self.receipt['phases'])
        except Exception as error:
            self.receipt['error'] = str(error)
            raise
        finally:
            self.save()
        return 0 if self.receipt['passed'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True, help='New absolute private output directory')
    parser.add_argument('--ssh-host', required=True, help='Already authorized SSH host alias')
    parser.add_argument('--remote-root', type=Path, required=True,
                        help='New absolute private directory; its parent must already exist')
    parser.add_argument('--bonjour-control', action='store_true', help='Also test native macOS advertisement')
    args = parser.parse_args()
    if not args.output.is_absolute() or not args.remote_root.is_absolute():
        parser.error('Output and remote directory must be absolute')
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.@-]*', args.ssh_host):
        parser.error('Use a plain, trusted SSH alias')
    if not re.fullmatch(r'/[A-Za-z0-9_./-]+', str(args.remote_root)) or '..' in args.remote_root.parts:
        parser.error('Use a portable remote path without parent traversal')
    os.umask(0o077)
    return asyncio.run(Qualification(args).run())


if __name__ == '__main__':
    sys.exit(main())
