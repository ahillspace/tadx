---
name: tadx-build
description: Build one explicitly named TADX capability or slice with bounded context, repository architecture, safety gates, verification, and manual handoff. Invoke only with $tadx-build.
---

# Build one TADX slice

Apply this workflow only to the task named in the explicit invocation.
Do not infer additional capabilities, external mutations, review automation, or product decisions.

## Orchestrate the build

Start from updated `main` on one new feature branch.
Use one shared branch and checkout for all agents unless the user requests worktrees.
The coordinator assigns disjoint paths, owns shared integration files, and prevents agents from editing the same files.
Slice agents run their focused checks and return integration requirements to the coordinator.

## Load bounded context

Run `tadx capability list`, then `tadx capability get <id> --full` for each assigned capability.
Read one closest action or adapter example.
For remote behavior, read one relevant record under `docs/evidence/`.
Search the local Tableau API capture only for a missing specific detail, then use official web documentation if the local capture is insufficient.
Do not read `archived/`, the full arc42, the full capability contract, or all API documentation end to end.
If evidence is docs-only or blocked, stop at the adapter seam and do not make the capability executable.

## Build with tests first

Write the externally visible behavior tests before implementation and confirm they fail for the expected reason.
Keep one executable operation in one `actions/<domain>/<verb>` package with typed input and output.
Define narrow dependency interfaces in the action package that consumes them.
Actions never import Cobra, `net/http`, another action, or a concrete resource adapter.
Cobra parses arguments, calls the action, renders through the shared output layer, and maps exit codes.
Put released Tableau HTTP behavior in `internal/tableau/<resource>` through the shared transport.
Put pagination and exact identity resolution in `internal/resources/<resource>`.
Bridge concrete adapters to action-owned interfaces only in the composition root.

Default output is a bounded compact TOON projection.
Use `details: "--full"` when the same command has additional bounded detail.
`--full` changes presentation only and never widens pagination or changes behavior.
Persist and render artifact paths relative to the resolved workspace with forward slashes.

Treat Tableau LUIDs as authoritative, fail ambiguous selectors, and never fuzzy-match or prompt interactively.
Authenticate to Tableau with PATs only.
Consequential mutations execute by default when enabled with `TADX_ENABLE_MUTATIONS=1`; use `--preview` for a read-only plan.
`--force` does not bypass mutation policy, and mutation discovery never grants authorization.
Never persist or print PATs, session tokens, or machine-specific paths.

## Integrate without generated drift

Only the coordinator changes contract rows, the implementation manifest, shared command mounting, and app composition.
Never hand-edit `internal/capability/registry_gen.go` or `docs/reference/capabilities.md`.
After source updates, run `go generate ./...` and `go run ./cmd/gencapdocs -out docs/reference/capabilities.md`.

## Verify and hand off

Run focused action, adapter, contract, CLI, and architecture tests named by the task.
Run `git diff --check` and the required repository tests before reporting completion.
Run live tests only when the user authorizes the exact disposable target and usable credentials are present.
Never add live tests to the standard suite, mutate original test content, or delete content the build did not create.
Attempt cleanup of disposable content and report uncertain outcomes without unsafe retries.

Report changed files, focused checks, integration changes, live-test status, and unresolved evidence or contract conflicts.
Stop for the build owner's manual testing and review.
Do not start a comprehensive review, review board, no-mistakes run, push, or pull request unless the user requests it.
