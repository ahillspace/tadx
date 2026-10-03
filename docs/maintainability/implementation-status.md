# Architecture implementation status

Status: baseline corrections and deterministic authentication test synchronization committed; integrated verification continues and final live coverage needs additional fixture authority.
Approval date: 2026-10-03.
Baseline: `dd22c33bd1d123066f25ec09e2d612cd95717400`.
Shared branch: `refactor/cohesive-cli`.

## Approved scope

The maintainer approved D1-D7 in the [architecture proposal](architecture-proposal.md), including repository-wide consistency and the [exact ownership map](architecture-decision-map.json).
This session coordinates implementation and independent in-session reviews instead of separate ChatGPT reviews.
The [regression gates](regression-gates.md) still govern checkpoint and final acceptance.
No acceptance criterion is waived by the request for unattended completion.

## Baseline isolation

Updated main matches the assessed revision.
Existing user edits, untracked experiments, and ignored evidence remain untouched.
An initial checkout-wide test run encountered copied Go packages under ignored experiment directories.
Its setup and architecture-scan failures are not usable product regression evidence.
Gate execution uses a clean archive of the pinned revision outside the checkout, excluding those experiments.
Source snapshots are not additional Git worktrees.

## Completed preparation

- Clean baseline `go test -count=1 ./...` passes on Windows with Go 1.26.5.
- Clean baseline `go vet ./...` passes.
- The bounded gate runner passes 20 unit tests and independent review.
- Its baseline run passes 20 required contract tests across six groups and detects all seven seeded defects.
- Offline final live-action coverage and evidence-identity validation passes 23 synthetic tests; independent review reports no remaining material findings.
- Repository compatibility promises and bounded publication failure-state coverage are inspected below.

The G6 import-rule portion now permits 16 exact adapter/consumer pairs and exercises 41 positive and negative matrix cases.
An independent review reports no actionable findings.
Full architecture-package tests and vet pass in a fresh source snapshot containing exactly the three architecture changes, excluding user experiments and the separately pending publication tests.
That scoped snapshot has source SHA-256 `c7fe0a61c740e16e9767ee69ac5947306ec581c8a98abb043259c8533de455e0`.
This validates the replacement import rule, not completed repository-wide G6 conformance.
The reviewed checker changes are committed separately as `0866e2b530672f4a0821e24fcb4242af0b751361`.

Assessment validators pass, including 20 assessment-tool unit tests.
These results do not establish candidate platform coverage, complete gate effectiveness, or live-task completion.
The gate run reports `initial_checks_passed`, with unchanged baseline source.
Ignored local evidence is retained in `.tadx-refactor/gate-baseline-20261003/`, including the summary, source manifest, command logs, and built binary.
The tested source SHA-256 is `a2c90cb904623e9d9a6242f815e90774eef826c8d1025efc528123881e655271`.
The built binary SHA-256 is `6348e1d28b36acac44078c05b711014ee83a3510a049f1c422f6b05e1fd3ec8c`.
Native process-containment and junction rejection tests use mocked branches; they do not establish full native isolation verification.
The archived snapshot materializes two tracked Claude skill symlinks as exact link-text files, recorded by the gate; this run does not verify symlink installation behavior.

Structural workflow migration has not started.
The separately approved reporting correction changes only accepted-publication error reporting.
The architecture checker now permits the approved exact adapter-to-consumer import pairs, with positive and negative fixtures.
New publication characterization tests expose the baseline reporting failures described below.
No model tasks, Tableau mutations, credential-store operations, or changes to saved consent have run.
Read-only environment, consent, and project inspection resolved the authorized live scope.
The separately approved reporting correction has local commit `4d3b6180cce3bcb8ba715615f9e02510619f60a8`.
No pushes have occurred.
Existing unrelated work remains unmodified and unstaged.

## Compatibility and recovery preparation

Tracked documentation describes a CLI product and contains no discovered Go library API or import-path stability promise.
The module remains technically importable; repository inspection cannot establish that no external consumers exist.
The approved migration preserves CLI commands, output, errors, configuration, artifacts, and durable recovery records while changing action package paths.

Existing publication characterization protects these contracts:

| Contract | Current test owner |
| --- | --- |
| Detached return, single submission, startup uncertainty, and process diagnostics | `internal/app/publication_worker_e2e_test.go` |
| Foreground cutoff without canceling a held submission | `internal/app/publication_batch_worker_e2e_test.go` |
| Accepted identity survives interruption without resubmission | `internal/app/publication_interruption_e2e_test.go` |
| Interrupted operation remains interrupted during later inspection | `internal/app/publication_interrupted_e2e_test.go` |
| Idempotent recovery, delayed visibility, mismatches, and mixed failures | `internal/app/publication_status_recovery_e2e_test.go` |
| Partial receipt progress and explicit target mismatch | `internal/app/publication_status_test.go` |
| Confirmed synchronous effect survives receipt persistence failure | `internal/app/flow_publish_receipt_e2e_test.go` |
| Observation errors do not invent remote failure; stale persistence cannot erase confirmation | `internal/jobmonitor/monitor_test.go`, `internal/jobmonitor/store_test.go` |
| Interrupted operations cannot replay | `internal/operationrun/store_test.go` |

