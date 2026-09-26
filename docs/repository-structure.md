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
| `actions/workbook`, `actions/datasource`, `actions/flow` | Resource packages with distinct typed lifecycle operations |
| Other `actions/<domain>/<verb>` packages | Isolated operations until separately assessed |
| `internal/resources` | Resource adapters, exact identity resolution, and normalized provider results |
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

Actions own the narrow interfaces they consume.
The composition root supplies resource adapters and local implementations through those interfaces.
Actions do not import concrete adapters, Tableau clients, Cobra, or `net/http`.
Resource adapters normalize provider behavior without importing actions or using HTTP directly.
Tableau clients own request and response mechanics.
Shared value types in `internal/value` depend only on the standard library.

The [architecture checker](../internal/architecture/architecture.go) defines the complete import allowlist for production Go files.
Unknown local package dependencies fail the check.
Additional external-import rules keep Cobra out of action, adapter, client, and foundation layers and HTTP out of actions, adapters, and CLI plumbing.
The allowlist remains authoritative as individual foundation packages evolve.

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
