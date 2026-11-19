#!/usr/bin/env python3
"""Check a scoped Codex 0.169 native launch witness, without executing its text.

This verifies native policy/tool records, not WebMux integration, authentication
provenance, OS process identity or the occurrence of a manager/host restart.
"""
import argparse
import hashlib
import json
from pathlib import Path


def verify(session, cwd, challenge, rig_id, node_id, network):
    expected_policy = {"type": "read-only"}
    if network:
        expected_policy["network_access"] = True
    contexts = []
    calls = {}
    completed_calls = set()
    results = []
    replies = []
    metadata = []
    for event in session:
        payload = event.get("payload", {})
        if not isinstance(payload, dict):
            raise ValueError("invalid_payload")
        kind = event.get("type")
        if kind == "session_meta":
            metadata.append(payload)
        if kind == "turn_context":
            if (payload.get("cwd") != cwd or payload.get("approval_policy") != "never"
                    or payload.get("sandbox_policy") != expected_policy):
                raise ValueError("native_policy_or_project_changed")
            # The granular profile must corroborate read-only network-enabled
            # policy; never accept a legacy summary hiding writable paths.
            filesystem = payload.get("file_system_sandbox_policy")
            if filesystem is not None and filesystem != {
                "kind": "restricted", "entries": [
                    {"path": {"type": "path", "path": "/"}, "access": "read"}
                ]
            }:
                raise ValueError("native_filesystem_changed")
            if network and filesystem is None:
                raise ValueError("granular_filesystem_missing")
            contexts.append(payload)
        if kind != "response_item":
            continue
        if payload.get("type") == "custom_tool_call":
            call_id = payload.get("call_id")
            if not isinstance(call_id, str) or call_id in calls:
                raise ValueError("invalid_tool_correlation")
            calls[call_id] = payload.get("input", "")
        if payload.get("type") == "custom_tool_call_output":
            call_id = payload.get("call_id")
            if call_id not in calls or call_id in completed_calls:
                raise ValueError("uncorrelated_tool_output")
            completed_calls.add(call_id)
            for block in payload.get("output", []):
                text = block.get("text", "")
                if not text.startswith("{"):
                    continue
                try:
                    value = json.loads(text)
                except ValueError:
                    continue
                if isinstance(value, dict) and isinstance(value.get("output"), str):
                    results.append((calls[call_id], value))
        if payload.get("type") == "message" and payload.get("role") == "assistant":
            replies.append("".join(block.get("text", "") for block in payload.get("content", [])
                                   if block.get("type") == "output_text"))
    if len(metadata) != 1 or metadata[0].get("cwd") != cwd or not contexts:
        raise ValueError("native_session_or_context_missing")
    read = any("challenge.txt" in call and result.get("exit_code") == 0
               and result["output"].strip() == challenge for call, result in results)
    denied = any("readonly-denied.txt" in call and result.get("exit_code") == 1
                 and "Operation not permitted" in result["output"] for call, result in results)
    identities = 0
    live_rig_names = set()
    for call, result in results:
        if "rig status --json" not in call or result.get("exit_code") != 0:
            continue
        try:
            status = json.loads(result["output"])
        except ValueError:
            continue
        if status.get("daemon", {}).get("reachable") is True:
            live_rig_names.update(row.get("name") for row in status.get("rigs", [])
                                  if row.get("id") == rig_id and isinstance(row.get("name"), str))
    for call, result in results:
        if "rig whoami --json" not in call or result.get("exit_code") != 0:
            continue
        try:
            identity = json.loads(result["output"])
        except ValueError:
            continue
        if (identity.get("partial") is not True and identity.get("daemonReachable") is not False
                and identity.get("identity", {}).get("rigName") in live_rig_names
                and identity.get("identity", {}).get("nodeId") == node_id):
            identities += 1
    if not read or not denied or not identities:
        raise ValueError("native_tool_evidence_missing")
    return {"passed": True, "nativeContexts": len(contexts),
            "actualChallengeRead": read, "actualWriteRefusal": denied,
            "verifiedCoordinatorReplies": identities,
            "rememberedMarkerReply": any(challenge in reply and
                "NATIVE_COORDINATOR_RESTART_COMPLETE" in reply for reply in replies),
            "scope": "Native policy/tool witness only; restart and process provenance require separate evidence."}


def read_events(path):
    if path.stat().st_size > 32 * 1024 * 1024:
        raise ValueError("native_session_exceeds_limit")
    events = []
    with path.open(encoding="utf-8") as stream:
        for line in stream:
            if len(line.encode("utf-8")) > 8 * 1024 * 1024:
                raise ValueError("native_event_exceeds_limit")
            value = json.loads(line)
            if not isinstance(value, dict):
                raise ValueError("invalid_native_event")
            events.append(value)
    return events


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--session", required=True, type=Path)
    parser.add_argument("--cwd", required=True)
    parser.add_argument("--challenge-file", required=True, type=Path)
    parser.add_argument("--rig-id", required=True)
    parser.add_argument("--node-id", required=True)
    parser.add_argument("--network", choices=["enabled", "disabled"], required=True)
    args = parser.parse_args()
    try:
        challenge = args.challenge_file.read_text().strip()
        if len(challenge) != 32 or any(c not in "0123456789abcdef" for c in challenge):
            raise ValueError("invalid_fixture_challenge")
        result = verify(read_events(args.session), args.cwd, challenge,
                        args.rig_id, args.node_id, args.network == "enabled")
        if (Path(args.cwd) / "readonly-denied.txt").exists():
            raise ValueError("forbidden_output_exists")
        result["nativeSessionSHA256"] = hashlib.sha256(args.session.read_bytes()).hexdigest()
        result["challengeSHA256"] = hashlib.sha256(challenge.encode()).hexdigest()
        print(json.dumps(result, indent=2))
        return 0
    except (ValueError, OSError, TypeError, KeyError, AttributeError):
        # Do not echo arbitrary native text, local paths or tool arguments.
        print(json.dumps({"passed": False, "code": "native_witness_refused"}))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
