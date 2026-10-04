# Architecture implementation status

Status: project, catalog, auth, agent/version, capability, env/mutation, admin, content mutations, local helper ownership, and initial shared inventory changes are integrated.
Full migration and candidate gates remain incomplete.
Approval date: 2026-10-03.
Baseline: `dd22c33bd1d123066f25ec09e2d612cd95717400`.
Shared branch: `refactor/cohesive-cli`.

## Attached-label workflow ownership

One `actions/contentlabel` Service now owns list, inspect, update, and delete entry points, replacing four app operation facades.
Private production runners retain the existing attachment algorithms and distinct output contracts.
The existing native metadata client implements the narrow shared-value ports directly, without a new forwarding adapter.
Update and delete retain the indirect label-definition authorization check before target setup, including preview requests.
Existing algorithm assertions move into the owner package; new Service tests protect validation, authorization, canonical target binding, preview behavior, and setup failures.

The independently reviewed 14-path draft is based on `21b1051dea1abb1b27a160e87e38978fd13a64ad`.
Its 1,411-file ordinal fingerprint is `2ee4b9c18174c4118b9ea980f4a3479a348e9d699df832a655cd5fc65c1819ed`.
All affected baseline hashes remain unchanged through the intervening subscription and cache integrations.
Focused action, catalog/root CLI, label app E2E, architecture, full compilation, and scoped vet checks pass.
Ignored evidence remains in `.tadx-refactor/contentlabel-21b/`.
Integrated gates and live attachment acceptance remain outstanding.

## Cache service and generation ownership

Cache refresh and status now belong to one `actions/cache` Service.
Neutral generation collection, scoped publication, and persisted status facts belong to `internal/inventory`; native request transport remains in the Tableau layer.
App retains target, executor, storage, and clock construction instead of cache workflow algorithms.
Moved tests retain streaming publication, row-free receipts, persisted counts, rollback, and shared transport contracts.
All four relocated TOON goldens remain byte-identical.

Independent review rejected an initial draft that added per-request policy rechecks to refresh.
A separate corrected revision restores baseline one-time refresh preflight; admin inventory retains its existing request-level authorization.
A regression test reproduced the extra check before that draft correction and passes afterward.
The final merge preserves five overlapping admin, catalog, content, and architecture files.

The independently reviewed candidate is based on `b1ed766aa21e616eb4bcba8b99d0dac26185074b`, changes 41 paths, and contains 1,421 files.
Its ordinal fingerprint is `45d5f1783cfcc930dd67b67bb8273aa67db814bc467ab5154383f082ffe30115`.
Focused owner, inventory, native, CLI, app, and architecture checks, full compilation, and scoped vet pass.
Ignored evidence remains in `.tadx-refactor/cache-merge-b1/`; original and corrected drafts remain separate.
Integrated full gates and live cache acceptance remain outstanding.

## Pulse subscription ownership

Subscription discovery now uses one `actions/pulse/subscription` Service rather than a per-verb package and app operation facade.
The Service validates local options once, binds the canonical target and authenticated user, and records live source metadata.
The private runner retains bounded pagination, user-bound cursors, exact enrichment IDs, partial results, and native request IDs.
`internal/resources/pulse` translates native subscription and enrichment responses; app constructs the authenticated reader.
Definition, metric, and bundle ownership changes remain pending.

Independent review held the original draft because its Service and exported runner repeated local validation.
The corrected immutable revision removes that duplicate and exercises existing list assertions through the Service.
Its 1,414-file ordinal fingerprint is `5b90b151e9db299bd42d9837f538dc87369b303ab65c2dfe6f42262ad49a69a4` across 11 changed paths relative to `39b470a6b371ed0bb98db01b47aa74a567e795d3`.
All changed paths still match their baseline hashes after the intervening catalog-constructor integration.
Focused action, resource, Pulse CLI, app E2E, architecture, full compilation, and scoped vet checks pass.
The original and corrected source ZIPs remain separate in `.tadx-refactor/subscription-39b/` and `.tadx-refactor/subscription-revision-39b/`.
No live subscription case has run, and final candidate acceptance remains outstanding.

## Catalog command ownership

