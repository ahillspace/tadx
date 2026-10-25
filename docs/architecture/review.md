# Architecture review record

This record captures the CLI architecture audit and the subsequent extension integration refresh on 2026-10-22.
Resolution notes dated 2026-10-25 record the later authorized extension release and setup work.
It separates examined source behavior, documentation repairs, proposed simplifications, and verification limits.
Start with the [architecture guide](README.md) for the current-system explanation.

## Examined baseline

The repository is `ahillspace/tadx`, on branch `feat/vscode-artifact-explorer` at base commit `8976a2b42f71c6ec586b64f7dbcd760bb2a27b0f`.
The working tree already contained modified source and tests, local proposals, and untracked development files.
This review examines that working tree rather than attributing all observed behavior to the committed revision.
Existing edits remain in place; the audit does not commit, push, change Tableau state, or modify application behavior.
Relative source links identify the maintained owners and symbols without creating misleading commit permalinks for dirty files.

The review inventories 115 Go packages under the maintained `actions`, `cmd`, `internal`, and `scripts` roots.
The command used is:

```sh
go list ./actions/... ./cmd/... ./internal/... ./scripts
```

A recursive `go list ./...` also discovers local source snapshots and third-party examples in this checkout.
Those are not additional shipped TADX components.
Archives, downloaded content, private captures, local configuration, credentials, dependency directories, and generated build output are excluded from current architecture evidence.
The untracked editor extension receives a separate source review because it is relevant development work outside the established CLI build.
The integration refresh follows the extension build and traces activation, inventory, previews, execution, receipts, UI messages, filesystem actions, and packaging.
It changes documentation only; the earlier, separately authorized checker repair is recorded below as historical work.

## Assessment

The implemented design has clear primary boundaries: Cobra plumbing, composition wiring, resource actions, provider normalization, and Tableau transport.
Local artifacts, observed cache data, accepted jobs, detached operations, and last output have distinct owners and recovery contracts.
The import checker enforces dependency direction, and capability registration connects actual commands to maintained ownership facts.
Similar command verbs do not imply identical mutation sequences or justify a generic workflow runner.

The main documentation gap was the absence of a connected explanation spanning these boundaries.
The existing package reference, security guide, search map, output contract, and upstream evidence were already useful and remain authoritative.
The audit adds runtime, resource/state, and dependency/delivery guides and connects human and agent entry points to them.
The preserved HTML pull example now identifies its logical action scope and links to the worker-aware runtime explanation.

No broad rewrite or reproduced authorization defect follows from this architecture review.
The extension refresh finds a concrete status-inspection contract mismatch and an inventory consistency limitation, demonstrated through synthetic CLI-boundary probes.
It also identifies missing real-CLI compatibility tests and an incomplete extension distribution workflow.
The 2026-10-25 release work resolves the inspection parser and compatibility/delivery findings below; cache-generation consistency remains open.
The project-resolution simplification remains supported, but the earlier account omitted a production dependency-acquisition caller of the private fallback.
Larger execution and retained-state changes overlap existing proposals and require separate design and implementation work.

## Coverage ledger

`Traced` means verified through implementations, callers, and meaningful alternative or failure paths in the stated source scope.
`Mapped` means partly verified through ownership and representative consumers without a complete behavioral trace of every operation.
Neither status means live Tableau confirmation, exhaustive correctness review, or that every adjacent test ran.
Validation results appear separately below.

