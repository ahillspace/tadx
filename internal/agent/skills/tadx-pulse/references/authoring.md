# Design a useful Pulse definition

Use this reference for recommendations, creation design, and unresolved source or field meaning.
For a fully specified configuration, the creation contract can be sufficient; read this reference when design evidence is still needed.

## Frame the business question and reuse existing work

Honor the requested audience, source, scope, and quantity.
For a broad setup without an audience, infer a plausible persona from actual datasource fields and state that assumption briefly.
Build a coherent collection of useful outcomes and drivers, rather than an arbitrary three-KPI shortlist or every numeric column.
A request for one KPI remains one KPI, with generous slicers.

Resolve the business quantity, population, event date, period versus snapshot behavior, units, sentiment, and useful slicing dimensions.
Use metadata and existing definitions to understand row or entity grain when it affects aggregation.
Ask only when unresolved meaning materially changes the result and available evidence cannot resolve it.
Use a clear business-facing name and description; words such as unique, net, weighted, and on-time must reflect actual calculation logic.
Recommendations do not authorize creation or subscriptions.

After identifying the datasource, inspect likely existing definitions before expensive new field discovery.
Reuse their proven naming, date basis, units, sentiment, and slicing conventions where these fit the business request.
Compare the actual datasource, quantity, aggregation, date, population, temporality, and available slicing and time options; names alone do not establish equivalence.
Reuse a compatible definition, or fork an inspected metric when only period or population differs.
Compatible extra slicers are useful; missing required capabilities can prevent reuse.

## Discover fields for a new definition

Use a supplied published datasource LUID, or resolve its exact name and project.
Do not substitute an embedded workbook datasource for a published source.

For a broad design or unresolved field meaning, inspect the relevant measure inventory rather than filtering to one quantity.
When the user supplies exact fields and material semantics, validate those fields directly instead of broadening discovery.
For entity counts, search the identifier among dimensions as well as measures.
Schema `--query` is a case-insensitive substring, not semantic search or an OR expression.
An empty query result does not prove that the intended quantity is absent.

For a broad design or unresolved slicing need, consider the complete eligible dimension inventory for the source, even when no breakdown was requested.
For a fully specified configuration, validate the supplied dimensions directly and do not add a broader inventory or extra slicers.
Reuse a completed inventory for related definitions on that unchanged source.
Use reported coverage to distinguish a bounded sample from complete discovery.
If the bound is exceeded, partition discovery by supported role or table filters and keep coverage explicit.
`--cache` uses only previously captured schema and never establishes current completeness.

If field meaning or role remains unresolved, inspect the selected measure, date, and derived dimensions together before finalizing them.
For supplied exact fields with established material semantics, validate those fields directly without an extra inspection.
Do not accidentally retain discovery filters that exclude one of these selected fields.
Pulse creation also accepts unique captions or labels and resolves them to raw IDs automatically before previewing or publishing.
Duplicate captions can belong to different tables; an ID that still matches multiple fields remains ambiguous.
Use full metadata for derived dimensions when their meaning requires it.
Read the semantics reference for calculations, counts, averages, rates, percentages, snapshots, and measures spanning multiple facts.
Resolve material values questions with authorized data evidence when metadata is insufficient.

## Select generous, useful slicers

Include dimensions reasonably related to the metric and favor inclusion when their usefulness is uncertain.
Consider geography, product hierarchies, brands, channels, customers, accounts, segments, stores, facilities, suppliers, owners, teams, and statuses.
Retain complementary hierarchy levels.
High-cardinality customers, stores, products, and SKUs can be valuable filters; neither cardinality nor identifier-like appearance alone justifies exclusion.
Exclude ineligible dimensions and clearly unrelated, sensitive, or purely technical fields.
Do not limit the set to the breakdowns the user named or the number of flags in an example.

Choose at least one useful eligible dimension; never invent a dummy field.
Rank the most valuable insight dimensions first.
The first 20 adjustable dimensions support insights and breakdowns; additional useful dimensions remain filters.
TADX preserves your supplied order while deduplicating IDs.
Keep useful fields beyond 20 instead of silently truncating the set.

Allowed dimensions enable slicing but do not select members or change the headline population.
Use a metric variant for a requested region or segment.
A fixed definition such as sales excluding returns requires actual implementing logic; an allowed Status dimension does not implement it.

## Resolve settings and feasibility

| Choice | Business reasoning |
| --- | --- |
| Quantity | SUM additive contributions once at their grain; AVERAGE weights rows; COUNT counts non-null observations; COUNT_DISTINCT counts distinct values; MIN/MAX select extrema, not latest-by-date. |
| Calculation | Use USER when `requires_user_aggregation` is true; row-level calculations can need SUM. |
| Time field | Use the intended event or snapshot date, not an ETL timestamp chosen for recency. |
| Temporality | OVER_TIME for period activity; LATEST for a supported snapshot convention, not per-entity historical row selection. |
| Minimum granularity | Use DAY nearly always. Use a coarser value only when existing metadata or conventions clearly establish that need, or the user explicitly requires it. Do not query values just to discover grain. |
| Chart | Running totals require additive cumulative meaning, SUM, and OVER_TIME; keep ratios, percentages, averages, and snapshots non-cumulative. |
| Units | Select NUMBER, CURRENCY, or PERCENT from evidence. Currency formatting does not convert currencies, and percent formatting does not repair scale. |
| Sentiment | UP for favorable increases, DOWN for favorable decreases, NONE when neither direction is universally favorable. |

Month-to-date is a reporting period, not a reason to choose MONTH minimum granularity.
If the task requires a particular period or population, check the variants contract before creating its base.
Resolve missing fixed filters, inline calculations, custom calendars, comparisons, or offsets before writing a materially different substitute.
For a current or demo-ready result, establish data recency from available evidence or a bounded authorized data query when needed.
LATEST and a different grain do not make historical data current.
State when current values or insight readiness remain unverified.
