"""Offline profile and no-command watcher tests; no hosted dispatch."""

import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import types
import unittest
from unittest.mock import Mock, patch


SOURCE = Path(__file__).resolve().parents[1] / "source/integration"
WORKER = Path(__file__).resolve().parents[3] / "g9"
SHA40 = "a" * 40
SHA64 = "b" * 64


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    loaded = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(loaded)
    return loaded


class WindowsProfileTest(unittest.TestCase):
    def modules(self):
        package = types.ModuleType("integration")
        package.__path__ = [str(SOURCE)]
        gate = module("integration.g9_windows_gate", WORKER / "windows_gate.py")
        relay = types.ModuleType("integration.g9_windows_host_relay")
        relay.Refused = gate.Refused
        relay.prepare_hosted = Mock()
        relay.abort_hosted = Mock(return_value={"branch_cleanup": "verified",
                                                "native_cleanup": "verified"})
        common = types.ModuleType("integration.scenario_common")
        common.bind_fixture = Mock(side_effect=lambda req, public, roles, expect, before,
                                            **kwargs: {"expect": expect, "public": public,
                                                       "roles": roles})
        common.instructions = Mock(return_value="fixture instructions")
        scope = patch.dict(sys.modules, {"integration": package,
                                         "integration.g9_windows_gate": gate,
                                         "integration.g9_windows_host_relay": relay,
                                         "integration.scenario_common": common})
        scope.start()
        self.addCleanup(scope.stop)
        profile = module("integration.g9_windows_installer_profile",
                         SOURCE / "g9_windows_installer_profile.py")
        sys.modules["integration.g9_windows_installer_profile"] = profile
        self.addCleanup(sys.modules.pop, "integration.g9_windows_installer_profile", None)
        return gate, relay, common, profile

    def test_preflight_requires_exact_accepted_source_before_hosted_setup(self):
        gate, relay, _, profile = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "scripts").mkdir()
            (root / "scripts/install.ps1").write_text("fixture", encoding="utf-8")
            frozen = {"source_commit": SHA40, "root": str(root),
                      "binaries": {"windows": {"sha256": SHA64}}}
            req = {"exercise": {"id": "V-installer-windows-fresh"},
                   "run_constraints": {"g9_windows_installer_version": "0.1.3-g9"}}
            with self.assertRaises(gate.Refused):
                profile.preflight(req, frozen)
            self.assertFalse(relay.prepare_hosted.called)
            req["run_constraints"]["g9_accepted_gate_sha"] = SHA40
            self.assertIn("platform:windows", profile.preflight(req, frozen)[
                "qualified_requirements"])

    def test_pre_task_baseline_binds_guard_and_missing_native_result_fails(self):
        gate, relay, common, profile = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / "source"
            (source / "scripts").mkdir(parents=True)
            (source / "scripts/install.ps1").write_text("fixture", encoding="utf-8")
            setup = gate.setup_request("fresh", SHA40, "nonce_123456789",
                                       version="0.1.3-g9", binary_sha256=SHA64,
                                       installer_sha256=profile.hashlib.sha256(b"fixture").hexdigest())
            before = {"binary_sha256": None, "completion_markers": 0,
                      "user_path_count": 0, "sentinel_sha256": "c" * 64,
                      "profile_sentinel": True}
            session = {"setup": setup, "setup_commit": "d" * 40,
                       "baseline_sha256": "e" * 64, "run_id": 17,
                       "baseline": {"before": before}, "area": str(root / "hosted")}
            relay.prepare_hosted.return_value = session
            req = {"exercise": {"id": "V-installer-windows-fresh"},
                   "private_case_dir": str(root), "case_id": "case-1",
                   "run_constraints": {"g9_accepted_gate_sha": SHA40,
                                       "g9_windows_installer_version": "0.1.3-g9"}}
            frozen = {"source_commit": SHA40, "root": str(source),
                      "binaries": {"windows": {"sha256": SHA64}}}
            prepared = profile.prepare(req, frozen, root)
            self.assertEqual(prepared["broker_guard"]["setup_commit"], "d" * 40)
            self.assertEqual(prepared["broker_guard"]["credential_input"], False)
            self.assertEqual(prepared["fixture"]["expect"]["result"]["binary_sha256"], SHA64)
            self.assertTrue(common.bind_fixture.called)
            observed = profile.observe(req, {}, {}, root, [])
            self.assertFalse(observed["native_command_verified"])
            self.assertIsNone(observed["canonical"])
            self.assertTrue(observed["unowned_changes"])
            self.assertEqual(profile.cleanup(req, {}, root)["status"], "pass")
            relay.abort_hosted.assert_called_once_with(session)

    def test_no_helper_request_cancels_hosted_session_without_result(self):
        _, relay, _, profile = self.modules()
        watcher = module("integration.g9_windows_installer_watch",
                         SOURCE / "g9_windows_installer_watch.py")
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            session = {"setup_commit": "d" * 40, "area": str(root / "hosted")}
            (root / profile.SESSION).write_text(json.dumps(session), encoding="utf-8")
            docker = Mock(return_value=types.SimpleNamespace(returncode=1))
            result = watcher.run_model({"private_case_dir": str(root)},
                                       {"broker_container_id": "exact-container",
                                        "delivery_complete": True},
                                       lambda: {"task": "finished"}, docker)
            self.assertEqual(result, {"task": "finished"})
            self.assertFalse((root / profile.OUTCOME).exists())
            self.assertEqual(json.loads((root / watcher.WATCH).read_text())["status"],
                             "no_request")
            relay.abort_hosted.assert_called_once_with(session)

    def test_fixture_assembly_failure_requires_verified_native_abort(self):
        gate, relay, common, profile = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / "source"
            (source / "scripts").mkdir(parents=True)
            (source / "scripts/install.ps1").write_text("fixture", encoding="utf-8")
            setup = gate.setup_request("fresh", SHA40, "nonce_123456789",
                                       version="0.1.3-g9", binary_sha256=SHA64,
                                       installer_sha256=profile.hashlib.sha256(b"fixture").hexdigest())
            session = {"setup": setup, "setup_commit": "d" * 40,
                       "baseline_sha256": "e" * 64, "run_id": 17,
                       "baseline": {"before": {"sentinel_sha256": "c" * 64}}}
            relay.prepare_hosted.return_value = session
            common.bind_fixture.side_effect = RuntimeError("synthetic fixture failure")
            req = {"exercise": {"id": "V-installer-windows-fresh"},
                   "private_case_dir": str(root), "case_id": "case-1",
                   "run_constraints": {"g9_accepted_gate_sha": SHA40,
                                       "g9_windows_installer_version": "0.1.3-g9"}}
            frozen = {"source_commit": SHA40, "root": str(source),
                      "binaries": {"windows": {"sha256": SHA64}}}
            with self.assertRaisesRegex(RuntimeError, "synthetic fixture failure"):
                profile.prepare(req, frozen, root)
            relay.abort_hosted.assert_called_once_with(session)
            marker = json.loads((root / profile.WATCH).read_text())
            self.assertEqual(marker, {"status": "prepare_failed",
                                      "branch_cleanup": "verified",
                                      "native_cleanup": "verified"})
            self.assertEqual(profile.cleanup(req, {}, root)["status"], "pass")
            relay.abort_hosted.assert_called_once()

    def test_result_cleanup_failure_blocks_profile_cleanup(self):
        _, relay, _, profile = self.modules()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / profile.SESSION).write_text("{}", encoding="utf-8")
            (root / profile.OUTCOME).write_text(json.dumps({
                "branch_cleanup": "verified", "native_result": {"cleanup_ok": False}}),
                encoding="utf-8")
            req = {"private_case_dir": str(root)}
            self.assertEqual(profile.cleanup(req, {}, root)["status"], "blocked")
            relay.abort_hosted.assert_not_called()


if __name__ == "__main__":
    unittest.main()