| Area | Evidence and examined contract | Status | Canonical explanation and remaining limit |
| --- | --- | --- | --- |
| Process and composition | [`main`](../../cmd/tadx/main.go), [`app.Run`](../../internal/app/app.go), [`commandRuntime`](../../internal/app/command_runtime.go); startup, command binding, setup reuse, and cleanup | Traced | [Runtime](runtime.md); embedded callers can choose inline execution |
| CLI, capabilities, and help | [`cli.NewRoot`](../../internal/cli/root.go), [`capability.ValidateBindings`](../../internal/capability/registry.go), help renderers, shorthand, and registry definitions | Traced | [Command structure](../command-structure.md), [package map](../repository-structure.md); individual flags remain owned by executable help |
| Configuration and target selection | [`config.Load`](../../internal/config/config.go), codec, write selection, locked updates, and workspace resolution | Traced | [Runtime](runtime.md), [workspaces](../workspaces.md); no private configuration inspected |
| PAT lifecycle and sessions | [`PATSourceResolver`](../../internal/auth/credentials.go), [`CommandSessions`](../../internal/auth/command_sessions.go); source precedence, locks, site identity, and sign-in epochs | Traced | [Runtime](runtime.md), [security](../security.md); native backends and live authentication not exercised |
| Policy and consent | [`bindManagedPolicy`](../../internal/app/managed_policy.go), CLI mutation guards, worker re-entry, and managed-policy protection | Traced | [Runtime](runtime.md), [managed policy](../managed-policy.md); privileged installation and all native protection variants not exercised |
| Transport and upstream contracts | [`Transport.Do`](../../internal/tableau/transport.go), version negotiation, bounded responses, redaction, cancellation, and one-request submission | Traced | [Runtime](runtime.md), [evidence index](../evidence/README.md); no external contract recertification |
| Native content lifecycle | Workbook acquisition and publication; independent datasource and flow publication ordering, identity checks, and destination uncertainty | Traced | [Resources and state](resources-and-state.md); not every lifecycle verb traced independently |
| Projects and selectors | Shared project resolver, per-phase hierarchy snapshots, exact identity, collision and ambiguity handling | Traced | [Resources and state](resources-and-state.md); legacy fallback is a proposed simplification below |
| Lineage and schema | Direct dependency discovery, standalone graph acquisition, Metadata ID namespaces, datasource schema fallbacks, and partial graph evidence | Traced | [Resources and state](resources-and-state.md), [metadata coverage](../workbook-metadata-coverage.md); permission-restricted graphs remain incomplete evidence |
| Catalog and labels | Catalog mutation steps, confirmed partial outcomes, metadata clients, and label ownership | Traced and mapped | [Resources and state](resources-and-state.md); tag, label, and description variants have distinct contracts |
| Administration | User/group/member ownership, exact permission mutation and saved-rule verification, label definition clients | Traced and mapped | [Resources and state](resources-and-state.md); permission creation is the detailed mutation representative |
| Pulse lifecycle | Authoring, saved configuration, definition and metric ownership, explicit bundle mappings, new-object recreation, and retained partial identities | Traced | [Resources and state](resources-and-state.md), bundled Pulse Guidance; no analytical values or generated insights |
| Search and source selection | Native search, supplemental inventories, Catalog search, explicit cache readers, and continuation | Traced through existing source map and consumers | [Search source map](search.md); no implicit offline-to-live fallback |
| Cache and collection | Staged refresh, scope closure, generation publication, detail observations, coverage, snapshot-bound continuation, and write-through limits | Traced | [Resources and state](resources-and-state.md); no production cache contents inspected |
| Workspace and artifacts | Registry selection, source provenance, native package validation, dirty guards, staged writes, journals, locks, move, and deletion revalidation | Traced | [Workspaces](../workspaces.md), [resources and state](resources-and-state.md); crash recovery inspected rather than fault-injected during this audit |
| Operations, jobs, and results | Worker handoff, immutable request identity, accepted receipts, monitor observation, reconciliation, error phases, and replaceable last output | Traced | [Runtime](runtime.md), [output contract](../toon.md); uncertainty never proves mutation replay safe |
| Support packages | Identity, source metadata, paging, batch parsing, ordered content batches, path contracts, Pulse schemas, locks, replacement, and command hints | Mapped through consumers | [Package map](../repository-structure.md); mechanisms do not imply uniform operation semantics |
| Diagnostics and small service commands | Auth, environment, doctor, session overview, version, last, policy, and agent action owners | Mapped, with shared runtime traces | [Resources and state](resources-and-state.md), [delivery](dependencies-and-delivery.md); not every local mutation path traced |
| Dependencies and toolchains | Manifest, production imports, CLI package graph, module graph, platform files, and test-only GraphQL parser | Traced | [Dependencies and delivery](dependencies-and-delivery.md); not a license or vulnerability audit |
| Guidance, installation, and updates | Embedded skill bundles, supported host registry, staged ownership receipts, legacy migration, installer checksums, and updater subprocess environment | Traced and mapped | [Dependencies and delivery](dependencies-and-delivery.md), [getting started](../getting-started.md); no installation or update performed |
| Generation, site, and release | Capability generator, explicit site allowlist, manual CI, native-policy gates, release matrix, archives, checksums, and publication conditions | Traced by workflow inspection | [Dependencies and delivery](dependencies-and-delivery.md), [website maintenance](../website.md); no workflow dispatch or publication |
| Documentation and instructions | Architecture, contribution, security, output, search, workspace, evidence, and root agent entry points | Reconciled | [Architecture index](README.md); current facts and proposed changes remain separate |
| Editor extension | [`activate`](../../extensions/vscode/src/extension.ts), [`Controller`](../../extensions/vscode/src/controller.ts), [`Inventory`](../../extensions/vscode/src/inventory.ts), [`Operations`](../../extensions/vscode/src/operations.ts), [`TadxClient`](../../extensions/vscode/src/cli.ts), panel and path guards; source selection, cached reads, previews, consent denial, partial/unknown outcomes, restore, and disposal | Traced, with synthetic contract probes | [Editor architecture](editor-extension.md), [delivery](dependencies-and-delivery.md); no live Tableau or remote-host acceptance, and no host rerun in this refresh |
| Excluded material | Ignored snapshots, archives, downloaded content, credentials, dependency trees, generated build output, and unrelated evaluation directories | Excluded | Not treated as maintained runtime components; no private contents inspected |

