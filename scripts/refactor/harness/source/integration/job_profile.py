"""Blocked-suite profile for three independently owned exact-job cases.

The setup and cleanup artifacts come from an authorized, separately qualified
fixture worker. Missing artifacts block dispatch or cleanup; this module never
submits a replacement job or adopts a job by name.
"""

import hashlib
import json
from pathlib import Path
import urllib.parse

from integration import g9_job_contract as contract, g9_job_fixture as fixture_contract


SUPPORTED = {"P-job-inspect", "P-job-wait", "P-job-cancel"}
BASELINE = "g9-job-baseline.json"
SETUP = "g9-job-setup.json"
PROTECTED_AFTER = "g9-job-protected-after.json"
CLEANUP = "g9-job-cleanup.json"


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def retained_job_response(events, path, raw):
    """Require one sanitized HTTP event with unchanged exact job response bytes."""
    matching = [event for event in events if isinstance(event, dict)
                and event.get("method") == "GET" and event.get("path") == path
                and event.get("status") == 200]
    return (len(matching) == 1 and isinstance(matching[0].get("response"), str)
            and hashlib.sha256(matching[0]["response"].encode()).digest()
            == hashlib.sha256(raw).digest())


def preflight_record(req, binding, record):
    """Block before model allocation when setup or role identity is uncertain."""
    if not isinstance(req, dict) or not isinstance(binding, dict) or not isinstance(record, dict):
        raise fixture_contract.FixtureBlocked("Exact-job preflight input is missing")
    case_id = req.get("case_id")
    if case_id not in SUPPORTED or req.get("exercise", {}).get("id") != case_id:
        raise fixture_contract.FixtureBlocked("Exact-job case is not declared")
    if set(record) != {"owner", "accepted", "observed"}:
        raise fixture_contract.FixtureBlocked("Exact-job setup record has missing or extra sections")
    verified = fixture_contract.verify_setup(record["owner"], record["accepted"], record["observed"])
    owner = record["owner"]
    if any((owner.get("run_id") != req.get("run_id"), owner.get("case_id") != case_id,
            owner.get("site_luid") != binding.get("site_luid"),
            owner.get("environment") != binding.get("environment"),
            owner.get("site") != binding.get("site_content_url"),
            owner.get("api_version") != binding.get("api_version"))):
        raise fixture_contract.FixtureBlocked("Exact-job setup differs from approved binding")
    roles = [binding.get(role) for role in ("agent", "observer", "provisioner")]
    for key in ("pat_name", "pat_secret"):
        values = [role.get(key) if isinstance(role, dict) else None for role in roles]
        if not all(isinstance(value, str) and value for value in values) or len(set(values)) != 3:
            raise fixture_contract.FixtureBlocked("Exact-job credential roles are not distinct")
    return verified


def prepared_record(req, binding, verified):
    """Build only public task inputs and a strict broker guard from preflight."""
    from integration import remote_read_profiles as remote

    case, guard = verified["case"], verified["guard"]
    public = {"site": case["site"], "job_id": case["job_id"]}
    expected_status = verified["before_status"]
    if case["action"] == "job.cancel":
        expected_status = "cancelled"
    fixture = {"handle": req["case_id"], "semantic_digest": digest({"guard": guard}),
               "public": public, "sites": {"A": {"id": case["site_luid"],
                                                  "environment": case["environment"]}},
               "roles": {"job": {"id": case["job_id"]},
                         "owned_datasource": {"id": req["job_owner"]["datasource_luid"]},
                         "sentinel": {"path": "sentinel.txt"}},
               "expect": {"job_status": expected_status} if case["action"] != "job.wait" else {},
               "expected_bindings_provenance": {"/fixture/expect/job_status": {
                   "phase": "setup", "source_kind": "independent_baseline", "evidence": [SETUP]}}
                   if case["action"] != "job.wait" else {},
               "coverage_assertions": []}
    seed = {"version": 1, "default_environment": binding["environment"],
            "environments": {binding["environment"]: {
                "url": binding["server_url"], "site_content_url": binding["site_content_url"],
                "api_version": binding["api_version"],
                "auth": {"type": "pat", "pat_name_env": remote.ENV_NAMES["pat_name"],
                         "pat_secret_env": remote.ENV_NAMES["pat_secret"]}}}}
    before = {"job": {"id": case["job_id"], "status": verified["before_status"]},
              "protected_sha256": case["protected_sha256"]}
    return {"fixture": fixture, "before": before, "broker_guard": guard,
            "config_seed": seed, "credential_env_names": remote.ENV_NAMES,
            "mutation_policy": "enabled" if case["action"] == "job.cancel" else None,
            "harness_report_required": False,
            "baseline_evidence": [SETUP, BASELINE],
            "baseline_assertions": [{"id": "independent-exact-job-setup", "status": "pass",
                                      "evidence": [SETUP]}]}


