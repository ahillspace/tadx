---
name: tadx
description: Use whenever a user asks to work with Tableau Cloud or Tableau Server, even when they do not mention TADX. TADX finds, inspects, downloads, publishes, moves, renames, deletes, organizes, and administers Tableau resources, and manages local workspaces, catalogs, artifacts, and Pulse definitions. Use Tableau MCP only for analytical results that TADX does not provide.
---

# Operate Tableau with TADX

## What TADX is

TADX is the deterministic Tableau lifecycle and development CLI for agents and humans.
It owns content discovery and lifecycle, metadata and lineage, catalogs, local artifacts, workspaces, administration, and Pulse definition lifecycle.
Use TADX by default for Tableau work.

Default output is compact TOON.
Use `--full` only when expanded bounded details are needed.
`--env` is an alias for `--environment` on commands that accept an environment.
Use one relevant leaf `--help` only when this Guidance and its references do not answer the question.

## TADX versus Tableau MCP

Use Tableau MCP only for:

- Datasource value queries and cardinality profiling.
- Published-view and custom-view data.
- Published-view and custom-view images.
- Current Pulse values and generated insights.
- Interactive visualization rendering without a TADX equivalent.

Do not defer to Tableau MCP for discovery, identity, TADX-supported metadata or schema inspection, lineage, lifecycle, administration, workspaces, or local artifacts.
Use TADX datasource schema discovery for tables, fields, roles, data types, and aggregations.
TADX never calls, proxies, configures, or reports the connection state of Tableau MCP.

## Critical rules

- Reuse known environment aliases, workspace names, and LUIDs instead of rediscovering them.
- Use exact LUIDs after discovery.
- Never select the first fuzzy match or resolve ambiguity interactively.
- TADX authenticates with PATs only.
- Never expose PATs or session tokens.
- Never run concurrent authenticated TADX commands with the same PAT because a new Tableau session can invalidate the other session.
- Remote mutations run by default when `TADX_ENABLE_MUTATIONS=1`; use `--preview` when review is useful.
- Discovery and previews do not authorize mutation, and `--force` never bypasses mutation policy.
- Inspect an uncertain remote outcome before retrying a write.
- Keep persisted and rendered artifact paths relative to the workspace with forward slashes.

## Catalog versus live reads

Live reads are the default.
Live `tadx search` uses Tableau's native search and inherits the search capabilities and ranking available on that site.
`--catalog` searches cached metadata locally using lexical matching only.
Prefer live search for broad or conceptual discovery, and catalog search for fast, repeated known-term lookup.
Use the catalog when freshness is acceptable and the task benefits from repeated discovery, broad inventory, cross-resource comparison, or cached datasource schemas.
Refresh the catalog before broad or repeated work against an unknown or stale environment.
Use live reads for authoritative state before consequential changes, details not indexed in the catalog, targeted inspection after remote changes, and uncached datasource schemas.

`--catalog` is local-only and never falls back to Tableau.
A catalog miss does not prove remote absence.
Live schema reads write through to the catalog.
Targeted live reads do not establish complete inventory coverage.
A full refresh replaces the current generation rather than merging earlier scopes.
Request every required refresh scope together.
Treat the catalog as a cache, not authoritative truth for consequential remote changes.
An item-level permission denial can produce a usable `partial` catalog with explicit warnings and `complete: false`.
Inspect `tadx catalog status --full` before treating cached permission coverage as complete.

## Workspaces

A TADX workspace is a named, registered local directory for managed Tableau artifacts and metadata.
Pull commands write there, and publish commands read from there.
`--workspace` accepts the registered workspace name, not a filesystem path.
TADX does not silently create workspaces.
Returned artifact paths remain relative to the workspace so they are portable across machines.

## Read the relevant reference before acting

| Intent | Read first |
|---|---|
| Search, inspect, pull, publish, move, rename, delete, lineage, or datasource schema | [Content lifecycle](references/content-lifecycle.md) |
| Users, groups, memberships, ownership, permissions, or projects | [Administration](references/administration.md) |
| Workspace creation, registration, defaults, local artifact movement, root relocation, or cleanup | [Workspaces](references/workspace.md) |
| Datasource value queries or cardinality profiling | [Tableau MCP routing](references/tableau-mcp.md) |
| Published-view or custom-view data or images | [Tableau MCP routing](references/tableau-mcp.md) |
| Current Pulse values or generated insights | [Tableau MCP routing](references/tableau-mcp.md) |
| Interactive visualization rendering without a TADX equivalent | [Tableau MCP routing](references/tableau-mcp.md) |
| Pulse definition creation, forking, validation, or management | Separate `tadx-pulse` Guidance |

Read the relevant reference before acting.
Do not act from this root summary alone when a reference owns the task.
Use leaf command help only as a fallback after reading that reference.