Before publication extraction, add targeted coverage for actual child-process death at receipt/linkage persistence boundaries, asynchronous receipt-write failure through the CLI, flow identity retention after persistence failure, and representative datasource recovery parity.
The map is bounded source inspection, not an exhaustive behavioral audit or seeded-defect demonstration.

### Newly reproduced reporting failures

`internal/app/publication_persistence_contract_test.go` reproduces workbook and datasource receipt-storage failure after native acceptance through `app.Run` and a test-owned HTTP server.
Each test observes one submission and no job polling.
The unchanged baseline retains the accepted job ID and reports a pending result, unknown outcome, and non-retry guidance.
However, it labels the local receipt-storage failure as `verification` rather than `persistence`, and the receipt-failure error omits the known acceptance request ID.
Compact success projections intentionally omit that diagnostic field and must remain unchanged.
Full output incidentally retains the ID through the result, but the shared error contract supports request IDs in both modes.
The accepted-publication requirements in `docs/evidence/phase1-rest-contract.md` and `docs/evidence/datasource-lifecycle-rest-contract.md` require errors to retain accepted job and request IDs.
Independent contract review confirms these defects rather than treating newly authored assertions as sufficient evidence.
Strict new assertions fail against the unchanged baseline; they are baseline defects, not migration regressions.
The maintainer separately approves both reporting corrections on 2026-10-03.
The correction retains accepted job and request IDs, uses the persistence phase, and preserves pending unknown outcomes and non-retry guidance.
Compact success projections, native requests, and observation order remain unchanged.
Independent review catches and resolves misleading receipt wording and a fixture that previously failed validation before reaching storage.
Focused tests pass on the final corrected source, and independent review reports no remaining findings in this scope.
The full Go suite and vet pass on the final correction in an isolated snapshot that also contains the three reviewed architecture-checker changes.
That snapshot has source SHA-256 `d1dee6cff18f3564821681b4edf17283d1095e6d2f61a5520832f78be6e26979`.
The separate reporting gate passes all three selected tests and detects both seeded reporting defects without changing source.
Ignored evidence is retained in `.tadx-refactor/gate-reporting-20261003/`.
The new datasource delayed-destination recovery case passes independently.

### Additional approved recovery corrections