Catalog-mounted lineage and attached-label constructors now live in `internal/cli/catalog`, not `internal/cli/content`.
Root construction binds their dependencies independently and preserves the existing command paths, flags, capability IDs, help, preview handling, and argument errors.
This completes the constructor-location portion of D6, not the pending lineage and content-label workflow extraction.
The maintained implementation map also points to the integrated resource/service owners instead of deleted wrappers.

The independently reviewed merge is based on `39b470a6b371ed0bb98db01b47aa74a567e795d3` and changes 13 source paths.
Its 1,409-file ordinal fingerprint is `db703d8a011115364374b9c0e4bf24f4d53bb70ee785b34b8265a844e63be476`.
The two overlapping files preserve the integrated admin dependencies and assertions.
Focused command and app tests, full compilation, scoped vet, formatting, and architecture checks pass with unchanged source hashes.
Ignored evidence remains in `.tadx-refactor/catalog-cli-merge-39b/`.
The earlier 7b-based draft remains immutable with its own manifest and ZIP.
These bounded results do not establish final integrated race or live acceptance.

## Latest admin integration

The admin revision consolidates membership and permission inspection into their resource packages and adds cohesive user, group, permission, and label-definition services.
Typed user/group cached reads, inventory projections, and detail publication now belong to `internal/resources/admin`.
The neutral `internal/inventory.PublishDetail` mechanism preserves best-effort publication; app binds stores, executors, targets, and clocks.
Observed-member coverage, filtered upserts, request IDs, and unbounded cached details remain distinct from bounded output.
The exact adapter dependency edges have positive and sibling/nested rejection fixtures.
No test assertion is intentionally removed; moved membership and permission tests retain their protected contracts.

The reviewed revision is based on `7b153c14d5e74483c21d25cc7c9d56bd65c38fc5` and changes 61 paths.
Its immutable source contains 1,409 files with ordinal SHA-256 fingerprint `1f08766acacf118ae67d3019944973d09f31e4ae9198d10b0ea2bad28c7337be`.
Independent review verified all before/after hashes, owner tests, focused app cache/inventory tests, and dependency enforcement.
Author checks also include full compilation, scoped vet, and formatting.
Ignored evidence remains in `.tadx-refactor/admin-revision-7b/`.
Final integrated gates and live acceptance remain outstanding.

The preceding exact `7b153c1` candidate passed all three baseline-relative gate manifests: 12 controls and 14 seeded rejections across 422 declared paths.
Its exact-source default manifest also passed eight controls and nine seeded rejections, with all 1,389 archive entries unchanged.
These results do not establish verification of the later admin integration.

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

