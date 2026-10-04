# Architecture implementation status

Status: resource and service ownership migrations are in progress; completed slices and evidence are recorded below.
Full migration and candidate gates remain incomplete.
Approval date: 2026-10-03.
Baseline: `dd22c33bd1d123066f25ec09e2d612cd95717400`.
Shared branch: `refactor/cohesive-cli`.

## Datasource schema ownership

The datasource Service owns schema validation, continuation target selection, cache/live choice, and output sequencing.
The datasource adapter owns native schema translation, metadata enrichment, deep-copy isolation, and typed schema-cache publication.
App constructs providers and no longer implements the schema workflow or copy adapter.
Test-only public schema entry points are removed; CLI help-value acceptance still invokes the public Service.

Independent review finds and rejects changed error classification for a cached continuation with an unknown environment.
A new command-level regression reproduces the difference before the corrected revision restores raw environment-selection errors.
The exact `e1aa36078337e9d66bb6ae6334462392ca592b38` merge has 16 changed paths and 1,476 files.
Its fingerprint is `7db3a38355fc4ce7028fbee3aaca84a447cd2efd9a975970e31c357a09df64dd`.
Datasource action/resource, CLI, architecture, focused app schema/lineage/workspace tests, full compilation, and scoped vet pass.
The intervening output commit leaves every schema preimage unchanged.
Evidence remains in `.tadx-refactor/schema-merge-e1a/`.
Pull, publication, and final integrated/live acceptance remain outstanding.

## Detached-operation output ownership

Output owns saved-operation snapshot projection, bounded compact/full capture, receipt-result merging, and recovery-state predicates.
The operation store exposes neutral snapshot records without depending on rendering.
Job outputs use the same neutral item record, with unchanged serialized fields.
App no longer contains copy-only projection helpers; stateful receipt reconciliation remains a pending workflow move.

Independent review rejects an unused import in the first current-build merge and passes the corrected immutable revision.
The 13-path revision is based on `e1aa36078337e9d66bb6ae6334462392ca592b38`.
Its 1,478-file fingerprint is `2a9d011810a6e7f599cfd96f47fd7f38413cee848f118bd9144cfe858ab8ae4a`.
Output, operation storage, value, job, monitoring, architecture, and focused app publication/operation tests pass, along with full compilation and scoped vet.
Tests preserve distinct accepted-versus-pending predicates, invalid saved evidence, nil values, and compact/full projection behavior.
Evidence remains in `.tadx-refactor/operation-output-revision-e1a/`.
Final integrated and live acceptance remain outstanding.

## Job service and shared observation

The job Service owns exact inspection, cancellation, and durable receipt recovery.
App constructs native readers, command-scoped authentication, storage, and CLI dependencies.
The old public projection entry points are private helpers beneath the Service.
The job-monitor Observer owns repeated authentication, exact target and job-type checks, and cancellation-independent coordination release.
Publication and explicit job recovery share this mechanism without sharing their distinct error messages or resubmitting accepted work.

Independent review passes the 16-path exact `2d160f08d04c11094ee1648e0b9e7758dcbebeb0` snapshot.
Its 1,471-file fingerprint is `9bf0bb484157b10e79aa53d2724f0e27f552fa9d9c1c79b39db85d9b212ef8ea`.
Job action, monitoring, CLI, architecture, focused app job/cross-site/publication recovery tests, full compilation, and scoped vet pass.
New tests cover validation before dependency access, preview without submission, retained cancellation acknowledgement, and per-receipt release ordering.
Evidence remains in `.tadx-refactor/job-service-2d1/`.
Detached-operation inspection remains a separate pending move from app; final integrated and live acceptance remain outstanding.

## Search routing ownership

The search action owns native-versus-dedicated routing, combined continuations, and result normalization through explicit page-reader ports.
The dedicated source declares bounded reads directly rather than probing optional runtime interfaces.
Neutral search records preserve the exact JSON shapes used in cursor fingerprints.
Pure routing tests follow their owner; command HTTP tests remain in app.

Independent review rejects the initial migrated test's invalid generic error interface and passes a corrected immutable revision.
The eight-path revision has 1,460 files and fingerprint `6d58e26ec57a0b09b2730d4f0ccb9367b2c5ba2726bad8095b6541839462712a`.
Search action, resource, value, architecture, focused app tests, full compilation, and scoped vet pass.
All preimages still match the current workspace-integrated checkout.
Evidence remains in `.tadx-refactor/search-routing-revision-9e3/`.
Cache translation, complete-list pagination, and native listers remain pending.

