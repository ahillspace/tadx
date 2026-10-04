"""Prepare, then execute one fixed installer case on a hosted Windows VM.

No Tableau or OpenAI credentials are accepted. This script is not a local-host
installer path. Its workflow must prove an exact one-file request commit.
"""

from __future__ import annotations

import base64
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import time
import zipfile

from windows_gate import (Refused, baseline_issues, command_request,
                          observed_issues, setup_request, stop_request)
from windows_build import TOOLCHAIN, build_candidate

SETUP_PATH = Path(".g9-windows-installer-setup.json")
COMMAND_PATH = Path(".g9-windows-installer-command.json")
MAX_OUTPUT = 8192


def strict_json(raw):
    def unique(pairs):
        value = {}
        for key, item in pairs:
            if key in value:
                raise Refused("Duplicate native fixture request field")
            value[key] = item
        return value
    return json.loads(raw, object_pairs_hook=unique)


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def child_process_environment():
    """Keep the job's GitHub token out of builds and native installer children."""
    return {key: value for key, value in os.environ.items()
            if key != "G9_GITHUB_TOKEN" and not key.startswith("GIT_CONFIG_")}


def git_network_environment():
    """Give only a fixed Git network command process-scoped read credentials."""
    remote = command("git", "remote", "get-url", "origin", timeout=10)
    if remote.returncode or remote.stdout.strip() not in {
            "https://github.com/ahillspace/tadx", "https://github.com/ahillspace/tadx.git"}:
        raise Refused("Job-scoped Git credential cannot reach an unknown remote")
    token = os.environ.get("G9_GITHUB_TOKEN")
    if not token or "\n" in token or "\r" in token:
        raise Refused("Job-scoped GitHub read token is unavailable")
    encoded = base64.b64encode(("x-access-token:" + token).encode("utf-8")).decode("ascii")
    env = child_process_environment()
    env.update(GIT_CONFIG_COUNT="1",
               GIT_CONFIG_KEY_0="http.https://github.com/.extraheader",
               GIT_CONFIG_VALUE_0="AUTHORIZATION: basic " + encoded,
               GIT_TERMINAL_PROMPT="0")
    return env


def command(*args, timeout=300, env=None, cwd=None):
    env = child_process_environment() if env is None else env
    return subprocess.run(args, capture_output=True, text=True, encoding="utf-8",
                          errors="replace", timeout=timeout, env=env, cwd=cwd,
                          check=False)


def git_value(*args):
    env = git_network_environment() if args and args[0] == "ls-remote" else None
    result = command("git", *args, timeout=20, env=env)
    if result.returncode:
        raise Refused("Git source identity unavailable")
    return result.stdout.strip()


def validate_setup(raw):
    if not isinstance(raw, dict):
        raise Refused("Installer setup is not an object")
    body = setup_request(raw.get("case"), raw.get("source_sha"),
                         raw.get("nonce"), version=raw.get("fixture_version"),
                         binary_sha256=raw.get("binary_sha256"),
                         installer_sha256=raw.get("installer_sha256"))
    if raw != body:
        raise Refused("Installer setup has extra or changed fields")
    parent = git_value("rev-parse", "HEAD^")
    head = git_value("rev-parse", "HEAD")
    changed = git_value("diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD").splitlines()
    if (parent != body["source_sha"] or head != os.environ.get("GITHUB_SHA")
            or changed != [SETUP_PATH.as_posix()]
            or os.environ.get("GITHUB_REF") != f"refs/heads/g9-win/{body['nonce']}"):
        raise Refused("Hosted branch is not the exact candidate plus one setup")
    if git_value("remote", "get-url", "origin") not in {
            "https://github.com/ahillspace/tadx", "https://github.com/ahillspace/tadx.git"}:
        raise Refused("Hosted command source is not the fixed repository")
    if sha("scripts/install.ps1") != body["installer_sha256"]:
        raise Refused("Candidate installer source changed")
    return body


def user_path():
    import winreg
    try:
        with winreg.OpenKey(winreg.HKEY_CURRENT_USER, "Environment", 0, winreg.KEY_READ) as key:
            return winreg.QueryValueEx(key, "Path")
    except FileNotFoundError:
        return None


