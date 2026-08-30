# Tableau MCP Tools - Complete Reference

Generated from the Tableau MCP server tool definitions (the `tableau-local_*` tools). This is the operational surface an agent uses to explore Tableau content, read/query data source views, and manage Tableau Pulse metrics.

## Contents

1. [Content Discovery & Listing](#content-discovery--listing)
2. [Views & Visualizations (Data and Images)](#views--visualizations-data-and-images)
3. [Data Sources & Querying](#data-sources--querying)
4. [Search](#search)
5. [Pulse: Metric Definitions](#pulse-metric-definitions)
6. [Pulse: Metrics](#pulse-metrics)
7. [Pulse: Subscriptions & Insights](#pulse-subscriptions--insights)

---

# Content Discovery & Listing

## list-workbooks

Retrieves a list of workbooks on a Tableau site (name, description, views inside them) with optional `field:operator:value` filtering.

**Parameters**
- `filter` *(optional)* - `field:operator:value` expression string, e.g. `name:eq:Superstore`. Fields: `createdAt`, `contentUrl`, `displayTabs`, `favoritesTotal`, `hasAlerts`, `hasExtracts`, `name`, `ownerDomain`, `ownerEmail`, `ownerName`, `projectName`, `sheetCount`, `size`, `subscriptionTotal`, `tags`, `updatedAt`. Operators: `eq`, `in`, `gt`, `gte`, `lt`, `lte`.
- `pageNumber` *(optional)* - 1-based page (default 1); 1000-item pages.
- `limit` *(optional)* - max items on the page (<= 1000); use `limit: 1` to read `totalAvailable` cheaply.

**Notes**
- Paginate until you have gathered `totalAvailable` items.
- Filter expressions: no `&` or `,` inside values; combine multiple fields with `,` (logical AND); same field repeated uses last occurrence.
- Filtering guidance in the tool doc applies to all list-* tools below.

## list-views

Lists views (sheets) with metadata: name, caption, owner, workbook, sheetType, tags, usage hits.

**Parameters** - `filter`, `pageNumber`, `limit` as above. Filter fields include `name`, `caption`, `contentUrl`, `title`, `viewUrlname`, `sheetType`, `sheetNumber`, `workbookName`, `projectName`, `ownerEmail`, `ownerName`, `favoritesTotal`, `hitsTotal`, `tags`, `fields`, `createdAt`, `updatedAt`.

## list-datasources

Lists published data sources: connection metadata, owner, extract status, size, DB/user, tags.

**Parameters** - `filter`, `pageNumber`, `limit`. Filter fields include `name`, `connectionType`, `connectionTo`, `databaseName`, `databaseUserName`, `serverName`, `serverPort`, `authenticationType`, `connectedWorkbookType`, `hasExtracts`, `hasEmbeddedPassword`, `hasAlert`, `isCertified`, `isConnectable`, `isDefaultPort`, `isHierarchical`, `isPublished`, `tableName`, `projectName`, `size`, `tags`, `createdAt`, `updatedAt`, `ownerName`, `ownerEmail`.

## list-projects

Lists projects (including parent project, content permissions, owner).

**Parameters** - `filter`, `pageNumber`, `limit`. Filter fields: `name`, `topLevelProject`, `parentProjectId`, `ownerName`, `ownerEmail`, `ownerDomain`, `createdAt`, `updatedAt`. Helpful: `topLevelProject:eq:true` and `parentProjectId:eq:<id>` for hierarchy exploration.

## list-custom-views

Lists custom views for a workbook (name, owner, originating view). The workbookId is always included in the query.

**Parameters**
- `workbookId` *(required, LUID)*
- `filter` *(optional)* - only `ownerId:eq:<id>` and `viewId:eq:<id>` are supported on this endpoint.
- `limit` *(optional, max 1000)*

## get-workbook

Returns info about a workbook (metadata + the views it contains). Use to enumerate a workbook's sheets before drilling into a specific view.

**Parameters** - `workbookId` *(required, LUID)*.

---

# Views & Visualizations (Data and Images)

## get-view

Returns facts only (no visual): upstream datasources, workbook, project, owner, tags, usage statistics for a view. Use as the discovery step before deciding how to render a view.

**Parameters** - `viewId` *(required, LUID)*.

## get-view-data

Retrieves the CSV data rendered by a view, including any saved/user filters.

**Parameters**
- `viewId` *(required - the view LUID from the content URL, NOT the published view id)*
- `viewFilters` *(optional)* - map of filter field name to value, e.g. `{"Region": "West"}`.

**Notes**
- For dashboards, only the dashboard's **first** view's data is returned.
- For personalized/saved filter states use `get-custom-view-data` instead.

## get-view-image

Returns a static, non-interactive image (PNG or SVG) of a view.

**Parameters**
- `viewId` *(required)*
- `format` *(optional, "PNG" default)* - use "PNG" when the image will be analyzed/interpreted; "SVG" when it will be displayed to a user (scalable, smaller files).
- `width` / `height` *(optional, px)*
- `viewFilters` *(optional)* - filter field name/value map.

**Notes**
- Use ONLY when the user explicitly wants an image artifact (screenshot, thumbnail, embed, export). For a bare "show me / open / explore" request the user wants the interactive embed, not this tool.
- For custom (saved) views, use `get-custom-view-image`.

## get-custom-view-data

Returns CSV data for a Tableau Custom View (saved/personalized view state), including the user's filters.

**Parameters**
- `customViewId` *(required - the custom view LUID from the content URL, not the published view id)*
- `viewFilters` *(optional)*

**Notes** - For published (non-custom) views use `get-view-data`.

## get-custom-view-image

Returns a PNG/SVG image of a saved custom view.

**Parameters**
- `customViewId` *(required - LUID from the content URL)*
- `format` *(optional, "PNG" default)*
- `width` / `height` *(optional, px)*
- `viewFilters` *(optional)*

---

# Data Sources & Querying

## get-datasource-metadata

Returns the datasource model, fields, and parameters for a published data source, gathered from Tableau's VizQL Data Service and enriched with the Metadata API.

**Parameters** - `datasourceLuid` *(required)*

**Notes** - Use this FIRST to ground any `query-datasource` call: field names, dataType, roles, descriptions, parameters. Query fields must match the datasource's actual field captions/names.

## query-datasource

Executes VizQL queries against a published Tableau data source to answer business questions with aggregated, filtered, grouped data. This is the primary "ask the data" tool.

**Parameters**
- `datasourceLuid` *(required)*
- `query.fields` *(required)* - each field is one of:
  - dimension: `{fieldCaption}` (e.g. `{"fieldCaption": "Category"}`)
  - measure: `{fieldCaption, function}` with `SUM|AVG|MEDIAN|COUNT|COUNTD|MIN|MAX|STDEV|VAR|COLLECT|YEAR|QUARTER|MONTH|WEEK|DAY|TRUNC_YEAR|TRUNC_QUARTER|TRUNC_MONTH|TRUNC_WEEK|TRUNC_DAY|AGG|NONE|UNSPECIFIED`, plus optional `fieldAlias`, `maxDecimalPlaces`, `sortDirection`, `sortPriority`
  - calculated: `{fieldCaption, calculation}` (Tableau calc syntax, e.g. `SUM([Profit]) / SUM([Sales])`) - note created fields cannot be referenced by other calculations/filters
  - bin: `{fieldCaption, binSize}` (positive number) - must be paired with a measure field on the same `fieldCaption`; overrides existing datasource bins; measures only
- `query.filters` *(optional)* - one of:
  - `SET` `{field, values[], exclude?}` - include/exclude specific values (strings, numbers, or booleans)
  - `TOP` `{field, howMany, direction: TOP|BOTTOM, fieldToMeasure:{fieldCaption,function}, context?}` - top/bottom N by a measure; pairs with `context` to scope (see Notes)
  - `MATCH` `{field, startsWith|contains|endsWith|exclude?}` - substring/wildcard matches; cannot apply an aggregation function to the field
  - `QUANTITATIVE_NUMERICAL` `{min,max, coefficient...}` with `quantitativeFilterType` `RANGE|MIN|MAX|ONLY_NULL|ONLY_NON_NULL`
  - `QUANTITATIVE_DATE` `{minDate,maxDate}` with `quantitativeFilterType` `RANGE|MIN|MAX|ONLY_NULL|ONLY_NON_NULL`
  - DATE relative filters: `{periodType: MINUTES|HOURS|DAYS|WEEKS|MONTHS|QUARTERS|YEARS, dateRangeType: CURRENT|LAST|NEXT|TODATE|LASTN|NEXTN, anchorDate?, rangeN?}`
- `query.parameters` *(optional)* - `{parameterCaption, value}` list (numeric, string, boolean, null) tied to datasource-defined parameters.
- `limit` *(optional)*

**Notes**
- **Always prefer aggregation** (SUM/COUNT/AVG) over row-level pulls; profile first with a COUNT query when unsure of scale; `> 10,000` records → aggregate.
- Use TOP filters to rank at the database level. `context: true` on scope filters (SET/DATE/QUANTITATIVE) + `context: false` on the TOP filter = "top N *within* the scope".
- QUANTITATIVE_NUMERICAL min/max are **inclusive**; use small offsets for strict `>`/`<` (e.g. `min: 10.01` for ">10").
- Don't call it if fields/dimensions aren't available in the metadata; instead suggest alternative questions or another datasource.

## search-content

Keyword/free-text search across many content types at once (workbooks, views, datasources, projects, lenses, flows, tables, databases, virtual connections, data roles, collections), returned as a single ranked page of the top matches (default 100, max 2000).

**Parameters**
- `terms` *(optional)* - the search keywords.
- `filter.contentTypes` *(optional)* - `workbook|view|datasource|project|lens|flow|table|database|virtualconnection|datarole|collection`.
- `filter.ownerIds` *(optional)* - array of int owner ids.
- `filter.modifiedTime` *(optional)* - ISO 8601 range `{startDate, endDate}` or array of exact date-times.
- `orderBy` *(optional)* - `[{method, sortDirection}]`; methods `hitsTotal`, `hitsSmallSpanTotal` (last month), `hitsMediumSpanTotal` (last 3 mo), `hitsLargeSpanTotal` (last yr), `downstreamWorkbookCount` (only for database/table content); sortDirection `asc|desc` (default asc); first element is primary sort.
- `limit` *(optional, default 100, max 2000)*

**Notes** - Default sort is Tableau's relevance score. Prefer over list-* when you want the most popular/relevant items, don't know the content type, or want to hit several types at once.

---

# Pulse: Metric Definitions

## list-all-pulse-metric-definitions

Lists all published Pulse Metric Definitions on the current site.

**Parameters**
- `view` *(optional, DEFINITION_VIEW_BASIC default)*
  - `DEFINITION_VIEW_BASIC` - definitions only
  - `DEFINITION_VIEW_DEFAULT` - definitions + their default metric
  - `DEFINITION_VIEW_FULL` - definitions + up to 5 metrics each
- `limit` *(optional)* / `pageSize` *(optional)* - cap definitions returned / per request.

**Notes** - To see all metrics for definitions, fetch metrics via `list-pulse-metrics-from-metric-definition-id` (FULL view returns at most 5).

## list-pulse-metric-definitions-from-definition-ids

Fetches specific definitions by LUID list.

**Parameters** - `metricDefinitionIds` *(required, list of 36-char LUIDs)*, `view` *(optional, as above)*.

## create-pulse-metric-definition

Creates a Pulse metric definition - the shared blueprint (datasource, measure, time dimension, filterable dimensions, formatting) for all related metrics. Creating a definition automatically creates its default (unfiltered) metric.

**Parameters**
- `name` *(required)*
- `description` *(optional)*
- `specification` *(required)*:
  - `datasource.id` (LUID) and optional `datasource.id_type` (`DATASOURCE_ID_TYPE_LUID` default, or `DATASOURCE_ID_TYPE_WORKBOOK_DATASOURCE` for embedded)
  - exactly one of `basic_specification` { `measure` {field, aggregation: `AGGREGATION_SUM|AVERAGE|MEDIAN|MAX|MIN|COUNT|COUNT_DISTINCT|USER`}, `time_dimension` {field}, `filters[]` {field, operator `OPERATOR_EQUAL|OPERATOR_NOT_EQUAL`, categorical_values []} } OR `viz_state_specification` {viz_state_string}
  - `is_running_total` (bool, usually false)
  - `temporality` *(optional)*: `TEMPORALITY_OVER_TIME` (default) or `TEMPORALITY_LATEST_POINT_IN_TIME` (omits time_dimension; no running totals)
  - **field naming**: use RAW SOURCE COLUMN NAMES as in the datasource extract (CSV headers / DB column names - often lowercase for CSV; multi-table datasources may append `" (table.csv)"`). VizQL display names fail with error 404904.
- `extensionOptions` *(required)* - `allowed_dimensions` (e.g. `["Region","Category"]`), `allowed_granularities` (e.g. `GRANULARITY_BY_DAY|GRANULARITY_BY_WEEK|GRANULARITY_BY_MONTH`), optional `offset_from_today`.
- `representationOptions` *(required)* - `type` `NUMBER_FORMAT_TYPE_NUMBER|PERCENT|CURRENCY`, `sentiment_type` `SENTIMENT_TYPE_UP_IS_GOOD|DOWN_IS_GOOD|NONE`; currency needs `currency_code` (e.g. `CURRENCY_CODE_USD`); counts can set `number_units` {singular_noun, plural_noun}.
- `insightsOptions`, `comparisons`, `datasourceGoals` *(optional)*.

**Notes**
- Validate first with `validate-pulse-metric-definition` (identical payload, no side effects) to catch enum/shape rejections.
- Requires write+publish access to the datasource. Tableau Server does not support Pulse.
- Goals/thresholds can be plain targets or viz-state specs.

## update-pulse-metric-definition

Updates a definition (rename, change measure/time-dim/filters, allowed dimensions/granularities, formatting, insight settings). Omitted params are unchanged, BUT if `specification` is provided it must be COMPLETE (the API replaces the whole spec, not a partial diff).

**Parameters** - `definitionId` *(required)* plus any of: `name`, `description`, `specification`, `extensionOptions`, `representationOptions`, `insightsOptions`, `comparisons`, `datasourceGoals`.

**Notes** - Fetch the live definition first (list-all / list-from-ids) and rebuild the spec from current values. Requires write+publish access to the datasource.

## delete-pulse-metric-definition

**DESTRUCTIVE - cascading**: deletes the definition AND all its metrics AND all subscriptions/follows. Every follower loses the metric permanently. Confirm the cascade with the user before calling.

**Parameters** - `definitionId` *(required)*.

**Notes** - Show the user how many metrics are affected first (`list-pulse-metrics-from-metric-definition-id`). Prefer rename/disable via update when the intent is cleanup. Verifies datasource against any allowed-datasources config before deleting.

## validate-pulse-metric-definition

Pre-flight validation of a proposed definition payload WITHOUT calling Tableau (no side effects). Checks name/description shape; spec completeness (datasource.id + exactly one of basic/viz-state); aggregation enum; time_dimension present unless `TEMPORALITY_LATEST_POINT_IN_TIME`; running-total conflicts (AGGREGATION_USER, point-in-time); filter operator/categorical_values; extension option presence; representation options.

**Parameters** - identical to create, all optional; only provided fields are checked.

**Notes** - Does NOT verify fields exist in the datasource or publish access - pair with `get-datasource-metadata` for field checks.

---

# Pulse: Metrics

## list-pulse-metrics-from-metric-definition-id

Lists all published Pulse Metrics for a specific definition.

**Parameters** - `pulseMetricDefinitionID` *(required, 36-char LUID, not a name)*.

## list-pulse-metrics-from-metric-ids

Lists Pulse Metrics given metric LUIDs (e.g. from a subscription's `metric_id`). Returns full metric metadata including specification (filters, granularity, comparison) and extension/representation options needed to build insight bundles.

**Parameters** - `metricIds` *(required list of 36-char LUIDs)*.

**Notes** - `00000000-0000-0000-0000-000000000000` is never a valid datasource id; fetch the parent definition for valid datasource info.

## create-pulse-metric

Creates a customized variant ("fork") of an existing definition: applies a specific measurement period, comparison, and optional dimension filters on top of the definition's shared measure/time dimension. Use ONLY for ADDITIONAL variants - creating a definition already makes its default (unfiltered) metric.

**Parameters**
- `definitionId` *(required)*
- `specification` *(required)*: `measurement_period` {granularity `GRANULARITY_BY_DAY|WEEK|MONTH|QUARTER|YEAR|FISCAL_QUARTER|FISCAL_YEAR`, range `RANGE_CURRENT_PARTIAL|RANGE_LAST_COMPLETE|RANGE_BY_CONFIG`}, `comparison` {`TIME_COMPARISON_NONE|PREVIOUS_PERIOD|YEAR_AGO_PERIOD|FISCAL_YEAR_AGO_PERIOD`}, optional `filters` [{field, operator `OPERATOR_EQUAL|NOT_EQUAL`, categorical_values [{string_value|bool_value|null_value}]}].

**Notes** - Check `allowed_dimensions`/`allowed_granularities` on the parent definition first (filters on non-allowed dimensions are rejected). Preview insights before creating via `generate-pulse-metric-value-insight-bundle` with the same specification. Pulse is Cloud-only. Scopes: `tableau:insight_metrics:create`.

## update-pulse-metric

Updates a metric's measurement period, comparison, dimension filters, or goal. Omitted parameters unchanged; if `specification` is given it REPLACES the whole spec (include every filter you want to keep).

**Parameters** - `metricId` *(required)*, `specification` *(optional)*, `goals` {target {value}} *(optional)*.

**Notes** - Fetch the current metric first to preserve its filters. Requires write+publish access to the datasource. Scopes: `tableau:insight_metrics:update`.

## delete-pulse-metric

Deletes ONE metric variant without touching the parent definition or siblings. DESTRUCTIVE - followers lose it permanently; confirm first.

**Parameters** - `metricId` *(required)*.

**Notes** - A definition's DEFAULT metric cannot be deleted directly (400 error) - delete the whole definition instead. To remove all variants, delete the parent definition. Verifies datasource scope before deleting. Scopes: `tableau:insight_metrics:read` (ownership) + `tableau:insight_metrics:delete`.

---

# Pulse: Subscriptions & Insights

## list-pulse-metric-subscriptions

Lists all Pulse Metric Subscriptions for the current user.

**Parameters** - none.

**Notes** - Subscriptions return `metric_id` values - to see definitions, resolve metric ids via `list-pulse-metrics-from-metric-ids`, then definitions via `list-pulse-metric-definitions-from-definition-ids`.

## generate-pulse-metric-value-insight-bundle

Generates an insight bundle for a metric's current aggregated value: current value, period-over-period change, and top insights - rendered as HTML.

**Parameters**
- `bundleType` *(optional, "ban" default)*:
  - `ban` - current value + period-over-period change + highest-ranked insight per filterable dimension
  - `springboard` - current value, change, highest-ranked insight
  - `basic` - like springboard but focuses on low-bandwidth dimensions (small value sets)
  - `detail` - full: performance over time, high/low summary, top-contributor breakdowns, follow-up insights
- `bundleRequest` *(required)*:
  - `bundle_request.version` = 1
  - `bundle_request.options` = `{output_format: OUTPUT_FORMAT_HTML, time_zone: e.g. "UTC", language: LANGUAGE_EN_US, locale: LOCALE_EN_US}`
  - `bundle_request.input.metadata` = {name, metric_id, definition_id}
  - `bundle_request.input.metric.definition` = {datasource {id, id_type?} + basic_specification {measure {field, aggregation}, time_dimension {field}, filters[]} + is_running_total}
  - `bundle_request.input.metric.metric_specification` = {filters[], measurement_period {granularity, range}, comparison {TIME_COMPARISON_PREVIOUS_PERIOD|...}}
  - `bundle_request.input.metric.extension_options` = {allowed_dimensions[], allowed_granularities[], offset_from_today}
  - `bundle_request.input.metric.representation_options` = {type, number_units, row_level_id_field, row_level_entity_names, row_level_name_field, currency_code}
  - `bundle_request.input.metric.insights_options` = {show_insights, settings[]}
  - `bundle_request.input.metric.goals` = {target {value}}

**Notes** - The metric context must be COMPLETE (real allowed_dimensions/granularities, representation options, insights settings) or the API errors. Use data from `list-pulse-metrics-from-metric-ids` / `list-pulse-metrics-from-metric-definition-id`. Use `DATASOURCE_ID_TYPE_WORKBOOK_DATASOURCE` when the metric is based on an embedded workbook datasource.

## generate-pulse-insight-brief

AI-powered conversational insight briefs - natural-language answers ("why did sales increase?"), summaries across metrics, and advice, rendered for a chat interface.

**Parameters**
- `briefRequest` *(required)*:
  - `language` (e.g. LANGUAGE_EN_US) and `locale` (e.g. LOCALE_EN_US) *(required)*
  - `now` *(optional)* - "YYYY-MM-DD HH:MM:SS" or date
  - `time_zone` *(optional)*
  - `messages[]` *(required)* - each {action_type `ACTION_TYPE_ANSWER|SUMMARIZE|ADVISE`, content, role `ROLE_USER|ROLE_ASSISTANT`, `metric_group_context[]`, `metric_group_context_resolved` (bool)}
  - each metric_group_context item = {metadata {name, id, definition_id}, metric {definition (full), metric_specification, candidates[]}}

**Notes**
- Keep metrics in one `metric_group_context` call to the SAME datasource for best results (backend applies consistent filters); make separate calls per datasource otherwise.
- metric_group_context must include complete metric data (extension_options with real arrays, representation_options with sentiment/currency, insights_options settings) or the API errors.
- For follow-ups, include a compact summary of prior turns in `messages` (user Q + assistant answer + new question). Do NOT dump full histories.