## Workspace ownership and integrated checkpoint

The workspace Service owns operation sequencing through explicit native ports.
Core workspace guards own root equivalence and bounded unmanaged-entry checks.
The artifact package owns ambiguity errors and confirmed partial-result projection.
App retains construction and lazy selected-configuration binding.
The selected-configuration command tests remain end-to-end; pure guard and ambiguity tests follow their owners.

Independent review passes the actual `9e3796b38c03463c75a84cfef976bd65657a4a36` merge with 15 changed paths.
Its 1,462-file fingerprint is `5e816976088e179fa85a6de87d17e31003e1d780be27c6592c8ccfc69439fd2c`.
Focused workspace, artifact, app ambiguity, CLI, and architecture tests pass.
The four shared-file merges preserve publication recording, lineage, policy, session, and content-read boundaries.
Evidence remains in `.tadx-refactor/workspace-merge-9e3/`.

The earlier exact `e16b7f08423ea521b96dd8ec88d93002b9404d0d` checkpoint passes full race tests, vet, module verification, formatting, and generation cleanliness.
Offline tooling suites pass with 24 runner, 29 live-gate, 7 capture, and 27 harness checks.
Default, reporting, and baseline-fix controls pass against 610 explicit baseline-relative changed paths.
Hosted CI run `37178194083` and gate-tooling run `37178193986` succeed, including native policy checks across Linux, Windows, and macOS.
Evidence remains in `.tadx-refactor/integrated-e16-20261003215236/`.
These checks cover that exact historical build, not later slices or final live acceptance.

## Search source-selection ownership

The search Service owns input validation, administrative prerequisites, cursor preflight, source selection, canonical target binding, and setup errors.
App constructs cached, complete-list, or native/dedicated sources through explicit provider methods.
Administrative denial and invalid continuations still reject before source construction.
Cache and live default-site rules remain distinct and unchanged.

Independent review passes for the three-path snapshot based on `df6f34c17a6e2e102c58f870a69f736038995f3a`.
Its 1,453-file fingerprint is `e81957cb539e82c2aefb9322e44d564acfddf4d30ea0328cabb574e373d7a25f`.
Focused search, app, and architecture tests, scoped vet, and full compilation pass.
The intervening lineage change does not overlap these paths.
Evidence remains in `.tadx-refactor/search-service-df6/`.
Lower routing, combined cursors, cache translation, and native listers remain pending; this is not complete search migration.

## Lineage ownership

The cohesive lineage Service owns validation, workspace selection, canonical target binding, and operation sequencing.
Its resource ports own exact cross-resource resolution, native metadata translation, artifact writing, and preview translation.
App retains construction and selected-session binding rather than those algorithms.
Preview is an explicit capability in the declared session contract, not an optional runtime writer assertion.
The revision preserves missing-preview error timing and performs no metadata capture or write during preview.

Independent review rejects the initial optional-preview design and passes its corrected immutable revision.
The final merge is based on `df6f34c17a6e2e102c58f870a69f736038995f3a` and preserves publication coordinator calls and existing architecture boundaries.
Its 30 changed paths and 1,454 files have fingerprint `cf53c9399111eb06b512653800e94644ee0c583c4b27749fefd8e735de6a75ab`.
Focused lineage, resource, architecture, app lineage/publication/job-monitor tests, and full compilation pass.
Original goldens remain byte-identical; pure adapter assertions move with their owner.
Evidence remains in `.tadx-refactor/lineage-merge-df6/`.
Final integrated and live acceptance remain outstanding.

## Accepted publication receipt ownership

`internal/jobmonitor.Publication` owns durable result recording, accepted-receipt persistence, and bounded wait coordination.
It never submits writes and receives explicit observation, session suspension, progress, deadline, and accepted-link callbacks.
Identity storage precedes linking and observation; the five-second cancellation-independent persistence window remains unchanged.
Bulk pooling, manual-only behavior, request identity, partial-error precedence, and wait-limit interpretation remain unchanged.
Typed destination verification and resource completion remain separate pending moves from app.

The first frozen draft failed compilation because two callers retained the old private wait name.
The corrected draft passes independent focused tests, compilation, and scoped vet.
The exact `e16b7f08423ea521b96dd8ec88d93002b9404d0d` merge independently preserves all current policy, session, and content-read changes.
Its ten changed paths and 1,451 files have fingerprint `dc0a1283bd988e35efc2a8b1c55794f9b5bd2195e29c7974c119712fa50b618a`.
Focused app, job-monitor, and architecture contracts pass on that merge.
New tests cover persistence before linking/notice despite cancellation and retained request identity after link failure.
The moved receipt-failure test follows its owner; end-to-end publication assertions remain in app.
Evidence remains in `.tadx-refactor/publication-receipts-merge-e16/`.
Final integrated and live acceptance remain outstanding.

