---
title: Movement and logistics readiness proposal
status: accepted
created: 2026-08-31
decided: 2026-09-01
decision_scope: Pre-fanout foundations and vertical slices
authoritative: false
decision_owner: maintainer
---

# Movement and logistics readiness proposal

## Decision

TADX freezes the shared contracts and vertical slices in this record before movement, logistics, and remaining resource actions fan out.
This accepted proposal records the reviewed direction.
The authoritative product, capability, architecture, and build-order documents carry the executable contract.

## Locked constraints

- PAT authentication remains the only authentication mode.
- TOON remains the default output, and `--full` requests bounded additional detail.
- Consequential mutations preview by default and require `--apply`.
- Tableau LUIDs remain authoritative, and ambiguous selectors fail.
- Secrets never enter persisted artifacts or rendered output.
- Generic cross-resource remote movement and hierarchy migration remain outside V1.
- The admitted resource-specific `flow.move` action remains inside V1.
- Promotion remains an agent-composed pull and publish sequence.
- Blocked and docs-only capabilities cannot gain live Tableau behavior before their evidence gates close.

## Foundation decisions

### Named workspace identity

`--workspace` accepts a registered, case-insensitively unique logical name instead of an arbitrary filesystem path.
Global configuration maps each workspace name to a stable workspace ID and canonical path.
Workspace-local configuration stores the same name, stable ID, and a schema version.

All commands use one resolution order:

1. Use the explicit workspace name.
2. Use the registered workspace containing the current directory.
3. Use the selected environment's default workspace.
4. Use the global default workspace.
5. Return a deterministic error.

A separate recovery-only path option can exist if a later contract requires it.
It cannot create a second identity for the same workspace.
Configuration updates use one atomic read-modify-write store.

### Artifact identity and local logistics

Every artifact metadata document carries a versioned common identity envelope.
The envelope includes server origin, site LUID, resource kind, and Tableau LUID.
Resource-specific metadata stays in typed extensions.

One inspection contract defines payload discovery, baseline fingerprinting, dirty state, dependency references, and invalid metadata behavior.
Local `workspace.move` transfers an artifact between named workspaces with explicit dependency handling, collision behavior, rollback, and interruption recovery.
Local artifact deletion returns to V1 as a resource-specific action.
General cleanup workflows remain deferred.

Persisted artifact paths are slash-delimited and relative to the workspace.
First-party source, tests, documentation, and generated output cannot contain developer names, private project names, or machine-specific paths.

### Workspace concurrency

ADR-032 remains authoritative and V1 does not add workspace locks.
Existing hybrid `.tadx.lock` behavior must be removed instead of copied into new local actions.
Atomic replacement, optimistic validation, and recoverable operation boundaries protect local mutations.

### Publish targets and source provenance

Workbook, datasource, and flow mutations use one destination model.
Stable server origin and site LUID determine site equivalence.
Environment aliases do not establish resource identity.

When `--environment` is omitted, artifact source provenance supplies the environment, site, and project defaults.
When `--environment` selects a different environment, the caller supplies the exact target project.
An artifact without source provenance requires an explicit target environment and project.

For same-site republish, the recorded source LUID can identify the existing target.
For cross-site publish, the source LUID remains provenance only.
The target resolves by exact name and canonical project path, and ambiguity fails.
Every publish action uses the same collision, overwrite, and immediate pre-mutation revalidation rules.

### Cross-site published datasource behavior

Tableau rejects an unchanged workbook package when its published datasource binding is unavailable on the target site.
A sanitized live cross-site test confirmed that behavior with Tableau error code `400011`.

TADX does not add a second fail-closed dependency gate before Tableau receives the package.
Preview warns when source provenance differs from the explicit target.
Apply sends the native package unchanged and preserves Tableau's structured rejection, request ID, and corrective context.
TADX does not claim automatic dependency rebinding or dependency-aware promotion.

### Shared publish infrastructure

The second publisher reuses the proven upload-session and asynchronous-job behavior from workbook publish.
The shared layer owns upload sequencing, session identity validation, response bounds, terminal states, timeouts, and request and job identifiers.
Resource clients retain their multipart bodies and resource-specific REST contracts.
A shared project resolver remains resource-neutral.

### Output enforcement

Compact TOON remains the default, and `--full` remains the universal request for bounded detail.
Compact output includes `details: "--full"` only when it intentionally omits useful details.
Mutation previews retain every authorization-critical destination field in compact output.
An architecture or contract test rejects detail-bearing output types without explicit compact and full projections.

