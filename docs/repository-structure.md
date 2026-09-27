# Repository architecture

TADX is one Go module and one CLI binary with a runtime scoped to each command.
It has no resident service and does not call or proxy Tableau MCP.
The [architecture diagram](architecture/index.html) shows command flow and dependency wiring.
The [SVG](architecture/tadx-architecture.svg) and [editable Excalidraw source](architecture/tadx-architecture.excalidraw) accompany the diagram.

## Package responsibilities

| Package | Responsibility |
| --- | --- |
| `cmd/tadx` | Process entry point |
| `internal/app` | Composition root, target and workspace selection, command runtime, and concrete dependency wiring |
| `internal/cli` | Cobra command tree, argument parsing, and action invocation |
| `actions/workbook`, `actions/datasource`, `actions/flow` | Cohesive resource packages for related lifecycle operations |
| `actions/workspace`, `actions/job` | Shared operation packages with explicit methods and operation-specific contracts |
| `actions/admin` subpackages | Related lifecycle or paired mutation packages where records and validation match |
| `actions/pulse/definition`, `actions/pulse/metric` | Resource packages with explicit operations and distinct output contracts |
| `actions/env/profile` | Profile operations with a shared profile representation |
| `actions/catalog/update`, `actions/catalog/read` | Related database, table, and column operations with shared rules and explicit projections or mutation sequences |
| Other `actions/<domain>/<operation>` packages | Standalone boundaries where operation contracts or responsibilities differ |
| `internal/resources` | Resource adapters, exact identity resolution, and normalized provider results |
| `internal/value` | Shared typed observations and requests without behavior or provider dependencies |
| `internal/tableau` | Tableau API clients, shared HTTP transport, and inventory collectors |
| `internal/auth` | PAT resolution, native credential storage, authenticated sessions, and credential-scoped coordination |
| `internal/operationrun` | Durable local operation records, detached worker launch, and process-lifetime coordination |
| `internal/jobmonitor` | Accepted Tableau job receipts, bounded observation, and shared monitoring coordination |
| `internal/workspace`, `internal/artifact` | Named workspace registration, native packages, provenance, and dirty guards |
| `internal/cache` | SQLite cache generations, scoped observations, and local queries |
| `internal/config` | Nonsecret configuration, environment aliases, and opaque credential references |
| `internal/output`, `internal/toon` | Bounded output projections, redaction, and TOON encoding |
| `internal/capability` | Typed capability facts, validation, and command bindings |
| `internal/architecture` | Automated import-boundary checks |

## Dependency boundaries

Action packages keep operation rules near related operations, sharing a package when their responsibilities match.
Some actions use focused interfaces; others use direct function or service calls.
Choose the dependency shape that preserves clear ownership without adding forwarding layers for each command.
`internal/app` composes command dependencies, including resource adapters where remote identity resolution or provider normalization needs a distinct owner.
Actions do not import concrete resource adapters, Tableau clients, Cobra, or `net/http`.
Resource adapters do not import actions or use HTTP directly.
Tableau clients own request and response mechanics.
Shared value types in `internal/value` depend only on the standard library.
Actions and providers can share identical normalized observations and typed requests without a composition-layer field map.
Artifact lineage uses the same node, edge, and failure values while retaining its storage envelope and independent collection copies.
Raw protocol payloads, enriched inspection results, and distinct public projections remain separate.
An action result also serves as its full output when the representations match; compact output remains independently bounded.

The [architecture checker](../internal/architecture/architecture.go) defines allowed production Go imports.
Unknown local package dependencies fail the check.
Additional external-import rules keep Cobra out of action, adapter, client, and foundation layers and HTTP out of actions, adapters, and CLI plumbing.
These boundaries constrain package dependencies; they do not require an adapter or forwarding interface for every operation.

## Runtime and local state

`internal/app` resolves the configuration, workspace, authentication, and clients needed for one invocation, including supported sequential batches.
Mutation validation uses fresh state at the relevant validation boundaries.
Tableau LUIDs provide authoritative remote identity; ambiguous name and project selectors fail deterministically.
Configuration stores credential references, while approved persistent PATs reside only in the native OS credential store.
Workspaces hold managed content artifacts; the cache holds cached observations and does not replace a live source implicitly.
Persisted artifact paths are relative to the workspace and use forward slashes.

Workbook, datasource, and flow publish and pull can hand execution to a detached instance of the same binary.
This is a per-operation worker, not a resident service.
The default foreground wait ends at completion or the shared 20-minute invocation limit; `--no-wait` returns after durable handoff with one status command for the single operation or batch.
Publication receipts retain accepted Tableau job identities, while download workers save native files and metadata without inventing remote job IDs.
Local operation records live outside the source checkout and are inspected through `tadx job inspect --operation-id`.
Active transfers can hold the shared PAT lease; returning the terminal does not promise concurrent remote access with that credential.

## Capability metadata and documentation

[Typed definitions](../internal/capability/definitions.go) are the source of capability ownership, selectors, safety, evidence, implementation status, and command bindings.
The registry exposes those definitions to discovery and CLI binding checks.
`cmd/gencapdocs` generates [the capability reference](reference/capabilities.md), its [JSON data](reference/capabilities.json), and the data block in the [interactive HTML map](reference/capability-map.html).
Generated documentation is distinct from the maintained architecture diagram and these explanatory pages.

[CONTRIBUTING.md](../CONTRIBUTING.md) introduces contributions.
The repository [build skill](../.agents/skills/tadx-build/SKILL.md) contains implementation procedures and verification requirements.
