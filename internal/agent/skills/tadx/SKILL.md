---
name: tadx
description: Use for TADX CLI workflows involving Tableau content discovery and lifecycle, datasource schemas, upstream catalog metadata, administration, caches, workspaces, local artifacts, and Pulse definitions.
---

# Operate Tableau with TADX

Start each TADX session by running `tadx` once.
It reports local environment, credential configuration, workspace selection, and mutation policy without signing in or changing state.
Use that context instead of rediscovering setup; credential configuration is not proof of successful authentication.

## What TADX is

TADX is the deterministic Tableau lifecycle and development CLI for agents and humans.
It supports content discovery and lifecycle, metadata and lineage, caches, local artifacts, workspaces, administration, and Pulse definition lifecycle.
This Guidance teaches TADX commands; respect the user's chosen tools for other workflows.
TADX does not query datasource values, render views, or retrieve current Pulse values and insights.

Default output is compact TOON.
Use `--json` for scripts; `--full` controls detail independently of encoding.
Reuse returned IDs and confirmed result fields; batch already-decided steps instead of repeating discovery or inspection.
Use `--full` only when expanded bounded details are needed.
`tadx last` displays the previous execution's saved full result and timestamp without repeating it; one global result is retained, not history.
Help places aliases beside canonical names, such as `content (con)` and `--environment (--env, -e)`.
Aliases apply only where the canonical command or flag is accepted.
Use `-f` only for `--full`; `--force` remains distinct and never bypasses mutation policy.
Canonical command names, structured output fields, registry IDs, and LUIDs remain preferred in durable instructions and scripts.

## Command discovery

Use `tadx --help` for the command roadmap.
`tadx content --help` lists workbook, datasource, flow, and project actions; a resource reference such as `tadx content workbook --help` includes all its verbs, required inputs, options, concrete values, defaults, omission behavior, constraints, and supported batch syntax.
Every content verb shows its owning resource reference; reuse it without repeated drilldown.
`catalog lineage` captures lineage, and `catalog label` manages labels attached to assets.
Other categories currently include descendant actions in category help: `tadx admin group --help` includes memberships, while `tadx admin --help` and `tadx pulse --help` cover their entire categories.
Both `-h` and `--help` are help-only paths and never run actions or read credentials.
Use `tadx capability list` and `tadx capability get` for feature inventory and availability diagnostics, not as a prerequisite to command syntax discovery.

## Critical rules

- Reuse known environment aliases, workspace names, LUIDs, and successful readback evidence; retrieve only missing or freshness-sensitive details.
- Use exact LUIDs after discovery.
- Never select the first fuzzy match or resolve ambiguity interactively.
- TADX authenticates with PATs only.
- Never expose PATs or session tokens.
- TADX serializes commands sharing a PAT on this machine; other machines and external tools need separate PATs or coordination because a new session can invalidate an existing one.
- Remote mutations run by default when enabled by the effective mutation policy; supported read-only `--preview` operations remain available when the gate is off.
- Pull and local-state previews inspect the planned scope without writing artifacts or configuration; retain their stated verification limits.
- Discovery and previews do not authorize mutation, and `--force` never bypasses mutation policy.
- Errors can include confirmed results in `output`, a failure `phase`, an `outcome`, and a missing `prerequisite`.
- Preserve completed work, resolve the missing prerequisite, and retry only unfinished operations; inspect unknown write outcomes before repeating a mutation.
- Follow-up commands preserve configuration context, but their presence does not authorize execution or establish that they fit the user's destination.
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
`mutation set` and interactive `auth login` have no preview mode; release checks use `tadx update --check`.
An explicitly configured `TADX_ENABLE_MUTATIONS=0` or `1` overrides the saved policy for that process; without either setting, execution is disabled.

## Environment selection

With one configured environment, TADX uses it when a selector is omitted.
With multiple environments, reads can use the configured default, but remote writes require `--env`.
Adding a second environment announces this transition; artifact provenance and workspace placement never choose a publish destination.

## Cache versus live reads

`cache` is TADX's local inventory and observation store; `catalog` inspects and enriches upstream Tableau metadata.
Live reads are the default.
Live content searches use Tableau's native search and inherit the search capabilities and ranking available on that site.
`--cache` searches cached metadata locally using lexical matching only.
Prefer live search for broad or conceptual discovery, and cache search for fast, repeated known-term lookup.
Use the cache when freshness is acceptable and the task benefits from repeated discovery, broad inventory, cross-resource comparison, or cached datasource schemas.
Refresh the required cache scopes when their cached coverage or freshness does not meet the task.
Use live reads for authoritative state before consequential changes, details not indexed in the cache, targeted inspection after remote changes, and uncached datasource schemas.

`--cache` is local-only and never falls back to Tableau.
A cache miss does not prove remote absence.
Use `coverage_reason` when present; partial coverage alone does not establish an access denial or missing remote content.
Live schema reads write through to the cache.
Targeted live reads do not establish complete inventory coverage.
Ordinary content and administration lists and live searches use bounded provider reads without accessing SQLite.
Content and user/group inventory `--all` explicitly collects all selected records within 10,000 and attempts a cache update.
An unfiltered complete `--all` replaces that resource scope; filtered `--all` records observations without claiming complete site coverage.
A failed cache write does not discard the collected live answer; retain its warning.
A refresh atomically replaces the requested inventory scopes while preserving independently cached observations and their timestamps.
The default refresh collects inventory without permissions.
Include `permissions` explicitly in `--scope` for a bulk permission inventory.
Treat the cache as a cache, not authoritative truth for consequential remote changes.
Refresh preserves independently cached schema, Pulse, and unrequested inventory observations without changing their timestamps or freshness.
An item-level permission denial can produce a usable `partial` cache with explicit warnings and `complete: false`.
Inspect `tadx cache status --full` before treating cached permission coverage as complete.
Older cache schemas require an explicit refresh; ordinary reads do not rebuild the cache.
Cache identity is bound to the actual server endpoint and site, not its editable environment alias; legacy unbound caches require refresh.

Cache collection starts with up to four concurrent reads and increases gradually toward the environment's ceiling, default 32.
Throttled reads share a cooldown within the run.
Set the ceiling from 1 to 256 with `tadx env update <alias> --cache-max-concurrency <count>`; `--clear-cache-max-concurrency` restores 32.
The same setting is available on `env add` and is stored as `cache_max_concurrency`.

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
| Upstream databases, tables, columns, descriptions, tags, metadata audits, or labels | [Catalog metadata](references/catalog.md) |
| Users, groups, memberships, ownership, permissions, or projects | [Administration](references/administration.md) |
| Workspace creation, registration, defaults, local artifact movement, root relocation, or cleanup | [Workspaces](references/workspace.md) |
| Repeat an action, supply different settings per item, or capture results in a script | [Batching and scripts](references/batching.md) |
| Pulse definition creation, forking, validation, or management | Separate `tadx-pulse` Guidance |

Read the relevant reference before acting.
Do not act from this root summary alone when a reference owns the task.
References supply task-specific judgment and contracts; category help supplies command syntax, flags, and examples.
