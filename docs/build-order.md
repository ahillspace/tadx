# Build order

This is the control layer that keeps parallel capability builds from drifting or racing on shared foundations.

Governing rule: freeze the foundation before fanning out actions.
A shared dependency (transport, an adapter pattern, the pagination envelope, the output codec) is built once by a single agent and frozen before the actions that depend on it fan out to parallel agents.
Do not let two agents invent the same foundation twice.

## Phase 0: repository foundation

Scope: the one-time scaffold. It establishes structure and implements only capability list and capability get; no other command becomes executable.

It must establish and freeze:
- the modular-monolith package layout and the dependency direction, with an architecture test that fails on violation;
- the executable capability registry and its validation;
- the thin Cobra layer and the per-capability action pattern;
- the authentication provider seam (PAT hidden behind it);
- the identity and selector model with the exact-match helper;
- the configuration model;
- the output layer and the TOON codec;
- the structured error type and exit-code mapping;
- generated capability reference docs and the clean-diff check;
- CI (format, vet, test, race, cross-compile, tidy).

Phase 0 exit gate:
- Phase 0 merges green on all target platforms.
- The TOON codec is chosen or implemented, conformance-tested, and frozen.
- axi, toon, action-guidelines, scope-v1, and AGENTS are checked in.

Do not start any resource slice before this gate closes.

## Phase 1: the thin vertical slice

Scope: prove the whole spine end to end on one narrow path before broad fan-out.

The slice: auth check, then catalog search (or content search), then workbook pull, then workbook publish (preview and apply), all driven through the registry, rendered in TOON, under the evidence gate.

Why this slice: it exercises PAT sign-in, the transport, REST read, pagination, identity resolution, the artifact model (pull), and preview/apply plus async upload (publish). If the spine holds here, it holds for the rest.

Building the slice also builds and freezes the shared spine:
- Tableau transport (HTTP execution, base URL, request authorization, standard headers, correlation and Tableau request IDs, response reading, upstream error capture, redaction). Port from the trusted Go inventory implementation.
- The first REST client family and the first resource adapter, establishing the adapter pattern other resources copy.
- The pagination normalization envelope.
- The pull artifact contract (canonical payload, metadata/provenance, required human-readable view, baseline fingerprint) and the re-pull dirty-guard.
- The publish preview/apply pattern with bounded internal async polling and upload sessions.

Phase 1 exit gate: the slice passes contract and golden tests; the transport, adapter pattern, pagination envelope, artifact contract, and publish pattern are frozen and documented.

## Phase 2: fan-out waves (buildable-now capabilities)

Within a wave, one agent builds the wave's shared dependency (the resource adapter) first; then the wave's actions fan out to parallel agents, each in its own action package.
A capability whose evidence is docs-only is built only to the adapter seam until its upstream contract is captured.

- Wave A, local-contract (no remote calls): environment profiles, auth status, workspace commands, catalog search/get/status. These depend only on Phase 0.
- Wave B, remote read: catalog refresh, content get, and list/get for workbook, datasource, flow, project. Proves each adapter's read path and pagination.
- Wave C, deliver in: pull for datasource (ordinary), flow, and Pulse definition; plus the Pulse definition and metric read commands and their artifacts.
- Wave D, deliver out: publish for datasource (ordinary), flow; project create and update.
- Wave E, administration: user list/get/create/update/delete, group list/get/create/update/delete, permission get.
- Wave F, doctor: full diagnostics once auth, catalog, workspace, and MCP-availability checks exist.

Fan-out rule: a resource adapter is written once by one agent and frozen before that resource's actions fan out. Mutations in any wave still preview by default and require --apply, and are hidden from default discovery unless mutation discovery is enabled.

## Blocked: not scheduled until proof

These stay registry metadata only and are not assigned to a build wave until their gate closes with captured official source and a passing contract test.

- B1 datasource field-description write (published-datasource-field level): unblocks on the released TDS datasource-field API.
- B2 composable datasource round-trip: datasource pull/composition-update/publish for composed artifacts; needs a controlled multi-parent fixture proving safe serialization and round-trip.
- B3 Pulse mutations: definition and metric create/update/delete/follow/unfollow; needs pinned schemas and destructive/idempotency behavior.
- B4 shallow project enumeration: project pull and publish; needs a proven direct-content enumeration contract with no child recursion.
