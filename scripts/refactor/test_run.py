"""Tests for gate evidence handling; no Go execution or external services."""

import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("refactor_gates", Path(__file__).with_name("run.py"))
gates = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gates)


def events(*items):
    return "\n".join(json.dumps(item) for item in items)


def successful(name="TestContract"):
    return events({"Action": "run", "Test": name}, {"Action": "pass", "Test": name}, {"Action": "pass"})


def rejected(name="TestContract", message="assertion mismatch"):
    return events({"Action": "run", "Test": name}, {"Action": "output", "Test": name, "Output": message}, {"Action": "fail", "Test": name}, {"Action": "fail"})


class EvidenceTests(unittest.TestCase):
    def test_pass_requires_exact_requested_tests_and_package_completion(self):
        self.assertEqual(gates.evaluate_go_result(0, successful(), ["TestContract"])["status"], "passed")
        for output in [successful("TestDifferent"), events({"Action": "pass"}), events({"Action": "run", "Test": "TestContract"})]:
            with self.subTest(output=output), self.assertRaises(gates.GateError):
                gates.evaluate_go_result(0, output, ["TestContract"])

    def test_compile_failure_is_not_a_detected_seed(self):
        output = events({"Action": "build-output", "Output": "compile failed"}, {"Action": "fail"})
        with self.assertRaises(gates.GateError):
            gates.evaluate_go_result(1, output, ["TestContract"], "TestContract", "mismatch")

    def test_seed_requires_assertion_failure_not_pass_crash_or_wrong_exit(self):
        result = gates.evaluate_go_result(1, rejected(), ["TestContract"], "TestContract", "mismatch")
        self.assertEqual(result["status"], "detected")
        for code, output in [(0, successful()), (2, rejected()), (1, rejected(message="panic: assertion mismatch")), (1, rejected(message="different failure"))]:
            with self.subTest(code=code, output=output), self.assertRaises(gates.GateError):
                gates.evaluate_go_result(code, output, ["TestContract"], "TestContract", "mismatch")

    def test_skipped_subtest_or_package_never_passes(self):
        for event in [{"Action": "skip", "Test": "TestContract/platform"}, {"Action": "skip"}]:
            with self.subTest(event=event), self.assertRaises(gates.GateError):
                gates.evaluate_go_result(0, successful() + "\n" + events(event), ["TestContract"])

    def test_unrelated_failure_invalidates_seed(self):
        output = rejected() + "\n" + events({"Action": "run", "Test": "TestOther"}, {"Action": "fail", "Test": "TestOther"})
        with self.assertRaises(gates.GateError):
            gates.evaluate_go_result(1, output, ["TestContract"], "TestContract", "mismatch")

    def test_expected_subtest_assertion_counts(self):
        output = events({"Action": "run", "Test": "TestContract"}, {"Action": "run", "Test": "TestContract/changed"}, {"Action": "output", "Test": "TestContract/changed", "Output": "assertion mismatch"}, {"Action": "fail", "Test": "TestContract/changed"}, {"Action": "fail", "Test": "TestContract"}, {"Action": "fail"})
        self.assertEqual(gates.evaluate_go_result(1, output, ["TestContract"], "TestContract", "mismatch")["status"], "detected")

    def test_malformed_output_fails_closed(self):
        for output in ["not json", "[]", "null", successful() + "\ntruncated"]:
            with self.subTest(output=output), self.assertRaises(gates.GateError):
                gates.evaluate_go_result(0, output, ["TestContract"])


class SourceTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        (self.root / "go.mod").write_bytes(b"module example.test/gates\ngo 1.26\n")
        (self.root / "contract_test.go").write_bytes(b"package gates\n")
        self.original = gates.source_manifest(self.root)
        rows = []
        for name in self.original:
            data = (self.root / name).read_bytes()
            oid = gates.hashlib.sha1(b"blob " + str(len(data)).encode() + b"\0" + data).hexdigest()
            rows.append(f"100644 blob {oid}\t{name}".encode())
        self.tree = b"\0".join(rows) + b"\0"

    def fake_git(self, repo, *args):
        if args[:2] == ("rev-parse", "--verify"):
            return b"1234567890abcdef\n"
        if args == ("rev-parse", "--show-object-format"):
            return b"sha1\n"
        return self.tree

    def provenance(self, allowed=()):
        with patch.object(gates, "git", side_effect=self.fake_git):
            return gates.capture_provenance(self.root, self.root, "baseline", allowed)

    def test_identical_snapshot_records_exact_revision_and_fixture_digest(self):
        record = self.provenance()
        self.assertEqual(record["candidate_revision"], "1234567890abcdef")
        self.assertEqual(record["changed_paths"], [])
        self.assertEqual(record["candidate_source_sha256"], gates.manifest_digest(self.original))
        self.assertEqual(len(record["fixture_and_test_sha256"]), 64)

    def test_tracked_link_is_only_accepted_as_exact_regular_file_content(self):
        self.tree = self.tree.replace(b"100644 blob", b"120000 blob", 1)
        record = self.provenance()
        self.assertEqual(len(record["materialized_link_paths"]), 1)
        self.assertEqual(record["changed_paths"], [])

    def test_changed_missing_and_extra_files_require_explicit_scope(self):
        (self.root / "contract_test.go").unlink()
        (self.root / "new.go").write_text("package gates\n")
        with self.assertRaises(gates.GateError):
            self.provenance()
        record = self.provenance(["contract_test.go", "new.go"])
        self.assertEqual(record["changed_paths"], ["contract_test.go", "new.go"])
        self.assertIsNone(record["candidate_revision"])

    def test_directory_scope_does_not_match_similar_prefix(self):
        (self.root / "actions2").mkdir()
        (self.root / "actions2" / "new.go").write_text("package fixture\n")
        with self.assertRaises(gates.GateError):
            self.provenance(["actions"])

    def test_rejects_worktree_and_unsafe_scopes(self):
        for path in ["", ".", "../outside", "/outside", "C:/outside", "a/../b", "a\\b"]:
            with self.subTest(path=path), self.assertRaises(gates.GateError):
                gates.safe_relative(path)
        (self.root / ".git").mkdir()
        with self.assertRaises(gates.GateError):
            gates.source_manifest(self.root)

    def test_seed_is_unique_and_does_not_write_source(self):
        seed = {"id": "fixture", "path": "contract_test.go", "before": "package gates", "after": "package changed"}
        self.assertEqual(gates.seeded_source(self.root, seed), "package changed\n")
        self.assertEqual(gates.source_manifest(self.root), self.original)
        seed["before"] = "missing"
        with self.assertRaises(gates.GateError):
            gates.seeded_source(self.root, seed)
        (self.root / "contract_test.go").write_text("missing missing")
        with self.assertRaises(gates.GateError):
            gates.seeded_source(self.root, seed)

    def test_reparse_point_is_rejected_before_traversal(self):
        metadata = unittest.mock.Mock(st_mode=0o040755, st_file_attributes=0x400)
        with patch.object(Path, "lstat", return_value=metadata), self.assertRaises(gates.GateError):
            gates.source_manifest(self.root)


