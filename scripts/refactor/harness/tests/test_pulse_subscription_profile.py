"""Synthetic subscription-list fixture checks; no network or credentials."""

import hashlib
import importlib.util
import json
from pathlib import Path
import sys
import types
import unittest
from unittest.mock import patch


SOURCE = Path(__file__).resolve().parent.parent / "source/integration/pulse_subscription_profile.py"


class Blocked(Exception):
    pass


def subscription(raw):
    follower = raw["follower"]
    return {"subscription_id": raw["id"], "metric_id": raw["metric_id"],
            "principal_type": "USER", "principal_id": follower["user_id"]}


def strict_json(raw):
    if len(raw) > 1024:
        raise Blocked("Oversized JSON")
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise Blocked("Duplicate JSON field")
            result[key] = value
        return result
    value = json.loads(raw, object_pairs_hook=unique,
                       parse_constant=lambda value: (_ for _ in ()).throw(Blocked(value)))
    if not isinstance(value, dict):
        raise Blocked("JSON object required")
    return value


class Oracle:
    def __init__(self, pages):
        self.pages = list(pages)
        self.queries = []

    def pulse(self, suffix):
        self.queries.append(suffix)
        return self.pages.pop(0)


def row(identity, user="user-1"):
    return {"id": identity, "metric_id": "metric-1", "follower": {"user_id": user}}


class SubscriptionProfileTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        core = types.ModuleType("bench.core")
        core.Blocked = Blocked
        core.digest = lambda value: hashlib.sha256(json.dumps(value, sort_keys=True).encode()).hexdigest()
        core.load = lambda path: json.loads(Path(path).read_text())
        core.save = lambda path, value: Path(path).write_text(json.dumps(value))
        pulse = types.ModuleType("integration.pulse_profiles")
        pulse.exact = lambda value, label: value if isinstance(value, str) and value else (_ for _ in ()).throw(Blocked(label))
        pulse.subscription = subscription
        pulse._audited_flags = lambda command, action: command["flags"]
        pulse._decode_native = json.loads
        pulse._json = strict_json
        pulse.MAX_BODY = 1024
        pulse.relation = lambda value: {key: value[key] for key in ("metric_id", "principal_type", "principal_id")}
        remote = types.ModuleType("integration.remote_read_profiles")
        validation = types.ModuleType("integration.validation_profiles")
        validation._gate_authorized = lambda req: None
        bench = types.ModuleType("bench")
        bench.__path__ = []
        integration = types.ModuleType("integration")
        integration.__path__ = []
        cls.modules = patch.dict(sys.modules, {"bench": bench, "bench.core": core,
            "integration": integration, "integration.pulse_profiles": pulse,
            "integration.remote_read_profiles": remote,
            "integration.validation_profiles": validation})
        cls.modules.start()
        spec = importlib.util.spec_from_file_location("subscription_under_test", SOURCE)
        cls.profile = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.profile)

    @classmethod
    def tearDownClass(cls):
        cls.modules.stop()

    def test_complete_paginated_user_inventory(self):
        oracle = Oracle([{"subscriptions": [row("sub-1")], "next_page_token": "next"},
                         {"subscriptions": [row("sub-2")], "next_page_token": ""}])
        result = self.profile.user_subscriptions(oracle, "user-1")
        self.assertEqual([item["subscription_id"] for item in result], ["sub-1", "sub-2"])
        self.assertIn("user_id=user-1", oracle.queries[0])
        self.assertIn("page_token=next", oracle.queries[1])

    def test_reject_duplicate_wrong_user_and_missing_continuation(self):
        pages = [
            [{"subscriptions": [row("sub-1"), row("sub-1")]}],
            [{"subscriptions": [row("sub-1", "other-user")]}],
            [{"subscriptions": [], "next_page_token": "next"}],
            [{"subscriptions": [row("sub-1")], "total_available": 2}],
        ]
        for sequence in pages:
            with self.subTest(sequence=sequence), self.assertRaises(Blocked):
                self.profile.user_subscriptions(Oracle(sequence), "user-1")

    def command(self, data, *, flags=None, binary=None):
        binary = binary or "a" * 64
        return {"capability": "pulse.subscription.list", "phase": "task", "executed": True,
                "exit_status": 0, "flags": {"all": True, "json": True, "full": True} if flags is None else flags,
                "id": "cmd-1", "binary_sha256": binary,
                "evidence": "audit/commands.jsonl#cmd-1", "stdout_evidence": "audit/cmd-1.stdout.txt",
                "stdout": json.dumps(data)}

    def test_native_requires_complete_exact_user_and_owned_relation(self):
        expected = [{"subscription_id": "sub-1", "metric_id": "metric-1",
                     "principal_type": "USER", "principal_id": "user-1"}]
        baseline = {"settings": {"user_id": "user-1"},
                    "before": {"records": expected, "owned_subscription_id": "sub-1"}}
        req = {"frozen_cli": {"binaries": {"linux": {"sha256": "a" * 64}}}}
        binding = {"environment": "fixture", "site_content_url": "site"}
        data = {"status": "listed", "user_luid": "user-1", "environment": "fixture", "site": "site",
                "coverage": {"scope": "authenticated_user_filter", "complete": True,
                             "more_available": False}, "count": 1,
                "subscriptions": [{"subscription_luid": "sub-1", "metric_luid": "metric-1",
                                   "follower_type": "USER", "follower_luid": "user-1"}]}
        result = self.profile.native_evidence(req, [self.command(data)], binding, baseline)
        self.assertEqual(result["status"], "verified")
        self.assertEqual(result["candidate"]["records"], expected)
        cases = [(dict(data, user_luid="other"), None),
                 (dict(data, coverage={**data["coverage"], "complete": False}), None),
                 (dict(data, subscriptions=[]), None),
                 (data, {"json": True, "full": True}),
                 (data, {"all": True}),
                 (data, {"all": True, "json": True}),
                 (data, {"all": True, "full": True}),
                 (data, {"all": True, "limit": 25})]
        for changed, flags in cases:
            with self.subTest(changed=changed, flags=flags):
                rejected = self.profile.native_evidence(req, [self.command(changed, flags=flags)], binding, baseline)
                self.assertEqual(rejected["status"], "not_verified")
                self.assertIsNone(rejected["candidate"])
        self.assertEqual(self.profile.native_evidence(req, [self.command(data), self.command(data)], binding, baseline)
                         ["status"], "not_verified")
        self.assertEqual(self.profile.native_evidence({"frozen_cli": {}}, [self.command(data)], binding, baseline)
                         ["status"], "not_verified")
        toon = self.command(data)
        toon["stdout"] = "status: listed\ncount: 1\n"
        self.assertEqual(self.profile.native_evidence(req, [toon], binding, baseline)["status"], "not_verified")
        for raw in ("[]", '{"status":"listed","status":"listed"}', "{" + " " * 1024 + "}"):
            with self.subTest(raw=raw[:40]):
                malformed = self.command(data)
                malformed["stdout"] = raw
                self.assertEqual(self.profile.native_evidence(req, [malformed], binding, baseline)
                                 ["status"], "not_verified")


if __name__ == "__main__":
    unittest.main()
