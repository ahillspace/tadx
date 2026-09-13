# Fork a metric's period or population

Read before forking and before creating a base whose requested result depends on a particular period or population.
A fork keeps the shared measure, aggregation, date, allowed dimensions, units, and sentiment.
It changes the metric's period or filters within that definition.

## Inspect the base and verify member values

Inspect the source metric's complete filters and period, plus the definition's allowed dimensions and granularities.
Use exact member values from an inspected existing metric, precise user input, or other verified data evidence.
Schema gives field identities, not member values; a field name does not establish that West, Enterprise, or Active exists.

Provide at least one period or filter change.
Repeated members for one field form alternatives; filters on different fields apply together.
Use separate flags for separate values: a comma-separated string remains one literal member.
TADX deduplicates and sorts member values within a field.
Unique field captions and labels resolve to raw IDs before previewing or creating a variant.
Exact allowed raw IDs take precedence; resolved fields must belong to the definition's allowed dimensions.
Using both the raw ID and its display name combines values for that field; mixing inclusion and exclusion fails.
Categorical member values are not translated from aliases.
Prefer a meaningful published grouping over an unnecessarily large member list.

## Preserve inherited population

A supplied field replaces the source filter for that field; unspecified field filters remain.
A period-only fork inherits every source filter.
Replacing a field also replaces its operator and null policy.
There is no clear-filter flag; choose an inspected appropriate base, often the default metric, when an unfiltered population is needed.
Definition-level and source-level filters still apply.

New filters use literal string members and exclude nulls.
An exclusion such as exclude X while retaining null or unknown rows is not expressible through these flags.
An untouched inherited filter retains its null policy, but replacing that field does not.
Do not coerce Boolean, numeric, or date members from their display labels without verified representation.
The literal member `null` is not a missing value.
Whitespace and commas within members remain literal; comparison operators and wildcards are not supported.
The first `=` separates field ID from member, so a field ID containing `=` has no supported escape syntax here.

## Select a compatible reporting period

| `--period` | Required grain | Reporting window |
| --- | --- | --- |
| TODAY | DAY | Current partial day |
| THIS_WEEK | WEEK | Current partial week |
| MONTH_TO_DATE | MONTH | Current partial month |
| QUARTER_TO_DATE | QUARTER | Current partial quarter |
| YEAR_TO_DATE | YEAR | Current partial year |
| YESTERDAY | DAY | Last complete day |
| LAST_WEEK | WEEK | Last complete week |
| LAST_MONTH | MONTH | Last complete month |
| LAST_QUARTER | QUARTER | Last complete quarter |
| LAST_YEAR | YEAR | Last complete year |
| LAST_7_DAYS, LAST_14_DAYS, LAST_30_DAYS, LAST_60_DAYS, LAST_90_DAYS | DAY | Rolling window including the current day |
| CUSTOM_N_DAYS with `--days` | DAY | Specified rolling day count including the current day |

The chosen grain must be allowed by the definition; TADX checks compatibility before writing.
A MONTH-minimum definition cannot support a daily rolling variant.
Existing calendars and offsets can affect the meaning of periods, so inspect them when material.

## Preview, execute, and verify

Review the default preview's final population, including inherited filters, replacements, null policy, period, and definition linkage.
Check the resulting population, not just the requested changes.
An explicitly incomplete summary needs expanded evidence; do not treat omitted or unsupported settings as reviewed.
Check `review_complete` and `requires_full`; `--full` can expose omitted details but does not establish missing semantics.
For authorized execution, reuse the reviewed flags and remove only `--preview`.
Use the fork's verified saved read-back in `--full` output to compare the complete filters and period to the intended result.
Inspect the exact metric separately only when verification is unresolved, evidence is missing, or you need a later observation.
`created: false` with a returned metric identity means successful reuse.
Keep definition linkage and reconciliation evidence; inventory visibility can lag an exact verified object and is not required for fork completion.
A failed follow-up inspection does not prove the fork failed.
Retain its ID, reconcile read-only, and report unresolved verification before another write.

No fork changes shared measure math, allowed dimensions, or fixed definition filters.
Missing required dimensions need definition-edit capability, not an invented filter key.
There are no arbitrary historical start/end-date flags.
Use the operations route when subscriptions or deletion are requested.
The configuration-context reference distinguishes saved settings from numeric verification.
