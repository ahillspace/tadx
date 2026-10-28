# Resources and local state

This page explains resource ownership, representative operations, and the state contracts those operations preserve.
Its evidence comes from the examined working tree; the [architecture review](review.md) records the baseline and verification limits.
Read the [repository architecture](../repository-structure.md) first for dependency direction and command composition.
Use [named workspaces](../workspaces.md), [security](../security.md), and the [search source map](search.md) for their canonical contracts.

## Domain ownership

Actions own operation-specific rules and output projections.
Resource adapters resolve exact identities and normalize provider results when that responsibility needs a separate boundary.
Tableau clients own request construction, response validation, API pagination, and provider-specific contracts.
The composition root supplies explicit conversions between these contracts; every operation does not require every layer.

| Domain | Action owner | Adapter and integration owners | Distinct responsibility |
| --- | --- | --- | --- |
| Workbooks | [`actions/workbook`](../../actions/workbook) | [`resources/workbook`](../../internal/resources/workbook), [`tableau/workbook`](../../internal/tableau/workbook) | Native lifecycle, inventory, grouped schemas, and workbook-specific output. |
| Datasources | [`actions/datasource`](../../actions/datasource) | [`resources/datasource`](../../internal/resources/datasource), [`tableau/datasource`](../../internal/tableau/datasource), [`tableau/fieldcatalog`](../../internal/tableau/fieldcatalog) | Published datasource lifecycle, package composition facts, and canonical field semantics. |
| Flows | [`actions/flow`](../../actions/flow) | [`resources/flow`](../../internal/resources/flow), [`tableau/flow`](../../internal/tableau/flow) | Native flow acquisition, publication, and lifecycle. |
| Projects | [`actions/project`](../../actions/project) | [`resources/project`](../../internal/resources/project), [`tableau/project`](../../internal/tableau/project) | Canonical project hierarchy, full paths, collisions, and project lifecycle. |
| Lineage | [`actions/lineage/pull`](../../actions/lineage/pull) | [`resources/lineage`](../../internal/resources/lineage), [`tableau/metadata`](../../internal/tableau/metadata) | REST-rooted Metadata graphs and standalone captures. |
| Catalog | [`actions/catalog`](../../actions/catalog) | [`resources/catalog`](../../internal/resources/catalog), [`tableau/metadataassets`](../../internal/tableau/metadataassets) | Database, table, and column metadata; audits, descriptions, tags, and catalog search. |
| Content labels | [`actions/contentlabel`](../../actions/contentlabel) | [`app/labels.go`](../../internal/app/labels.go), [`tableau/metadataassets/labels.go`](../../internal/tableau/metadataassets/labels.go) | Label attachments and their definitions, separate from object tags. |
| Administration | [`actions/admin`](../../actions/admin) | [`resources/admin`](../../internal/resources/admin), [`tableau/admin`](../../internal/tableau/admin) | Users, groups, memberships, explicit permissions, and label definition administration. |
| Pulse definitions | [`actions/pulse/definition`](../../actions/pulse/definition) | [`app/pulse.go`](../../internal/app/pulse.go), [`app/pulse_bundle.go`](../../internal/app/pulse_bundle.go), [`tableau/pulse`](../../internal/tableau/pulse) | Authoring, saved configuration, portable bundles, and definition lifecycle. |
| Pulse metrics | [`actions/pulse/metric`](../../actions/pulse/metric), [`subscription/list`](../../actions/pulse/subscription/list) | [`app/pulse.go`](../../internal/app/pulse.go), [`tableau/pulse`](../../internal/tableau/pulse) | Metric variants, ownership verification, followers, and subscriptions. |
| Search | [`actions/search`](../../actions/search) | [`resources/search`](../../internal/resources/search), [`tableau/search`](../../internal/tableau/search), [`app/search.go`](../../internal/app/search.go) | Native search, additional inventories, bounded continuation, and explicit cache search. |
| Cache | [`actions/cache`](../../actions/cache) | [`tableau/cache`](../../internal/tableau/cache), [`internal/cache`](../../internal/cache) | Remote collection and local generation publication have separate owners. |
| Workspaces | [`actions/workspace`](../../actions/workspace) | [`internal/workspace`](../../internal/workspace), [`internal/artifact`](../../internal/artifact) | Registry lifecycle and managed artifact operations, separate from remote deletion. |