The first project structural candidate is integrated after the corrected baseline passes local and hosted checks.
Its offline verification is recorded below; integration is not checkpoint acceptance.
The separately approved reporting correction changes only accepted-publication error reporting.
The architecture checker now permits the approved exact adapter-to-consumer import pairs, with positive and negative fixtures.
New publication characterization tests expose the baseline reporting failures described below.
No live CLI model tasks, Tableau mutations, credential-store operations, or changes to saved consent have run.
Read-only environment, consent, and project inspection resolved the authorized live scope.
The separately approved reporting correction has local commit `4d3b6180cce3bcb8ba715615f9e02510619f60a8`.
The reviewed baseline and gate tooling are pushed to the shared feature branch; no merge or release has occurred.
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
Exact baseline `a9ccd924d83767b415f96306ae986a128eed4f45` subsequently passes the full local race suite, vet, formatting, module checks, and all three defect-detection manifests.
Its extracted source manifest SHA-256 is `3e2b077126666fa405f1dc6ebd645f72e49b12d3915de5aeffbc4b500728a8a6`, unchanged after verification.
Hosted [CI run 37156195315](https://github.com/ahillspace/tadx/actions/runs/37156195315) passes all nine jobs, including Linux and Windows standard/race checks and native policy checks on three operating systems.
Hosted [gate-tooling run 37156195258](https://github.com/ahillspace/tadx/actions/runs/37156195258) passes both operating-system jobs.
These results establish corrected-baseline offline verification, not live acceptance or verification of later structural candidates.
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
The subsequent reviewed D3 draft removes all six app project operation facades and project-specific cache and inventory conversion adapters.
One project service owns validation, provider selection, inspect write-through decisions, and mutation warnings.
CLI operations and complete search use that service; bounded live search uses the direct resource list port.
App constructs lazy providers and temporarily retains the shared inventory mechanism and shared source/coverage/error helpers.
The separately approved inventory extraction is the bounded exit condition for that remaining shared app ownership.
Two adapter normalization tests move to the resource owner; app composition assertions remain at the CLI boundary.
The exact resource-project imports of cache and readsource have positive and negative architecture fixtures under the approved D7 ownership change.
Independent Sol review passes focused action, resource, architecture, CLI, and full app package tests.
The reviewed draft preserves 38 existing action tests and eight byte-identical TOON goldens, and adds two workflow tests.
Its reviewed source fingerprint is `bbe7c8774192d7b464cc98dbb35349b0621daf8b23e68e1ade5ee25fefd5673d` against base `12b20c101550ee4fabd19f2fd474e62cbcfb296d`.
Mechanical integration verifies all 83 changed paths, preserves unrelated edits, and removes only 28 obsolete tracked per-verb files recoverable from Git.
The integrated candidate still requires its own complete gates and live verification; it is not an accepted checkpoint or repository-wide D3 completion.
Auth consolidation proceeds in a separate isolated draft, not in this project candidate.

### Integrated project candidate and subsequent review

Commit `1346e59124b769493d6b56c9b8f1bffbad261f65` passes the full local Windows race suite, vet, formatting, module consistency, and generated-file checks.
The three defect-detection manifests pass against its isolated source, including the reviewed relocation of the project identity seed.
Its 1,353-file source manifest SHA-256 is `6b9aae8abe5cb8cd547c077f879c54279bb5ce84de0f422015b291cc39e0c320`.
Hosted [CI run 37158425041](https://github.com/ahillspace/tadx/actions/runs/37158425041) passes all nine jobs.
Hosted [gate-tooling run 37158424896](https://github.com/ahillspace/tadx/actions/runs/37158424896) passes both operating-system jobs.
No live checkpoint acceptance is claimed.

Evidence inspection finds a truncated local-profile path in one emitted seeded-test log.
A separately reviewed runner correction redacts separator-bounded fragments derived from known roots and passes 24 unit tests.
It does not change raw-output assertions or source checks.
The historical log receives redaction-only postprocessing with original and replacement hashes recorded; its evidence remains labeled unclean at creation and is not acceptance evidence.
A later diagnostic creates five temporary telemetry files in the extracted source after all gate runs.
Those exact task-owned files are removed, restoring the original source manifest; this cleanup does not retroactively qualify the unclean evidence.

Subsequent architecture review identifies redundant unchanged-input validation and nested service construction in the project candidate.
An isolated follow-up removes that scaffolding while preserving target-resolution and fresh-state checks.
Independent review of the follow-up catches a first-page cursor binding regression when provider resolution canonicalizes an empty or aliased target.
Separate live and cached continuation tests reproduce the regression before its correction and pass afterward.
Independent review approves the corrected follow-up with source fingerprint `b26e3f43bfd9168516199015462ca781c8959c997f3a0d35224c1fed9507d229` for integration.
Its focused project and search checks pass, and all eight project output goldens remain byte-identical.
Mechanical integration copies exactly its 20 reviewed paths after source-fingerprint and existing-edit checks; complete integrated gates remain pending.
The committed candidate does not contain this draft regression.

Authentication consolidation is integrated after the separately committed reporting corrections; complete candidate acceptance remains pending.
The required authentication external-deletion failure followed by failed restoration reproduces a pre-existing reporting defect on both exact baseline `1346e591` and the isolated auth draft.
Fake-store CLI execution returns nonzero with the configuration reference cleared and the credential retained, but omits persistence phase, unknown outcome, and orphan recovery guidance.
Independent review confirms the state and reporting gap; no real credential store is used.
The generic joined error does not preserve whether failed restoration left the replacement configuration installed.
A separate correction receives explicit maintainer approval with regression tests and must distinguish restoration failure before replacement from failure after installation.
The independently reviewed seven-path correction preserves both failure causes and reports persistence phase with unknown outcome without changing storage ordering.
When deletion is unconfirmed and the cleared-reference configuration remains installed, guidance identifies the entry to inspect and remove if present.
If restoration installs the previous reference but directory synchronization fails, recovery guidance describes uncertain durability without claiming an orphan.
The Windows fake-store CLI fixture fails before correction and passes afterward; portable configuration branch tests and logout action tests also pass.
A subsequent independently reviewed reporting follow-up narrows both app and action classification to concrete configuration errors.
Negative fake-store tests reject unrelated errors that expose similar methods.
The follow-up also removes inherited wording that treats a deletion error as proof the credential remains.
The correction is integrated separately before auth consolidation from reviewed source fingerprint `9809a68028de6b6b3240d07afe9b427213c4702f1f3a115937adc62c6efe2483`.
Complete integrated and native-platform verification remains pending.
Catalog revision `556a1fdc5bedf7e2e0f9cdd3d2042aedbd4825a5d223a5bf061c4f31b519f56b` removes eight redundant public entry points used only by tests.
Their surviving assertions use the service boundary, and the action test count remains 62.
Independent review and focused catalog, CLI, architecture, and native-client tests pass.
Mechanical integration verifies the complete draft fingerprint and all 68 changed paths against their baseline, then copies 43 files and removes 25 obsolete tracked files.
Those deletions remain recoverable from Git; the combined candidate still requires integrated gates.
The initial shared inventory extraction is integrated as described below; typed readers and full-generation refresh remain later ownership moves.

### Agent and version consolidation

The independently reviewed agent/version draft consolidates install/uninstall and version/get into their service owners.
Version consumes the existing release observation directly, removing the app release-copy adapter.
The version mechanism import exception permits only the exact action package, with a nested-package rejection fixture.
Focused action, CLI, app, and architecture tests pass; both agent TOON goldens remain byte-identical.
Mechanical integration checks the before and after hashes for all 17 entries, covering eight relocations and nine modified files.
The reviewed source fingerprint is `56e9c205b5436a5e871f744f9b0e80f6652d300204dfa74681c44f26372b7f2e` against base `3c2ac9f`.
Integrated gates and live installer verification remain pending.

### Authentication consolidation

One auth service owns check, login preflight/login, logout, status, and cross-store workflow coordination.
Core auth and config retain session, native credential-store, locking, and replacement mechanisms.
The migration retains both approved restoration-reporting corrections and the real configuration and Windows fake-store regression fixtures.
Three output goldens remain byte-identical, and the exact action-to-core-auth import edge has negative fixtures.
Final independent review verifies all 44 changed paths against base `75d02a0`, with source fingerprint `d03c4b2be148e2a054bc01353fdf52e9bc52a3cc9530d6ae0822fda556b5e46a`.
Full compilation and focused CLI root tests pass on that source.
Mechanical integration copies 32 reviewed files and removes 12 obsolete tracked paths, all recoverable from Git.

An earlier integration check detects a stale root CLI test overlap with the agent/version migration.
The coordinator restores 12 task-owned deletions after a failed precheck; no user files change and no stale replacement is retained.
The final manifest records every actual-base before/after hash and preserves the version alias assertions alongside the auth changes.
An earlier gate run rejects two Python cache files created during review; they are quarantined outside the source, and the rejected evidence remains recorded.
Subsequent checks disable Python bytecode generation and use explicit ordinal fingerprint ordering.
The clean pre-overlap-correction auth gate run passes eight controls and detects nine seeded defects, but belongs to its earlier source fingerprint.
The integrated candidate must rerun those gates; historical results are not attributed to the final merged source.
The expanded initial manifest adds two portable restoration-reporting controls and two seeded defects without weakening existing assertions.

### Shared inventory collection

The independently reviewed 14-path inventory slice moves neutral collection, row projection, completeness, and scoped cache publication to `internal/inventory`.
Six app callers use it directly; filtered upsert and unfiltered scope replacement retain their different semantics.
Shared source and warning facts remain separate from each action's output binding.
The reviewed source fingerprint is `dad18c267b0045f15707f69c43c909fc21bff6fca3566423020dc87d8de6544a` against base `2010844`.
The architecture merge receives a separate independent check and retains the exact project, version, and auth exceptions.
Its new inventory fixtures exercise five permitted and eleven rejected dependencies.
Focused inventory, architecture, and app tests pass; existing CLI tests and moved pure assertions survive.
Mechanical integration copies 13 unchanged reviewed paths and the checked architecture merge, with no deletions.
Typed page readers, cache-only reads, full-generation refresh, and search routing remain explicitly pending; this slice is not full D4 completion.
Complete integrated gates remain pending.

## Content mutation integration

The independently reviewed content mutation candidate starts from exact commit `1a9ee346696d4e5f1da19fce23e1022f607d01dc`.
Its 1,380-file source fingerprint is `10654c6784c8b0712dfba609fe27466d7a7251ee91f4e84441bf6179bf597072`.
Integration verifies actual-base and candidate hashes for all 49 changed paths before copying 46 paths and removing three obsolete app files.
The removed files remain recoverable from Git; their behavior assertions move to the owning action and resource tests.
Workbook, datasource, and flow Services own mutation validation, canonical target binding, and move, update, and delete sequencing.
Direct resource ports replace nine app operation facades and copy-only adapters while preserving fresh prewrite checks, request IDs, and partial evidence.
The two overlapping app files preserve the previously integrated auth and inventory changes.
Workbook deletion rejects whitespace-only environments before opening a provider and still binds an omitted site to the selected canonical site.
Independent review checks the complete manifest, overlap diffs, focused behavior tests, and CLI wiring.
Full compile, affected package tests, focused app and CLI tests, and scoped vet pass on the isolated candidate.
Integrated full gates remain pending.
Publication preparation, content reads, cache routing, pull, and publication recovery remain separate unfinished ownership moves.

## Integrated verification update

### Capability consolidation

The capability Service combines named get and list operations with registry discovery and injected policy and site-readiness observations.
The app discovery facade and both verb-only action packages are removed; the CLI retains parsing, rendering, and partial-result wrapping.
The exact `049406f` integration candidate has source fingerprint `4dec0b70ca6e6a09aaf1ca2ef9cf6d7c27b4e16d458f520ee1c51e8500e45d96` across 1,383 files.
Independent review verifies all 26 actual-base and candidate path hashes and the single overlapping app constructor merge.
Both moved output goldens remain byte-identical.
A new test catches a draft-only regression where a policy resolver returning both true and an error could enable remote rows; the corrected draft preserves baseline fail-closed behavior.
Full compilation, focused action, registry, CLI, output, architecture, and app tests, and scoped vet pass on the isolated candidate.
Mechanical integration preserves the previously integrated auth and content wiring.
Integrated full gates remain pending.

### Hosted and local checks

Historical commit `f23b59560370f13009ff33e04ac4092d59fceee2` passes the full local race suite, vet, module and generated-file checks, tooling suites, and 12 controls plus 14 seeded-defect checks.
Its formatting check fails on one catalog help fixture; commit `1a9ee346696d4e5f1da19fce23e1022f607d01dc` corrects that fixture.
Exact `1a9ee3` passes formatting across 1,102 Go files and the controls and seeded-defect checks with unchanged source fingerprints.
Those results do not transfer to later commits.
Hosted Windows standard tests on `1a9ee3` fail `TestCachedGroupMembersRequireObservedCoverageThroughCLI`: three cached-member output modes return `cache.uninitialized` unexpectedly.
The failure remains under investigation; later steps skipped by that job are not passes.
Exact `1a9ee3` hosted Linux quality gates complete successfully, including standard and race tests, fuzz checks, module and generated-file checks, and installer and packaging checks.
All four build jobs and three native managed-policy jobs also pass, but the overall run fails because of the Windows test failure.
An isolated diagnostic omits best-effort cache publication and reproduces the hosted test symptom.
This demonstrates the fixture's dependency, not the precise reason that hosted publication failed.
A test-only correction explicitly seeds observed and unobserved member-coverage states while retaining live CLI calls and the existing assertions.
Independent review accepts that correction, and commit `e050e18` integrates it separately without production changes.
Forced-omission and normal-source focused tests pass ten repetitions; separate tests retain admin-detail and workbook write-through coverage.
The original diagnostic log required post-capture local-path sanitization; its failed result and original/revised hashes remain recorded rather than claiming clean creation.
The pushed `e050e18` batch includes content mutations and capability discovery and requires its own full candidate checks.
Exact `e050e18fcd2af0ff94a9c76bb191757787032427` subsequently passes full local race tests, vet, formatting across 1,115 Go files, module/generated-file checks, tooling suites, and assessment validators.
All three baseline-relative manifests pass 12 controls and 14 seeded defects across 354 declared changed paths; its exact-revision default manifest passes eight controls and nine seeds.
The 1,383-file ordinal source fingerprint remains `4fc1455b293bc0e9ea10817cf8b9ff26c932cae294bbac985c54589c1ee2f04f` before and after checks.
The runner's source digest is `b33ced1c6c3ec05f670d4ef423717cd3046bcb50d330cbb428cc54ac526ff386` under its separately defined manifest format.
Hosted CI run `37170991864` and gate-tooling run `37170991768` both pass on exact `e050e18`, including Windows quality gates and the previously failing fixture.
These results do not transfer to the newer local-services integration.
No integrated checkpoint is accepted.

## Environment, consent, and local helper ownership

Independent review rejects the first isolated env/mutation consolidation despite its focused tests and compile passing.
The draft captures the configuration path during construction, before Cobra parses the selected configuration flag.
A shared capability CLI regression passes on the baseline and fails on that draft, demonstrating lost selected-site consent behavior.
Review also finds four unintended help-text changes from mechanical renaming.
Both issues require correction and selected-configuration positive and inverse-denial tests before integration.
No remote task uses this draft.
The author subsequently modifies the initial frozen sources in place; their first manifests remain historical records and no longer describe the current bytes.
Those handoffs are superseded, not accepted evidence.
Revisions use new source directories and retained ZIP archives with complete fresh hashes before independent re-review.

The corrected env/mutation revision passes independent re-review, including the original failing CLI test and new selected-config positive and inverse-denial cases.
Its exact-`e050e18` candidate has 67 paths and source fingerprint `ba3d9e908c0ab1345af77e61c32dde99c1161c5254b4f6ee21c322b7f22cf38b`.
One env Service owns six named operations and configuration coordination; one mutation Service owns status, set, and selected-site policy observations.
Configuration retains canonical consent storage, locking, and persistence mechanisms.
Ten relocated output goldens remain byte-identical; 32 env action tests and three moved app store tests retain their assertions.

Two separately reviewed local-helper moves put configured PAT-variable enumeration in config and terminal credential prompting in the auth CLI package.
The updater callback reads the selected path at invocation time; terminal construction remains lazy and retains the existing injected-prompter path.
The enumeration test moves with its owner, while the CLI flag-selected child-environment regression remains at the app boundary.
Baseline and candidate verification reruns use the gate runner's sanitized subprocess environment; earlier unsanitized focused runs are excluded from assurance.
Fake prompters do not establish native terminal secret-echo behavior, which remains a verification limit rather than a new claim.

Independent integration review verifies the combined 73-path candidate and both constructor edits in the sole overlapping app file.
Its 1,389-file ordinal fingerprint is `a9099a577cadd7eaf3d989d567d47f429adecf96929030c65426280420efe4c5`.
Its retained ZIP has SHA-256 `c5113267762557735be9ede9aea11dfb447942d81a697f56dbfd96cffd7f536f`.
All other paths match their independently reviewed inputs exactly.
Clean-environment focused tests, full compile, scoped vet, architecture checks, and formatting pass with unchanged source.
Mechanical integration checks every actual-base and candidate path hash, copies 44 files, and removes 29 obsolete paths whose behavior and assertions survive under the new owners.
Removed files remain recoverable from Git.
Full integrated and live gates remain pending for this new source.

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
Two bounded networked text-only setup probes use the pinned runtime with no TADX, repository, or Tableau access.
Both fail before completion and establish no successful model access or live verification.
Same-thread rollout records expose provider, model, and effort as effective per-turn configuration; requested settings alone remain insufficient evidence.
The final probe reports an availability/access-related phrase without a retained structured code or HTTP status; the precise cause is unproven.
Subsequent diagnostics improve error capture through six synthetic tests and independent review before one additional bounded probe.
The sanitized, same-thread result establishes that the pinned Codex 0.154.0 ChatGPT-authenticated route rejects `gpt-6-luna` with HTTP 400 and `invalid_request_error`.
This establishes failure of that route, not universal model unavailability or successful inference.
The requested model and medium effort appear in effective rollout configuration, but the task never completes.
A separately isolated current-client setup diagnostic is authorized; no model substitution or live acceptance follows from this authorization.
That single Codex 0.160.0 diagnostic completes successfully with exit zero and a completed turn.
Its same-thread rollout records provider `openai`, model `gpt-6-luna`, and effort `medium`.
The verified disposable image runs a static text-only request with read-only root, no host mounts, and authentication confined to tmpfs.
Its container and derived image are removed afterward; the original pinned image remains unchanged.
The sanitized result SHA-256 is `ebb5e1874141d25cdab2ddebd18175625a68b21402f49d67c1ff4039296e24e7`.
This qualifies an available setup route, not a final worker image, TADX build, Tableau task, or G9 outcome.
Final worker preparation must pin the current client and retain actual-session model evidence again.
The OpenAI Docs check confirms [Luna supports medium reasoning](https://developers.openai.com/api/docs/models/gpt-6-luna), but requested flags alone do not establish actual runtime configuration.
The [documented noninteractive JSON event stream](https://learn.chatgpt.com/docs/non-interactive-mode) does not establish those actual values by itself.

The offline evidence validator now supports independently reviewed, candidate-bound checkpoint action scopes.
Its 29 synthetic tests preserve mandatory final full-catalog coverage and reject relabeling checkpoint results as final acceptance.
Independent review of the checkpoint extension reports no actionable findings.

Local Linux containers are available.
The current Windows process is not elevated, and native macOS is unavailable locally.
Required native platform evidence needs hosted checks or another suitable authorized environment.
Cross-compilation and historical baseline CI do not satisfy candidate native runtime verification.

### Additional fixture authority

The initial authority covers run-owned project fixtures, not the complete executable action catalog.
The maintainer subsequently explicitly approves changes across the additional fixture categories requested for the selected site.
That approval covers disposable content and jobs, site users and groups, membership and owned-content permissions, labels, Pulse resources, and upstream metadata targets.
Prefer run-owned resources; any necessary existing target needs an exact identity, recorded initial state, bounded changes, and restoration plan before use.
Fixture preparation must still establish usable identities and cleanup evidence; authorization alone does not demonstrate runtime readiness or passed coverage.
Saved site consent does not expand fixture authority.
Changing persisted consent or persisting PATs requires separate explicit permission.
Missing task mappings, installer stages, and runtime qualification are technical gaps, not substitute authority.
No Tableau mutations or live CLI model tasks have occurred; the recorded text-only setup diagnostics do not count as CLI tasks.
The maintainer directs continued implementation and verification; broader fixture authority is now recorded, while runtime qualification and candidate gates remain unresolved.
A subsequent read-only consent check confirms that the previously selected site's saved mutation consent is already enabled; no setting changes occur.
No incomplete gate is waived or reported as passing.

## Completion rule

The maintainer additionally requires a final independent whole-repository architecture review after implementation.
Compare the resulting code with every approved D1-D7 decision and the pinned Helm and GitHub CLI references from the proposal.
Assess cross-category consistency, contributor navigation, redundant handoffs and records, workflow ownership, test purpose, and maintainability.
Report where TADX matches the reference principles, where its differences are justified, and where material gaps remain, with concrete source evidence.
Give an explicit overall comparison rather than treating green tests or package counts as architectural acceptance.
This review supplements G8 and does not replace any other required gate.

Do not report the refactor complete until all approved ownership moves, surviving behavior contracts, documentation, review, and required candidate gates have evidence.
The strict characterization tests for the three separately reviewed baseline defects are committed with their corrections and pass focused checks.
Do not describe full candidate verification as complete based on those focused checks or an earlier snapshot.
The maintainer has approved correcting the three defects as separate reviewed changes.
Rerun their strict fixtures and the complete candidate checks, and retain the original baseline evidence.
Missing authority, unavailable required checks, excluded live cases, and unresolved harmful cleanup remain explicit blockers, not passes.
