# Resolve Pulse calculation and population semantics

| Action | Purpose | Selectors and useful flags |
| --- | --- | --- |
| `tadx content datasource schema` | Read selected formulas, roles, aggregation, and exclusion evidence. | `--environment`, `--id <datasource-luid>`, `--field-id <exact-field-id>`, `--full` |
| `tadx pulse definition inspect` | Inspect an existing measure, date, offset, and population convention. | `--environment`, `--id <definition-luid>`, `--full` |
| `tadx pulse metric inspect` | Confirm the actual variant's filters and period. | `--environment`, `--id <metric-luid>`, `--full` |

Read in full when selecting a calculated measure, entity count, average/rate/percentage, snapshot, or multi-fact measure.
These are creation decisions, not instructions to query through the CLI.
Resolve material data questions with authorized evidence when metadata is insufficient.
Examples are hypothetical reasoning, never fields or formulas that TADX creates.

## Counts and source grain

Identify what one row represents before choosing an aggregation.
An order-line row, order, customer activity, shipment event, and daily account snapshot are different units.
A numeric column does not establish additive business meaning; duplicated joins can repeat valid values.

COUNT counts non-null observations.
COUNT_DISTINCT counts distinct values of the selected field.
For "number of orders," determine the actual order identifier and event date, not COUNT(Sales), a generic row count, or a customer count.
A published row-count measure can be correct when each row genuinely represents one requested entity; do not assume that grain.

Both count operators are valid Pulse aggregations for eligible dimensions as well as measures.
An already-published equivalent count calculation can be reused, but preserve its aggregation semantics; an aggregate COUNTD calculation generally needs USER.

Check the effect of missing identifiers, duplicate entity rows, and the selected population.
A null/unavailable count is not zero.
Do not change the entity merely to obtain an accepted definition.

## Ratios, averages, and percentages

| Request | Correct basis to establish | Frequent wrong choice |
| --- | --- | --- |
| Margin or conversion/defect/on-time rate | Appropriate aggregate numerator divided by denominator, for the same intended population. | SUM of row percentages or an unweighted average of ratios. |
| Average selling price | Revenue divided by units if a unit-weighted price is intended. | AVERAGE of unit prices on rows with unequal quantities. |
| Average order value | Correct order-grain measure or revenue divided by distinct orders. | AVERAGE of line-item Sales labeled as an order average. |
| Average duration | The intended events/entities and weights, including how repeats/nulls are handled. | Averaging duplicates introduced by joins. |

A row average can be correct when equal row weighting is the request.
Do not force every average into a ratio.
Inspect an existing formula and units; if the required expression is absent, the CLI cannot create it inline.
Report the exact missing measure instead of substituting arithmetic with a different denominator.

A percentage stored as 0.12 is not interchangeable with a percentage-point value of 12.
Formatting does not rescale the data.
Inspect formula/metadata or bounded representative values when necessary.
Keep percentages and ratios non-cumulative.
Check denominator-zero/null handling in existing calculations when material; do not silently replace missing results with zero.

## Calculations and dependency evidence

`SUM([Profit]) / SUM([Sales])` is aggregate logic; preserve it with USER when correctly classified.
`[Unit Price] * [Quantity]` is row-level logic and can need SUM.
A FIXED/INCLUDE/EXCLUDE expression has its own grain and filter behavior; simply finding SUM inside its text does not prove that the outer result requires USER.

Inspect the selected measure's full formula, `default_aggregation`, `requires_user_aggregation`, exclusion reason, and provenance.
Table calculations and dependent calculations must not become Pulse measures.
A visible calculation can legitimately use hidden ordinary components; hiddenness alone is not grounds to reject it.

TADX includes hidden nodes when analyzing calculation dependencies.
When formula and classification conflict, inspect the selected field's full metadata and report the specific unresolved issue.
Preserve the intended math rather than trying aggregations until one publishes.

## Snapshots and dates

Use the date representing the requested business event.
Order Date, Ship Date, and Payment Date can yield different legitimate definitions.
Do not use load timestamps simply because they are most recent.
A requested business-day/time-zone/fiscal convention needs source/configuration evidence, not renaming a timestamp.

LATEST selects the latest point in a period; it does not select each entity's most recent row and carry it forward.
It still requires time-series observations; a lone current snapshot without history does not meet Pulse's temporal data requirement.
Verify whether balances/headcount/inventory have aligned snapshots and one appropriate contribution per entity/date.
Missing entities on the latest date and duplicate snapshots remain modeling problems.
Summing snapshots across days normally measures a different quantity.

Minimum granularity, metric reporting period, and date offset are separate decisions.
Monthly observations are not daily observations with permission to select LAST_30_DAYS.
A rolling period includes the current day in this CLI.
A snapshot definition without recent data is not repaired by LATEST or a different aggregation.
Inspect an existing definition's offsets/calendar when reusing it.

## Mixed tables, populations, and units

Check that selected dimensions apply to the measure's population and relationships.
A product slice can be appropriate for sales but unrelated to staffing.
Do not assume all fields in one published datasource share a grain.
Examine repeated values, entity coverage, and relationship behavior when they affect the KPI.
A FIXED calculation may not respond to ordinary filters as the user expects.

Source-level filters, row-level security, fixed definition filters, and adjustable metric filters are distinct.
A variant cannot restore rows already excluded upstream; adding an allowed dimension is not access control.
An administrator's numeric validation does not establish every follower's visible population.
Do not broaden credentials or remove filters to obtain a value.

CURRENCY changes display, not exchange rates.
Do not sum mixed local currencies and label the result USD.
Use an evidenced reporting-currency measure or the intended scoped population.
NONE is appropriate when neither up nor down alone represents success, such as inventory around a target.

Return to the execution preflight once these choices are supported.
If a material issue cannot be resolved, identify the exact blocked candidate without discarding unrelated verified work.
