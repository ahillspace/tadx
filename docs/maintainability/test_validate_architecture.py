"""Exercise assessment accounting with isolated, in-memory documents."""

import unittest
from unittest.mock import patch

import validate_architecture as validator


class ValidateArchitectureTests(unittest.TestCase):
    def setUp(self):
        self.inventory = {
            "source_revision": "pinned",
            "files": [
                {"path": path, "physical_lines": 10}
                for path in (
                    "actions/auth/login/action.go",
                    "actions/auth/status/action.go",
                    "internal/app/auth.go",
                )
            ] + [{
                "path": "actions/auth/login/action_test.go",
                "physical_lines": 20,
                "declarations": [{"name": "TestLogin"}],
            }],
        }
        self.plan = {
            "source_revision": "pinned",
            "rules": [
                {"sources": ["actions/auth/login", "actions/auth/status"],
                 "target": "actions/auth"},
                {"sources": ["internal/app"], "target": "same"},
            ],
            "new_packages": [{"target": "actions/mutation"}],
            "app_responsibility_moves": [{
                "source": "internal/app/auth.go",
                "targets": ["internal/app", "actions/auth", "actions/mutation"],
            }],
        }
        self.documents = {
            "inventory-dd22c33.json": self.inventory,
            "architecture-decision-map.json": self.plan,
            **{name: {"source_revision": "pinned", "reviewed_paths": []}
               for name in validator.ASSESSMENTS},
        }
        self.report = self.documents[validator.ASSESSMENTS[0]]
        self.report.update({
            "reviewed_paths": [{"path": "internal/app/auth.go"}],
            "evidence": [{"path": "internal/app/auth.go", "line": 10}],
            "tests": [{"path": "actions/auth/login/action_test.go",
                       "symbol": "TestLogin"}],
            "test_groups": [{"paths": ["actions/auth/login/action_test.go"],
                             "tests": ["TestLogin"]}],
        })
        reader = patch.object(validator, "read", side_effect=lambda root, name: self.documents[name])
        self.mock_read = reader.start()
        self.addCleanup(reader.stop)

    def assert_rejected(self, message):
        with self.assertRaisesRegex(ValueError, message):
            validator.validate()

    def test_accepts_complete_mapping_and_pinned_evidence(self):
        self.assertEqual(validator.validate(), {
            "source_packages_accounted_for": 3,
            "app_files_accounted_for": 1,
            "current_action_packages": 2,
            "consolidated_existing_action_owners": 1,
            "proposed_action_packages_including_new": 2,
            "proposed_package_directories": 3,
            "source_paths_with_scoped_review_entries": 1,
            "evidence_references_checked": 1,
            "named_test_references_checked": 1,
            "runtime_verification": "not performed",
        })
        self.assertEqual(self.mock_read.call_count, 2 + len(validator.ASSESSMENTS))

    def test_rejects_missing_source_package(self):
        self.plan["rules"][0]["sources"].pop()
        self.assert_rejected("Package map has missing or unknown sources")

    def test_rejects_unknown_source_package(self):
        self.plan["rules"][0]["sources"].append("actions/unknown")
        self.assert_rejected("Package map has missing or unknown sources")

    def test_rejects_duplicate_source_package(self):
        self.plan["rules"][0]["sources"].append("actions/auth/login")
        self.assert_rejected("Duplicate package assignment")

    def test_rejects_missing_app_file(self):
        self.plan["app_responsibility_moves"].clear()
        self.assert_rejected("App map has missing or unknown source files")

    def test_rejects_unknown_destination(self):
        self.plan["app_responsibility_moves"][0]["targets"].append("internal/unknown")
        self.assert_rejected("Unknown app destination")

    def test_rejects_mixed_plan_revision(self):
        self.plan["source_revision"] = "other"
        self.assert_rejected("Mixed plan revision")

    def test_rejects_mixed_evidence_revision(self):
        self.report["source_revision"] = "other"
        self.assert_rejected("Mixed revision")

    def test_rejects_invalid_evidence_lines(self):
        for line in (0, -1, 11, "10", None):
            with self.subTest(line=line):
                self.report["evidence"][0]["line"] = line
                self.assert_rejected("Invalid evidence reference")

    def test_rejects_unknown_evidence_path(self):
        self.report["evidence"][0]["path"] = "internal/app/missing.go"
        self.assert_rejected("Invalid evidence reference")

    def test_rejects_unknown_test_symbol(self):
        self.report["tests"][0]["symbol"] = "TestMissing"
        self.assert_rejected("Unknown test symbol")

    def test_rejects_symbol_on_wrong_path(self):
        self.report["tests"][0]["path"] = "internal/app/auth.go"
        self.assert_rejected("Unknown test symbol")

    def test_rejects_unknown_grouped_test_symbol(self):
        self.report["test_groups"][0]["tests"] = ["TestMissing"]
        self.assert_rejected("Unknown grouped test")


if __name__ == "__main__":
    unittest.main()