class ConfigurationTests(unittest.TestCase):
    def test_repository_manifest_validates(self):
        manifest = json.loads(Path(__file__).with_name("gates.json").read_text())
        self.assertEqual(len(gates.validate_manifest(manifest)["seeds"]), 7)

    def test_reporting_manifest_has_both_approved_defect_demonstrations(self):
        manifest = json.loads(Path(__file__).with_name("reporting-gates.json").read_text())
        result = gates.validate_manifest(manifest)
        self.assertEqual(len(result["checks"][0]["tests"]), 3)
        self.assertEqual({seed["id"] for seed in result["seeds"]},
                         {"seed-receipt-phase", "seed-receipt-request-identity"})

    def test_baseline_fixes_manifest_has_three_approved_defect_demonstrations(self):
        manifest = json.loads(Path(__file__).with_name("baseline-fixes-gates.json").read_text())
        result = gates.validate_manifest(manifest)
        self.assertEqual(len(result["checks"]), 3)
        self.assertEqual({seed["id"] for seed in result["seeds"]},
                         {"seed-flow-request-identity", "seed-project-decoded-identity", "seed-receipt-intent-scope"})

    def test_missing_control_or_unsafe_seed_is_rejected(self):
        manifest = json.loads(Path(__file__).with_name("gates.json").read_text())
        manifest["seeds"][0]["control"] = "missing"
        with self.assertRaises(gates.GateError):
            gates.validate_manifest(manifest)
        manifest["seeds"][0]["path"] = "../outside"
        with self.assertRaises(gates.GateError):
            gates.validate_manifest(manifest)

    def test_child_environment_drops_credentials_and_go_overrides(self):
        with tempfile.TemporaryDirectory() as directory:
            probe = subprocess.CompletedProcess([], 0, "cache\nmodules\n", "")
            with patch.dict(os.environ, {"PATH": "tools", "TYPESAFE_API_KEY": "sensitive", "TADX_PAT_SECRET": "sensitive", "GOFLAGS": "-overlay=untrusted"}, clear=True), patch.object(gates.subprocess, "run", return_value=probe) as call:
                env = gates.child_environment(Path(directory) / "isolated-home")
            bootstrap = call.call_args.kwargs["env"]
            self.assertNotIn("TYPESAFE_API_KEY", bootstrap)
            self.assertNotIn("GOFLAGS", bootstrap)
            self.assertEqual(bootstrap["GOTOOLCHAIN"], "local")
            self.assertEqual(bootstrap["GOPROXY"], "off")
            self.assertEqual(env["PATH"], "tools")
            self.assertEqual(env["GOPROXY"], "off")
            self.assertEqual(env["GOWORK"], "off")
            self.assertNotIn("TYPESAFE_API_KEY", env)
            self.assertNotIn("TADX_PAT_SECRET", env)
            self.assertNotIn("GOFLAGS", env)

    def test_logs_redact_roots(self):
        source = Path(tempfile.gettempdir()) / "isolated-source"
        self.assertNotIn(str(source), gates.redact("source=" + str(source), [source]))

    def test_logs_redact_nested_escaped_windows_paths(self):
        source = r"C:\Users\Example User\project\source"
        for width in (1, 2, 4, 8):
            with self.subTest(width=width):
                escaped = source.replace("\\", "\\" * width)
                for message in (escaped, json.dumps({"path": escaped, "digest": "a" * 64})):
                    redacted = gates.redact(message, [source])
                    self.assertNotIn("Example User", redacted)
                    self.assertNotIn("project", redacted)
                    self.assertIn("<local-root>", redacted)
                    if "digest" in message:
                        self.assertIn("a" * 64, redacted)

        other = r"C:\Users\Private Person\different"
        escaped_other = other.replace("\\", "\\\\\\\\")
        self.assertNotIn("Private Person", gates.redact(escaped_other, []))

    def test_logs_redact_truncated_windows_home_fragments(self):
        source = r"C:\Users\Example User\project\source"
        for fragment in (
            "/Example User/projects/demo",
            "Users/Example User/projects/demo",
            r"\Example User\projects\demo",
            r"Users\Example User\projects\demo",
        ):
            with self.subTest(fragment=fragment):
                message = json.dumps({"Output": fragment, "digest": "a" * 64, "site": "team-site"})
                redacted = gates.redact(message, [source])
                self.assertNotIn("Example User", redacted)
                self.assertIn("<user-root>", redacted)
                parsed = json.loads(redacted)
                self.assertEqual(parsed["digest"], "a" * 64)
                self.assertEqual(parsed["site"], "team-site")

    def test_timeout_stops_tree_and_never_passes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            process = unittest.mock.Mock(pid=123, returncode=-1)
            process.communicate.side_effect = [subprocess.TimeoutExpired(["go"], 1), ("partial", "")]
            with patch.object(gates.subprocess, "Popen", return_value=process), patch.object(gates, "stop_process_tree") as stop:
                runner = gates.Runner(root, root, {}, 1)
                with self.assertRaises(gates.GateError):
                    runner.command("timeout", ["go", "test"])
            stop.assert_called_once_with(process, {})
            self.assertTrue(runner.commands[0]["timed_out"])
            self.assertIsNone(runner.commands[0]["exit_code"])

    def test_unconfirmed_timeout_cleanup_is_explicit(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            process = unittest.mock.Mock(pid=123)
            process.communicate.side_effect = subprocess.TimeoutExpired(["go"], 1)
            escaped = str(root).replace("\\", "\\\\\\\\")
            with patch.object(gates.subprocess, "Popen", return_value=process), patch.object(gates, "stop_process_tree", side_effect=gates.GateError("unconfirmed at " + escaped)):
                runner = gates.Runner(root, root, {}, 1)
                with self.assertRaisesRegex(gates.GateError, "containment"):
                    runner.command("timeout", ["go", "test"])
            self.assertEqual(runner.commands[0]["containment_error"], "unconfirmed at <local-root>")


if __name__ == "__main__":
    unittest.main()
