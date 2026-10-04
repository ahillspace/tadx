"""Pure, fail-closed grading for three exact-job fixtures.

The caller must independently provision and observe the job. This module never
creates a job, reads credentials, invokes TADX, or contacts Tableau.
"""

import json
import hashlib
import re


ACTIONS = {"job.inspect", "job.wait", "job.cancel"}
TERMINAL = {"succeeded", "failed", "cancelled"}
ACTIVE = {"pending", "running"}
UUID = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\Z", re.I)
HASH = re.compile(r"[0-9a-f]{64}\Z")


def _hash(value):
    return isinstance(value, str) and bool(HASH.fullmatch(value))


def _fixture_issues(case):
    if not isinstance(case, dict) or case.get("action") not in ACTIONS:
        return ["Unknown exact-job case"]
    issues = []
    if case.get("case_id") != "P-" + case["action"].replace(".", "-"):
        issues.append("Case identity differs from its action")
    if not isinstance(case.get("job_id"), str) or not UUID.fullmatch(case["job_id"]):
        issues.append("Fixture lacks one exact job LUID")
    if not isinstance(case.get("site_luid"), str) or not UUID.fullmatch(case["site_luid"]):
        issues.append("Fixture lacks one exact site LUID")
    if not isinstance(case.get("datasource_luid"), str) or not UUID.fullmatch(case["datasource_luid"]):
        issues.append("Fixture lacks one owned datasource LUID")
    if not isinstance(case.get("api_version"), str) or not re.fullmatch(r"[1-9][0-9]*\.[0-9]+", case["api_version"]):
        issues.append("Fixture lacks an exact API version")
    for key in ("run_id", "environment", "site"):
        if not isinstance(case.get(key), str) or not case[key].strip():
            issues.append("Fixture lacks " + key)
    for key in ("ownership_sha256", "before_sha256", "protected_sha256", "binary_sha256"):
        if not _hash(case.get(key)):
            issues.append("Fixture lacks hashed " + key)
    return issues


def _observation_issues(case, observed):
    if not isinstance(observed, dict) or observed.get("independent") is not True:
        return ["Independent job observation is absent"]
    issues = []
    for key in ("run_id", "case_id", "job_id", "site_luid"):
        if observed.get(key) != case.get(key):
            issues.append("Observer " + key + " differs from fixture")
    before, after = observed.get("before"), observed.get("after")
    for label, value in (("before", before), ("after", after)):
        if not isinstance(value, dict) or value.get("id") != case.get("job_id") or not value.get("status"):
            issues.append("Independent " + label + " state lacks the exact job")
    if observed.get("before_sha256") != case.get("before_sha256"):
        issues.append("Prelaunch observation differs from frozen baseline")
    if observed.get("ownership_sha256") != case.get("ownership_sha256"):
        issues.append("Independent ownership proof differs from frozen fixture")
    if not _hash(observed.get("after_sha256")):
        issues.append("Final observation evidence is missing")
    if observed.get("protected_sha256") != case.get("protected_sha256"):
        issues.append("Protected state changed")
    if not isinstance(before, dict) or not isinstance(after, dict):
        return issues
    action = case["action"]
    if action == "job.inspect" and (before.get("status") not in TERMINAL or
                                     after.get("status") != before.get("status")):
        issues.append("Inspect did not observe a stable terminal job")
    if action == "job.wait" and (before.get("status") not in ACTIVE or
                                 after.get("status") not in TERMINAL):
        issues.append("Wait did not establish an active-to-terminal job transition")
    if action == "job.cancel" and (before.get("status") not in ACTIVE or
                                    after.get("status") != "cancelled"):
        issues.append("Cancellation was not independently established")
    return issues


