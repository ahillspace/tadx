"""Hermetic tests for the two selected mutation cases in the 131-case union."""

import unittest

import prepare as prep


def pair(case):
    action = case.removeprefix("P-").replace("-", ".")
    exercise = {"id": case, "agent": {"prompt": "synthetic source task"},
                "evaluator": {"fixture_profile": action, "intent": {"saved_enabled": True},
                              "expected_outcomes": [], "oracle_requirements": []}}
    profile = {"id": case, "base_profile": action,
               "intent": {"saved_enabled": True}, "expected_outcome_assertions": [],
               "normalized_assertion_fields": [],
               "expected_value_bindings_required_before_launch": []}
    return exercise, profile


class MutationUnionTests(unittest.TestCase):
    def test_only_selected_p_pairs_receive_reviewed_definitions(self):
        files = {}
        for case in ("P-mutation-set", "P-mutation-status"):
            exercise, profile = pair(case)
            files["suite/exercises/" + case + ".json"] = prep.encode(exercise)
            files["fixtures/profiles/" + case + ".json"] = prep.encode(profile)
        patches = prep.apply_mutation_definitions(files)
        self.assertEqual(len(patches), 4)
        self.assertEqual({item["path"] for item in patches}, set(files))
        for case in ("P-mutation-set", "P-mutation-status"):
            exercise = prep.parse(files["suite/exercises/" + case + ".json"])
            profile = prep.parse(files["fixtures/profiles/" + case + ".json"])
            self.assertEqual(exercise["evaluator"]["intent"]["site_content_url"],
                             "session-fixture")
            self.assertEqual(profile["independent_observation_contract"],
                             "exact_site_mutation_consent")

    def test_partial_or_unselected_mutation_pair_fails_closed(self):
        exercise, profile = pair("P-mutation-set")
        files = {"suite/exercises/P-mutation-set.json": prep.encode(exercise),
                 "fixtures/profiles/P-mutation-set.json": prep.encode(profile)}
        with self.assertRaisesRegex(ValueError, "incomplete"):
            prep.apply_mutation_definitions(files)
        files = {"suite/exercises/V-mutation-env-precedence.json": b"{}"}
        with self.assertRaisesRegex(ValueError, "Unselected"):
            prep.apply_mutation_definitions(files)

    def test_full_source_requires_all_three_reviewed_mutation_owners(self):
        files = {"suite/exercises/P-case-" + str(index) + ".json": b"{}"
                 for index in range(116)}
        files["integration/docker_local_bridge.py"] = b"synthetic original"
        with self.assertRaisesRegex(ValueError, "incomplete"):
            prep.apply_mutation_runtime(files)
        files["integration/session_broker.cjs"] = b"synthetic original"
        files["integration/session_profiles.py"] = b"synthetic original"
        patches = prep.apply_mutation_runtime(files)
        self.assertEqual({item["path"] for item in patches},
                         {"integration/docker_local_bridge.py",
                          "integration/session_broker.cjs",
                          "integration/session_profiles.py"})
        for item in patches:
            self.assertEqual(prep.sha(files[item["path"]]), item["after_sha256"])


if __name__ == "__main__":
    unittest.main()
