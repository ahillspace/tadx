---
title: Movement and logistics readiness proposal
status: awaiting-collaborator-review
created: 2026-08-31
decision_scope: Pre-fanout foundations and vertical slices
authoritative: false
decision_owner: Andrew Hill
collaboration_prompt: |
  You are the collaborating TADX architecture agent working from the current main branch of this GitHub repository.
  Review this proposal against the implementation and the authoritative sources listed below, including the AXI contract.
  Challenge every proposed prerequisite and identify any missing cross-cutting contract or vertical slice.
  Do not implement code or edit authoritative contracts during this review.
  Put your review only under the Collaborator response heading at the end of this file so the proposal remains easy to compare with your conclusions.
  Cite exact repository files for every disagreement or material addition, and explain the failure, ambiguity, or rework that your recommendation prevents.
  Specifically answer whether any additional vertical slices are required before movement and logistics work fans out, which proposed slices are true pre-fanout gates, how ordinary versus composed datasource commands should be represented while B2 is open, what proof is required before cross-site published datasource promotion is safe, and the minimum safe build order with exit gates.
  For every numbered proposal section, return approve, revise, or reject with a short rationale.
  End with a prioritized build order, the vertical slices you recommend locking, and any decisions that require Andrew.
---

# Movement and logistics readiness proposal

## Decision requested

Before TADX fans out across movement, logistics, and the remaining resource actions, decide which shared contracts and vertical slices must be frozen first.

This proposal is intentionally not an implementation authorization.
It is a review surface for two agents and Andrew to converge on the smallest complete foundation that prevents later slices from inventing incompatible behavior.

## Authoritative sources

Review this proposal against these sources in precedence order:

1. `AGENTS.md`
2. `tadx-v1-capability-contract-final.md`
3. `tableau-agent-development-harness-arc42-locked.md`
4. `docs/scope-v1.md`
5. `docs/build-order.md`
6. `docs/axi.md`
7. `docs/contributing/output-guidelines.md`
8. `docs/contributing/adding-a-capability.md`
9. `docs/phase1-spine.md`

## Current position

The Phase 1 workbook spine is complete.
Workbook pull and publish have passed a live round trip against the disposable Tableau Cloud development site.
Published datasource discovery and acquisition have also passed a live contract test and a live workbook pull.
The compact and full output contract has been introduced for workbook pull.

These successes prove the first resource path, but they do not yet make unrestricted parallel implementation safe.
Several cross-cutting contracts are still either absent, implemented only for workbooks, or contradicted by current code.

## Existing locked constraints

The following constraints remain in force unless Andrew explicitly approves an amendment to their authoritative source:

- PAT authentication is the only authentication mode.
- TOON is the default output and `--full` requests bounded additional detail.
- Consequential mutations preview by default and require `--apply`.
- Tableau LUIDs are authoritative, and ambiguous selectors fail.
- Secrets are never persisted or printed.
- Generic remote content movement and hierarchy migration are outside V1.
- Promotion is composed from pull and publish rather than hidden inside a generic remote movement command.
- Blocked and docs-only capabilities cannot acquire executable Tableau behavior before their evidence gates close.

This proposal distinguishes local workspace and artifact logistics from remote delivery between Tableau sites.

## Proposed pre-fanout decisions

### 1. Named workspace identity and one resolver

`--workspace` should accept a registered, case-insensitively unique logical name such as `dev`, not an arbitrary filesystem path.

The global config should map each name to a stable workspace ID and canonical absolute path.
Each workspace-local `tadx.yaml` should contain a schema version, the workspace name, and the same stable ID.
Environment and global defaults should store workspace names rather than paths.

All commands should use one resolution order:

1. An explicit workspace name.
2. The registered workspace containing the current directory.
3. The selected environment's default workspace.
4. The global default workspace.
5. A deterministic error.

Actions should receive a validated resolved-workspace value rather than an unchecked string.
If a direct path escape hatch remains necessary for recovery, it should be a separate `--workspace-path` option and should never create a second identity for the same workspace.

