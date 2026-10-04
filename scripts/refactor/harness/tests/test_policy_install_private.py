"""Offline stopped-overlay capture checks; no Docker or host policy writes."""

import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "source/integration"))
import policy_install_private as capture


class CaptureTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.identity = "a" * 64
        self.req = {"private_case_dir": str(self.root)}
        self.state = {"broker_guard": {"family": "g9-policy-install"},
                      "broker_container_id": self.identity}
        self.diff = "C /etc\nA /etc/tadx\nA /etc/tadx/managed-policy.json\nA /etc/tadx-policy-location.json\n"

    def docker(self, operation, source, *rest):
        if operation == "diff":
            return type("Response", (), {"stdout": self.diff})()
        self.assertEqual(operation, "cp")
        self.assertTrue(source.startswith(self.identity + ":/etc/"))
        target = Path(rest[0])
        if source.endswith("tadx-policy-location.json"):
            target.write_text(json.dumps("/etc/tadx") + "\n", encoding="utf-8")
        else:
            target.mkdir()
            (target / "managed-policy.json").write_text('{"version":1}\n', encoding="utf-8")
        return None

    def test_exact_stopped_policy_paths_are_captured(self):
        result = capture.capture(self.req, self.state, self.identity, self.docker)
        self.assertEqual(result["status"], "copied")
        self.assertEqual(result["container_id"], self.identity)
        self.assertEqual(len(result["locator_sha256"]), 64)
        self.assertEqual(len(result["policy_sha256"]), 64)

    def test_wrong_identity_and_unowned_overlay_change_fail_closed(self):
        with self.assertRaisesRegex(ValueError, "identity differs"):
            capture.capture(self.req, self.state, "b" * 64, self.docker)
        self.diff += "A /root/.config/unrelated\n"
        with self.assertRaisesRegex(ValueError, "outside the exact"):
            capture.capture(self.req, self.state, self.identity, self.docker)
        self.assertFalse((self.root / "policy-install-native").exists())

    def test_existing_evidence_or_wrong_locator_is_rejected(self):
        (self.root / "policy-install-native").mkdir()
        with self.assertRaisesRegex(ValueError, "already exists"):
            capture.capture(self.req, self.state, self.identity, self.docker)
        (self.root / "policy-install-native").rmdir()
        def wrong(operation, source, *rest):
            result = self.docker(operation, source, *rest)
            if operation == "cp" and source.endswith("tadx-policy-location.json"):
                Path(rest[0]).write_text(json.dumps("/etc/other"))
            return result
        with self.assertRaisesRegex(ValueError, "another directory"):
            capture.capture(self.req, self.state, self.identity, wrong)


if __name__ == "__main__":
    unittest.main()
