"""Validate a journaled, independently observed exact-job setup record.

This pure module does not submit a refresh or contact Tableau. A trusted
provisioner must record the accepted POST before supplying these records.
"""

from datetime import datetime, timedelta, timezone
import hashlib
import json
import re
import xml.etree.ElementTree as ET


ACTIONS = {"job.inspect", "job.wait", "job.cancel"}
ACTIVE = {"pending", "running"}
TERMINAL = {"succeeded", "failed", "cancelled"}
UUID = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\Z", re.I)
HASH = re.compile(r"[0-9a-f]{64}\Z")
VERSION = re.compile(r"[1-9][0-9]*\.[0-9]+\Z")


class FixtureBlocked(ValueError):
    """The accepted job cannot be safely attributed to this exact fixture."""


def require(condition, reason):
    if not condition:
        raise FixtureBlocked(reason)


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def valid_hash(value):
    return isinstance(value, str) and bool(HASH.fullmatch(value))


def valid_id(value):
    return isinstance(value, str) and bool(UUID.fullmatch(value))


def timestamp(value):
    require(isinstance(value, str) and value.endswith("Z"), "Fixture timestamp is not UTC")
    try:
        parsed = datetime.fromisoformat(value[:-1] + "+00:00")
    except ValueError as exc:
        raise FixtureBlocked("Fixture timestamp is malformed") from exc
    require(parsed.tzinfo == timezone.utc, "Fixture timestamp is not UTC")
    return parsed


def verified_job_xml(raw, expected_sha256):
    require(isinstance(raw, str) and 0 < len(raw) <= 65536 and
            "<!DOCTYPE" not in raw.upper() and "<!ENTITY" not in raw.upper(),
            "Raw job response is absent or unsafe")
    require(hashlib.sha256(raw.encode()).hexdigest() == expected_sha256,
            "Raw job response differs from its evidence hash")
    try:
        root = ET.fromstring(raw)
    except ET.ParseError as exc:
        raise FixtureBlocked("Raw job response is malformed") from exc
    jobs = [node for node in root.iter() if node.tag.rsplit("}", 1)[-1] == "job"]
    require(len(jobs) == 1, "Raw job response lacks one exact job")
    return jobs[0]


def job_status(job):
    try:
        progress, finish = int(job.get("progress")), int(job.get("finishCode"))
    except (TypeError, ValueError) as exc:
        raise FixtureBlocked("Raw job status is incomplete") from exc
    require(0 <= progress <= 100, "Raw job progress is invalid")
    if progress < 100 and not job.get("completedAt"):
        return "pending" if progress == 0 else "running"
    terminal = {0: "succeeded", 1: "failed", 2: "cancelled"}
    require(finish in terminal, "Raw job terminal state is unsupported")
    return terminal[finish]