The global config also needs one atomic read-modify-write store before environment and workspace management actions are built.

This changes the current path-based configuration and deterministic resolution contract in the arc42, including ADR-007.
Approval therefore requires an explicit architecture amendment rather than an implementation-only interpretation.

Acceptance gate:

- Duplicate names are rejected using the same case rules on Windows, macOS, and Linux.
- Duplicate workspace IDs and duplicate canonical paths are rejected.
- Moved workspace roots are detected and repaired explicitly.
- All commands use the same resolver and precedence rules.
- Concurrent config changes cannot silently overwrite one another.

### 2. Common artifact identity and movement semantics

Every artifact metadata document should have a schema version and a common identity envelope.
The common identity should include server origin, site LUID, resource kind, and Tableau LUID.
Resource-specific metadata should remain in typed extensions.

One generic inspection contract should define canonical payload discovery, baseline fingerprinting, dirty state, dependency references, and unsupported or corrupt metadata behavior.

The public meaning of local movement must also be explicit.
The capability registry should distinguish reorganizing an artifact within a workspace, transferring an artifact between named workspaces, and relocating an entire workspace root.
The proposal should decide whether local artifact copy is a separate capability rather than overloading move.

Movement behavior must define cross-volume operations, destination collisions, dirty destinations, dependency closure, rollback, and interruption recovery.
Published datasource links must resolve by stable identity or be updated atomically so moving a workbook or datasource cannot leave stale relative paths.

Acceptance gate:

- A workbook and all selected dependencies can be transferred between two named workspaces without broken references.
- Dirty sources and destinations produce deterministic, non-destructive outcomes.
- A process interruption leaves either the old state or the new state recoverable.
- Cross-volume behavior is verified on the supported operating-system families.
- Artifact identity never depends on a mutable display name or environment alias.

### 3. One workspace concurrency policy

The authoritative contract says V1 has no workspace locks, while the artifact implementation currently uses `.tadx.lock` and the architecture allowlist permits it.
This hybrid state should not be copied into additional local actions.

ADR-032 currently prohibits workspace locks.
Formal advisory locks are therefore a proposal to reopen that decision, not a compatible implementation detail.

The decision should either remove workspace locking completely and specify safe atomic-operation boundaries, or formalize one advisory locking policy for every local mutation.
The policy must cover lock ownership, stale lock recovery, process crashes, read behavior during writes, and commands that touch multiple workspaces.

Atomic replacement, optimistic validation, and recovery journals should be preferred if they protect the required transactions without coordination semantics.
The collaborating review should determine whether any demonstrated multi-artifact failure mode actually requires reopening ADR-032.

Acceptance gate:

- The capability contract, arc42, implementation, tests, and cleanup behavior describe the same policy.
- Two concurrent writers cannot silently corrupt config, artifacts, or dependency references.
- Stale recovery cannot discard a live writer's ownership.

### 4. Shared destination and promotion semantics

Workbook, datasource, flow, and project mutations should use one destination model.

For a same-site republish, a recorded source LUID may identify the existing target.
For a cross-site publish, a source LUID is provenance only and can never identify the target resource.
Cross-site targets should resolve by exact name and exact canonical project path, with ambiguity rejected.

Every publish action should use the same create, collision, overwrite, and immediate pre-POST revalidation rules.
The contract should also distinguish artifact fields that may provide target defaults from CLI fields that must remain explicit for authorization.
Site equivalence should use stable server-origin and site-LUID identity rather than environment alias labels.

Acceptance gate:

- Identical collision scenarios produce identical plans across resource types.
- Cross-site publication cannot overwrite a resource because a source-site LUID happened to match.
- The exact overwrite target is revalidated immediately before the final mutation request.
- Mutation previews never hide authorization-critical destination fields.

### 5. Shared publish infrastructure

Before datasource and flow publish actions fan out, extract the reusable upload-session and asynchronous-job behavior proven by workbook publish.

The shared layer should own upload initiation, append sequencing, upload-session identity validation, response bounds, terminal job-state vocabulary, timeout behavior, and request-ID and job-ID preservation.
Resource clients should continue to own their multipart bodies and resource-specific REST contracts.

