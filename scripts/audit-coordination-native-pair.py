#!/usr/bin/env python3
"""Audit actual private native CLI events, never model claims of success."""
import argparse
import hashlib
import json
from pathlib import Path


def objects(value):
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from objects(child)
    elif isinstance(value, list):
        for child in value:
            yield from objects(child)
    elif isinstance(value, str):
        # Native Claude flattens MCP error content into an explanation followed
        # by the server's structured JSON on its own line. Inspect that formal
        # payload only inside an actual tool_result, never assistant prose.
        for line in value.splitlines():
            if line[:1] not in ('{', '['):
                continue
            try:
                yield from objects(json.loads(line))
            except ValueError:
                pass


def calls(path):
    result, pending = [], {}
    for number, line in enumerate(path.read_text().replace('\r', '').splitlines()):
        try:
            event = json.loads(line)
        except ValueError:
            continue
        item = event.get('item')
        if isinstance(item, dict) and item.get('type') == 'mcp_tool_call' and item.get('status') == 'completed':
            result.append(dict(index=number, name=item.get('tool', ''), args=item.get('arguments') or {}, output=item.get('result')))
        message = event.get('message')
        if not isinstance(message, dict) or not isinstance(message.get('content'), list):
            continue
        for block in message['content']:
            if not isinstance(block, dict):
                continue
            if event.get('type') == 'assistant' and block.get('type') == 'tool_use':
                pending[block.get('id')] = dict(name=block.get('name', ''), args=block.get('input') or {})
            if event.get('type') == 'user' and block.get('type') == 'tool_result' and block.get('tool_use_id') in pending:
                call = pending.pop(block['tool_use_id'])
                call.update(index=number, output=block.get('content'), is_error=bool(block.get('is_error')))
                result.append(call)
    for call in result:
        if isinstance(call['args'], str):
            try:
                call['args'] = json.loads(call['args'])
            except ValueError:
                call['args'] = {}
    return result


def tool(call, kind):
    return call['name'] == 'webmux_coordination_' + kind or call['name'].endswith('__webmux_coordination_' + kind)


def outcomes(call, status=None):
    return [value for value in objects(call['output']) if 'status' in value and (status is None or value['status'] == status)]


