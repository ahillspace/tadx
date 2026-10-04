"""Seven fixed installer cases with a pre-task hosted Windows baseline.

This profile belongs only in a blocked, source-locked prepared suite. The
hosted branch and job are provisioned before the model receives its task.
"""

import hashlib
import json
from pathlib import Path

from integration import scenario_common as common
from integration import g9_windows_gate as gate
from integration import g9_windows_host_relay as relay


PREFIX = "V-installer-windows-"
SESSION = "g9-windows-session.json"
OUTCOME = "g9-windows-outcome.json"
BASELINE = "g9-windows-baseline.json"
WATCH = "g9-windows-watch.json"


def _case(req):
    identity = req["exercise"]["id"]
    if not isinstance(identity, str) or not identity.startswith(PREFIX):
        raise gate.Refused("Windows installer profile received another action")
    case = identity[len(PREFIX):]
    if case not in gate.CASES:
        raise gate.Refused("Windows installer profile received an unknown variant")
    return case


def _save(path, value):
    path = Path(path)
    with path.open("x", encoding="utf-8") as stream:
        json.dump(value, stream, sort_keys=True, separators=(",", ":"))
        stream.write("\n")


def _load(path):
    return json.loads(Path(path).read_text(encoding="utf-8"))


def _identity(req, frozen):
    case = _case(req)
    constraints = req.get("run_constraints") or {}
    source = frozen.get("source_commit")
    if source != constraints.get("g9_accepted_gate_sha"):
        raise gate.Refused("Accepted candidate gate revision is not pinned")
    version = constraints.get("g9_windows_installer_version")
    gate.argv_for(case, version)
    binary = frozen["binaries"]["windows"]["sha256"]
    installer = Path(frozen["root"]) / "scripts" / "install.ps1"
    if not installer.is_file() or installer.is_symlink():
        raise gate.Refused("Exact candidate installer source is missing")
    installer_sha = hashlib.sha256(installer.read_bytes()).hexdigest()
    return case, source, version, binary, installer_sha


def preflight(req, frozen):
    _identity(req, frozen)
    if req.get("deterministic") is True:
        raise gate.Refused("Hosted native installer requires an actual model task")
    return {"qualified_requirements": ["platform:windows", "qualified_release_installer"],
            "hosted_fixture": "pre-task baseline and exact one-command child required"}


def _expected(case, binary, before):
    if case == "uninstall":
        return {"binary_sha256": None, "completion_markers": 0, "user_path_count": 0}
    if case == "failed-download-preserves-binary":
        return {key: before[key] for key in ("binary_sha256", "completion_markers",
                                               "user_path_count")}
    return {"binary_sha256": binary,
            "completion_markers": 0 if case == "no-completion" else 1,
            "user_path_count": 0 if case == "no-modify-path" else 1}


def prepare(req, frozen, work):
    preflight(req, frozen)
    case, source, version, binary, installer = _identity(req, frozen)
    root = Path(req["private_case_dir"])
    if any((root / name).exists() for name in (SESSION, OUTCOME, BASELINE)):
        raise gate.Refused("Windows installer case already has hosted state")
    session = relay.prepare_hosted(case=case, source_sha=source, version=version,
                                   binary_sha256=binary, installer_sha256=installer,
                                   accepted_gate_sha=source)
    try:
        _save(root / SESSION, session)
        baseline = session["baseline"]
        _save(root / BASELINE, baseline)
        return _prepared_result(req, case, source, version, binary, installer, session)
    except BaseException:
        cleanup = relay.abort_hosted(session)
        if cleanup.get("branch_cleanup") != "verified" or cleanup.get("native_cleanup") != "verified":
            raise gate.Refused("Prepared hosted fixture did not prove cleanup") from None
        _save(root / WATCH, {"status": "prepare_failed", **cleanup})
        raise


