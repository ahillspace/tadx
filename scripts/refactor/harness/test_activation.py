"""Offline authorization checks with synthetic evidence and no dispatcher."""

import json
from pathlib import Path
import tempfile
import unittest

import activation


def encoded(value):
    return (json.dumps(value, sort_keys=True, indent=2) + "\n").encode()


class ActivationTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.prepared = self.root / "prepared"
        self.prepared.mkdir()
        self.source = "a" * 40
        self.cases = [f"P-synthetic-{index:03d}" for index in range(131)]
        self.candidate = {
            "source_revision": self.source,
            "source_tree_sha256": "b" * 64,
            "builds": {"linux/amd64": "c" * 64, "windows/amd64": "d" * 64},
        }
        self.settings = self.write("settings.json", {"settings": {"selected_site": "site-a"}},
                                   base=self.root)
        self.remote = self.write("remote.json", {
            "environment": "site-a", "server_url": "https://example.invalid",
            "site_content_url": "site-a", "site_luid": "1" * 36,
            "api_version": "3.25", "agent": {"pat_secret": "FAKE-SENTINEL"}}, base=self.root)
        self.runner = {
            "deployment": {"operator_settings_file": str(self.settings),
                           "remote_binding_file": str(self.remote)},
            "run_constraints": {"g9_saved_consent_authority": {
                "consent_evidence_sha256": "e" * 64}}}
        self.write("candidate/candidate.json", self.candidate)
        self.capture = {"source_commit": self.source,
                        "source_tree_digest": self.candidate["source_tree_sha256"],
                        "binaries": self.candidate["builds"],
                        "skills": {"tadx": {"SKILL.md": "1" * 64}}}
        self.write("candidate/capture/manifest.json", self.capture)
        self.write("source/runner.local.json", self.runner)
        self.preparation = {
            "status": "prepared_blocked", "live_execution_enabled": False,
            "candidate": self.candidate, "cases": self.cases,
            "authority_sha256": "f" * 64, "consent_evidence_sha256": "e" * 64,
            "runner_authority_sha256": activation._sha(encoded(
                self.runner["run_constraints"]["g9_saved_consent_authority"])),
            "capture_manifest_sha256": "2" * 64,
            "snapshot_files": {
                "candidate/candidate.json": activation._sha(encoded(self.candidate)),
                "candidate/capture/manifest.json": activation._sha(encoded(self.capture)),
                "source/runner.local.json": activation._sha(encoded(self.runner)),
            },
        }
        self.write("preparation.json", self.preparation)
        self.setup = {"provider": "openai", "model": "gpt-6-luna", "effort": "medium",
                      "thread_started": True, "thread_matched": True, "turn_completed": True,
                      "container_removed": True, "process_exit": 0,
                      "image_id": "sha256:" + "2" * 64,
                      "runtime_version": "codex-cli 0.160.0"}
        self.write("setup.json", self.setup, base=self.root)
        self.qualified = {"status": "copied_inputs_qualified_offline",
                          "live_execution_enabled": False, "candidate": self.candidate,
                          "worker_input_sha256": "1" * 64}
        self.write("qualified.json", self.qualified, base=self.root)
        self.write("gates.json", {"accepted_source": self.source}, base=self.root)
        self.record = {
            "schema_version": 1, "status": "authorized_g9",
            "preparation_sha256": self.digest("preparation.json"),
            "candidate_sha256": self.digest("candidate/candidate.json"),
            "runner_config_sha256": self.digest("source/runner.local.json"),
            "source_revision": self.source,
            "source_tree_sha256": self.candidate["source_tree_sha256"],
            "builds": self.candidate["builds"], "cases": self.cases,
            "selected_ids": self.cases, "purpose": "full_catalog",
            "authority_sha256": "f" * 64, "consent_evidence_sha256": "e" * 64,
            "operator_settings_input": {"path": str(self.settings),
                                        "sha256": self.digest("settings.json", base=self.root)},
            "remote_target": {"path": str(self.remote),
                              "identity": activation._remote_target(self.remote)},
            "prior_gates": {f"G{index}": "passed" for index in range(9)},
            "prior_gates_evidence": self.bound("gates.json"),
            "copied_inputs_qualification": self.bound("qualified.json"),
            "worker_input_sha256": "1" * 64,
            "image": {"id": "sha256:" + "2" * 64, "worker_input_sha256": "1" * 64},
            "model_setup": self.setup, "model_setup_evidence": self.bound("setup.json"),
            "windows_fixture_version": "1.0.0-g9.git" + self.source[:12],
        }
        self.write("authorization.json", self.record, base=self.root)

    def write(self, name, value, *, base=None):
        path = (base or self.prepared) / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(encoded(value))
        return path

    def digest(self, name, *, base=None):
        return activation._sha(((base or self.prepared) / name).read_bytes())

    def bound(self, name):
        return {"path": str(self.root / name), "sha256": self.digest(name, base=self.root)}

    def verify(self):
        return activation.verify(self.root / "authorization.json",
                                 self.digest("authorization.json", base=self.root),
                                 self.prepared, check_snapshot=True, check_launch_inputs=True)

    def test_exact_operator_digest_and_snapshot_pass(self):
        self.assertEqual(self.verify(), self.record)

    def test_missing_or_self_claimed_operator_digest_fails(self):
        with self.assertRaisesRegex(ValueError, "operator digest missing"):
            activation.verify(self.root / "authorization.json", None, self.prepared)
        with self.assertRaisesRegex(ValueError, "operator digest differs"):
            activation.verify(self.root / "authorization.json", "0" * 64, self.prepared)

    def test_tampered_prepared_input_fails(self):
        self.write("source/runner.local.json", {"changed": True})
        with self.assertRaisesRegex(ValueError, "candidate, preparation, runner"):
            self.verify()

    def test_same_source_capture_with_altered_guidance_fails(self):
        self.write("candidate/capture/manifest.json",
                   {**self.capture, "skills": {"tadx": {"SKILL.md": "3" * 64}}})
        with self.assertRaisesRegex(ValueError, "prepared snapshot changed"):
            self.verify()

    def test_unsupported_plan_or_gate_fails(self):
        for key, value in (("cases", self.cases[:-1]),
                           ("selected_ids", self.cases[:-1]),
                           ("prior_gates", {"G0": "passed"}),
                           ("windows_fixture_version", "1.0.0-g9.git" + "0" * 12)):
            original = self.record[key]
            self.record[key] = value
            self.write("authorization.json", self.record, base=self.root)
            with self.subTest(key=key), self.assertRaises(ValueError):
                self.verify()
            self.record[key] = original

    def test_exact_hosted_pilot_is_distinct_from_full_catalog(self):
        self.record["purpose"] = "windows_qualification"
        self.record["selected_ids"] = ["V-installer-windows-fresh"]
        self.preparation["cases"] = sorted([*self.cases[:-1], "V-installer-windows-fresh"])
        self.record["cases"] = self.preparation["cases"]
        self.write("preparation.json", self.preparation)
        self.record["preparation_sha256"] = self.digest("preparation.json")
        self.write("authorization.json", self.record, base=self.root)
        self.assertEqual(self.verify()["selected_ids"], ["V-installer-windows-fresh"])
        self.record["purpose"] = "full_catalog"
        self.write("authorization.json", self.record, base=self.root)
        with self.assertRaisesRegex(ValueError, "full catalog"):
            self.verify()

    def test_image_model_or_evidence_mismatch_fails(self):
        changes = (("image", {"id": "mutable:latest", "worker_input_sha256": "1" * 64}),
                   ("model_setup", {**self.setup, "effort": "high"}),
                   ("model_setup_evidence", {**self.bound("setup.json"), "sha256": "0" * 64}))
        for key, value in changes:
            original = self.record[key]
            self.record[key] = value
            self.write("authorization.json", self.record, base=self.root)
            with self.subTest(key=key), self.assertRaises(ValueError):
                self.verify()
            self.record[key] = original

    def test_settings_or_remote_target_change_before_launch_fails(self):
        self.write("settings.json", {"settings": {"selected_site": "site-b"}}, base=self.root)
        with self.assertRaisesRegex(ValueError, "operator settings changed"):
            self.verify()
        self.write("settings.json", {"settings": {"selected_site": "site-a"}}, base=self.root)
        self.write("remote.json", {**self.record["remote_target"]["identity"],
                                   "site_content_url": "site-b"}, base=self.root)
        with self.assertRaisesRegex(ValueError, "remote target changed"):
            self.verify()

    def test_remote_credential_value_is_not_in_authorization(self):
        self.assertNotIn("FAKE-SENTINEL", encoded(self.record).decode())

    def test_saved_consent_authority_hash_mismatch_fails(self):
        self.preparation["runner_authority_sha256"] = "0" * 64
        self.write("preparation.json", self.preparation)
        self.record["preparation_sha256"] = self.digest("preparation.json")
        self.write("authorization.json", self.record, base=self.root)
        with self.assertRaisesRegex(ValueError, "saved consent authority differs"):
            self.verify()


if __name__ == "__main__":
    unittest.main()
