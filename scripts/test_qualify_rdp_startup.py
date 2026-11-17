import unittest
import importlib.util
from pathlib import Path

spec = importlib.util.spec_from_file_location("rdp_startup", Path(__file__).with_name("qualify-rdp-startup.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
admits_legacy_loss, admits_qualified = module.admits_legacy_loss, module.admits_qualified


class RdpAdmission(unittest.TestCase):
    def setUp(self):
        self.pixels = [{"phase": phase, "rgba": color} for phase, color in [
            ("initial", [0, 0, 255, 255]), ("update", [0, 255, 0, 255]),
            ("reload", [0, 255, 0, 255]), ("fullscreen", [0, 255, 0, 255])]]

    def test_unrelated_failure_is_not_the_known_legacy_race(self):
        failure = [{"kind": "failure", "phase": "reload"},
                   {"kind": "failure-canvases", "canvases": [{"width": 0, "height": 0}, {"width": 64, "height": 64}]}]
        marker = "initial_size_before_registration=1"
        receipt = {"stalledPrevious": False, "events": self.pixels[:2] + failure}
        self.assertTrue(admits_legacy_loss(1, receipt, marker))
        self.assertFalse(admits_legacy_loss(0, receipt, marker))
        self.assertFalse(admits_legacy_loss(2, receipt, marker))
        self.assertFalse(admits_legacy_loss(1, receipt, ""))
        self.assertFalse(admits_legacy_loss(1, {"events": failure}, marker))
        failure[0]["phase"] = "setup"
        self.assertFalse(admits_legacy_loss(1, receipt, marker))

    def test_qualified_requires_all_pixel_phases_and_keyboard_execution(self):
        receipt = {"stalledPrevious": False, "events": self.pixels + [{"kind": "process-start", "command": "xev"}]}
        marker = "RDP_SCHEDULE delayed registration;"
        self.assertTrue(admits_qualified(0, receipt, marker))
        self.assertFalse(admits_qualified(1, receipt, marker))
        self.assertFalse(admits_qualified(0, receipt, ""))
        self.assertFalse(admits_qualified(0, {"events": self.pixels}, marker))
        self.assertFalse(admits_qualified(0, {"events": receipt["events"][1:]}, marker))
        self.assertFalse(admits_qualified(0, receipt, marker + "RDP_SCHEDULE ERROR"))

    def test_jpeg_rounding_needs_transport_evidence_and_still_rejects_stale_frames(self):
        event = {"phase": "reload", "rgba": [0, 254, 1, 255]}
        self.assertFalse(module.matches_pixel(event, "reload", [0, 255, 0, 255]))
        event["jpeg"] = True
        self.assertTrue(module.matches_pixel(event, "reload", [0, 255, 0, 255]))
        for rgba in ([0, 0, 253, 255], [0, 0, 0, 255], [0, 255, 0, 254], []):
            event["rgba"] = rgba
            self.assertFalse(module.matches_pixel(event, "reload", [0, 255, 0, 255]))


if __name__ == "__main__":
    unittest.main()
