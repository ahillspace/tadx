"""Synthetic exact-job setup checks; no remote call or credential is used."""

import importlib.util
import hashlib
from pathlib import Path
import unittest


MODULE = Path(__file__).resolve().parents[1] / "source/integration/job_fixture.py"
SPEC = importlib.util.spec_from_file_location("job_fixture", MODULE)
job_fixture = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(job_fixture)

JOB = "11111111-1111-4111-8111-111111111111"
SITE = "22222222-2222-4222-8222-222222222222"
DATA = "33333333-3333-4333-8333-333333333333"
HASH = "a" * 64


def records(action="job.cancel", state="running"):
    response = f'<tsResponse><job id="{JOB}" type="RefreshExtract"/></tsResponse>'
    progress, finish = {"pending": (0, 1), "running": (50, 0),
                        "succeeded": (100, 0), "failed": (100, 1),
                        "cancelled": (100, 2)}[state]
    observed_xml = (f'<tsResponse><job id="{JOB}" type="RefreshExtract" '
                    f'progress="{progress}" finishCode="{finish}"/></tsResponse>')
    protected = {"site_luid": SITE, "unowned": [{"id": "other", "sha256": HASH}],
                 "sentinel_sha256": HASH, "owned_datasource_sha256": HASH}
    owner = {"run_id": "run-1", "case_id": "P-" + action.replace(".", "-"),
             "action": action, "environment": "fixture", "site": "fixture-site",
             "site_luid": SITE, "api_version": "3.29", "datasource_luid": DATA, "datasource_owned": True,
             "datasource_sha256": HASH, "protected_sha256": job_fixture.digest(protected),
             "protected_snapshot": protected, "binary_sha256": HASH,
             "role_sha256": {"agent": "1" * 64, "observer": "2" * 64, "provisioner": "3" * 64}}
    accepted = {"method": "POST", "path": f"/api/3.29/sites/{SITE}/datasources/{DATA}/refresh",
                "http_status": 202, "attempt_count": 1, "job_id": JOB,
                "job_type": "RefreshExtract", "response_xml": response,
                "response_sha256": hashlib.sha256(response.encode()).hexdigest(),
                "accepted_at": "2026-10-03T12:00:00Z"}
    observed = {"role": "observer", "site_luid": SITE, "job_id": JOB,
                "job_type": "RefreshExtract", "status": state,
                "datasource_luid": DATA, "response_xml": observed_xml,
                "evidence_sha256": hashlib.sha256(observed_xml.encode()).hexdigest(),
                "checked_at": "2026-10-03T12:00:05Z",
                "protected_sha256": owner["protected_sha256"]}
    return owner, accepted, observed


class JobFixtureTests(unittest.TestCase):
    def test_owned_fresh_cancel_fixture_binds_exact_ack_and_independent_read(self):
        owner, accepted, observed = records()
        result = job_fixture.verify_setup(owner, accepted, observed)
        self.assertEqual(result["guard"]["family"], "job")
        self.assertEqual(result["guard"]["job_id"], JOB)
        self.assertEqual(result["case"]["ownership_sha256"], result["guard"]["ownership_sha256"])

    def test_wrong_or_unknown_submission_never_adopts_job(self):
        owner, accepted, observed = records()
        for delta in ({"http_status": 200}, {"attempt_count": 2}, {"job_id": ""},
                      {"path": f"/api/3.29/sites/{SITE}/jobs/{JOB}"}):
            with self.subTest(delta=delta), self.assertRaises(job_fixture.FixtureBlocked):
                job_fixture.verify_setup(owner, {**accepted, **delta}, observed)

    def test_observer_mismatch_or_shared_role_fails(self):
        owner, accepted, observed = records()
        for delta in ({"job_id": DATA}, {"site_luid": DATA}, {"datasource_luid": JOB},
                      {"role": "provisioner"}, {"protected_sha256": "b" * 64}):
            with self.subTest(delta=delta), self.assertRaises(job_fixture.FixtureBlocked):
                job_fixture.verify_setup(owner, accepted, {**observed, **delta})
        owner["role_sha256"]["observer"] = owner["role_sha256"]["provisioner"]
        with self.assertRaises(job_fixture.FixtureBlocked):
            job_fixture.verify_setup(owner, accepted, observed)

    def test_claimed_protected_hash_without_matching_raw_snapshot_fails(self):
        owner, accepted, observed = records()
        owner["protected_snapshot"]["unowned"] = []
        with self.assertRaises(job_fixture.FixtureBlocked):
            job_fixture.verify_setup(owner, accepted, observed)

    def test_unverified_raw_ack_and_observation_cannot_be_invented(self):
        owner, accepted, observed = records()
        for delta in ({"response_xml": ""}, {"response_sha256": HASH},
                      {"response_xml": accepted["response_xml"].replace(JOB, DATA)}):
            with self.subTest(delta=delta), self.assertRaises(job_fixture.FixtureBlocked):
                job_fixture.verify_setup(owner, {**accepted, **delta}, observed)
        for delta in ({"response_xml": ""}, {"evidence_sha256": HASH},
                      {"response_xml": observed["response_xml"].replace('progress="50"', 'progress="100"')}):
            with self.subTest(delta=delta), self.assertRaises(job_fixture.FixtureBlocked):
                job_fixture.verify_setup(owner, accepted, {**observed, **delta})

    def test_active_to_terminal_case_preconditions_and_freshness(self):
        for action, state in (("job.inspect", "succeeded"), ("job.wait", "running"),
                              ("job.cancel", "pending")):
            owner, accepted, observed = records(action, state)
            self.assertEqual(job_fixture.verify_setup(owner, accepted, observed)["case"]["action"], action)
        owner, accepted, observed = records("job.cancel", "succeeded")
        with self.assertRaises(job_fixture.FixtureBlocked):
            job_fixture.verify_setup(owner, accepted, observed)
        owner, accepted, observed = records("job.cancel", "running")
        observed["checked_at"] = "2026-10-03T12:10:00Z"
        with self.assertRaises(job_fixture.FixtureBlocked):
            job_fixture.verify_setup(owner, accepted, observed)


if __name__ == "__main__":
    unittest.main()
