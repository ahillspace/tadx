---
name: tadx
description: Operate Tableau content, catalogs, workspaces, local artifacts, and administration with TADX Guidance. Use tadx-pulse for Pulse authoring and Tableau MCP for analytical work.
---

# Operate Tableau with TADX

TADX owns content lifecycle, artifacts, workspaces, administration, and Pulse definitions.
Tableau MCP owns datasource queries, view data and images, custom-view data and images, and Pulse values and insights.
TADX never configures, selects, calls, proxies, or reports the connection state of Tableau MCP.
The host agent and user own the active Tableau MCP connection.

## Run with bounded discovery

Reuse the task's known environment, workspace, and LUIDs.
Use a provided environment alias verbatim without revalidating it through `env get`, `env list`, or a preliminary auth check.
When no alias is provided or known, use `env list`; diagnose configuration/authentication only after an actual command fails.
Avoid chaining environment list/get, auth status/check, and doctor for an already working target.
PATs remain environment-variable references; never print or persist PATs or session tokens.

Batch independent commands into one shell tool call as separate sequential lines.
Never run authenticated TADX calls concurrently with the same PAT, including across agents or background jobs.
Wait for each process to finish; gate dependent commands on successful prior results.
Quote names, field IDs, and paths containing spaces; quote a spaced executable path using the shell's invocation syntax.

Use these recipes directly, replacing bracketed placeholders with known values.
Use compact TOON and returned `help[]`; request `--full` only for a specific missing detail.
Avoid root/category help, broad capability dumps, repeated probes, and delegation for bounded CLI reads.
If installed guidance is insufficient, use at most one relevant leaf `--help` probe per workflow; unresolved gaps require a concrete limitation report.
For availability uncertainty, one exact `tadx capability get <id>` can replace that help probe.

## Choose the owning tool

Use TADX for content identity, lifecycle, local artifacts, workspaces, administration, configuration, and Pulse definition or metric lifecycle.
Use Tableau MCP directly for analytical results:

- Use `list-views` and `get-view` for view discovery and metadata.
- Use `get-view-data` or `get-view-image` for published-view results.
- Use `list-custom-views`, `get-custom-view-data`, or `get-custom-view-image` for saved view states.
- Use `get-datasource-metadata` before `query-datasource` for business questions against published data.
- Use the Pulse insight tools described by the `tadx-pulse` Guidance for current values and explanations.

Do not use `tadx doctor` or capability discovery to test whether Tableau MCP is connected.
Read [Tableau MCP routing](references/tableau-mcp.md) only when an analytical request needs a more specific tool choice.

## Content lifecycle

Use native live search when the name is unknown: `tadx search "<term>" --type workbook --environment <alias>`.
Use an explicitly filtered list for a bounded exact candidate query, and use returned LUIDs for subsequent operations.
Run an unfiltered resource list only when the task needs complete live inventory; it refreshes that catalog scope before rendering a bounded page.
Ambiguity fails; never select the first fuzzy match.
For the Tableau-managed imported project, use `Imported`; TADX normalizes that selector to Tableau's `(imported)` project path.

```text
tadx content workbook list --environment <alias> --name "<name>" --limit 5
tadx content workbook inspect --environment <alias> --id <workbook-luid>
tadx content workbook pull --environment <alias> --id <workbook-luid> --workspace <workspace> --include-extract=false
tadx content workbook publish --environment <destination-alias> --workspace <workspace> --artifact "artifacts/workbook/<directory>" --project-id <project-luid> --preview
tadx content workbook move --environment <alias> --id <workbook-luid> --destination-project-id <project-luid> --preview
tadx content workbook update --environment <alias> --id <workbook-luid> --new-name "<name>" --preview
tadx content workbook delete --environment <alias> --id <workbook-luid> --preview
```

For multiple exact resources of one type, repeat `--id` on pull or `--artifact` on publish up to 100 times.
Batches run sequentially, apply shared flags to every item, continue independent failures, and return one aggregate result.
Use one publish command per destination project, and publish datasource dependencies before workbooks.

Keep extracts when required; `--include-pds` acquires direct published datasource dependencies without recursion.
Publish selects the managed artifact directory, not its payload file.
Use the returned artifact path and explicit destination environment/project.
Pull `--overwrite` discards dirty local edits; publish `--overwrite` replaces a remote collision.
Use either only when that replacement is authorized.
For datasource publish modes or uncertain jobs, read [content details](references/content-lifecycle.md).

## Catalog, flow, and lineage

