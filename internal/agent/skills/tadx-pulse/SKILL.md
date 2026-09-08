---
name: tadx-pulse
description: Turn business intent into useful Tableau Pulse definitions, select measures and slicers, create and verify metrics, fork period or population variants, and manage followers with TADX. Use Tableau MCP for actual values and generated insights.
---

# Author and manage Tableau Pulse metrics

A definition owns the shared measure, aggregation, date, allowed dimensions, and display settings.
A metric is a period and population variant of that definition; Tableau creates a default metric with each new definition.
Followers subscribe to metrics, and insights are computed analytical results.

## Read the relevant reference before acting

| Task | Read first |
| --- | --- |
| Recommend KPIs, select fields or slicers, or design a definition | [Authoring](references/authoring.md) |
| Create a definition or review its preview | [Authoring](references/authoring.md) and [Creation contract](references/authoring-contract.md) |
| Choose a calculation, entity count, average, rate, percentage, snapshot, or measure spanning multiple facts | Also read [Semantics](references/semantics.md) before finalizing that choice |
| Fork a metric or satisfy a particular period or filtered population | [Variants](references/variants.md), before creating a base that depends on that choice |
| Resolve names, list, inspect, pull, follow, unfollow, delete, or assess an edit | [Operations](references/operations.md) |
| Query values, validate numbers, retrieve insights, or prepare a brief | [Analytics context](references/analytics-context.md) |

Read only the routes needed for the task.
Authoring and analytics references include prerequisite discovery commands; these reads do not require loading the operations route too.
Keep needed guidance and verified choices available across task handoffs.

## Shared rules

Use exact identities in the selected environment; datasource, field, definition, metric, user, and group IDs are different identifiers.
`--env` aliases `--environment` on commands that accept an environment.
Reuse inspected existing definitions and conventions early, before comprehensive new field discovery.
Use live reads for authoritative authoring decisions; cached coverage does not establish current completeness.

For new definitions, consider the complete dimension inventory and include generously useful eligible slicers.
Favor inclusion when relevance is uncertain; exclude clearly unrelated, sensitive, or technical fields.
Identifier-like appearance and high cardinality alone are not exclusions.
Use `DAY` nearly always; do not query datasource values merely to discover minimum granularity.

TADX constructs requests from verified CLI flags.
Review `--preview --full` before every authorized create or fork, then inspect the saved result.
A preview proves local validation, not Tableau acceptance or numeric correctness.
Mutations, including their previews, require `TADX_ENABLE_MUTATIONS=1`; that setting does not grant user authorization.
Use the installed TADX root Guidance for authentication, workspace selection, and general lifecycle boundaries.

Retain exact selected fields, measure meaning, date, units, slicer set and order, metric filters and period, reviewed flags, returned IDs, and verification status.
Reuse discovery for an unchanged source and refresh affected evidence after source changes or identity errors.
Reconcile uncertain writes by returned identity and inventory before another create.
Report configuration verification separately from values and insights.
