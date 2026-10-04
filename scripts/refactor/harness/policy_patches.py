"""Generate three local-only policy cases in the blocked copied suite."""

import copy


POLICY_CASES = ["P-policy-" + verb for verb in ("samples", "validate", "status")]


def policy_definitions(exercise_template, profile_template):
    result = {}
    prompts = {
        "policy.samples": ("Create the three non-activated policy samples only at "
                           "${public.output_path}. Report what was created."),
        "policy.validate": ("Validate only the candidate at ${public.candidate_path}. "
                            "Report its validity without activating it."),
        "policy.status": ("Inspect the local machine policy status and report whether "
                          "a managed policy is active. Do not install one."),
    }
    for case_id in POLICY_CASES:
        action = case_id.removeprefix("P-").replace("-", ".")
        exercise = copy.deepcopy(exercise_template)
        profile = copy.deepcopy(profile_template)
        assertions = [
            {"id": "native-policy-contract", "actual": "/after/policy_contract_passed",
             "op": "eq", "expected": True},
            {"id": "expected-policy-result", "actual": "/after/native_result_status",
             "op": "eq", "expected": {"policy.samples": "created",
                                      "policy.validate": "valid",
                                      "policy.status": "unmanaged"}[action]},
            {"id": "protected-preserved", "actual": "/after/protected_sha256",
             "op": "eq", "expected": {"ref": "/before/protected_sha256"}},
            {"id": "no-unowned-changes", "actual": "/after/unowned_changes",
             "op": "eq", "expected": []},
        ]
        public = {"output_path": "fixture.public.output_path"} if action == "policy.samples" else (
            {"candidate_path": "fixture.public.candidate_path"} if action == "policy.validate" else {})
        exercise.update(id=case_id, category="policy", tags=["base", "policy", "local"],
                        capability_coverage=[{"capability": action, "variant": "base"}],
                        fixture_specification="fixtures/profiles/" + case_id + ".json",
                        source_evidence=["actions/policy/" + action.split(".")[1] + ".go",
                                         "internal/cli/policy/command.go"])
        exercise["agent"] = {
            "prompt": (prompts[action] + " Use the installed TADX CLI with JSON and full output. "
                       "Work only in the disposable local area. Do not contact Tableau, "
                       "change credentials or configuration, or install software."),
            "public_bindings": public,
            "authorization": "Only this disposable local policy diagnostic is in scope.",
        }
        evaluator = exercise["evaluator"]
        evaluator.update(fixture_profile=action, required_roles=["sentinel"],
                         starting_state=("Require a pinned network-none worker with no machine policy, "
                                         "an independent local baseline, and exact CLI image before allocation."),
                         dependencies=[],
                         requirements=["cli_audit", "feedback_disabled", "frozen_official_skill_trees",
                                       "frozen_pre_task_expectations", "independent_observer",
                                       "installed_binary_fingerprint", "isolated_agent_filesystem",
                                       "observable_required_variant", "strict_output_decoder",
                                       "unchanged_mutation_policy", "action_write_request_audit"],
                         intent={"kind": "local_policy", "action": action},
                         allowed_effects={"remote": False, "local": action == "policy.samples",
                                          "targets": "only the run-owned policy sample directory or candidate",
                                          "fields": "only the requested local result"},
                         isolation={"leases": [], "credential_policy": "no credential input or persistence",
                                    "sitewide_tests": "not applicable; CLI network is disabled"},
                         observations={"contract": "local_policy", "before": True, "after": True,
                                       "protected": "bounded no-follow local snapshot and sentinel",
                                       "candidate_report": "native JSON plus independent local observation",
                                       "raw_evidence_required": True},
                         expected_outcomes=assertions,
                         coverage_evidence={"required_actions": [action], "required_variant": "base",
                                            "source": "audited installed CLI argv, stdout, stderr and exit status",
                                            "outcome_alone_is_not_coverage": True,
                                            "command_predicates": [{"exact_policy_action": action}]},
                         verification_recipe=["verify one exact native policy command",
                                              "compare files and configuration with independent baseline",
                                              "verify network-none isolation and local cleanup"])
        profile.update(id=case_id, base_profile=action, roles=["sentinel"],
                       public_scalar_bindings=list(public),
                       intent={"kind": "local_policy", "action": action},
                       starting_state_recipe=["pin the accepted worker image and exact CLI binary",
                                              "independently prove no machine policy in the disposable image",
                                              "seed only a run-owned candidate and sentinel, then freeze baseline"],
                       independent_observation_contract="local_policy",
                       required_raw_fields=["native.argv", "native.stdout", "native.exit_status",
                                            "local.before", "local.after", "network.mode"],
                       expected_outcome_assertions=copy.deepcopy(assertions),
                       normalized_assertion_fields=[row["actual"] for row in assertions],
                       implementation_binding=("g9_policy_profile.prepare, observe, cleanup; image "
                                               "and runtime qualification remain required before dispatch"))
        result[case_id] = exercise, profile
    return result
