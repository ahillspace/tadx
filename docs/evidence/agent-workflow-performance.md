# Historical agent workflow performance

This record preserves observations from the Guidance evaluation campaign.
It is not a current run protocol or evidence that later revisions reproduce these results.

## Observed Luna baseline

The completed primary-agent trials report these approximate totals before the Guidance revision.
Counts come from trial observations, not a new instrumented rerun; token accounting is the reported aggregate.

| Trial | Action attempts | Help/capability probes | Elapsed minutes | Reported tokens |
| --- | ---: | ---: | ---: | ---: |
| S1 | 22 | 17 | 7 | 87,700 |
| S2 | 18 | 10 | 7 | 120,000 |
| S3 | 25 | 15 | 11 | 179,000 |
| S4 | 31 | 9 | 9 | 125,000 |

S2 also delegated a bounded read-only operation unnecessarily.
Repeated help discovery, redundant environment/authentication chains, and tool-call overhead were identified as sources of delay.
These observations do not isolate model latency from upstream latency or establish a post-change improvement.

## Historical comparison limits

The campaign's latency target was less than 120 seconds per representative workflow, excluding separately measured long upstream jobs.
The proposed acceptance threshold required three successful repetitions per scenario, including correctness and command/probe targets.
The narrower definition-only Pulse target allowed at most eight command attempts.
The samples below do not establish that repeated-run threshold.
CLI flag/link validation and installer tests verify bundle structure and recipes, not the behavioral latency target.

## Fresh-trial corrections

Follow-up Luna trials identified redundant alias validation, missing project/group recipes, ambiguous datasource names, and table-name schema matches as remaining overhead.
The revisions addressed supplied-alias reuse, project and permission recipes, exact datasource selection, bounded detail retrieval, and existing Pulse definition discovery.
They also clarified default-metric deletion through definition deletion and removed redundant discovery steps.
These corrections did not establish that the latency target passed.

A subsequent administrative rerun succeeded in 68 seconds but repeated permission inspection to obtain full rule details.
The revised recipe combined principal selection with full rule details to remove that extra command.
This single run does not establish the three-run acceptance target.

The second Pulse rerun took 129.9 seconds, exceeding the 120-second target.
Subsequent revisions addressed direct datasource inspection, unchanged-fork rereads, and redundant deletion checks.
Two fresh Codex agents first looked for Guidance under `.codex/skills`, costing a model turn each.
The installer revision targeted that runtime root and added migration of recognized legacy packages with recoverable backups for forced replacement of divergent packages.

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
These single samples establish a successful smoke result, not the campaign's three-run acceptance threshold.
