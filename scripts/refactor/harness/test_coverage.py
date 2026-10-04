"""Synthetic integrity checks for the required-action inventory."""

import json
from pathlib import Path
import tempfile
import unittest

import coverage


class CoverageTests(unittest.TestCase):
    def test_current_lock_extends_provenance_without_extra_mutation_variants(self):
        owner = Path(__file__).resolve().parent
        original = json.loads((owner / "source-lock.json").read_text(encoding="utf-8"))["files"]
        current = json.loads((owner / "source-lock-124.json").read_text(encoding="utf-8"))["files"]
        self.assertEqual(len(original), 118)
        self.assertEqual(len(current), 373)
        self.assertTrue(all(current.get(name) == value for name, value in original.items()))
        self.assertEqual(sum(name.startswith("suite/exercises/V-installer-windows-")
                             or name.startswith("fixtures/profiles/V-installer-windows-")
                             for name in current), 14)
        self.assertFalse(any("V-mutation-" in name for name in current))

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "suite/exercises").mkdir(parents=True)
        (self.root / "fixtures/profiles").mkdir(parents=True)
        self.entries = []
        self.locked = {}
        for name in ("one", "two"):
            case = "P-fixture-" + name
            exercise_path = "suite/exercises/" + case + ".json"
            profile_path = "fixtures/profiles/" + case + ".json"
            exercise = json.dumps({"id": case}).encode()
            profile = json.dumps({"id": case, "base_profile": "fixture." + name,
                                  "independent_observation_contract": "synthetic"}).encode()
            (self.root / exercise_path).write_bytes(exercise)
            (self.root / profile_path).write_bytes(profile)
            self.entries.append({"id": case, "path": exercise_path, "category": "fixture"})
            if name == "one":
                self.locked[exercise_path] = coverage._hash(exercise)
                self.locked[profile_path] = coverage._hash(profile)
        self.write_index()
        self.lock = self.root / "source-lock.json"
        self.lock.write_text(json.dumps({"files": self.locked}), encoding="utf-8")

    def write_index(self):
        (self.root / "suite/index.json").write_text(json.dumps({"exercises": self.entries}),
                                                     encoding="utf-8")

    def test_exact_action_set_preserves_unqualified_and_unwritten_gaps(self):
        result = coverage.build(self.root, self.lock, expected_existing=2)
        self.assertEqual(result["action_count"], 10)
        rows = {row["id"]: row for row in result["actions"]}
        self.assertTrue(rows["P-fixture-one"]["source_locked"])
        self.assertFalse(rows["P-fixture-two"]["source_locked"])
        self.assertEqual(rows["P-policy-install"]["fixture_state"], "offline_authored_blocked")
        self.assertEqual(rows["P-pulse-subscription-list"]["fixture_state"], "offline_authored_blocked")
        self.assertTrue(all(row["fixture_state"] != "qualified" for row in result["actions"]))

    def test_duplicate_or_changed_case_rejected(self):
        self.entries[1] = dict(self.entries[0])
        self.write_index()
        with self.assertRaisesRegex(ValueError, "base-action set changed"):
            coverage.build(self.root, self.lock, expected_existing=2)
        self.entries[1] = {"id": "P-fixture-two", "path": "suite/exercises/elsewhere.json",
                           "category": "fixture"}
        self.write_index()
        with self.assertRaisesRegex(ValueError, "path is not exact"):
            coverage.build(self.root, self.lock, expected_existing=2)


if __name__ == "__main__":
    unittest.main()