Live reads are the default; supported `--catalog` reads stay local without refresh or live fallback.
Live content terms use Tableau native search; administration and Pulse searches use their dedicated APIs.
An unfiltered workbook, datasource, flow, project, user, or group list collects the complete live scope and atomically replaces only that catalog scope.
For those complete lists, `--limit` bounds rendered rows only, and continuation reads the same local snapshot without repeating the live traversal.
Adding an exact resource filter keeps the list bounded against Tableau and records a partial cache update; it does not prove complete scope coverage.
Reuse a sufficiently fresh catalog for repeated discovery; a miss or partial scope does not prove remote absence.
Follow cursors only when the task needs more rendered results.

```text
tadx catalog status --environment <alias>
tadx catalog refresh --environment <alias> --scope projects --scope workbooks --scope flows
tadx content flow list --environment <alias> --name "<name>" --limit 5 --catalog
tadx content flow pull --environment <alias> --id <flow-luid> --workspace <workspace>
tadx content flow move --environment <alias> --id <flow-luid> --destination-project-id <project-luid> --preview
tadx content lineage pull --environment <alias> --kind workbook --id <workbook-luid> --workspace <workspace> --direction upstream --depth 1
```

A full catalog refresh replaces the environment/site's current generation and cached entries; it does not merge earlier scopes.
Request all required scopes together; dependency collection does not establish complete inventory for unrequested scopes.
Successful live reads cache targeted observations without proving full site coverage.
Prefer a targeted live inspect after mutation over a full refresh.
Lineage is bounded evidence; missing edges do not prove independence.

## Administration and projects

```text
tadx admin user list --environment <alias> --name "<username>" --limit 5
tadx admin group list --environment <alias> --name "<group-name>" --limit 5
tadx admin group inspect --environment <alias> --id <group-luid> --members
tadx admin group member add --environment <alias> --group-id <group-luid> --user-id <user-luid> --preview
tadx admin group member remove --environment <alias> --group-id <group-luid> --user-id <user-luid> --preview
tadx admin permission inspect --environment <alias> --kind workbook --id <workbook-luid> --principal-id <principal-luid> --full
tadx admin permission create --environment <alias> --kind workbook --id <workbook-luid> --principal-type group --principal-id <group-luid> --capability Read --mode Allow --preview
tadx admin permission delete --environment <alias> --kind workbook --id <workbook-luid> --principal-type group --principal-id <group-luid> --capability Read --mode Allow --preview
tadx content project create --environment <alias> --name "<project-name>" --parent-id <parent-project-luid> --preview
tadx content project update --environment <alias> --project-id <project-luid> --description "<description>" --preview
tadx content project move --environment <alias> --project-id <project-luid> --parent-id <parent-project-luid> --preview
tadx content project delete --environment <alias> --project-id <project-luid> --preview
```

Choose the requested capability and exact `Allow`/`Deny` mode; there is no atomic permission update.
Omit project create's `--parent-id` for a top-level project.
Project deletion selects `--project-id`; its preview identifies the project without enumerating descendant deletion effects.
Read [administration details](references/administration.md) only for permission semantics, membership replacement, or project deletion.

## Local workspaces and safe changes

```text
tadx workspace status --workspace <workspace>
tadx workspace create <workspace>
tadx workspace register <workspace> --path "<existing root>"
tadx workspace set-default <workspace>
tadx workspace artifact move --source <workspace> --destination <workspace> --artifact "artifacts/<kind>/<directory>"
```

Run only the needed local command; workspace commands take no `--environment` and make no Tableau changes.
`--workspace` is a logical registered name; pull/publish never create workspaces implicitly.
Artifact paths remain workspace-relative with forward slashes.
`workspace artifact move` transfers one managed artifact and never relocates a registered workspace root.
Read [workspace details](references/workspace.md) only for defaults, cloning, artifact movement, root relocation, or cleanup.

Remote mutations require `TADX_ENABLE_MUTATIONS=1` and run by default; `--preview` plans without applying, and there is no `--apply`.
Use previews when review or uncertainty warrants them; existing authorization does not require repeated approval.
Discovery/previews do not authorize changes, and `--force` never bypasses policy.
Inspect uncertain remote outcomes before retrying; report unresolved failures without repeated writes.
Use the separate `tadx-pulse` Guidance for Pulse work.

## Local Guidance and CLI utilities

Use `tadx agent uninstall --target codex --preview` before removing installed Guidance for Codex.
Replace `codex` with `claude` or `cursor` for the other supported targets.
Run without `--preview` to remove matching packages; divergent packages require `--force` and remain in recoverable backups.
Use `tadx version` offline, or use `tadx version --check` for one bounded GitHub release check.
Use `tadx completion <shell>` to print completion for `bash`, `zsh`, `fish`, or `powershell`.
