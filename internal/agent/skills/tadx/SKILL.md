---
name: tadx
description: "Operate Tableau with TADX: discover and manage content, inspect datasource schemas and upstream metadata, administer sites, and work with local artifacts and Pulse definitions."
---

# TADX

TADX is a deterministic Tableau lifecycle and development CLI for humans and agents.
It manages content, metadata, administration, local workspaces and caches, and Pulse definitions; it does not query data values, render views, or retrieve Pulse insights.

## Learn once, then operate

Run `tadx` once per session for local environments, credential configuration, workspace selection, and mutation policy; this does not authenticate or change state.
Use `tadx -h` for the roadmap, then go directly to the relevant resource help, for example `tadx content datasource -h`.
Category help navigates resources; resource help contains all its verbs, syntax, defaults, constraints, and batch options.
Categories with direct actions provide their reference there instead.
Verb `-h` mirrors the owning reference, so repeated drilldown adds context without new information.
Reuse help already loaded; capability discovery is for availability diagnostics, not another required step.

Use search for a business concept or approximate name, list for inventory, inspect for details without downloading, and pull for local files.
Reuse returned identities and verified results rather than rediscovering them.
Default output is compact TOON; `--json` is for scripts, `--full` adds detail, and `tadx last` retrieves the last saved full result without rerunning the operation.
Use supported batches for already-decided repeated work; preserve per-item successes when another item fails.

Live reads are default; `--cache` is local-only and a miss does not prove remote absence.
Use cached observations when freshness is acceptable, live evidence before consequential writes.
Workspaces are named local directories, not Tableau sites; moving files never changes their remote identity or publish destination.

## Authority and recovery

TADX uses PATs; never expose credentials or session tokens.
Commands sharing a PAT are coordinated locally, not across machines or external tools.
Use exact returned LUIDs after discovery; do not guess through ambiguity.
Remote writes execute when enabled; `--preview` is the no-change path and does not authorize execution.
Before changing `TADX_ENABLE_MUTATIONS` or the saved mutation setting by any mechanism, obtain explicit permission for that setting change and scope, including disabling or unsetting it.
A requested Tableau operation is not permission to change policy; session approval is not approval for future shells.
`--force` never bypasses policy.
Retain confirmed identities and partial results; inspect unknown write outcomes before repeating a mutation.

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
| Partial batch results and dependent scripted steps | [Batching](references/batching.md) |
| Meaningful Pulse authoring, variants, and subscriptions | Separate `tadx-pulse` skill |

Respect the user's tool choice outside TADX; this skill does not configure or instruct Tableau MCP.