New strict fixtures reproduce two further baseline defects before publication extraction.
Flow receipt-storage failure preserves the confirmed flow identity and outcome but omits the known request ID from its structured error.
Terminating a real publication worker after saving its accepted receipt but before linking it to the operation leaves operation inspection without the accepted job.
The durable receipt survives, exact-job recovery works, and the test observes no resubmission.
Termination after linkage retains the accepted job as expected.
The fixtures are `internal/app/flow_persistence_identity_contract_test.go` and `internal/app/publication_process_boundary_test.go`.
Their initial isolated source SHA-256 is `f012b14261535921f1c599faafc2d1387915d1f32a263994f296e446cf57376c`.
These are not migration regressions, and the stricter tests are not passing gates.
Independent review confirms both defects and identifies a test cleanup gap, now corrected by registering child cleanup immediately after startup.
The focused process test still fails only before linkage; the after-link control passes.
The cleanup-corrected snapshot has source SHA-256 `339feff108fd6647a611d9fa3813cda838f3dd404a35b60ab4b639a05c8c94af`.
The maintainer approves the consolidated three-defect correction scope on 2026-10-03, without broadening retries or changing native operations.
Each correction receives its own implementation, regression verification, and review before structural migration resumes.
The flow request-ID correction is committed separately as `ed1f816cae3f8da355bc282a397c22f8d560f685` after focused regression checks and independent Sol review.
The receipt-linkage recovery correction is committed separately as `99838c8d1a2f99352a77b1623330d521757a23b1` after focused clean-snapshot checks and independent Sol review.
It persists receipt ownership before submission and recovers only matching intent-bound receipts after interruption, without resubmitting publication.
Coverage includes asynchronous jobs and synchronous succeeded or unknown outcomes for workbook, datasource, and flow publication.
Unknown outcomes remain unknown, unrelated historical receipts remain excluded, and normal successful item projections stay unchanged.
Integrated checks on commit `99838c8d1a2f99352a77b1623330d521757a23b1` pass the complete Go suite, vet, formatting, module consistency, and generated-file consistency.
Selected packaging checks and all three configured defect-detection manifests pass: ten controls and twelve seeded defects.
The tested source manifest SHA-256 is `139c8a24af9222b608d40c2131a462866b41509fd8ab89dde685bc2da796610a`.
One platform-permission exploration test skips; race, hosted native checks, and live verification remain outstanding.
Coverage inspection then identifies a partial-batch ordering gap: recovering the first receipt after the second was linked reverses their output order.
A CLI regression reproduces that gap, and correction `6af94e1` restores intent order without remote requests or resubmission.
Focused clean-snapshot tests and independent Sol review pass for that correction.
Commit `12b20c1` adds independently reviewed project freshness and exact output contracts.
These later commits still require integrated verification; the earlier full-suite result does not establish their acceptance.
Subsequent exact-source verification on `acfa9a72a3046cdab5cb407948531da8d8c94c22` passes the standard Go suite, vet, formatting, module consistency, and all three defect-detection manifests.
Its source manifest SHA-256 is `7e9ef772fa7df8242f7de13bf2edf6d28b601791a79ff1ffb42430f695c7a682`.
Runner, live-validator, harness, and assessment unit suites pass with 23, 29, 27, and 20 tests respectively.
The race suite fails the existing authentication foreground-priority test under concurrent load.
Its fixed sleep does not establish that the foreground waiter registered before releasing the current holder.
Twenty focused repetitions pass, but that result does not cure the load-sensitive failure.
Independent review rejects a proposed readiness-helper substitution because a monitor's transient lock can satisfy the same probe.
That uncommitted test edit is restored; authentication production code remains unchanged and the race gate remains unresolved.
Subsequent correction `b4ed1f112f0242f66ee29c93cf938b702c553f87` uses a test-owned admission lease and foreground-specific wait signals to establish the ordering premise.
It preserves the foreground-priority assertion and changes no production authentication code.
Twenty focused race repetitions, three auth-package race runs, vet, and independent Sol review pass.
The corrected fixture still requires full integrated race verification; focused success does not retroactively pass the failed run.
The maintainer's updated model boundary excludes Astra from testing, validation, research, and judging runs.
Subsequent implementation verification and review assignments explicitly use Sol; task-driven live targets remain Luna at medium reasoning.

## Project consolidation preparation

A type-aware dry run consolidates six project action packages in a temporary source snapshot without changing the working production tree.
It maps 165 package-level objects across 44 changed or relocated files and preserves eight golden fixtures byte-for-byte.
The final dry-run snapshot passes focused project, content CLI, output, and app tests, focused vet, and repository-wide compilation.
This proves the mechanical rewrite compiles; it does not establish checkpoint acceptance or removal of redundant handoffs.
Project G1-G5 inspection identifies missing late-collision, fresh descendant-cycle, projection, and native-error propagation fixtures.
New create/move late-collision, fresh descendant-cycle, exact source LUID, and compact/full JSON/TOON projection tests pass in a clean snapshot.
Projection coverage is bounded to create and move, not all six project operations.
Strict CLI fixtures reproduce additional baseline evidence loss after an accepted project-create request.
A decoded project identity disappears when its acknowledged name differs from the request, and an interrupted acknowledgement loses the explicit unknown outcome.
Both cases correctly return nonzero and retain the request ID and non-retry guidance; they are not false-success findings.
Independent review confirms the defects and corrects the test to accept identity preservation through the shared error `resource` field.
These fault-injection cases establish behavior under malformed or interrupted acknowledgements, not evidence that live Tableau routinely returns them.
The project correction is committed separately as `8f2c9b136b1d48bde1258ce59639e1eb35b78174` after focused package and CLI tests and independent Sol review.
It preserves observed identity on malformed acceptance, reports unknown outcomes for ambiguous attempted writes, and retains explicit rejection for complete nontransient HTTP 4xx responses.
Successful native operations and output projections remain unchanged.
The typed shared consumer is `search.run` through project inventory; `cache.refresh` remains a conservative fixture compatibility scenario rather than a proven typed consumer of this package move.
A subsequent isolated project draft consolidates six packages into one service and moves native conversion ports from app to the project resource adapter.
Focused tests, compilation, and independent review find no demonstrated behavior regression in that draft.
The draft is not integrated or accepted.
Project operation facades remain in app, with substantive provider selection and warning handling still requiring extraction under D3.
Two adapter normalization tests should move to their resource owner while app composition tests remain at the CLI boundary.
The draft has no complete candidate, native-platform, or live acceptance evidence.

## Acceptance dependencies

The current executable catalog contains 124 action IDs.
The existing external benchmark harness has authored mappings for 116 IDs; mappings alone do not prove execution coverage.
The missing mappings are `job.cancel`, `job.inspect`, `job.wait`, `policy.install`, `policy.samples`, `policy.status`, `policy.validate`, and `pulse.subscription.list`.
Its saved model and frozen binary do not match the required final candidate, so both need explicit qualification.
Its Windows installer implementation is absent.