Performance coverage concerns implemented bounds, batching, coordination, and memory behavior visible in source.
No throughput, latency, peak-memory, or scale benchmark ran.
Earlier extension checks remain in the [build validation record](../vscode-extension-validation.md), separate from this refresh's validation below.
Observability coverage concerns structured errors, bounded diagnostics, correlation IDs, provenance, and retained recovery state.
The audit does not infer an external telemetry system or monitoring service.

## Extension findings

### Distinguish status inspection from the saved operation outcome

**Status:** resolved in the client parser on 2026-10-25.
**Original category and priority:** integration defect; high because recovery hid newly confirmed failure evidence.
**Evidence limit:** the original reproduction and parser regressions use local fixtures; live transfer acceptance remains separate.

[`job.Inspect`](../../actions/job/inspect.go) returns a successful inspection for an exact retained operation, including failed or partial operations.
`InspectOutput.render` flattens its saved snapshot, which can retain the original transfer's top-level `error`.
[`publicationSnapshots`](../../internal/app/publication_worker.go) preserves that error and confirmed output when saving failed executions.
The status command can therefore exit zero while returning the earlier operation error as data.

At the audit baseline, [`TadxClient.run`](../../extensions/vscode/src/cli.ts) rejected every document containing `error`, regardless of exit status or command.
[`Operations.inspect`](../../extensions/vscode/src/operations.ts) kept its previous state and changed the summary after that rejection.
It did not project the returned document's confirmed partial results into Activity.
The retained operation ID survived, but the user saw a status-check failure instead of the newly observed transfer outcome.

A synthetic zero-exit inspection containing `status: partial`, an operation ID, confirmed item output, and the saved error reproduced this behavior.
A restored Activity entry stayed `unknown` and had no retained result.
This probe invokes the real TypeScript client and operation manager through an injected runner; it does not simulate a real Tableau transfer.

The resolved parser accepts a saved error only for a successful `job inspect --operation-id` response with the exact requested identity and a recognized retained failure state.
It also requires an operation name and an error object.
Nonzero exits, mismatched identities, malformed outcomes, and errors from other commands still fail.
The retained result reaches the existing Activity projection without adding a retry or changing CLI persistence.
[`cli.test.ts`](../../extensions/vscode/tests/cli.test.ts) covers the accepted saved failure and the rejected identity, shape, and exit-status cases.
The existing operation tests cover receipt preservation when inspection itself fails.
These checks do not establish a real failed Tableau transfer and recovery sequence.

### Preserve cache generation evidence in combined inventory

**Category:** current consistency limitation and proposed correction.
**Priority:** medium; display coherence is affected, while execution still revalidates through the CLI.
**Confidence:** high for the missing comparison; a concurrent-refresh end-to-end scenario has not run.

[`TadxClient.siteInventory`](../../extensions/vscode/src/cli.ts) combines cache status and four resource lists from independent processes.
It parses per-resource `generation_id` but never compares those observations or preserves project-list source metadata in `SiteInventory`.
[`ReadResources`](../../internal/cache/resources.go) uses per-kind snapshots, which can legitimately retain different observation generations after scoped refreshes.
Concurrent refresh is another way the independent reads can describe different observations.

A synthetic probe supplies complete, nonstale lists from two generation IDs.
The real parser returns `complete: true` and no warnings.
Completeness here describes individual list coverage; it does not establish one coherent site snapshot.
Project labels and artifact locations can therefore reflect different observations without an explicit consistency indicator.

