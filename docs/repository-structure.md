# Repository structure

TADX uses one Go module and one primary binary organized as a modular monolith.
Each public operation owns a small action package, while reusable behavior stays behind internal package boundaries.

## Package layout

```text
actions/
  <domain>/<verb>/       One package for each implemented public operation
cmd/
  tadx/                  Process entry point and primary binary
  gencapdocs/            Capability reference generator
docs/
  contributing/          Capability development contracts
  reference/             Generated capability documentation
internal/
  app/                   Composition root and process-level orchestration
  architecture/          Executable import-boundary checks
  artifact/              Canonical payloads, provenance, fingerprints, and dirty guards
  auth/                  PAT resolution and authenticated-session provider
  catalog/               Versioned SQLite generations, ingestion, and bounded local queries
  capability/            Executable capability registry and generation
  cli/                   Thin Cobra command and flag plumbing
  config/                Non-secret configuration and environment resolution
  errs/                  Structured errors and exit-code mapping
  identity/              Exact LUID, name, and project-path resolution
  output/                Structured rendering boundary
  toon/                  Frozen TOON codec
  workspace/             Named workspace registration and deterministic resolution
  resources/             Resource adapters added with implemented slices
  tableau/               Released Tableau API clients, shared transport, and fast catalog collectors
```

Only implemented slices create action or resource packages.
The repository does not create empty packages to reserve future names.

## Dependency direction

The `internal/app` composition root wires concrete dependencies.
All other packages depend toward narrower contracts and lower-level mechanisms.

```text
cmd/tadx -> internal/app
                 |
                 +-> internal/cli -> actions/<domain>/<verb>
                 |                         |
                 |                         +-> narrow interfaces owned by the action
                 |
                 +-> concrete adapters and renderers

actions -> internal/{capability,config,errs,identity,output}
resource adapters -> internal/{identity,tableau}
Tableau clients -> internal/{auth,tableau}
internal/tableau/catalog -> internal/tableau/catalog/tabxml
internal/output -> internal/{errs,toon}
internal/workspace -> internal/config
```

Actions declare the narrow interfaces they consume.
Resource adapters implement those interfaces without creating a shared service interface.
Foundation packages do not import Cobra, the composition root, the CLI, actions, resource adapters, or Tableau clients.

## Enforced boundaries

The architecture test scans every non-test Go file and checks every module-local import against an explicit allowlist for its source layer.
An unrecognized local import target fails, and an unrecognized source package cannot import any local package.

The local allowlists enforce these dependencies:

- Actions import only `internal/capability`, `internal/config`, `internal/errs`, `internal/identity`, and `internal/output`.
- CLI packages import only action packages, CLI subpackages, and `internal/errs`.
- Resource adapters import only `internal/identity` and Tableau client packages.
- Tableau clients import only `internal/auth`, the shared `internal/tableau` package, and the catalog client's private `tabxml` parser.
- `internal/output` imports only `internal/errs` and `internal/toon`.
- `internal/workspace` imports only `internal/config`.
- `internal/artifact` and `internal/catalog` are independent foundation packages with no higher-layer imports.
- `internal/catalog` owns one config-root `catalog/catalog.sqlite` database with immutable environment and site generations.
- The composition root imports only recognized actions, CLI packages, adapters, clients, and its required foundation packages.
- `cmd/tadx` imports only `internal/app`, and `cmd/gencapdocs` imports only `internal/capability`.

Independent external-import rules reject Cobra from actions, resource adapters, Tableau clients, and foundation packages.
They also reject `net/http` from actions and CLI plumbing.

The scanner ignores `archived/` because that directory contains obsolete reference material, not build inputs.
Run `go test ./internal/architecture` to check the boundaries locally.

## Package responsibilities

### Actions

Each action package contains one public operation with typed input and output.
An action owns orchestration, validation, and narrow dependency interfaces.
It does not contain CLI plumbing or direct HTTP behavior.

### CLI

The CLI parses arguments, invokes actions, passes results to the output layer, and maps errors to exit codes.
Each domain owns its own thin Cobra package under `internal/cli/<domain>`, and the composition root (`internal/cli/root.go` `NewRoot`) is the single place those packages are mounted and their dependencies supplied.
The CLI does not resolve Tableau identity, authenticate, call Tableau, or construct output payloads.

### Authentication and configuration

Configuration contains environment aliases, Tableau server URLs, site content URLs, workspace defaults, and PAT environment-variable references.
Configuration never contains PAT values or session tokens.
The authentication provider resolves PAT references and returns a session that authorizes requests without exposing the PAT to consumers.

### Identity

Tableau LUIDs are authoritative.
An explicit LUID takes precedence over human labels.
Otherwise, the resolver matches exact, case-sensitive names and exact slash-delimited project paths.
Zero matches and multiple authoritative LUID matches produce deterministic errors.

### Registry and generated documentation

The capability registry is the executable source of truth for ownership, selectors, safety, evidence, blockers, and command bindings.
Generated registry code and `docs/reference/capabilities.md` are marked as generated and must not receive manual edits.
Run `go generate ./...` and `go run ./cmd/gencapdocs -out docs/reference/capabilities.md` to refresh them.