A shared project resolver and client should also replace any temptation for future resource adapters to depend on workbook-specific project logic.

Acceptance gate:

- Workbook publish still passes without behavioral change after extraction.
- A second resource proves reuse without copying workbook transport code.
- Terminal failure, unknown status, timeout, and malformed upload-session responses have behavioral tests.

### 6. Output contract enforcement

Compact TOON remains the default, and `--full` remains the universal request for bounded detail.
Compact output should include `details: "--full"` only when useful details were intentionally omitted.
This is the token-efficient discovery mechanism and avoids advertising the option on responses where it adds nothing.

The renderer should not silently expose a complete detail-bearing struct when a capability has not implemented compact and full projections.
Before other mutations copy workbook publish, workbook publish should be brought under the same compact and full contract as workbook pull.

Representative output shapes should be frozen for a one-resource read, a bounded list, a local change, a mutation preview and result, and a partial outcome.
Mutation previews must keep all authorization-critical fields in compact output.

Acceptance gate:

- Every implemented detail-bearing capability has explicit compact and full projections.
- An architecture or contract test rejects accidental fallback to complete output.
- Full output is bounded and cannot print secrets, raw unbounded upstream bodies, or full binary payloads.
- The compact response tells an agent how to obtain hidden detail without requiring command hunting.

### 7. Fanout-safe CLI integration

Parallel slice work currently converges on shared hot files for the capability manifest, root wiring, app services, and content command registration.
Command registration should be split so each implemented capability can mount independently.
Domain command factories and application services should be divided by resource or capability rather than accumulated in single files.

One integration owner should own manifest updates, generated registry and documentation output, and final root wiring for a fanout batch.
Slice agents should not each regenerate and commit shared generated files.

The contributor instructions should also resolve the current conflict between a task template that assumes the capability registry row exists and a guide that tells every slice agent to add it.

Acceptance gate:

- A capability can be compiled and tested without placeholder services for sibling capabilities.
- Independent slice branches do not require routine edits to the same root files.
- Generated files have one deterministic integration step and are never edited manually.

### 8. Live development testing policy

The repository should state that the configured `dev` environment and its shell-referenced PAT are valid and authorized for live development tests against the disposable Tableau Cloud instance.
The standard test suite must remain hermetic.
Live tests should remain explicitly tagged or opt-in, sanitize evidence, use unique disposable content where possible, and clean up only content they created.

Each remote action slice should run a focused live verification after local tests and before its upstream contract is marked verified.

Acceptance gate:

- Agents can discover the authorized live-test policy without asking for credentials.
- PAT names, PAT secrets, session tokens, and sensitive response bodies never enter source, logs, fixtures, or evidence.
- Standard CI never depends on the live site.
- Live cleanup is ownership-scoped and cannot delete unrelated content.

## Contract gaps that affect the build order

### Ordinary and composed datasource actions

The scope and build-order documents describe ordinary datasource pull and publish as buildable, while the canonical capability registry currently marks the full commands blocked by B2 composition semantics.
Contributor rules prohibit executable code for a blocked capability.

The proposed resolution is to make the ordinary standalone path buildable while requiring an authoritative composition preflight.
Artifacts classified as `composed` or `unknown` should fail closed until B2 is resolved.
Only artifacts authoritatively classified as standalone should be publishable.

Published datasource sibling artifacts currently record `composition_status: unknown`, so they should not be promoted until that classification is resolved.
The public capability IDs should remain stable unless the collaborator identifies a stronger contract reason to split them.

### Cross-site published datasource promotion

The current evidence proves discovery and acquisition, not cross-site rebinding after a dependency is published to a target site.
Before TADX claims dependency-aware promotion, a live contract must prove publish order, target datasource identity mapping, unchanged workbook bytes, same-name collision behavior, and failures for missing or inaccessible dependencies.

Until that evidence exists, applying a source-site-bound workbook to a different site should fail closed rather than merely warn.
The review should decide whether this needs a new named evidence gate and whether a second disposable Tableau site is a prerequisite.

