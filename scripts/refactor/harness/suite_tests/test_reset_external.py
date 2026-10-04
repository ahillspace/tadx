"""Exercise exact copied reset behavior with separately pinned offline tests."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


HERE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(HERE))
import prepare  # noqa: E402
from reset_absence_patches import (patch_cleanup_config_tests, patch_content_operator_tests,
                                   patch_operator_reset_tests, patch_reset_site_tests)


EXTERNAL_TESTS = {
    "tests/test_reset_site.py": (
        "b61302f8700004561976834524e63f0db7f5e1be2c427cb81679a831dc53db0b",
        patch_reset_site_tests,
    ),
    "tests/test_operator_reset.py": (
        "23dd3a01a544ec02dd431081faefe4b4bc64a960e9ad9f726949c03e53bb73d4",
        patch_operator_reset_tests,
    ),
    "tests/test_cleanup_config.py": (
        "5304634c268214d320366dc5bb905e96eddad603d26f5cb41a050e08efe5ef61",
        patch_cleanup_config_tests,
    ),
    "tests/test_content_operator_profiles.py": (
        "af04c1b5f24b825e1472ab3457277fcd1db6802495195e53215cc4af6ba5300a",
        patch_content_operator_tests,
    ),
}


class ResetExternalTests(unittest.TestCase):
    def test_scoped_original_reset_contracts_run_only_in_hash_pinned_offline_copy(self):
        supplied = os.environ.get("TADX_G9_CURRENT_SUITE")
        if not supplied:
            raise RuntimeError("Exact current source-locked suite is required")
        original = Path(supplied).resolve(strict=True)
        lock = json.loads((HERE / "source-lock-124.json").read_text(encoding="utf-8"))
        self.assertEqual(len(lock["files"]), 373)
        files = {}
        for name, expected in lock["files"].items():
            blob = (original / name).read_bytes()
            self.assertEqual(prepare.sha(blob), expected, name)
            files[name] = blob
        prepare.apply_mutation_runtime(files)
        prepare.patch_sources(files)
        self.assertTrue(all(name not in files for name in EXTERNAL_TESTS))
        for name, (expected, transform) in EXTERNAL_TESTS.items():
            blob = (original / name).read_bytes()
            self.assertEqual(prepare.sha(blob), expected, name)
            source = transform(blob.decode("utf-8").replace("\r\n", "\n"),
                               prepare.replace_once)
            if name == "tests/test_operator_reset.py":
                source = prepare.replace_once(
                    source,
                    "        for concurrency in (2, True):\n",
                    "        for concurrency in (True, 1.0):\n",
                )
                source = prepare.replace_once(
                    source,
                    "with self.assertRaisesRegex(OperatorResetError, 'serial execution'):",
                    "with self.assertRaisesRegex(OperatorResetError, 'bounded parallel execution setting'):",
                )
            if name == "tests/test_cleanup_config.py":
                source = prepare.replace_once(
                    source,
                    "        for concurrency in (2, True, '1'):\n",
                    "        bounded = copy.deepcopy(self.config)\n"
                    "        bounded['runtime']['concurrency'] = 2\n"
                    "        advanced = apply_cleanup_config(bounded, response)\n"
                    "        self.assertEqual(advanced['runtime']['concurrency'], 2)\n"
                    "        self.assertEqual(advanced['deployment']['operator_settings'],\n"
                    "                         apply_cleanup_config(self.config, response)['deployment']['operator_settings'])\n"
                    "        for concurrency in (True, '1'):\n",
                )
            files[name] = source.encode()
        with tempfile.TemporaryDirectory(prefix="g9-reset-tests-") as temporary:
            root = Path(temporary)
            for name, blob in files.items():
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(blob)
            home = root / "offline-home"
            home.mkdir()
            env = {
                "PYTHONPATH": str(root), "PYTHONDONTWRITEBYTECODE": "1",
                "HOME": str(home), "USERPROFILE": str(home),
                "TEMP": str(home), "TMP": str(home),
            }
            if os.name == "nt":
                env["SystemRoot"] = os.environ.get("SystemRoot", "C:\\Windows")
            selected = (
                "test_reset_site",
                "test_operator_reset.OperatorResetTests.test_verified_reset_saves_new_ids_to_both_files_and_preserves_other_fields",
                "test_operator_reset.OperatorResetTests.test_unknown_publish_cannot_be_replayed_or_saved_as_a_success",
                "test_operator_reset.OperatorResetTests.test_owned_cleanup_is_exact_and_unproven_cleanup_cannot_run",
                "test_operator_reset.OperatorResetTests.test_cleanup_failure_prevents_fixture_deletion_and_id_saves",
                "test_operator_reset.OperatorResetTests.test_public_adapter_rejects_nonserial_settings_and_unknown_operations",
                "test_cleanup_config.CleanupConfigTests.test_selective_reset_replaces_one_kind_and_advances_full_snapshot_from_preserved_phases",
                "test_cleanup_config.CleanupConfigTests.test_native_null_empty_list_requires_explicit_complete_zero_page_before_publish",
                "test_cleanup_config.CleanupConfigTests.test_incomplete_or_quarantined_content_restore_cannot_advance_ids",
                "test_cleanup_config.CleanupConfigTests.test_site_runtime_other_targets_and_credential_refs_cannot_change",
                "test_content_operator_profiles.ContentOperatorProfilesTests.test_scoped_delete_recovery_preserves_other_content_ids_and_native_resources",
                "test_content_operator_profiles.ContentOperatorProfilesTests.test_cleanup_failure_quarantines_and_never_reports_a_model_failure",
                "test_content_operator_profiles.ContentOperatorProfilesTests.test_publish_cleanup_requires_exact_audited_creation_and_removes_only_owned_id",
                "test_content_operator_profiles.ContentOperatorProfilesTests.test_ambiguous_publish_outcomes_are_quarantined_without_deleting_a_candidate",
                "test_reset_absence",
            )
            result = subprocess.run(
                [sys.executable, "-B", "-m", "unittest", *selected], cwd=root / "tests", env=env,
                capture_output=True, text=True, encoding="utf-8", timeout=90, check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == "__main__":
    unittest.main()