Preserve source evidence for every list, including projects, in the combined inventory model.
Separate coverage from cross-resource consistency and expose mixed observations without automatically replacing cached reads with live requests.
If a coherent acquisition is required, define that contract in the CLI cache reader instead of inferring it in the editor.
Remove the implicit equivalence between complete lists and a consistent combined view.
Preserve per-scope retention, staleness, acquisition bounds, partial failures, and fresh execution checks.

Validate with a real cache fixture whose scoped refresh preserves older resource snapshots and a controlled refresh between independent reads.
Check both known generation mismatches and missing generation evidence; absence of evidence cannot establish consistency.
If mixed observations are an intentional product choice, document and display that choice rather than reporting an atomic snapshot.

### Verify the installed CLI contract and define extension delivery

**Status:** implementation gaps resolved on 2026-10-25; actual publication and platform acceptance remain separately gated.
**Original category and priority:** compatibility and delivery gaps; medium before distributing the VSIX independently.

The extension has its own version and invokes an independently installed CLI.
At the audit baseline, [`TadxClient.version` and `capabilities`](../../extensions/vscode/src/cli.ts) existed, but activation and inventory loading did not call them.
The extension requirements named required behaviors without a tested CLI version or contract baseline.
Malformed output fails visibly; compatible JSON does not prove that required preview, status, and batch semantics match.

The original tests supplied JSON runners or compiled a synthetic CLI fixture.
They verified editor behavior against supplied contracts without consuming the current Go implementation's output.
The inspected CLI workflows contained no extension checks or VSIX delivery stage.
Local `npm run package` constructed a VSIX without running the separate unit, browser, and host checks.

Activation and explicit reload now check capability implementation, preview, and batch support through [`compatibility.ts`](../../extensions/vscode/src/compatibility.ts).
The client blocks unsupported commands while preserving supported ones and still validates actual JSON responses.
Capability metadata does not authorize mutations or replace saved consent and managed policy.

[`test-cli.mjs`](../../extensions/vscode/scripts/test-cli.mjs) builds the Go CLI and exercises its output through the actual adapter in isolated local fixtures.
[`test-release-cli.mjs`](../../extensions/vscode/scripts/test-release-cli.mjs) runs the same read-contract checks against a checksum-pinned published binary.
The Windows amd64 CLI `1.0.0` archive passes those read contracts; other platforms and live Tableau operations require their own evidence.
The [compatibility tests](../../extensions/vscode/tests/compatibility.test.ts) cover incomplete and unavailable command contracts without conflating consent with compatibility.

The new [manual extension workflow](../../.github/workflows/vscode-extension.yml) owns validation and explicitly selected publication.
[`release.mjs`](../../extensions/vscode/scripts/release.mjs) verifies version, tag, channel, source revision, package contents, and checksums.
The [release tests](../../extensions/vscode/tests/release.test.ts) exercise invalid metadata and package integrity failures.
Publishing consumes the verified artifact without rebuilding it; ordinary pushes and tags do not dispatch this workflow.
Independent extension versions and `extension-vX.Y.Z` tags preserve CLI release discovery.
The [release guide](../vscode-extension-release.md) records setup ownership, pinned release policy, publisher prerequisites, and remaining verification limits.

## Supported simplification opportunities

### Require the shared workbook project resolver

**Category:** simplification opportunity, not a reproduced runtime defect.
**Confidence:** high for duplicate ownership and current production wiring.

[`workbook.NewAdapter`](../../internal/resources/workbook/adapter.go) retains a no-resolver construction path and private `projectPathIndex` and `allProjects` implementations.
The production constructors found in [`app.go`](../../internal/app/app.go), [`content_remote.go`](../../internal/app/content_remote.go), [`publication_status.go`](../../internal/app/publication_status.go), and [`search.go`](../../internal/app/search.go) use `NewAdapterWithProjectResolver`.
That constructor does not require the optional `ResolveProject` method used by `Adapter.ResolveProject`.
`pullService.Execute` supplies [`commandDiscoveryPaths`](../../internal/app/command_runtime.go), which implements path lookup but lacks that method.
`pullReader.DownloadPublishedDatasource` therefore reaches the private fallback when resolving a published datasource's project during `--include-pds` acquisition.
The shared [`project.Adapter`](../../internal/resources/project/adapter.go) already owns hierarchy validation, project paths, and fresh per-phase resolution.
Legacy workbook fixtures still use the alternative constructor.