## Content reads and session ownership

Workbook, datasource, and flow Services own list/inspect sequencing, while their resource adapters own typed live/cache reads and inventory publication.
Nine app operation facades and the obsolete public inspection/validation entry points are removed.
Production-used pre-resolved list operations remain available to search without reauthentication.
Canonical cursors, project filtering, cache coverage, best-effort publication, and existing projections remain unchanged.
Inspect tests now enter the Service with fake providers; invalid selectors assert no provider opening.

The session Service owns local configuration, consent, credential-readiness, and workspace aggregation.
App binds loaders and projections; neutral credential readiness lives in core auth and also serves authentication status.
Tests preserve process-variable precedence, cancellation, sorting, and output, and add selected-configuration coverage.

The combined integration snapshot is based on `6520279111b2b1046f38b328d126ac829b54bc85`.
Its 61 changed paths contain 59 previously reviewed nonmerge paths and two independently reviewed shared-file merges.
The 1,448-file fingerprint is `131efe135caa0344bb04eab4cf52e2f472fd1cf4e5a1c0052a3b5722218a355c`.
Session, auth, content, resource, architecture, CLI, and focused app tests, full compilation, scoped vet, and formatting pass.
The post-test fingerprint remains unchanged.
The shared merges preserve policy enforcement, saved-result recording, and existing inventory boundaries.
Evidence remains in `.tadx-refactor/content-session-merge-652/`.
Datasource schema, pull/publication, and final live acceptance remain separate unfinished work.

## Managed policy ownership

The cohesive `actions/policy` Service owns installation sequencing, candidate validation, sample creation, status assembly, and partial-effect errors.
Native policy protection and persistence remain in `internal/managedpolicy`.
App binds the command-scoped policy source and retains invocation prerequisite capture.
Cobra enforcement wrapping moves to `internal/cli`, which receives callbacks and does not import policy classification mechanisms.
Validation, recovery access, preview handling, exact paths, exclusive sample creation, and native installation remain unchanged.

Independent review passes for the corrected 12-path snapshot based on `d2b07d4651d6076c2ab345e54ea75164cf64ef8d`.
Its 1,434-file fingerprint is `30d7c14b7d618a17ddb1e4859fab9431292722d610d10a228c4e522684151923`.
Focused policy, app, and CLI tests, complete architecture checks, compilation, scoped vet, and changed-file formatting pass.
The initial frozen draft failed architecture checks; the corrected draft removes its forbidden CLI dependency and reconciles the approved owner edge explicitly.
The other 15 foundation assertions and new sibling/obsolete-package rejection fixtures remain enforced.
All integration preimages match after the separate cache-format correction.
Evidence remains in `.tadx-refactor/policy-owner-revision-d2b/`.
No native installation, credentials, or Tableau mutations occur in these checks.

## Integrated verification corrections

Hosted CI for `1ba1e6185ad8dee8532962e1a7beb77da0d8941e` fails its Linux and Windows formatting gate on `actions/cache/refresh_test.go`.
Build and native policy jobs pass, but the failed quality jobs are not accepted.
The cache-test formatting correction is separate from ownership changes and preserves its assertions.
Exact `ec52b79e5a124608c65a3c7eac986130b630f2bf` local gates pass from a clean archive: 12 controls and 14 seeds across the three manifests, plus the exact-revision run.
All 1,424 archive entries remain byte-identical after those checks.
Those bounded contracts do not establish final integrated or live acceptance.

## Saved-result recording ownership

`internal/lastcommand.Recorder` now owns snapshot, timestamp, prerequisite capture, unavailable tombstone, and bounded background persistence coordination.
App retains process capture, selected-store construction, deferred dispatch, and renderer binding.
`internal/output` owns the unchanged storage-warning projection.
The recorder preserves snapshot-before-clock ordering, write-error precedence, independent two-second persistence timeout, and the saved-result flag.

Independent review passes for the exact `2cf6ef1e86eb2960fe7b84a45f9f9c75d3c36e83` integration snapshot.
Its five-path, 1,428-file fingerprint is `930b8ed62c4bf54e88b3b91c58e5d846cf1269fcc9a5710a4066a6bade812f5d`.
Focused recorder, last, selected-config, managed-policy, and publication tests, compilation, architecture, formatting, and scoped vet pass.
All five integration preimages match after the independent doctor change.
Evidence remains in `.tadx-refactor/last-recorder-merge-2cf/`.
The original recorder snapshot directory was polluted by a broad test selector; its sealed ZIP remains intact, but that directory is not acceptance evidence.
The fresh integration snapshot has no source integrity leak.

