---
name: tadx-build
description: Implement or extend TADX actions and Tableau adapters using repository architecture, output contracts, safety rules, integration points, and tests. Use when explicitly asked to build with tadx-build, not to operate Tableau through the installed CLI.
---

# Build a TADX action

This package is the maintained development standard for TADX contributors.
It has no dependency on a developer's installed skills and requires no OpenSpec, Superpowers, or review plugin.
Apply it only to the explicitly assigned build; do not infer additional capabilities, external mutations, or product decisions.
The installed `tadx` and `tadx-pulse` skills teach CLI operation, not application development.

## Orchestrate the build

Work from the repository root and read `AGENTS.md` for checkout-specific instructions.
Start from updated `main` on one new feature branch, preserving existing changes; do not switch away from an assigned in-progress branch.
Use one shared branch and checkout for all agents unless the user requests worktrees.
The coordinator assigns disjoint paths, owns shared integration files, and prevents agents from editing the same files.
Slice agents run their focused checks and return integration requirements to the coordinator.
Define the capability ID and command, user outcome, inputs, exact selectors, compact/full output, preview behavior, bounds, exclusions, and acceptance tests.
Keep the brief proportional to the task; no mandatory planning artifacts or framework commands.

## Load bounded context

Use the checkout's registry, not an older installed binary: `go run ./cmd/tadx capability get workbook.inspect --full` is one example.
For a new capability, inspect the closest existing capability rather than treating its absent registry entry as a blocker.
Read one comparable action and its wiring; use [references/implementation-map.md](references/implementation-map.md) to locate the appropriate layer.
For remote behavior, read one relevant record under `docs/evidence/`.
Local API captures are optional, ignored, and absent from fresh clones.
Use the official documentation linked in [the evidence index](../../../docs/evidence/README.md) when the relevant local capture is unavailable or insufficient.
Search only for the required operation, type, or field; never load whole API manuals or archived plans.
If evidence is docs-only or blocked, stop at the adapter seam and do not make the capability executable.
Record bounded behavioral evidence before promoting an evidence level; prose claims alone are insufficient.

## Build with tests first

Write the externally visible behavior tests before implementation and confirm they fail for the expected reason.
Keep workbook, datasource, and flow operations in their cohesive `actions/<resource>` packages as they migrate, with explicitly named inputs, outputs, and operations.
Admin group membership and permission mutations also share cohesive packages; other operations retain their assessed boundaries.
For resource consolidation, follow the [resource refactoring playbook](../../../docs/resource-refactoring-playbook.md); preserve differing workflow sequences and remove unnecessary mappings rather than moving them unchanged.
Define narrow dependency interfaces in the action package that consumes them.
Actions never import Cobra, `net/http`, another action, or a concrete resource adapter.
Cobra parses arguments, invokes actions, and renders through the shared output layer; the application maps structured errors to exit codes.
Put released Tableau HTTP behavior in `internal/tableau/<resource>` through the shared transport.
Put provider adaptation and exact identity resolution in `internal/resources/<resource>`; resource adapters do not import actions, Cobra, or `net/http`.
Use typed bounded provider pages; actions own requested result windows and traversal policy through shared paging helpers.
Bridge concrete adapters to action-owned interfaces only in the composition root.
Share genuinely identical identity, schema, and lineage records through dependency-free `internal/value`, not a universal resource object or generic CRUD service.
Reuse shared transport, authentication, errors, output, and identity machinery instead of parallel implementations.
Resolve configuration, workspace, and authenticated clients once per command, including batches.
Reuse sessions only for the same server, site, and actual credential identity; retain the local credential lock until session use ends.
Preserve sequential batch order and per-item failures.
Validate local inputs, bounds, selectors, and artifact prerequisites before authentication, then validate remote state.
Reuse a project index within one validation phase, but obtain fresh evidence for a separate pre-write phase.
Do not introduce persisted sessions, generic retries, or implicit remote-to-cache fallback as shortcuts.

## Preserve output and safety contracts

Default output is an explicit bounded compact TOON projection: status, exact identities, decision-relevant fields, actionable warnings, and completeness.
Use `details: "--full"` immediately before `help[]` when the same command has additional bounded detail.
`--full` changes presentation only; it never changes authentication, requests, pagination, mutations, or redaction.
Use separate compact/full golden fixtures and retain fixed scalar list columns, including empty values, at every limit.
Report `more_available` and incomplete coverage honestly; keep opaque upstream continuation tokens internal while preserving documented user-facing cursor selectors.
Previews include the exact target and every consequential setting, including inherited settings and null handling; provider envelopes belong in full output.
Preserve confirmed partial results and exact created identities through action, batch, error, and renderer boundaries when verification fails.
An unknown write outcome is not success and must not encourage an unsafe retry.
Use shared structured errors with operation/target context, upstream evidence, preserved retryability, and relevant corrective action.
Generated follow-up commands retain the resolved environment, exact identity, and workspace where applicable, using shared shell-safe hints.
Persist and render artifact paths relative to the resolved workspace with forward slashes.
Preserve staged recoverable writes and dirty-artifact protection.

