import unittest
from pathlib import Path
import sys
import json
import os
import shutil
import tempfile
import hashlib
from types import SimpleNamespace
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/g9"))
sys.path.insert(0, str(ROOT / "scripts/refactor/harness"))

from windows_gate import (CASES, Refused, argv_for, baseline_issues, command_request,
                          observed_issues, result_issues, setup_request, stop_request)
import gh_fixture
import windows_worker
import host_relay
import windows_preparer
import windows_broker_patches
import subprocess


SHA = "a" * 40
VERSION = "0.1.3-g9"


def selected(case):
    return setup_request(case, SHA, "nonce_123456789", version=VERSION,
                         binary_sha256="b" * 64, installer_sha256="c" * 64)


class ContractTest(unittest.TestCase):
    def test_all_fixed_cases_build(self):
        for case in CASES:
            with self.subTest(case=case):
                body = selected(case)
                self.assertEqual(body["case"], case)
                self.assertEqual(len(body["setup_sha256"]), 64)

    def test_completion_opt_in_has_a_distinct_existing_installation(self):
        before = {"binary_sha256": "b" * 64, "completion_markers": 0,
                  "user_path_count": 1, "profile_sentinel": True,
                  "sentinel_sha256": "c" * 64, "profile_sha256": "d" * 64}
        after = {**before, "completion_markers": 1, "profile_sha256": "e" * 64}
        setup = selected("completion-opt-in")
        baseline = {"protocol": "tadx-windows-installer-baseline/1",
                    "case": setup["case"], "nonce": setup["nonce"],
                    "source_sha": SHA, "setup_sha256": setup["setup_sha256"],
                    "setup_commit": "f" * 40, "run_id": 17,
                    "binary_sha256": "b" * 64, "installer_sha256": "c" * 64,
                    "before": before, "pre_task_observed": True}
        run = {"id": 17, "head_sha": "f" * 40,
               "head_branch": "g9-win/" + setup["nonce"], "event": "push",
               "path": ".github/workflows/g9-windows-installer.yml"}
        self.assertEqual(baseline_issues(setup, "f" * 40, run, baseline), [])
        transport = [{"asset": "checksums.txt", "failed": False},
                     {"asset": f"tadx_{VERSION}_windows_amd64.zip", "failed": False}]
        self.assertEqual(observed_issues("completion-opt-in", before, after, 0,
                                         "b" * 64, transport, VERSION), [])
        self.assertTrue(observed_issues("completion-opt-in", before,
                                        {**after, "completion_markers": 0}, 0,
                                        "b" * 64, transport, VERSION))
        for invalid_after in ({**after, "profile_sha256": before["profile_sha256"]},
                              {**after, "binary_sha256": "f" * 64},
                              {**after, "user_path_count": 0}):
            self.assertTrue(observed_issues("completion-opt-in", before, invalid_after,
                                            0, "b" * 64, transport, VERSION))

    def test_completion_opt_in_prepares_native_no_completion_starting_state(self):
        setup = selected("completion-opt-in")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "fixture"
            root.mkdir()
            (root / "home").mkdir()
            with patch.object(windows_worker, "build_assets",
                              return_value=(root / "binary", root / "assets")), \
                 patch.object(windows_worker, "child_environment", return_value={}), \
                 patch.object(windows_worker, "invoke",
                              return_value=SimpleNamespace(returncode=0)) as invoked, \
                 patch.object(windows_worker, "snapshot", return_value={}) as observed:
                windows_worker.prepare(setup, root)
            self.assertEqual(invoked.call_args.args[0], "no-completion")
            observed.assert_called_once()

    def test_unknown_or_extra_argv_refused(self):
        for case, argv in (("other", ["install"]), ("fresh", ["install", "--version", VERSION, "-Command", "whoami"]), ("uninstall", ["uninstall", "--force"])):
            with self.subTest(case=case, argv=argv), self.assertRaises(Refused):
                command_request(selected("fresh" if case == "other" else case), argv,
                                "d" * 40, "e" * 64)

    def test_invalid_revision_or_nonce_refused(self):
        for sha, nonce in (("bad", "nonce_123456789"), ("a" * 64, "nonce_123456789"),
                           (SHA, "../../other")):
            with self.assertRaises(Refused):
                setup_request("fresh", sha, nonce, version=VERSION,
                              binary_sha256="b" * 64, installer_sha256="c" * 64)
        with self.assertRaises(Refused):
            command_request(selected("fresh"), list(argv_for("fresh", VERSION)),
                            "d" * 64, "e" * 64)

    def test_evidence_requires_native_identity_and_cleanup(self):
        body = selected("fresh")
        run = {"id": 17, "head_sha": "d" * 40, "event": "push", "conclusion": "success",
               "head_branch": "g9-win/nonce_123456789",
               "path": ".github/workflows/g9-windows-installer.yml"}
        before = {"binary_sha256": None, "completion_markers": 0, "user_path_count": 0,
                  "profile_sentinel": True, "sentinel_sha256": "e" * 64}
        after = {**before, "binary_sha256": "b" * 64,
                 "completion_markers": 1, "user_path_count": 1}
        transport = [{"asset": "checksums.txt", "failed": False},
                     {"asset": f"tadx_{VERSION}_windows_amd64.zip", "failed": False}]
        baseline = {"protocol": "tadx-windows-installer-baseline/1", "case": "fresh",
                    "nonce": body["nonce"], "source_sha": SHA, "setup_sha256": body["setup_sha256"],
                    "setup_commit": "d" * 40, "run_id": 17, "binary_sha256": "b" * 64,
                    "installer_sha256": "c" * 64, "before": before, "pre_task_observed": True}
        model_command = command_request(body, list(argv_for("fresh", VERSION)), "d" * 40, "f" * 64)
        result = {"protocol": "tadx-windows-installer-result/1", "case": "fresh",
                  "nonce": body["nonce"], "setup_sha256": body["setup_sha256"],
                  "command_sha256": model_command["command_sha256"],
                  "baseline_sha256": "f" * 64, "run_id": 17, "native_executed": True,
                  "cleanup_ok": True, "setup_commit": "d" * 40,
                  "command_commit": "1" * 40, "native_exit_status": 0,
                  "binary_sha256": "b" * 64, "installer_sha256": "c" * 64,
                  "argv": model_command["argv"], "fresh_before_command": True,
                  "before": before, "after": after, "transport": transport, "observer_issues": []}
        self.assertEqual(baseline_issues(body, "d" * 40, run, baseline), [])
        self.assertEqual(result_issues(body, "d" * 40, model_command, "1" * 40,
                                       baseline, "f" * 64, run, result), [])
        for changed in ({"run_id": 18}, {"native_executed": False}, {"cleanup_ok": False},
                        {"observer_issues": ["binary differs"]}, {"after": None},
                        {"fresh_before_command": False}, {"baseline_sha256": "2" * 64}):
            with self.subTest(changed=changed):
                self.assertTrue(result_issues(body, "d" * 40, model_command, "1" * 40,
                                              baseline, "f" * 64, run, {**result, **changed}))
        self.assertTrue(baseline_issues(body, "d" * 40, run,
                                        {**baseline, "pre_task_observed": False}))
        self.assertTrue(result_issues(body, "d" * 40, model_command, "1" * 40,
                                       baseline, "f" * 64, {**run, "head_sha": "e" * 40}, result))

    def test_case_specific_observer_does_not_accept_invented_success(self):
        before = {"binary_sha256": None, "completion_markers": 0, "user_path_count": 0,
                  "profile_sentinel": True, "sentinel_sha256": "e" * 64}
        after = {**before, "binary_sha256": "b" * 64,
                 "completion_markers": 1, "user_path_count": 1}
        transport = [{"asset": "checksums.txt", "failed": False},
                     {"asset": f"tadx_{VERSION}_windows_amd64.zip", "failed": False}]
        self.assertEqual(observed_issues("fresh", before, after, 0, "b" * 64,
                                         transport, VERSION), [])
        self.assertTrue(observed_issues("fresh", before, {**after, "binary_sha256": "c" * 64},
                                        0, "b" * 64, transport, VERSION))
        installed = after
        self.assertTrue(observed_issues("failed-download-preserves-binary", installed, installed,
                                        1, "b" * 64, [], VERSION))
        failure = [{"asset": "checksums.txt", "failed": False},
                   {"asset": f"tadx_{VERSION}-missing_windows_amd64.zip", "failed": True},
                   {"asset": f"tadx_{VERSION}-missing_windows_amd64.zip", "failed": True,
                    "transport": "https"}]
        self.assertEqual(observed_issues("failed-download-preserves-binary", installed, installed,
                                         1, "b" * 64, failure, VERSION), [])

    def test_request_branch_must_be_exact_one_file_child(self):
        with self.assertRaises(Refused):
            windows_worker.strict_json('{"case":"fresh","case":"uninstall"}')
        body = selected("fresh")
        parent, head = SHA, "d" * 40
        values = {("rev-parse", "HEAD^"): parent, ("rev-parse", "HEAD"): head,
                  ("remote", "get-url", "origin"): "https://github.com/ahillspace/tadx.git",
                  ("diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"):
                      ".g9-windows-installer-setup.json"}
        env = {"GITHUB_SHA": head, "GITHUB_REF": "refs/heads/g9-win/nonce_123456789"}
        with patch.dict(os.environ, env), patch.object(windows_worker, "git_value",
            side_effect=lambda *args: values[args]), patch.object(windows_worker, "sha", return_value="c" * 64):
            self.assertEqual(windows_worker.validate_setup(body), body)
            values[("diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")] += "\nscripts/install.ps1"
            with self.assertRaises(Refused):
                windows_worker.validate_setup(body)
            values[("diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")] = ".g9-windows-installer-setup.json"
            values[("remote", "get-url", "origin")] = "https://github.com/unrelated/project.git"
            with self.assertRaises(Refused):
                windows_worker.validate_setup(body)

    def test_fixture_transport_rejects_unknown_command_and_records_exact_asset(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            temporary = root / "home"
            assets = root / "assets"
            temporary.mkdir()
            assets.mkdir()
            output = temporary / "checksums.txt"
            (assets / "checksums.txt").write_text("fixture checksum\n")
            log = root / "transport.jsonl"
            log.write_text("")
            env = {"G9_FIXTURE_VERSION": VERSION, "G9_ASSET_DIR": str(assets),
                   "G9_NATIVE_TEMP": str(temporary), "G9_TRANSPORT_LOG": str(log),
                   "G9_FAIL_ARCHIVE": "0"}
            with patch.dict(os.environ, env):
                args = ["release", "download", VERSION, "--repo", "ahillspace/tadx",
                        "--pattern", "checksums.txt", "--output", str(output)]
                self.assertEqual(gh_fixture.main(args), 0)
                self.assertEqual(output.read_text(), "fixture checksum\n")
                self.assertEqual(json.loads(log.read_text().splitlines()[0])["asset"], "checksums.txt")
                missing = f"tadx_{VERSION}-missing_windows_amd64.zip"
                os.environ["G9_FAIL_ARCHIVE"] = "1"
                failed_args = ["release", "download", VERSION + "-missing", "--repo", "ahillspace/tadx",
                               "--pattern", missing, "--output", str(temporary / missing)]
                self.assertEqual(gh_fixture.main(failed_args), 1)
                self.assertEqual(json.loads(log.read_text().splitlines()[-1]),
                                 {"asset": missing, "failed": True})
                self.assertNotEqual(gh_fixture.main(args[:-2] + ["--output", str(root / "escape")]), 0)
                self.assertNotEqual(gh_fixture.main(["api", "user"]), 0)

    def test_https_fallback_uses_only_exact_local_assets_or_recorded_fault(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            home, assets = root / "home", root / "assets"
            home.mkdir()
            assets.mkdir()
            (assets / "checksums.txt").write_text("fixture checksum\n", encoding="ascii")
            log = root / "transport.jsonl"
            log.write_text("", encoding="ascii")
            env = {"G9_FIXTURE_VERSION": VERSION, "G9_ASSET_DIR": str(assets),
                   "G9_NATIVE_TEMP": str(home), "G9_TRANSPORT_LOG": str(log),
                   "G9_FAIL_ARCHIVE": "1"}
            base = "https://github.com/ahillspace/tadx/releases/download/"
            missing = f"tadx_{VERSION}-missing_windows_amd64.zip"
            with patch.dict(os.environ, env):
                self.assertEqual(gh_fixture.main(["https", base + VERSION + "/checksums.txt",
                                                  str(home / "checksums.txt")]), 0)
                self.assertEqual((home / "checksums.txt").read_text(), "fixture checksum\n")
                self.assertEqual(gh_fixture.main(["https", base + VERSION + "-missing/" + missing,
                                                  str(home / missing)]), 1)
                self.assertFalse((home / missing).exists())
                rows = [json.loads(line) for line in log.read_text().splitlines()]
                self.assertEqual(rows[-1], {"asset": missing, "failed": True,
                                            "transport": "https"})
                for uri in ("https://example.com/" + missing,
                            base + VERSION + "/checksums.txt?token=bad",
                            base + VERSION + "/%2e%2e/checksums.txt"):
                    self.assertEqual(gh_fixture.main(["https", uri,
                                                      str(home / "rejected.txt")]), 2)
                self.assertEqual(len(log.read_text().splitlines()), 2)

    def test_wrapper_function_is_visible_to_a_synthetic_child_script(self):
        shell = shutil.which("powershell")
        if shell is None:
            self.skipTest("Native Windows PowerShell is unavailable")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            scripts = root / "scripts"
            (scripts / "g9").mkdir(parents=True)
            wrapper = scripts / "g9" / "installer_wrapper.ps1"
            shutil.copyfile(ROOT / "scripts/g9/installer_wrapper.ps1",
                            wrapper)
            (scripts / "install.ps1").write_text(
                "$bound = Get-Command Invoke-WebRequest -ErrorAction Stop\n"
                "if ($bound.CommandType -ne 'Function') { throw 'transport not local' }\n"
                "if ($bound.ScriptBlock.ToString() -notlike '*G9_ASSET_HELPER*') { throw 'wrong function' }\n"
                "Write-Output 'fixture-bound'\n", encoding="utf-8")
            env = {key: value for key, value in os.environ.items()
                   if key.upper() in {"SYSTEMROOT", "WINDIR", "COMSPEC", "PATH", "PATHEXT"}}
            env.update(HOME=str(root), USERPROFILE=str(root), APPDATA=str(root),
                       LOCALAPPDATA=str(root), TEMP=str(root), TMP=str(root))
            result = subprocess.run([shell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy",
                                     "Bypass", "-File", str(wrapper)],
                                    env=env, capture_output=True, text=True, timeout=60)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("fixture-bound", result.stdout)

    def test_native_powershell_argv_is_fixed_and_test_owned(self):
        root = Path("C:/fixture-owned")
        install_dir = root / "bin"
        profile = root / "profile.ps1"
        normal = windows_worker.native_args("fresh", VERSION, install_dir, profile)
        self.assertEqual(normal[:4], ["-Action", "Install", "-Version", VERSION])
        self.assertEqual(normal[-2:], ["-Target", "codex"])
        self.assertEqual(windows_worker.native_args("no-completion", VERSION, install_dir, profile)[-1],
                         "-NoCompletion")
        self.assertEqual(windows_worker.native_args("no-modify-path", VERSION, install_dir, profile)[-1],
                         "-NoModifyPath")
        self.assertEqual(windows_worker.native_args("uninstall", VERSION, install_dir, profile)[:2],
                         ["-Action", "Uninstall"])

    def test_child_environment_carries_no_inherited_credentials(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            inherited = {"OPENAI_API_KEY": "secret", "GH_TOKEN": "secret", "TADX_PAT_SECRET": "secret",
                         "PATH": "C:/fixture-path", "SystemRoot": "C:/Windows"}
            with patch.dict(os.environ, inherited):
                env = windows_worker.child_environment(root, root / "assets", VERSION, False)
            self.assertNotIn("OPENAI_API_KEY", env)
            self.assertNotIn("GH_TOKEN", env)
            self.assertNotIn("TADX_PAT_SECRET", env)
            self.assertEqual(env["USERPROFILE"], str(root / "home"))
            self.assertEqual(env["CODEX_HOME"], str(root / "home" / "codex_home"))
            self.assertEqual(windows_worker.child_environment(root, root / "assets", VERSION,
                                                               True)["G9_FAIL_ARCHIVE"], "1")
            self.assertTrue((root / "home").is_dir())

    def test_github_token_is_only_available_to_fixed_git_network_child(self):
        with patch.dict(os.environ, {"G9_GITHUB_TOKEN": "synthetic-token",
                                     "GIT_CONFIG_COUNT": "9",
                                     "GIT_CONFIG_KEY_0": "unsafe"}):
            def child_result(args, **kwargs):
                value = "https://github.com/ahillspace/tadx.git" if args[:3] == (
                    "git", "remote", "get-url") else ""
                return SimpleNamespace(returncode=0, stdout=value)
            with patch.object(windows_worker.subprocess, "run",
                              side_effect=child_result) as child:
                windows_worker.command("go", "version")
                ordinary = child.call_args.kwargs["env"]
                self.assertNotIn("G9_GITHUB_TOKEN", ordinary)
                self.assertNotIn("GIT_CONFIG_COUNT", ordinary)
                windows_worker.git_value("ls-remote", "origin", "refs/heads/fixture")
                network = child.call_args.kwargs["env"]
                self.assertNotIn("G9_GITHUB_TOKEN", network)
                self.assertEqual(network["GIT_CONFIG_COUNT"], "1")
                self.assertEqual(network["GIT_CONFIG_KEY_0"],
                                 "http.https://github.com/.extraheader")
                self.assertNotIn("synthetic-token", network["GIT_CONFIG_VALUE_0"])
            with tempfile.TemporaryDirectory() as directory:
                native = windows_worker.child_environment(Path(directory), Path(directory) / "assets",
                                                          VERSION, False)
                self.assertNotIn("G9_GITHUB_TOKEN", native)
                self.assertFalse(any(key.startswith("GIT_CONFIG_") for key in native))
        with patch.dict(os.environ, {"G9_GITHUB_TOKEN": ""}):
            with self.assertRaises(Refused):
                windows_worker.git_network_environment()

    def test_run_selection_requires_exact_branch_commit_and_unique_push(self):
        row = {"databaseId": 41, "headSha": "d" * 40,
               "headBranch": "g9-win/nonce_123456789", "event": "push"}
        self.assertEqual(host_relay.choose_run([row], row["headBranch"], row["headSha"]), row)
        self.assertIsNone(host_relay.choose_run([{**row, "headSha": "e" * 40}],
                                                row["headBranch"], row["headSha"]))
        with self.assertRaises(Refused):
            host_relay.choose_run([row, {**row, "databaseId": 42}],
                                  row["headBranch"], row["headSha"])

    def test_disposable_branch_cleanup_uses_an_exact_git_lease(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            remote = root / "remote.git"
            local = root / "local"
            def git(*args, cwd=None):
                result = subprocess.run(["git", *args], cwd=cwd, capture_output=True,
                                        text=True, timeout=15)
                self.assertEqual(result.returncode, 0, result.stderr)
                return result.stdout.strip()
            git("init", "--bare", str(remote))
            git("init", str(local))
            git("-C", str(local), "config", "user.name", "Fixture")
            git("-C", str(local), "config", "user.email", "fixture@invalid.example")
            git("-C", str(local), "remote", "add", "origin", str(remote))
            branch = "g9-win/fixture"
            (local / "marker").write_text("first", encoding="utf-8")
            git("-C", str(local), "add", "marker")
            git("-C", str(local), "commit", "-m", "first")
            first = git("-C", str(local), "rev-parse", "HEAD")
            git("-C", str(local), "push", "origin", "HEAD:refs/heads/" + branch)
            (local / "marker").write_text("second", encoding="utf-8")
            git("-C", str(local), "commit", "-am", "second")
            second = git("-C", str(local), "rev-parse", "HEAD")
            git("-C", str(local), "push", "origin", "HEAD:refs/heads/" + branch)
            with patch.object(host_relay, "remote_head", return_value=first):
                self.assertFalse(host_relay.remove_exact_branch(local, branch, first))
            self.assertEqual(host_relay.remote_head(local, branch), second)
            self.assertTrue(host_relay.remove_exact_branch(local, branch, second))
            self.assertIsNone(host_relay.remote_head(local, branch))

    def test_unused_hosted_job_needs_native_rollback_evidence(self):
        setup = selected("fresh")
        before = {"binary_sha256": None, "completion_markers": 0,
                  "user_path_count": 0, "profile_sentinel": True,
                  "sentinel_sha256": "c" * 64}
        baseline = {"protocol": "tadx-windows-installer-baseline/1",
                    "case": "fresh", "nonce": setup["nonce"], "source_sha": SHA,
                    "setup_sha256": setup["setup_sha256"], "setup_commit": "d" * 40,
                    "run_id": 17, "binary_sha256": "b" * 64,
                    "installer_sha256": "c" * 64, "before": before,
                    "pre_task_observed": True}
        session = {"area": "fixture-owned-area", "branch": "g9-win/" + setup["nonce"],
                   "setup": setup, "setup_commit": "d" * 40,
                   "baseline": baseline, "baseline_sha256": "e" * 64,
                   "run_id": 17}
        run = {"id": 17, "head_sha": "d" * 40, "head_branch": session["branch"],
               "event": "push", "path": ".github/workflows/g9-windows-installer.yml", "status": "completed",
               "conclusion": "failure"}
        result = {"protocol": "tadx-windows-installer-result/1",
                  "case": "fresh", "nonce": setup["nonce"],
                  "setup_commit": "d" * 40, "setup_sha256": setup["setup_sha256"],
                  "baseline_sha256": "e" * 64, "run_id": 17,
                  "binary_sha256": "b" * 64, "installer_sha256": "c" * 64,
                  "before": before, "command_commit": "f" * 40,
                  "stop_requested": True, "fresh_before_stop": True,
                  "native_executed": False, "cleanup_ok": True}
        with patch.object(host_relay, "remote_head", side_effect=["d" * 40, "f" * 40] * 5), \
             patch.object(host_relay, "sha", return_value="e" * 64), \
             patch.object(host_relay, "run_metadata", return_value=run), \
             patch.object(host_relay, "one_file_commit", return_value="f" * 40), \
             patch.object(host_relay, "checked"), \
             patch.object(host_relay, "remove_exact_branch", return_value=True) as branch_removed, \
             patch.object(host_relay, "wait_for_completion", return_value=run), \
             patch.object(host_relay, "download_artifact", return_value=True), \
             patch.object(host_relay, "read_artifact", return_value=(result, "f" * 64)) as artifact, \
             patch.object(host_relay, "remove_area") as removed:
            self.assertEqual(host_relay.abort_hosted(session),
                             {"branch_cleanup": "verified", "native_cleanup": "verified"})
            removed.assert_called_once_with(session["area"])
            for changed in ({"cleanup_ok": False}, {"command_commit": "0" * 40},
                            {"stop_requested": False}, {"fresh_before_stop": False}):
                artifact.return_value = ({**result, **changed}, "f" * 64)
                removed.reset_mock()
                branch_removed.reset_mock()
                with self.subTest(changed=changed), self.assertRaises(Refused):
                    host_relay.abort_hosted(session)
                removed.assert_not_called()
                branch_removed.assert_not_called()

    def test_unverified_command_job_retains_branch_and_artifacts(self):
        setup = selected("fresh")
        session = {"area": "fixture-owned-area", "branch": "g9-win/" + setup["nonce"],
                   "setup": setup, "setup_commit": "d" * 40,
                   "baseline_sha256": "e" * 64, "run_id": 17,
                   "baseline": {"before": {}}}
        with patch.object(host_relay, "remote_head", side_effect=["d" * 40, "f" * 40]), \
             patch.object(host_relay, "sha", return_value="e" * 64), \
             patch.object(host_relay, "run_metadata", return_value={}), \
             patch.object(host_relay, "baseline_issues", return_value=[]), \
             patch.object(host_relay, "one_file_commit", return_value="f" * 40), \
             patch.object(host_relay, "checked"), \
             patch.object(host_relay, "wait_for_completion", side_effect=Refused("no result")), \
             patch.object(host_relay, "command") as canceled, \
             patch.object(host_relay, "remove_exact_branch") as removed, \
             patch.object(host_relay, "remove_area") as erased:
            with self.assertRaises(Refused):
                host_relay.execute_hosted(session, list(argv_for("fresh", VERSION)))
            canceled.assert_not_called()
            removed.assert_not_called()
            erased.assert_not_called()

    def test_failed_or_stale_native_cleanup_never_deletes_branch(self):
        setup = selected("fresh")
        session = {"area": "fixture-owned-area", "branch": "g9-win/" + setup["nonce"],
                   "setup": setup, "setup_commit": "d" * 40,
                   "baseline_sha256": "e" * 64, "run_id": 17,
                   "baseline": {"before": {}}}
        for result in ({"cleanup_ok": False},
                       {"cleanup_ok": True, "setup_commit": "0" * 40}):
            with self.subTest(result=result), \
                 patch.object(host_relay, "remote_head", side_effect=["d" * 40, "f" * 40, "f" * 40]), \
                 patch.object(host_relay, "sha", return_value="e" * 64), \
                 patch.object(host_relay, "run_metadata", return_value={}), \
                 patch.object(host_relay, "baseline_issues", return_value=[]), \
                 patch.object(host_relay, "one_file_commit", return_value="f" * 40), \
                 patch.object(host_relay, "checked"), \
                 patch.object(host_relay, "wait_for_completion", return_value={"id": 17}), \
                 patch.object(host_relay, "download_artifact", return_value=True), \
                 patch.object(host_relay, "read_artifact", return_value=(result, "a" * 64)), \
                 patch.object(host_relay, "result_issues", return_value=["native cleanup unverified"]), \
                 patch.object(host_relay, "remove_exact_branch", return_value=True) as removed, \
                 patch.object(host_relay, "remove_area") as erased:
                with self.assertRaises(Refused):
                    host_relay.execute_hosted(session, list(argv_for("fresh", VERSION)))
                removed.assert_not_called()
                erased.assert_not_called()

    def test_unused_session_waits_for_cleanup_proof_before_branch_deletion(self):
        setup = selected("fresh")
        before = {"binary_sha256": None, "completion_markers": 0,
                  "user_path_count": 0, "profile_sentinel": True,
                  "sentinel_sha256": "c" * 64}
        baseline = {"protocol": "tadx-windows-installer-baseline/1",
                    "case": "fresh", "nonce": setup["nonce"], "source_sha": SHA,
                    "setup_sha256": setup["setup_sha256"], "setup_commit": "d" * 40,
                    "run_id": 17, "binary_sha256": "b" * 64,
                    "installer_sha256": "c" * 64, "before": before,
                    "pre_task_observed": True}
        session = {"area": "fixture-owned-area", "branch": "g9-win/" + setup["nonce"],
                   "setup": setup, "setup_commit": "d" * 40,
                   "baseline": baseline, "baseline_sha256": "e" * 64,
                   "run_id": 17}
        run = {"id": 17, "head_sha": "d" * 40, "head_branch": session["branch"],
               "event": "push", "path": ".github/workflows/g9-windows-installer.yml",
               "status": "completed", "conclusion": "failure"}
        result = {"protocol": "tadx-windows-installer-result/1",
                  "case": "fresh", "nonce": setup["nonce"],
                  "setup_commit": "d" * 40, "setup_sha256": setup["setup_sha256"],
                  "baseline_sha256": "e" * 64, "run_id": 17,
                  "binary_sha256": "b" * 64, "installer_sha256": "c" * 64,
                  "before": before, "command_commit": "f" * 40,
                  "stop_requested": True, "fresh_before_stop": True,
                  "native_executed": False, "cleanup_ok": True}
        events = []
        def removed(*_):
            events.append("branch_deleted")
            return True
        def observed(*_):
            events.append("native_observed")
            return result, "f" * 64
        with patch.object(host_relay, "remote_head", side_effect=["d" * 40, "f" * 40]), \
             patch.object(host_relay, "sha", return_value="e" * 64), \
             patch.object(host_relay, "run_metadata", return_value=run), \
             patch.object(host_relay, "one_file_commit", return_value="f" * 40), \
             patch.object(host_relay, "checked"), \
             patch.object(host_relay, "remove_exact_branch", side_effect=removed), \
             patch.object(host_relay, "wait_for_completion", return_value=run), \
             patch.object(host_relay, "download_artifact", return_value=True), \
             patch.object(host_relay, "read_artifact", side_effect=observed), \
             patch.object(host_relay, "remove_area"):
            host_relay.abort_hosted(session)
        self.assertLess(events.index("native_observed"), events.index("branch_deleted"))

    def test_missing_abort_result_retains_branch_and_local_evidence(self):
        setup = selected("fresh")
        session = {"area": "fixture-owned-area", "branch": "g9-win/" + setup["nonce"],
                   "setup": setup, "setup_commit": "d" * 40,
                   "baseline_sha256": "e" * 64, "run_id": 17,
                   "baseline": {"before": {}}}
        with patch.object(host_relay, "remote_head", side_effect=["d" * 40, "f" * 40]), \
             patch.object(host_relay, "sha", return_value="e" * 64), \
             patch.object(host_relay, "run_metadata", return_value={"id": 17}), \
             patch.object(host_relay, "one_file_commit", return_value="f" * 40), \
             patch.object(host_relay, "checked"), \
             patch.object(host_relay, "remove_exact_branch", return_value=True) as removed, \
             patch.object(host_relay, "wait_for_completion", return_value={"id": 17}), \
             patch.object(host_relay, "baseline_issues", return_value=[]), \
             patch.object(host_relay, "download_artifact", return_value=False), \
             patch.object(host_relay, "remove_area") as erased:
            with self.assertRaises(Refused):
                host_relay.abort_hosted(session)
            removed.assert_not_called()
            erased.assert_not_called()

    def test_failed_preparation_after_hosted_run_retains_branch_and_area(self):
        with tempfile.TemporaryDirectory() as directory:
            clone = Path(directory) / "repository"
            for name in ("scripts/g9/windows_worker.py", "scripts/g9/windows_gate.py",
                         "scripts/g9/gh_fixture.py", "scripts/g9/installer_wrapper.ps1",
                         ".github/workflows/g9-windows-installer.yml"):
                target = clone / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text("synthetic reviewed source", encoding="utf-8")
            with patch.object(host_relay.tempfile, "mkdtemp", return_value=directory), \
                 patch.object(host_relay, "checked"), \
                 patch.object(host_relay, "remote_head", side_effect=[None, "d" * 40]), \
                 patch.object(host_relay, "one_file_commit", return_value="d" * 40), \
                 patch.object(host_relay, "wait_for_run", return_value={"databaseId": 17}), \
                 patch.object(host_relay, "wait_for_baseline", side_effect=Refused("no baseline")), \
                 patch.object(host_relay, "command"), \
                 patch.object(host_relay, "remove_exact_branch", return_value=True) as removed, \
                 patch.object(host_relay, "remove_area") as erased:
                with self.assertRaises(Refused):
                    host_relay.prepare_hosted(case="fresh", source_sha=SHA,
                                              version=VERSION, binary_sha256="b" * 64,
                                              installer_sha256="c" * 64,
                                              accepted_gate_sha=SHA)
                removed.assert_not_called()
                erased.assert_not_called()

    def test_pre_task_broker_request_is_bound_to_exact_hosted_session(self):
        setup = selected("fresh")
        session = {"setup": setup, "setup_commit": "d" * 40,
                   "baseline_sha256": "e" * 64, "run_id": 17}
        body = {"protocol": "tadx-windows-installer-broker/1", "id": "cmd-00001",
                "case": "fresh", "argv": list(argv_for("fresh", VERSION)),
                "source_sha": SHA, "binary_sha256": "b" * 64,
                "installer_sha256": "c" * 64, "fixture_version": VERSION,
                "setup_commit": "d" * 40, "baseline_sha256": "e" * 64,
                "run_id": 17}
        body["command_sha256"] = hashlib.sha256(json.dumps(body, separators=(",", ":")).encode()).hexdigest()
        self.assertEqual(host_relay.verify_broker_request(session, body), body)
        for changed in ({"run_id": 18}, {"baseline_sha256": "f" * 64},
                        {"argv": [*body["argv"], "-Command", "whoami"]},
                        {"id": "cmd-../../escape"}, {"unexpected": True}):
            with self.subTest(changed=changed), self.assertRaises(Refused):
                host_relay.verify_broker_request(session, {**body, **changed})

    def test_broker_response_preserves_verified_native_refusal(self):
        request = {"id": "cmd-00001", "command_sha256": "a" * 64,
                   "setup_commit": "b" * 64, "baseline_sha256": "c" * 64,
                   "run_id": 17}
        verified = {"status": "verified", "native_result": {
            "native_exit_status": 1, "native_stdout": "", "native_stderr": "download unavailable"}}
        answer = host_relay.broker_response(request, verified)
        self.assertEqual(answer["exit_status"], 1)
        self.assertEqual(answer["status"], "verified")
        rejected = host_relay.broker_response(request, {**verified, "status": "not_verified"})
        self.assertEqual((rejected["status"], rejected["exit_status"]), ("not_verified", 125))
        self.assertNotIn("download unavailable", rejected["stderr"])
        with self.assertRaises(Refused):
            host_relay.broker_response(request, {"status": "verified", "native_result": {
                "native_exit_status": 125, "native_stdout": "", "native_stderr": ""}})

    def test_hosted_worker_accepts_only_one_child_command_file(self):
        setup = selected("fresh")
        commit = "f" * 40
        requested = command_request(setup, list(argv_for("fresh", VERSION)), "d" * 40, "e" * 64)
        values = {("ls-remote", "origin", "refs/heads/g9-win/nonce_123456789"):
                  commit + "\trefs/heads/g9-win/nonce_123456789",
                  ("rev-parse", "FETCH_HEAD"): commit,
                  ("rev-parse", commit + "^"): "d" * 40,
                  ("diff-tree", "--no-commit-id", "--name-only", "-r", commit):
                  ".g9-windows-installer-command.json",
                  ("cat-file", "-s", commit + ":.g9-windows-installer-command.json"):
                  "1024",
                  ("show", commit + ":.g9-windows-installer-command.json"):
                  json.dumps(requested)}
        with patch.object(windows_worker, "git_value", side_effect=lambda *args: values[args]), \
             patch.object(windows_worker, "command", return_value=SimpleNamespace(
                 returncode=0, stdout="https://github.com/ahillspace/tadx.git")), \
             patch.dict(os.environ, {"G9_GITHUB_TOKEN": "synthetic-token",
                                      "GITHUB_RUN_ID": "17"}):
            self.assertEqual(windows_worker.wait_for_command(setup, "d" * 40, "e" * 64,
                                                              timeout=1), (commit, requested))
            stop = stop_request(setup, "d" * 40, "e" * 64, 17)
            values[("show", commit + ":.g9-windows-installer-command.json")] = json.dumps(stop)
            self.assertEqual(windows_worker.wait_for_command(setup, "d" * 40, "e" * 64,
                                                              timeout=1), (commit, None))
            values[("show", commit + ":.g9-windows-installer-command.json")] = json.dumps(
                {**stop, "run_id": 18})
            with self.assertRaises(Refused):
                windows_worker.wait_for_command(setup, "d" * 40, "e" * 64, timeout=1)
            values[("show", commit + ":.g9-windows-installer-command.json")] = json.dumps(requested)
            values[("diff-tree", "--no-commit-id", "--name-only", "-r", commit)] += "\nscripts/install.ps1"
            with self.assertRaises(Refused):
                windows_worker.wait_for_command(setup, "d" * 40, "e" * 64, timeout=1)
            values[("diff-tree", "--no-commit-id", "--name-only", "-r", commit)] = ".g9-windows-installer-command.json"
            values[("cat-file", "-s", commit + ":.g9-windows-installer-command.json")] = "4097"
            with self.assertRaises(Refused):
                windows_worker.wait_for_command(setup, "d" * 40, "e" * 64, timeout=1)

    def test_workflow_publishes_starting_state_before_model_command(self):
        workflow = (ROOT / ".github/workflows/g9-windows-installer.yml").read_text()
        steps = [workflow.index(item) for item in ("windows_worker.py prepare",
                "g9-windows-baseline-${{ github.run_id }}", "windows_worker.py execute",
                "g9-windows-result-${{ github.run_id }}")]
        self.assertEqual(steps, sorted(steps))
        self.assertIn("- '.g9-windows-installer-setup.json'", workflow)
        self.assertNotIn("- '.g9-windows-installer-command.json'", workflow)
        self.assertIn("fetch-depth: 2", workflow)
        self.assertIn("persist-credentials: false", workflow)
        self.assertIn("G9_GITHUB_TOKEN: ${{ github.token }}", workflow)

    def test_hosted_build_hash_mismatch_blocks_before_asset_creation(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with patch.object(windows_worker, "command", return_value=SimpleNamespace(returncode=0)) as issued, \
                 patch.object(windows_worker, "git_value", return_value=SHA), \
                 patch.object(windows_worker, "sha", return_value="f" * 64):
                with self.assertRaises(Refused):
                    windows_worker.build_assets(selected("fresh"), root)
            build = [call for call in issued.call_args_list if call.args[:2] == ("go", "build")]
            self.assertEqual(len(build), 1)
            self.assertIn("-buildvcs=false", build[0].args)
            self.assertEqual({key: build[0].kwargs["env"][key]
                              for key in ("CGO_ENABLED", "GOOS", "GOARCH", "GOTOOLCHAIN")},
                             {"CGO_ENABLED": "0", "GOOS": "windows", "GOARCH": "amd64",
                              "GOTOOLCHAIN": "local"})
            self.assertFalse((root / "assets").exists())

    def test_failed_archive_requires_both_local_transport_refusals(self):
        before = {"binary_sha256": "b" * 64, "completion_markers": 1,
                  "user_path_count": 1, "profile_sentinel": True,
                  "sentinel_sha256": "c" * 64}
        transport = [{"asset": "checksums.txt", "failed": False},
                     {"asset": f"tadx_{VERSION}-missing_windows_amd64.zip", "failed": True}]
        self.assertTrue(observed_issues("failed-download-preserves-binary", before,
                                        before, 1, "b" * 64, transport, VERSION))

    def test_native_invocation_uses_bounded_https_interceptor(self):
        with patch.object(windows_worker, "command") as issued:
            windows_worker.invoke("fresh", selected("fresh"), Path("fixture-root"), {})
        self.assertIn("scripts/g9/installer_wrapper.ps1", issued.call_args.args)

    def test_failed_prepare_restoration_retains_native_root_and_failure_artifact(self):
        setup = selected("fresh")
        with tempfile.TemporaryDirectory() as directory:
            temporary = Path(directory)
            root = temporary / "g9-installer" / setup["nonce"]
            setup_path = temporary / "setup.json"
            setup_path.write_text(json.dumps(setup), encoding="utf-8")
            failure_path = temporary / "setup-failure.json"
            env = {"GITHUB_SHA": "d" * 40, "GITHUB_RUN_ID": "17",
                   "G9_BASELINE_PATH": str(temporary / "baseline.json"),
                   "G9_SETUP_FAILURE_PATH": str(failure_path)}
            with patch.object(windows_worker, "SETUP_PATH", setup_path), \
                 patch.object(windows_worker, "validate_setup", return_value=setup), \
                 patch.object(windows_worker, "fixture_root", return_value=(temporary, root)), \
                 patch.object(windows_worker, "user_path", return_value=None), \
                 patch.object(windows_worker, "prepare", side_effect=Refused("synthetic setup failure")), \
                 patch.object(windows_worker, "restore_user_path", side_effect=OSError("synthetic restore failure")), \
                 patch.object(windows_worker, "remove_candidate_worktree") as removed, \
                 patch.object(windows_worker.shutil, "rmtree") as erased, \
                 patch.dict(os.environ, env):
                with self.assertRaises(Exception):
                    windows_worker.prepare_main()
            self.assertTrue(root.is_dir())
            removed.assert_not_called()
            erased.assert_not_called()
            failure = json.loads(failure_path.read_text(encoding="utf-8"))
            self.assertIs(failure["cleanup_ok"], False)
            self.assertIs(failure["fixture_retained"], True)

    def test_failed_prepare_deletes_fixture_only_after_verified_restoration(self):
        setup = selected("fresh")
        with tempfile.TemporaryDirectory() as directory:
            temporary = Path(directory)
            root = temporary / "g9-installer" / setup["nonce"]
            setup_path = temporary / "setup.json"
            setup_path.write_text(json.dumps(setup), encoding="utf-8")
            failure_path = temporary / "setup-failure.json"
            env = {"GITHUB_SHA": "d" * 40, "GITHUB_RUN_ID": "17",
                   "G9_BASELINE_PATH": str(temporary / "baseline.json"),
                   "G9_SETUP_FAILURE_PATH": str(failure_path)}
            cleanup_order = []

            def restored(_):
                cleanup_order.append("restore")

            def removed(_):
                cleanup_order.append("worktree")
                return True

            with patch.object(windows_worker, "SETUP_PATH", setup_path), \
                 patch.object(windows_worker, "validate_setup", return_value=setup), \
                 patch.object(windows_worker, "fixture_root", return_value=(temporary, root)), \
                 patch.object(windows_worker, "user_path", return_value="original"), \
                 patch.object(windows_worker, "prepare",
                              side_effect=Refused("synthetic setup failure")), \
                 patch.object(windows_worker, "restore_user_path", side_effect=restored), \
                 patch.object(windows_worker, "remove_candidate_worktree",
                              side_effect=removed), \
                 patch.dict(os.environ, env):
                with self.assertRaises(Refused):
                    windows_worker.prepare_main()
            self.assertEqual(cleanup_order, ["restore", "worktree"])
            self.assertFalse(root.exists())
            failure = json.loads(failure_path.read_text(encoding="utf-8"))
            self.assertIs(failure["user_path_restored"], True)
            self.assertIs(failure["worktree_removed"], True)
            self.assertIs(failure["fixture_retained"], False)
            self.assertIs(failure["cleanup_ok"], True)

    def test_uncertain_setup_push_retains_local_area_on_different_remote_head(self):
        with tempfile.TemporaryDirectory() as directory:
            clone = Path(directory) / "repository"
            (clone / ".git").mkdir(parents=True)
            for name in ("scripts/g9/windows_worker.py", "scripts/g9/windows_gate.py",
                         "scripts/g9/gh_fixture.py", "scripts/g9/installer_wrapper.ps1",
                         ".github/workflows/g9-windows-installer.yml"):
                target = clone / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text("synthetic reviewed source", encoding="utf-8")
            def issued(argv, **_):
                if argv[:2] == ["git", "push"]:
                    raise Refused("uncertain push result")
                return ""
            with patch.object(host_relay.tempfile, "mkdtemp", return_value=directory), \
                 patch.object(host_relay, "checked", side_effect=issued), \
                 patch.object(host_relay, "remote_head", side_effect=[None, "f" * 40]), \
                 patch.object(host_relay, "one_file_commit", return_value="d" * 40), \
                 patch.object(host_relay, "remove_exact_branch") as removed, \
                 patch.object(host_relay, "remove_area") as erased:
                with self.assertRaises(Refused):
                    host_relay.prepare_hosted(case="fresh", source_sha=SHA,
                                              version=VERSION, binary_sha256="b" * 64,
                                              installer_sha256="c" * 64,
                                              accepted_gate_sha=SHA)
                removed.assert_not_called()
                erased.assert_not_called()

    def test_deleted_setup_branch_emits_native_cleanup_result(self):
        setup = selected("fresh")
        with tempfile.TemporaryDirectory() as directory:
            temporary = Path(directory)
            root = temporary / "g9-installer" / setup["nonce"]
            root.mkdir(parents=True)
            (root / "private-state.json").write_text(json.dumps({
                "setup": setup, "setup_commit": "d" * 40, "run_id": 17,
                "original_user_path": None}), encoding="utf-8")
            setup_path = temporary / "setup.json"
            setup_path.write_text(json.dumps(setup), encoding="utf-8")
            baseline_path = temporary / "baseline.json"
            baseline_path.write_text(json.dumps({"before": {}}), encoding="utf-8")
            result_path = temporary / "result.json"
            env = {"G9_BASELINE_PATH": str(baseline_path),
                   "G9_RESULT_PATH": str(result_path),
                   "GITHUB_SHA": "d" * 40, "GITHUB_RUN_ID": "17"}
            with patch.object(windows_worker, "SETUP_PATH", setup_path), \
                 patch.object(windows_worker, "validate_setup", return_value=setup), \
                 patch.object(windows_worker, "fixture_root", return_value=(temporary, root)), \
                 patch.object(windows_worker, "baseline_issues", return_value=[]), \
                 patch.object(windows_worker, "wait_for_command", side_effect=Refused("branch removed")), \
                 patch.object(windows_worker, "restore_user_path") as restored, \
                 patch.object(windows_worker, "user_path", return_value=None), \
                 patch.object(windows_worker, "remove_candidate_worktree", return_value=True), \
                 patch.dict(os.environ, env):
                self.assertEqual(windows_worker.execute_main(), 1)
            self.assertFalse(root.exists())
            restored.assert_called_once_with(None)
            result = json.loads(result_path.read_text(encoding="utf-8"))
            self.assertIs(result["native_executed"], False)
            self.assertIs(result["cleanup_ok"], True)
            self.assertEqual(result["run_id"], 17)

    def test_internal_stop_child_restores_state_without_native_installer(self):
        setup = selected("fresh")
        with tempfile.TemporaryDirectory() as directory:
            temporary = Path(directory)
            root = temporary / "g9-installer" / setup["nonce"]
            root.mkdir(parents=True)
            (root / "private-state.json").write_text(json.dumps({
                "setup": setup, "setup_commit": "d" * 40, "run_id": 17,
                "original_user_path": None}), encoding="utf-8")
            setup_path = temporary / "setup.json"
            setup_path.write_text(json.dumps(setup), encoding="utf-8")
            baseline_path = temporary / "baseline.json"
            baseline_path.write_text(json.dumps({"before": {}}), encoding="utf-8")
            result_path = temporary / "result.json"
            env = {"G9_BASELINE_PATH": str(baseline_path),
                   "G9_RESULT_PATH": str(result_path),
                   "GITHUB_SHA": "d" * 40, "GITHUB_RUN_ID": "17"}
            with patch.object(windows_worker, "SETUP_PATH", setup_path), \
                 patch.object(windows_worker, "validate_setup", return_value=setup), \
                 patch.object(windows_worker, "fixture_root", return_value=(temporary, root)), \
                 patch.object(windows_worker, "baseline_issues", return_value=[]), \
                 patch.object(windows_worker, "wait_for_command", return_value=("f" * 40, None)), \
                 patch.object(windows_worker, "snapshot", return_value={}), \
                 patch.object(windows_worker, "invoke") as installed, \
                 patch.object(windows_worker, "restore_user_path") as restored, \
                 patch.object(windows_worker, "user_path", return_value=None), \
                 patch.object(windows_worker, "remove_candidate_worktree", return_value=True), \
                 patch.dict(os.environ, env):
                self.assertEqual(windows_worker.execute_main(), 1)
            installed.assert_not_called()
            restored.assert_called_once_with(None)
            self.assertFalse(root.exists())
            result = json.loads(result_path.read_text(encoding="utf-8"))
            self.assertEqual(result["command_commit"], "f" * 40)
            self.assertIs(result["stop_requested"], True)
            self.assertIs(result["fresh_before_stop"], True)
            self.assertIs(result["native_executed"], False)
            self.assertIs(result["cleanup_ok"], True)

    def test_pre_command_identity_refusal_still_restores_native_state(self):
        setup = selected("fresh")
        with tempfile.TemporaryDirectory() as directory:
            temporary = Path(directory)
            root = temporary / "g9-installer" / setup["nonce"]
            root.mkdir(parents=True)
            (root / "private-state.json").write_text(json.dumps({
                "setup": setup, "setup_commit": "d" * 40, "run_id": 17,
                "original_user_path": None}), encoding="utf-8")
            setup_path = temporary / "setup.json"
            setup_path.write_text(json.dumps(setup), encoding="utf-8")
            baseline_path = temporary / "baseline.json"
            baseline_path.write_text(json.dumps({"before": {}}), encoding="utf-8")
            result_path = temporary / "result.json"
            env = {"G9_BASELINE_PATH": str(baseline_path),
                   "G9_RESULT_PATH": str(result_path),
                   "GITHUB_SHA": "d" * 40, "GITHUB_RUN_ID": "17"}
            with patch.object(windows_worker, "SETUP_PATH", setup_path), \
                 patch.object(windows_worker, "validate_setup", return_value=setup), \
                 patch.object(windows_worker, "fixture_root", return_value=(temporary, root)), \
                 patch.object(windows_worker, "baseline_issues", return_value=["identity changed"]), \
                 patch.object(windows_worker, "wait_for_command") as waited, \
                 patch.object(windows_worker, "restore_user_path") as restored, \
                 patch.object(windows_worker, "user_path", return_value=None), \
                 patch.object(windows_worker, "remove_candidate_worktree", return_value=True), \
                 patch.dict(os.environ, env):
                self.assertEqual(windows_worker.execute_main(), 1)
            waited.assert_not_called()
            restored.assert_called_once_with(None)
            self.assertFalse(root.exists())
            result = json.loads(result_path.read_text(encoding="utf-8"))
            self.assertIs(result["native_executed"], False)
            self.assertIs(result["cleanup_ok"], True)

    def test_only_seven_hosted_cases_extend_wall_budget(self):
        for identity in windows_preparer.IDS:
            original = {"id": identity, "evaluator": {"budgets": {"wall_seconds": 600,
                         "tokens": 1000}}}
            changed = windows_preparer.extend_budget(original)
            self.assertEqual(changed["evaluator"]["budgets"],
                             {"wall_seconds": 2400, "tokens": 1000})
            self.assertEqual(original["evaluator"]["budgets"]["wall_seconds"], 600)
        for bad in ("V-installer-unix-fresh", "P-policy-install", "V-installer-windows-other"):
            with self.assertRaises(ValueError):
                windows_preparer.extend_budget({"id": bad, "evaluator": {"budgets": {"wall_seconds": 600}}})
        with self.assertRaises(ValueError):
            windows_preparer.extend_budget({"id": "V-installer-windows-fresh",
                                            "evaluator": {"budgets": {"wall_seconds": 1800}}})

    def test_broker_edits_anchor_exact_current_suite_and_parse(self):
        root = ROOT / "scripts/g9"
        broker_source = (root / "test_fixtures/local_broker.cjs").read_text(encoding="utf-8")
        client_source = (root / "test_fixtures/tadx_client.cjs").read_text(encoding="utf-8")
        broker = windows_broker_patches.broker(broker_source)
        client = windows_broker_patches.client(client_source)
        self.assertIn("windowsInstallerBroker.allowed(args,policyState.guard)", broker)
        self.assertIn("windows-response.json", broker)
        self.assertIn("2160000", client)
        with self.assertRaises(ValueError):
            windows_broker_patches.broker(broker)
        with tempfile.TemporaryDirectory() as directory:
            for name, source in (("local_broker.cjs", broker), ("tadx_client.cjs", client)):
                target = Path(directory) / name
                target.write_text(source, encoding="utf-8")
                checked = subprocess.run(["node", "--check", str(target)], capture_output=True, text=True)
                self.assertEqual(checked.returncode, 0, checked.stderr)


if __name__ == "__main__":
    unittest.main()
