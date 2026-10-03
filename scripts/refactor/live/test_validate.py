"""Synthetic files exercise offline G9 acceptance and rejection paths."""

import copy
import json
from pathlib import Path
import tempfile
import unittest

import validate as gate


MISSING_EIGHT = (
    "job.cancel", "job.inspect", "job.wait", "policy.install", "policy.samples",
    "policy.status", "policy.validate", "pulse.subscription.list",
)


class LiveGateTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.build = self.root / "candidate.bin"
        self.build.write_bytes(b"synthetic executable; never executed")
        proof = self.root / "proof.txt"
        proof.write_text("Synthetic proof for validator unit tests only.", encoding="utf-8")
        self.ref = {"path": "proof.txt", "sha256": gate.digest(proof)}
        self.catalog = [{"id": action, "owner": "cli", "implementation": "implemented",
                         "command_path": action.split("."), "remote_mutation": False}
                        for action in ("auth.status", *MISSING_EIGHT)]
        self.write("catalog.json", self.catalog)
        self.candidate = {"source_revision": "a" * 40, "source_tree_sha256": "b" * 64,
                          "catalog_sha256": gate.digest(self.root / "catalog.json"),
                          "builds": {"linux/amd64": gate.digest(self.build)}}
        self.rows = [self.record(row["id"], index) for index, row in enumerate(self.catalog)]
        self.run = {"schema_version": 1, "scope": "final", "run_id": "synthetic-run",
                    "candidate": copy.deepcopy(self.candidate),
                    "prior_gates": {f"G{i}": "passed" for i in range(9)},
                    "prior_gates_evidence": self.ref,
                    "attempt_ids": [row["attempt_id"] for row in self.rows]}

    def write(self, name, value):
        (self.root / name).write_text(json.dumps(value), encoding="utf-8")

    def record(self, action, index):
        return {"attempt_id": f"attempt-{index}", "case_id": f"case-{index}",
                "action_id": action, "run_id": "synthetic-run", "attempt_number": 1,
                "previous_attempt_id": None, "candidate": copy.deepcopy(self.candidate),
                "platform": "linux/amd64", "binary_sha256": self.candidate["builds"]["linux/amd64"],
                "requested_model": gate.MODEL, "actual_model": gate.MODEL,
                "actual_provider": "openai", "requested_reasoning_effort": "medium",
                "actual_reasoning_effort": "medium", "fixture_id": "synthetic-fixture",
                "classification": "completed", "confidence": "high", "reason": "Synthetic fixture.",
                "harness_agreement": "unavailable", "intentional_refusal": False,
                "telemetry": dict.fromkeys(gate.TELEMETRY),
                "evidence": {key: copy.deepcopy(self.ref) for key in
                             ("task", "transcript", "commands", "model_metadata", "fixture_authority", "assessment")},
                "proof": {"kind": "local_state", "evidence": copy.deepcopy(self.ref)},
                "cleanup": {"status": "not_needed", "evidence": copy.deepcopy(self.ref)}}

    def validate(self, checkpoint=None):
        self.write("candidate.json", self.candidate)
        self.write("run.json", self.run)
        (self.root / "results.jsonl").write_text(
            "\n".join(json.dumps(row) for row in self.rows), encoding="utf-8")
        return gate.validate(self.root / "candidate.json", self.root / "catalog.json",
                             self.root / "run.json", self.root / "results.jsonl", self.root,
                             {"linux/amd64": self.build}, checkpoint_path=checkpoint)

    def checkpoint(self):
        scope = {"schema_version": 1, "checkpoint_id": "synthetic-checkpoint",
                 "candidate": copy.deepcopy(self.candidate),
                 "required_actions": [row["action_id"] for row in self.rows[:2]],
                 "scope_review": self.ref}
        self.write("checkpoint.json", scope)
        self.run.update(scope="checkpoint", checkpoint=copy.deepcopy(scope))
        self.rows = self.rows[:2]
        self.run["attempt_ids"] = [row["attempt_id"] for row in self.rows]
        return self.root / "checkpoint.json", scope

    def test_checkpoint_covers_independently_supplied_scope_not_final_catalog(self):
        path, _ = self.checkpoint()
        result = self.validate(path)
        self.assertEqual(result["scope"], "checkpoint")
        self.assertEqual(result["required_actions"], 2)
        self.assertEqual(result["catalog_executable_actions"], 9)
        with self.assertRaisesRegex(gate.InvalidEvidence, "Require final-sweep"):
            self.validate()

    def test_checkpoint_cannot_be_relabelled_as_final(self):
        path, _ = self.checkpoint()
        self.run["scope"] = "final"
        with self.assertRaisesRegex(gate.InvalidEvidence, "Checkpoint scope mismatch"):
            self.validate(path)
        with self.assertRaisesRegex(gate.InvalidEvidence, "Required actions lack completed"):
            self.validate()

    def test_checkpoint_cannot_reduce_expected_scope_in_run(self):
        path, _ = self.checkpoint()
        self.run["checkpoint"]["required_actions"].pop()
        with self.assertRaisesRegex(gate.InvalidEvidence, "Checkpoint scope mismatch"):
            self.validate(path)

    def test_checkpoint_rejects_unknown_duplicate_or_empty_selection(self):
        path, scope = self.checkpoint()
        for selection in ([], ["unknown.action"], ["auth.status", "auth.status"]):
            with self.subTest(selection=selection):
                scope["required_actions"] = selection
                self.write("checkpoint.json", scope)
                self.run["checkpoint"] = copy.deepcopy(scope)
                with self.assertRaisesRegex(gate.InvalidEvidence, "Invalid checkpoint action scope"):
                    self.validate(path)

    def test_checkpoint_is_bound_to_candidate_and_scope_review(self):
        path, scope = self.checkpoint()
        scope["candidate"]["source_revision"] = "c" * 40
        self.write("checkpoint.json", scope)
        self.run["checkpoint"] = copy.deepcopy(scope)
        with self.assertRaisesRegex(gate.InvalidEvidence, "Checkpoint candidate mismatch"):
            self.validate(path)
        scope["candidate"] = copy.deepcopy(self.candidate)
        scope["scope_review"] = None
        self.write("checkpoint.json", scope)
        self.run["checkpoint"] = copy.deepcopy(scope)
        with self.assertRaisesRegex(gate.InvalidEvidence, "Invalid evidence reference"):
            self.validate(path)

    def test_checkpoint_exclusion_does_not_cover_required_action(self):
        path, _ = self.checkpoint()
        self.rows[0]["classification"] = "excluded"
        with self.assertRaisesRegex(gate.InvalidEvidence, "Required actions lack completed"):
            self.validate(path)

    def rejected(self, message):
        with self.assertRaisesRegex(gate.InvalidEvidence, message):
            self.validate()

    def test_complete_synthetic_sweep_accepts_null_telemetry(self):
        result = self.validate()
        self.assertEqual(result["covered_actions"], 9)
        self.assertEqual(result["attempts_missing_telemetry"], 9)
        self.assertEqual(result["status"], "evidence_accounting_passed")

    def test_missing_eight_existing_harness_gaps_are_rejected(self):
        self.rows = self.rows[:1]
        self.run["attempt_ids"] = [self.rows[0]["attempt_id"]]
        with self.assertRaises(gate.InvalidEvidence) as caught:
            self.validate()
        for action in MISSING_EIGHT:
            self.assertIn(action, str(caught.exception))

    def test_wrong_binary_bytes(self):
        self.build.write_bytes(b"other candidate")
        self.rejected("Binary differs")

    def test_wrong_attempt_build(self):
        self.rows[0]["binary_sha256"] = "c" * 64
        self.rejected("binary/platform mismatch")

    def test_wrong_source_identity(self):
        self.run["candidate"]["source_revision"] = "c" * 40
        self.rejected("Run candidate identity mismatch")

    def test_old_candidate_attempt(self):
        self.rows[0]["candidate"]["source_tree_sha256"] = "c" * 64
        self.rejected("Attempt candidate identity mismatch")

    def test_catalog_tampering(self):
        self.catalog.pop()
        self.write("catalog.json", self.catalog)
        self.rejected("Catalog differs")

    def test_wrong_model_or_effort(self):
        for key, value in (("actual_model", "gpt-5.6-luna"), ("requested_model", "other"),
                           ("actual_reasoning_effort", "high"), ("requested_reasoning_effort", "low"),
                           ("actual_provider", "other")):
            with self.subTest(key=key):
                original = self.rows[0][key]
                self.rows[0][key] = value
                self.rejected("Model/provider/reasoning mismatch")
                self.rows[0][key] = original

    def test_excluded_is_not_coverage(self):
        self.rows[0]["classification"] = "excluded"
        self.rejected("Required actions lack completed evidence")

    def test_cleanup_blocks_completed_outcome(self):
        for status in ("unresolved", "failed"):
            with self.subTest(status=status):
                self.rows[0]["cleanup"]["status"] = status
                self.rejected("Unresolved cleanup")

    def retry(self):
        old = self.rows[0]
        new = copy.deepcopy(old)
        new.update(attempt_id="retry", attempt_number=2, previous_attempt_id=old["attempt_id"])
        self.rows.append(new)
        self.run["attempt_ids"].append("retry")
        return old, new

    def test_retains_excluded_attempt_and_successful_retry(self):
        old, _ = self.retry()
        old["classification"] = "excluded"
        for key in ("actual_model", "actual_provider", "actual_reasoning_effort"):
            old[key] = None
        for key in ("transcript", "commands", "model_metadata"):
            old["evidence"][key] = None
        result = self.validate()
        self.assertEqual(result["attempts"], 10)
        self.assertEqual(result["classifications"]["excluded"], 1)
        self.assertEqual(result["covered_actions"], 9)

    def test_failed_attempt_is_not_erased_by_retry(self):
        old, _ = self.retry()
        old["classification"] = "failed"
        self.rejected("Correctness failures")

    def test_preserves_wrong_configuration_exclusion_and_valid_retry(self):
        old, _ = self.retry()
        old.update(classification="excluded", requested_model="unavailable-model",
                   actual_model="other-model", actual_provider="other-provider",
                   requested_reasoning_effort="high", actual_reasoning_effort="low")
        result = self.validate()
        self.assertEqual(result["classifications"]["excluded"], 1)
        self.assertEqual(result["covered_actions"], 9)
        self.assertEqual(old["requested_model"], "unavailable-model")
        self.assertEqual(old["actual_model"], "other-model")
        old["classification"] = "completed"
        self.rejected("Model/provider/reasoning mismatch")

    def test_remote_mutation_rejects_local_proof_and_accepts_native_proof(self):
        self.catalog[0]["remote_mutation"] = True
        self.write("catalog.json", self.catalog)
        self.candidate["catalog_sha256"] = gate.digest(self.root / "catalog.json")
        self.run["candidate"] = copy.deepcopy(self.candidate)
        for row in self.rows:
            row["candidate"] = copy.deepcopy(self.candidate)
        self.rejected("Remote mutation requires native effect")
        row = self.rows[0]
        row["proof"]["kind"] = "native_effect"
        for key in ("native_acknowledgement", "request_response"):
            row["evidence"][key] = self.ref
        self.assertEqual(self.validate()["covered_actions"], 9)
        row["proof"]["kind"] = "native_refusal"
        row["intentional_refusal"] = True
        self.assertEqual(self.validate()["covered_actions"], 9)

    def test_omitted_attempt_fails_manifest_accounting(self):
        self.retry()
        self.rows.pop(0)
        self.rejected("Missing, reordered, or duplicate attempts")

    def test_discontinuous_retry_chain(self):
        _, new = self.retry()
        new["previous_attempt_id"] = None
        self.rejected("Retry chain")

    def test_outcome_proof_is_required(self):
        self.rows[0].pop("proof")
        self.rejected("Missing requested-outcome proof")

    def test_native_refusal_requires_requested_refusal(self):
        self.rows[0]["proof"]["kind"] = "native_refusal"
        self.rejected("Refusal does not match")
        self.rows[0]["intentional_refusal"] = True
        self.assertEqual(self.validate()["covered_actions"], 9)

    def test_remote_effect_requires_acknowledgement_and_request_evidence(self):
        self.rows[0]["proof"]["kind"] = "native_effect"
        self.rejected("Invalid evidence reference")
        for key in ("native_acknowledgement", "request_response"):
            self.rows[0]["evidence"][key] = self.ref
        self.assertEqual(self.validate()["covered_actions"], 9)

    def test_tampered_and_escaping_evidence(self):
        evidence = self.rows[0]["proof"]["evidence"]
        evidence["sha256"] = "0" * 64
        self.rejected("Evidence hash mismatch")
        evidence.update(path="../outside.txt", sha256=self.ref["sha256"])
        self.rejected("Evidence path escapes")

    def test_workaround_requires_explicit_review(self):
        self.rows[0]["classification"] = "completed_with_workaround"
        self.rejected("Workaround friction")
        self.rows[0]["workaround_review"] = "accepted"
        self.rows[0]["evidence"]["workaround_review"] = self.ref
        self.assertEqual(self.validate()["classifications"]["completed_with_workaround"], 1)

    def test_missing_telemetry_fields_do_not_become_inferred_values(self):
        self.rows[0]["telemetry"].pop("model_turns")
        self.rejected("Telemetry requires explicit")

    def test_prior_gate_failure_blocks_g9(self):
        self.run["prior_gates"]["G7"] = "excluded"
        self.rejected("G0-G8 must pass")


if __name__ == "__main__":
    unittest.main()
