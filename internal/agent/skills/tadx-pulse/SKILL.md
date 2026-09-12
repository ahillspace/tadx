---
name: tadx-pulse
description: Author and manage Tableau Pulse definitions with TADX, including business meaning, measure and slicer selection, creation, saved configuration verification, metric variants, and followers.
---

# Author and manage Tableau Pulse metrics

Before starting a TADX Pulse task, read the installed `tadx` root skill at `../tadx/SKILL.md`, beside this skill's directory.
It covers general discovery, datasource schema, caches, workspaces, and administration; consult its relevant references before assuming a capability is missing from TADX.
Use `tadx pulse --help` once for all Pulse commands, flags, and examples, including nested definition and metric actions.
Leaf help is an optional focused lookup; the references below retain the authoring judgment and verification contracts that command syntax alone does not provide.

A definition owns the shared measure, aggregation, date, allowed dimensions, and display settings.
A metric is a period and population variant of that definition; Tableau creates a default metric with each new definition.
Followers subscribe to metrics, and insights are computed analytical results.
This Guidance covers TADX authoring and lifecycle commands, respecting the user's chosen tools for other work.
TADX configuration reads do not return current metric values or generated insights.

## Read the relevant reference before acting

| Task | Read first |
| --- | --- |
| Recommend KPIs, select fields or slicers, or design a definition | [Authoring](references/authoring.md) |
| Create a definition or review its preview | [Authoring](references/authoring.md) and [Creation contract](references/authoring-contract.md) |
| Choose a calculation, entity count, average, rate, percentage, snapshot, or measure spanning multiple facts | Also read [Semantics](references/semantics.md) before finalizing that choice |
| Fork a metric or satisfy a particular period or filtered population | [Variants](references/variants.md), before creating a base that depends on that choice |
| Resolve names, list, inspect, pull or publish a portable bundle, follow, unfollow, delete, or assess an edit | [Operations](references/operations.md) |
| Verify saved settings or retain configuration context for numeric validation | [Configuration context](references/analytics-context.md) |

Read only the routes needed for the task.
Authoring includes its discovery commands, so those reads do not require loading the operations route too.
Keep needed guidance and verified choices available across task handoffs.

## Shared rules

Use exact identities in the selected environment; datasource, field, definition, metric, user, and group IDs are different identifiers.
`--env` aliases `--environment` on commands that accept an environment.
For repeated lifecycle operations or JSON scripting, read `references/batching.md` in the installed `tadx` root skill.
Reuse inspected existing definitions and conventions early, before comprehensive new field discovery.
Use live reads for authoritative authoring decisions; cached coverage does not establish current completeness.

For new definitions, consider the complete dimension inventory and include generously useful eligible slicers.
Favor inclusion when relevance is uncertain; exclude clearly unrelated, sensitive, or technical fields.
Identifier-like appearance and high cardinality alone are not exclusions.
Use `DAY` nearly always; do not query datasource values merely to discover minimum granularity.

TADX constructs requests from verified CLI flags.
Review `--preview` before every authorized create or fork; its default summary includes the consequential settings.
Use `--full` for expanded evidence or an explicitly incomplete summary, and do not treat omitted settings as reviewed.
Verify the saved configuration from returned read-back or an exact inspection when needed.
A preview proves local validation, not Tableau acceptance or numeric correctness.
Execution requires enabled mutation policy, but supported read-only previews remain available when the gate is off.
A preview does not authorize execution.
Before changing the environment override or saved mutation setting by any mechanism, obtain explicit user permission for that setting change and its scope.
A requested Pulse operation does not authorize changing the flag; prior permission applies only to the explicitly approved setting change and scope.
Follow the TADX root skill's single-question approval guidance, including future-shell scope for persistent changes.
Use the installed TADX root Guidance for authentication, workspace selection, and general lifecycle boundaries.

Retain exact selected fields, measure meaning, date, units, slicer set and order, metric filters and period, reviewed flags, returned IDs, and verification status.
Reuse discovery for an unchanged source and refresh affected evidence after source changes or identity errors.
Retain confirmed creation results even when verification fails, and reconcile by the returned identity before another create.
Report configuration verification separately from values and insights.
