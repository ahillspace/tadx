---
name: tadx-pulse
description: Author Tableau Pulse definitions from business intent and published datasource fields, verify definitions, fork variants, and manage followers through TADX. Use Tableau MCP for metric values or insights.
---

# Author Tableau Pulse metrics

Use TADX for definition/metric lifecycle and subscriptions; Tableau MCP separately owns values and insights.
TADX never calls or proxies MCP.

## Start from known context

Reuse the environment, datasource LUID, and prior successful authentication.
Use a provided environment alias verbatim without revalidation; diagnose configuration/authentication only after an actual command fails.
Never print or persist PATs or session tokens.
Batch independent TADX commands into one shell tool call as sequential lines; never run authenticated calls concurrently with the same PAT.
Gate dependent commands on successful prior results, and quote spaced paths, names, and exact field IDs.
Use these recipes directly; do not probe help for flags already shown or repeat a prior probe.
Avoid broad help/capability dumps; allow at most one relevant leaf `--help` probe only for an unresolved installed flag.

Resolve the requested quantity, population, audience, and event/snapshot date from available context.
Ask only when unresolved meaning changes the result.
A recommendation request does not authorize creation or subscriptions.

## Find only the fields needed

Exact datasource names can be duplicated across projects; never select the first name match.
Use a supplied LUID directly.
When exact datasource name and project path are supplied, call `inspect` directly; do not list first.
Only when those selectors are unavailable, use `tadx content datasource list --environment <alias> --name "<datasource-name>" --limit 5`.
Replace placeholders with verified values; batch the needed schema reads sequentially after resolving the datasource LUID.

```text
tadx content datasource inspect --environment <alias> --name "<datasource-name>" --project "<exact/project/path>"
tadx content datasource schema --environment <alias> --id <datasource-luid> --query "<measure-concept>" --role measure --limit 10
tadx content datasource schema --environment <alias> --id <datasource-luid> --role date --limit 10
tadx content datasource schema --environment <alias> --id <datasource-luid> --query "<breakdown-concept>" --role dimension --limit 10
```

Skip dimension discovery when no breakdown is needed.
Copy returned field `id` values verbatim; captions and display names are not identifiers.
Read live by default; supported `--catalog` reads are local without fallback, and incomplete coverage does not prove absence.
Choose only semantically exact captions, then copy their IDs; multiple results alone do not justify wider pagination or a broader query.
Inspect an uncertain candidate with `--field-id "<exact-field-id>" --full`; follow cursors only when the intended field is still absent.
Do not download a datasource or collect its complete schema for ordinary field selection.
Visibility does not establish Pulse eligibility; Tableau validates access, connection, and fields during live operations.

## Create and verify

Check existing definitions for the selected datasource before creating.
The bounded list has no datasource filter flag: match returned `datasource_luid` and compare measure/date/aggregation fields from `--full`.
Tableau rejects duplicate semantic definitions even with different names; reuse a matching definition when it satisfies the request.
For an authorized distinct definition, choose a distinct measure/date combination only when it fits the requested meaning; otherwise report the collision.
Never retry an identical semantic payload or rename it to evade duplication.

The following template describes an additive amount over time; substitute deliberate semantics rather than copying unsuitable defaults.

```text
tadx pulse definition list --environment <alias> --limit 25 --full
tadx pulse definition create --environment <alias> --name "<business-name>" --description "<quantity and time basis>" --datasource-id <datasource-luid> --measure-field "<measure-field-id>" --aggregation SUM --date-field "<date-field-id>" --dimension "<dimension-field-id>" --temporality OVER_TIME --minimum-granularity DAY --number-format CURRENCY --currency <currency-code> --sentiment UP
tadx pulse definition inspect --environment <alias> --id <definition-luid> --full
```

Omit `--dimension` when unnecessary; repeat it for additional allowed dimensions.
It enables segmentation, not a fixed population filter.
Select `NUMBER`, `CURRENCY`, or `PERCENT` and `UP`, `DOWN`, or `NONE` from business meaning; currency must be explicit when used.
Use `SUM` for additive measures, `USER` when `requires_user_aggregation` is true, and never average an aggregated ratio.
Both the measure-role field and a genuine date field are required; strings named Date and table calculations do not qualify.
`LATEST` represents snapshots only when source grain supports them.
Running totals require `SUM` and `OVER_TIME`; minimum granularity limits allowed grains rather than choosing a reporting period.
Read [semantic details](references/semantics.md) only for weighting, counts, exclusions, snapshots, or percentage scale uncertainty.

Authorized mutations run by default with `TADX_ENABLE_MUTATIONS=1`; add `--preview` when reviewing unresolved intent or settings before creation.
Preview validates live fields/name collisions without creating; omit it to apply, with no separate `--apply`.
Verify the returned definition and default metric LUIDs; numerical correctness requires a data tool when requested.
If creation is uncertain, inspect returned IDs/current inventory before retrying.
If only the default metric is unresolved, list metrics for that definition instead of recreating it.
Stop repeated writes on the same eligibility failure and report the source/access requirement.

## Variants and followers

Inspect the source metric and shared definition before changing a variant's period or population.
Use verified allowed dimensions/values; retain existing subscriptions unless their change is authorized.

```text
tadx pulse metric list --environment <alias> --definition-id <definition-luid> --limit 5
tadx pulse metric fork --environment <alias> --id <metric-luid> --period MONTH_TO_DATE --filter "<allowed-field-id>=<verified-value>"
tadx pulse metric follow --environment <alias> --id <metric-luid> --user-id <user-luid>
tadx pulse metric followers --environment <alias> --id <metric-luid>
```

Use `--group-id` instead of `--user-id` for an authorized group subscription.
Forks replace filters on supplied fields and preserve other source settings; an equivalent variant can already exist.
If a fork returns `created: false` or `existing` with the source LUID, it created no non-default variant; do not inspect or list again.
Repeated values form one filter; do not mix include/exclude for the same field.
Use `metric unfollow` for subscription removal and `metric delete` only for non-default variants.
The default metric cannot be deleted directly; removing it requires `definition delete` with authorization to delete the shared definition.
Do not replace a request to remove one non-default variant with definition deletion.
Deletion previews do not enumerate dependencies or cascade effects; Tableau determines those effects.
Trust a successful delete result; do not issue a confirmation list unless the outcome is uncertain or the user explicitly requests verification.
