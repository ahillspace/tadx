# Record Tableau MCP delegation

## Boundary

TADX owns Tableau lifecycle, artifacts, workspaces, administration, configuration, and Pulse object lifecycle.
Tableau MCP owns analytical data, rendered view results, and generated Pulse insights.
TADX never configures, selects, calls, proxies, or reports the connection state of Tableau MCP.
The user and host agent own Tableau MCP configuration, active connection selection, and availability handling.

## Local source

The targeted source is `Tableau API Documentation/tableau_mcp_tools.md`.
The source describes the exact Tableau MCP tool names and their primary parameters.
This inventory records routing evidence only and does not make the tools executable through TADX.

## Delegated analytical intents

| Intent | Tableau MCP tool | Routing note |
| --- | --- | --- |
| Discover published views | `list-views` | Use bounded filters before exact view inspection. |
| Inspect one view | `get-view` | Supply the exact view LUID. |
| Read published-view data | `get-view-data` | Use the view LUID and optional filters. |
| Render a published-view image | `get-view-image` | Use only when the user requests an image artifact. |
| Discover saved custom views | `list-custom-views` | Supply the owning workbook LUID. |
| Read saved custom-view data | `get-custom-view-data` | Supply the exact custom-view LUID. |
| Render a saved custom-view image | `get-custom-view-image` | Supply the exact custom-view LUID. |
| Inspect published datasource fields | `get-datasource-metadata` | Ground analytical queries in returned field metadata. |
| Query a published datasource | `query-datasource` | Use field captions verified through metadata. |
| List current-user Pulse subscriptions | `list-pulse-metric-subscriptions` | Resolve returned metric IDs through Pulse read tools when needed. |
| Generate Pulse values and ranked insights | `generate-pulse-metric-value-insight-bundle` | Supply complete metric context. |
| Generate a Pulse insight brief | `generate-pulse-insight-brief` | Group related metrics by datasource. |

## Exclusions

Do not add fake TADX command paths for these intents.
Do not route TADX-owned lifecycle mutations to Tableau MCP.
Do not advertise Tableau MCP mutation tools for capabilities that TADX owns or explicitly defers.
Do not add Tableau MCP checks to `tadx doctor`, capability discovery, or authentication commands.
