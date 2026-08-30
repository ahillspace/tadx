# Action guidelines: adding one TADX capability

This is the coding contract for a single capability slice.
The goal is that every capability, built by any contributor or agent, looks and behaves the same way.
A reviewer starting in one action package must understand its complete public behavior without searching the whole codebase.

## Folder layout (one package per public operation)

Each executable operation lives in its own action package under the actions tree, named by domain and verb:

```
actions/<domain>/<verb>/
  action.go        operation-specific orchestration
  types.go         typed Input and Output (and Plan/Result for mutations)
  action_test.go   behavior tests, written first
  doc.go           package documentation
  testdata/        golden fixtures
```

Example: actions/workbook/pull/.

Do not create empty future packages to reserve names. Create a package only when its real slice is implemented.
Do not create packages named utils, helpers, common, or services.

## Required artifacts per action

- A typed Input and a typed Output. Never use a map as the normal action boundary.
- Narrow dependency interfaces, defined by and owned by the consuming action (for example a WorkbookReader the action declares), not one large shared service interface.
- doc.go package documentation describing the operation and any important behavior.
- Tests written before implementation, including selector ambiguity, output shape, error and exit behavior, and, where applicable, secret redaction, mutation discovery, preview/apply, and artifact/provenance behavior.
- Golden fixtures in testdata for stable rendering.
- Exactly one capability registry entry.

## Action shape

Read-only operation:

```
type Input struct { /* explicit inputs */ }
type Output struct { /* stable result */ }
type Action struct { /* narrow explicit dependencies */ }

func New(dep SomeDependency) *Action
func (a *Action) Execute(ctx context.Context, input Input) (Output, error)
```

Consequential remote mutation (two visible stages):

```
func (a *Action) Plan(ctx context.Context, input Input) (Plan, error)
func (a *Action) Apply(ctx context.Context, plan Plan) (Result, error)
```

Plan may perform the authoritative reads needed to build the preview.
Apply executes only the plan that Plan produced.
There is no second confirmation prompt and no production-only prompt. --force never authorizes execution.

Do not force every operation into one universal interface. Keep operation-specific types concrete.

## Dependency rules (authoritative import and layering rules; the architecture test enforces these)

- Actions must not import Cobra, must not use net/http directly, and must not import another action.
- Cobra code contains no Tableau behavior, identity resolution, authentication logic, or output construction.
- Resource adapters must not import Cobra, CLI, or action packages.
- Only the composition root wires concrete implementations; there is no global mutable state.
- Each capability domain registers its implemented commands through its own registrar function; do not centralize all wiring in one file.

## Identity

- Tableau LUID is authoritative. Names, project paths, and local paths are selectors or labels.
- Prefer an explicit LUID; otherwise resolve an exact name and, where applicable, an exact slash-delimited project path.
- Zero matches and multiple matches are deterministic errors. Never fuzzy-match, prompt interactively, or silently redirect after a remote rename or move.
- Every remote mutation re-resolves the exact target against Tableau, even if a cached catalog hit was used to select it.

## Output and errors

- Render through the output layer in TOON. Never build output in Cobra.
- --raw is available only if the registry marks the capability raw-capable. Secret redaction always precedes rendering.
- Use the structured error type with its optional fields (stable ID, operation, selector/resource identity, environment, site, summary, upstream cause, retryability, corrective action, validation details, Tableau request and job IDs).
- Exit codes: 0 success or no-op, 1 operation or runtime failure, 2 usage error.

## Evidence gate

- If the capability's contract evidence is docs-only, build only up to the adapter seam: types, validation, local orchestration, fixtures, and tests.
- Do not write live API-calling code until the exact upstream contract is captured (official endpoint, API version, request/response/error schema, any authoritative ID mapping) with a passing contract test.
- A blocked capability stays registry metadata only and must not become an executable command.

## Registry entry

- One row per public operation. Generated help and reference docs derive from the registry; do not maintain a second table.
- Model the four independent status axes rather than one field: product disposition (ship or delegated), evidence level (architecture-locked, local-contract, docs-only, live-verified), verification readiness (ready or blocked), and implementation state (planned, implemented, external/delegated).
- Local write, remote mutation, and requires-apply are independent flags.
- Registry validation must reject: duplicate IDs, duplicate implemented command paths, missing required fields, remote mutation without requires-apply, a delegated capability with a local command binding, an implemented CLI capability without a binding, a binding without a registry entry, a blocked capability marked executable, and invalid owner or blocker references.

## Completion checklist

1. Registry metadata exists and ownership is correct.
2. Behavior tests were written before implementation.
3. Selector ambiguity behavior is tested.
4. Human and TOON output are tested with golden fixtures.
5. Error and exit behavior is tested.
6. Secret redaction is tested where applicable.
7. Mutation discovery behavior is tested if mutating.
8. Preview and --apply behavior is tested if consequential.
9. Local artifact and provenance behavior is tested where applicable.
10. Any reused prototype code was reviewed for fit, quality, latency, and coupling.
11. Upstream API behavior was verified against captured evidence (not docs-only) before live wiring.
12. Generated documentation is regenerated as the final step and committed.