def restore_user_path(original):
    import winreg
    with winreg.CreateKeyEx(winreg.HKEY_CURRENT_USER, "Environment", 0, winreg.KEY_SET_VALUE) as key:
        if original is None:
            try:
                winreg.DeleteValue(key, "Path")
            except FileNotFoundError:
                pass
        else:
            winreg.SetValueEx(key, "Path", 0, original[1], original[0])


def path_count(raw, directory):
    value = "" if raw is None else raw[0]
    return sum(part.strip().rstrip("\\/").casefold() == str(directory).rstrip("\\/").casefold()
               for part in value.split(";") if part.strip())


def snapshot(binary, profile, sentinel, install_dir):
    content = profile.read_text(encoding="utf-8-sig") if profile.is_file() else ""
    return {"binary_sha256": sha(binary) if binary.is_file() else None,
            "profile_sha256": sha(profile) if profile.is_file() else None,
            "completion_markers": content.count("# tadx-installer-completion"),
            "profile_sentinel": "# unrelated fixture sentinel" in content,
            "user_path_count": path_count(user_path(), install_dir),
            "sentinel_sha256": sha(sentinel)}


def native_args(case, version, install_dir, profile):
    args = ["-InstallDir", str(install_dir), "-CompletionProfile", str(profile),
            "-Target", "codex"]
    if case == "uninstall":
        return ["-Action", "Uninstall", *args]
    selected = version + "-missing" if case == "failed-download-preserves-binary" else version
    args = ["-Action", "Install", "-Version", selected, *args]
    if case == "no-completion":
        args.append("-NoCompletion")
    if case == "no-modify-path":
        args.append("-NoModifyPath")
    return args


def sanitized_output(raw, fixture):
    text = raw[:MAX_OUTPUT].replace(str(fixture), "<fixture>")
    text = re.sub(r"[A-Za-z]:\\Users\\[^\\\s]+", "<host-user>", text, flags=re.I)
    return text


def build_assets(body, root):
    source = root / "candidate-source"
    cloned = command("git", "clone", "--local", "--no-hardlinks", "--no-checkout",
                     ".", str(source), timeout=120)
    if cloned.returncode:
        raise Refused("Separate candidate source clone was not established")
    checkout = command("git", "-C", str(source), "checkout", "--detach",
                       body["source_sha"], timeout=120)
    if checkout.returncode or git_value("-C", str(source), "rev-parse", "HEAD") != body["source_sha"]:
        raise Refused("Exact candidate commit was not checked out")
    binary = root / "candidate.exe"
    built_sha = build_candidate(source, binary, body["source_sha"],
                                body["fixture_version"], TOOLCHAIN,
                                environment=child_process_environment())
    if built_sha != body["binary_sha256"]:
        raise Refused("Final candidate Windows build differs from independently pinned binary")
    assets = root / "assets"
    assets.mkdir()
    name = f"tadx_{body['fixture_version']}_windows_amd64.zip"
    archive = assets / name
    with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_STORED) as output:
        for asset, label in ((binary, "tadx.exe"), (source / "LICENSE", "LICENSE"),
                             (source / "scripts/install.ps1", "install.ps1"),
                             (source / "scripts/install.sh", "install.sh")):
            output.write(asset, label)
    (assets / "checksums.txt").write_text(f"{sha(archive)}  {name}\n", encoding="ascii")
    return binary, assets


def remove_candidate_checkout(root):
    source = root / "candidate-source"
    if not source.exists() and not source.is_symlink():
        return True
    if (root.is_symlink() or not root.is_dir() or source.is_symlink()
            or not source.is_dir() or source.resolve().parent != root.resolve()):
        return False
    try:
        shutil.rmtree(source)
    except OSError:
        return False
    return not source.exists() and not source.is_symlink()


