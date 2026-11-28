"""Fast guard for native fault injection targets; no servers or credentials."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("credential_qualifier", Path(__file__).with_name("qualify-credential-hub.py"))
qualifier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(qualifier)

class EnrolledHubPinTests(unittest.TestCase):
    def test_local_and_other_peers_cannot_select_the_fault_target(self):
        local = {"id": "local", "identity": {"tls_key_sha256": ""}}
        other = {"id": "other", "identity": {"tls_key_sha256": "b" * 64}}
        hub = {"id": "hub", "identity": {"tls_key_sha256": "a" * 64}}
        for peers in ([local, other, hub], [hub, other, local]):
            self.assertEqual(qualifier.enrolled_hub_pin({"servers": peers}), "a" * 64)

    def test_missing_ambiguous_or_unverified_hub_is_refused_before_mutation(self):
        hub = {"id": "hub", "identity": {"tls_key_sha256": "a" * 64}}
        for peers in ([], [hub, hub], [{"id": "local"}],
                      [{"id": "hub", "identity": {}}],
                      [{"id": "hub", "identity": {"tls_key_sha256": "unverified"}}]):
            with self.subTest(peers=peers), self.assertRaises(AssertionError):
                qualifier.enrolled_hub_pin({"servers": peers})

if __name__ == "__main__":
    unittest.main()