def _command_issues(case, commands, observed):
    if not isinstance(commands, list):
        return ["Native command audit is missing"], []
    action = case["action"]
    permitted = {"version.get", "capability.list", "capability.get", "job.inspect"}
    if action in ("job.wait", "job.cancel"):
        permitted.add("job.wait")
    if action == "job.cancel":
        permitted.add("job.cancel")
    issues, target = [], []
    for command in commands:
        if not isinstance(command, dict) or command.get("phase") != "task" or command.get("executed") is not True:
            continue
        capability = command.get("capability")
        if capability not in permitted:
            issues.append("Out-of-scope native command was executed")
            continue
        if capability.startswith("job."):
            flags = command.get("flags")
            if not isinstance(flags, dict) or set(flags) - {"environment", "site", "id", "json", "full", "preview"}:
                issues.append("Job command flags are not bounded")
                continue
            if any(flags.get(key) != case.get(source) for key, source in
                   (("environment", "environment"), ("site", "site"), ("id", "job_id"))):
                issues.append("Job command did not select the exact fixture target")
            if flags.get("json") is not True or flags.get("full") is not True:
                issues.append("Job command omitted bounded structured output")
            if "preview" in flags and (capability != "job.cancel" or flags["preview"] is not True):
                issues.append("Job command has an invalid preview flag")
        if command.get("binary_sha256") != case.get("binary_sha256"):
            issues.append("Native command used a different candidate binary")
        if capability == action and command.get("flags", {}).get("preview") is not True:
            target.append(command)
    if len(target) != 1:
        issues.append("Exactly one executed non-preview target command is required")
        return issues, target
    command = target[0]
    if command.get("exit_status") != 0:
        issues.append("Native target command did not succeed")
    raw = command.get("stdout")
    if not isinstance(raw, str) or len(raw) > 65536:
        issues.append("Native output is missing or oversized")
        return issues, target
    try:
        output = json.loads(raw)
    except (ValueError, TypeError):
        issues.append("Native output is not one JSON document")
        return issues, target
    after = observed.get("after") if isinstance(observed, dict) else None
    if not isinstance(output, dict) or not isinstance(output.get("job"), dict) or not isinstance(after, dict):
        issues.append("Native output lacks an independently comparable job")
        return issues, target
    if output["job"].get("id") != case.get("job_id") or output["job"].get("status") != after.get("status") or output.get("status") != after.get("status"):
        issues.append("Native output differs from exact independent job state")
    if output.get("environment") != case.get("environment") or output.get("site") != case.get("site"):
        issues.append("Native output differs from selected environment or site")
    if action == "job.cancel" and (output.get("confirmed") is not True or
                                   not isinstance(output.get("request_id"), str) or
                                   not output["request_id"] or
                                   command.get("native_ack_sha256") != hashlib.sha256(raw.encode()).hexdigest()):
        issues.append("Native cancellation acknowledgement or confirmation is missing")
    return issues, target


def grade_case(case, commands, observed):
    """Require one native target command and matching independent job state."""
    issues = _fixture_issues(case)
    if issues:
        return {"passed": False, "issues": issues, "command_ids": []}
    issues.extend(_observation_issues(case, observed))
    command_issues, target = _command_issues(case, commands, observed)
    issues.extend(command_issues)
    return {"passed": not issues, "issues": issues,
            "command_ids": [row.get("id") for row in target if isinstance(row.get("id"), str)]}


def cleanup_case(case, observation):
    """Do not call cleanup complete without exact independent owned-asset absence."""
    if _fixture_issues(case) or not isinstance(observation, dict):
        return {"status": "unresolved"}
    exact = all(observation.get(key) == case.get(key) for key in
                ("run_id", "case_id", "job_id", "site_luid", "datasource_luid"))
    absence = observation.get("datasource_absence")
    path = (f"/api/{case['api_version']}/sites/{case['site_luid']}/"
            f"datasources/{case['datasource_luid']}")
    absent = (isinstance(absence, dict) and absence.get("method") == "GET"
              and absence.get("path") == path and absence.get("http_status") == 404
              and isinstance(absence.get("response_body"), str)
              and len(absence["response_body"]) <= 65536
              and _hash(absence.get("response_sha256"))
              and hashlib.sha256(absence["response_body"].encode()).hexdigest()
              == absence["response_sha256"])
    if (exact and observation.get("independent") is True and
            observation.get("job_terminal") is True and
            observation.get("job_status") in TERMINAL and
            (case["action"] != "job.cancel" or observation.get("job_status") == "cancelled") and
            observation.get("ownership_sha256") == case.get("ownership_sha256") and
            observation.get("owned_assets_absent") is True and
            absent and
            observation.get("protected_sha256") == case.get("protected_sha256") and
            _hash(observation.get("evidence_sha256"))):
        return {"status": "completed"}
    return {"status": "unresolved"}
