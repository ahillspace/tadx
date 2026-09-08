# Guidance and CLI remediation verification

Review the feature branch against `f091482a11d0fe1bfea41454b36054f30110f2dc`.
This record describes working-tree checks before the remediation commit, not a released version.

## Scope

Pulse field eligibility and forks, cursor-free bounded discovery, project identity handling, resilient catalog reads, actionable diagnostics, environment aliases, completion installers, and bundled Guidance.
The capability map is maintainer-authored documentation, not generated runtime behavior.
Historical discoverability proposals were archived locally because their instructions conflict with the current implementation.

## Checks

The complete Go test suite, `go vet ./...`, diff checks, and isolated installer tests passed.
Installed Guidance and references were checked for Codex, Claude, and Cursor.
Native Zsh/Fish execution was unavailable; do not interpret their installer checks as native execution coverage.

Six targeted OpenCode exercises used one pinned rebuilt binary and isolated installed Guidance.
Five primary processes completed; that does not establish five unqualified goal passes.
Pulse definition creation, a timeframe fork, and user/group follower readbacks succeeded, but the full workflow and its cleanup continuation timed out.
An independent pass verified all 11 allowlisted temporary remote resource IDs absent and removed remaining local artifact copies.
Raw transcripts, credentials, runtime configuration, and private-site evidence stay outside Git.

## Limits

Unsupported agent claims and original cleanup failures remain findings, even after evaluator cleanup.
Live permission-denial hydration and nonempty Pulse filter forks were not exercised by this batch.
Historical inventory-drop totals and duplicate-search reports still need raw upstream evidence.
See [TODO.md](../../TODO.md) for remaining investigations.
The maintainer handles the comprehensive branch review; these checks are not a substitute.