### Project pull and publish

Project pull and publish remain blocked by B4.
The missing contract includes shallow direct-content enumeration, manifest schema, direct membership semantics, published datasource sibling inclusion, target mapping, operation order, and partial outcomes.
Cloud and representative Server fixtures are both required before this package should be treated as buildable.

### Partial outcomes

The current action and CLI patterns discard useful output when an error is returned.
Before multi-item project operations or group-membership changes are implemented, TADX needs one partial-outcome shape covering completed items, the failed substep, not-attempted items, retry safety, exit code 1, and compact omitted counts.

### Remote copy, move, and promote

Generic remote copy, move, and promote should remain outside V1.
Promotion should remain an agent-composed pull and publish workflow unless an authoritative capability is added later.
Local workspace and artifact logistics are separate product capabilities and should not imply a hidden remote orchestration command.

## Candidate vertical slices

### Slice A: Configuration and named workspace lifecycle

Purpose: prove the registry and resolution contract before every later action begins accepting logical workspace names.

Proposed scenario:

1. Register a workspace with a logical name, stable ID, and canonical path.
2. Set global and environment defaults by name.
3. Resolve explicit, containing, environment-default, and global-default cases.
4. Rename, relocate, and remove a workspace without creating duplicate identity.
5. Exercise concurrent config updates and interrupted writes.

Exit gate: case-insensitive name uniqueness, stable IDs, canonical paths, discovery, defaults, rename, relocation, removal, atomic updates, and cross-platform behavior are verified.

Proposed classification: required before broad fanout.

### Slice B: Local workspace and artifact logistics

Purpose: prove named workspace identity, atomic config updates, the common artifact envelope, status, movement or copy semantics, dependency handling, and the chosen concurrency policy as one end-to-end path.

Proposed scenario:

1. Create or register two workspaces by logical name.
2. Pull a workbook with a published datasource dependency using `--workspace dev`.
3. Inspect status through the logical name.
4. Transfer or move the selected artifact and dependency closure to the second workspace.
5. Verify stable identity, clean state, valid dependency resolution, and recoverable source behavior.

Exit gate: duplicate names, moved roots, collisions, dirty state, dependency policy, rollback, interruption, and supported cross-platform behavior are all verified.

Proposed classification: required before broad movement and logistics fanout, after Slice A.

### Slice C: Bounded read

Purpose: freeze pagination, exact selectors, compact and full list output, continuation metadata, and bounded result behavior before many list and get actions are implemented independently.

The slice should use one natural list and get pair from an unblocked resource rather than add a synthetic command.

Proposed scenario:

1. List a resource with a deliberately small page bound.
2. Continue from the returned position without duplicates or omissions.
3. Resolve one exact resource by LUID and by its supported human selector.
4. Reject ambiguity and invalid selector combinations deterministically.
5. Compare compact and full output while preserving bounds in both modes.

Exit gate: local and live tests prove stable ordering, continuation behavior, exact resolution, ambiguity failure, compact omitted counts, and bounded full detail.

Proposed classification: required before broad list and get fanout.

### Slice D: Flow full round trip as the second resource

Purpose: prove that the workbook spine generalizes and that shared project, upload, job, artifact, target, and output contracts are genuinely resource-neutral.

Flow is the proposed second resource because it is a native non-workbook artifact and does not carry the unresolved datasource composition blocker.

Proposed scenario:

1. List or get a flow from the disposable development site.
2. Pull its TFL or TFLX artifact into a named workspace.
3. Preview a publish to an exact project and unique target name.
4. Apply the plan and verify the remote identity and terminal outcome.
5. Pull the result and verify baseline and dirty-state behavior.

Exit gate: the slice reuses shared infrastructure, contains no workbook-specific imports, and passes focused local and live tests with compact and full output.

Proposed classification: required before parallel datasource and flow lifecycle implementation.

### Slice E: Ordinary datasource full round trip

Purpose: separate the ordinary `.tds` and `.tdsx` lifecycle from composed datasource behavior that remains behind B2.

