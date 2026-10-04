"""Synthetic checks for the broker-private policy seed and stopped output copy."""

import hashlib
from pathlib import Path
import sys
import tempfile
import types
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "source/integration"))
import policy_private


class Blocked(Exception):
    pass


class PrivateBoundaryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.work = self.root / "agent-public"
        self.work.mkdir()
        self.sentinel = b"test-owned sentinel"
        self.candidate = b'{"version":1,"allowed_capabilities":[],"remote_mutations":false}'
        (self.work / "policy-sentinel.txt").write_bytes(self.sentinel)
        (self.work / "policy-candidate.json").write_bytes(self.candidate)
        core = types.ModuleType("bench.core")
        core.Blocked = Blocked
        bench = types.ModuleType("bench")
        bench.__path__ = []
        self.modules = patch.dict(sys.modules, {"bench": bench, "bench.core": core})
        self.modules.start()
        self.addCleanup(self.modules.stop)

    def state(self, action):
        return {"broker_container": "owned-cli", "broker_container_id": "immutable-id",
                "broker_guard": {"family": "g9-local-policy", "action": action,
                                 "case_id": "P-" + action.replace(".", "-"),
                                 "sentinel_sha256": hashlib.sha256(self.sentinel).hexdigest(),
                                 "candidate_sha256": hashlib.sha256(self.candidate).hexdigest()}}

    def request(self, action):
        return {"case_id": "P-" + action.replace(".", "-"),
                "private_case_dir": str(self.root)}

    def test_setup_copies_only_exact_seed_to_private_broker_tmpfs(self):
        calls = []

        def docker(*args, **kwargs):
            calls.append(args)
            if args[0:2] == ("exec", "owned-cli") and "node" in args:
                return types.SimpleNamespace(stdout='["' + hashlib.sha256(self.sentinel).hexdigest() +
                                             '","' + hashlib.sha256(self.candidate).hexdigest() + '"]')
            return types.SimpleNamespace(returncode=0)

        result = policy_private.setup(self.request("policy.validate"),
                                      self.state("policy.validate"), self.work, docker)
        self.assertEqual(result["status"], "verified")
        copies = [call for call in calls if call[0] == "cp"]
        self.assertEqual([call[2] for call in copies],
                         ["owned-cli:/cli-state/g9-policy-private/sentinel.txt",
                          "owned-cli:/cli-state/g9-policy-private/candidate.json"])
        self.assertFalse(any("/work/" in str(call[2]) for call in copies))

    def test_setup_rejects_changed_seed_before_copy(self):
        state = self.state("policy.validate")
        (self.work / "policy-candidate.json").write_bytes(b"changed")
        calls = []
        with self.assertRaises(Blocked):
            policy_private.setup(self.request("policy.validate"), state, self.work,
                                 lambda *args, **kwargs: calls.append(args))
        self.assertFalse(any(call[0] == "cp" for call in calls))

    def test_setup_rejects_preexisting_private_directory_before_copy(self):
        calls = []

        def docker(*args, **kwargs):
            calls.append(args)
            return types.SimpleNamespace(returncode=1)

        with self.assertRaises(Blocked):
            policy_private.setup(self.request("policy.samples"),
                                 self.state("policy.samples"), self.work, docker)
        self.assertEqual(len(calls), 1)
        self.assertNotIn("cp", calls[0])

    def test_capture_uses_only_stopped_owned_broker_and_rejects_old_target(self):
        calls = []

        def docker(*args, **kwargs):
            calls.append(args)
            return types.SimpleNamespace(returncode=0)

        state = self.state("policy.samples")
        result = policy_private.capture(self.request("policy.samples"), state,
                                        "immutable-id", docker)
        self.assertEqual(result["status"], "copied")
        self.assertEqual(calls, [("cp", "immutable-id:/cli-state/g9-policy-private/candidates",
                                  str(self.root / "policy-native-samples"))])
        (self.root / "policy-native-samples").mkdir()
        with self.assertRaises(Blocked):
            policy_private.capture(self.request("policy.samples"), state,
                                   "immutable-id", docker)


if __name__ == "__main__":
    unittest.main()
