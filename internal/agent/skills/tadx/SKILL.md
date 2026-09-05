---
name: tadx
description: Operate Tableau content, catalogs, workspaces, local artifacts, and administration with the TADX CLI. Use tadx-pulse for Pulse definition authoring; use Tableau MCP for data queries and metric insights.
---

# Operate Tableau with TADX

TADX owns Tableau content lifecycle, local artifacts, workspaces, administration, and Pulse definitions.
Tableau MCP owns data queries, view data and images, and Pulse metric values and insights.
TADX never calls or proxies MCP.

## Establish the target

Use `tadx env list` and `tadx env get <alias>` to identify the configured environment.
Pass its exact alias with `--environment` when target selection matters.
Use `tadx auth status --environment <alias>` to inspect PAT references without revealing values.
Use `auth check` to verify sign-in and the selected site, or `tadx doctor` for broader diagnostics.
Profiles store environment-variable references, never PAT values; never print or persist PATs or session tokens.

Use `tadx workspace list` and `tadx workspace status --workspace <name>` before artifact work.
`--workspace` takes a registered logical name, not a directory.
Pull and publish do not create workspaces implicitly.
Artifact selectors use workspace-relative managed paths with forward slashes.
For creation, selection defaults, and local cleanup, read [workspace and artifact guidance](references/workspace.md).

## Choose live or catalog reads

Reads contact Tableau by default.
On supported commands, `--catalog` reads local inventory without contacting Tableau; it does not refresh or silently fall back to live data.
Use live reads for current state and final identity checks before mutations.
Use catalog reads for repeated discovery when their age and coverage suit the task.
Catalog absence does not establish remote absence.

Check `tadx catalog status --environment <alias>` for generation age, completeness, source, and staleness.
Successful supported live reads also cache observed resources, but those observations do not establish complete site coverage.
For a few known resources, live `inspect`, bounded `list`, or datasource `schema` reads provide targeted refreshes without hydrating the site.

For broader inventory, request only needed scopes:

```text
tadx catalog refresh --environment <alias> --scope workbooks
tadx catalog refresh --environment <alias> --scope users --scope groups
```

Workbook, datasource, and flow scopes also collect projects; views and permissions also collect workbooks and projects.
Implicit dependencies do not become requested inventory scopes; request projects explicitly if you need project inventory.
A refresh replaces the current generation and its cached resource entries for that environment and site, rather than merging previous scopes.
Request all scopes needed together; omit `--scope` only when full inventory is useful.
After remote changes, refresh affected observations or scopes before relying on catalog data again.

## Resolve exact identity

Use `tadx search <term> --type <resource> --environment <alias>` to discover candidates; add `--catalog` for local discovery.
Search terms match name substrings without case sensitivity; they are not exact selectors.
Narrow the type to avoid scanning unrelated resources.
Use returned Tableau LUIDs for `--id`; otherwise use exact names and supported slash-delimited project selectors.
Never fuzzy-match or choose the first ambiguous result.
Follow returned cursors with the same source and filters when further pages are needed.

## Perform the requested operation

Remote mutations remain discoverable when disabled; `TADX_ENABLE_MUTATIONS=1` enables them.
When enabled, mutation commands perform changes by default.
Use `--preview` for a read-only plan; there is no separate `--apply` step.
Discovery and previews do not authorize changes; act within the user's existing authorization.
`--force` does not bypass mutation policy.
Before retrying an uncertain mutation, inspect the remote outcome and follow the reported retry guidance.

For specific work, read only the applicable guidance:

- [Content lifecycle](references/content-lifecycle.md): inspect, pull, publish, delete, and bounded lineage.
- [Administration](references/administration.md): users, groups, and permission rules.
- Use the separate `tadx-pulse` skill for Pulse authoring and definition lifecycle.
  Resolve datasource LUIDs and fields before authoring; route metric values and insights to Tableau MCP.

## Discover details as needed

Use `tadx <category> --help`, then the operation's `--help` for supported selectors and flags.
For availability or ownership, use `tadx capability list --resource <resource>` and `tadx capability get <id> --full`.
Do not infer executability from discovery; unavailable or blocked capabilities remain non-executable.
Use compact TOON by default and follow `help[]` for the next command.
Use `--full` when expanded bounded detail is needed; it changes presentation, not requests, pagination, or mutation behavior.