The slice is valid only after the capability registry and authoritative contracts clearly permit the standalone path and define the fail-closed composition preflight.

Exit gate: a standalone datasource passes pull, inspect, publish preview, apply, target verification, and repull while composed and unknown artifacts are rejected before mutation.

Proposed classification: required before ordinary datasource lifecycle fanout, but blocked until the registry contradiction is resolved.

### Slice F: Dependency-aware cross-site promotion verification

Purpose: prove the exact live sequence for publishing an acquired published datasource, mapping its target identity, publishing the unchanged workbook, and verifying the relationship on the target site.

Negative cases should include missing dependencies, duplicate target names, inaccessible dependencies, and a workbook that remains bound to the source site.

Exit gate: sanitized behavioral evidence demonstrates successful rebinding and deterministic fail-closed behavior for every negative case.

Proposed classification: required before dependency-aware cross-site promotion is advertised, but not necessarily before basic flow or project reads.

### Slice G: Partial outcome through a natural multi-item capability

Purpose: prove the standard partial-outcome contract through the first real capability that performs multiple independently reportable substeps.

This should not create a synthetic public command solely for testing.
The first B4 project action or a group-membership update should carry the slice once its upstream contract is available.

Exit gate: compact and full results preserve completed, failed, and not-attempted work together with retry guidance and exit code 1.

Proposed classification: required before multi-item mutation fanout, but not before independent single-resource actions.

## Proposed build order

1. Freeze the decisions in this proposal and amend every controlling contract that changes.
2. Build Slice A as the configuration and named-workspace proof.
3. Build Slice B as the local workspace and artifact logistics proof.
4. Enforce output projections and make capability registration safe for parallel work.
5. Build Slice C as the bounded read proof.
6. Extract and verify shared project, upload-session, and asynchronous-job infrastructure.
7. Build Slice D as the flow full-round-trip proof.
8. Fan out independent environment, workspace, workbook read, flow, and unblocked project actions under one integration owner.
9. Resolve the datasource ordinary-versus-composed registry state, then build Slice E and only the authoritatively allowed datasource path.
10. Build Slice F before enabling dependency-aware cross-site promotion.
11. Close B2 and B4 with captured contracts rather than inferred API behavior.
12. Build Slice G before project package mutations or other multi-item actions fan out.

## Decisions requested from the collaborator

1. Which numbered foundations are mandatory before any fanout, and which can be deferred to a specific wave?
2. Are Slices A through D sufficient to prove the shared foundation, or is another vertical slice required first?
3. Should local movement mean move, copy, transfer, or separate capabilities with distinct safety semantics?
4. Should V1 formalize advisory workspace locks or remove them in favor of atomic operations and optimistic validation?
5. Should ordinary standalone datasource behavior be unblocked within the existing capability IDs while composed and unknown artifacts fail closed?
6. Does safe cross-site published datasource promotion require a second disposable site and a new evidence gate?
7. Which natural capability should first prove partial outcomes?
8. What should be changed in the proposed build order before it becomes authoritative?

## Contract changes required after approval

This proposal is not authoritative and should not remain the only record of an approved decision.
After Andrew approves a final decision set, update the arc42 decision ledger and affected ADRs, the V1 capability contract, the build order, the scope summary, and the contributing guides in one reviewed change.
Update executable capability metadata only when its controlling evidence and blocker state have actually changed.
Do not implement slices against proposal text that has not yet been transferred into the authoritative sources.

## Non-goals

- This proposal does not authorize implementation.
- This proposal does not unblock B2 or B4 without captured upstream evidence.
- This proposal does not add generic remote copy, move, or promote commands.
- This proposal does not place live credentials or live tests in the standard test suite.
- This proposal does not allow agents to infer blocked Tableau behavior from documentation alone.

## Collaborator response

<!-- Collaborating agent: replace this comment and the Pending line with your structured review. Do not edit the proposal above during the first review pass. -->

Pending.

## Final decision record

Pending Andrew's decision after collaborator review.
