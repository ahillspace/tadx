# Repository architecture

TADX is one Go module and one CLI executable.
Each invocation constructs its own dependencies; detached transfers start another instance of the same executable, not a resident service.
TADX manages Tableau lifecycle operations and never calls or proxies Tableau MCP.
The [architecture diagram](architecture/index.html) summarizes runtime calls and dependency construction.
Its arrows are not a Go import graph.

## Find the owner

Start with the resource or service, then the operation file.
Workbook operations belong to `actions/workbook`; project operations belong to `actions/project`; authentication belongs to `actions/auth`.
Admin and Pulse directories are domain namespaces, not extra execution layers.
Distinct operation inputs, sequencing, projections, and safety rules remain explicit within each owner.

| Location | Responsibility |
| --- | --- |
| `cmd/tadx` | Process entry and exit status. |
| `internal/cli/<category>` | Cobra commands, flags, argument parsing, command-specific help, and presentation binding. |
| `actions/<resource-or-service>` | Requested outcome, local validation, operation sequencing, preview decisions, and result contracts. |
| `internal/resources/<resource>` | Exact resolution, provider normalization, and implementations of action-owned ports. |
| `internal/tableau/<api>` | Native HTTP/GraphQL requests, response contracts, and acknowledgements. |
| `internal/app` | Invocation-scoped construction, client/session lifetime, target binding, and private process dispatch. |
| Named infrastructure packages | Shared mechanisms with concrete consumers and matching invariants. |

Action Services expose named operations through explicit dependencies.
Providers open lazily after input validation when authentication or remote access is required.
Help and invalid arguments must not authenticate or contact Tableau.
App supplies providers; it does not add a forwarding method for every command.

An operation does not need a wrapper in every layer.
Catalog can receive native metadata clients directly because they already implement narrow value-based contracts.
Resource adapters remain useful when exact identity, hierarchy, provider normalization, or resource-specific scope requires translation.

## Follow calls and imports

Runtime calls follow this path when provider translation is needed:

```text
command -> action Service -> injected resource port -> native client
```

Construction happens outside that chain in app.
The action declares the port it consumes, and the resource adapter imports that contract to implement it.
The action does not import the resource adapter.
Actions do not import sibling actions, Tableau clients, Cobra, or HTTP clients.
Resource adapters do not call action workflow entry points.
Native clients do not import actions, resources, or CLI packages.

The architecture checker enforces explicit production import boundaries.
Selected infrastructure dependencies are exact-owner permissions, not permission for neighboring or nested packages.
Shared observations in `internal/value` depend only on the standard library.
Native wire types and operation-specific output types remain separate when their semantics differ.

## Keep shared mechanisms distinct

`internal/inventory` owns neutral bounded collection, completeness observations, and scoped cache publication.
`internal/cache` owns storage, generations, and local queries.
Resource actions choose cache or live behavior; adapters translate typed observations.
An explicit cached read never silently becomes a native read.
Complete unfiltered inventory replacement remains distinct from filtered or partial updates.

`internal/jobmonitor` owns accepted Tableau job receipts and bounded shared observation.
`internal/operationrun` owns local worker records, startup acknowledgement, leases, detached waiting, and durable process state.
A local worker is not a Tableau job.
`actions/job` reconciles saved operation evidence without resubmitting an operation.
Each resource owns publication completion and destination verification.

`internal/workspace` owns workspace registration and rooted local state.
`internal/artifact` owns package persistence, provenance, and dirty-file guards.
`internal/config` owns nonsecret configuration and durable replacement.
`internal/auth` owns credential-store and session mechanisms; `actions/auth` owns ordering and compensation across those boundaries.

`internal/output` owns shared bounded presentation and saved-operation snapshots.
`internal/toon` owns TOON encoding.
`internal/lastcommand` records saved results without redefining the remote operation's outcome.
Resource-named files group command constructors and their help facts within each category package.
Category registration and genuinely shared flags stay centralized; shared help code handles metadata and rendering.

## Preserve evidence and safety

Tableau LUIDs are authoritative, and ambiguous names fail deterministically.
Fresh-state checks remain where remote or local facts can change.
Preview does not authorize a mutation or enable site consent.
Mutation execution uses saved consent for the canonical server and exact site, plus all other policy checks.

Configuration stores opaque credential references, not PATs.
Approved persistent PATs belong only in the native OS credential store.
Credentials and session tokens never belong in logs, fixtures, artifacts, receipts, or output.
Persisted artifact paths stay relative to the resolved workspace and use forward slashes.

Native acknowledgements, request IDs, confirmed identities, and unknown effects survive partial failures.
Transport retryability does not establish that resubmission is safe.
Accepted native jobs and local download workers retain different recovery evidence.
Compact and full output remain projections of the same operation; `--json` changes encoding, not behavior.

## Navigate tests and contributions

Action tests protect validation, sequencing, previews, outcomes, and recovery contracts.
Resource and native-client tests protect identity translation, request shape, acknowledgements, and protocol failures.
CLI tests protect syntax and presentation.
App tests exercise real command paths against controlled stores or HTTP servers.
Shared-mechanism tests protect bounds, persistence, concurrency, and failure recovery across consumers.

Name a test for a concrete failure it detects, not the review phase that introduced it.
Move meaningful assertions with their owner instead of retaining obsolete production wrappers for tests.
Native platform checks and authorized live tasks provide evidence that isolated tests cannot establish.
No finite suite guarantees zero regressions.

The [contributor walkthrough](contributor-walkthrough.md) traces a read, a mutation, and a local-state change through these boundaries.
[CONTRIBUTING](../CONTRIBUTING.md) is the human entry point; the repository build skill supplies focused implementation references for agent-assisted work.
[Capability definitions](../internal/capability/definitions.go) remain separate from executable syntax.
`cmd/gencapdocs` generates their Markdown, JSON, and HTML references through the existing tooling.
The [implementation record](maintainability/implementation-status.md) and [regression gates](maintainability/regression-gates.md) track acceptance evidence separately from this architecture description.