def load_binding(req):
    from integration.operator_binding import load_operator_binding
    from bench.core import Blocked

    binding = load_operator_binding(req)
    if not binding:
        raise Blocked("Exact-job role binding is unavailable")
    return binding


def prepare(req, binding, work=None):
    from bench.core import Blocked, load, save

    root = Path(req["private_case_dir"])
    path = root / SETUP
    if not path.is_file():
        raise Blocked("Independently provisioned exact-job setup is absent")
    record = load(path)
    try:
        verified = preflight_record(req, binding, record)
    except fixture_contract.FixtureBlocked as exc:
        raise Blocked(str(exc)) from exc
    with_owner = {**req, "job_owner": record["owner"]}
    prepared = prepared_record(with_owner, binding, verified)
    save(root / BASELINE, {"case": verified["case"], "guard": verified["guard"],
                           "before": prepared["before"],
                           "protected_snapshot": verified["protected_snapshot"]})
    return prepared


def observe(req, binding, work=None, commands=None):
    from bench.core import Blocked, load, save
    from bench.tableau import Site, Tableau

    root = Path(req["private_case_dir"])
    baseline = load(root / BASELINE)
    case, before = baseline["case"], baseline["before"]
    protected_path = root / PROTECTED_AFTER
    if not protected_path.is_file():
        raise Blocked("Independent protected-state observation is absent")
    protected = load(protected_path)
    if (not isinstance(protected, dict) or protected.get("independent") is not True
            or any(protected.get(key) != case[key] for key in ("run_id", "case_id", "site_luid"))
            or protected.get("protected_sha256") != case["protected_sha256"]
            or protected.get("snapshot") != baseline.get("protected_snapshot")
            or fixture_contract.digest(protected.get("snapshot")) != case["protected_sha256"]
            or not contract._hash(protected.get("evidence_sha256"))):
        raise Blocked("Protected-state observation differs from exact fixture")
    events = []
    reader = Tableau(Site(binding["server_url"], binding["site_content_url"],
                          binding["api_version"], binding["site_luid"]),
                     evidence=events.append, **binding["observer"])
    try:
        reader.signin()
        path = reader.path("jobs/" + urllib.parse.quote(case["job_id"], safe=""))
        status_code, raw, _ = reader._request("GET", path)
        if status_code != 200:
            raise Blocked("Independent exact-job read did not return HTTP 200")
        xml = raw.decode("utf-8")
        job = fixture_contract.verified_job_xml(xml, hashlib.sha256(raw).hexdigest())
        if job.get("id") != case["job_id"] or job.get("type") != "RefreshExtract":
            raise Blocked("Independent exact-job read changed identity or type")
        status = fixture_contract.job_status(job)
    finally:
        try:
            reader.signout()
        finally:
            save(root / "g9-job-after-http.json", events)
    if not retained_job_response(events, path, raw):
        raise Blocked("Retained independent job response differs from exact raw observation")
    observed = {"independent": True, "run_id": case["run_id"], "case_id": case["case_id"],
                "job_id": case["job_id"], "site_luid": case["site_luid"],
                "before": before["job"], "after": {"id": case["job_id"], "status": status},
                "ownership_sha256": case["ownership_sha256"],
                "before_sha256": case["before_sha256"],
                "after_sha256": hashlib.sha256(raw).hexdigest(),
                "protected_sha256": protected["protected_sha256"]}
    audit = []
    for row in commands or []:
        item = dict(row)
        if item.get("capability") == "job.cancel" and isinstance(item.get("stdout"), str):
            item["native_ack_sha256"] = hashlib.sha256(item["stdout"].encode()).hexdigest()
        audit.append(item)
    grade = contract.grade_case(case, audit, observed)
    return {"after": {"job_contract_passed": grade["passed"],
                      "job_status": status, "protected_sha256": protected["protected_sha256"],
                      "unowned_changes": []},
            "evidence": [PROTECTED_AFTER, "g9-job-after-http.json", "audit/commands.jsonl"],
            "grade": grade}


def cleanup(req, binding, work=None):
    from bench.core import load

    root = Path(req["private_case_dir"])
    path = root / CLEANUP
    if not path.is_file() or not (root / BASELINE).is_file():
        return {"cleanup_status": "blocked", "reason": "Exact owned cleanup evidence is absent"}
    case = load(root / BASELINE)["case"]
    result = contract.cleanup_case(case, load(path))
    return {"cleanup_status": "pass" if result["status"] == "completed" else "blocked",
            "evidence": [CLEANUP]}