def audit(reviewer_dir, verifier_dir, transcripts=False):
    failures, evidence = [], {}
    def require(name, condition, detail=None):
        evidence[name] = detail if condition and detail is not None else bool(condition)
        if not condition:
            failures.append(name)
    roots = [reviewer_dir, verifier_dir]
    starts = [json.loads((root / 'started.json').read_text()) for root in roots]
    refs = [start['session'] for start in starts]
    event_name = 'native-transcript.jsonl' if transcripts else 'native-output.log'
    events = [calls(root / event_name) for root in roots]
    require('distinct_real_nodes', refs[0]['node_id'] != refs[1]['node_id'])
    resource = 'node:' + refs[1]['node_id'] + ':heavy'
    for index, (role, root) in enumerate(zip(('reviewer', 'verifier'), roots)):
        views = [value for call in events[index] if tool(call, 'view') for value in objects(call['output']) if value.get('contract') == 'webmux-session-coordination/v1' and (value.get('self') or {}).get('ref') == refs[index]]
        require(role + '_native_scoped_view', bool(views))
        require(role + '_verified_project', any((v.get('self', {}).get('project_status') == 'verified-git' and v.get('self', {}).get('project', {}).get('id')) for v in views))
        require(role + '_discovered_real_counterpart', any(member.get('ref') == refs[1-index] for v in views for peer in v.get('peers', []) if peer.get('state') == 'current' for member in (peer.get('snapshot') or {}).get('members', [])))
        finished = json.loads((root / 'finished.json').read_text()) if (root / 'finished.json').exists() else {}
        require(role + '_native_process_exited', finished.get('native_process_exited') is True)
        require(role + '_driver_unchanged', finished.get('job_driver_unchanged') is True)
    actions = [[call for call in group if tool(call, 'act')] for group in events]
    claims = [[(call, value) for call in group if call['args'].get('action') == 'claim' and call['args'].get('kind') == 'file' and call['args'].get('resource') == 'task.py' for value in outcomes(call, 'claimed')] for group in actions]
    for index, role in enumerate(('reviewer', 'verifier')):
        require(role + '_file_claim', bool(claims[index]))
    require('real_counterpart_overlap', any(c.get('actor') == refs[0] for _, value in claims[1] for c in value.get('conflicts', [])) or any(c.get('actor') == refs[1] for _, value in claims[0] for c in value.get('conflicts', [])))
    grants = [[(call, value) for call in group if call['args'].get('action') == 'reserve' and call['args'].get('resource') == resource for value in outcomes(call, 'reserved') if (value.get('lease') or {}).get('current') is True and value.get('expires')] for group in actions]
    for index, role in enumerate(('reviewer', 'verifier')):
        require(role + '_current_shared_grant', bool(grants[index]))
    busy_calls = [call for call in actions[1] if call['args'].get('action') == 'reserve' and call['args'].get('resource') == resource and outcomes(call, 'busy')]
    require('verifier_busy_before_handoff', bool(busy_calls) and bool(grants[1]) and min(c['index'] for c in busy_calls) < grants[1][-1][0]['index'])
    if all(grants):
        grant_a, grant_b = grants[0][0][1], grants[1][-1][1]
        require('authority_advanced_fence', grant_b['fence'] > grant_a['fence'], dict(reviewer=grant_a['fence'], verifier=grant_b['fence']))
        for index, (role, grant) in enumerate(zip(('reviewer', 'verifier'), (grant_a, grant_b))):
            require(role + '_released_original', any(call['args'].get('action') == 'release-reservation' and call['args'].get('record_id') == grant['record_id'] and call['args'].get('fence') == grant['fence'] and outcomes(call, 'released') for call in actions[index]))
        job = json.loads((verifier_dir / 'job.json').read_text()) if (verifier_dir / 'job.json').exists() else {}
        require('guarded_actual_job', job.get('cases') == 201 and all(job.get(stage, {}).get('ref') == refs[1] and job[stage].get('resource') == resource and job[stage].get('record') == grant_b['record_id'] and job[stage].get('fence') == grant_b['fence'] and (job[stage].get('lease') or {}).get('lease', {}).get('current') is True for stage in ('before', 'after')))
        require('native_job_command_and_result', any(call['name'] == 'Bash' and str(call['args'].get('command', '')).startswith('python3 verify.py ') and 'GUARDED_NATIVE_JOB_PASSED' in str(call['output']) and not call.get('is_error') for call in events[1]))
    else:
        require('guarded_actual_job', False)
    handoffs = [(call, value) for call in actions[0] if call['args'].get('action') == 'message' and call['args'].get('kind') == 'handoff' and call['args'].get('target') == refs[1] for value in outcomes(call, 'admitted')]
    require('native_cross_host_handoff', bool(handoffs))
    if handoffs and grants[0]:
        a_grant = grants[0][0][1]
        require('reviewer_released_before_handoff', any(c['index'] < handoffs[0][0]['index'] and c['args'].get('action') == 'release-reservation' and c['args'].get('record_id') == a_grant['record_id'] and outcomes(c, 'released') for c in actions[0]))
    handoff_ids = {value['record_id'] for _, value in handoffs}
    for action, state in (('delivered', 'delivered'), ('acknowledge', 'acknowledged'), ('complete', 'completed')):
        require('handoff_' + state, any(call['args'].get('action') == action and call['args'].get('record_id') in handoff_ids and any(v.get('state') == state for v in outcomes(call, 'recorded')) for call in actions[1]))
    return dict(passed=not failures, failures=failures, evidence=evidence, native_tool_call_counts=[len(group) for group in events], source_hashes={str(root / name):hashlib.sha256((root / name).read_bytes()).hexdigest() for root in roots for name in ('started.json', event_name)})


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('reviewer', type=Path)
    parser.add_argument('verifier', type=Path)
    parser.add_argument('--output', type=Path)
    parser.add_argument('--native-transcripts', action='store_true', help='Use the exact selected native conversation JSONL, retaining bounded stdout separately')
    args = parser.parse_args()
    result = audit(args.reviewer, args.verifier, args.native_transcripts)
    data = json.dumps(result, indent=2) + '\n'
    if args.output:
        args.output.write_text(data)
    print(data, end='')
    return 0 if result['passed'] else 1

if __name__ == '__main__':
    raise SystemExit(main())
