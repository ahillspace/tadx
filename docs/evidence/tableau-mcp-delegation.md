# Analytical scope boundary

## Boundary

TADX owns Tableau lifecycle, artifacts, workspaces, administration, configuration, and Pulse object lifecycle.
Analytical data queries, rendered view results, and generated Pulse insights are outside TADX's scope.
Tableau MCP is a separate integration that can support those outcomes.
TADX never configures, selects, calls, proxies, or reports the connection state of Tableau MCP.
The user and host agent own Tableau MCP configuration, active connection selection, and availability handling.

## Local source

The historical targeted source is `Tableau API Documentation/tableau_mcp_tools.md`; see [capture availability](README.md).
Its header identifies generated server tool definitions, but records no official source URL or server version.
The [official Tableau MCP documentation](https://tableau.github.io/tableau-mcp/docs/configuration/mcp-config/site-settings) provides current site configuration requirements.
That page does not establish the historical tool schemas in the capture.
The capture describes server tool names and primary parameters; this record preserves only the scope boundary.
It does not establish current server capabilities or prescribe an external workflow.

## Out-of-scope analytical intents

The historical inventory covered these analytical outcomes:

- Published and custom-view discovery, data retrieval, and image rendering.
- Analytical datasource queries and metadata used to construct them.
- Current-user Pulse subscriptions, metric values, insights, and briefs.

TADX's own datasource schema discovery and Pulse object lifecycle remain separate from those analytical outcomes.

## Exclusions

TADX provides no proxy commands or automatic handoff for these analytical intents.
Its diagnostics, capability discovery, and authentication commands do not test Tableau MCP connectivity.
Users retain their choice of tools for both lifecycle and analytical work.
