# Contribute one TADX action

This is the day-to-day guide for building one bounded TADX action or resource slice.
Use `$tadx-build` when you want an agent to apply the repository build workflow to an explicitly named task.

## Prepare the build

Update `main`, then create one feature branch for the build.
All agents share that branch and checkout unless the build owner requests worktrees.
Assign each agent disjoint paths and reserve shared integration files for one coordinator.

Discover the capability before reading implementation files:

```sh
git switch main
git pull --ff-only
git switch -c feat/<capability-id>

go run ./cmd/tadx capability list
go run ./cmd/tadx capability get <capability-id> --full
```

Write a compact task brief containing:

- The capability ID, required behavior, and explicit exclusions.
- The owned files and forbidden shared files.
- One closest action or adapter example.
- The compact fields, full-only fields, bounds, and next-command help.
- One bounded evidence record for remote behavior.
- The focused test commands and any separately authorized live check.

Do not read `archived/`, the full arc42, the full V1 capability contract, or all Tableau API documentation end to end.
For remote behavior, start with one relevant record under [`docs/evidence/`](docs/evidence/).
Search the local Tableau API capture for one missing detail and use current official Tableau documentation only when the local capture is insufficient.

## Build the action

Write behavior tests first and confirm they fail for the expected reason.
Create one package under `actions/<domain>/<verb>` with typed input and output, operation logic, tests, and stable TOON fixtures.
Define narrow dependency interfaces in the action package that consumes them.
Actions do not import Cobra, `net/http`, another action, or a concrete resource adapter.

Keep Cobra limited to argument parsing, action invocation, shared rendering, and exit mapping.
Put released Tableau HTTP behavior in `internal/tableau/<resource>` through the shared transport.
Put pagination and LUID-authoritative exact resolution in `internal/resources/<resource>`.
Bridge concrete adapters to action-owned interfaces only in the composition root.

If the evidence level is docs-only or blocked, build only types, validation, local orchestration, fixtures, tests, and the adapter seam.
Do not write live API code or make that capability executable until bounded evidence verifies the upstream contract.

## Preserve output and safety contracts

Default output is an explicit bounded compact TOON projection containing the status, authoritative identity, next safe decision fields, warnings, continuation state, and help.
When additional bounded details exist, compact output includes the exact top-level marker `details: "--full"` immediately before `help[]`.
`--full` is a bounded superset for the same operation and never changes requests, mutation behavior, pagination, or secret redaction.
Use separate compact and full golden fixtures for detail-bearing output.

Persist and render artifact paths relative to the resolved workspace with forward slashes.
Resolve absolute paths only at runtime and never emit machine-specific paths.
Treat Tableau LUIDs as authoritative, fail ambiguous selectors, and never fuzzy-match or prompt interactively.
Authenticate to Tableau with PATs only.
Consequential mutations preview by default and require `--apply`.
Remote mutation commands and capabilities remain discoverable when execution is disabled.
`TADX_ENABLE_MUTATIONS=1` enables mutation commands, but each command still previews by default and requires `--apply` for the remote change.
`--force` never means `--apply`.
Never persist or print PATs or session tokens.

## Integrate the capability

The slice owner returns integration requirements instead of editing shared files unless the task assigns integration ownership.
The coordinator owns the exact capability contract row, `internal/capability/implementation.go`, shared CLI mounting, app composition, generated files, and binding tests.
Never hand-edit `internal/capability/registry_gen.go` or `docs/reference/capabilities.md`.

After updating the authoritative sources, run:

```sh
go generate ./...
go run ./cmd/gencapdocs -out docs/reference/capabilities.md
```

## Verify the slice

Replace angle-bracket placeholders with the assigned packages and omit the adapter line when the task owns no adapter.

```sh
go test ./actions/<domain>/<verb> -count=1
go test ./internal/tableau/<resource> ./internal/resources/<resource> -count=1
go test ./internal/architecture -count=1

git diff --check
go vet ./...
go test ./...
```

Run live tests only when the build owner authorizes the exact disposable Tableau target and credentials are available to the process.
Keep live tests outside the standard suite, never mutate original content, and delete only disposable content created by the test lane.
Report an uncertain mutation outcome without an unsafe retry.

## Hand off for review

Report the capability ID, changed files, focused checks, integration changes, live-test status, and unresolved evidence or contract conflicts.
Stop for the build owner's manual testing and review.
Do not start a comprehensive review, review board, no-mistakes run, push, or pull request unless the build owner requests it.