Make the required resolver capabilities explicit at workbook adapter construction and migrate dependency-acquisition wiring as well as legacy fixtures.
Then remove the private inventory/path fallback and assess the workbook client's duplicate project-list responsibility.
This removes an alternate dependency state and gives project hierarchy behavior one owner.
Preserve exact ambiguity failures, conflicting identity checks, hierarchy cycles, per-phase freshness, command-scoped display reuse, and dependency provenance.
Do not replace fresh consequential validation with the display-path cache.

Before implementation, repeat the constructor and consumer search, including tests and any newly introduced wiring.
Use [`project_collection_test.go`](../../internal/resources/workbook/project_collection_test.go), shared project adapter tests, [`project_selector_consistency_e2e_test.go`](../../internal/app/project_selector_consistency_e2e_test.go), and native publication revalidation coverage.
Also cover `TestWorkbookPullAcquiresDirectPublishedDatasourceArtifactsThroughCLI` in [`phase1_e2e_test.go`](../../internal/app/phase1_e2e_test.go) and dependency publication preflight tests.
An undiscovered consumer that needs the legacy behavior would change the migration scope.

### Use one retained publication state for recovery and projection

**Category:** architectural proposal related to the existing [durable-operation and addressable-result proposals](../potential-improvements.md).
**Confidence:** high for duplicated result-shape knowledge; the proposed replacement is not validated.

[`operationrun.Record`](../../internal/operationrun/store.go) retains compact and full JSON results.
[`reconcilePublicationReceipts`](../../internal/app/publication_status.go), `mergePublicationResult`, `mergePublicationValue`, and `setPublicationIdentity` reconcile saved result shapes with exact receipt observations.
Recovery therefore knows presentation shapes as well as operation state.

Assess a versioned, typed retained publication state that owns identities, confirmed outcomes, uncertainty, and per-item progress before projecting compact or full output.
That could remove JSON-shape patching while leaving resource-specific mutation sequences explicit.
Preserve record compatibility, accepted identities, partial batches, destination verification, historical readers, and refusal to replay interrupted or uncertain submissions.
Validate migration and old-record recovery with [`publication_status_recovery_e2e_test.go`](../../internal/app/publication_status_recovery_e2e_test.go) and projection tests before replacing persisted formats.
Do not treat a presentation refactor as permission to discard durable evidence.

### Evaluate a typed execution boundary as a separate design

**Category:** existing proposal, supported by current structural evidence.
**Confidence:** high for argv-based dispatch; benefits and migration require a separate design review.

[`beginPublicationWorker`](../../internal/app/publication_worker.go) stores command arguments and the worker re-enters `Run` and Cobra.
[`internal/cli/batch.go`](../../internal/cli/batch.go) also reconstructs callbacks for ordered per-item work.
The existing [typed-operation and authorization proposal](../potential-improvements.md) offers an explicit operation request boundary shared by these callers.

Assess whether that boundary removes parsing and callback handoffs while preserving capability registration and every existing guard.
It must preserve policy checks before dispatch, worker-side current policy and consent, exact targets, bounded batch snapshots, and resource-specific outcomes.
Current source does not establish an authorization bypass.
Do not implement this by introducing a configurable generic workflow runner or moving business rules into Cobra.

### Keep diagram authority and execution scope explicit

**Category:** documentation repair applied in this run.
The visual overview remains useful at component and logical action level, while the [runtime guide](runtime.md) owns process and recovery details.
The architecture index names those scopes and links the preserved HTML walkthrough to the current explanation.
Keep the editable diagram, SVG, and HTML presentation synchronized when their represented component relationships change.
No automatic diagram regeneration pipeline is claimed.

## Earlier authorized validation traversal repair

The initial normal validation command times out after ten minutes in `TestRepositoryRespectsImportBoundaries`.
A bounded rerun reproduces the traversal failure.
The checker previously walked the checkout broadly, entering ignored local snapshots and third-party dependency trees outside its intended production-source boundary.
The developer-home-path test used a separate broad walk over the same checkout.
The earlier audit records separate user authorization for diagnosis and repair.
The extension integration refresh does not repeat or extend that source change.

[`internal/architecture/architecture.go`](../../internal/architecture/architecture.go) now owns a shared first-party inventory for import enforcement and the privacy test.
Git supplies tracked and nonignored files; owned Go source roots also retain ignored production files so ignore rules cannot hide imports.
Explicit local import targets remain covered even when Go wildcard discovery omits their directories.
Separate valid nested Go modules own their import policies; malformed module evidence fails.
Source exports without Git metadata use a filesystem fallback, while broken repository metadata fails explicitly.
Source reads remain confined to the selected checkout, and unsupported nonregular Go inputs fail rather than becoming unexamined source.

