# Adding a TADX capability

This is the maintainer reference for building and integrating one capability slice.
The goal is that every capability, built by any contributor or agent, looks and behaves the same way.
A reviewer starting in one action package must understand its complete public behavior without searching the whole codebase.

This guide names the files each role may touch and gives the mechanical steps in order.
The module path is `github.com/ahillspace/tadx`.
Read `docs/repository-structure.md` for the enforced import boundaries and `docs/build-order.md` for when a slice may be built.

## Roles

The slice agent owns the assigned action package, tests, fixtures, and any explicitly assigned adapter package.
The slice agent returns integration requirements and does not edit shared integration files.

One integration owner owns the capability contract row, generated registry, implementation manifest, CLI mounting, app composition, generated reference documentation, and shared binding tests.
This keeps parallel slices independent and makes contract reconciliation a single responsibility.

Start each build from updated `main` on one new feature branch.
Agents share that branch and checkout unless the user explicitly requests worktrees.

## Mental model: how a capability becomes an executable command

There are two separate sources of truth, and the integration owner edits both.

- The capability registry describes product intent.
It is generated from a markdown table and every generated row starts as `planned`.
You never hand-edit the generated Go.
- The implementation manifest records code that actually exists.
Adding a manifest entry is what flips a capability from `planned` to `implemented` and assigns its CLI command path.

Startup validation then cross-checks the manifest against the real Cobra command tree and hard-fails on any drift.
The full chain is: contract table -> generated registry -> implementation manifest -> CLI command annotation -> startup binding validation.

## The ordered build steps

### 1. Create the action package

Each executable operation lives in its own action package under `actions/`, named by domain and verb.

```text
actions/<domain>/<verb>/
  action.go        operation-specific orchestration and validation
  types.go         typed Input and Output (and Plan/Result for mutations)
  doc.go           one-line package documentation
  action_test.go   behavior tests, written first
  testdata/        golden fixtures
```

Worked example: `actions/workbook/pull/`.

Do not create empty future packages to reserve names.
Create a package only when its real slice is implemented.
Do not create packages named `utils`, `helpers`, `common`, or `services`.

Required artifacts:

- A typed `Input` and a typed `Output`.
Never use a map as the normal action boundary.
- Narrow dependency interfaces defined by and owned by the action itself, for example the `Reader` interface `actions/workbook/pull` declares.
Never depend on one large shared service interface.
- `doc.go` package documentation describing the operation.
- Tests written before implementation.
- Golden fixtures under `testdata/` for stable rendering.

Input validation lives in the action, not in Cobra.
The Cobra layer only checks argument arity and required-flag presence, then hands a populated `Input` to the action.

### 2. Choose the action shape

Read-only operation:

```go
type Input struct { /* explicit inputs */ }
type Output struct { /* stable result, includes Help []string */ }
type Action struct { /* narrow explicit dependencies */ }

func New(dep SomeDependency) *Action
func (a *Action) Execute(ctx context.Context, input Input) (Output, error)
```

Consequential remote mutation (two visible stages plus a wrapper):

```go
func (a *Action) Plan(ctx context.Context, input Input) (Plan, error)
func (a *Action) Apply(ctx context.Context, plan Plan) (Result, error)
func (a *Action) Execute(ctx context.Context, input Input, apply bool) (Output, error)
```

`Plan` may perform the authoritative reads needed to build the preview.
`Apply` executes only the plan that `Plan` produced, and re-resolves the exact target before mutating.
`Execute(ctx, input, apply)` is the wrapper the CLI actually calls: it always plans, and applies only when `apply` is true.
See `actions/workbook/publish/action.go` for the worked example.
There is no second confirmation prompt and no production-only prompt.
`--force` never authorizes execution.

Every `Output` carries a uniform `Help []string` of concrete next-command hints.
Keep operation-specific types concrete; do not force every operation into one universal interface.

### 3. Write the fixtures and tests first

Golden fixtures use stable names by shape:

- Read-only outputs render to `testdata/output.toon`.
- Mutation previews render to `testdata/preview.toon`.

Tests must cover output shape and golden rendering, selector ambiguity, error and exit behavior, and, where applicable, secret redaction, mutation discovery, preview and apply, and artifact and provenance behavior.

### 4. Emit stable errors from the action