Administrative label categories and values use the metadata asset client rather than the user/group client.
Pulse uses composition-root adapters without a separate `internal/resources/pulse` package.
Tableau MCP owns data queries, view data and images, and Pulse values and generated insights; these CLI paths do not proxy MCP.
The [capability reference](../reference/capabilities.md) supplies the operation inventory and readiness for each public capability.

Small command families retain their own owners:

| Family | Operation and state ownership |
| --- | --- |
| Authentication | [`actions/auth`](../../actions/auth), [`internal/auth`](../../internal/auth), and [`tableau/auth`](../../internal/tableau/auth) resolve PATs, credential references, and command sessions. |
| Environment profiles | [`actions/env/profile`](../../actions/env/profile) and [`internal/config`](../../internal/config) maintain nonsecret target selection and profile settings. |
| Managed policy | [`actions/policy`](../../actions/policy) and [`internal/managedpolicy`](../../internal/managedpolicy) inspect and install machine policy. |
| Jobs | [`actions/job`](../../actions/job), [`internal/jobmonitor`](../../internal/jobmonitor), and [`internal/operationrun`](../../internal/operationrun) distinguish remote jobs from local operation records. |
| Agent installation | [`actions/agent`](../../actions/agent) and [`internal/agent`](../../internal/agent) own installed guidance and installation receipts. |
| Updates and version | [`actions/update`](../../actions/update), [`internal/update`](../../internal/update), and [`actions/version/get`](../../actions/version/get) own binary updates and version reporting. |
| Discovery and diagnosis | [`actions/capability`](../../actions/capability), [`actions/doctor/run`](../../actions/doctor/run), and [`actions/session/overview`](../../actions/session/overview) expose capability facts and bounded environment diagnostics. |
| Last result | [`actions/last`](../../actions/last), [`app/last_result.go`](../../internal/app/last_result.go), and [`internal/lastcommand`](../../internal/lastcommand) retain output and command context for later inspection. |

## Identity and metadata boundaries

