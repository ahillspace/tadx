# Resolve uncertain Pulse semantics

Read only when ordinary field selection leaves a substantive ambiguity.

## Quantity and source grain

Choose the date representing the measure's event; different definitions can use different dates.
Use business captions without changing exact Tableau field IDs.
If no eligible date exists, explain the source requirement instead of inventing one.

- `SUM` combines additive quantities.
- `AVERAGE` averages source rows; confirm that this matches the intended weighting.
- `COUNT` counts non-null observations; `COUNT_DISTINCT` counts entities only when duplication and eligibility support that interpretation.
- `MIN` and `MAX` require meaningful extrema; neither substitutes for a latest snapshot.
- `USER` preserves a calculation that already aggregates, such as `SUM([Profit]) / SUM([Sales])`.

Creation requires a measure-role field even for `COUNT_DISTINCT`.
If an entity ID is only a dimension, find an eligible published calculation or explain the required datasource change.
Never bypass validation or silently change meaning.
Table calculations and calculations depending on them cannot serve as Pulse measures.
A calculation depending on hidden components can remain eligible; trust the returned classification.
Use targeted `--field-id "<exact-id>" --full` inspection for formulas, provenance, and exclusions.

## Time, scale, and population

`OVER_TIME` describes period measures; `LATEST` describes snapshots supported by the source grain.
Temporality does not repair duplicated snapshots or mismatched data.
Running totals require an intended cumulative additive measure with `SUM` and `OVER_TIME`.
Keep percentages and aggregated ratios noncumulative.

Minimum granularity is the finest supported grain: `MONTH` permits month, quarter, and year, not a monthly reporting-period selection.
Metric variants select periods separately.
Choose currency from evidence; do not inherit USD accidentally.
Confirm whether percentages represent fractions or percentage points when scale is uncertain.

Prefer meaningful allowed dimensions such as region or product category over identifiers and emails.
Adding a dimension does not filter the definition's population.
Fork with verified dimension values for a filtered metric.
Preserve filters on unspecified fields and inspect the result for variant reuse.