Treat Tableau LUIDs as authoritative, fail ambiguous selectors, and never fuzzy-match or prompt interactively.
Reject incomplete identities and do not assume project display or leaf names are unique.
Project names can contain literal slashes; retain their content and keep exact-LUID operations usable when a path is ambiguous.
Infer a remote-write environment only when exactly one is configured; with multiple environments require it explicitly.
Artifact source metadata never chooses the publish destination.
Authenticate to Tableau with PATs only.
Remote mutation commands remain discoverable while execution is disabled.
Supported `--preview=true` performs no consequential remote writes while site consent or managed remote mutations are disabled; explicit `--preview=false` cannot bypass either gate.
Enabled mutations execute by default when the selected site's saved consent and the managed policy allow them.
Use `tadx mutation status` for all configured environments, or add `--environment <alias>` for one alias.
Use `--full` for canonical server and exact site details, and `tadx mutation set --environment <alias> --enabled=<true|false>` for canonical server and exact site consent.
Changing site consent requires explicit permission covering the selected site and persisted scope, separate from permission for the operation itself.
Legacy global settings and `TADX_ENABLE_MUTATIONS=0` or `1` do not authorize remote writes.
Never change a developer's operational mutation setting to make a test pass.
`--force` does not bypass mutation policy, and mutation discovery never grants authorization.
Persist PATs only after explicit user approval through the native OS credential store.
Store only opaque credential references in configuration, and never use a plaintext credential fallback.
Never place PATs or session tokens in configuration values, output, logs, artifacts, caches, fixtures, or diagnostics.
Resolve machine-local roots only at runtime; expose them only where the output contract provides local locations, such as full workspace status.
Never hardcode developer paths, usernames, private site names, or unrelated local project names in tracked files or fixtures.
Keep capabilities outside TADX distinct from executable commands; do not teach or proxy external MCP tools.

For inventory, search, cache, or cache work, read [references/inventory-and-cache.md](references/inventory-and-cache.md).
Do not load that reference for an unrelated action.

## Integrate without generated drift

The coordinator owns typed definitions and implementation metadata in `internal/capability/definitions.go`, shared command mounting, and app composition.
Do not resurrect Markdown compilation, a separate implementation manifest, or `registry_gen.go` as a source of truth.
Preserve canonical capability annotations and actual command-tree binding tests.
Reuse canonical selectors/flags and add collision-free shorthand in `internal/cli/shorthand.go` for new names longer than three characters, or document a justified exception.
When changing arguments, test canonical and alias state, repeated values, and explicit Boolean false.
Follow [the command structure standard](../../../docs/command-structure.md) for root/category navigation and complete resource help; verb help mirrors its owning reference.
Keep syntax, flags, constraints, defaults, and examples in help, not duplicated in installed Guidance.
The root skill teaches efficient discovery; optional references explain Tableau concepts and task-specific judgment, not command manuals or workarounds for CLI defects.
For every new or materially changed command, use [the command integration checklist](references/command-integration.md) to update all affected registrations, help, documentation, Guidance, generated capability outputs, website packaging, policy coverage, and acceptance checks.
Never hand-edit generated files or `CHANGELOG.md`; regenerate the capability map's data block while preserving its authored layout and styling.

```text
go generate ./...
```

The generator directive updates `docs/reference/capabilities.md`, `docs/reference/capabilities.json`, and the data block in `docs/reference/capability-map.html`.

## Verify and hand off

For bugs, reproduce the user command through app/HTTP fixtures or a subprocess, not just a helper unit test.
Run focused action, adapter, contract, CLI, and architecture tests while building.
Cover applicable invalid inputs, ambiguous identities, paging bounds, partial failures, preview/no-write behavior, and compact/full output.
Evidence claims must match the tested behavior, including multipart upload blocks and async terminal outcomes where implemented.
Run `gofmt` on changed Go files and verify regeneration leaves no unexplained diff.

```text
git diff --check
go vet ./...
go test ./...
go test -race ./...
go test ./internal/architecture -count=1
```
Run live tests only when the user authorizes the exact disposable target and usable credentials are present.
Keep live tests outside the standard suite, use run-scoped disposable resources, and clean up only what the run created.
Attempt cleanup of disposable content and report uncertain outcomes without unsafe retries.

Report changed files, focused checks, integration changes, live-test status, and unresolved evidence or contract conflicts.
Stop for the build owner's manual testing and review.
Do not start agent reviews, no-mistakes, pushes, PRs, merges, or releases without authorization.
When a push is authorized, include repo, branch, base/head commits, focused review scope, and verification limits in a concise review prompt.
