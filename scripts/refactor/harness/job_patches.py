"""Fail-closed exact-job broker integration for the blocked copied suite."""

import copy


JOB_CASES = ["P-job-" + action for action in ("inspect", "wait", "cancel")]


def job_definitions(exercise_template, profile_template):
    """Generate explicit exact-job cases without changing the locked suite."""
    definitions = {}
    for case_id in JOB_CASES:
        action = case_id.removeprefix("P-").replace("-", ".")
        exercise = copy.deepcopy(exercise_template)
        profile = copy.deepcopy(profile_template)
        exercise.update(id=case_id, category="jobs", tags=["base", "jobs", "standard"],
                        capability_coverage=[{"capability": action, "variant": "base"}],
                        fixture_specification="fixtures/profiles/" + case_id + ".json",
                        source_evidence=["actions/job/" + action.split(".")[1] + ".go",
                                         "internal/cli/job/command.go"])
        verb = {"job.inspect": "Inspect", "job.wait": "Wait for the terminal result of",
                "job.cancel": "Cancel"}[action]
        exercise["agent"] = {
            "prompt": (f"{verb} only Tableau job ${{public.job_id}} on ${{public.site}}. "
                       "Report the authoritative job ID and status at deliverables/result.json. "
                       "Use the installed TADX CLI. Work only in the supplied run scope and "
                       "disposable local area. Do not change unrelated resources or local credentials."),
            "public_bindings": {"job_id": "fixture.public.job_id", "site": "fixture.public.site"},
            "authorization": ("Only this exact run-owned job is in scope. The supplied effective mutation "
                              "policy is already configured; do not change it.")}
        evaluator = exercise["evaluator"]
        evaluator.update(fixture_profile=action, required_roles=["job", "owned_datasource", "sentinel"],
                         starting_state=("Require journaled ownership, one accepted refresh response, "
                                         "and a fresh independent exact-job observation before model allocation."),
                         intent={"kind": "exact_job", "action": action},
                         allowed_effects={"remote": action == "job.cancel", "local": True,
                                          "targets": "only the exact journal-owned job ID",
                                          "fields": "only the requested job outcome"},
                         observations={"contract": "exact_job", "before": True, "after": True,
                                       "protected": "independent non-target state digest and sentinel",
                                       "candidate_report": "deliverables/result.json",
                                       "raw_evidence_required": True},
                         expected_outcomes=[
                             {"id": "exact-native-and-observer-contract", "actual": "/after/job_contract_passed",
                              "op": "eq", "expected": True},
                             {"id": "authoritative-job-status", "actual": "/after/job_status", "op": "eq",
                              "expected": {"ref": "/fixture/expect/job_status"}},
                             {"id": "protected-preserved", "actual": "/after/protected_sha256", "op": "eq",
                              "expected": {"ref": "/before/protected_sha256"}},
                             {"id": "no-unowned-changes", "actual": "/after/unowned_changes", "op": "eq",
                              "expected": []}],
                         coverage_evidence={"required_actions": [action], "required_variant": "base",
                                            "source": "audited installed CLI argv, stdout, stderr and exit status",
                                            "outcome_alone_is_not_coverage": True,
                                            "command_predicates": [{"exact_job_id": "fixture.public.job_id"}]},
                         verification_recipe=["verify one native exact-target command against a distinct observer",
                                              "verify no unowned state changed and cleanup by journaled IDs"])
        evaluator["requirements"] = sorted(set(evaluator["requirements"]) | {"action_write_request_audit"}) if action == "job.cancel" else evaluator["requirements"]
        if action == "job.wait":
            evaluator["expected_outcomes"] = [row for row in evaluator["expected_outcomes"]
                                               if row["id"] != "authoritative-job-status"]
        profile.update(id=case_id, base_profile=action, roles=["job", "owned_datasource", "sentinel"],
                       public_scalar_bindings=["job_id", "site"],
                       intent={"kind": "exact_job", "action": action},
                       independent_observation_contract="exact_job",
                       required_raw_fields=["job.id", "job.type", "job.status", "job.progress",
                                            "accepted_refresh.http_status", "protected_sha256"],
                       starting_state_recipe=["journal one run-owned datasource and distinct credential roles",
                                              "retain one accepted HTTP 202 refresh response and exact job ID",
                                              "independently observe the same job before model allocation"],
                       expected_outcome_assertions=copy.deepcopy(evaluator["expected_outcomes"]),
                       normalized_assertion_fields=[row["actual"] for row in evaluator["expected_outcomes"]],
                       implementation_binding=("g9_job_profile.prepare, observe, cleanup; external exact-job "
                                               "fixture worker and protected-state/cleanup observer remain required"))
        definitions[case_id] = (exercise, profile)
    return definitions


def replace_once(source, before, after):
    if source.count(before) != 1:
        raise ValueError("Exact-job patch anchor differs from reviewed source")
    return source.replace(before, after, 1)


def patch_job_broker(source):
    """Route job guards before every disposable-native admission path."""
    source = replace_once(
        source,
        "const projectBroker = require('./project_broker.cjs');",
        "const projectBroker = require('./project_broker.cjs');\n"
        "const g9JobBroker = require('./g9_job_broker.cjs');",
    )
    source = replace_once(
        source,
        "  if(state.guard?.execution_mode==='disposable_native')return false;",
        "  if(state.guard?.family==='job')return g9JobBroker.allowed(parsed,classify(args,state.registry||registry),state);\n"
        "  if(state.guard?.execution_mode==='disposable_native')return false;",
    )
    source = replace_once(
        source,
        "  if(state.guardPresent)try{state.guard=JSON.parse(fs.readFileSync('/cli-state/task-policy.json','utf8'));}catch{}\n  return state;",
        "  if(state.guardPresent)try{state.guard=JSON.parse(fs.readFileSync('/cli-state/task-policy.json','utf8'));}catch{}\n"
        "  if(state.guard?.family==='job'){\n"
        "    state.jobOwned=state.guard.preflight_verified===true;\n"
        "    state.jobFresh=['pending','running'].includes(state.guard.before_status);\n"
        "    state.cancelAttempted=fs.existsSync('/cli-state/g9-job-cancel-attempted');\n"
        "  }\n  return state;",
    )
    source = replace_once(
        source,
        "    let result, executed=allowed(args,policyState);",
        "    let result, executed=allowed(args,policyState);\n"
        "    if(executed&&policyState.guard?.family==='job'&&classify(args)==='job.cancel'"
        "&&flagsOf(args).preview!==true){\n"
        "      try{fs.writeFileSync('/cli-state/g9-job-cancel-attempted',id,{flag:'wx',mode:0o600});}\n"
        "      catch{executed=false;}\n"
        "    }",
    )
    return source


def patch_job_image(source):
    return replace_once(
        source,
        "broker_files=(*broker_files,'fault_broker.cjs','credential_pty.py','g9_project_read_broker.cjs')",
        "broker_files=(*broker_files,'fault_broker.cjs','credential_pty.py','g9_project_read_broker.cjs','g9_job_broker.cjs')",
    )
