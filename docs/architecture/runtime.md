# Command runtime and recovery

This page explains the shared runtime in the repository source, including detached transfers, credential coordination, and retained results.
It describes source behavior, not availability in a published release.
The [architecture audit](review.md) records the examined revision, working-tree basis, coverage, and verification limits.

Start with the [package reference](../repository-structure.md) for dependency direction.
Use the [security model](../security.md) for security limits, consent requirements, native credential backends, and managed-policy procedures.
The [output contract](../toon.md) owns encoding and presentation rules.
The [dependency and delivery guide](dependencies-and-delivery.md) covers optional release networking, installers, updates, and shipped Guidance.

## Process and command lifetime

TADX is one executable with a runtime for each command invocation.
Native publish and pull operations can start a detached instance of that executable; there is no resident TADX service.

[The process entry point](../../cmd/tadx/main.go), `main`, handles the scoped policy-install helper before normal command startup.
Normal startup creates an interrupt-aware context and starts eligible guidance and release notices.
It calls [`app.Run`](../../internal/app/app.go) with stdout, stderr, and detached publication workers enabled.
Release notices use stderr and preserve the command's exit status.

`Run` handles the private `__publication-worker` selector before constructing an ordinary runtime.
For an ordinary command, it performs these steps:

1. Resolve process defaults and create command-scoped dependencies through `newRuntime`.
2. Wire actions, resource adapters, and concrete services into [`cli.NewRoot`](../../internal/cli/root.go).
3. Compare actual command registrations with capability definitions through `RegisteredCommands` and `capability.ValidateBindings`.
4. Validate help arguments, locate the command, and validate arguments.
5. Wrap eligible transfer commands with durable dispatch, then wrap runnable commands with managed-policy checks.
6. Run the selected command through Cobra.
7. Capture and save the bounded result, render stdout, and release command-scoped resources.

The deferred result path runs before [`runtimeDependencies.Close`](../../internal/app/command_runtime.go).
Cleanup releases credential coordination and discards command-local setup state.
It does not send a Tableau sign-out request.

## Execution guards and wrapper order

The wrapper order matters because the executable and detached worker enter the same command construction path.
The following table describes the effective path after Cobra argument validation:

| Boundary | Owner | Behavior |
| --- | --- | --- |
| Capability permission | [`bindManagedPolicy`](../../internal/app/managed_policy.go) | Checks the selected capability before dispatch or inline work. |
| Managed remote-write ceiling | `bindManagedPolicy` | Checks the remote mutation ceiling unless running a permitted read-only preview. |
| Durable transfer dispatch | [`bindPublicationExecution`](../../internal/app/publication_worker.go) | Dispatches eligible non-preview operations when the executable enables workers. |
| Saved site consent | [`applyMutationExecutionPolicy`](../../internal/cli/root.go) | Checks execution consent for registry-defined remote mutations before invoking the action. |
| Operation-specific validation | Action and resource owners | Validate artifacts, exact targets, current remote state, and operation prerequisites where required. |
| Tableau authorization | Tableau API | Applies the authenticated user's permissions to the actual request. |

The mutation wrapper is created inside `NewRoot`.
The app adds durable dispatch around that callback, then adds managed policy around dispatch.
The parent therefore rejects managed-policy denials before launching a worker.
The worker reconstructs the command tree and independently checks its current policy and consent before remote execution.

[`explicitMutationPreview`](../../internal/cli/root.go) recognizes only an explicitly changed local Boolean preview flag whose value is true.
A preview bypasses execution consent only where the command supports a read-only plan.
Capability permission still applies.
Discovery, previews, and `--force` do not authorize consent changes.

[`checkManagedCapability`](../../internal/app/managed_policy.go) also checks indirect capability use and records the capabilities needed to produce a result.
Saved-result reads apply current policy to those recorded prerequisites.
See the [managed-policy guide](../managed-policy.md) for policy loading, installation, and recovery behavior.

## Setup snapshots and fresh state

[`commandRuntime`](../../internal/app/command_runtime.go) owns setup reuse within one `Run`.
It does not own a shared mutable snapshot of remote inventory or successful mutation validation.