def child_environment(root, assets, version, fail_archive):
    allowed = {"SYSTEMROOT", "WINDIR", "COMSPEC", "PATH", "PATHEXT", "PROCESSOR_ARCHITECTURE",
               "PROCESSOR_ARCHITEW6432", "OS", "NUMBER_OF_PROCESSORS", "PYTHONIOENCODING"}
    env = {key: value for key, value in os.environ.items() if key.upper() in allowed}
    isolated = root / "home"
    if isolated.is_symlink():
        raise Refused("Disposable installer home is a link")
    isolated.mkdir(exist_ok=True)
    if not isolated.is_dir() or isolated.resolve() != (root / "home").resolve():
        raise Refused("Disposable installer home changed")
    for key in ("HOME", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "TEMP", "TMP"):
        env[key] = str(isolated)
    for key in ("CODEX_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME"):
        env[key] = str(isolated / key.lower())
    env["PATH"] = str(root / "transport") + os.pathsep + env.get("PATH", "")
    env.update(G9_ASSET_DIR=str(assets), G9_FIXTURE_VERSION=version,
               G9_NATIVE_TEMP=str(isolated), G9_FAIL_ARCHIVE="1" if fail_archive else "0",
               G9_TRANSPORT_LOG=str(root / "transport.jsonl"),
               G9_ASSET_HELPER=str(root / "transport" / "gh_fixture.py"),
               TADX_FEEDBACK_MODE="off")
    return env


def invoke(case, body, root, env):
    return command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy",
                   "Bypass", "-File", "scripts/g9/installer_wrapper.ps1",
                   *native_args(case, body["fixture_version"], root / "install-bin",
                                root / "home" / "profile.ps1"),
                   timeout=300, env=env)


def prepare(body, root):
    binary, assets = build_assets(body, root)
    transport = root / "transport"
    transport.mkdir()
    shutil.copyfile(Path(__file__).with_name("gh_fixture.py"), transport / "gh_fixture.py")
    (transport / "gh.cmd").write_text('@echo off\r\npython "%~dp0gh_fixture.py" %*\r\n', encoding="ascii")
    env = child_environment(root, assets, body["fixture_version"], False)
    home = root / "home"
    install_dir = root / "install-bin"
    profile = home / "profile.ps1"
    profile.write_text("# unrelated fixture sentinel\n", encoding="utf-8")
    sentinel = root / "sentinel.txt"
    sentinel.write_text("protected fixture\n", encoding="utf-8")
    transport_log = root / "transport.jsonl"
    transport_log.write_text("", encoding="ascii")
    if body["case"] in {"idempotent", "uninstall", "failed-download-preserves-binary",
                        "completion-opt-in"}:
        setup_case = "no-completion" if body["case"] == "completion-opt-in" else "fresh"
        setup = invoke(setup_case, body, root, env)
        if setup.returncode:
            raise Refused("Native fixture installation did not establish its starting state")
    before = snapshot(install_dir / "tadx.exe", profile, sentinel, install_dir)
    transport_log.write_text("", encoding="ascii")
    return before


def fixture_root(body):
    runner_temp = Path(os.environ["RUNNER_TEMP"]).resolve(strict=True)
    root = runner_temp / "g9-installer" / body["nonce"]
    return runner_temp, root


def write_json(path, body):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(body, sort_keys=True, separators=(",", ":")) + "\n",
                    encoding="utf-8")