Use the structured error type `errs.Error` from `internal/errs`.
Populate the optional fields it defines: stable `ID`, `Operation`, selector or `Resource` identity, `Environment`, `Site`, `Summary`, `Cause`, `Retryable`, `CorrectiveAction`, `Validation`, the upstream Tableau fields, `TableauRequestID`, and `TableauJobID`.

The stable error `ID` follows the convention `<domain>.<verb>.<stage>`, for example `workbook.pull.download` or `workbook.publish.prepare`.
Use `errs.CompleteRetryAdvice` to carry retry advice from the cause and `errs.TableauRequestID` to surface the upstream request ID.

Exit codes come from `errs.ExitCode`: `0` success or no-op, `1` operation or runtime failure, `2` usage error (`errs.KindUsage`).

### 5. Integration owner: add the contract row and regenerate the registry

Add exactly one row to the capability registry table in `tadx-v1-capability-contract-final.md`.
The table has 17 columns, matching this header:

```text
Capability ID | Public command or delegated surface | Resource and user outcome | Type | V1 status | Owner | MCP overlap | Selectors | Products / availability | Local write | Remote mutation | Requires `--apply` | Safety / guard | Artifact effect | Upstream operation | Evidence | Validation / blocker
```

The capability ID is a stable dotted ID such as `workbook.pull` or `capability.list`, matching `^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*){1,2}$`.
Model the independent status axes rather than one field: product disposition (Ship or Delegated), operation type (Find, Inspect, Change, Deliver), owner, evidence, and blocker.
Local write, remote mutation, and requires-apply are independent Yes or No flags; a remote mutation must also require `--apply`.

Then run generation from the repository root:

```sh
go generate ./...
```

The `//go:generate` directive in `internal/capability/registry.go` runs `internal/capability/generate/main.go`, which parses the contract table and writes `internal/capability/registry_gen.go`.
That file is generated and marked `DO NOT EDIT`; never hand-edit it.
Every generated row is `ImplementationPlanned` and has no command binding.

### 6. Integration owner: flip planned to implemented in the manifest

Add one entry to the hand-edited `implementationManifest` map in `internal/capability/implementation.go`, keyed by the capability ID.

```go
"workbook.pull": {CommandPath: []string{"content", "workbook", "pull"}},
```

This is the single file that promotes a capability from `planned` to `implemented` and assigns its `CommandPath`.
Set `RawCapable: true` only if the capability may emit `--raw` output.

Note that the action-package domain is not the CLI command path.
`CommandPath` is the exact path a user types.
The workbook, datasource, flow, lineage, and project verbs all nest under `content`, so `actions/workbook/pull` has `CommandPath` `["content", "workbook", "pull"]` and surfaces as `tadx content workbook pull`.
This mapping is applied by `classify()` in `internal/app/app.go`: a CLI-owned capability whose ID starts with `workbook`, `datasource`, `flow`, `lineage`, or `project` is classified under the `content` domain.

### 7. Integration owner: wire the CLI command

Each CLI domain owns a thin Cobra package at `internal/cli/<domain>/command.go` (for example `internal/cli/content/command.go`).
The command that runs the action must carry the capability annotation:

```go
Annotations: map[string]string{"tadx.capability": "workbook.pull"}
```

The annotation key is the exported constant `cli.CapabilityAnnotation` in `internal/cli/root.go`.
A read-only command calls `deps.<Action>.Execute(ctx, input)`; a mutation command reads an `--apply` flag and calls `deps.<Action>.Execute(ctx, input, apply)`, and sets `Hidden: !deps.MutationsEnabled`.
Cobra code contains no Tableau behavior, identity resolution, authentication, or output construction.

### 8. Integration owner: wire the composition root

The composition root wires everything in two files; this is intentional and there is no global mutable state.

In `internal/cli/root.go`:

- Add a narrow consumer interface for the new action if one does not exist (for example `WorkbookPuller`).
- Add the corresponding fields to the `Dependencies` struct, including the `Use` and `Short` string fields.
- Add a conditional `AddCommand` for the domain in `NewRoot`, guarded on the dependency being non-nil so an unwired capability never appears.

In `internal/app/app.go`:

