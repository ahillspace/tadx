"""Two-phase, fixed-case contract for disposable hosted Windows installer jobs."""

from __future__ import annotations

import hashlib
import json
import re

CASES = frozenset({"fresh", "idempotent", "no-completion", "completion-opt-in",
                   "failed-download-preserves-binary", "uninstall", "no-modify-path"})
HEX64 = re.compile(r"[0-9a-f]{64}\Z")
GIT_SHA = re.compile(r"[0-9a-f]{40}\Z")
NONCE = re.compile(r"[A-Za-z0-9_-]{12,64}\Z")
VERSION = re.compile(r"[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?\Z")
WORKFLOW = ".github/workflows/g9-windows-installer.yml"


class Refused(ValueError):
    """The requested case, source, or hosted evidence is outside fixed scope."""


def hex64(value, label):
    if not isinstance(value, str) or not HEX64.fullmatch(value):
        raise Refused("Invalid " + label)
    return value


def git_sha(value, label):
    if not isinstance(value, str) or not GIT_SHA.fullmatch(value):
        raise Refused("Invalid " + label)
    return value


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def argv_for(case, version):
    if case not in CASES or not isinstance(version, str) or not VERSION.fullmatch(version):
        raise Refused("Unknown installer case or unsafe fixture version")
    if case == "uninstall":
        return ("uninstall",)
    selected = version + "-missing" if case == "failed-download-preserves-binary" else version
    extras = {"no-completion": ("--no-completion",),
              "no-modify-path": ("--no-modify-path",)}.get(case, ())
    return ("install", "--version", selected, *extras)


def setup_request(case, source_sha, nonce, *, version, binary_sha256, installer_sha256):
    argv_for(case, version)
    if not isinstance(nonce, str) or not NONCE.fullmatch(nonce):
        raise Refused("Invalid setup nonce")
    body = {"protocol": "tadx-windows-installer-setup/1", "case": case,
            "source_sha": git_sha(source_sha, "candidate source commit"), "nonce": nonce,
            "fixture_version": version,
            "binary_sha256": hex64(binary_sha256, "candidate Windows binary"),
            "installer_sha256": hex64(installer_sha256, "candidate installer")}
    body["setup_sha256"] = digest(body)
    return body


def command_request(setup, argv, setup_commit, baseline_sha256):
    if type(argv) is not list or tuple(argv) != argv_for(setup["case"], setup["fixture_version"]):
        raise Refused("Model helper argv exceeds the fixed installer case")
    body = {"protocol": "tadx-windows-installer-command/1",
            "case": setup["case"], "nonce": setup["nonce"], "argv": argv,
            "setup_commit": git_sha(setup_commit, "setup commit"),
            "setup_sha256": setup["setup_sha256"],
            "baseline_sha256": hex64(baseline_sha256, "immutable baseline")}
    body["command_sha256"] = digest(body)
    return body


def stop_request(setup, setup_commit, baseline_sha256, run_id):
    """Bind an internal unused-session stop to the same hosted job and baseline."""
    if type(run_id) is not int or run_id <= 0:
        raise Refused("Hosted run identity is missing")
    body = {"protocol": "tadx-windows-installer-stop/1",
            "case": setup["case"], "nonce": setup["nonce"],
            "source_sha": setup["source_sha"],
            "setup_commit": git_sha(setup_commit, "setup commit"),
            "setup_sha256": setup["setup_sha256"],
            "baseline_sha256": hex64(baseline_sha256, "immutable baseline"),
            "run_id": run_id}
    body["stop_sha256"] = digest(body)
    return body


def run_issues(run, setup, setup_commit):
    issues = []
    if run.get("head_sha") != setup_commit or run.get("head_branch") != "g9-win/" + setup["nonce"]:
        issues.append("Hosted run is not the exact setup branch head")
    if run.get("event") != "push" or run.get("path") != WORKFLOW:
        issues.append("Hosted run used another event or workflow")
    if not isinstance(run.get("id"), int) or run["id"] <= 0:
        issues.append("Hosted run identity is missing")
    return issues


def baseline_issues(setup, setup_commit, run, baseline):
    issues = run_issues(run, setup, setup_commit)
    expected = {"protocol": "tadx-windows-installer-baseline/1",
                "case": setup["case"], "nonce": setup["nonce"],
                "source_sha": setup["source_sha"], "setup_sha256": setup["setup_sha256"],
                "setup_commit": setup_commit, "run_id": run.get("id"),
                "binary_sha256": setup["binary_sha256"],
                "installer_sha256": setup["installer_sha256"]}
    if any(baseline.get(key) != value for key, value in expected.items()):
        issues.append("Pre-task baseline identity differs")
    before = baseline.get("before")
    if not isinstance(before, dict) or before.get("profile_sentinel") is not True:
        issues.append("Independent starting-state snapshot is missing")
    else:
        expected_binary = setup["binary_sha256"] if setup["case"] in {
            "idempotent", "uninstall", "failed-download-preserves-binary",
            "completion-opt-in"} else None
        if before.get("binary_sha256") != expected_binary:
            issues.append("Starting binary does not match the declared fixture")
        expected_markers = 0 if setup["case"] == "completion-opt-in" else (1 if expected_binary else 0)
        if before.get("completion_markers") != expected_markers:
            issues.append("Starting completion state differs")
        if before.get("user_path_count") != (1 if expected_binary else 0):
            issues.append("Starting User PATH state differs")
        if not isinstance(before.get("sentinel_sha256"), str) or not HEX64.fullmatch(before["sentinel_sha256"]):
            issues.append("Starting sentinel hash is missing")
    if baseline.get("pre_task_observed") is not True:
        issues.append("Starting state was not independently observed")
    return issues


