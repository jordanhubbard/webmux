#!/usr/bin/env python3
"""Verify native SSH adoption using actual Claude events and the source ledger."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path

spec = importlib.util.spec_from_file_location(
    'native_pair', Path(__file__).with_name('audit-coordination-native-pair.py'))
pair = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pair)


def audit(root):
    expected = json.loads((root / 'expected.json').read_text())
    raw = (root / 'native.jsonl').read_text()
    events = [json.loads(line) for line in raw.splitlines() if line.strip()]
    ledger = json.loads((root / 'view.json').read_text())
    calls = pair.calls(root / 'native.jsonl')
    view_name, act_name = expected['tools']
    server = view_name.split('__')[1]
    evidence = {}

    def require(name, condition):
        evidence[name] = bool(condition)

    def subject(member):
        return (member.get('ref') == expected['subject']
                and member.get('project_status') == 'verified-git'
                and member.get('project', {}).get('repository')
                == 'example.test/team/native-ssh-channel'
                and member.get('transport') == 'ssh')

    require('native_connected_dynamic_server', any(
        e.get('type') == 'system' and e.get('subtype') == 'init'
        and any(s.get('name') == server and s.get('status') == 'connected'
                and s.get('source') == 'dynamic' for s in e.get('mcp_servers', []))
        for e in events))
    views = [c for c in calls if c['name'] == view_name and not c.get('is_error')
             and any(v.get('contract') == 'webmux-session-coordination/v1'
                     and v.get('enabled') is True and subject(v.get('self', {}))
                     for v in pair.objects(c['output']))]
    require('native_scoped_verified_view', views)
    actions = [c for c in calls if c['name'] == act_name and not c.get('is_error')
               and c['args'].get('action') == 'work'
               and c['args'].get('text') == expected['task']
               and c['args'].get('request_id')
               and pair.outcomes(c, 'updated')]
    require('native_work_after_view', any(v['index'] < a['index']
                                        for v in views for a in actions))
    require('source_ledger_confirms_task', any(
        subject(m) and m.get('task') == expected['task']
        and m.get('interface_state') == 'interface-confirmed'
        for m in ledger.get('members', [])))
    uses = [b for e in events if e.get('type') == 'assistant'
            for b in e.get('message', {}).get('content', [])
            if isinstance(b, dict) and b.get('type') == 'tool_use']
    require('only_approved_tools_and_discovery', uses and all(
        b.get('name') in (view_name, act_name, 'ToolSearch') for b in uses))
    require('all_tool_calls_completed', len(uses) == len(calls)
            and len({b.get('id') for b in uses}) == len(uses))
    require('native_success', any(e.get('type') == 'result'
                                 and e.get('subtype') == 'success'
                                 and e.get('is_error') is False for e in events))
    require('capability_not_in_output', 'wmc1.' not in raw
            and 'wmc1.' not in (root / 'terminal.log').read_text()
            and 'wmc1.' not in (root / 'native.stderr').read_text())
    return dict(passed=all(evidence.values()), evidence=evidence,
                failures=[k for k, v in evidence.items() if not v],
                native_tool_call_count=len(calls), source_hashes={
                    name: hashlib.sha256((root / name).read_bytes()).hexdigest()
                    for name in ('expected.json', 'native.jsonl', 'view.json',
                                 'terminal.log', 'native.stderr')})


def audit_reconnect(root):
    result = audit(root)
    expected = json.loads((root / 'expected.json').read_text())
    indexed_events = [(i, json.loads(line)) for i, line in enumerate(
        (root / 'native.jsonl').read_text().replace('\r', '').splitlines()) if line.strip()]
    events = [event for _, event in indexed_events]
    calls = pair.calls(root / 'native.jsonl')
    continuity = json.loads((root / 'reconnect.json').read_text())
    ledger = json.loads((root / 'view-reconnected.json').read_text())
    view_name, act_name = expected['tools']

    def require(name, condition):
        result['evidence'][name] = bool(condition)
        if not condition:
            result['failures'].append(name)

    before, after = continuity.get('native_before', {}), continuity.get('native_after', {})
    require('same_live_cli_and_mcp_processes', before == after and all(
        type(before.get(k, {}).get('pid')) is int and before[k]['pid'] > 0
        and str(before[k].get('start', '')).isdigit() for k in ('cli', 'mcp', 'launcher'))
        and before.get('cli') != before.get('mcp'))
    require('same_live_processes_after_second_turn',
            bool(before) and before == continuity.get('native_completed'))
    require('same_provider_new_control', continuity.get('control_replaced') is True
            and continuity.get('old_master_stopped') is True
            and continuity.get('pane_before')
            and continuity.get('pane_before') == continuity.get('pane_after'))
    inits = [e for e in events if e.get('type') == 'system' and e.get('subtype') == 'init']
    results = [e for e in events if e.get('type') == 'result']
    # Claude stream-json emits init for each input turn. Conversation identity
    # and the independently observed live process identities establish continuity.
    require('one_native_conversation_two_successful_turns', len(inits) == 2
            and len(results) == 2 and inits[0].get('session_id')
            and all(e.get('session_id') == inits[0]['session_id'] for e in inits + results)
            and all(e.get('subtype') == 'success' and e.get('is_error') is False
                    for e in results)
            and all(any(s.get('name') == view_name.split('__')[1]
                        and s.get('status') == 'connected' and s.get('source') == 'dynamic'
                        for s in e.get('mcp_servers', [])) for e in inits))
    actions = [c for c in calls if c['name'] == act_name and not c.get('is_error')
               and c['args'].get('action') == 'work' and pair.outcomes(c, 'updated')]
    first = [c for c in actions if c['args'].get('text') == expected['task']]
    second = [c for c in actions if c['args'].get('text') == 'native SSH channel reconnected'
              and c['args'].get('request_id')]
    views = [c for c in calls if c['name'] == view_name and not c.get('is_error')
             and any(v.get('contract') == 'webmux-session-coordination/v1'
                     and v.get('enabled') is True
                     and v.get('self', {}).get('ref') == expected['subject']
                     and v.get('self', {}).get('task') == expected['task']
                     and v.get('self', {}).get('project_status') == 'verified-git'
                     and v.get('self', {}).get('project', {}).get('repository')
                     == 'example.test/team/native-ssh-channel'
                     for v in pair.objects(c['output']))]
    init_indices = [i for i, e in indexed_events
                    if e.get('type') == 'system' and e.get('subtype') == 'init']
    result_indices = [i for i, e in indexed_events if e.get('type') == 'result']
    require('fresh_view_and_action_after_reconnect', len(init_indices) == 2
            and len(result_indices) == 2 and any(
        a['index'] < result_indices[0] < init_indices[1] < v['index']
        < b['index'] < result_indices[1]
        and a['args'].get('request_id') != b['args']['request_id']
        for a in first for v in views for b in second))
    require('source_confirms_reconnected_task', any(
        m.get('ref') == expected['subject']
        and m.get('task') == 'native SSH channel reconnected'
        and m.get('interface_state') == 'interface-confirmed'
        and m.get('project_status') == 'verified-git'
        and m.get('project', {}).get('repository') == 'example.test/team/native-ssh-channel'
        for m in ledger.get('members', [])))
    for name in ('reconnect.json', 'view-reconnected.json'):
        result['source_hashes'][name] = hashlib.sha256((root / name).read_bytes()).hexdigest()
    result['passed'] = not result['failures']
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root', type=Path)
    parser.add_argument('--output', type=Path)
    parser.add_argument('--reconnect', action='store_true',
                        help='Also require one persistent native CLI/MCP pair across SSH reconnect')
    args = parser.parse_args()
    result = audit_reconnect(args.root) if args.reconnect else audit(args.root)
    data = json.dumps(result, indent=2) + '\n'
    if args.output:
        args.output.write_text(data)
    print(data, end='')
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