REST LUIDs identify remote content authoritatively.
Name and project selectors use exact matching, and ambiguous matches fail.
[`identity.Resolve`](../../internal/identity/identity.go) owns shared selector behavior.
[`project.Adapter`](../../internal/resources/project/adapter.go) owns canonical project paths, hierarchy cycles, and conflicting project records.
Its `BeginProjectResolution` creates a lazy hierarchy snapshot for one validation phase.
A new phase obtains fresh state before consequential revalidation.
Workbook dependency acquisition retains a second project-resolution path.
[`pullReader.DownloadPublishedDatasource`](../../internal/app/app.go) calls `workbook.Adapter.ResolveProject` after the pull service supplies a display-path resolver.
That resolver lacks the optional `ResolveProject` method, so the workbook adapter uses its private hierarchy collector and path index.
The [review finding](review.md#require-the-shared-workbook-project-resolver) includes this production consumer in the proposed consolidation.

Metadata IDs identify GraphQL objects and are distinct from REST LUIDs.
[`value.MetadataIdentity`](../../internal/value/metadata.go) carries both namespaces without treating one as the other.
Embedded workbook datasources and fields remain metadata objects rather than independently managed remote assets.

Three integration owners serve different metadata needs:

- [`tableau/metadata`](../../internal/tableau/metadata) reads direct published datasource parents and lineage relationships.
- [`tableau/metadataassets`](../../internal/tableau/metadataassets) reads catalog and workbook metadata and performs supported descriptions, tags, and label operations.
- [`tableau/fieldcatalog`](../../internal/tableau/fieldcatalog) normalizes datasource field identities, roles, formulas, dependencies, and logical-table facts.

The [workbook metadata contract](../workbook-metadata-coverage.md) describes embedded and published group semantics, audit coverage, and optional field lineage.
Unknown metadata remains distinct from confirmed missing metadata.
An incomplete graph or permission-restricted response cannot establish the absence of upstream content.

## Workbook acquisition

[`workbook.Pull`](../../actions/workbook/pull.go) receives an exact selector, resolved target, workspace, reader, and artifact writer.
The composition root supplies these dependencies through [`app`](../../internal/app/app.go) and its content wiring.

1. Validate the request and resolve the authoritative workbook.
2. For `--preview`, inspect the local acquisition destination and requested dependency discovery without saving native content; with `--include-pds`, download the workbook without extracts to detect its published datasources.
3. Download the native workbook and attempt automatic asset lineage with direction `both` and depth one.
4. Detect direct published datasource parents from the downloaded workbook file, resolve them by exact content URL, and record portability.
5. If `--include-pds` is requested, download each dependency and verify its authoritative LUID.
6. Write the workbook alone or commit its acquired dependencies and workbook as one local bundle transaction.
7. Return portable workspace-relative paths, provenance, confirmed counts, and bounded diagnostics.

Acquisition preview does not verify native payload validity, download permission, optional lineage availability, or filesystem write permission.
Execution checks these prerequisites and rechecks local conflicts; [`acquisitionPlan`](../../internal/app/acquisition_preview.go) records the preview limitations.

`pullCaptureAutomaticLineage` keeps a valid native download when automatic enrichment fails.
It retains confirmed graph evidence, marks incomplete coverage, and withholds unverified counts.
Without explicit dependency acquisition, failed parent discovery leaves portability unknown and records an enrichment warning.
With `--include-pds`, incomplete discovery or a failed dependency download fails the explicit acquisition request.
Acquiring published datasources does not rewrite workbook references or prove cross-site publication readiness.

[`WorkbookManager.Pull`](../../internal/artifact/workbook.go) matches existing local content by server origin, site LUID, and workbook LUID.
It protects local edits with the saved baseline fingerprint and requires overwrite before replacing dirty native bytes.
[`WorkbookBundleManager.Pull`](../../internal/artifact/bundle.go) stages the workbook and datasources, journals the installation, and recovers interrupted bundle transactions.

Datasource and flow pulls have their own contracts in [`datasource/pull.go`](../../actions/datasource/pull.go) and [`flow/pull.go`](../../actions/flow/pull.go).
Standalone [`lineage.Execute`](../../actions/lineage/pull/action.go) resolves its REST root, captures the requested bounded graph, and saves confirmed partial evidence when capture fails.
Its artifact is metadata-only; it does not download a native workbook, datasource, or flow.

## Native publication

[`workbook.Publisher`](../../actions/workbook/publish.go) separates planning, preparation, commit, and destination verification.
`Plan` reads the canonical artifact, resolves the destination, checks collisions, and constructs the exact request.
It matches each `--datasource-binding` to one published datasource reference read from the uploaded file and reads its exact target by LUID.
[`RewriteWorkbookDatasources`](../../internal/artifact/workbook_datasources.go) rebinds only a temporary upload copy; the source file is unchanged.
References without a binding are left to Tableau, and its datasource-not-found rejection is explained with exact candidates from one bounded site listing.
Asynchronous publish jobs fail without a reason, so a failed job gets the same explanation only when the listing shows an unbound content URL missing from the site.
Artifact-source republishing uses the recorded project, name, and LUID and implies overwrite.
It fails if the recorded source was moved, renamed, removed, or replaced.

`Apply` rechecks the overwrite target before upload and before final commit.
The client preparation receives the expected payload fingerprint, size, and native path.
`Complete` observes the result without submitting again.
It distinguishes a confirmed destination from delayed name-index visibility, unavailable identity, and destination mismatch.
Known accepted job IDs and confirmed content identities remain available when later verification fails.

Datasource and flow actions share the publication boundary but preserve different sequences:

| Publication | Distinct behavior and source |
| --- | --- |
| Workbook | [`Publisher.Apply`](../../actions/workbook/publish.go) rechecks overwrite identity around preparation; `Complete` preserves successful publication despite delayed destination visibility. |
| Datasource | [`Publisher.Execute`](../../actions/datasource/publish.go) rereads native fingerprint and composition references, revalidates the destination and collision, then prepares and commits; `Complete` reports the assigned content URL and compares it with the file's own repository location to report a Tableau collision suffix. |
| Flow | [`Publisher.Execute`](../../actions/flow/publish.go) completes artifact, destination, and collision revalidation before upload preparation, leaving no fallible action gate between preparation and commit. |

Preparation can create a server-side upload, so its placement is part of each resource's recovery design.
Do not substitute one generic mutation sequence for these explicit differences.
[`ReadNative`](../../internal/artifact/native.go) validates supported standalone XML or ZIP packages and bounds their input before publication.
Managed artifact publication instead resolves the selected workspace artifact and its provenance.

The composition root can hand workbook, datasource, and flow transfers to a detached instance of the same binary.
[`publication_worker.go`](../../internal/app/publication_worker.go), [`publication_status.go`](../../internal/app/publication_status.go), and [`operationrun`](../../internal/operationrun) own durable handoff and observation.
Local operation IDs and accepted Tableau job IDs describe different objects.
Use returned inspection commands to recover observation; an unknown submission outcome does not authorize automatic resubmission.

## Cached inventory and detail reads

[`NewTargetStore`](../../internal/cache/target.go) binds storage to a canonical server, retained reverse-proxy base path, and exact site.
The physical filename uses a digest; aliases remain presentation and query fields.
Unbound legacy stores do not become evidence for a selected target.

[`app/cache_reads.go`](../../internal/app/cache_reads.go) selects explicit offline readers and maps cache failures into the public error contract.
These readers do not silently substitute a live answer.
Unsupported cached filters fail with a corrective command.
[`Store.ReadResources`](../../internal/cache/resources.go) uses a read transaction, exact selectors, stable ordering, and generation/query-bound continuation.
Changed snapshots invalidate continuation rather than combine observations from different generations.
`UpsertResources` marks touched complete inventory snapshots incomplete because a detail observation cannot preserve their original membership claim.
The prior continuation then fails; the confirmed detail remains readable with partial inventory coverage.

Three kinds of local evidence remain distinct:

| Evidence | Meaning |
| --- | --- |
| Complete inventory scope | The recorded scope observation covers its requested inventory, subject to its reported source and observation time. |
| Detail observation | A live inspection cached one resource projection; it does not establish an exhaustive inventory. |
| Partial inventory | Confirmed records remain useful, but bounds or provider problems prevent a complete-scope claim. |

[`readsource.Metadata`](../../internal/readsource/readsource.go) reports source mode, observation time, completeness, reason, staleness, and generation information.
A summary-only record can satisfy a list while leaving an inspect projection partial.
A live read remains authoritative if best-effort local caching fails.
Inventory publication failures expose `cache_warning`; ordinary detail write-through uses a bounded timeout and does not fail the live operation.

[`cache.Refresh`](../../actions/cache/refresh.go) returns a row-free receipt for requested and implicit scopes.
The fixed collectors cover users, groups, projects, workbooks, datasources, flows, views, and workbook permissions.
Pulse records and datasource schemas enter the cache through their live read paths rather than inventory refresh.

[`Engine.run`](../../internal/tableau/cache/engine.go) collects the scope dependency closure through bounded tasks, adaptive concurrency, and a batch pump.
It validates pagination, identities, response size, and row bounds before publication.
[`beginStagedRefresh`](../../internal/cache/staging.go) builds a disposable disk database before acquiring the target publication transaction.
[`GenerationWriter.Publish`](../../internal/cache/writer.go) checks admitted scope completion, normalizes records, fingerprints content, and validates schema and integrity.
`publishStaged` copies ready rows and atomically promotes the target generation.
Denied resource-specific permission reads can retain useful inventory while the permission scope stays explicitly incomplete.
An unsuccessful refresh does not publish its unfinished generation.

Schema incompatibility, refresh-required schema, missing scope, missing record, ambiguity, and invalid continuation have separate recovery advice in `cacheReadError`.
Refresh replaces the current generation used by [`Store.Search`](../../internal/cache/store.go); include each scope needed in that search generation.
Resource reads also use independent per-kind snapshots.
`GenerationWriter.replaceResourceEntries` replaces requested kinds while preserving unrelated resource entries and their existing observation times.
Retained entries do not acquire the new refresh's freshness or prove that unrelated inventory was refreshed.
[`rebuildSchemaForRefresh`](../../internal/cache/schema.go) replaces older supported schemas within the publication transaction; a failed refresh rolls back that replacement.
Include all required inventory scopes when rebuilding, then reacquire detail-only projections through their live read commands.
Do not repair an incompatible schema by silently deleting or reinterpreting the saved database.

## Schemas, catalog, and labels

[`SchemaAdapter.ReadDatasourceSchema`](../../internal/resources/datasource/schema.go) verifies the authoritative datasource before reading field metadata.
It rejects mismatched schema identity or incomplete required field facts.
[`fieldcatalog.Client.Read`](../../internal/tableau/fieldcatalog/client.go) tries VDS `read-metadata`, VDS `describe-datasource`, and then Metadata API.
Authentication failures stop fallback rather than being concealed by another source.
Published workbook groups reuse this owner, while embedded groups retain Metadata identities and their observed relationships.
[`app/datasource_schema.go`](../../internal/app/datasource_schema.go) separately wires live schema persistence and explicit cached schema retrieval.

Catalog reads, audits, and changes remain separate operations:

- [`actions/catalog/read`](../../actions/catalog/read) owns exact selectors, bounded lists, inspect projections, and continuation.
- [`actions/catalog/audit`](../../actions/catalog/audit) distinguishes present, missing, and unknown facts, and counts assessed objects against the request bound.
- [`actions/catalog/update`](../../actions/catalog/update) computes requested differences, revalidates state, and performs property writes, tag additions, and individual removals explicitly.

Catalog mutations are not cross-resource transactions.
Their results retain completed steps, the failed step, and confirmed identities after partial completion.
Unknown tag observations do not become confirmed empty tag lists.
[`metadataassets.Client`](../../internal/tableau/metadataassets/client.go) enforces capability checks, operation-specific REST minimums, and bounded responses.
Checked-in [metadata evidence](../evidence/metadata-semantics-contract.md) supplies upstream contract context; it does not prove access on every deployment.

[`contentlabel.Update`](../../actions/contentlabel/update.go) resolves either an attachment or an exact asset target and label value.
It validates the saved definition and rejects system-managed label changes through this surface.
Label attachments, label categories and values, object tags, and inherited field descriptions are different contracts.
Update their respective owners rather than introducing a shared string-metadata workflow.

## Administrative permissions

[`permission.Create`](../../actions/admin/permission/create.go) reads and validates one exact explicit rule before building its preview.
Before mutation, it rereads the snapshot and rejects changed state.
An identical rule returns `unchanged`; a conflicting mode requires explicit deletion before replacement.
After an acknowledged write, it reads the rule again and verifies the saved mode.
Failed readback retains the acknowledged result and reports verification failure with a confirmed outcome.

[`admin.Adapter.GetPermissionRule`](../../internal/resources/admin/permission_mutations.go) verifies principal type and identity before reading resource rules.
It does not calculate effective user access from group membership, inheritance, or Tableau's broader permission model.
User, group, and membership actions keep their own lifecycle and receipt contracts in [`actions/admin`](../../actions/admin).
Supported remote administration remains subject to the same selected-site consent and managed-policy checks described in [security](../security.md).
A preview or discovery result does not authorize a write.

## Pulse authoring and portable bundles

[`definition.Create`](../../actions/pulse/definition/create.go) resolves exact live field IDs and checks both names and semantically equivalent existing definitions.
Equivalent definitions depend on datasource, measure, aggregation, time dimension, running total, temporality, and filters.
Names and adjustable dimensions alone do not distinguish otherwise equivalent definitions.
The composition root validates field eligibility using canonical datasource schema facts in `pulseDefinitionFieldValidator`.
Creation retains the confirmed definition LUID if default-metric resolution fails and advises inspection rather than repeating creation.

[`definition.Publish`](../../actions/pulse/definition/publish.go) recreates a portable bundle as new objects.
Its input requires explicit source-to-destination datasource mappings, including publication within the same site.
`PublishPrepareBundle` reconstructs supported business configuration from retained provider responses and rejects unsupported sections instead of discarding them.
Destination fields and variant constraints are validated again before creating objects.
[`pulseBundleAdapter`](../../internal/app/pulse_bundle.go) connects this action to schema validation and provider calls.

[`VerifyBundleDefinition`](../../internal/tableau/pulse/bundle_reconcile.go) verifies saved shared configuration before metric recreation.
`ReconcileBundleMetric` checks definition, site, datasource, and saved specification ownership.
Bounded retries accommodate 404 visibility delays; mismatched ownership does not become an automatic retry.
Confirmed source-to-destination mappings remain available after partial completion.
No distributed transaction or automatic removal of already created objects is promised.
Followers, users, values, and generated insights are not copied.

Metric forking and following retain separate actions in [`actions/pulse/metric`](../../actions/pulse/metric).
[`metric_reconcile.go`](../../internal/tableau/pulse/metric_reconcile.go) owns variant readback, while definition deletion and metric deletion have their own contracts.
Portable Pulse bundles and native workbook dependency acquisition therefore provide different portability guarantees.

## Persistent state and shared foundations

| State or contract | Owner | Lifetime and protection |
| --- | --- | --- |
| Workspace registration and manifest | [`workspace.Manager`](../../internal/workspace/manager.go), [`config`](../../internal/config) | Stable logical names and IDs map to machine-local roots; see [workspace selection](../workspaces.md). |
| Canonical native content and sidecars | [`artifact`](../../internal/artifact) | Source provenance, baseline fingerprints, managed containment, and transaction recovery protect saved artifacts. |
| Cache generations and observations | [`cache`](../../internal/cache) | Target-scoped SQLite state preserves scope coverage, detail observations, and schema compatibility separately. |
| Local operations and remote job receipts | [`operationrun`](../../internal/operationrun), [`jobmonitor`](../../internal/jobmonitor) | Durable observation survives terminal return without promising unrestricted concurrent PAT use. |
| Credentials and target consent | [`auth`](../../internal/auth), [`config`](../../internal/config), [`managedpolicy`](../../internal/managedpolicy) | Native credential storage and exact selected-site authorization follow the [security contract](../security.md). |
| Output and recovery context | [`output`](../../internal/output), [`lastcommand`](../../internal/lastcommand), [`commandhint`](../../internal/commandhint) | Bounded projections, saved results, redaction, and contextual follow-up commands. |
| Capability facts | [`capability`](../../internal/capability) | Typed definitions own implementation state, evidence, readiness, safety, selectors, batching, and command bindings. |
| Shared values and errors | [`value`](../../internal/value), [`errs`](../../internal/errs), [`readsource`](../../internal/readsource) | Standard value contracts preserve identities, error phases/outcomes, and source coverage across owners. |
| Paths, locking, and replacement | [`pathspec`](../../internal/pathspec), [`lock`](../../internal/lock), [`fsreplace`](../../internal/fsreplace) | Platform-specific filesystem behavior supports higher-level containment and transactional writes. |
| Bounded collections and batches | [`paging`](../../internal/paging), [`batchspec`](../../internal/batchspec), [`contentbatch`](../../internal/contentbatch) | Pagination, request bounds, ordered batch outcomes, and deferred operation observation. |

[`artifactTreeFingerprint`](../../internal/artifact/snapshot.go) covers payload, metadata, and sidecars for local destructive-operation revalidation.
[`artifact.Move`](../../internal/artifact/move.go) and [`artifact.Delete`](../../internal/artifact/delete.go) revalidate selected content before transfer or removal.
Workspace locks coordinate managed writes, and multi-workspace operations order locks consistently.
Overwrite or force options do not remove containment, exact identity, or changed-state checks.
Artifact paths persist relative to the resolved workspace with forward slashes.
Local deletion never implies remote content deletion.

[`workspace.StatusArtifact`](../../actions/workspace/status.go) exposes acquired source provenance in full status output.
[`statusArtifact`](../../internal/app/workspace.go) projects the validated inventory's server origin, site LUID, environment label, and site label without resolving current aliases.
Compact status retains its existing identity, path, and local-state projection.
The server origin, site LUID, and content LUID distinguish acquisitions when names or LUIDs recur across servers or sites.
Environment and site labels describe the acquisition source; they do not prove a current remote match.
The `clean` state compares local content with its saved baseline, not with current remote bytes.
The [extension contributor guide](../../extensions/vscode/README.md) explains how the editor consumes this public CLI projection.

## Locate a feature change

Use this map to identify owners and existing regression coverage before editing:

| Intended change | Starting source | Existing behavioral coverage |
| --- | --- | --- |
| Cached inventory field consumed by the editor | Resource `list_types.go`, normalized values, provider decoding and adapters, [REST cache collectors](../../internal/tableau/cache/collectors.go), [cached list readers](../../internal/app/cache_reads.go), then the [editor field path](editor-extension.md#change-placement-and-verification) | Workbook examples: [`list_test.go`](../../actions/workbook/list_test.go), [`workbook_inventory_test.go`](../../internal/app/workbook_inventory_test.go), provider and collector tests; extension parser, projection, and browser fixtures. |
| Workbook inspect field or projection | [`inspect_types.go`](../../actions/workbook/inspect_types.go), [`value/content.go`](../../internal/value/content.go), [`app/workbook_inventory.go`](../../internal/app/workbook_inventory.go), provider decoding, and cache readers | [`inspect_test.go`](../../actions/workbook/inspect_test.go), [`workbook_inventory_test.go`](../../internal/app/workbook_inventory_test.go), [`workbook_inspect_metadata_cache_e2e_test.go`](../../internal/app/workbook_inspect_metadata_cache_e2e_test.go). |
| Workbook publication or source republish | [`workbook/publish.go`](../../actions/workbook/publish.go), [`tableau/workbook/client.go`](../../internal/tableau/workbook/client.go), publication wiring | [`publish_project_phase_test.go`](../../actions/workbook/publish_project_phase_test.go), [`publish_visibility_test.go`](../../actions/workbook/publish_visibility_test.go), [`publish_test.go`](../../actions/workbook/publish_test.go). |
| Workbook dependency acquisition or lineage warnings | [`workbook/pull.go`](../../actions/workbook/pull.go), [`artifact/bundle.go`](../../internal/artifact/bundle.go) | [`workbook/pull_test.go`](../../actions/workbook/pull_test.go) and artifact transaction tests. |
| Datasource fields or workbook published groups | [`fieldcatalog`](../../internal/tableau/fieldcatalog), [`resources/datasource/schema.go`](../../internal/resources/datasource/schema.go), [`workbook/schema.go`](../../actions/workbook/schema.go) | [`datasource/schema_test.go`](../../actions/datasource/schema_test.go), [`workbook/schema_test.go`](../../actions/workbook/schema_test.go), provider fixtures. |
| Cache filtering, scope coverage, or freshness | [`cache/resources.go`](../../internal/cache/resources.go), [`cache/resource_scope.go`](../../internal/cache/resource_scope.go), [`app/cache_reads.go`](../../internal/app/cache_reads.go) | Existing `internal/cache` tests and [`actions/cache/refresh_test.go`](../../actions/cache/refresh_test.go). |
| Catalog metadata changes | [`catalog/update`](../../actions/catalog/update), [`metadataassets`](../../internal/tableau/metadataassets) | [`failure_characterization_test.go`](../../actions/catalog/update/failure_characterization_test.go), tag acknowledgment and workflow tests. |
| Permission rule changes | [`admin/permission`](../../actions/admin/permission), [`resources/admin/permission_mutations.go`](../../internal/resources/admin/permission_mutations.go) | [`create_action_test.go`](../../actions/admin/permission/create_action_test.go), [`delete_action_test.go`](../../actions/admin/permission/delete_action_test.go), provider contract tests. |
| Pulse authoring or bundle portability | [`pulse/definition`](../../actions/pulse/definition), [`app/pulse_bundle.go`](../../internal/app/pulse_bundle.go), [`tableau/pulse`](../../internal/tableau/pulse) | [`create_partial_test.go`](../../actions/pulse/definition/create_partial_test.go), [`publish_action_test.go`](../../actions/pulse/definition/publish_action_test.go), reconciliation tests. |
| Artifact selection or destructive operations | [`artifact/inventory.go`](../../internal/artifact/inventory.go), [`snapshot.go`](../../internal/artifact/snapshot.go), move/delete owners | [`actions/workspace/artifact_delete_test.go`](../../actions/workspace/artifact_delete_test.go), [`move_test.go`](../../actions/workspace/move_test.go), artifact filesystem tests. |
| Workspace inventory source provenance | [`workspace/status.go`](../../actions/workspace/status.go), [`app/workspace.go`](../../internal/app/workspace.go), [`artifact/inventory.go`](../../internal/artifact/inventory.go) | [`workspace_source_e2e_test.go`](../../internal/app/workspace_source_e2e_test.go) checks full output, compact compatibility, recurring identities, and the default site's empty label. |

The listed tests locate existing contracts; this page does not claim they were run during source inspection.
For inspect changes, choose the compact and full projections explicitly; JSON encoding uses the same selected projection.
For newly acquired fields, trace provider decoding and normalized values through live and cached inspection before changing output.
Workbook live inspection caches its normalized REST record before Metadata enrichment; cached inspection reports enrichment as `not_captured`.
Metadata-derived fields therefore need their own acquisition and partial-coverage contract rather than an assumed cache field.
Follow [CONTRIBUTING.md](../../CONTRIBUTING.md) for required validation rather than inventing a separate documentation-specific gate.
When a capability changes, update its typed definition, binding, help, and maintained guidance through their owners.
Generate derived capability references with the maintained generator; do not edit them manually.

## Coverage limits

This page documents source-traced operation families and inventory-level ownership for smaller command families.
It is not a line-by-line correctness review of every operation or a claim of live compatibility with every Tableau deployment.
External access, licensing, version support, and site permissions can constrain available metadata independently of local implementation.
Historical live checks and build-specific findings remain scoped to their original evidence.
The [architecture review](review.md) distinguishes current validation, documentation gaps, and proposed simplifications from implemented behavior.
