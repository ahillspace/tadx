# Build context routing

This document keeps capability builds focused and token efficient.

## Coordinator context

The coordinator starts from updated `main` on one new feature branch.
All agents share that branch and checkout unless the user explicitly requests worktrees.
The coordinator assigns disjoint files and owns shared registry, manifest, CLI root, composition root, generated documentation, and final verification changes.

The coordinator may reconcile `docs/build-order.md`, the arc42, and the full capability contract.
Those are authoritative source documents, not routine slice-agent reading.

## Build completion and review handoff

The coordinator creates each build branch from updated `main`.
Build agents run only the focused tests, contract tests, and task-level review named in their task cards.
When those checks pass, build agents report their results and stop.
Build agents and the coordinator do not start a comprehensive branch review, a review board, or the no-mistakes pipeline unless the build owner requests it.

For the first grouped build, the build owner performs manual testing after the agents stop.
The build owner then decides when to start the separate multi-agent branch review.
Review findings return to the same branch for focused fixes and verification.
The coordinator's final verification covers the required build checks only and does not authorize a comprehensive branch review.

## Slice-agent context

Give each slice agent only:

1. `AGENTS.md`.
2. A filled `docs/contributing/task-template.md` card.
3. The output of `tadx capability get <id> --full`.
4. One closest action or adapter example.
5. `docs/contributing/output-guidelines.md`.
6. One bounded evidence excerpt when remote behavior is involved.

Do not send a slice agent to read product-wide documents.
The task card contains the accepted behavior, exclusions, output fields, owned files, and verification commands.

## Shared integration files

Only the integration owner changes these files unless a task card explicitly delegates one:

- `tadx-v1-capability-contract-final.md`
- `internal/capability/registry_gen.go` through generation
- `internal/capability/implementation.go`
- `internal/cli/root.go`
- `internal/app/app.go`
- `docs/reference/capabilities.md` through generation

Agents return required integration changes in their reports.