| Setup state | Reuse key or lifetime | Important boundary |
| --- | --- | --- |
| Configuration | One load through `configuration` per command. | Configuration mutations use separate locked update transactions. |
| REST transport | Canonical server through `transport`. | Version negotiation occurs before PAT sign-in. |
| Authenticated session | Actual PAT, normalized server, and exact site through `CommandSessions`. | Cross-process locks and sign-in epochs constrain reuse. |
| Resource clients | Session and transport through `clients`. | Clients share setup without sharing mutation-validation results. |
| Workspace resolution | Selector and environment alias through `resolveWorkspace`. | This cache holds read-only setup resolution. |
| Project display paths | Session and transport through `discoveryPaths`. | Selector and path validation use the fresh project adapter. |

[`runtimeDependencies.environment`](../../internal/app/app.go) chooses read or write resolution according to the operation.
An explicit `--config` selects the configuration file for the command.
Without that override, embedding callers can supply `Options.ConfigPath`; otherwise [`UserConfigPath`](../../internal/config/config.go) uses the platform user configuration directory's `tadx/config.yaml`.
The current directory does not select a configuration file.
External callers, including the VS Code extension, inherit the host user's configuration unless they pass `--config`.
[`ResolveEnvironment`](../../internal/config/config.go) selects reads by explicit alias, saved `default_environment`, then the sole configured alias when no default exists.
An invalid selected alias fails instead of silently falling back to another profile.
[`ResolveWriteEnvironment`](../../internal/config/write_target.go) accepts an explicit alias or selects the sole configured environment.
It rejects ambiguous write selection and does not infer a target from a read default or artifact provenance.

[`config.Load`](../../internal/config/config.go) reads bounded configuration without writing migrations.
The [configuration codec](../../internal/config/codec.go), `decodeConfiguration`, separates file-level trust failures from invalid environment and workspace entries.
Unknown top-level fields and unrelated invalid entries can survive supported updates; malformed consent and unsupported configuration versions fail at file level.

[`UpdateWithRollback`](../../internal/config/config.go) and `UpdateWithPostSave` hold the configuration lock across coordinated local state changes.
They coordinate configuration persistence with external rollback or post-save work.
`InstalledError` identifies a durability failure when the prior configuration could not be reinstalled and the new configuration remains active.
It also reports whether the required external commit completed.
Post-save failures use a separate restoration path; callers must preserve restoration failures instead of assuming that every error means rollback succeeded.

## Credentials and cross-process sessions

[`PATSourceResolver.Resolve`](../../internal/auth/credentials.go) chooses a complete configured environment-variable pair before a native stored PAT.
A partial pair fails rather than combining sources.
Stored credentials must match the configured server and exact site fingerprint.
Configuration contains references, while the native credential store holds approved persistent PAT values.

[`Session`](../../internal/auth/auth.go) exposes request authorization and authoritative site and user identities without a token getter.
`signIn` rejects incomplete authenticated identities.
Credential-bearing request and response types redact formatting and omit secrets from JSON.

[`CommandSessions`](../../internal/auth/command_sessions.go) owns credential resolution, session reuse, and PAT coordination for one command.
It captures configured environment-variable values, including missing values, once per variable during that command.
It resolves each source tuple once and coordinates authenticated use by actual PAT and normalized server.

The coordination lock spans sites because another sign-in with the same PAT can invalidate earlier sessions.
Session reuse additionally requires the exact site.
`authenticate` reads the shared numeric sign-in epoch, signs in when the command's session for the site is older than the epoch, and advances the epoch after success.
A command keeps one session per PAT and site and refreshes it in place, so every client holding it sends the new token.
The epoch and lock names retain opaque credential-derived identity, not an authenticating credential.

A session signed in with a stored PAT is saved beside that PAT in the native credential store, in its own entry for the same credential reference.
The saved record holds the target fingerprint, the opaque PAT identity digest, the sign-in epoch, the token, and the site and user LUIDs.
A later command reuses it only when the target, the digest, and the epoch all match, so a replaced PAT for the same site or a newer sign-in with the same PAT for another site signs in instead.
`ReplacePAT` and `DeletePAT` also remove the saved session.
Sessions from environment-variable PATs are never saved.
Saving or loading a session never fails a command; any store failure signs in.
`SignIn`, used by `tadx auth check`, always signs in and saves the new session, because the check exists to validate the PAT.

