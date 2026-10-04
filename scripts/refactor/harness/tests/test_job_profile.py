"""Synthetic exact-job profile admission; no credentials or network are used."""

import importlib.util
from pathlib import Path
import sys
import types
import unittest

from test_job_fixture import JOB, SITE, records


SOURCE = Path(__file__).resolve().parents[1] / "source/integration"
integration = types.ModuleType("integration")
integration.__path__ = [str(SOURCE)]
sys.modules["integration"] = integration
for alias, source in (("g9_job_contract", "job_contract"),
                      ("g9_job_fixture", "job_fixture")):
    spec = importlib.util.spec_from_file_location("integration." + alias, SOURCE / (source + ".py"))
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
remote = types.ModuleType("integration.remote_read_profiles")
remote.ENV_NAMES = {"pat_name": "SYNTHETIC_AGENT_NAME", "pat_secret": "SYNTHETIC_AGENT_SECRET"}
sys.modules[remote.__name__] = remote
integration.remote_read_profiles = remote
spec = importlib.util.spec_from_file_location("integration.g9_job_profile", SOURCE / "job_profile.py")
profile = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = profile
spec.loader.exec_module(profile)


def binding():
    result = {"environment": "fixture", "server_url": "https://example.test",
              "site_content_url": "fixture-site", "api_version": "3.29", "site_luid": SITE}
    for role in ("agent", "observer", "provisioner"):
        result[role] = {"pat_name": "synthetic-" + role,
                        "pat_secret": "synthetic-secret-" + role}
    return result


class JobProfileTests(unittest.TestCase):
    def test_retained_observer_response_must_match_one_raw_http_read(self):
        path = "/api/3.29/sites/owned/jobs/exact"
        raw = b"<tsResponse><job id='exact'/></tsResponse>"
        event = {"method": "GET", "path": path, "status": 200,
                 "response": raw.decode()}
        self.assertTrue(profile.retained_job_response([event], path, raw))
        self.assertFalse(profile.retained_job_response([], path, raw))
        self.assertFalse(profile.retained_job_response([event, event], path, raw))
        self.assertFalse(profile.retained_job_response([{**event, "response": "changed"}], path, raw))
        self.assertFalse(profile.retained_job_response([{**event, "status": 500}], path, raw))

    def test_three_cases_have_exact_public_binding_and_guard(self):
        for action, before in (("job.inspect", "succeeded"), ("job.wait", "running"),
                               ("job.cancel", "pending")):
            with self.subTest(action=action):
                owner, accepted, observed = records(action, before)
                case_id = owner["case_id"]
                req = {"run_id": "run-1", "case_id": case_id,
                       "exercise": {"id": case_id}, "job_owner": owner}
                verified = profile.preflight_record(req, binding(),
                                                     {"owner": owner, "accepted": accepted,
                                                      "observed": observed})
                prepared = profile.prepared_record(req, binding(), verified)
                self.assertEqual(prepared["fixture"]["public"],
                                 {"site": "fixture-site", "job_id": JOB})
                self.assertEqual(prepared["broker_guard"]["action"], action)
                self.assertEqual(prepared["broker_guard"]["job_id"], JOB)
                self.assertNotIn("synthetic-secret", str(prepared))
                self.assertEqual(prepared["mutation_policy"] == "enabled", action == "job.cancel")
                self.assertEqual("job_status" in prepared["fixture"]["expect"],
                                 action != "job.wait")

    def test_wrong_site_and_shared_credential_roles_block_before_model(self):
        owner, accepted, observed = records()
        req = {"run_id": "run-1", "case_id": owner["case_id"],
               "exercise": {"id": owner["case_id"]}}
        record = {"owner": owner, "accepted": accepted, "observed": observed}
        wrong = binding()
        wrong["site_luid"] = JOB
        with self.assertRaises(profile.fixture_contract.FixtureBlocked):
            profile.preflight_record(req, wrong, record)
        shared = binding()
        shared["observer"] = shared["agent"]
        with self.assertRaises(profile.fixture_contract.FixtureBlocked):
            profile.preflight_record(req, shared, record)
        with self.assertRaises(profile.fixture_contract.FixtureBlocked):
            profile.preflight_record(req, binding(), {**record, "extra": True})


if __name__ == "__main__":
    unittest.main()
