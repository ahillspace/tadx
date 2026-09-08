# Retrieve actual Pulse values and insights

| Action or tool | Purpose | Selectors and useful flags |
| --- | --- | --- |
| `tadx search <term>` | Resolve a named metric or definition. | `--environment`, `--type metric\|definition\|pulse`, `--limit` |
| `tadx pulse definition list` | Resolve an exact definition name across provider pages. | `--environment`, `--name <exact-name>`, `--all`, `--full` |
| `tadx pulse metric list` | Discover variants for a known definition. | `--environment`, `--definition-id`, `--all`, `--full` |
| `tadx pulse metric inspect` | Verify complete saved metric context. | `--environment`, `--id <metric-luid>`, `--full` |
| `tadx pulse definition inspect` | Verify shared measure and date context. | `--environment`, `--id <definition-luid>`, `--full` |
| Tableau MCP value bundle or insight brief | Retrieve actual values, comparisons, and generated insights. | Use the connected tool's schema with exact metric identities and context. |

Use this route for data queries, numeric verification, existing metric values/insights, and briefs.
Authoring a definition configures what Pulse will calculate; it is not a data query.
An analytics-only task does not require the creation contract.

## Tools and identities

TADX owns datasource/field discovery, definitions, variants, and followers.
It does not select, configure, call, proxy, or check the connection state of Tableau MCP.
For actual analytics, discover the connected tool schemas before constructing arguments.
Official Tableau MCP tool names include:

- `generate-pulse-metric-value-insight-bundle` for metric values, comparisons, and ranked insights.
- `generate-pulse-insight-brief` for questions or summaries across related metrics.

Availability and arguments depend on the connected version.
These names are not request schemas.
Do not invent tool parameters, JSON wrappers, MCP URLs, authentication headers, or direct HTTP workarounds.
Report an unavailable tool instead of inventing a result.
For datasource questions such as percentage scale or member values, use the actual connected query tool with a bounded, specific question.
Do not export the entire datasource when a small query answers it.

Resolve a named metric before asking Tableau MCP for values or insights.
Search the business name with `--type metric`, inspect the returned metric LUID, and verify its full period and population.
If the name identifies a definition, find it with exact `definition list --name`, then list its metrics and choose the matching inspected variant.
Use `--all` for complete definition or metric discovery, bounded at 10,000 records.
Do not choose the first ambiguous name match or substitute a definition LUID for a metric LUID.

```text
tadx search --environment '<alias>' --type metric '<business-name>'
tadx pulse definition list --environment '<alias>' --name '<exact-definition-name>' --all --full
tadx pulse metric list --environment '<alias>' --definition-id '<definition-luid>' --all --full
```

Inspect the chosen identities:

```text
tadx pulse metric inspect --environment '<alias>' --id '<metric-luid>' --full
tadx pulse definition inspect --environment '<alias>' --id '<definition-luid>' --full
```

## Carry enough context for the question

Retain environment/site, datasource/definition/metric LUIDs, business name, measure/aggregation, time field and temporality, units/currency, and the metric's complete filters and actual period.
Include offsets, calendars, comparison basis, and relevant dimensions when material.
Pass only the arguments required by the discovered tool schema; this context record is not a universal input format.

Do not substitute shorthand such as MONTH_TO_DATE for a required saved specification, discard inherited filters, mix site IDs, or substitute an unfiltered metric.
Source access and row-level security can make two callers' results differ.
Numeric comparisons require the same relevant identity/access context, date basis, period, population, aggregation, and scale.

For a brief, group metrics by datasource while preserving individual periods/populations.
Do not compare unlike currencies, fiscal conventions, or time windows as equivalent.
An allowed dimension is not proof that every member exists or applies to every measure.
A missing allowed dimension cannot simply be sent as a fork filter.

Creating a new persisted variant requires the variants route in SKILL.md.
Do not create one merely to answer a question when the connected analytics tool already supports the requested exploration.
An analytics request does not automatically authorize a write, subscription, or broader access.

## Interpret only the returned evidence

Distinguish configuration verified, values retrieved, and insights retrieved.
An empty response is not zero; a tool failure is not evidence of no change.
Preserve units/scale, population, period, comparison basis, and freshness.
Do not multiply a percentage again, relabel currency, reverse sentiment, or fabricate unavailable insights.

Distinct counts are generally non-additive across overlapping segments or periods.
An entity can occur once in each segment but once in the combined distinct total; differing breakdown sums are not by themselves evidence of an incorrect metric.
Ratios and averages likewise need their underlying weighting, not sums of displayed values.

Separate a contribution/correlation from a causal explanation.
Support conclusions with actual returned evidence.
State when numeric validation remains unperformed rather than treating a successful configuration read as a numeric test.

Keep the small verified context needed for follow-up questions, not all schema pages or unrelated metric bundles.
Re-fetch when settings could have changed or evidence has left the working context.
