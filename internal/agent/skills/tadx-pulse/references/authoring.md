# Design a useful Pulse definition

| Action | Purpose | Selectors and useful flags |
| --- | --- | --- |
| `tadx search <term>` | Discover a published datasource or existing Pulse object. | `--environment`, `--type datasource\|pulse`, `--limit` |
| `tadx pulse definition list` | Find existing definitions and conventions. | `--environment`, `--name <exact-name>`, `--datasource-id <luid>`, `--all`, `--full` |
| `tadx pulse definition inspect` | Compare actual business and display settings. | `--environment`, `--id <definition-luid>`, `--full` |
| `tadx content datasource inspect` | Verify an exact published source. | `--environment`, `--id <datasource-luid>`, `--full` |
| `tadx content datasource schema` | Discover measures, dates, dimensions, and formulas. | `--environment`, `--id <datasource-luid>`, `--query`, `--role`, `--table`, `--field-id`, `--limit`, `--all`, `--full` |

Use this reference for recommendations and creation design.
Before creating or previewing, also read the creation contract.

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

```text
tadx pulse definition list --environment '<alias>' --datasource-id '<datasource-luid>' --all --full
tadx pulse definition inspect --environment '<alias>' --id '<definition-luid>' --full
```

## Discover fields for a new definition

Use a supplied published datasource LUID, or resolve its exact name and project.
Do not substitute an embedded workbook datasource for a published source.

```text
tadx search --environment '<alias>' --type datasource '<source-concept>'
tadx content datasource inspect --environment '<alias>' --id '<datasource-luid>' --full
tadx content datasource schema --environment '<alias>' --id '<datasource-luid>' --role measure --query '<measure-concept>' --limit 100
tadx content datasource schema --environment '<alias>' --id '<datasource-luid>' --role date --all
tadx content datasource schema --environment '<alias>' --id '<datasource-luid>' --role dimension --all
```

For a broad metric set, omit the measure query and inspect the relevant measures with `--all`.
For entity counts, search the identifier among dimensions as well as measures.
Schema `--query` is a case-insensitive substring, not semantic search or an OR expression.
An empty query result does not prove that the intended quantity is absent.

Consider the complete eligible dimension inventory for each new source, even when no breakdown was requested.
Reuse it for related definitions on that unchanged source.
Schema defaults to 20 returned fields; `--limit` and `--all` support up to 10,000 matching fields.
Use `more_available` to detect bounded output and `--all` for complete discovery; do not combine `--all` with `--limit`.
If the bound is exceeded, partition discovery by supported role or table filters and keep coverage explicit.
`--catalog` uses only previously captured schema and never establishes current completeness.

If the required full details were not already returned, read selected measure, date, and derived dimension IDs together before finalizing them:

```text
tadx content datasource schema --environment '<alias>' --id '<datasource-luid>' --field-id '<exact-measure-id>' --field-id '<exact-date-id>' --field-id '<exact-derived-dimension-id>' --full
```

Remove prior role, query, and table filters for that exact-field read.
Repeat `--field-id` for each needed field; one invocation fetches the schema once and rejects missing or ambiguous selections.
Use the returned `id` for schema `--field-id` selections.
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
