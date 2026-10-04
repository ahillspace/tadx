# Follow a command through TADX

TADX is one CLI executable.
Cobra parses command syntax, an action owns the requested outcome, and injected providers perform remote or local operations.
The composition root constructs those dependencies for one invocation.
It does not own resource workflows.

## Find the owner

Start with the resource or service, not the verb.
The [repository architecture](repository-structure.md) defines ownership and dependency direction.
These traces show how those boundaries work in concrete operations and where their tests belong.
No operation needs a forwarding wrapper in every layer.

## Trace a read: project list

Start with `internal/cli/content/project.go`, then follow `actions/project/workflow.go` and `actions/project/list.go`.
The command collects input; the project Service validates it and selects the declared read contract.
The action binds continuation state to the canonical target and filters.
The resource provider resolves and normalizes project observations.

`internal/resources/project/ports.go` implements typed live ports, and `cache.go` implements cached reads.
The same package's `inventory.go` translates complete project collections and inspect observations into typed results and cache publication.
The inventory mechanism owns matching collection and cache-publication rules.
A bounded native page, a complete inventory collection, and a cached read have different contracts.
Do not merge them into a fallback that silently contacts Tableau when the caller requests cache-only behavior.

Read tests protect bounds, continuation identity, completeness, partial observations, and compact/full projections.
Start with the adjacent project list and workflow tests, then resource port tests and CLI integration tests.
When a shared inventory record changes, test its other resource consumers too.

## Trace a mutation: workbook move

Start with `internal/cli/content/workbook_mutations.go`, then follow `actions/workbook/service.go` and `actions/workbook/move.go`.
Workbook lifecycle commands and help facts live in `workbook_lifecycle.go` and `workbook_help.go` in the same CLI package.
The Service validates local input before opening the selected provider.
The workflow resolves the exact source and destination and retains fresh observations needed before writing.
Preview produces a plan without authorizing or performing the write.

`internal/resources/workbook/mutation_port.go` implements the action's native mutation boundary.
The native workbook client owns the request and acknowledgement.
The action determines the outcome from that evidence, including confirmed identity and uncertain effects.
A transport error is not permission to resubmit a possibly accepted mutation.

Tests must distinguish invalid input, refused scope, preview, identity drift, native acceptance, and failed verification.
Use action tests for sequencing, resource tests for translation, and `app.Run` with a fake HTTP server for command behavior.
Preserve request IDs and confirmed partial results even when receipt storage or follow-up verification fails.

## Trace local state: auth logout

Start with `internal/cli/auth/command.go`, then `actions/auth/logout.go` and `actions/auth/persistence.go`.
Logout removes local credential references and stored credentials; it does not revoke the Tableau PAT.
Preview reports the proposed local change without accessing stored credential values.

The auth workflow owns ordering and compensation across configuration and credential storage.
`internal/config` retains locking, replacement, and durability mechanics.
`internal/auth` retains the native credential-store boundary.
Those boundaries remain separate because each can fail independently.

Tests distinguish a failed configuration replacement, failed credential deletion, failed restoration, and uncertain directory durability.
An error from credential deletion does not prove the credential still exists or that deletion succeeded.
Output must report only the established local state and provide bounded recovery guidance.
Use fake stores for automated tests; real PAT persistence requires explicit approval.

## Add or change behavior

1. Find the existing owner and trace its direct and shared consumers.
2. Write or identify a test that fails for the user-visible regression at risk.
3. Keep the operation's input, workflow, output, and narrow dependencies with its action owner.
4. Extend substantive provider translation or native protocol behavior only where required.
5. Bind the operation through the category command and app constructor.
6. Update capability facts and regenerate marked references through their existing tooling.
7. Run affected tests, import-boundary checks, and the required integrated verification.

Do not add a second command registry, a generic workflow engine, or compatibility forwarding packages for internal moves.
Share a mechanism only when its consumers have the same invariant and execution contract.
Repeated checks remain necessary when they observe mutable state at different times.

## Give each test a purpose

Name a test for the behavior it protects, not the review phase that discovered it.
For every test, identify a concrete failure and the observable assertion that detects it.
Keep action tests beside workflows, protocol tests beside providers, and process-boundary tests in app or CLI.
Keep an assertion when its implementation moves; do not retain obsolete plumbing solely to satisfy an internal test.

Golden output protects serialization, not proof of a remote effect.
Fake HTTP tests protect requests and evidence handling, not live platform availability.
Native platform checks and authorized live tasks provide different evidence and do not replace unit tests.
Missing evidence, exclusions, and unavailable telemetry remain explicit.

## Verify a structural change

Use the repository-wide ownership map and regression gates for cross-category migrations.
Record the exact source revision and test manifest so results cannot transfer silently to a newer build.
Keep user changes and ignored experiments out of the verification snapshot.
Do not weaken a gate or change golden output to conceal a regression.

Acceptance requires preserved behavior and understandable ownership, not a target package count.
The final architecture review compares the resulting code with the approved plan and the pinned Helm and GitHub CLI references.
