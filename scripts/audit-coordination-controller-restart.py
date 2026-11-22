#!/usr/bin/env python3
"""Audit native calls and retained state across two production controllers."""
import argparse
import importlib.util
import json
import hashlib
from pathlib import Path

spec = importlib.util.spec_from_file_location('ssh_audit', Path(__file__).with_name('audit-coordination-ssh-agent.py'))
ssh = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ssh)


def audit(root):
    result = ssh.audit(root)
    load = lambda name: json.loads((root / name).read_text())
    expected, continuity = load('expected.json'), load('restart.json')
    before, restored, after = [load(n) for n in ('view.json', 'view-restored.json', 'view-after.json')]
    calls = ssh.pair.calls(root / 'native.jsonl')
    indexed = [(i, json.loads(line)) for i, line in enumerate((root / 'native.jsonl').read_text().splitlines()) if line.strip()]
    inits = [(i, e) for i, e in indexed if e.get('subtype') == 'init']
    results = [(i, e) for i, e in indexed if e.get('type') == 'result']
    view_name, act_name = expected['tools']

    def require(name, condition):
        result['evidence'][name] = bool(condition)
        if not condition:
            result['failures'].append(name)

    native = continuity.get('native_before', {})
    require('same_native_processes', native == continuity.get('native_after') and all(
        isinstance(native.get(k, {}).get('pid'), int) and native[k]['pid'] > 0
        and str(native[k].get('start', '')).isdigit() for k in ('cli', 'mcp', 'launcher')))
    require('different_production_controllers_same_pane', all(
        isinstance(continuity.get(k), int) and continuity[k] > 0
        for k in ('controller_before', 'controller_after'))
        and continuity['controller_before'] != continuity['controller_after']
        and continuity.get('pane_before') and continuity.get('pane_before') == continuity.get('pane_after'))
    require('same_history_epoch', before.get('history_epoch')
            and before.get('history_epoch') == restored.get('history_epoch') == after.get('history_epoch'))
    claims = before.get('claims', [])
    retained = restored.get('claims', [])
    require('original_claim_retained_exactly', len(claims) == 1 and claims == retained
            and claims[0].get('actor') == expected['subject']
            and claims[0].get('resource') == 'restart-owned.txt')
    require('task_retained_before_new_native_turn', any(
        m.get('ref') == expected['subject'] and m.get('task') == expected['task']
        and m.get('project_status') == 'verified-git' for m in restored.get('members', [])))
    require('one_conversation_two_successful_turns', len(inits) == len(results) == 2
            and inits[0][1].get('session_id')
            and all(e.get('session_id') == inits[0][1]['session_id'] for _, e in inits + results)
            and all(e.get('subtype') == 'success' and e.get('is_error') is False for _, e in results)
            and all(any(s.get('name') == view_name.split('__')[1] and s.get('status') == 'connected'
                        for s in e.get('mcp_servers', [])) for _, e in inits))
    actions = [c for c in calls if c['name'] == act_name and not c.get('is_error')]
    admitted = [c for c in actions if c['args'].get('action') == 'claim'
                and c['args'].get('request_id') == 'restart-claim-1'
                and any(v.get('record_id') == claims[0].get('id') for v in ssh.pair.outcomes(c, 'claimed'))] if claims else []
    views = [c for c in calls if c['name'] == view_name and not c.get('is_error')
             and any(v.get('contract') == 'webmux-session-coordination/v1' and v.get('enabled') is True
                     and v.get('self', {}).get('ref') == expected['subject']
                     and v.get('self', {}).get('task') == expected['task'] and v.get('claims') == claims
                     for v in ssh.pair.objects(c['output']))]
    released = [c for c in actions if c['args'].get('action') == 'release-claim'
                and c['args'].get('record_id') == claims[0].get('id')
                and c['args'].get('request_id') == 'restart-release-1'
                and ssh.pair.outcomes(c, 'released')] if claims else []
    require('native_retained_claim_view_and_release', len(inits) == len(results) == 2
            and any(c['index'] < results[0][0] < inits[1][0] < v['index'] < r['index'] < results[1][0]
                    for c in admitted for v in views for r in released))
    first_work = [c for c in actions if c['args'].get('action') == 'work'
                  and c['args'].get('request_id') == 'restart-task-1'
                  and c['args'].get('text') == expected['task'] and ssh.pair.outcomes(c, 'updated')]
    second_work = [c for c in actions if c['args'].get('action') == 'work'
                   and c['args'].get('request_id') == 'restart-task-2'
                   and c['args'].get('text') == 'native SSH restart resumed' and ssh.pair.outcomes(c, 'updated')]
    require('native_work_on_both_controllers', len(inits) == len(results) == 2 and any(
        a['index'] < results[0][0] < inits[1][0] < v['index'] < b['index'] < results[1][0]
        for a in first_work for v in views for b in second_work))
    require('no_recreated_claim', len([c for c in actions if c['args'].get('action') == 'claim']) == 1)
    require('source_confirms_resumed_task_and_release', after.get('claims') == [] and any(
        m.get('ref') == expected['subject'] and m.get('task') == 'native SSH restart resumed'
        and m.get('project_status') == 'verified-git' for m in after.get('members', [])))
    for name in ('restart.json', 'view-restored.json', 'view-after.json'):
        result['source_hashes'][name] = hashlib.sha256((root / name).read_bytes()).hexdigest()
    result['passed'] = not result['failures']
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root', type=Path)
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    result = audit(args.root)
    data = json.dumps(result, indent=2) + '\n'
    if args.output:
        args.output.write_text(data)
    print(data, end='')
    raise SystemExit(0 if result['passed'] else 1)
