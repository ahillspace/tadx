# Verify saved Pulse configuration

| Action | Purpose | Selectors and useful flags |
| --- | --- | --- |
| `tadx pulse metric inspect` | Read a saved metric's filters and period. | `--environment`, `--id <metric-luid>`, `--full` |
| `tadx pulse definition inspect` | Read its shared measure, date, and display settings. | `--environment`, `--id <definition-luid>`, `--full` |

Use known identities and verified evidence already available for the task.
When TADX configuration inspection is needed, inspect the metric and its definition in the same environment:

```text
tadx pulse metric inspect --environment '<alias>' --id '<metric-luid>' --full
tadx pulse definition inspect --environment '<alias>' --id '<definition-luid>' --full
```

A definition LUID is not a metric LUID.
The metric's saved period and full population, including inherited filters, determine the variant being verified.
Use the operations reference if TADX discovery is needed to resolve a missing identity.

Retain environment/site, datasource/definition/metric LUIDs, business name, measure and aggregation, time field and temporality, units/currency, filters, and actual period.
Include date offsets, calendars, and comparison basis when material.
These are configuration facts, not a prescribed request format for another tool.

TADX validates and inspects saved configuration; a successful read or write does not verify numeric correctness, source access for every viewer, data freshness, or insight availability.
State separately whether configuration, actual values, and insights were verified, using the evidence available.
Keep the user's selected tool and workflow for any data validation outside TADX.

Numeric comparisons require compatible identity/access context, period, population, aggregation, and scale.
Distinct counts are generally non-additive across overlapping segments or periods, and ratios and averages require their underlying weighting.
An empty response is not zero, and a failed read does not establish that nothing changed.
Do not relabel currencies or treat percentage formatting as data rescaling.

Keep the small verified context needed for follow-up work and refresh affected evidence after settings change.