def observed_issues(case, before, after, exit_status, binary_sha, transport, version):
    issues = []
    if (not isinstance(before, dict) or not isinstance(after, dict)
            or before.get("sentinel_sha256") != after.get("sentinel_sha256")
            or before.get("profile_sentinel") is not True
            or after.get("profile_sentinel") is not True):
        return ["Unrelated fixture state is missing or changed"]
    if not isinstance(transport, list):
        issues.append("Exact fixture transport evidence missing")
        transport = []
    archive = f"tadx_{version}_windows_amd64.zip"
    missing_archive = f"tadx_{version}-missing_windows_amd64.zip"
    if case == "uninstall":
        if transport:
            issues.append("Uninstall unexpectedly retrieved an asset")
    elif case == "failed-download-preserves-binary":
        expected_transport = [{"asset": "checksums.txt", "failed": False},
                              {"asset": missing_archive, "failed": True},
                              {"asset": missing_archive, "failed": True,
                               "transport": "https"}]
        if transport != expected_transport:
            issues.append("Both local asset transport refusals were not observed in order")
        if (exit_status == 0 or before.get("binary_sha256") != binary_sha
                or after != before):
            issues.append("Failed archive retrieval did not preserve the installation")
        return issues
    elif transport != [{"asset": "checksums.txt", "failed": False},
                       {"asset": archive, "failed": False}]:
        issues.append("Exact local checksum and archive retrieval was not observed")
    if exit_status != 0:
        return [*issues, "Native installer did not complete"]
    if case == "uninstall":
        if (before.get("binary_sha256") != binary_sha or after.get("binary_sha256") is not None
                or after.get("completion_markers") != 0 or after.get("user_path_count") != 0):
            issues.append("Uninstall did not remove managed state")
        return issues
    if case in {"fresh", "no-completion", "no-modify-path"}:
        if before.get("binary_sha256") is not None or before.get("completion_markers") != 0:
            issues.append("Fresh fixture had preexisting managed state")
    if case == "completion-opt-in":
        if (before.get("binary_sha256") != binary_sha
                or before.get("completion_markers") != 0
                or before.get("user_path_count") != 1
                or before.get("profile_sha256") == after.get("profile_sha256")):
            issues.append("Completion opt-in did not start from an installed opt-out state")
    if after.get("binary_sha256") != binary_sha:
        issues.append("Installed binary differs from exact candidate build")
    if case == "idempotent" and before != after:
        issues.append("Idempotent install changed established state")
    if case == "no-completion":
        if after.get("completion_markers") != 0:
            issues.append("Completion changed despite opt-out")
    elif after.get("completion_markers") != 1:
        issues.append("Managed completion is not present exactly once")
    if case == "no-modify-path":
        if before.get("user_path_count") != 0 or after.get("user_path_count") != 0:
            issues.append("User PATH changed despite opt-out")
    elif after.get("user_path_count") != 1:
        issues.append("Managed User PATH is not present exactly once")
    if case == "completion-opt-in" and before.get("user_path_count") != after.get("user_path_count"):
        issues.append("Completion opt-in changed the existing User PATH entry")
    return issues


def result_issues(setup, setup_commit, command, command_commit, baseline, baseline_sha,
                  run, result):
    issues = run_issues(run, setup, setup_commit)
    if run.get("conclusion") != "success":
        issues.append("Hosted job did not complete cleanly")
    expected = {"protocol": "tadx-windows-installer-result/1",
                "case": setup["case"], "nonce": setup["nonce"],
                "setup_commit": setup_commit, "command_commit": command_commit,
                "setup_sha256": setup["setup_sha256"],
                "command_sha256": command["command_sha256"],
                "baseline_sha256": baseline_sha, "run_id": run.get("id"),
                "binary_sha256": setup["binary_sha256"],
                "installer_sha256": setup["installer_sha256"]}
    if any(result.get(key) != value for key, value in expected.items()):
        issues.append("Native result identity differs from both commits and baseline")
    if result.get("argv") != command["argv"] or result.get("native_executed") is not True:
        issues.append("Exact model-selected native command is not proved")
    if result.get("before") != baseline.get("before") or result.get("fresh_before_command") is not True:
        issues.append("Starting fixture changed before the model command")
    if result.get("cleanup_ok") is not True or result.get("observer_issues") != []:
        issues.append("Native observation or cleanup is incomplete")
    if not isinstance(result.get("before"), dict) or not isinstance(result.get("after"), dict):
        issues.append("Native before or after snapshot is missing")
    else:
        issues.extend(observed_issues(setup["case"], result["before"], result["after"],
                                      result.get("native_exit_status"), setup["binary_sha256"],
                                      result.get("transport"), setup["fixture_version"]))
    return issues
