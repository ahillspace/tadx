---
name: tadx-pulse
description: Author Tableau Pulse definitions from business intent and published datasource fields, then verify definitions, fork metric variants, or manage followers through TADX. Use for Pulse lifecycle work, not metric values or insights.
---

# Author Tableau Pulse metrics

Use TADX for Pulse definitions, metric variants, and subscriptions.
Tableau MCP separately handles metric values and insights; TADX never calls or proxies MCP.
Use installed command help for current flags instead of constructing REST payloads.
Start with `tadx pulse --help`, then load only the relevant leaf command help.

## Establish the metric's meaning

Identify the audience, decision, business quantity, population, and event or snapshot date from the request.
For an open-ended request, propose a small, focused set using datasource evidence and state your audience assumption.
Ask only when unresolved meaning materially changes the result, such as gross versus net revenue or order versus shipment date.
Do not interpret a request for recommendations as authorization to create definitions.
Preserve existing definitions and subscriptions unless their modification is within the request.

Give each proposed metric a business name and a brief description of its quantity, aggregation, and time basis.
Improve captions without inventing business meaning or changing field identity.

## Find the source with bounded discovery

Resolve the environment and published datasource before selecting fields.
Use `tadx content datasource list` with known exact filters, such as `--name` or `--project-name`, and a bounded `--limit`.
Resolve ambiguity through exact inspection and retain the authoritative datasource LUID.
Datasource visibility and discoverable fields do not prove Pulse eligibility.
Tableau determines permission, connection, field, and data eligibility during live operations.

Read commands use live Tableau by default; `--catalog` reads only locally indexed data.
Use available catalog observations to narrow candidates, noting source coverage and staleness.
A partial page or catalog miss does not prove a field or datasource is absent.
Use live discovery when cached coverage is insufficient or freshness matters.
Keep subsequent reads and mutations on the same intended environment.

Inspect the selected datasource's VDS field catalog incrementally:

1. Search measure concepts with `tadx content datasource schema --id <datasource-luid> --query <term> --role measure --limit 10`.
2. Discover candidate dates using `--role date`, and business breakdowns using `--role dimension` with relevant terms.
3. Inspect an uncertain candidate using `--field-id '<exact-field-id>' --full` for its formula, provenance, or exclusion reason.
4. If targeted searches fail, broaden terms or narrow by `--table`, then follow returned cursors as needed.

Keep compact output for ordinary selection.
`--full` expands details for the same bounded result; it does not request every field.
Avoid collecting the full schema before targeted discovery fails.
Retain only selected fields and unresolved candidates in working context.

## Select valid fields and aggregations

Copy each returned field `id` verbatim, including brackets or generated calculation names.
Captions support reasoning; they are not substitute identifiers.
Use a field with the required role: `measure`, `date`, or `dimension`.
A string named "Date" does not qualify as a date field.
Choose the date representing each measure's business event; different definitions can use different dates.
If no eligible date exists, explain the source requirement instead of inventing one.

Choose aggregation from meaning and source grain:

- Use `SUM` for additive quantities, such as transaction amounts.
- Use `AVERAGE` only when averaging source rows matches the intended weighting.
- Use `COUNT` for non-null observations and `COUNT_DISTINCT` for distinct entities, after checking duplication and field eligibility.
- Use `MIN` or `MAX` for meaningful extrema, not as substitutes for a latest snapshot.
- Use `USER` when `requires_user_aggregation` is true; do not aggregate an already aggregated calculation again.

For example, `SUM([Profit]) / SUM([Sales])` needs `USER`, not an average of row percentages.
Current TADX creation requires a measure-role field even for `COUNT_DISTINCT`.
If an entity identifier appears only as a dimension, find an eligible published calculation or explain the required datasource change.
Do not bypass validation or silently change the metric's meaning.

Table calculations, including calculations that depend on table calculations, cannot serve as Pulse measures.
Inspect exclusions when a candidate is missing; use an eligible equivalent only when it preserves the requested meaning.
A calculation depending on hidden components can remain eligible; trust its returned classification rather than excluding it by association.

Choose allowed dimensions that answer meaningful segmentation questions, such as region or product category.
Avoid identifiers and emails as default breakdowns.
Adding `--dimension` enables later filtering and breakdowns; it does not restrict the definition to a particular value.

## Set time behavior and presentation

Use `OVER_TIME` for quantities measured across periods and `LATEST` for snapshot quantities, such as inventory on hand.
Confirm that the source grain supports the intended snapshot; temporality does not repair duplicated or mismatched data.
Use `--running-total` only for an intended cumulative additive measure.
TADX requires `SUM` and `OVER_TIME` for running totals.
Keep rates, percentages, and already aggregated ratios noncumulative.

Set minimum granularity to the finest useful supported grain given the source cadence and request.
`--minimum-granularity MONTH` permits month, quarter, and year; it does not select a monthly reporting period.
Metric variants select periods separately.

Choose `NUMBER`, `CURRENCY`, or `PERCENT` from the quantity's unit.
For currency, supply the known currency code explicitly rather than inheriting USD accidentally.
Check percentage scale when uncertain; formatting does not establish whether source values represent fractions or percentage points.
Use sentiment `UP` when increases are favorable, `DOWN` when unfavorable, and `NONE` when direction has no clear preference.

## Create and verify

Use `tadx pulse definition create --help` to translate the selected intent into CLI flags.
Supply the explicit write environment, datasource LUID, exact fields, and deliberate semantic settings.
Use `--preview` when the plan helps resolve uncertainty or review consequential settings.
Preview validates live fields and name collisions without creating the definition.
When creation is authorized, run the command without `--preview`; mutations run by default when enabled.

Inspect the returned definition LUID and default metric LUID in the write environment.
Use bounded `--full` inspection when required to verify time behavior, dimensions, formatting, or configuration.
Report actual identifiers and any verification limits.
Creation success does not prove the metric's numerical correctness; value validation requires an appropriate data tool when requested.

If a timeout or partial result leaves creation uncertain, inspect returned IDs and current inventory before retrying.
If the definition exists but its default metric is unresolved, list that definition's metrics instead of creating another definition.
For name collisions, inspect the existing definition and determine whether it already satisfies the request.
For eligibility errors, inspect the targeted live fields and returned corrective action, then correct only the supported mismatch.
Stop repeated writes when the same eligibility issue persists; report the concrete source or access requirement.

## Manage variants and followers

Distinguish a definition's shared measurement configuration from a metric's period and dimensional filters.
Fork an existing metric when the request changes its period or selected population.
Inspect the source metric and definition first, then use `tadx pulse metric fork --help`.
Filter fields must belong to the definition's allowed dimensions, and values must come from supplied or verified evidence.
Repeated values for one field form one filter; do not mix include and exclude filters for that field.
A fork replaces an existing filter on a supplied field and preserves other source settings.
Inspect the result because an equivalent variant can already exist.

Follow or unfollow the exact metric for the requested user or group LUID, and verify through its followers.
Do not infer subscription authorization from metric creation.
For deletion, resolve whether the user means a metric variant or its shared definition and inspect the exact target.
Deletion previews do not enumerate dependencies or cascade effects; Tableau determines those effects.
Do not substitute definition deletion for removing one variant or follower.