def verify_setup(owner, accepted, observed):
    """Bind one accepted refresh to owned content and a distinct observer."""
    require(all(isinstance(row, dict) for row in (owner, accepted, observed)),
            "Exact-job setup records are missing")
    action = owner.get("action")
    require(action in ACTIONS and owner.get("case_id") == "P-" + action.replace(".", "-"),
            "Unsupported exact-job case")
    for key in ("run_id", "environment", "site"):
        require(isinstance(owner.get(key), str) and owner[key].strip() == owner[key] and owner[key],
                "Missing exact-job " + key)
    for key in ("site_luid", "datasource_luid"):
        require(valid_id(owner.get(key)), "Invalid exact-job " + key)
    require(isinstance(owner.get("api_version"), str) and VERSION.fullmatch(owner["api_version"]),
            "Invalid exact API version")
    require(owner.get("datasource_owned") is True, "Refresh target is not journal-owned")
    for key in ("datasource_sha256", "protected_sha256", "binary_sha256"):
        require(valid_hash(owner.get(key)), "Missing hashed exact-job " + key)
    protected = owner.get("protected_snapshot")
    require(isinstance(protected, dict) and set(protected) ==
            {"site_luid", "unowned", "sentinel_sha256", "owned_datasource_sha256"}
            and protected["site_luid"] == owner["site_luid"]
            and isinstance(protected["unowned"], list)
            and valid_hash(protected["sentinel_sha256"])
            and protected["owned_datasource_sha256"] == owner["datasource_sha256"]
            and digest(protected) == owner["protected_sha256"],
            "Protected baseline is not bound to raw scoped state")
    roles = owner.get("role_sha256")
    require(isinstance(roles, dict) and set(roles) == {"agent", "observer", "provisioner"}
            and all(valid_hash(value) for value in roles.values())
            and len(set(roles.values())) == 3, "Independent credential roles are not distinguished")

    expected_path = (f"/api/{owner['api_version']}/sites/{owner['site_luid']}/"
                     f"datasources/{owner['datasource_luid']}/refresh")
    require(accepted.get("method") == "POST" and accepted.get("path") == expected_path
            and accepted.get("http_status") == 202 and accepted.get("attempt_count") == 1,
            "One exact accepted refresh POST is not established")
    require(valid_id(accepted.get("job_id")) and accepted.get("job_type") == "RefreshExtract"
            and valid_hash(accepted.get("response_sha256")),
            "Accepted refresh lacks one supported exact job")
    ack_job = verified_job_xml(accepted.get("response_xml"), accepted["response_sha256"])
    require(ack_job.get("id") == accepted["job_id"] and ack_job.get("type") == "RefreshExtract",
            "Raw acceptance response differs from exact job identity")
    accepted_at = timestamp(accepted.get("accepted_at"))

    require(observed.get("role") == "observer" and observed.get("site_luid") == owner["site_luid"]
            and observed.get("datasource_luid") == owner["datasource_luid"]
            and observed.get("job_id") == accepted["job_id"]
            and observed.get("job_type") == "RefreshExtract",
            "Independent observer did not confirm the exact accepted job")
    require(valid_hash(observed.get("evidence_sha256"))
            and observed.get("protected_sha256") == owner["protected_sha256"],
            "Independent job or protected-state evidence is missing")
    raw_job = verified_job_xml(observed.get("response_xml"), observed["evidence_sha256"])
    require(raw_job.get("id") == observed["job_id"] and
            raw_job.get("type") == "RefreshExtract" and
            job_status(raw_job) == observed.get("status"),
            "Independent raw job response differs from recorded state")
    checked_at = timestamp(observed.get("checked_at"))
    require(accepted_at <= checked_at <= accepted_at + timedelta(minutes=10),
            "Independent observation is outside the bounded setup window")
    status = observed.get("status")
    if action == "job.inspect":
        require(status in TERMINAL, "Inspect fixture has no terminal job")
    else:
        require(status in ACTIVE, "Active-to-terminal fixture is no longer active")
        require(checked_at <= accepted_at + timedelta(seconds=30),
                "Active job observation is stale")

    ownership = digest({"run_id": owner["run_id"], "case_id": owner["case_id"],
                        "site_luid": owner["site_luid"], "datasource_luid": owner["datasource_luid"],
                        "job_id": accepted["job_id"], "accepted_sha256": accepted["response_sha256"],
                        "observer_sha256": observed["evidence_sha256"]})
    case = {"run_id": owner["run_id"], "case_id": owner["case_id"], "action": action,
            "environment": owner["environment"], "site": owner["site"],
            "site_luid": owner["site_luid"], "api_version": owner["api_version"],
            "datasource_luid": owner["datasource_luid"], "job_id": accepted["job_id"],
            "ownership_sha256": ownership, "before_sha256": observed["evidence_sha256"],
            "protected_sha256": owner["protected_sha256"],
            "binary_sha256": owner["binary_sha256"]}
    guard = {"family": "job", "action": action, "run_id": owner["run_id"],
             "case_id": owner["case_id"], "environment": owner["environment"],
             "site": owner["site"], "site_luid": owner["site_luid"],
             "job_id": accepted["job_id"], "ownership_sha256": ownership,
             "baseline_sha256": observed["evidence_sha256"],
             "before_status": status, "preflight_verified": True}
    return {"case": case, "guard": guard, "before_status": status,
            "protected_snapshot": protected,
            "accepted_response_sha256": accepted["response_sha256"]}
