#!/usr/bin/env python3
"""Qualify production execution-host Git inspection over authenticated SSH.

Requires transferred source-bound WebMux and recovery test executables. Creates
one new private remote fixture directory; starts no provider or installed service.
Random session authentication travels only through SSH stdin and stays out of
the receipt. This is RPC/component evidence, not remote agent onboarding.
"""

import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import secrets
import shlex
import subprocess
import uuid


SETUP = r'''
import hashlib,json,os,pathlib,subprocess,sys
i=json.load(sys.stdin); root=pathlib.Path(i['root']); root.mkdir(mode=0o700,parents=False)
repo=root/'project'; repo.mkdir(); cache=root/'cache'; cache.mkdir(mode=0o700)
env=dict(os.environ,GIT_CONFIG_GLOBAL='/dev/null',GIT_CONFIG_NOSYSTEM='1',GIT_TERMINAL_PROMPT='0')
def git(*args):
 return subprocess.check_output(['git','-c','core.hooksPath=/dev/null','-C',str(repo),*args],env=env,stderr=subprocess.DEVNULL,text=True).strip()
git('init'); git('-c','user.name=Fixture','-c','user.email=fixture@example.test','commit','--allow-empty','-m','native RPC fixture')
git('remote','add','origin','https://example.test/team/native-ssh-project.git')
git('checkout','-b','rpc-original')
oid=git('rev-parse','HEAD')
print(json.dumps({'repository':'example.test/team/native-ssh-project','root_oid':oid}))
'''

CHANGE = r'''
import json,os,pathlib,subprocess,sys
i=json.load(sys.stdin); repo=pathlib.Path(i['root'])/'project'
env=dict(os.environ,GIT_CONFIG_GLOBAL='/dev/null',GIT_CONFIG_NOSYSTEM='1',GIT_TERMINAL_PROMPT='0')
subprocess.run(['git','-c','core.hooksPath=/dev/null','-C',str(repo),'checkout','-b','rpc-changed'],env=env,stderr=subprocess.DEVNULL,check=True)
print('{}')
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--ssh-host', required=True)
    parser.add_argument('--root', required=True, help='new directory under an existing private remote qualification directory')
    parser.add_argument('--helper', required=True, help='absolute transferred production executable')
    parser.add_argument('--test-helper', required=True, help='absolute transferred recovery test executable')
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    for value in (args.root, args.helper, args.test_helper):
        path = PurePosixPath(value)
        if not path.is_absolute() or '..' in path.parts or any(ord(c) < 32 for c in value):
            parser.error('remote paths must be absolute without parent traversal or control characters')
    if len(PurePosixPath(args.root).parts) < 5:
        parser.error('--root must name a new private qualification subdirectory')
    ssh = ['ssh', '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes', '-o', 'ConnectTimeout=5', '--', args.ssh_host]

    def remote(command, value):
        result = subprocess.run(ssh + [command], input=json.dumps(value).encode(), stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, timeout=15, check=False)
        if result.returncode:
            raise RuntimeError(f'SSH fixture command failed with exit {result.returncode}: {result.stderr.decode(errors="replace")}')
        return json.loads(result.stdout)

    fixture = remote('python3 -c ' + shlex.quote(SETUP), {'root': args.root})
    session = str(uuid.uuid4())
    key = secrets.token_hex(16)
    request = {'protocol': 1, 'session_id': session, 'key': key, 'operation': 'coordination-project',
               'coordination_directory': args.root + '/project'}
    environment = 'env ' + shlex.quote('WEBMUX_RECOVERY_CACHE_HOME=' + args.root + '/cache') + ' '
    initialized = remote(environment + 'WEBMUX_TEST_COORDINATION_PROJECT_INITIALIZE=1 ' + shlex.quote(args.test_helper)
                         + ' -test.run=^TestCoordinationProjectNativeInitialize$', request)
    assert initialized['outcome'] == 'initialized'
    command = environment + shlex.quote(args.helper) + ' --recovery-rpc'
    original = remote(command, request)
    project = original.get('coordination_project')
    assert original['protocol'] == 1 and not original.get('error') and project, original
    expected = hashlib.sha256(('webmux-repository/v1\n' + fixture['repository'] + '\n' + fixture['root_oid']).encode()).hexdigest()
    assert project['id'] == expected and project['repository'] == fixture['repository'], project
    assert project['worktree'] == args.root + '/project' and project['branch'] == 'rpc-original' and project['source'] == 'verified-git', project
    assert not original.get('events') and not original['recording'], original
    controls = []
    for name, changed in [
        ('foreign-key', {'key': secrets.token_hex(16)}),
        ('foreign-session', {'session_id': str(uuid.uuid4())}),
        ('missing-path', {'coordination_directory': ''}),
        ('relative-path', {'coordination_directory': 'project'}),
        ('non-repository', {'coordination_directory': args.root + '/cache'}),
    ]:
        response = remote(command, dict(request, **changed))
        assert response.get('error') and not response.get('coordination_project'), (name, response)
        controls.append({'control': name, 'result': 'refused'})
    remote('python3 -c ' + shlex.quote(CHANGE), {'root': args.root})
    changed = remote(command, request)
    assert not changed.get('error') and changed['coordination_project']['id'] == expected
    assert changed['coordination_project']['branch'] == 'rpc-changed', changed
    receipt = {'version': 1, 'scope': 'production read-only repository RPC over SSH; no agent onboarding',
               'repository_id': expected, 'repository': fixture['repository'], 'recording': False,
               'initial_branch': project['branch'], 'changed_branch': changed['coordination_project']['branch'],
               'same_project_after_branch_change': True, 'controls': controls,
               'qualifier_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(receipt, indent=2) + '\n')
    print(json.dumps(receipt))


if __name__ == '__main__':
    main()
