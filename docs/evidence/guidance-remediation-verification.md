# Guidance and CLI remediation verification

The original remediation comparison used base commit `f091482a11d0fe1bfea41454b36054f30110f2dc`.
This historical record describes working-tree checks before the remediation commit, not a released version or a current verification run.

## Scope

Pulse field eligibility and forks, cursor-free bounded discovery, project identity handling, resilient catalog reads, actionable diagnostics, environment aliases, completion installers, and bundled Guidance.
The capability map is maintainer-authored documentation, not generated runtime behavior.
Historical discoverability proposals were archived locally because their instructions conflict with the current implementation.

## Checks

The complete Go test suite, `go vet ./...`, diff checks, and isolated installer tests passed.
Installed Guidance and references were checked for Codex, Claude, and Cursor.
Native Zsh/Fish execution was unavailable, so their installer checks did not establish native execution coverage.

Six targeted OpenCode exercises used one pinned rebuilt binary and isolated installed Guidance.
Five primary processes completed; that does not establish five unqualified goal passes.
Pulse definition creation, a timeframe fork, and user/group follower readbacks succeeded, but the full workflow and its cleanup continuation timed out.
An independent pass verified all 11 allowlisted temporary remote resource IDs absent and removed remaining local artifact copies.
Raw transcripts, credentials, runtime configuration, and private-site evidence stay outside Git.

## Limits

Unsupported agent claims and original cleanup failures remain findings, even after evaluator cleanup.
Live permission-denial hydration and nonempty Pulse filter forks were not exercised by this batch.
Historical inventory-drop totals and duplicate-search reports still need raw upstream evidence.
Remaining investigations are tracked in [TODO.md](../../TODO.md).
These checks did not constitute a comprehensive branch review.

## Follow-up to the second-pass review

The second-pass follow-up used base commit `00e5991102d3dc9d26d899513cfc21d333edc141`.
The earlier live exercises above do not establish live coverage of these subsequent changes.

Behavioral regressions cover cached project path ambiguity, incomplete admin inventories, hidden calculation comments and strings, and grouped search continuation completeness.
CLI-level tests exercise bounded live lists without usable catalog storage and environment concurrency configuration round trips.
Catalog tests cover explicit schema rebuilds, indexed project identity, atomic refresh failure, default permission exclusion, and best-effort persistence after full live reads.
Synthetic traffic tests cover configurable concurrency and shared Retry-After delays, including delays longer than 30 seconds, extension by another worker, cancellation, and numeric overflow.
These are deterministic HTTP fixtures and simulated-time tests, not a large Tableau Server load test.

The PowerShell installer regression matrix runs against isolated profiles, including Unicode paths, existing UTF BOM encodings, ANSI profiles, recoverable backups, and reinstall/uninstall.
Windows PowerShell 5.1 was available for local checks; PowerShell 7 coverage belonged to the separate GitHub CI step.
Bundled Guidance checks verify package structure, references, and CLI recipes; they do not establish blind-agent behavior or absence of interference through a live agent experiment.
No new live exercises or agent-based code reviews were run for this follow-up.

Final local gates passed: `go test ./...`, `go test -race ./...`, `go vet ./...`, focused paging race tests, formatting, module tidiness, stable regeneration, and both available installer harnesses.
GitHub CI remains the source of Linux, PowerShell 7, and cross-platform build results for the pushed commit.

## Subsequent bounded follow-up

The subsequent bounded follow-up used base commit `a24ba3990cd4c5a8797ab208ca5be8af38b0f6b7`.
Its scope covered invocation-local project hierarchy reuse, shared resource-owned filter builders, accurate catalog recovery advice, and removal of unused producer-side snapshot state.
Regressions target multiple project/datasource pages, fresh hierarchy data on a later invocation, ordinary/full-list filter parity, and executing the suggested scoped refresh before repeating a local-only read.
A CLI compatibility test preserves previously stored partial-snapshot continuation without Tableau requests.
This follow-up does not introduce persistent hierarchy caching, change mutation revalidation, or remove supported legacy snapshot readers.
No new live tests or agent-based reviews are claimed.

Focused workflow tests and affected-package race checks passed.
The hierarchy fixture covers 205 datasources over three pages and 1,001 projects over two pages: project requests decrease from six to two per discovery invocation, and a subsequent invocation observes renamed/reparented projects.
Filter parity tests cover all six inventory types, empty filters, invalid delimiters, project booleans, and datasource timestamp bounds before resource requests.
Recovery tests execute the suggested scoped refresh for all six cached-list types and an old-schema fixture, then repeat the cached read without contacting Tableau.
