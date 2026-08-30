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
- Catalog: refresh, search, get, status (normalized local cache; refresh may exhaust remote pages internally).
- Workspace: create, list, status, move, clean (local; no locking).
- Cross-content read: content search, content get.
- Workbook: list, get, pull, publish.
- Datasource: list, get, pull (ordinary), publish (ordinary).
- Flow: list, get, pull, publish.
- Project: list, get, create, update.
- Pulse: definition list/get/pull; metric list/get; definition/metric artifacts.
- Administration: user list/get/create/update/delete; group list/get/create/update/delete; permission get (read only).
- Doctor: non-mutating diagnostics.

## In the V1 registry but blocked (not executable until proof)

These are real V1 commitments, present as registry metadata, but not executable and not to be guessed into existence.
Each unblocks only when its named gate closes with captured official source plus a passing contract test.

- B1 datasource field-description write (published-datasource-field level): deferred fast-follow, pending the near-release TDS datasource-field API. Metadata API writes descriptions only at the upstream-table granularity, which is a different resource and not this row.
- B2 composable datasource round-trip and serialization: datasource pull/composition-update/publish for composed artifacts.
- B3 Pulse mutation schemas: pulse definition/metric create/update/delete/follow/unfollow.
- B4 shallow project direct-content enumeration: project pull, project publish.

## Delegated (discoverable, executed elsewhere)

Discoverable through the registry so agents are routed correctly, but TADX never executes them and never proxies MCP.

- Datasource analytical query / VDS: Tableau MCP.
- View and custom-view data/images: Tableau MCP.
- Pulse current values, insight bundles, briefs: Tableau MCP.
- Workbook semantic authoring/modification: Tableau Desktop / Desktop MCP.
- Datasource field-description generation: agent reasoning or a first-party skill (the reviewed write remains B1).

## Out of scope or deferred (do not build in V1)

- A separate datasource SDK product. The 12 SDK primitives are internal concerns, not a second product.
- Hyper API, and pack / unpack, and Hyper to CSV conversion.
- TDS remote work-copy editing, work-copy diff, and staged-change impact analysis.
- Generic lineage or dependency-graph traversal.
- Recursive project migration; generic bulk pull/publish.
- Permission mutation.
- Generic remote content move or hierarchy migration.
- OAuth, JWT, UAT, Connected App authentication.
- Plugin system, background daemon or sync, offline mutation queue.
- Phone-home telemetry, embedded auto-update, binary signing.
- Tableau Next.
- Any TADX verb that invokes or proxies MCP.

## Hard invariants (always true)

- PAT authentication only.
- Default output is TOON; JSON is an interop conversion target, not a second output mode.
- Consequential remote mutations are preview by default and require --apply. --force never means --apply.
- Mutation discovery gating (TADX_ENABLE_MUTATIONS=1) changes discovery only; it is never authorization.
- Tableau LUIDs are authoritative identity; names and paths are selectors; ambiguity is a deterministic error; no fuzzy or interactive resolution.
- Secrets TADX handles are never persisted in config values, output, logs, artifacts, catalog, fixtures, or diagnostics.
- One Go module, one primary binary, modular monolith.
- Release platforms: windows/amd64, darwin/amd64, darwin/arm64, linux/amd64.
- Exit codes: 0 success or no-op, 1 operation or runtime failure, 2 usage error.
