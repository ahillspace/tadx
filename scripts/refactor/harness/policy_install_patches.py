"""Generate one protected policy-install case for an isolated root overlay."""

import copy

CASE = "P-policy-install"
ACTION = "policy.install"


def definitions(exercise_template, profile_template):
    exercise = copy.deepcopy(exercise_template)
    profile = copy.deepcopy(profile_template)
    assertions = [
        {"id": "native-protected-install", "actual": "/after/native_command_verified",
         "op": "eq", "expected": True},
        {"id": "active-protected-policy", "actual": "/after/policy_active",
         "op": "eq", "expected": True},
        {"id": "protected-preserved", "actual": "/after/protected_sha256",
         "op": "eq", "expected": {"ref": "/before/protected_sha256"}},
        {"id": "no-unowned-changes", "actual": "/after/unowned_changes",
         "op": "eq", "expected": []},
    ]
    exercise.update(id=CASE, category="policy", tags=["base", "policy", "protected-install"],
                    capability_coverage=[{"capability": ACTION, "variant": "base"}],
                    fixture_specification="fixtures/profiles/" + CASE + ".json",
                    source_evidence=["actions/policy/install.go", "internal/managedpolicy/install_unix.go",
                                     "internal/cli/policy/command.go"])
    exercise["agent"] = {
        "prompt": ("Install and activate the read-only managed policy inside this disposable Linux "
                   "worker at ${public.output_path}. Use the installed TADX CLI with JSON and full output. "
                   "This worker has no Tableau access or host policy mount. Do not use another path or template."),
        "public_bindings": {"output_path": "fixture.public.output_path"},
        "authorization": "Only the isolated broker root overlay may receive the protected policy installation.",
    }
    exercise["evaluator"].update(
        fixture_profile=ACTION, required_roles=["sentinel"],
        starting_state="Pinned network-none broker overlay has no machine policy or protected locator.",
        dependencies=[],
        requirements=["cli_audit", "feedback_disabled", "frozen_official_skill_trees",
                      "frozen_pre_task_expectations", "independent_observer",
                      "installed_binary_fingerprint", "isolated_agent_filesystem",
                      "observable_required_variant", "strict_output_decoder",
                      "unchanged_mutation_policy", "action_write_request_audit"],
        intent={"kind": "protected_policy_install", "action": ACTION},
        allowed_effects={"remote": False, "local": True,
                         "targets": "only /etc/tadx and /etc/tadx-policy-location.json inside stopped broker overlay",
                         "fields": "read-only managed policy and its exact locator"},
        isolation={"leases": [], "credential_policy": "no credential input or persistence",
                   "sitewide_tests": "not applicable; CLI network is disabled"},
        observations={"contract": "protected_policy_install", "before": True, "after": True,
                      "protected": "stopped broker overlay diff, fixed policy files, and local sentinel",
                      "candidate_report": "native receipt plus independent stopped-overlay copy",
                      "raw_evidence_required": True},
        expected_outcomes=assertions,
        coverage_evidence={"required_actions": [ACTION], "required_variant": "base",
                           "source": "audited installed CLI argv, stdout, stderr and exit status",
                           "outcome_alone_is_not_coverage": True,
                           "command_predicates": [{"exact_policy_action": ACTION}]},
        verification_recipe=["prove immutable worker image has no policy",
                             "audit one root-only native install in network-none broker overlay",
                             "copy and inspect only fixed protected paths after stop",
                             "destroy the exact labeled broker overlay and verify absence"],
    )
    profile.update(id=CASE, base_profile=ACTION, roles=["sentinel"],
                   public_scalar_bindings=["output_path"],
                   intent={"kind": "protected_policy_install", "action": ACTION},
                   starting_state_recipe=["prove no protected policy in pinned base image",
                                          "create writable root overlay only for exact broker",
                                          "keep model worker read-only and separate"],
                   independent_observation_contract="protected_policy_install",
                   required_raw_fields=["native.argv", "native.stdout", "native.exit_status",
                                        "overlay.diff", "locator", "policy"],
                   expected_outcome_assertions=copy.deepcopy(assertions),
                   normalized_assertion_fields=[row["actual"] for row in assertions],
                   implementation_binding="g9_policy_install_profile.prepare, observe, cleanup")
    return {CASE: (exercise, profile)}