`Session.Renew` signs in again after Tableau rejects a session, whether it expired, was superseded by a sign-in outside TADX, or was never valid.
Renewal compares the rejected token with the current one, so concurrent rejections sign in once and a request rejected with an already-renewed token resends without signing in.
It reacquires the PAT lock if the command suspended it, advances the epoch, saves the session, and rejects a sign-in that returns a different site or user.

`Suspend` releases PAT locks while retaining command-local credentials and sessions.
Later authentication checks the epoch before reusing those sessions.
Job-monitor authentication yields to queued foreground authentication and releases coordination between individual observations.
`Close` releases remaining locks and discards cached credentials and sessions.

Returning control to the terminal does not guarantee that the same PAT is available for another remote command.
An active transfer worker can still hold its coordination lease.
The [security model](../security.md#authentication-and-credentials) explains the protection limits of credentials and same-user processes.

## REST negotiation and request bounds

[`Transport.NegotiateAPIVersion`](../../internal/tableau/version.go) reads unauthenticated Server Info before sending PAT credentials.
It bounds and validates the XML response, requires PAT-compatible REST support, and selects the lower of server support and `MaximumAPIVersion`.
User settings do not choose the REST contract.
The [version negotiation contract](../evidence/rest-version-negotiation-contract.md) records the upstream evidence.

[`NewTransport`](../../internal/tableau/transport.go) copies the supplied HTTP client and clears its whole-request timeout.
The shared transport instead uses a 120-second no-progress watchdog plus caller cancellation and deadlines.
A transfer that continues making progress can exceed two minutes.
The watchdog distinguishes sending a body, waiting for response headers, and reading a body.

`Transport.Do` performs one HTTP request.
When Tableau rejects the request's session, with HTTP 401 and REST code 401002 or with no code from the Metadata, search, and Pulse APIs, it renews the session and sends the same request once more.
Tableau rejects the session before processing the request, so the rejected attempt changed nothing; the [session rejection contract](../evidence/session-rejection-contract.md) records the evidence.
A renewal failure is reported after the original rejection, and no other failure is retried.
It rejects authenticated cleartext requests, cross-origin redirects, and HTTPS downgrade redirects.
It attaches the session through `Session.Authorize`, adds a command correlation ID, and sanitizes request IDs and upstream diagnostics.
It buffers ordinary request bodies and API response bodies; the shared API response ceiling is 256 MiB, and callers can request smaller bounds.
Content downloads set `Request.OpenResponseBody`, which checks the response headers before any body byte is read and returns the destination.
A successful download body streams to that destination under the stall watchdog without a size ceiling, while an error body is buffered under a fixed 4 MiB bound and decoded.
The rejected attempt of a session renewal writes nothing, so the replayed download writes one complete body.
Multipart uploads instead stream from a fingerprint-verified snapshot copy of the artifact through [`MultipartBody`](../../internal/tableau/multipart.go), which buffers only part framing, sends an exact `Content-Length`, and can reopen the body when the HTTP client replays a request.
The API response ceiling is an implementation limit, not a promise that every command accepts a response of that size.

Transport errors expose retry advice, upstream status, request identity, and available `Retry-After` guidance.
`SubmissionAttempted` distinguishes errors reached after a request attempt from failures before submission.
Retryable transport advice does not establish that repeating a mutation is safe.
Operation owners must retain known identities and represent uncertainty about remote effects.

## Durable transfers and accepted jobs

Local operation tracking and Tableau job tracking have different identities and lifetimes:

| Record | Owner | Meaning |
| --- | --- | --- |
| Local operation | [`operationrun.Record`](../../internal/operationrun/store.go) | Tracks a detached TADX publish or pull invocation, including batches and retained results. |
| Publication receipt | [`jobmonitor.Receipt`](../../internal/jobmonitor/store.go) | Retains an exact accepted Tableau job or publication result independently of terminal output. |
| Last result | [`lastCapture`](../../internal/app/last_result.go) | Retains the most recent bounded command result for expansion. |

[`nativeLongOperation`](../../internal/app/publication_worker.go) selects workbook, datasource, and flow publish and pull operations.
Supported previews remain inline.
Embedded callers can leave workers disabled and run inline; `--no-wait` requires the detached runtime.

`beginPublicationWorker` records the request before launching the same executable with an exact local operation ID.
It snapshots bounded batch JSON rather than relying on a caller-editable batch file after handoff.
The launch inherits environment values in memory without persisting them in the operation record.
Arguments, configuration paths, working directories, batch inputs, and results remain potentially sensitive local data.

The parent waits for a bounded local startup acknowledgment.
That acknowledgment proves worker startup, not Tableau acceptance or completion.
With `--no-wait`, the parent returns after startup with the operation status command.
Otherwise, it waits until completion or the shared 20-minute invocation limit.
Interrupt or wait expiry ends foreground observation and leaves the worker responsible for its operation.

[`Store.Lease`](../../internal/operationrun/store.go) prevents competing workers and rejects replay of an already-started interrupted operation.
Records use atomic replacement, bounded JSON, revision numbers, immutable request identity, and separate worker and foreground leases.
Their phases are `requested`, `running`, `remote_pending`, `completed`, and `failed`.
An interrupted worker does not become permission to repeat the remote operation.

[`runPublicationWorker`](../../internal/app/publication_worker.go) reconstructs the command and guards through `Run`.
It saves compact and expanded results, bounded batch progress, and accepted receipt paths.
Downloads retain local operation identity without inventing Tableau job IDs.
Windows launches a hidden detached process; Unix launches a new session through the platform implementations of [`operationrun.Launch`](../../internal/operationrun/launch.go).

[`runtimeDependencies.publication`](../../internal/app/publication.go) prepares a publication receipt independently of submitting a write.
Workbook, datasource, and flow publishing are synchronous: TADX never requests a Tableau job, and the publish response carries the resulting identity.
`publication.record` saves that result and preserves it when local receipt persistence fails.
Local storage failure must not trigger resubmission to recover a receipt.

[`Monitor.Wait`](../../internal/jobmonitor/monitor.go) observes accepted identities and never submits or cancels remote work.
It stops automatic observation at the shared deadline or after three consecutive read failures while retaining the known identity.
Observation rounds coordinate by opaque credential and exact site, pool eligible due receipts, and release PAT coordination between individual reads.
Returned observations receive a separate local persistence budget when cancellation arrives at the response boundary.

[`inspectPublicationOperation`](../../internal/app/publication_status.go) reads live-worker state without authenticating or disturbing that worker.
For stopped or remote-pending work, it can perform bounded reads against saved exact receipts.
`reconcilePublicationReceipts` updates retained status from those observations while preserving partial failure and destination uncertainty.
Recovery observes known work; it does not restart the transfer or resubmit the mutation.

[`job.Inspect`](../../actions/job/inspect.go) separates inspection success from the saved transfer outcome.
Reading a failed or partially completed operation can succeed with exit code zero.
`InspectOutput.render` preserves the saved result at the document root, including an original `error` and confirmed `output` when present.
The document's operation status describes the transfer; `tracking.worker_exit_code` preserves the worker's exit code in full output.
Clients must distinguish a failed inspection from an inspected operation that failed, and retain confirmed output in either case.

## Results, partial failure, and last output

[`writerRenderer`](../../internal/app/app.go) captures action results for the shared result path.
[`RenderWithOptions`](../../internal/output/output.go) selects an explicit compact or full projection, binds contextual recovery commands, and selects TOON or JSON encoding.
`--full` expands retained details without changing acquisition limits or operation behavior.
`--json` changes encoding while preserving the chosen result contract.

[`errs.Error`](../../internal/errs/errs.go) separates failure kind from operation phase and known mutation outcome.
Phases identify validation, setup, submission, verification, or persistence.
Outcomes distinguish `not_attempted`, `not_applied`, `confirmed`, and `unknown`.
`not_applied` means Tableau received the request and definitively rejected it without applying a change.
The error can preserve confirmed completed items, a failed item, missing prerequisites, upstream details, and accepted job identity.
`ExitCode` returns zero for success, two for usage errors, and one for other failures.

[`RenderError`](../../internal/output/output.go) retains partial operation output beside its structured error when the error carries `OperationOutput`.
Consumers must inspect both the document and exit status.
Warnings and confirmed partial results do not disappear because a later phase fails.

[`lastCapture.save`](../../internal/app/last_result.go) saves a bounded expanded snapshot beside configuration before final rendering.
It also saves the capability prerequisites recorded during execution.
`last` and the session overview do not replace the last operational result.
A save failure emits a stderr warning without changing the operation's outcome.
The prior last result can then be stale, and expansion hints appear only when current result persistence succeeds.

Use durable operation IDs and job receipts to recover accepted work.
A replaceable last-result document is not the authority for whether a consequential operation should be repeated.

## Change locations and verification

Keep shared wiring in the composition root and resource-specific behavior in its established action and adapter owners.
The [contribution guide](../../CONTRIBUTING.md#validate-locally) defines the complete local validation requirements.
The following files help locate representative changes and existing regression coverage:

| Change | Implementation owners | Representative coverage |
| --- | --- | --- |
| Startup, result capture, or wiring | [`app.Run`](../../internal/app/app.go), [`main`](../../cmd/tadx/main.go) | [`last_e2e_test.go`](../../internal/app/last_e2e_test.go), [`main_notice_test.go`](../../cmd/tadx/main_notice_test.go). |
| Consent or indirect capability enforcement | [`applyMutationExecutionPolicy`](../../internal/cli/root.go), [`bindManagedPolicy`](../../internal/app/managed_policy.go) | [`managed_policy_e2e_test.go`](../../internal/app/managed_policy_e2e_test.go), including worker and preview denials; [`mutation_setting_e2e_test.go`](../../internal/app/mutation_setting_e2e_test.go). |
| PAT reuse or monitor admission | [`CommandSessions`](../../internal/auth/command_sessions.go) | [`command_sessions_cross_site_test.go`](../../internal/auth/command_sessions_cross_site_test.go), [`command_sessions_priority_test.go`](../../internal/auth/command_sessions_priority_test.go). |
| HTTP timing, redaction, or redirects | [`Transport.Do`](../../internal/tableau/transport.go), [`NegotiateAPIVersion`](../../internal/tableau/version.go) | [`transport_test.go`](../../internal/tableau/transport_test.go), [`version_test.go`](../../internal/tableau/version_test.go). |
| Detached execution or receipt recovery | [`publication_worker.go`](../../internal/app/publication_worker.go), [`publication_status.go`](../../internal/app/publication_status.go), [`operationrun.Store`](../../internal/operationrun/store.go) | [`publication_worker_e2e_test.go`](../../internal/app/publication_worker_e2e_test.go), [`publication_interrupted_e2e_test.go`](../../internal/app/publication_interrupted_e2e_test.go), [`store_test.go`](../../internal/operationrun/store_test.go). |
| Observation limits or job pooling | [`Monitor`](../../internal/jobmonitor/monitor.go) | [`monitor_test.go`](../../internal/jobmonitor/monitor_test.go), [`foreground_test.go`](../../internal/jobmonitor/foreground_test.go). |
| Output projection or error recovery | [`RenderWithOptions`](../../internal/output/output.go), [`errs.Structure`](../../internal/errs/errs.go) | [`partial_test.go`](../../internal/output/partial_test.go), [`snapshot_test.go`](../../internal/output/snapshot_test.go), [`errs_test.go`](../../internal/errs/errs_test.go). |

For changes within these shared boundaries, this focused command runs local regression coverage without intentionally enabling live Tableau tests:

```sh
go test -count=1 ./cmd/tadx ./internal/app ./internal/cli/... ./internal/auth ./internal/config ./internal/tableau ./internal/operationrun ./internal/jobmonitor ./internal/output ./internal/errs ./internal/architecture
```

It supplements the contribution checks and does not replace native-platform or privileged policy validation.
The audit's coverage record distinguishes inspected tests from tests actually run.
