# Complete V1 lifecycle and Guidance

## Decision

This build closes the remaining standard lifecycle symmetry gaps and expands explicit routing to Tableau MCP.
The collective term **Guidance** means installed skills, optional skill references, and CLI help.
TADX never configures, selects, probes, or reports Tableau MCP connections.
The user and host agent own MCP connection selection and availability.

## Executable TADX scope

Add these exact capabilities:

- `workbook.move` and `workbook.update` for exact project movement, rename, and supported owner replacement.
- `datasource.move` and `datasource.update` for exact project movement, rename, and supported owner replacement.
- `flow.update` for supported owner replacement while retaining the existing `flow.move` action.
- `project.move` for exact hierarchy reparenting.
- `workspace.set-default`, `workspace.unregister`, and `workspace.delete` for explicit local workspace lifecycle.
- `admin.group.member.add` and `admin.group.member.remove` for exact membership deltas without replacing the complete member set.
- `version.get`, surfaced as `tadx version`, with an optional bounded release check.
- `agent.uninstall` for removal of the installed TADX Guidance packages from one explicit target.
- Shell completion generation for Bash, Zsh, Fish, and PowerShell.

Remote mutations run by default only when the mutation gate is enabled and support `--preview`.
Selectors use authoritative LUIDs, fail on ambiguity, revalidate immediately before mutation, and preserve uncertain outcomes.
Update actions report exact changed fields and return an exit-zero no-op when authoritative state already matches.
Move actions remain within one Tableau site and never imply recursive migration or dependency rewriting.

Workspace unregister preserves files.
Workspace delete removes one exact registered workspace only after dirty and containment checks, supports preview, and never accepts a broad directory.
Changing the default workspace requires an existing available registration.

Group member changes add or remove one exact user from one exact group.
Repeated add or remove operations return an exit-zero no-op without replacing unrelated membership.

`tadx version` remains offline by default.
Its optional release check uses one bounded request, reports installed and latest versions, and never updates the binary.
Completion generation is local output and does not become a capability-registry action.

## Delegated Tableau MCP scope

Add non-executable registry entries for stable analytical intents that TADX intentionally delegates:

- View discovery and inspection through `list-views` and `get-view`.
- View data and image retrieval through `get-view-data` and `get-view-image`.
- Custom-view discovery, data, and images through `list-custom-views`, `get-custom-view-data`, and `get-custom-view-image`.
- Published datasource analytical metadata and queries through `get-datasource-metadata` and `query-datasource`.
- Pulse subscription inspection through `list-pulse-metric-subscriptions`.
- Pulse metric value and insight generation through `generate-pulse-metric-value-insight-bundle`.
- Pulse insight briefs through `generate-pulse-insight-brief`.

Each delegated entry reports `owner: tableau-mcp`, `execution_enabled: false`, the exact MCP tool hint, and the boundary between TADX and MCP.
Do not add fake executable commands or make TADX call MCP.
Do not add MCP connectivity checks to `doctor`, capability discovery, or any other command.
Do not advertise MCP mutation tools for lifecycle operations that TADX owns or has explicitly deferred.

## Guidance scope

Update Guidance so agents can route work without hunting:

- Define the TADX lifecycle boundary and the Tableau MCP analytical boundary in the root Guidance.
- Put Pulse authoring and Pulse analytical routing in the Pulse Guidance.
- Add direct recipes for every new executable action.
- Route view data, view images, datasource queries, Pulse values, Pulse insights, and Pulse briefs to the exact MCP tools.
- State that the host agent owns MCP connection selection and that TADX does not test connectivity.
- Preserve the trial findings: use supplied aliases verbatim, keep shared-PAT calls sequential, quote paths, avoid broad help, resolve exact identities, and trust clear terminal outcomes.
- Keep optional references bounded and load them only for mechanics not covered by the root Guidance.
- Use the term Guidance consistently in current build and operator documentation.

CLI category help must summarize delegated boundaries where an agent naturally looks, especially root, content, datasource, and Pulse help.
CLI help must not imply that a delegated intent is executable through TADX.

## Explicit exclusions

This build does not add Tableau MCP management, connection selection, connection status, or proxy calls.
It does not add Pulse definition or metric update commands.
It does not add datasource composition authoring, datasource field-description mutation, flow rename, schedules, flow runs, extract refreshes, generic bulk operations, or recursive project migration.
It does not edit, format, regenerate, or stage `docs/tadx-capability-map.html`.

## Verification

Write failing behavior tests before implementation.
Capture bounded Tableau REST evidence for each admitted remote mutation before making it executable.
Test compact and full TOON, preview, no-op, drift, ambiguity, policy denial, and uncertain outcomes.
Test Guidance links and every command recipe against the generated executable registry.
Test delegated entries as non-executable and verify that no Cobra command is mounted for them.
Run focused tests, architecture checks, generated-file checks, `go vet ./...`, and `go test ./...`.
Use only the authorized disposable `dev` Tableau Cloud site for bounded live verification.
Do not run a comprehensive branch review or no-mistakes pipeline automatically.