The new fixtures distinguish ignored snapshots from first-party violations, including new package roots and ignored Go files in existing or newly added packages.
They also cover root-level Go source, tracked deletions, source exports, malformed Git and nested-module metadata, and explicitly imported hidden source.
The repair retains import rules and tests their enforcement rather than extending timeouts or suppressing existing violations.
Native symlink checks still require their platform privileges; skipped fixtures remain reported limits.

## Earlier CLI audit validation

The audit uses Go 1.26.5 on Windows amd64 and the available Node.js runtime.
Source inspection, automated local checks, independent reader exercises, and live-system confirmation are different evidence types.
The completed local checks and any blockers are recorded here before handoff.

| Check | Outcome |
| --- | --- |
| Global skill structure | `quick_validate.py` reports `Skill is valid!`. |
| Independent skill trial | Isolated Python/Node repository: 15 behavioral assertions and 20 documentation links pass; all five source/generated hashes remain unchanged. |
| Maintained Go package inventory | 115 packages resolved through explicit source roots. |
| Production dependency graph | CLI dependencies resolve; test-only GraphQL parser remains outside the inspected executable graph. |
| Static website packaging | Five tests pass with `node --test scripts/tests/site_test.mjs`. |
| Capability and bundled Guidance tests | Capability passes. Bundled Guidance passes the final serial run with its normal timeout in 21.1 seconds. An intermediate combined run with a temporary 60-second limit times out in an unrelated migration test; no agent implementation changes are made. |
| Import-boundary test | Initial normal run times out after ten minutes; bounded reproduction confirms the traversal problem. After repair, the architecture suite and new source-ownership regressions pass. |
| Architecture race, vet, and formatting | Targeted race tests, `go vet ./internal/architecture`, changed-file formatting, and whitespace checks pass. |
| Windows source symlink fixture | Skipped because this account cannot create the required symlink; native coverage is not claimed. |
| Source build and CLI help | `go build -trimpath` succeeds to temporary output; the resulting executable renders root help. |
| Documentation links, anchors, whitespace, and instruction navigation | All 493 local links and anchors across ten documents resolve; `git diff --check` passes. The count includes reader corrections and extension integration. |
| Independent TADX reader exercise | Reader traces workbook acquisition and recovery, identifies inspect-field owners and checks, and cross-checks selection, failures, policy, dependencies, and delivery. No material source contradiction found in that scope; two navigation gaps are corrected. |

No live Tableau request, credential-store persistence, remote write, managed-policy installation, installer execution, release publication, or hosted CI dispatch occurs.
Only the architecture package receives a targeted race run after the validation repair.
Broader application race tests, fuzzing, cross-compilation, native macOS/Linux behavior, and privileged platform policy checks do not run.
The complete pre-push procedure remains in [CONTRIBUTING.md](../../CONTRIBUTING.md#validate-locally); this audit does not bypass or replace it.

## Extension integration refresh validation

The refresh inspects the current working tree after the extension build, including the CLI boundaries used by the editor.
Earlier CLI and extension results above remain historical evidence; they are not counted as repeated checks.

| Check | Outcome |
| --- | --- |
| Runtime recovery, policy, and last-output fixtures | Six focused app tests pass in 8.312 seconds, including their nested scenarios. |
| Production dependency inspection | `go list -deps ./cmd/tadx` passes; the test-only GraphQL parser remains outside the executable graph. |
| Inspection contract probe | A zero-exit saved partial failure reproduces stale Activity state and omitted refreshed output through the real TypeScript parser and operation manager. |
| Cache consistency probe | Complete lists with different generation IDs return `complete: true` without a warning through the real TypeScript parser. |
| Independent fresh-reader exercise | Six navigation and source-cross-check tasks pass. A list-versus-inspect field-placement gap is corrected in the resource and editor guides. |
| Documentation validation | All 587 local links across 13 documents resolve, including 22 fragment references. Changed-document whitespace, dash, and developer-home-path checks pass. |

The probes use injected synthetic output and perform no CLI subprocess or Tableau operation.
They establish consumer behavior for those inputs; real-CLI end-to-end reproduction remains required before implementing the proposed fixes.
The refresh makes no source, dependency, generated-file, credential, consent, installation, or release changes.
It does not run broad Go suites, extension unit/browser/host suites, installers, privileged native checks, or live Tableau acceptance.
See the [extension build validation](../vscode-extension-validation.md) for the earlier fixture-based Windows results and their limits.
