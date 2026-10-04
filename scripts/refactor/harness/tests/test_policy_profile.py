"""Offline synthetic checks for local policy setup, native proof, and cleanup."""

import json
from pathlib import Path
import sys
import tempfile
import types
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "source/integration"))
import policy_profile as profile


class Blocked(Exception):
    pass


def save(path, value):
    Path(path).write_text(json.dumps(value), encoding="utf-8")


def load(path):
    return json.loads(Path(path).read_text(encoding="utf-8"))


def parse_arguments(argv):
    if argv[0] != "tadx":
        raise ValueError("not TADX")
    words, flags = [], {}
    args = iter(argv[1:])
    for value in args:
        if value in {"--json", "--full"}:
            flags[value[2:]] = True
        elif value == "--output":
            flags["output"] = next(args)
        else:
            words.append(value)
    return words, flags


class ProfileTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.work = self.root / "agent-public"
        self.work.mkdir()
        (self.work / "deliverables").mkdir()
        self.registry = self.root / "registry.json"
        save(self.registry, [{"id": "policy.samples", "owner": "cli", "implementation": "implemented",
                              "command_path": ["policy", "samples"], "remote_mutation": False,
                              "administrative": False},
                             {"id": "policy.validate", "owner": "cli", "implementation": "implemented",
                              "command_path": ["policy", "validate"], "remote_mutation": False,
                              "administrative": False},
                             {"id": "policy.status", "owner": "cli", "implementation": "implemented",
                              "command_path": ["policy", "status"], "remote_mutation": False,
                              "administrative": False},
                             {"id": "admin.user.delete", "owner": "cli", "implementation": "implemented",
                              "command_path": ["admin", "user", "delete"], "remote_mutation": True,
                              "administrative": True}])
        self.image = "sha256:" + "a" * 64
        core = types.ModuleType("bench.core")
        core.Blocked, core.load, core.save = Blocked, load, save
        bench = types.ModuleType("bench")
        bench.__path__ = []
        validation = types.ModuleType("integration.validation_profiles")
        validation.parse_arguments = parse_arguments
        integration = types.ModuleType("integration")
        integration.__path__ = []
        self.modules = patch.dict(sys.modules, {"bench": bench, "bench.core": core,
                                                "integration": integration,
                                                "integration.validation_profiles": validation})
        self.modules.start()
        self.addCleanup(self.modules.stop)

    def request(self, action):
        case = "P-" + action.replace(".", "-")
        return {"case_id": case, "run_id": "run-1", "private_case_dir": str(self.root),
                "exercise": {"id": case}}

    def prepared(self, action):
        req = self.request(action)
        save(self.root / "qualification.json", {"image_id": self.image,
                                                 "profile": {"policy_case_id": req["case_id"],
                                                             "machine_policy_absent": True,
                                                             "inspected_image_id": self.image}})
        prepared = profile.prepare(req, {}, self.work)
        save(self.root / "delivery-evidence.json", {
            "cli_host_config": {"NetworkMode": "none"},
            "cli_mounts": [{"Destination": "/work", "RW": False}],
            "policy_private_setup": {"status": "verified"}})
        return req, prepared

    def command(self, action, result):
        words = action.split(".")
        flags = {"json": True, "full": True}
        argv = ["tadx", *words]
        if action == "policy.validate":
            argv.append(profile.BROKER_CANDIDATE)
        if action == "policy.samples":
            argv += ["--output", profile.BROKER_SAMPLES]
            flags["output"] = profile.BROKER_SAMPLES
        argv += ["--json", "--full"]
        return {"argv": argv, "flags": flags, "capability": action, "executed": True,
                "exit_status": 0, "stdout": json.dumps(result), "evidence": "audit/commands.jsonl#1"}

    def state(self):
        return {"frozen": True, "runtime_network": "none", "manifest": {"root": str(self.root)},
                "policy_native_output_capture": {"status": "copied"}}

    def test_preflight_checks_immutable_network_none_image_without_mounts(self):
        with patch.object(profile.subprocess, "run", return_value=types.SimpleNamespace(returncode=0)) as run:
            result = profile.preflight(self.request("policy.samples"),
                                       {"root": str(self.root), "runtime_image": self.image})
        args = run.call_args.args[0]
        self.assertEqual(result["machine_policy_absent"], True)
        self.assertEqual(args[args.index("--network") + 1], "none")
        self.assertNotIn("--mount", args)
        self.assertNotIn("--volume", args)
        with self.assertRaises(Blocked):
            profile.preflight(self.request("policy.install"),
                              {"root": str(self.root), "runtime_image": self.image})

    def test_validate_observation_requires_exact_native_result_and_unchanged_files(self):
        req, prepared = self.prepared("policy.validate")
        command = self.command("policy.validate", {"status": "valid",
            "candidate": profile.BROKER_CANDIDATE, "allowed_capabilities": 0,
            "remote_mutations": False})
        after = profile.observe(req, self.state(), prepared["config_seed"], self.work, [command])
        self.assertTrue(after["policy_contract_passed"])
        self.assertEqual(after["native_result_status"], "valid")
        cleanup = profile.cleanup(req, self.state(), self.work)
        self.assertEqual(cleanup["cleanup_status"], "pass")
        self.assertEqual(cleanup["local_state_differences"], [])
        (self.work / profile.CANDIDATE).write_bytes(b"changed")
        with self.assertRaises(Blocked):
            profile.observe(req, self.state(), prepared["config_seed"], self.work, [command])

    def test_samples_observer_checks_all_three_documents_and_receipt(self):
        req, prepared = self.prepared("policy.samples")
        directory = self.root / profile.CAPTURED_SAMPLES
        directory.mkdir()
        all_ids = sorted(row["id"] for row in load(self.registry))
        for name in profile.SAMPLE_NAMES:
            ids = all_ids if name != "read-write-no-admin.json" else [item for item in all_ids
                                                                   if item != "admin.user.delete"]
            save(directory / name, {"version": 1, "allowed_capabilities": ids,
                                    "remote_mutations": name != "read-only.json"})
        command = self.command("policy.samples", {"status": "created", "files": [
            profile.BROKER_SAMPLES + "/" + name for name in profile.SAMPLE_NAMES]})
        after = profile.observe(req, self.state(), prepared["config_seed"], self.work, [command])
        self.assertEqual(after["native_result_status"], "created")
        (directory / "extra.json").write_text("{}", encoding="utf-8")
        with self.assertRaises(Blocked):
            profile.observe(req, self.state(), prepared["config_seed"], self.work, [command])

    def test_status_observer_rejects_unverified_result_and_network(self):
        req, prepared = self.prepared("policy.status")
        command = self.command("policy.status", {"state": "unmanaged", "candidate_valid": False})
        self.assertEqual(profile.observe(req, self.state(), prepared["config_seed"],
                                         self.work, [command])["native_result_status"], "unmanaged")
        with self.assertRaises(Blocked):
            profile.observe(req, self.state(), prepared["config_seed"], self.work,
                            [command, command])
        with self.assertRaises(Blocked):
            profile.observe(req, self.state(), prepared["config_seed"], self.work,
                            [self.command("policy.status", {"state": "active"})])
        save(self.root / "delivery-evidence.json", {"cli_host_config": {"NetworkMode": "bridge"}})
        with self.assertRaises(Blocked):
            profile.observe(req, self.state(), prepared["config_seed"], self.work, [command])


if __name__ == "__main__":
    unittest.main()