The maintainer selected a configured environment in response to the fixture-authority question.
Read-only lookup resolves an exact server and site with saved mutation consent already enabled.
Ignored `.tadx-refactor/fixture-authority.json` records the private scope and restrictions.
Live verification uses uniquely named run-owned fixtures, not existing content referenced by the previous harness configuration.
The dedicated test project is created only after the preceding candidate gates pass.
Changing saved consent or persisting credentials requires separate explicit approval.
The exploration skill does not authorize authentication, policy, installation, or ordinary configuration changes; it is not a substitute for isolated fixtures for those actions.

The existing external harness is not safe to launch unchanged for these fixtures: its default native mode bypasses its project-target guard.
Its original source remains unchanged.
An isolated preparation copy needs enforced target scope, preservation of existing exact-site consent, exact accepted binary hashes, and runtime model evidence.
Fresh preparation must also avoid its automatic discovery of nearby historical qualification runs.
The offline preparer in `scripts/refactor/harness` passes 11 hermetic tests and independent review against all 26 locked current source files.
It produces a blocked integration snapshot, enforces strict project mode, preserves independently verified exact-site consent, and pins accepted binaries without rebuilding them.
It does not launch tasks or establish live qualification.
Subsequent integration adds preserved-consent preflight, exact-owned fixture guards, and scoped project list/inspect support.
Its 21 offline tests pass against a 27-file source lock.
Review identifies and corrects uncertain setup-write adoption and incomplete read-state comparisons.
Unacknowledged setup writes remain quarantined rather than becoming cleanup targets; read comparisons include scoped content and permissions.
Live dispatch remains blocked, and these offline results do not establish live fixture or model qualification.
The latest offline preparation closes the selected runtime source set at 118 locked files and seven patches, with 27 passing hermetic tests.
Independent Sol review verifies that closure and reports no actionable findings.
Both copied entry points remain blocked until accepted build captures, immutable image identity, and actual-session model qualification are available.
This preparation does not establish actual runtime qualification or authorize bypassing preceding gates.
Commit `de823b8` adds independently reviewed prebuilt capture checks with seven passing offline tests.
Capture verifies all committed source bytes, including embedded installer scripts, rejects extra files, and requires clean native build metadata.
No accepted real capture, qualified worker image, or same-session Luna execution evidence exists yet.
The OpenAI Docs check confirms [Luna supports medium reasoning](https://developers.openai.com/api/docs/models/gpt-6-luna), but requested flags alone do not establish actual runtime configuration.
The [documented noninteractive JSON event stream](https://learn.chatgpt.com/docs/non-interactive-mode) does not establish those actual values by itself.

The offline evidence validator now supports independently reviewed, candidate-bound checkpoint action scopes.
Its 29 synthetic tests preserve mandatory final full-catalog coverage and reject relabeling checkpoint results as final acceptance.
Independent review of the checkpoint extension reports no actionable findings.

Local Linux containers are available.
The current Windows process is not elevated, and native macOS is unavailable locally.
Required native platform evidence needs hosted checks or another suitable authorized environment.
Cross-compilation and historical baseline CI do not satisfy candidate native runtime verification.

### Additional fixture authority required

The existing authority covers run-owned project fixtures, not the complete executable action catalog.
Full live acceptance additionally requires approved disposable content and jobs, site users and groups, membership and owned-content permissions, labels, Pulse resources, and upstream metadata targets.
Account and metadata fixtures need designated identities, allowed changes, and cleanup boundaries before use.
Saved site consent does not expand fixture authority.
Changing persisted consent or persisting PATs requires separate explicit permission.
Missing task mappings, installer stages, and runtime qualification are technical gaps, not substitute authority.
No broader mutations, model tasks, or pushes have occurred.
The maintainer directs continued offline implementation and verification while broader live-fixture scope remains unresolved.
A subsequent read-only consent check confirms that the previously selected site's saved mutation consent is already enabled; no setting changes occur.
No incomplete gate is waived or reported as passing.

## Completion rule

Do not report the refactor complete until all approved ownership moves, surviving behavior contracts, documentation, review, and required candidate gates have evidence.
The strict characterization tests for the three separately reviewed baseline defects are committed with their corrections and pass focused checks.
Do not describe full candidate verification as complete based on those focused checks or an earlier snapshot.
The maintainer has approved correcting the three defects as separate reviewed changes.
Rerun their strict fixtures and the complete candidate checks, and retain the original baseline evidence.
Missing authority, unavailable required checks, excluded live cases, and unresolved harmful cleanup remain explicit blockers, not passes.