## Doctor ownership

The cohesive `actions/doctor` Service owns configuration, credential-readiness, logging interpretation, and ordered check aggregation.
App binds native connectivity, cache, and workspace checks and supplies the selected configuration path lazily.
Existing check ordering, projections, and compact/full goldens remain unchanged.
The selected-configuration CLI regression covers path selection after argument parsing.

Independent review verifies all 15 changed paths against the frozen `dfdbb59846676f966deeb18a40c7429507e15492` source.
The 1,423-file fingerprint is `16a0c17b33342689599062736d2b35ea7d03d7f9b1d9514c2546775d95190936`.
Focused action, CLI, and app tests, full compilation, architecture checks, and scoped vet pass.
All changed-path preimages match the integration branch; unrelated saved-result changes remain intact.
Ignored evidence remains in `.tadx-refactor/doctor-slice-dfdb/`.
Final integrated and live acceptance remain outstanding.

## Protected saved-result ownership

The `actions/last` Service now owns saved-operation authorization, recorded prerequisites, and legacy search-row prerequisites.
App supplies a selected-store read function and the existing capability-check callback rather than a protected-read facade.
Replay still resolves the store after argument parsing, preserving the separate selected-config fix.
The legacy parser and policy ordering remain unchanged.
Storage errors retain their prior partial record; authorization errors discard the payload and retain the existing `last.unavailable` wrapper.

The reviewed six-path merge is based on `1ba1e6185ad8dee8532962e1a7beb77da0d8941e` and has 1,425 files.
Its ordinal fingerprint is `69be606efcc88c3e78374241820693e8f3956fbc524863551e20470027176a00`.
The intervening parser-fixture correction does not overlap any changed path.
Independent owner, CLI, selected-config, last-result, managed-policy, architecture, full compilation, and scoped vet checks pass.
Ignored evidence remains in `.tadx-refactor/last-reader-merge-1ba/`.
Capture/save coordination is recorded separately above; final integrated acceptance remains outstanding.

## Parser fixture isolation correction

The exact `1ba1e61` default gate passed eight controls and nine seeded rejections but failed source-integrity verification.
Its parser test supplied a relative `--config=portable/config.yaml`; the corrected saved-result behavior therefore created a result and lock inside the source snapshot.
No tracked source bytes changed, but the new files correctly rejected the run.
The failed evidence remains retained and is not relabeled as clean.
Baseline-relative gates did not run against that tainted snapshot.

A separate test-only correction supplies an absolute temporary configuration path while preserving flag order and parser assertions.
Independent review and the focused test pass, with all 1,424 snapshot files unchanged and no extra result files.
The production saved-result fix and source-integrity enforcement remain unchanged.
Ignored evidence remains in `.tadx-refactor/last-testfix-1ba/`; new integrated gates require a fresh source archive.

## Selected-configuration saved-result correction

An app.Run E2E test reproduced a pre-existing saved-result location bug for both `--config` and `--cfg`.
The command selected another configuration, but capture and replay retained a store created from the initial options before argument parsing.
The selected directory received no saved result, and replay used the default directory.
The separate fix resolves the existing store from the selected runtime configuration at save and read time.
It preserves store mechanics, snapshot bounds, authorization, legacy prerequisites, and error reporting.

The unchanged regression test now passes, including separate default/selected results, selected replay, and non-overwriting last behavior.
Independent focused last-result and managed-policy tests and full compilation also pass.
The baseline and corrected snapshots remain separate in `.tadx-refactor/last-config-baseline-b1/` and `.tadx-refactor/last-config-fix-dfd/`.
The corrected 1,424-file snapshot is based on `dfdbb59846676f966deeb18a40c7429507e15492`, changes three paths, and has ordinal fingerprint `3965e1b863af75103dfa99b8c0e205ff46c6562fb0d8c892f23e3e437d282c73`.
This correction precedes the saved-result ownership refactor and does not alter native operations or credentials.

The earlier exact `7b153c1` revision passed hosted CI `37173668908`, including Windows/Linux quality checks, three native policy platforms, and four build targets.
The exact `21b1051` revision passed hosted gate-tooling run `37174756124`; its main CI result was still pending at this record update.
Neither result establishes acceptance of the later integrated changes.

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
