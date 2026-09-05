# TADX V1 scope

Product outcome: Higher accuracy. Lower tokens. Faster.

This is the single token-light reference for what V1 does and does not build.
It is derived from the arc42 (architecture authority) and the V1 capability contract (capability inventory); it does not replace them.
When this page and a source doc disagree, the source docs win and this page is fixed.

## Buildable in V1 (Ship, CLI-owned)

These are the capabilities an agent may build to completion once each row's upstream API contract is captured (see the evidence gate).

- Environment profiles: list, get, add, update, remove, set-default (local config).
- Authentication: PAT sign-in check (remote) and local auth status. PAT only.
- Capability discovery: capability list, capability get.
- Catalog: refresh and status for the normalized local cache.
- Workspace: create, list, status, move, delete one explicit artifact, and clean disposable state (local; no locking).
- Shared search: live by default across content, administration, and Pulse, with optional `--catalog`.
- Lineage: bounded automatic capture with workbook, datasource, and flow pulls; standalone pull into a metadata-only artifact.
- Workbook: list, inspect, pull, publish, delete.
- Datasource: list, inspect, pull, publish, delete; pull and publish preserve existing ordinary or composed packages without authoring composition.
- Flow: list, inspect, pull, publish, move, delete.
- Project: list, inspect, create, update.
- Pulse: definition list/inspect/pull/create/delete; metric list/inspect/fork/delete/follow/unfollow/followers; definition/metric artifacts.
- Administration: user list/inspect/create/update/delete; group list/inspect/create/update/delete; permission inspect (read only).
- Doctor: non-mutating diagnostics.

## Deferred from V1

These capabilities do not appear in the executable V1 registry.

- Datasource field-description updates.
- Datasource composition authoring.
- Pulse definition and metric updates.
- Project pull and publish.

## Delegated (discoverable, executed elsewhere)

Discoverable through the registry so agents are routed correctly, but TADX never executes them and never proxies MCP.

- Datasource analytical query / VDS: Tableau MCP.
- View and custom-view data/images: Tableau MCP.
- Pulse current values, insight bundles, briefs: Tableau MCP.
- Workbook semantic authoring/modification: Tableau Desktop / Desktop MCP.
- Datasource field-description generation: agent reasoning or a first-party skill.

## Out of scope or deferred (do not build in V1)

- A separate datasource SDK product. The 12 SDK primitives are internal concerns, not a second product.
- Hyper API, and pack / unpack, and Hyper to CSV conversion.
- TDS remote work-copy editing, work-copy diff, and staged-change impact analysis.
- Recursive project migration; generic bulk pull/publish.
- Permission mutation.
- Generic remote content move or hierarchy migration beyond the admitted resource-specific `flow move` operation.
- Project deletion, recycle-bin recovery, permanent purge, and automatic cleanup policy.
- OAuth, JWT, UAT, Connected App authentication.
- Plugin system, background daemon or sync, offline mutation queue.
- Phone-home telemetry, embedded auto-update, binary signing.
- Tableau Next.
- Any TADX verb that invokes or proxies MCP.

## Hard invariants (always true)

- PAT authentication only.
- Default output is TOON; JSON is an interop conversion target, not a second output mode.
- Consequential mutations run by default when enabled and support `--preview` for a read-only plan.
- Remote mutation commands and capabilities are always discoverable.
- `TADX_ENABLE_MUTATIONS=1` enables mutation command execution.
- `--force` does not bypass mutation policy.
- Tableau LUIDs are authoritative identity; names and paths are selectors; ambiguity is a deterministic error; no fuzzy or interactive resolution.
- Workspace selectors are logical names, unique case-insensitively, and resolved through one canonical registry.
- A publish without an explicit target uses the artifact's recorded source environment, site, and project.
- An explicit environment override requires an exact target project.
- An artifact without complete source provenance requires an explicit target.
- Automatic lineage capture is bounded and best-effort, records incomplete results, and never hides a successful artifact pull.
- Persisted artifact paths are relative and slash-delimited.
- First-party source, documentation, generated files, fixtures, and persisted metadata contain no developer names, private project names, or machine-specific paths.
- Secrets TADX handles are never persisted in config values, output, logs, artifacts, catalog, fixtures, or diagnostics.
- One Go module, one primary binary, modular monolith.
- Release platforms: windows/amd64, darwin/amd64, darwin/arm64, linux/amd64.
- Exit codes: 0 success or no-op, 1 operation or runtime failure, 2 usage error.
