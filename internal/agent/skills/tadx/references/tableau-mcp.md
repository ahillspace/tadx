# Tableau MCP analytical work

## Available tools

Tool arguments can vary by installed Tableau MCP version, so use tool discovery only when an unresolved parameter is required.

| Tool | What it does | Useful inputs |
| --- | --- | --- |
| `query-datasource` | Query values from a published datasource with bounded grouping, filtering, and aggregation. | Exact datasource LUID, verified field captions, fields, filters, grouping, and row limits |
| `get-datasource-metadata` | Retrieve analytical metadata needed for a datasource query when TADX schema results are insufficient. | Exact datasource LUID and the narrowest available field selection |
| `get-view-data` | Retrieve CSV data rendered by one published view. | Exact view LUID and bounded view filters |
| `get-view-image` | Render a static published-view image. | Exact view LUID, requested format, filters, and bounded dimensions |
| `list-custom-views` | Resolve saved custom views belonging to one workbook. | Exact workbook LUID and bounded filters |
| `get-custom-view-data` | Retrieve data from one saved custom-view state. | Exact custom-view LUID and bounded filters |
| `get-custom-view-image` | Render an image of one saved custom-view state. | Exact custom-view LUID, requested format, and bounded dimensions |
| `list-pulse-metric-subscriptions` | Discover the current user's Pulse subscriptions for analytical follow-up. | Bounded subscription filters supported by the installed server |
| `generate-pulse-metric-value-insight-bundle` | Retrieve current values, period comparisons, and ranked insights. | Complete context for exact metric LUIDs |
| `generate-pulse-insight-brief` | Generate a natural-language answer or summary across related metrics. | Complete metric context, question, and metrics grouped by datasource |
| Host-provided interactive Tableau renderer | Open or render an interactive visualization when no TADX equivalent exists. | Exact view identity and the host's supported filter or embed options |

## Boundary

TADX is the default for discovery, identity, supported datasource schema inspection, lineage, lifecycle, administration, Pulse object lifecycle, workspaces, and local artifacts.
Tableau MCP is for datasource values and cardinality profiling, published or custom view data and images, current Pulse values and generated insights, and interactive visualization rendering without a TADX equivalent.

Do not defer to Tableau MCP for discovery, identity, TADX-supported schema inspection, lineage, lifecycle, administration, or local artifact work.
Use `tadx content datasource schema` before `get-datasource-metadata` for field and table discovery because TADX returns a compact targeted projection.
Use MCP metadata only when an analytical query requires details TADX does not expose.

The host agent and user own Tableau MCP configuration, availability, and active connection selection.
TADX never configures, selects, calls, proxies, or reports the connection state of Tableau MCP.

## Efficient analytical flow

Resolve content and exact LUIDs with TADX.
For datasource questions, search the needed fields with TADX schema, then call `query-datasource` using verified captions.
Use `get-datasource-metadata` only for missing analytical details or query construction that TADX schema cannot support.
For published or custom views, use TADX to resolve lifecycle identity, then request only the data or image the user needs through MCP.
Do not substitute a static image when the user requests an interactive visualization.

Use Tableau MCP for Pulse values and insights.
Use the separate `tadx-pulse` Guidance for definition creation, inspection, pull, deletion, metric variants, and follower lifecycle.
