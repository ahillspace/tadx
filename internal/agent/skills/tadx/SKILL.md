---
name: tadx
description: "Operate Tableau with TADX: discover and manage content, inspect datasource schemas and upstream metadata, administer sites, and work with local artifacts and Pulse definitions."
---

# TADX

TADX manages Tableau content, metadata, administration, local artifacts, caches, and Pulse definitions.
It does not query data values, render views, or retrieve Pulse insights.

## Learn once, then operate

Before Tableau or workspace work, run `tadx` once per session for local environments, credential configuration, workspace selection, and mutation policy; reuse that context until it changes.
Skip that overview for help, capabilities, installed version, or local Guidance installation/removal.
Go straight to known action help, for example `tadx content datasource inspect -h`; use `tadx -h` only when the route is unknown.
Category help navigates resources; resource help contains all its verbs, syntax, defaults, constraints, and batch options.
Categories with direct actions provide their reference there instead.
Verb `-h` is a sufficient focused subset of the same resource definitions and adds nothing after the complete reference.
Reuse help already loaded; capability discovery is for availability diagnostics, not another required step.

Search concepts, list inventory, inspect metadata, and pull files.
Reuse returned identities and verified results.
Default output is compact TOON; use `--json` for parsing or requested files and capture that native output directly.
`--full` adds bounded detail, not records, evidence, or API enrichment; supported `--all` collects the bounded complete inventory.
Use project IDs for workbook, datasource, and flow lists or exact-name inspection.
Use the returned immediate `tadx last --full` command to expand a saved result without rerunning the operation.
The last-result envelope is one replaceable record; later commands, including recorded usage failures, can replace it.
If saving failed, `last` may be stale; retain the current response's identities and do not replay a mutation to repair saving.
Use supported batches for already-decided repeated work; preserve per-item successes when another item fails.

Live reads are default; `--cache` is local-only and a miss does not prove remote absence.
Use cached observations when freshness is acceptable, live evidence before consequential writes.
Workspaces are named local directories, not Tableau sites; moving files never changes their remote identity or publish destination.
Resolve only the prerequisites the task needs: remote operations need their environment, managed artifact operations need a registered workspace, and native-file publication needs no workspace.
Capabilities describe local implementation, not site access or licensing; a Pulse filter needs no authoring workflow.

## Authority and recovery

TADX uses PATs; never expose credentials or session tokens.
Commands sharing a PAT are coordinated locally, not across machines or external tools.
Use exact returned LUIDs after discovery; do not guess through ambiguity.
Remote writes require selected-site consent plus applicable policy checks.
Ask before changing site consent; name the server, exact site, and persisted scope.
An operation request is not authorization.
Legacy global settings and `TADX_ENABLE_MUTATIONS` values do not authorize writes.
`--preview` is read-only and does not authorize execution.
`--force` never bypasses policy.
Retain confirmed identities and partial results; inspect unknown write outcomes before repeating a mutation.
TADX confirms writes automatically where their response is insufficient; do not add routine verification calls after a confirmed result.
Indexing delays do not undo acknowledged writes.
Preserve environment, configuration, selectors, workspace, filters, encoding, and cache mode in follow-up commands.

## Concepts when needed

Read only the relevant reference when the task needs these nuances, not before every command.
Syntax belongs in help, not these references.

| Question | Reference |
| --- | --- |
| Embedded versus published dependencies, portability, lineage limits | [Content lifecycle](references/content-lifecycle.md) |
| Upstream versus published-field descriptions, metadata identities, labels | [Catalog metadata](references/catalog.md) |
| Site roles, membership replacement, project locks, effective permissions | [Administration](references/administration.md) |
| Local identity, dirty artifacts, registration, moving or deleting files | [Workspaces](references/workspace.md) |
| Cache freshness, coverage, refresh effects, server load | [Cache](references/cache.md) |
| Managed policy | [Managed policy](references/managed-policy.md) |
| Partial batch results and dependent scripted steps | [Batching](references/batching.md) |
| Meaningful Pulse authoring, variants, and subscriptions | Separate `tadx-pulse` skill |

Respect the user's tool choice outside TADX; this skill does not configure or instruct Tableau MCP.
