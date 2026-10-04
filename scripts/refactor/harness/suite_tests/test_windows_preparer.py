"""Exact locked-suite Windows extension checks; no live dispatch."""

import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
SHA64 = "b" * 64
sys.path.insert(0, str(ROOT))
import prepare
from windows_preparer import IDS, HOSTED_WALL_SECONDS


class WindowsPreparerTest(unittest.TestCase):
    def test_installer_fixture_shape_passes_current_admission(self):
        raw = os.environ.get("TADX_G9_CURRENT_SUITE")
        if not raw:
            raise RuntimeError("Set TADX_G9_CURRENT_SUITE to the current source-locked suite")
        harness = Path(raw)
        sys.path.insert(0, str(harness))
        self.addCleanup(sys.path.remove, str(harness))
        from bench.admission import require_admission
        common_spec = importlib.util.spec_from_file_location(
            "g9_current_scenario_common", harness / "integration/scenario_common.py")
        common = importlib.util.module_from_spec(common_spec)
        common_spec.loader.exec_module(common)
        exercise = json.loads((harness / "suite/exercises/V-installer-windows-fresh.json").read_text())
        with tempfile.TemporaryDirectory() as directory:
            req = {"case_id": "case-1", "private_case_dir": directory,
                   "exercise": exercise}
            fixture = common.bind_fixture(req,
                                   {"task_inputs": {"command": "tadx-bench-installer",
                                                    "arguments": ["install", "--version", "0.1.3-g9"]}},
                                   {"target": {"path": "hosted:test-owned-install-dir"},
                                    "sentinel": {"path": "hosted:test-owned-sentinel"}},
                                   {"result": {"binary_sha256": SHA64},
                                    "canonical": {"case": "fresh"}},
                                   {"binary_sha256": None}, source="g9-windows-baseline.json",
                                   conditions=[{"helper": "installer"}])
            self.assertEqual(require_admission(exercise, fixture, private_root=directory,
                                               config={})["status"], "qualified")

    def test_locked_current_suite_windows_patch_and_bridge_compile(self):
        raw = os.environ.get("TADX_G9_CURRENT_SUITE")
        if not raw:
            raise RuntimeError("Set TADX_G9_CURRENT_SUITE to the current source-locked suite")
        harness = Path(raw)
        lock = json.loads((ROOT / "source-lock-124.json").read_text())
        files = {}
        for name, expected in lock["files"].items():
            blob = (harness / name).read_bytes()
            self.assertEqual(hashlib.sha256(blob).hexdigest(), expected, name)
            files[name] = blob
        patches = prepare.apply_mutation_runtime(files)
        patches.extend(prepare.patch_sources(files))
        selected = {row["id"] for row in json.loads(files["suite/index.json"])["exercises"]}
        self.assertEqual(len(selected), 131)
        self.assertEqual(sum(identity.startswith("P-") for identity in selected), 124)
        self.assertEqual(selected & IDS, IDS)
        self.assertFalse(selected & {"V-mutation-env-precedence",
                                     "V-mutation-no-setting-authorization"})
        self.assertEqual({p["path"] for p in patches if p["path"].startswith(
            ("suite/exercises/P-mutation-", "fixtures/profiles/P-mutation-"))},
            {folder + "/" + identity + ".json"
             for folder in ("suite/exercises", "fixtures/profiles")
             for identity in ("P-mutation-set", "P-mutation-status")})
        self.assertEqual({p["path"] for p in patches if p["path"].startswith(
            "suite/exercises/V-installer-windows-")},
            {"suite/exercises/" + identity + ".json" for identity in IDS})
        for identity in IDS:
            exercise = json.loads(files["suite/exercises/" + identity + ".json"])
            self.assertEqual(exercise["evaluator"]["budgets"]["wall_seconds"],
                             HOSTED_WALL_SECONDS)
        bridge = files["integration/docker_local_bridge.py"]
        self.assertIn(b"g9_windows_installer_watch import run_model", bridge)
        self.assertIn(b"g9_windows_installer_broker.cjs", bridge)
        self.assertIn(b"G9 bridge dispatch is blocked", bridge)
        compile(bridge, "integration/docker_local_bridge.py", "exec")
        for target, source in {"g9_consent.py": "consent.py",
                               "g9_project_read.py": "project_read.py",
                               "g9_project_read_broker.cjs": "project_read_broker.cjs",
                               "g9_job_broker.cjs": "source/integration/job_broker.cjs",
                               "g9_job_contract.py": "source/integration/job_contract.py",
                               "g9_job_fixture.py": "source/integration/job_fixture.py",
                               "g9_job_profile.py": "source/integration/job_profile.py",
                               "g9_policy_broker.cjs": "source/integration/policy_broker.cjs",
                               "g9_policy_profile.py": "source/integration/policy_profile.py",
                               "g9_policy_private.py": "source/integration/policy_private.py",
                               "g9_policy_install_profile.py": "source/integration/policy_install_profile.py",
                               "g9_policy_install_private.py": "source/integration/policy_install_private.py",
                               "g9_policy_install_broker.cjs": "source/integration/policy_install_broker.cjs",
                               "g9_pulse_subscription_profile.py": "source/integration/pulse_subscription_profile.py"}.items():
            files["integration/" + target] = (ROOT / source).read_bytes()
        files.update(prepare.windows_runtime_sources())
        self.assertEqual(files["integration/g9_windows_gate.py"],
                         (prepare.WORKER / "windows_gate.py").read_bytes())
        prepare.validate_source_closure(files, harness)
        with tempfile.TemporaryDirectory() as directory:
            local = Path(directory) / "local_broker.cjs"
            local.write_bytes(files["integration/local_broker.cjs"])
            checked = subprocess.run(["node", "--check", str(local)], capture_output=True,
                                     text=True, timeout=15)
            self.assertEqual(checked.returncode, 0, checked.stderr)


if __name__ == "__main__":
    unittest.main()