def prepare_main():
    if SETUP_PATH.stat().st_size > 4096:
        raise Refused("Hosted setup request exceeds its bounded contract")
    body = validate_setup(strict_json(SETUP_PATH.read_text(encoding="utf-8")))
    runner_temp, root = fixture_root(body)
    if root.exists() or root.is_symlink():
        raise Refused("Disposable fixture path already exists")
    original_path = user_path()
    try:
        root.mkdir(parents=True)
        before = prepare(body, root)
        baseline = {"protocol": "tadx-windows-installer-baseline/1",
                    "case": body["case"], "nonce": body["nonce"],
                    "source_sha": body["source_sha"], "setup_sha256": body["setup_sha256"],
                    "setup_commit": os.environ["GITHUB_SHA"],
                    "run_id": int(os.environ["GITHUB_RUN_ID"]),
                    "binary_sha256": body["binary_sha256"],
                    "installer_sha256": body["installer_sha256"],
                    "before": before, "pre_task_observed": True}
        state = {"setup": body, "setup_commit": os.environ["GITHUB_SHA"],
                 "run_id": baseline["run_id"], "original_user_path": original_path}
        write_json(root / "private-state.json", state)
        write_json(Path(os.environ["G9_BASELINE_PATH"]), baseline)
    except BaseException as failure:
        restored = False
        checkout_removed = False
        try:
            restore_user_path(original_path)
            restored = user_path() == original_path
        except OSError:
            restored = False
        if restored:
            try:
                checkout_removed = remove_candidate_checkout(root)
            except (OSError, Refused, subprocess.TimeoutExpired):
                checkout_removed = False
            if checkout_removed and root.is_dir() and not root.is_symlink():
                try:
                    shutil.rmtree(root)
                except OSError:
                    pass
        retained = root.exists() or root.is_symlink()
        cleanup_ok = restored and checkout_removed and not retained
        write_json(Path(os.environ["G9_SETUP_FAILURE_PATH"]), {
            "protocol": "tadx-windows-installer-setup-failure/1",
            "case": body["case"], "nonce": body["nonce"],
            "source_sha": body["source_sha"], "setup_sha256": body["setup_sha256"],
            "setup_commit": os.environ["GITHUB_SHA"],
            "run_id": int(os.environ["GITHUB_RUN_ID"]),
            "failure_type": type(failure).__name__,
            "user_path_restored": restored,
            "candidate_checkout_removed": checkout_removed,
            "fixture_retained": retained,
            "cleanup_ok": cleanup_ok})
        raise Refused("Hosted setup failed; bounded native cleanup evidence was recorded") from None
    return 0


def wait_for_command(body, setup_commit, baseline_sha, *, timeout=38 * 60):
    branch = "refs/heads/g9-win/" + body["nonce"]
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = git_value("ls-remote", "origin", branch)
        head = value.split()[0] if value else None
        if head is None:
            raise Refused("Disposable branch disappeared before native command")
        if head != setup_commit:
            fetched_result = command("git", "fetch", "--no-tags", "origin", branch,
                                     timeout=120, env=git_network_environment())
            if fetched_result.returncode:
                raise Refused("Native command child commit could not be fetched")
            fetched = git_value("rev-parse", "FETCH_HEAD")
            if fetched != head or git_value("rev-parse", fetched + "^") != setup_commit:
                raise Refused("Native command is not one child of setup")
            changed = git_value("diff-tree", "--no-commit-id", "--name-only", "-r", fetched)
            if changed != COMMAND_PATH.as_posix():
                raise Refused("Native command commit changed unrelated source")
            size = git_value("cat-file", "-s", fetched + ":" + COMMAND_PATH.as_posix())
            if not size.isdecimal() or int(size) > 4096:
                raise Refused("Native command child exceeds its bounded contract")
            raw = strict_json(git_value("show", fetched + ":" + COMMAND_PATH.as_posix()))
            if not isinstance(raw, dict):
                raise Refused("Native command child is not a JSON object")
            if raw.get("protocol") == "tadx-windows-installer-stop/1":
                expected = stop_request(body, setup_commit, baseline_sha,
                                        int(os.environ["GITHUB_RUN_ID"]))
                if raw != expected:
                    raise Refused("Unused-session stop is not bound to this run")
                return fetched, None
            expected = command_request(body, raw.get("argv"), setup_commit, baseline_sha)
            if raw != expected:
                raise Refused("Native command is not the fixed audited request")
            return fetched, expected
        time.sleep(10)
    raise Refused("No bounded model-selected native command arrived")


