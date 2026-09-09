# Command runtime efficiency

Base: `5c29aa33f856dc8a8faadf940dcdbb83f4703d37`.
The maintainer handles agent-based review; this build runs automated checks only.

## Runtime and usability

- Resolve shared configuration, workspace, authentication, and clients once per command, including sequential batches.
- Coordinate same-credential authentication across local processes with a cancelable credential-scoped lock held for the session lifetime.
- Keep credential identities isolated and keep mutation validation phases fresh.
- Complete Pulse forks from exact metric and definition read-back matching the requested saved specification, not list visibility.
- Support repeated exact datasource field selectors and datasource-scoped Pulse definition lists with truthful bounded filtering.
- Stabilize compact list columns regardless of requested limit or missing optional values.
- Synchronize installed Guidance with the maintained bundle while backing up and preserving intentional custom edits.

## Structural cleanup

- Share dependency-free value types only where identity or normalized metadata semantics are identical.
- Make typed Go capability definitions, including implementation metadata, canonical.
- Generate mechanical reference documentation and capability-map data while leaving the maintainer's HTML map unchanged.
- Keep action behavior and interfaces isolated, provider payloads separate, and qualitative Guidance handwritten.

## Verification and limits

Reproduce each behavior through the CLI or its closest HTTP/process boundary before fixing it.
Verify batches, credential-lock cancellation and isolation, exact fork reconciliation, bounded selectors, list formatting, and Cobra/registry parity.
Run focused tests, full tests, race checks, vet, formatting, and generated-output checks before pushing.
Do not add Pulse value reads, persistent sessions, background services, generic CRUD, or a new query language.
No new live Tableau tests or agent-based reviews are part of this build.

## Verification evidence

CLI HTTP fixtures verify 100 selected failures retain their order and per-item errors while sharing one sign-in.
A successful three-workbook pull writes all artifacts with one sign-in and one project inventory read, even if configuration changes during the command.
Process tests cover shared-PAT exclusion, cancellation, credential independence, failure cleanup, and opposite-order multi-credential contention.
Move fixtures verify one project hierarchy snapshot for preview and two for execution; planning, retained application, and post-upload checks use fresh phases and still reject drift.
Pulse fixtures verify exact saved specifications without inventory polling, retained-plan revalidation, filtered pagination, and unresolved-outcome evidence.
Schema and list fixtures verify exact multi-field selection, catalog isolation, missing identities, bounded output, and stable compact columns.
Generated-reference checks and Cobra binding tests verify the typed registry; cross-compilation covers Windows, Linux, and both macOS architectures.
