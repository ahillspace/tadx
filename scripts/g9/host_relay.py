"""Two-phase disposable Windows relay for seven fixed installer cases.

Prepare before the model task, then execute one audited helper argv. No dispatch
occurs at import time, and this draft is blocked pending independent review.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import tempfile
import threading
import time

from windows_gate import (Refused, baseline_issues, command_request, result_issues,
                          setup_request, stop_request)

REPO = "ahillspace/tadx"
REMOTE = "https://github.com/ahillspace/tadx.git"
WORKFLOW = "g9-windows-installer.yml"
SETUP_FILE = ".g9-windows-installer-setup.json"
COMMAND_FILE = ".g9-windows-installer-command.json"
PREPARE_LIMIT = 12 * 60
EXECUTE_LIMIT = 30 * 60
BROKER_ID = re.compile(r"cmd-[0-9]{5}\Z")


def verify_broker_request(session, value):
    """Accept only the exact broker audit record bound to the pre-task baseline."""
    setup = session["setup"]
    fields = ("protocol", "id", "case", "argv", "source_sha", "binary_sha256",
              "installer_sha256", "fixture_version", "setup_commit",
              "baseline_sha256", "run_id")
    if (not isinstance(value, dict) or set(value) != {*fields, "command_sha256"}
            or value.get("protocol") != "tadx-windows-installer-broker/1"
            or not isinstance(value.get("id"), str) or not BROKER_ID.fullmatch(value["id"])):
        raise Refused("Model helper audit request has an unknown shape")
    expected = {"protocol": "tadx-windows-installer-broker/1", "id": value["id"],
                "case": setup["case"], "argv": value["argv"],
                "source_sha": setup["source_sha"],
                "binary_sha256": setup["binary_sha256"],
                "installer_sha256": setup["installer_sha256"],
                "fixture_version": setup["fixture_version"],
                "setup_commit": session["setup_commit"],
                "baseline_sha256": session["baseline_sha256"],
                "run_id": session["run_id"]}
    command_request(setup, value["argv"], session["setup_commit"],
                    session["baseline_sha256"])
    encoded = json.dumps(expected, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
    expected["command_sha256"] = hashlib.sha256(encoded).hexdigest()
    if value != expected:
        raise Refused("Model helper audit request differs from the prewarmed session")
    return value


def broker_response(request, outcome):
    """Return bounded native feedback, not a substitute for independent grading."""
    verified = outcome["status"] == "verified"
    native = outcome["native_result"]
    if not isinstance(native, dict):
        raise Refused("Hosted native result is not an object")
    native_status = native.get("native_exit_status")
    if verified and (type(native_status) is not int or native_status < 0 or native_status >= 124):
        raise Refused("Verified native result has an invalid exit status")
    def bounded_text(value):
        if not isinstance(value, str):
            raise Refused("Native output is not bounded text")
        return value.encode("utf-8")[:8192].decode("utf-8", errors="ignore")

    return {"protocol": "tadx-windows-installer-response/1",
            "id": request["id"], "command_sha256": request["command_sha256"],
            "setup_commit": request["setup_commit"],
            "baseline_sha256": request["baseline_sha256"],
            "run_id": request["run_id"],
            "status": "verified" if verified else "not_verified",
            "exit_status": native_status if verified else 125,
            "stdout": bounded_text(native.get("native_stdout", "")) if verified else "",
            "stderr": bounded_text(native.get("native_stderr", "")) if verified else "Hosted native evidence did not verify.\n"}


def command(argv, *, cwd=None, timeout=60):
    try:
        return subprocess.run(argv, cwd=cwd, capture_output=True, text=True,
                              encoding="utf-8", errors="replace", timeout=timeout)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise Refused("Hosted fixture command unavailable: " + type(exc).__name__) from None


def checked(argv, *, cwd=None, timeout=60):
    result = command(argv, cwd=cwd, timeout=timeout)
    if result.returncode:
        raise Refused("Hosted fixture command failed: " + argv[0])
    return result.stdout.strip()


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def choose_run(rows, branch, setup_commit):
    matches = [row for row in rows if row.get("headSha") == setup_commit
               and row.get("headBranch") == branch and row.get("event") == "push"]
    if len(matches) > 1:
        raise Refused("More than one hosted run claims the exact setup")
    return matches[0] if matches else None


def wait_for_run(branch, setup_commit, deadline):
    while time.monotonic() < deadline:
        rows = json.loads(checked(["gh", "run", "list", "--repo", REPO, "--workflow", WORKFLOW,
                                   "--branch", branch, "--limit", "20", "--json",
                                   "databaseId,headSha,headBranch,event,status,conclusion"]))
        chosen = choose_run(rows, branch, setup_commit)
        if chosen:
            return chosen
        time.sleep(10)
    raise Refused("Dedicated hosted setup run did not start within its bound")


def run_metadata(run_id):
    data = json.loads(checked(["gh", "api", "-X", "GET", f"repos/{REPO}/actions/runs/{run_id}"]))
    return {"id": data.get("id"), "head_sha": data.get("head_sha"),
            "head_branch": data.get("head_branch"), "event": data.get("event"),
            "status": data.get("status"), "conclusion": data.get("conclusion"),
            "path": data.get("path")}


def wait_for_completion(run_id, deadline, stop_event):
    while time.monotonic() < deadline:
        if stop_event.is_set():
            raise Refused("Model task ended before the hosted result")
        run = run_metadata(run_id)
        if run["status"] == "completed":
            return run
        stop_event.wait(10)
    raise Refused("Dedicated hosted job did not finish within its bound")


def remote_head(clone, branch):
    value = checked(["git", "ls-remote", "--heads", "origin", "refs/heads/" + branch], cwd=clone)
    return value.split()[0] if value else None


def one_file_commit(clone, name, body, parent):
    target = clone / name
    if target.exists() or target.is_symlink():
        raise Refused("Disposable fixture command file already exists")
    target.write_text(json.dumps(body, sort_keys=True, separators=(",", ":")) + "\n",
                      encoding="utf-8")
    checked(["git", "add", "--", name], cwd=clone)
    identity = ["-c", "user.name=TADX fixture", "-c", "user.email=fixture@invalid.example"]
    checked(["git", *identity, "commit", "-m", "G9 Windows fixture " + body["nonce"]], cwd=clone)
    head = checked(["git", "rev-parse", "HEAD"], cwd=clone)
    changed = checked(["git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"], cwd=clone)
    if changed != name or checked(["git", "rev-parse", "HEAD^"], cwd=clone) != parent:
        raise Refused("Disposable commit changed candidate source or setup")
    return head


def remove_exact_branch(clone, branch, expected_head):
    current = remote_head(clone, branch)
    if current is None:
        return True
    if current != expected_head:
        return False
    lease = "--force-with-lease=refs/heads/" + branch + ":" + expected_head
    result = command(["git", "push", lease, "origin", "--delete", branch],
                     cwd=clone, timeout=120)
    return result.returncode == 0 and remote_head(clone, branch) is None


def remove_area(area):
    raw = Path(area).absolute()
    expected = Path(tempfile.gettempdir()).resolve(strict=True)
    if (raw.is_symlink() or raw.parent.resolve(strict=True) != expected
            or not raw.name.startswith("tadx-g9-windows-")):
        raise Refused("Disposable relay directory is outside its exact test-owned scope")
    area = raw.resolve(strict=True)
    if area != raw or not area.is_dir():
        raise Refused("Disposable relay directory changed")
    shutil.rmtree(area)


def download_artifact(run_id, name, destination):
    destination.mkdir(parents=True, exist_ok=True)
    outcome = command(["gh", "run", "download", str(run_id), "--repo", REPO,
                       "--name", name, "--dir", str(destination)], timeout=90)
    return outcome.returncode == 0


def read_artifact(destination, name):
    path = destination / name
    if (path.is_symlink() or not path.is_file() or path.resolve().parent != destination.resolve()
            or path.stat().st_size > 131072):
        raise Refused("Bounded hosted evidence artifact is missing")
    return json.loads(path.read_text(encoding="utf-8")), sha(path)


def wait_for_baseline(run_id, setup, setup_commit, destination, deadline):
    while time.monotonic() < deadline:
        if download_artifact(run_id, "g9-windows-baseline-" + str(run_id), destination):
            baseline, baseline_sha = read_artifact(destination, "baseline.json")
            run = run_metadata(run_id)
            problems = baseline_issues(setup, setup_commit, run, baseline)
            if problems:
                raise Refused("Pre-task baseline admission failed: " + "; ".join(problems))
            return baseline, baseline_sha
        run = run_metadata(run_id)
        if run.get("status") == "completed":
            raise Refused("Hosted setup finished without an admissible baseline")
        time.sleep(10)
    raise Refused("Pre-task baseline was not published within its bound")


def prepare_hosted(*, case, source_sha, version, binary_sha256, installer_sha256,
                   accepted_gate_sha):
    """Return a baseline-qualified session before any model installer task."""
    if source_sha != accepted_gate_sha:
        raise Refused("Final accepted gate revision differs from installer candidate")
    setup = setup_request(case, source_sha, secrets.token_hex(12), version=version,
                          binary_sha256=binary_sha256, installer_sha256=installer_sha256)
    area = Path(tempfile.mkdtemp(prefix="tadx-g9-windows-"))
    clone = area / "repository"
    branch = "g9-win/" + setup["nonce"]
    setup_commit = None
    run_id = None
    pushed = False
    push_attempted = False
    try:
        checked(["git", "clone", "--filter=blob:none", "--no-checkout", REMOTE, str(clone)], timeout=120)
        if remote_head(clone, branch) is not None:
            raise Refused("Disposable Windows branch already exists")
        checked(["git", "switch", "--detach", source_sha], cwd=clone)
        for path in ("scripts/g9/windows_worker.py", "scripts/g9/windows_gate.py",
                     "scripts/g9/windows_build.py",
                     "scripts/g9/gh_fixture.py", "scripts/g9/installer_wrapper.ps1",
                     ".github/workflows/g9-windows-installer.yml"):
            if not (clone / path).is_file():
                raise Refused("Candidate lacks reviewed Windows fixture source")
        checked(["git", "switch", "-c", branch], cwd=clone)
        setup_commit = one_file_commit(clone, SETUP_FILE, setup, source_sha)
        push_attempted = True
        checked(["git", "push", "--force-with-lease=refs/heads/" + branch + ":",
                 "origin", "HEAD:refs/heads/" + branch], cwd=clone, timeout=120)
        pushed = True
        if remote_head(clone, branch) != setup_commit:
            raise Refused("Disposable setup branch push was not confirmed")
        chosen = wait_for_run(branch, setup_commit, time.monotonic() + PREPARE_LIMIT)
        run_id = chosen["databaseId"]
        baseline, baseline_sha = wait_for_baseline(run_id, setup, setup_commit,
                                                   area / "baseline-artifact",
                                                   time.monotonic() + PREPARE_LIMIT)
        return {"area": str(area), "branch": branch, "setup": setup,
                "setup_commit": setup_commit, "run_id": run_id,
                "baseline": baseline, "baseline_sha256": baseline_sha}
    except BaseException:
        if pushed or push_attempted:
            raise Refused("Hosted setup publication or cleanup is unverified; exact branch and local evidence are retained") from None
        if (clone / ".git").is_dir() and remote_head(clone, branch) == setup_commit:
            raise Refused("Hosted setup publication is uncertain; exact branch and local evidence are retained") from None
        remove_area(area)
        raise


def execute_hosted(session, argv, stop_event=None):
    """Execute only the fixed audited argv in the prewarmed job."""
    area = Path(session["area"])
    clone = area / "repository"
    setup = session["setup"]
    branch = session["branch"]
    setup_commit = session["setup_commit"]
    expected_head = setup_commit
    outcome = None
    stop_event = stop_event or threading.Event()
    try:
        if remote_head(clone, branch) != setup_commit:
            raise Refused("Prewarmed branch changed before the model command")
        baseline_path = area / "baseline-artifact" / "baseline.json"
        if sha(baseline_path) != session["baseline_sha256"]:
            raise Refused("Pre-task baseline artifact changed")
        latest = run_metadata(session["run_id"])
        if baseline_issues(setup, setup_commit, latest, session["baseline"]):
            raise Refused("Prewarmed run identity changed")
        model_command = command_request(setup, argv, setup_commit, session["baseline_sha256"])
        expected_head = one_file_commit(clone, COMMAND_FILE, model_command, setup_commit)
        checked(["git", "push", "--force-with-lease=refs/heads/" + branch + ":" + setup_commit,
                 "origin", "HEAD:refs/heads/" + branch], cwd=clone, timeout=120)
        if remote_head(clone, branch) != expected_head:
            raise Refused("Model command child commit push was not confirmed")
        run = wait_for_completion(session["run_id"], time.monotonic() + EXECUTE_LIMIT,
                                  stop_event)
        artifact = area / "result-artifact"
        if not download_artifact(run["id"], "g9-windows-result-" + str(run["id"]), artifact):
            raise Refused("Native result artifact was not published")
        result, result_sha = read_artifact(artifact, "result.json")
        if not isinstance(result, dict):
            raise Refused("Native result has an invalid shape")
        issues = result_issues(setup, setup_commit, model_command, expected_head,
                               session["baseline"], session["baseline_sha256"], run, result)
        if issues:
            raise Refused("Native result or cleanup is not verified; exact branch and local evidence are retained")
        outcome = {"status": "verified" if not issues else "not_verified",
                   "issues": issues, "run_id": run["id"], "source_sha": setup["source_sha"],
                   "setup_commit": setup_commit, "command_commit": expected_head,
                   "baseline_sha256": session["baseline_sha256"],
                   "result_sha256": result_sha, "native_result": result,
                   "hosted_run": run}
    finally:
        if outcome is not None:
            actual_head = remote_head(clone, branch)
            if actual_head not in {setup_commit, expected_head}:
                raise Refused("Disposable Windows branch changed unexpectedly")
            if not remove_exact_branch(clone, branch, actual_head):
                raise Refused("Disposable Windows branch was not safely removed")
            remove_area(area)
    outcome["branch_cleanup"] = "verified"
    return outcome


def abort_hosted(session):
    """Close an unused session only after native rollback evidence arrives."""
    clone = Path(session["area"]) / "repository"
    branch = session["branch"]
    setup_commit = session["setup_commit"]
    if remote_head(clone, branch) != setup_commit:
        raise Refused("Cannot abort a branch with an unexpected command head")
    area = Path(session["area"])
    if sha(area / "baseline-artifact" / "baseline.json") != session["baseline_sha256"]:
        raise Refused("Pre-task baseline artifact changed before unused-session stop")
    latest = run_metadata(session["run_id"])
    if baseline_issues(session["setup"], setup_commit, latest, session["baseline"]):
        raise Refused("Prewarmed run identity changed before unused-session stop")
    stop = stop_request(session["setup"], setup_commit,
                        session["baseline_sha256"], session["run_id"])
    stop_commit = one_file_commit(clone, COMMAND_FILE, stop, setup_commit)
    checked(["git", "push", "--force-with-lease=refs/heads/" + branch + ":" + setup_commit,
             "origin", "HEAD:refs/heads/" + branch], cwd=clone, timeout=120)
    if remote_head(clone, branch) != stop_commit:
        raise Refused("Unused-session stop child commit push was not confirmed")
    run = wait_for_completion(session["run_id"], time.monotonic() + 120,
                              threading.Event())
    if baseline_issues(session["setup"], setup_commit, run, session["baseline"]):
        raise Refused("Unused hosted job identity changed during cleanup")
    artifact = area / "aborted-result-artifact"
    if not download_artifact(run["id"], "g9-windows-result-" + str(run["id"]), artifact):
        raise Refused("Unused hosted job has no native cleanup artifact")
    result, _ = read_artifact(artifact, "result.json")
    expected = {"protocol": "tadx-windows-installer-result/1",
                "case": session["setup"]["case"], "nonce": session["setup"]["nonce"],
                "setup_commit": setup_commit, "command_commit": stop_commit,
                "setup_sha256": session["setup"]["setup_sha256"],
                "baseline_sha256": session["baseline_sha256"], "run_id": run["id"],
                "binary_sha256": session["setup"]["binary_sha256"],
                "installer_sha256": session["setup"]["installer_sha256"],
                "before": session["baseline"]["before"]}
    if (not isinstance(result, dict)
            or any(result.get(key) != value for key, value in expected.items())
            or result.get("stop_requested") is not True
            or result.get("fresh_before_stop") is not True
            or result.get("native_executed") is not False
            or result.get("cleanup_ok") is not True):
        raise Refused("Unused hosted job lacks verified native cleanup")
    if not remove_exact_branch(clone, branch, stop_commit):
        raise Refused("Unused hosted branch was not safely removed after cleanup")
    remove_area(session["area"])
    return {"branch_cleanup": "verified", "native_cleanup": "verified"}
