# Create and verify a Pulse definition

Read with authoring.md before creating or previewing a definition.
TADX handles request construction and authentication; supply verified CLI arguments.
Each create item creates one definition and resolves its Tableau-created default metric.

## Field eligibility

TADX resolves field captions and labels from live datasource metadata to raw Tableau IDs before previewing or publishing.
Exact raw IDs take precedence over display names; ambiguous display names fail before publication.
The preview shows the resolved raw IDs that Tableau will receive.
This resolves field names only, not aliases for categorical member values.

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
Check the default preview's measure, aggregation, date, dimensions, allowed grains, temporality, running total, units, currency, sentiment, and fixed defaults.
A truncated or failed preview is not a completed review.
`review_complete: false` or `requires_full: true` means the summary needs more evidence; expanded output cannot repair missing source information.

## Execute and inspect saved state

For authorized creation, reuse the reviewed argument vector and remove only `--preview`.
If a substantive choice changes, preview again.
TADX revalidates on execution; a preview is not a remote-state lock.
Retain the returned definition and default metric LUIDs and verify both in the same environment, reusing complete read-back evidence when available.

Compare saved settings, definition linkage, complete population, and actual period.
The minimum-granularity flag does not determine the default metric's reporting period.
Use the variants route for a requested period or population.
The configuration-context reference explains what these saved settings establish; actual numbers require separate data evidence.
Do not follow an unfiltered default when the user requested a filtered variant.
For batches, verify a representative before scaling and separately verify different semantic families.
Keep successful work if another candidate fails.

## Recover from errors

Correct only a specifically evidenced input or source issue; preserve the requested business meaning.
For HTTP 400, retain Tableau's details and review the same flags with `--preview --full`; the status alone does not identify a cause.
For HTTP 409, inspect candidate definitions on the datasource, including different names, and compare semantics before choosing reuse.
A conflict status alone does not prove duplication.

If an ID was returned, including in the error resource, inspect it first.
Confirmed creation can appear in `output` alongside an error about verification; a nonzero exit does not erase that creation.
If default metric resolution failed, list that definition's metrics instead of creating again.
Timeouts and incomplete responses can follow a successful write; reconcile inventory before another attempt.
Limited visibility does not prove absence.
Keep the structured error and Tableau request ID without credentials.
Stop the affected candidate if the same unexplained blocker remains; do not rename, remove slicers, change aggregation, or delete and recreate merely to obtain acceptance.
