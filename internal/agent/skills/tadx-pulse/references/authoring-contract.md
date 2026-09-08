# Create and verify a Pulse definition

| Action | Purpose | Required and useful flags |
| --- | --- | --- |
| `tadx pulse definition create` | Preview or create one definition and its default metric. | `--environment`, `--name`, `--datasource-id`, `--measure-field`, `--date-field`, repeated `--dimension`; settings below; `--preview`, `--full` |
| `tadx pulse definition inspect` | Verify saved shared settings. | `--environment`, `--id <definition-luid>`, `--full` |
| `tadx pulse metric inspect` | Verify the default metric's actual period and population. | `--environment`, `--id <metric-luid>`, `--full` |
| `tadx pulse metric list` | Resolve a missing default metric identity. | `--environment`, `--definition-id <definition-luid>`, `--all`, `--full` |

Read with authoring.md before creating or previewing a definition.
TADX handles request construction and authentication; supply verified CLI arguments.
One create invocation creates one definition and resolves its Tableau-created default metric.

## Flags and field requirements

| Flag | Contract |
| --- | --- |
| `--environment` or `--env` | Required configured environment alias. |
| `--name` | Required business name, up to 255 Unicode characters after trimming. |
| `--description` | Useful quantity and date basis, up to 1,024 Unicode characters after trimming. |
| `--datasource-id` | Exact published datasource LUID in the selected environment. |
| `--measure-field` | Exact eligible schema ID for the requested quantity. |
| `--date-field` | Exact schema ID with date role. |
| `--aggregation` | SUM, AVERAGE, MIN, MAX, COUNT, COUNT_DISTINCT, or USER. Default SUM; select deliberately. |
| `--dimension` | Repeat once per eligible dimension ID. At least one is required; supplied order is preserved and duplicates removed. |
| `--minimum-granularity` | DAY, WEEK, MONTH, QUARTER, or YEAR. Default DAY, appropriate nearly always; permits that grain and coarser grains. |
| `--temporality` | OVER_TIME or LATEST; default OVER_TIME. |
| `--running-total` | Boolean switch, default false; requires SUM and OVER_TIME with cumulative additive business meaning. |
| `--number-format` | NUMBER, CURRENCY, or PERCENT; default NUMBER. |
| `--currency` | Three-letter currency code with CURRENCY. Select explicitly because omission defaults to USD. |
| `--sentiment` | UP, DOWN, or NONE; default NONE. |
| `--preview` | Read-only plan with validation and live reads; required before execution in this workflow. |
| `--full` | Expanded TOON details for reviewing the selected configuration. |

Use uppercase enums and exact IDs.
Repeat `--dimension` instead of combining IDs in a comma-separated argument.
A comma or space within one actual ID stays part of that ID.
Use the bare `--running-total` switch or `--running-total=false`, not a positional Boolean.
Preserve literal arguments using the executing shell's quoting rules; never alter IDs to make them easier to quote.

Select single, non-excluded fields.
COUNT and COUNT_DISTINCT accept eligible dimensions as well as measures.
Other aggregations use measure-role fields; SUM and AVERAGE require numeric input.
MIN and MAX must represent the requested extrema and do not select the latest observation.
Honor `requires_user_aggregation`: true requires USER; false requires an appropriate ordinary aggregation.
Read the semantics reference for nontrivial calculation and grain decisions.
Table calculations and calculations depending on them are excluded.
A visible calculation can use hidden ordinary components; use TADX's dependency-aware classification.
The date must have date role, and adjustable fields must have dimension role.
Metadata discovery does not establish source connectivity or every viewer's access.

## Settings without create flags

Creation uses no fixed definition filters, zero date offset with dynamic offset disabled, prior-period and prior-year comparisons, and built-in insight settings.
It does not add goals, related links, or certification.
No create flags expose inline calculations, fixed definition filters, custom calendars or comparisons, custom units, goals, or insight settings.
A required unsupported option must be resolved before creating a substitute.
Enabled insights do not prove that values or insights are available.

## Preview the complete selection

Confirm the authorized source and business scope, any required period or filter feasibility, inspected existing definitions, and exact field evidence.
Consider the complete dimension inventory and retain the generous relevant set in intentional order.
Check the full preview's measure, aggregation, date, dimensions, allowed grains, temporality, running total, units, currency, sentiment, and fixed defaults.
A truncated or failed preview is not a completed review.

This example creates an additive reporting-currency measure.
Replace every placeholder with a verified value, include the complete useful slicer set, and select display settings that match the measure.
The five dimensions are illustrative, not a limit.

```text
tadx pulse definition create --environment '<alias>' --name '<business-name>' --description '<quantity and business-event date basis>' --datasource-id '<datasource-luid>' --measure-field '<exact-measure-id>' --aggregation SUM --date-field '<exact-date-id>' --dimension '<exact-region-id>' --dimension '<exact-product-category-id>' --dimension '<exact-channel-id>' --dimension '<exact-customer-segment-id>' --dimension '<exact-facility-id>' --minimum-granularity DAY --temporality OVER_TIME --number-format CURRENCY --currency '<reporting-currency-code>' --sentiment UP --preview --full
```

## Execute and inspect saved state

For authorized creation, reuse the reviewed argument vector and remove only `--preview`.
If a substantive choice changes, preview again.
TADX revalidates on execution; a preview is not a remote-state lock.
Retain the returned definition and default metric LUIDs and inspect both in the same environment:

```text
tadx pulse definition inspect --environment '<alias>' --id '<definition-luid>' --full
tadx pulse metric inspect --environment '<alias>' --id '<default-metric-luid>' --full
```

Compare saved settings, definition linkage, complete population, and actual period.
The minimum-granularity flag does not determine the default metric's reporting period.
Use the variants route for a requested period or population, and the analytics route to verify actual numbers.
Do not follow an unfiltered default when the user requested a filtered variant.
For batches, verify a representative before scaling and separately verify different semantic families.
Keep successful work if another candidate fails.

## Recover from errors

Correct only a specifically evidenced input or source issue; preserve the requested business meaning.
For HTTP 400, retain Tableau's details and review the same flags with `--preview --full`; the status alone does not identify a cause.
For HTTP 409, inspect candidate definitions on the datasource, including different names, and compare semantics before choosing reuse.
A conflict status alone does not prove duplication.

```text
tadx pulse definition list --environment '<alias>' --all --full
tadx pulse metric list --environment '<alias>' --definition-id '<returned-definition-luid>' --all --full
```

If an ID was returned, including in the error resource, inspect it first.
If default metric resolution failed, list that definition's metrics instead of creating again.
Timeouts and incomplete responses can follow a successful write; reconcile inventory before another attempt.
Limited visibility does not prove absence.
Keep the structured error and Tableau request ID without credentials.
Stop the affected candidate if the same unexplained blocker remains; do not rename, remove slicers, change aggregation, or delete and recreate merely to obtain acceptance.
