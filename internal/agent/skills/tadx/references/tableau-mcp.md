# Route analytical work to Tableau MCP

Use TADX for lifecycle and administrative state.
Use Tableau MCP directly when the requested result contains data values, rendered views, or generated Pulse insights.
The host agent and user own Tableau MCP configuration, connection selection, and availability.
TADX never configures, selects, calls, proxies, or reports the connection state of Tableau MCP.

## Discover views

Use these Tableau MCP tools for view discovery:

- Use `list-views` to find sheets and dashboards by bounded metadata filters.
- Use `get-view` with an exact view LUID to inspect its workbook, project, datasources, tags, and usage facts.
- Use `list-custom-views` with a workbook LUID to find saved view states.

Use TADX search and inspect commands when the task needs lifecycle identity or artifact operations instead of analytical view details.

## Retrieve view results

Use `get-view-data` for CSV data from a published view.
A dashboard can return only its first view's data through this tool.
Use `get-custom-view-data` when the requested result depends on a saved custom-view state.

Use `get-view-image` only when the user explicitly requests an image artifact.
Use `get-custom-view-image` for an image of a saved custom view.
Do not substitute a static image when the user requests an interactive view.

## Query a published datasource

Call `get-datasource-metadata` with the datasource LUID before constructing a `query-datasource` request.
Use the returned field captions, data types, roles, descriptions, and parameters to ground the query.
Call `query-datasource` for aggregated, filtered, or grouped business results.
Do not infer field captions from a TADX content name or artifact payload.

## Read Pulse analytics

Use `list-pulse-metric-subscriptions` to discover the current user's Pulse subscriptions.
Use `generate-pulse-metric-value-insight-bundle` for current values, period comparisons, and ranked insights.
Use `generate-pulse-insight-brief` for natural-language explanations or summaries across related metrics.
Load complete metric context through Tableau MCP before generating a bundle or brief.
Use the separate `tadx-pulse` Guidance when the task creates, forks, follows, unfollows, or deletes Pulse objects.
