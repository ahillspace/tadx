"""Synthetic checks only; no provider, model, or native CLI is launched."""

import importlib.util
import hashlib
import json
from pathlib import Path
import unittest


MODULE = Path(__file__).resolve().parents[1] / "source/integration/job_contract.py"
SPEC = importlib.util.spec_from_file_location("job_contract", MODULE)
job_contract = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(job_contract)
JOB = "11111111-1111-4111-8111-111111111111"
DATA = "33333333-3333-4333-8333-333333333333"
HASH = "a" * 64


def fixture(action):
    return {"run_id": "run-1", "case_id": "P-" + action.replace(".", "-"),
            "action": action, "job_id": JOB, "environment": "fixture",
            "site": "fixture-site", "site_luid": "22222222-2222-4222-8222-222222222222",
            "api_version": "3.29", "datasource_luid": DATA,
            "ownership_sha256": HASH, "before_sha256": HASH,
            "protected_sha256": HASH, "binary_sha256": HASH}


def observation(before="succeeded", after="succeeded"):
    return {"independent": True, "run_id": "run-1", "case_id": "P-job-inspect",
            "job_id": JOB, "site_luid": "22222222-2222-4222-8222-222222222222",
            "before": {"id": JOB, "status": before},
            "after": {"id": JOB, "status": after},
            "ownership_sha256": HASH, "before_sha256": HASH, "after_sha256": HASH,
            "protected_sha256": HASH}


def command(action, status="succeeded", **output):
    payload = {"status": status, "environment": "fixture", "site": "fixture-site",
               "job": {"id": JOB, "status": status}, **output}
    raw = json.dumps(payload)
    return {"id": "cmd-1", "phase": "task", "executed": True,
            "capability": action, "flags": {"environment": "fixture", "site": "fixture-site",
                                          "id": JOB, "json": True, "full": True},
            "exit_status": 0, "stdout": raw,
            "native_ack_sha256": hashlib.sha256(raw.encode()).hexdigest(),
            "binary_sha256": HASH}


class JobContractTests(unittest.TestCase):
    def test_exact_inspect_matches_independent_stable_job(self):
        result = job_contract.grade_case(fixture("job.inspect"),
                                         [command("job.inspect")], observation())
        self.assertTrue(result["passed"], result["issues"])

    def test_wait_requires_independent_terminal_result_without_resubmit(self):
        case = fixture("job.wait")
        seen = observation("running", "succeeded")
        seen["case_id"] = case["case_id"]
        result = job_contract.grade_case(case, [command("job.wait")], seen)
        self.assertTrue(result["passed"], result["issues"])
        failed = observation("running", "failed")
        failed["case_id"] = case["case_id"]
        result = job_contract.grade_case(case, [command("job.wait", "failed")], failed)
        self.assertTrue(result["passed"], result["issues"])
        seen["after"]["status"] = "running"
        self.assertFalse(job_contract.grade_case(case, [command("job.wait")], seen)["passed"])
        seen = observation("succeeded", "succeeded")
        seen["case_id"] = case["case_id"]
        self.assertFalse(job_contract.grade_case(case, [command("job.wait")], seen)["passed"])
        unknown = observation("running", "unknown")
        unknown["case_id"] = case["case_id"]
        self.assertFalse(job_contract.grade_case(case, [command("job.wait", "unknown")], unknown)["passed"])
        interrupted = command("job.wait")
        interrupted["exit_status"] = 1
        completed = observation("running", "succeeded")
        completed["case_id"] = case["case_id"]
        self.assertFalse(job_contract.grade_case(case, [interrupted], completed)["passed"])

    def test_cancel_needs_real_command_ack_and_independent_cancellation(self):
        case = fixture("job.cancel")
        seen = observation("running", "cancelled")
        seen["case_id"] = case["case_id"]
        issued = command("job.cancel", "cancelled", confirmed=True, request_id="request-1")
        self.assertTrue(job_contract.grade_case(case, [issued], seen)["passed"])
        preview = dict(issued, flags={**issued["flags"], "preview": True})
        self.assertFalse(job_contract.grade_case(case, [preview], seen)["passed"])
        self.assertFalse(job_contract.grade_case(case, [issued, issued], seen)["passed"])
        self.assertFalse(job_contract.grade_case(case, [dict(issued, native_ack_sha256="")], seen)["passed"])
        self.assertFalse(job_contract.grade_case(case, [dict(issued, native_ack_sha256=HASH)], seen)["passed"])
        seen["after"]["status"] = "running"
        self.assertFalse(job_contract.grade_case(case, [issued], seen)["passed"])

    def test_wrong_job_or_site_and_unowned_mutation_never_pass(self):
        case = fixture("job.inspect")
        seen = observation()
        seen["job_id"] = "33333333-3333-4333-8333-333333333333"
        self.assertFalse(job_contract.grade_case(case, [command("job.inspect")], seen)["passed"])
        seen = observation()
        other = dict(command("job.inspect"), capability="project.delete")
        self.assertFalse(job_contract.grade_case(case, [command("job.inspect"), other], seen)["passed"])

    def test_ownership_proof_must_match_independent_observation(self):
        case = fixture("job.inspect")
        seen = observation()
        seen["ownership_sha256"] = "b" * 64
        self.assertFalse(job_contract.grade_case(case, [command("job.inspect")], seen)["passed"])

    def test_native_output_must_identify_selected_environment_and_site(self):
        case = fixture("job.inspect")
        seen = observation()
        for wrong in ({"environment": "other"}, {"site": "other"}):
            self.assertFalse(job_contract.grade_case(case, [command("job.inspect", **wrong)], seen)["passed"])

    def test_cleanup_requires_exact_owned_scope_and_independent_absence(self):
        case = fixture("job.cancel")
        response = "<tsResponse><error code='404002'/></tsResponse>"
        absence = {"method": "GET", "path": f"/api/3.29/sites/{case['site_luid']}/datasources/{DATA}",
                   "http_status": 404, "response_body": response,
                   "response_sha256": hashlib.sha256(response.encode()).hexdigest()}
        record = {"independent": True, "run_id": case["run_id"], "case_id": case["case_id"],
                  "job_id": JOB, "site_luid": case["site_luid"], "datasource_luid": DATA,
                  "datasource_absence": absence, "job_terminal": True,
                  "job_status": "cancelled", "ownership_sha256": HASH,
                  "owned_assets_absent": True, "protected_sha256": HASH,
                  "evidence_sha256": HASH}
        self.assertEqual(job_contract.cleanup_case(case, record)["status"], "completed")
        self.assertEqual(job_contract.cleanup_case(case, {**record, "owned_assets_absent": False})["status"], "unresolved")
        self.assertEqual(job_contract.cleanup_case(case, {**record, "site_luid": JOB})["status"], "unresolved")
        self.assertEqual(job_contract.cleanup_case(case, {**record, "ownership_sha256": "b" * 64})["status"], "unresolved")
        self.assertEqual(job_contract.cleanup_case(case, {**record, "job_status": "succeeded"})["status"], "unresolved")
        self.assertEqual(job_contract.cleanup_case(case, {**record, "datasource_absence":
                                                   {**absence, "response_sha256": HASH}})["status"], "unresolved")


if __name__ == "__main__":
    unittest.main()
