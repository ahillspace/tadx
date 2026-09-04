# Build order

This is the control layer that keeps parallel capability builds from drifting or racing on shared foundations.

Governing rule: freeze the foundation before fanning out actions.
A shared dependency (transport, an adapter pattern, the pagination envelope, the output codec) is built once by a single agent and frozen before the actions that depend on it fan out to parallel agents.
Do not let two agents invent the same foundation twice.

Create one feature branch for each build.
Agents share that checkout, work on assigned non-overlapping paths, and do not create separate worktrees unless the build owner explicitly requests them.
One integration owner controls shared wiring, manifests, contract rows, generated files, and required build verification.
Required build verification does not include a comprehensive branch review unless the build owner requests one.

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
- axi, toon, `CONTRIBUTING.md`, scope-v1, and `AGENTS.md` are checked in.

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

## Phase 2: grouped non-Pulse builds

The registry contains 59 CLI-owned non-Pulse capabilities.
Phase 1 already implements six: `auth.check`, `capability.list`, `capability.get`, `catalog.search`, `workbook.pull`, and `workbook.publish`.
The following groups cover the remaining 53 capabilities.

Before broad action fan-out, freeze these shared foundation slices in order:

1. Freeze compact versus `--full` output projections and their bounded output tests.
2. Freeze named workspace resolution and portable relative artifact paths.
3. Freeze the no-lock artifact envelope, local move behavior, and explicit artifact deletion boundary.
4. Freeze the lineage sidecar schema, bounded traversal contract, and workbook live proof.
5. Freeze the bounded read patterns used by resource adapters.
6. Freeze shared publish targeting, preview, upload, and terminal result behavior.
7. Prove unchanged TFL/TFLX flow pull and publish behavior with hermetic contract tests, and provide an opt-in live lifecycle test for deployment verification.

Within a group, one agent builds the group's shared dependency first.
After that dependency is frozen, action packages fan out to agents on the shared feature branch.
A capability whose evidence is docs-only is built only to the adapter seam until its upstream contract is captured.

### Group 1: operator setup and flow lifecycle

Build the 21 capabilities that establish daily operator setup and prove a complete second content-resource lifecycle:

- Environment profiles: `env.profile.list`, `env.profile.get`, `env.profile.add`, `env.profile.update`, `env.profile.remove`, and `env.profile.set-default`.
- Authentication state: `auth.status`.
- Named workspaces: `workspace.create`, `workspace.list`, `workspace.status`, `workspace.move`, and `workspace.artifact.delete`.
- Project selection: `project.list` and `project.get`.
- Flow lifecycle: `flow.list`, `flow.get`, `flow.pull`, `flow.publish`, `flow.move`, and `flow.delete`.
- Lineage: `lineage.pull`.

This group also freezes named workspace uniqueness, portable relative paths, compact and `--full` output, lineage sidecars, and unchanged TFL/TFLX round trips.
Workbook and flow pulls capture bounded lineage automatically while keeping lineage details out of compact output.
Flow actions do not rewrite packages, connections, credentials, schedules, linked tasks, or published datasource bindings.
The official-source captures and hermetic tests establish contract-verified project, flow, and lineage evidence.
They do not establish live-verified evidence.
Do not claim the build-tagged flow or lineage live tests have run until their opt-in deployment checks complete.

The build agents perform task-level review, focused tests, contract tests, and assigned integration tests.
They stop after those checks pass.
The build owner then performs manual testing and separately decides when to start the multi-agent branch review.
Do not start a comprehensive branch review, a review board, or the no-mistakes pipeline automatically.

Do not start Group 2 until the build owner accepts Group 1 after manual testing and branch review.
After that acceptance, proceed through Groups 2 through 5 in order to complete the remaining non-Pulse scope.
A comprehensive review between later groups is optional unless the build owner requests one.

### Group 2: discovery and remote inventory

Build these nine read-oriented capabilities:

- Catalog: `catalog.refresh`, `catalog.get`, and `catalog.status`.
- Generic content discovery: `content.search` and `content.get`.
- Workbook inventory: `workbook.list` and `workbook.get`.
- Datasource inventory: `datasource.list` and `datasource.get`.

The Group 2 build releases the catalog, workbook inventory, and datasource inventory actions above.
`content.search` and `content.get` remain non-executable action seams until their separate normalization evidence closes.

This group freezes bounded concurrent pagination, exact selection, selectable catalog scopes, transactional SQLite generations, staleness reporting, and shared remote read behavior.

### Group 3: remaining content lifecycle

Address these nine active capabilities:

- Workbook deletion: `workbook.delete`.
- Datasources: `datasource.pull`, `datasource.field-description.update`, `datasource.publish`, and `datasource.delete`.
- Projects: `project.create`, `project.update`, `project.pull`, and `project.publish`.

Build the capabilities whose evidence gates are open.
Keep blocked capabilities as registry metadata until their exact evidence gates close.
Do not guess an upstream contract to claim group completion.

Datasource pull and publish preserve ordinary and composed packages through the same user-facing workflow.
TADX preserves existing composition and required parent references without authoring or changing relationships.
`datasource.composition.update` is deferred indefinitely pending a supported TDS authoring API.
`datasource.field-description.update` remains blocked by B1, and project pull and publish remain blocked by B4.

### Group 4: administration

Build these 11 capabilities:

- Users: `admin.user.list`, `admin.user.get`, `admin.user.create`, `admin.user.update`, and `admin.user.delete`.
- Groups: `admin.group.list`, `admin.group.get`, `admin.group.create`, `admin.group.update`, and `admin.group.delete`.
- Permissions: `admin.permission.get`.

This group freezes exact administrative identity, bounded membership handling, preview and apply behavior, and permission inspection output.

### Group 5: diagnostics and closure

Build `doctor.run` after its auth, catalog, workspace, and MCP-availability checks exist.
Build `workspace.clean` last because it removes only explicitly selected disposable `.tadx/` state and never managed artifacts by default.

### Pulse exclusion

Do not include Tableau Pulse capabilities in these groups.
Pulse remains a separate body of work with its own evidence and review gates.

Fan-out rule: a resource adapter is written once by one agent and frozen before that resource's actions fan out.
Slice agents own only assigned action and resource packages unless the integration owner assigns a shared file.
Remote mutation commands and capabilities remain visible regardless of execution policy.
`TADX_ENABLE_MUTATIONS=1` enables mutation command execution; enabled commands still preview by default and require `--apply` for the remote change.

Flow scope stays narrow.
Pull and publish preserve TFL/TFLX bytes and let Tableau validate embedded published datasource, file, and database references.
TADX does not rewrite flow connections, credentials, published datasource bindings, schedules, linked tasks, or execution settings.

Use the local Tableau API documentation as the first targeted search surface.
Search only the relevant endpoint or schema section, and do not load an entire reference file into agent context.
Use official Tableau web documentation when the local capture is missing, ambiguous, or version-sensitive.
Every implemented remote contract still requires captured evidence and contract tests.

## Blocked: not scheduled until proof

These stay registry metadata only and are not assigned to a build wave until their gate closes with captured official source and a passing contract test.

- B1 datasource field-description write (published-datasource-field level): unblocks on the released TDS datasource-field API.
- B2 datasource composition authoring: `datasource.composition.update` is deferred indefinitely pending a supported TDS authoring API.
- B3 Pulse mutations: definition and metric create/update/delete/follow/unfollow; needs pinned schemas and destructive/idempotency behavior.
- B4 shallow project enumeration: project pull and publish; needs a proven direct-content enumeration contract with no child recursion.
