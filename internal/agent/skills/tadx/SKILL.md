---
name: tadx
description: Use for TADX CLI workflows involving Tableau content discovery and lifecycle, datasource schemas, administration, catalogs, workspaces, local artifacts, and Pulse definitions.
---

# Operate Tableau with TADX

## What TADX is

TADX is the deterministic Tableau lifecycle and development CLI for agents and humans.
It supports content discovery and lifecycle, metadata and lineage, catalogs, local artifacts, workspaces, administration, and Pulse definition lifecycle.
This Guidance teaches TADX commands; respect the user's chosen tools for other workflows.
TADX does not query datasource values, render views, or retrieve current Pulse values and insights.

Default output is compact TOON.
Use `--full` only when expanded bounded details are needed.
`tadx last` displays the previous execution's saved full result and timestamp without repeating it; one global result is retained, not history.
`--env` is an alias for `--environment` on commands that accept an environment.
Use one relevant leaf `--help` only when this Guidance and its references do not answer the question.

## Critical rules

- Reuse known environment aliases, workspace names, and LUIDs instead of rediscovering them.
- Use exact LUIDs after discovery.
- Never select the first fuzzy match or resolve ambiguity interactively.
- TADX authenticates with PATs only.
- Never expose PATs or session tokens.
- TADX serializes commands sharing a PAT on this machine; other machines and external tools need separate PATs or coordination because a new session can invalidate an existing one.
- Remote mutations run by default when enabled by the effective mutation policy; supported read-only `--preview` operations remain available when the gate is off.
- Discovery and previews do not authorize mutation, and `--force` never bypasses mutation policy.
- An error can include confirmed results in `output`; retain those identities and inspect uncertain outcomes before retrying a write.
- Keep persisted and rendered artifact paths relative to the workspace with forward slashes.

## Permission to change the mutation flag

Before changing `TADX_ENABLE_MUTATIONS` or the saved mutation setting, agents must obtain explicit user permission for that setting change and its scope.
This applies to enabling, disabling, or unsetting it through any mechanism, including command overrides, process or session environments, wrappers, scripts, shell profiles, and persistent user or machine settings.
A request to perform a Tableau operation does not authorize changing this flag.
Reuse prior permission only when it explicitly covers the same setting change and scope; session permission does not authorize persistence.
Ask one short question, for example: "May I enable remote mutations for this session, allowing TADX to create, change, or delete Tableau resources?"
For a persistent change, name its scope and explain that it affects future shells.
An already enabled flag does not authorize a remote operation; keep the requested operation within its separately authorized scope.
`tadx mutation status` reports the effective value and source.
`tadx mutation set --enabled=true` persists user policy until changed; `--enabled=false` disables the saved policy.
An explicitly configured `TADX_ENABLE_MUTATIONS=0` or `1` overrides the saved policy for that process; without either setting, execution is disabled.

## Environment selection

With one configured environment, TADX uses it when a selector is omitted.
With multiple environments, reads can use the configured default, but remote writes require `--env`.
Adding a second environment announces this transition; artifact provenance and workspace placement never choose a publish destination.

## Catalog versus live reads

Live reads are the default.
Live content searches use Tableau's native search and inherit the search capabilities and ranking available on that site.
`--catalog` searches cached metadata locally using lexical matching only.
Prefer live search for broad or conceptual discovery, and catalog search for fast, repeated known-term lookup.
Use the catalog when freshness is acceptable and the task benefits from repeated discovery, broad inventory, cross-resource comparison, or cached datasource schemas.
Refresh the required catalog scopes when their cached coverage or freshness does not meet the task.
Use live reads for authoritative state before consequential changes, details not indexed in the catalog, targeted inspection after remote changes, and uncached datasource schemas.

`--catalog` is local-only and never falls back to Tableau.
A catalog miss does not prove remote absence.
Live schema reads write through to the catalog.
Targeted live reads do not establish complete inventory coverage.
Ordinary content and administration lists and live searches use bounded provider reads without accessing SQLite.
`--all` explicitly collects all selected records within 10,000 and attempts a catalog update.
An unfiltered complete `--all` replaces that resource scope; filtered `--all` records observations without claiming complete site coverage.
A failed catalog write does not discard the collected live answer; retain its warning.
A refresh atomically replaces the requested inventory scopes while preserving independently cached observations and their timestamps.
The default refresh collects inventory without permissions.
Include `permissions` explicitly in `--scope` for a bulk permission inventory.
Treat the catalog as a cache, not authoritative truth for consequential remote changes.
Refresh preserves independently cached schema, Pulse, and unrequested inventory observations without changing their timestamps or freshness.
An item-level permission denial can produce a usable `partial` catalog with explicit warnings and `complete: false`.
Inspect `tadx catalog status --full` before treating cached permission coverage as complete.
Older catalog schemas require an explicit refresh; ordinary reads do not rebuild the cache.
Catalog identity is bound to the actual server endpoint and site, not its editable environment alias; legacy unbound caches require refresh.

Catalog collection starts with up to four concurrent reads and increases gradually toward the environment's ceiling, default 32.
Throttled reads share a cooldown within the run.
Set the ceiling from 1 to 256 with `tadx env update <alias> --catalog-max-concurrency <count>`; `--clear-catalog-max-concurrency` restores 32.
The same setting is available on `env add` and is stored as `catalog_max_concurrency`.

## Workspaces

A TADX workspace is a named, registered local directory for managed Tableau artifacts and metadata.
Pull commands write there, and publish commands read from there.
Content publish also accepts an existing native file with `--file`, without requiring a workspace.
`--workspace` accepts the registered workspace name, not a filesystem path.
TADX does not silently create workspaces.
Returned artifact paths remain relative to the workspace so they are portable across machines.
Managed content can be selected for publish by `--id` or exact `--artifact-name`; ambiguous matches fail.
Compact output shows identity and local state; use workspace status with `--full` for file locations and fingerprints.

## Read the relevant reference before acting

| Intent | Read first |
|---|---|
| Search, inspect, pull, publish, move, rename, delete, lineage, or datasource schema | [Content lifecycle](references/content-lifecycle.md) |
| Users, groups, memberships, ownership, permissions, or projects | [Administration](references/administration.md) |
| Workspace creation, registration, defaults, local artifact movement, root relocation, or cleanup | [Workspaces](references/workspace.md) |
| Pulse definition creation, forking, validation, or management | Separate `tadx-pulse` Guidance |

Read the relevant reference before acting.
Do not act from this root summary alone when a reference owns the task.
Use leaf command help only as a fallback after reading that reference.
