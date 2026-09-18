---
name: tadx-pulse
description: Author and manage Tableau Pulse definitions with TADX, including business meaning, measure and slicer selection, creation, saved configuration verification, metric variants, and followers.
---

# Author and manage Tableau Pulse metrics

Before starting a TADX Pulse task, read the installed `tadx` root skill at `../tadx/SKILL.md`, beside this skill's directory.
It supplies the shared operating model and routes to concepts only when needed.
Use `tadx pulse -h` to locate resources, then use focused `tadx pulse definition -h` or `tadx pulse metric -h` help for the applicable command syntax and flags.
Use `tadx content datasource -h` for schema discovery when that reference is not already loaded.
The references below retain authoring judgment and verification contracts that command syntax alone does not provide.

A definition owns the shared measure, aggregation, date, allowed dimensions, and display settings.
A metric is a period and population variant of that definition; Tableau creates a default metric with each new definition.
Followers subscribe to metrics, and insights are computed analytical results.
This Guidance covers TADX authoring and lifecycle commands, respecting the user's chosen tools for other work.
TADX configuration reads do not return current metric values or generated insights.

## Read the relevant reference before acting

| Task | Read first |
| --- | --- |
| Recommend KPIs, select fields or slicers, or design a definition | [Authoring](references/authoring.md) |
| Create a definition or review its preview | [Creation contract](references/authoring-contract.md); read [Authoring](references/authoring.md) when the source, fields, dimensions, or business meaning remain unresolved |
| Choose a calculation, entity count, average, rate, percentage, snapshot, or measure spanning multiple facts | Also read [Semantics](references/semantics.md) before finalizing that choice |
| Fork a metric or satisfy a particular period or filtered population | [Variants](references/variants.md), before creating a base that depends on that choice |
| Resolve names, list, inspect, pull or publish a portable bundle, follow, unfollow, delete, or assess an edit | [Operations](references/operations.md) |
| Verify saved settings or retain configuration context for numeric validation | [Configuration context](references/analytics-context.md) |

Read only the routes needed for the task.
Authoring covers source and field selection; it does not require the operations route unless existing-object lifecycle is also part of the task.
Keep needed guidance and verified choices available across task handoffs.

## Shared rules

Use exact identities in the selected environment; datasource, field, definition, metric, user, and group IDs are different identifiers.
For repeated lifecycle operations or JSON scripting, read `references/batching.md` in the installed `tadx` root skill.
Reuse inspected existing definitions and conventions early, before comprehensive new field discovery.
Use live reads for authoritative authoring decisions; cached coverage does not establish current completeness.

For a design request or unresolved field meaning, consider the complete dimension inventory and include generously useful eligible slicers.
When the user supplies exact fields and material semantics, validate those directly and do not broaden discovery or add slicers without evidence.
Favor inclusion when relevance is uncertain only during that design work; exclude clearly unrelated, sensitive, or technical fields.
Identifier-like appearance and high cardinality alone are not exclusions.
Use `DAY` nearly always; do not query datasource values merely to discover minimum granularity.

TADX constructs requests from verified CLI flags.
For an authorized create or fork with unresolved material choices, review `--preview` and its consequential settings.
For a fully specified configuration, validate supplied fields and material semantics directly, then rely on the operation's automatic saved read-back unless it is incomplete.
Use `--full` for expanded evidence or an explicitly incomplete summary, and do not treat omitted settings as reviewed.
Use `tadx last --full` or an exact inspection only when the returned read-back omits evidence needed for the next decision.
Verify the saved configuration from returned read-back first, and perform an extra read only when that evidence is missing or unresolved.
A preview proves local validation, not Tableau acceptance or numeric correctness.
The TADX root skill's authentication, mutation-policy permission, and recovery rules apply; a preview does not authorize execution.

Retain exact selected fields, measure meaning, date, units, slicer set and order, metric filters and period, reviewed flags, returned IDs, and verification status.
Reuse discovery for an unchanged source and refresh affected evidence after source changes or identity errors.
Retain confirmed creation results even when verification fails, and reconcile by the returned identity before another create.
Report configuration verification separately from values and insights.
Successful empty follower results are represented as `subscriptions: []`; unavailable or unattempted reads remain distinct from a confirmed empty collection.