def execute_main():
    if SETUP_PATH.stat().st_size > 4096:
        raise Refused("Hosted setup request exceeds its bounded contract")
    body = validate_setup(strict_json(SETUP_PATH.read_text(encoding="utf-8")))
    runner_temp, root = fixture_root(body)
    if root.is_symlink() or not root.is_dir() or root.parent.resolve() != (runner_temp / "g9-installer").resolve():
        raise Refused("Prepared fixture is not test-owned")
    state = json.loads((root / "private-state.json").read_text(encoding="utf-8"))
    baseline_path = Path(os.environ["G9_BASELINE_PATH"]).resolve(strict=True)
    baseline = json.loads(baseline_path.read_text(encoding="utf-8"))
    baseline_sha = sha(baseline_path)
    setup_commit = os.environ["GITHUB_SHA"]
    run = {"id": int(os.environ["GITHUB_RUN_ID"]), "head_sha": setup_commit,
           "head_branch": "g9-win/" + body["nonce"], "event": "push",
           "path": ".github/workflows/g9-windows-installer.yml"}
    original_path = state["original_user_path"]
    result = {"protocol": "tadx-windows-installer-result/1", "case": body["case"],
              "nonce": body["nonce"], "setup_commit": setup_commit,
              "setup_sha256": body["setup_sha256"], "baseline_sha256": baseline_sha,
              "run_id": run["id"], "binary_sha256": body["binary_sha256"],
              "installer_sha256": body["installer_sha256"], "before": baseline["before"]}
    try:
        if (state.get("setup") != body or state.get("setup_commit") != setup_commit
                or state.get("run_id") != run["id"] or baseline_issues(body, setup_commit, run, baseline)):
            raise Refused("Prepared fixture or immutable baseline identity differs")
        commit, requested = wait_for_command(body, setup_commit, baseline_sha)
        result["command_commit"] = commit
        if requested is None:
            result["stop_requested"] = True
            install_dir = root / "install-bin"
            current = snapshot(install_dir / "tadx.exe", root / "home" / "profile.ps1",
                               root / "sentinel.txt", install_dir)
            result["fresh_before_stop"] = current == baseline["before"]
            raise Refused("Unused hosted session stopped before native command")
        result.update(command_commit=commit, command_sha256=requested["command_sha256"],
                      argv=requested["argv"])
        install_dir = root / "install-bin"
        current = snapshot(install_dir / "tadx.exe", root / "home" / "profile.ps1",
                           root / "sentinel.txt", install_dir)
        result["fresh_before_command"] = current == baseline["before"]
        if not result["fresh_before_command"]:
            raise Refused("Prepared starting state changed before model command")
        env = child_environment(root, root / "assets", body["fixture_version"],
                                body["case"] == "failed-download-preserves-binary")
        native = invoke(body["case"], body, root, env)
        after = snapshot(install_dir / "tadx.exe", root / "home" / "profile.ps1",
                         root / "sentinel.txt", install_dir)
        transport = [json.loads(line) for line in (root / "transport.jsonl").read_text(
            encoding="ascii").splitlines()]
        result.update(native_executed=True, native_exit_status=native.returncode,
                      native_stdout=sanitized_output(native.stdout, root),
                      native_stderr=sanitized_output(native.stderr, root), after=after,
                      transport=transport,
                      observer_issues=observed_issues(body["case"], baseline["before"], after,
                                                     native.returncode, body["binary_sha256"],
                                                     transport, body["fixture_version"]))
    except (Refused, OSError, ValueError, subprocess.TimeoutExpired) as exc:
        result.update(native_executed=False, observer_issues=["Fixture blocked: " + type(exc).__name__])
    finally:
        try:
            restore_user_path(original_path)
            result["cleanup_ok"] = user_path() == tuple(original_path) if original_path is not None else user_path() is None
        except OSError:
            result["cleanup_ok"] = False
        if not remove_candidate_checkout(root):
            result["cleanup_ok"] = False
        if root.is_dir() and not root.is_symlink():
            try:
                shutil.rmtree(root)
            except OSError:
                result["cleanup_ok"] = False
    write_json(Path(os.environ["G9_RESULT_PATH"]), result)
    return 0 if result.get("native_executed") and result.get("cleanup_ok") and not result.get("observer_issues") else 1


def main(argv):
    if argv == ["prepare"]:
        return prepare_main()
    if argv == ["execute"]:
        return execute_main()
    raise Refused("Only prepare and execute phases are supported")


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