- Construct the action or a small service type that adapts the runtime to the action, and pass it into `cli.Dependencies` inside `Run`.
- Supply `Use` and `Short` from the registry via the `registryUse`, `registryLeafUse`, and `registryShort` helpers so command help derives from the registry.
- Adapt the resource adapter to the action-owned interface here (see `pullReader` and `publishAdapter` for the bridge pattern).

### 9. Integration owner: let startup validation prove the wiring

At startup, `Run` calls `cli.RegisteredCommands` to walk the real Cobra tree and then `capability.ValidateBindings`.
This hard-fails the process if a runnable command has no capability annotation, if an annotation or command path drifts from the manifest `CommandPath`, if a binding has no registry entry, or if an implemented registry entry has no binding.
There is no way to ship a command whose annotation, manifest path, and registry state disagree.

### 10. Integration owner: regenerate the capability reference docs

As the final step, regenerate the reference from the registry:

```sh
go run ./cmd/gencapdocs -out docs/reference/capabilities.md
```

`docs/reference/capabilities.md` is generated and must not be hand-edited.
Generated help and reference docs derive from the registry; do not maintain a second table.

## Identity

- Tableau LUID is authoritative.
Names, project paths, and local paths are selectors or labels.
- Prefer an explicit LUID; otherwise resolve an exact name and, where applicable, an exact slash-delimited project path.
- Zero matches and multiple matches are deterministic errors.
Never fuzzy-match, prompt interactively, or silently redirect after a remote rename or move.
- Every remote mutation re-resolves the exact target against Tableau, even if a cached catalog hit was used to select it.

## Output and errors

- Render through the output layer in TOON.
Never build output in Cobra.
- Follow `docs/contributing/output-guidelines.md` and classify every result field as compact or full before implementation.
- Detail-bearing actions implement an explicit compact projection and retain a bounded full typed result.
- Compact output includes `details: "--full"` only when expanded fields are available.
- Never hide mutation authorization fields, continuation state, partial outcomes, warnings, retry safety, or corrective action.
- Persist and render artifact paths relative to the resolved workspace with forward slashes.
- Resolve absolute filesystem paths only at runtime.
- Never store or emit a developer username, home directory, checkout path, or unrelated local project name.
- `--raw` is available only if the registry marks the capability raw-capable.
Secret redaction always precedes rendering.

## Evidence gate

- If the capability's contract evidence is docs-only, build only up to the adapter seam: types, validation, local orchestration, fixtures, and tests.
- Do not write live API-calling code until the exact upstream contract is captured (official endpoint, API version, request, response, and error schema, and any authoritative ID mapping) with a passing contract test.
See `docs/contributing/adding-an-adapter.md` for the adapter and contract-test build.
- A blocked capability stays registry metadata only and must never become an executable command.

## Dependency rules (the architecture test enforces these)

- Actions must not import Cobra, must not use `net/http` directly, and must not import another action.
- Cobra code contains no Tableau behavior, identity resolution, authentication logic, or output construction.
- Resource adapters must not import Cobra, CLI, or action packages.
- Only the composition root wires concrete implementations, and there is no global mutable state.
- The composition root (`internal/cli/root.go` `NewRoot` and `internal/app/app.go` `Run`) is the single place command wiring is assembled.
Each domain still owns its own Cobra package under `internal/cli/<domain>`, but the composition root is the one place those packages are mounted and their dependencies supplied.

## Completion checklist

1. The integration owner confirmed the contract row and its status axes and ownership.
2. The integration owner ran `go generate ./...` and regenerated `internal/capability/registry_gen.go` without hand editing it.
3. The `implementationManifest` entry exists with the correct `CommandPath`.
4. Behavior tests were written before implementation.
5. Selector ambiguity behavior is tested.
6. Compact and full TOON output are tested with separate golden fixtures whenever a capability has expanded detail.
7. Error and exit behavior is tested, and error IDs follow `<domain>.<verb>.<stage>`.
8. Secret redaction is tested where applicable.
9. Mutation discovery, preview, and `--apply` behavior are tested if consequential.
10. Local artifact and provenance behavior is tested where applicable.
11. The command carries its `tadx.capability` annotation and startup binding validation passes.
12. Upstream API behavior was verified against captured evidence (not docs-only) before any live wiring.
13. `go run ./cmd/gencapdocs -out docs/reference/capabilities.md` was run as the final step and committed.
14. Compact output remains bounded with many detail records, and `--full` changes presentation only.