def _prepared_result(req, case, source, version, binary, installer, session):
    baseline = session["baseline"]
    before = baseline["before"]
    argv = list(gate.argv_for(case, version))
    expected = {"result": _expected(case, binary, before),
                "canonical": {"case": case, "argv": argv}}
    public = {"task_inputs": {"command": "tadx-bench-installer", "arguments": argv,
                              "candidate_version": version, "behavior": case,
                              "scope": "one disposable hosted Windows installer fixture"}}
    if case == "completion-opt-in":
        public["task_inputs"]["starting_state"] = (
            "Pinned candidate binary and User PATH entry already exist without completion")
        public["task_inputs"]["goal"] = (
            "Enable completion exactly once while preserving the binary and User PATH entry")
    fixture = common.bind_fixture(req, public,
                                  {"target": {"path": "hosted:test-owned-install-dir"},
                                   "sentinel": {"path": "hosted:test-owned-sentinel"}},
                                  expected, before, source=BASELINE,
                                  conditions=[{"helper": "installer", "argv": argv,
                                               "refusal": case == "failed-download-preserves-binary"}])
    guard = {"mode": "shared-windows-installer", "family": "g9-windows-installer",
             "operations": [], "case": case, "exercise_id": req["exercise"]["id"],
             "source_sha": source, "binary_sha256": binary,
             "installer_sha256": installer, "setup_commit": session["setup_commit"],
             "baseline_sha256": session["baseline_sha256"],
             "hosted_run_id": session["run_id"], "fixture_version": version,
             "credential_input": False, "runtime_network": "none"}
    return {"fixture": fixture, "before": {"protected": {
                "sentinel_sha256": before["sentinel_sha256"]}},
            "runtime_network": "none", "config_seed": {"version": 1, "environments": {}},
            "broker_guard": guard, "baseline_evidence": [BASELINE],
            "baseline_assertions": [{"id": "hosted-pre-task-baseline", "status": "pass",
                                     "evidence": [BASELINE]}]}


def qualification_commands(req, fixture):
    case = _case(req)
    root = Path(req["private_case_dir"])
    session = _load(root / SESSION)
    return [{"argv": list(gate.argv_for(case, session["setup"]["fixture_version"])),
             "helper": "installer", "refusal": case == "failed-download-preserves-binary"}]


def observe(req, state, config, work, commands):
    root = Path(req["private_case_dir"])
    session = _load(root / SESSION)
    case = _case(req)
    expected_argv = list(gate.argv_for(case, session["setup"]["fixture_version"]))
    rows = [row for row in commands if row.get("phase") == "task"
            and row.get("helper") == "installer" and row.get("executed") is True]
    issues = []
    if len(rows) != 1 or rows[0].get("argv") != ["tadx-bench-installer", *expected_argv]:
        issues.append("Exact one-command installer audit is missing")
    outcome_path = root / OUTCOME
    outcome = _load(outcome_path) if outcome_path.is_file() else None
    if not isinstance(outcome, dict) or outcome.get("status") != "verified":
        issues.append("Hosted native result is not verified")
    result = outcome.get("native_result", {}) if isinstance(outcome, dict) else {}
    if not isinstance(result, dict):
        result = {}
    if not issues:
        command = gate.command_request(session["setup"], expected_argv,
                                       session["setup_commit"], session["baseline_sha256"])
        issues.extend(gate.result_issues(session["setup"], session["setup_commit"], command,
                                          outcome["command_commit"], session["baseline"],
                                          session["baseline_sha256"], outcome["hosted_run"], result))
        if rows[0].get("hosted_run_id") != session["run_id"] or rows[0].get(
                "hosted_command_sha256") != command["command_sha256"]:
            issues.append("Broker audit is not bound to the hosted native result")
    after = result.get("after") if isinstance(result.get("after"), dict) else {}
    actual = {key: after.get(key) for key in ("binary_sha256", "completion_markers",
                                             "user_path_count")}
    return {"native_command_verified": not issues,
            "result": actual,
            "canonical": {"case": case, "argv": expected_argv} if not issues else None,
            "protected": {"sentinel_sha256": after.get("sentinel_sha256")},
            "unowned_changes": issues, "boundary_or_helper_coverage_complete": not issues}


def cleanup(req, state, work=None):
    root = Path(req["private_case_dir"])
    session_path = root / SESSION
    if not session_path.is_file():
        return {"status": "blocked", "reason": "Hosted session identity is missing"}
    outcome_path = root / OUTCOME
    if not outcome_path.is_file():
        watch = root / WATCH
        watch_state = _load(watch) if watch.is_file() else {}
        if (watch_state.get("branch_cleanup") == "verified"
                and watch_state.get("native_cleanup") == "verified"):
            return {"status": "pass", "scope": "Hosted job and exact branch removed"}
        cleanup = relay.abort_hosted(_load(session_path))
        if cleanup.get("branch_cleanup") != "verified" or cleanup.get("native_cleanup") != "verified":
            return {"status": "blocked", "reason": "Unused hosted job cleanup is incomplete"}
        return {"status": "pass", "scope": "Unused hosted job restored; exact branch removed"}
    outcome = _load(outcome_path)
    native = outcome.get("native_result", {})
    return {"status": "pass" if outcome.get("branch_cleanup") == "verified"
            and isinstance(native, dict) and native.get("cleanup_ok") is True else "blocked",
            "scope": "Exact hosted branch and test-owned Windows fixture"}


def task_instructions(exercise, public):
    return common.instructions(exercise, public)
