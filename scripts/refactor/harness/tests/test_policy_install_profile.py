"""Synthetic native protected-install receipt checks without Docker."""

import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import types
import unittest
from unittest.mock import patch

SOURCE = Path(__file__).resolve().parent.parent / "source/integration/policy_install_profile.py"


def parse_arguments(argv):
    if not isinstance(argv, list) or not argv or argv[0] != "tadx":
        raise ValueError("Not TADX")
    words, flags = [], {}
    args = iter(argv[1:])
    for arg in args:
        if arg in {"--json", "--full"}:
            flags[arg[2:]] = True
        elif arg in {"--template", "--output"}:
            flags[arg[2:]] = next(args)
        else:
            words.append(arg)
    return words, flags


class NativeReceiptTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        integration = types.ModuleType("integration")
        integration.__path__ = []
        local = types.ModuleType("integration.g9_policy_profile")
        local.digest = lambda value: "a" * 64
        private = types.ModuleType("integration.g9_policy_install_private")
        private.POLICY_DIR = "/etc/tadx"
        private.POLICY = "/etc/tadx/managed-policy.json"
        private.LOCATOR = "/etc/tadx-policy-location.json"
        validation = types.ModuleType("integration.validation_profiles")
        validation.parse_arguments = parse_arguments
        cls.modules = patch.dict(sys.modules, {"integration": integration,
            "integration.g9_policy_profile": local, "integration.g9_policy_install_private": private,
            "integration.validation_profiles": validation})
        cls.modules.start()
        spec = importlib.util.spec_from_file_location("install_under_test", SOURCE)
        cls.profile = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.profile)

    @classmethod
    def tearDownClass(cls):
        cls.modules.stop()

    def command(self, **changes):
        flags = {"template": "read-only", "output": "/etc/tadx", "json": True, "full": True}
        result = {"path": "/etc/tadx/managed-policy.json", "template": "read-only",
                  "policy_written": True, "locator_published": True, "active": True, "phase": "complete"}
        command = {"capability": "policy.install", "phase": "task", "executed": True,
                   "exit_status": 0, "flags": flags,
                   "argv": ["tadx", "policy", "install", "--template", "read-only",
                            "--output", "/etc/tadx", "--json", "--full"],
                   "stdout": json.dumps(result)}
        command.update(changes)
        return command

    def test_exact_installed_receipt(self):
        command, result = self.profile._native_result([self.command()])
        self.assertEqual(command["capability"], "policy.install")
        self.assertTrue(result["active"])

    def test_preflight_inspects_immutable_image_as_root_without_mounts_or_network(self):
        calls = []
        def run(argv, **kwargs):
            calls.append(argv)
            return types.SimpleNamespace(returncode=0)
        with patch.object(self.profile.subprocess, "run", side_effect=run):
            self.profile._machine_policy_absent_root("sha256:" + "a" * 64)
        self.assertEqual(len(calls), 1)
        self.assertIn("0:0", calls[0])
        self.assertIn("--read-only", calls[0])
        self.assertIn("none", calls[0])
        self.assertNotIn("--mount", calls[0])
        with self.assertRaisesRegex(ValueError, "not immutable"):
            self.profile._machine_policy_absent_root("latest")

    def test_different_template_path_phase_or_multiple_commands_rejected(self):
        base = self.command()
        variants = [
            [self.command(exit_status=1)],
            [self.command(argv=["tadx", "policy", "install", "--template", "superuser",
                                "--output", "/etc/tadx", "--json", "--full"])],
            [self.command(stdout=json.dumps({**json.loads(base["stdout"]), "active": False}))],
            [base, base],
        ]
        for commands in variants:
            with self.subTest(commands=commands), self.assertRaises(ValueError):
                self.profile._native_result(commands)

    def test_mount_provenance_requires_exact_worker_sources_and_named_volumes(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        work = root / "agent-public"
        skills = root / "capture/internal/agent/skills"
        work.mkdir()
        skills.mkdir(parents=True)
        work_source, skills_source = str(work.resolve()), str(skills.resolve())
        state = {"socket_volume": "socket-case", "volume": "audit-case",
                 "config_volume": "config-case", "manifest": {"root": str(root / "capture")}}
        delivery = {"mounts": [
            {"Destination": "/work", "Type": "bind", "Source": work_source},
            {"Destination": "/skills", "Type": "bind", "Source": skills_source}],
            "cli_mounts": [
                {"Destination": "/work", "Type": "bind", "RW": False, "Source": work_source},
                {"Destination": "/skills", "Type": "bind", "RW": False, "Source": skills_source},
                {"Destination": "/run/tadx-broker", "Type": "volume", "RW": True, "Name": "socket-case"},
                {"Destination": "/audit", "Type": "volume", "RW": True, "Name": "audit-case"},
                {"Destination": "/cli-state", "Type": "volume", "RW": True, "Name": "config-case"}]}
        self.assertTrue(self.profile._mounts_proven(state, delivery, work))
        for changed in ({"Destination": "/work", "Type": "bind", "RW": False, "Source": "/etc"},
                        {"Destination": "/skills", "Type": "bind", "RW": True, "Source": skills_source},
                        {"Destination": "/audit", "Type": "volume", "RW": True, "Name": "other-audit"}):
            with self.subTest(changed=changed):
                altered = {**delivery, "cli_mounts": [
                    changed if row["Destination"] == changed["Destination"] else row
                    for row in delivery["cli_mounts"]]}
                self.assertFalse(self.profile._mounts_proven(state, altered, work))
        wrong_worker = {**delivery, "mounts": [
            {"Destination": "/work", "Type": "bind", "Source": "/etc"},
            delivery["mounts"][1]], "cli_mounts": [
            {**delivery["cli_mounts"][0], "Source": "/etc"}, *delivery["cli_mounts"][1:]]}
        self.assertFalse(self.profile._mounts_proven(state, wrong_worker, work))


if __name__ == "__main__":
    unittest.main()
