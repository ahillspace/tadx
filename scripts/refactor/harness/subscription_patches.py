"""Add one exact authenticated-user Pulse subscription-list fixture."""

import copy


CASE = "P-pulse-subscription-list"
ACTION = "pulse.subscription.list"


def definitions(exercise_template, profile_template):
    exercise = copy.deepcopy(exercise_template)
    profile = copy.deepcopy(profile_template)
    assertions = [
        {"id": "complete-user-subscriptions", "actual": "/after/candidate/records",
         "op": "set_eq", "expected": {"ref": "/before/records"}},
        {"id": "owned-subscription-visible", "actual": "/after/native_command_verified",
         "op": "eq", "expected": True},
        {"id": "protected-preserved", "actual": "/after/protected",
         "op": "eq", "expected": {"ref": "/before/protected"}},
        {"id": "no-unowned-changes", "actual": "/after/unowned_changes",
         "op": "eq", "expected": []},
    ]
    exercise.update(id=CASE, category="pulse", tags=["base", "pulse", "subscription"],
                    capability_coverage=[{"capability": ACTION, "variant": "base"}],
                    fixture_specification="fixtures/profiles/" + CASE + ".json",
                    source_evidence=["actions/pulse/subscription/list.go", "internal/cli/pulse/command.go"])
    exercise["agent"] = {
        "prompt": ("Use the installed TADX CLI to list all Pulse subscriptions for your authenticated "
                   "user on ${public.site}. Include the exact subscription and metric IDs, follower identity, "
                   "and any available saved metric details. Use native JSON and full output. Do not alter follows."),
        "public_bindings": {"site": "fixture.public.site"},
        "authorization": "Read the authenticated user's Pulse subscriptions only; fixture setup owns one temporary follow.",
    }
    exercise["evaluator"].update(
        fixture_profile=ACTION, required_roles=["user", "definition", "metric", "datasource", "sentinel"],
        starting_state="A verified follow created by this run's agent principal is visible to the same-user observer.",
        dependencies=[],
        requirements=["cli_audit", "feedback_disabled", "frozen_official_skill_trees",
                      "frozen_pre_task_expectations", "independent_observer",
                      "installed_binary_fingerprint", "isolated_agent_filesystem",
                      "observable_required_variant", "strict_output_decoder",
                      "unchanged_mutation_policy", "action_write_request_audit"],
        intent={"operation": ACTION},
        allowed_effects={"remote": False, "local": False, "targets": "none", "fields": "none"},
        isolation={"leases": ["exact Pulse user and metric"],
                   "credential_policy": "three distinct process-only PATs; no persistence",
                   "sitewide_tests": "not applicable; complete user-filtered bounded inventory"},
        observations={"contract": "pulse_configuration", "before": True, "after": True,
                      "protected": "independent selected Pulse configuration and complete user-filtered inventory",
                      "candidate_report": "one audited native JSON list compared to independent API",
                      "raw_evidence_required": True},
        expected_outcomes=assertions,
        coverage_evidence={"required_actions": [ACTION], "required_variant": "base",
                           "source": "audited installed CLI argv, stdout, stderr and exit status",
                           "outcome_alone_is_not_coverage": True,
                           "command_predicates": [{"exact_pulse_action": ACTION, "all": True,
                                                   "json": True, "full": True}]},
        verification_recipe=["prove the run-owned agent follow with a same-user observer",
                             "audit one complete native user-filtered list",
                             "remove only the new follow and verify original inventory"],
    )
    profile.update(id=CASE, base_profile=ACTION,
                   roles=["user", "definition", "metric", "datasource", "sentinel"],
                   public_scalar_bindings=["site"], intent={"operation": ACTION},
                   starting_state_recipe=["prove exact agent and observer user identity",
                                          "create one run-owned follow with agent credentials",
                                          "freeze complete independent user-filtered inventory"],
                   independent_observation_contract="pulse_configuration",
                   required_raw_fields=["subscription_id", "metric_id", "principal_type", "principal_id"],
                   expected_outcome_assertions=copy.deepcopy(assertions),
                   normalized_assertion_fields=[row["actual"] for row in assertions],
                   implementation_binding="g9_pulse_subscription_profile.prepare, observe, cleanup")
    return {CASE: (exercise, profile)}


def patch_pulse_profile(source, replace_once):
    return replace_once(source,
        "         'pulse.metric.inspect', 'pulse.metric.followers'}",
        "         'pulse.metric.inspect', 'pulse.metric.followers', 'pulse.subscription.list'}")


def patch_pulse_broker(source, replace_once):
    source = replace_once(source,
        "'pulse.metric.list', 'pulse.metric.followers']);",
        "'pulse.metric.list', 'pulse.metric.followers', 'pulse.subscription.list']);")
    return replace_once(source,
        "  if (reads.has(cap)) {\n",
        "  if (reads.has(cap)) {\n"
        "    if (cap === 'pulse.subscription.list') return flags.all === true && flags.json === true && flags.full === true && only(flags, ['all']);\n")
