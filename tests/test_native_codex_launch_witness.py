import copy
import importlib.util
import json
from pathlib import Path
import unittest


SPEC = importlib.util.spec_from_file_location("native_witness",
    Path(__file__).resolve().parents[1] / "scripts/verify-native-codex-launch.py")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def event(kind, payload):
    return {"type": kind, "payload": payload}


def tool(call_id, command, output, exit_code):
    return [event("response_item", {"type": "custom_tool_call", "call_id": call_id,
                                   "input": command}),
            event("response_item", {"type": "custom_tool_call_output", "call_id": call_id,
                "output": [{"type": "input_text", "text": json.dumps({
                    "output": output, "exit_code": exit_code})}]})]


class NativeWitnessTest(unittest.TestCase):
    def setUp(self):
        self.events = [event("session_meta", {"cwd": "/fixture"}),
            event("turn_context", {"cwd": "/fixture", "approval_policy": "never",
                "sandbox_policy": {"type": "read-only", "network_access": True},
                "file_system_sandbox_policy": {"kind": "restricted", "entries": [
                    {"path": {"type": "path", "path": "/"}, "access": "read"}]}})]
        self.events += tool("read", "cat challenge.txt", "challenge\n", 0)
        self.events += tool("write", "printf DENIED > readonly-denied.txt",
                            "readonly-denied.txt: Operation not permitted\n", 1)
        self.events += tool("identity", "rig whoami --json", json.dumps({
            "identity": {"rigName": "fixture", "nodeId": "node"}}), 0)
        self.events += tool("status", "rig status --json", json.dumps({
            "daemon": {"reachable": True}, "rigs": [{"id": "rig", "name": "fixture"}]}), 0)

    def verify(self):
        return MODULE.verify(self.events, "/fixture", "challenge", "rig", "node", True)

    def test_native_tool_witness_does_not_require_visible_terminal_text(self):
        result = self.verify()
        self.assertTrue(result["passed"])
        self.assertEqual(result["verifiedCoordinatorReplies"], 1)
        self.assertFalse(result["rememberedMarkerReply"])

    def test_model_claims_cannot_replace_native_commands(self):
        self.events = self.events[:2] + [event("response_item", {"type": "message",
            "role": "assistant", "content": [{"type": "output_text", "text":
                "Read challenge. Write refused: Operation not permitted. Daemon reachable."}]})]
        with self.assertRaisesRegex(ValueError, "native_tool_evidence_missing"):
            self.verify()

    def test_later_policy_or_project_change_refuses(self):
        for field, value in [("cwd", "/other"), ("approval_policy", "on-request"),
                             ("sandbox_policy", {"type": "danger-full-access"})]:
            with self.subTest(field=field):
                changed = copy.deepcopy(self.events[1])
                changed["payload"][field] = value
                self.events.append(changed)
                with self.assertRaisesRegex(ValueError, "native_policy_or_project_changed"):
                    self.verify()
                self.events.pop()

    def test_granular_write_cannot_hide_behind_readonly_summary(self):
        self.events[1]["payload"]["file_system_sandbox_policy"]["entries"][0]["access"] = "write"
        with self.assertRaisesRegex(ValueError, "native_filesystem_changed"):
            self.verify()

    def test_partial_unreachable_or_other_identity_refuses(self):
        for reply in [{"partial": True, "identity": {"rigName": "fixture", "nodeId": "node"}},
                      {"daemonReachable": False, "identity": {"rigName": "fixture", "nodeId": "node"}},
                      {"identity": {"rigName": "other", "nodeId": "node"}}]:
            self.events[7]["payload"]["output"][0]["text"] = json.dumps({
                "exit_code": 0, "output": json.dumps(reply)})
            with self.assertRaisesRegex(ValueError, "native_tool_evidence_missing"):
                self.verify()

    def test_failed_read_or_successful_write_cannot_qualify(self):
        for index, code in [(3, 1), (5, 0)]:
            block = self.events[index]["payload"]["output"][0]
            original = block["text"]
            value = json.loads(original)
            value["exit_code"] = code
            block["text"] = json.dumps(value)
            with self.assertRaisesRegex(ValueError, "native_tool_evidence_missing"):
                self.verify()
            block["text"] = original

    def test_uncorrelated_tool_output_refuses(self):
        self.events[3]["payload"]["call_id"] = "absent"
        with self.assertRaisesRegex(ValueError, "uncorrelated_tool_output"):
            self.verify()

    def test_duplicate_tool_output_is_not_another_observation(self):
        self.events.append(copy.deepcopy(self.events[3]))
        with self.assertRaisesRegex(ValueError, "uncorrelated_tool_output"):
            self.verify()

    def test_same_name_in_another_rig_or_unreachable_status_refuses(self):
        for status in [{"daemon": {"reachable": True},
                        "rigs": [{"id": "other", "name": "fixture"}]},
                       {"daemon": {"reachable": False},
                        "rigs": [{"id": "rig", "name": "fixture"}]}]:
            self.events[9]["payload"]["output"][0]["text"] = json.dumps({
                "exit_code": 0, "output": json.dumps(status)})
            with self.assertRaisesRegex(ValueError, "native_tool_evidence_missing"):
                self.verify()

    def test_duplicate_native_session_refuses(self):
        self.events.append(event("session_meta", {"cwd": "/fixture"}))
        with self.assertRaisesRegex(ValueError, "native_session_or_context_missing"):
            self.verify()


if __name__ == "__main__":
    unittest.main()