### Fanout integration

Slice agents own isolated action and resource packages.
An integration owner owns capability metadata, generated files, shared command mounting, and final binding validation for each fanout batch.
No slice agent regenerates or commits shared generated files unless assigned the integration role.
Agents share one feature branch for a coordinated build and do not create separate worktrees by default.

### Live development tests

The configured disposable development environment and its shell-referenced PAT are authorized for focused live tests.
The standard test suite remains hermetic.
Live tests remain tagged or opt-in, sanitize evidence, create uniquely named disposable content, and remove only content they created.

Agents search the captured local Tableau API documentation first by exact operation or schema symbol.
Agents do not load a complete captured reference into context.
When the local capture is missing, ambiguous, or version-sensitive, agents use current official Tableau documentation.

## Resource decisions

### Flow lifecycle

V1 includes flow list, get, pull, publish, delete, move between projects, and bounded lineage.
TADX preserves `.tfl` and `.tflx` packages unchanged.
TADX does not rewrite flow connections, credentials, or published datasource bindings.
Tableau remains authoritative for accepting or rejecting a flow package.

V1 excludes flow runs, orchestration, scheduling, run monitoring, cancellation, connection mutation, permission mutation, tags, quality-warning automation, keychains, and Recycle Bin recovery.
Ownership changes remain part of later administration work.

### Deletion

Resource-specific local artifact and supported remote content deletion return to V1.
This includes workbook, datasource, and flow deletion.
Project deletion remains deferred because it introduces cascade semantics.
General cleanup, retention, Recycle Bin recovery, and permanent purge remain deferred.

The mutation and deletion environment-flag policy remains tabled.
The current mutation discovery setting remains discovery-only and does not become authorization.

### Lineage

Bounded lineage returns to V1 for workbooks, datasources, and flows.
Automatic lineage capture rides with downloads, writes a relative graph sidecar, and records status and summary counts in artifact metadata.
Successful compact output omits the graph.
Partial or unavailable capture emits a warning and never presents incomplete lineage as complete.

An explicit lineage pull writes the metadata structure and lineage sidecar without downloading the native package.
Automatic capture uses bounded direct upstream and downstream relationships.
Explicit pull supports bounded direction, depth, and transitive traversal.
Metadata API identifiers remain separate from REST LUIDs and never identify write targets.

### Datasource composition

Ordinary standalone datasource pull and publish can use the existing capability IDs after an authoritative composition preflight exists.
Composed and unknown artifacts remain rejected until B2 closes.
Published datasource sibling artifacts currently classified as unknown remain non-promotable.

### Project packages and partial outcomes

Project pull and publish remain blocked by B4.
The first natural multi-item capability must establish the standard partial-outcome shape before multi-item mutations fan out.
The shape preserves completed items, the failed substep, unattempted items, retry safety, compact omitted counts, and exit code 1.

## Required vertical slices

1. Freeze authoritative decisions, output enforcement, and fanout-safe integration.
2. Build named workspace configuration and resolution.
3. Build local artifact movement with the common envelope and no-lock concurrency policy.
4. Build the lineage foundation before independent pull actions add automatic capture.
5. Build a bounded list and get slice with exact selectors and compact and full output.
6. Extract shared destination, upload-session, and asynchronous-job infrastructure.
7. Build a flow pull and publish round trip as the second native artifact.
8. Fan out independent reads and unblocked actions under one integration owner.
9. Resolve the standalone datasource preflight and build the ordinary datasource round trip.
10. Build the first natural partial-outcome slice before multi-item mutations.
11. Close B2 and B4 only with captured upstream contracts and focused tests.

## Final decision record

The maintainer accepts named workspaces and the ADR-007 amendment.
ADR-032 remains in force, so V1 removes hybrid lock behavior instead of formalizing workspace locks.
Local movement means `workspace.move`; local artifact copy is not admitted by this decision.
Flow lifecycle stays package-preserving and excludes runtime orchestration.
Tableau owns cross-site published datasource rejection for unchanged packages.
Lineage and resource-specific deletion return to V1 under the bounded contracts above.
Mutation and deletion configuration flags remain tabled until the action set provides concrete policy inputs.
No additional promotion slice or TADX fail-closed PDS gate blocks movement and logistics fanout.
