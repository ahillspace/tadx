# Evaluate agent workflow speed

## Observed Luna baseline

The completed primary-agent trials report these approximate totals before the skill revision.
Counts come from trial observations, not a new instrumented rerun; token accounting is the reported aggregate.

| Trial | Action attempts | Help/capability probes | Elapsed minutes | Reported tokens |
| --- | ---: | ---: | ---: | ---: |
| S1 | 22 | 17 | 7 | 87,700 |
| S2 | 18 | 10 | 7 | 120,000 |
| S3 | 25 | 15 | 11 | 179,000 |
| S4 | 31 | 9 | 9 | 125,000 |

S2 also delegated a bounded read-only operation unnecessarily.
Repeated help discovery, redundant environment/authentication chains, and tool-call overhead are the targeted sources of delay.
These observations do not isolate model latency from upstream latency or establish a post-change improvement.

## Acceptance target

Each representative Luna workflow finishes in less than 120 seconds, excluding separately measured long upstream jobs.
Keep ordinary authentication, request latency, agent reasoning, shell dispatch, and verification inside the timed interval.
Report raw elapsed time and excluded job seconds alongside adjusted elapsed time; never exclude failed attempts or help probes.
Start with known environment/workspace configuration and disposable targets authorized for the specific changes.

| Representative workflow | Command-attempt target | Required verified outcome |
| --- | ---: | --- |
| Known workbook pull, publish, and inspection | At most 8 | Managed artifact, exact destination identity, verified published state |
| Scoped catalog, flow discovery, and bounded lineage | At most 6 | Correct scope/source, selected flow LUID, bounded lineage artifact |
| Principal/rule inspection and one authorized administrative or project change | At most 7 | Exact target, intended state, preserved unrelated state |
| Published datasource to one Pulse definition and verification | At most 8 | Evidence-backed fields/semantics, definition LUID, default metric LUID |

Each workflow uses zero broad help probes and at most one leaf-help or exact-capability probe.
Count every TADX process, including retries, previews, auth checks, and probes, toward the command target; also report probes separately.
Track shell tool calls separately from TADX commands so sequential batching does not hide extra commands.
Run authenticated calls sequentially for a shared PAT, including across agents.
Use identical tasks, model/reasoning settings, source data, and upstream exclusion rules for baseline and revised runs.
Record the binary revision and bundled-skill digest; keep cold-configuration setup separate from these known-target workflows.

## Record a rerun

Record one row per scenario and repeat each scenario three times:

| Scenario/run | Binary/skill revision | Raw seconds | Long-job seconds | Adjusted seconds | Command attempts | Shell calls | Leaf/exact probes | Broad probes | Tokens | Correct outcome |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |

A scenario passes when all three runs meet correctness, safety, latency, and command/probe targets.
Use disposable live targets only under explicit task authorization; otherwise label the run a local rehearsal and leave live acceptance unverified.
Do not convert an upstream eligibility/access failure into repeated writes to satisfy a time target.
CLI flag/link validation and installer tests verify the bundle structure and recipes; they do not establish the two-minute behavioral target.

## Fresh-trial corrections

Follow-up Luna trials identify redundant alias validation, missing project/group recipes, ambiguous datasource names, and table-name schema matches as remaining overhead.
Bundled guidance now uses supplied aliases verbatim, supplies project-create/group-list recipes, and resolves datasource ambiguity through project paths or LUIDs.
Multiple schema matches do not trigger broader pagination when a semantically exact field is already available.
Pulse authoring checks existing datasource definitions before creation and avoids retrying duplicate semantic payloads.
Default-metric removal requires authorized definition deletion; metric deletion applies only to non-default variants.
These are observed-failure corrections, not evidence that the latency target passes; retain the rerun protocol above.

A subsequent administrative rerun succeeds in 68 seconds but repeats permission inspection to obtain full rule details.
The verification recipe now requests `--principal-id <principal-luid> --full` immediately, avoiding that extra command.
This single run does not establish the three-run acceptance target.

The second Pulse rerun takes 129.9 seconds, exceeding the 120-second target.
The next revision removes the obsolete table-name schema-query caveat, uses direct datasource inspection for known name/project selectors, and avoids rereading unchanged forks.
Successful deletes need no confirmation list unless the outcome is uncertain or verification is explicitly requested.
Two fresh Codex agents first look for skills under `.codex/skills`, costing a model turn each; Codex installation now targets that recognized runtime root.
The installer preserves existing `.agents/skills` packages without legacy cleanup.

## Post-change live sample

One fresh Luna agent completed each representative live workflow against the authorized disposable development site.
No long-job time was excluded.

| Workflow | Raw seconds | Result |
| --- | ---: | --- |
| Content search, inspect, workspace create, and workbook pull | 73.5 | Correct |
| Catalog refresh, catalog flow discovery, workspace create, and bounded lineage pull | 33.3 | Correct |
| Project create/update, group resolution, permission create/verify/delete, and project delete | 68.1 | Correct |
| Datasource/field discovery, Pulse definition create/verify, non-default metric fork/delete, and definition delete | 106.0 | Correct |

All four live samples meet the 120-second latency target.
The expanded Pulse sample used more commands than the narrower definition-only target because it also tested metric fork and both delete actions.
These single samples establish a successful smoke result, not the three-run hardening threshold defined above.
