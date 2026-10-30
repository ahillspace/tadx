# Content dependencies and portability

## Embedded versus published datasources

A workbook can contain embedded datasource definitions or reference independently published datasources (PDS).
A packaged workbook is not necessarily self-contained: its PDS references can still point to the source site.
Workbook pull includes extracts by default; acquiring direct PDS dependencies produces separate sibling artifacts, not an embedded or recursively rewritten workbook.
Before publishing a workbook, confirm that its referenced published datasources exist at the destination and that its references identify them correctly.
Publish missing referenced datasources first, then the dependent workbook.
Workbook publish does not publish those dependencies automatically.
Publishing those datasources elsewhere does not itself rebind the workbook to their new identities.
Tableau binds each reference by its datasource content URL on the destination site, not by name or LUID.
When that URL is already taken, Tableau assigns a suffixed URL such as `Sales_1`; datasource publish reports it and the workbook binding to use.
Workbook publish leaves each reference to Tableau unless an explicit binding maps it to an exact destination datasource.
Repeat `--datasource-binding <source content URL or unique exact caption>=<destination datasource LUID>` for each reference that should point to a different destination datasource.
An exact source content URL takes precedence over a caption.
Caption matching is exact, case-sensitive, and literal; ambiguous captions require a content URL.
TADX reads the destination by LUID; matching names or artifact provenance never infer a binding.
When Tableau rejects a publish because a referenced datasource is missing, the error lists each missing reference and exact same-name candidates; check a candidate before mapping to it.
Inspect dependency and portability metadata before promising a cross-site working copy; do not treat downloaded files as proof that every connection will work at the destination.

Flows can also reference published datasources, files, and databases.
Downloading or publishing a flow does not validate every external connection, provision credentials, recreate schedules, or rebuild refresh dependencies.
Tableau remains authoritative for destination acceptance and supported connection behavior.

## Local versus remote changes

Pull creates native files plus managed metadata; publish reads a managed artifact or an existing native file.
Managed source IDs select local workspace items during publish, not destination objects.
Provenance and workspace placement do not choose the destination site.
An in-site move changes the project, not the site; a migration requires publication and destination verification.
Local artifact deletion and remote content deletion are separate operations.
Do not remove originals merely because a destination accepted a package.

Preserve metadata sidecars when editing managed files.
Dirty re-pull protection prevents accidental loss of local changes; overwrite is a deliberate replacement, not a repair strategy.
Preview resolves scope and conflicts but does not prove native download validity, filesystem write access, or destination acceptance.
Workbook, datasource, and flow publication use synchronous Tableau requests; TADX does not request Tableau jobs, and a job-shaped publish response leaves the outcome unknown.
Explicit Tableau job monitoring polls every 2 seconds for the first 30 seconds, every 5 seconds through 10 minutes, then every 15 seconds, until the shared 20-minute invocation cutoff.
At that cutoff TADX returns the known identity and recovery information while the remote work continues.
Older accepted Tableau publication-job receipts remain available for recovery with `tadx job` commands; current content publishers do not create them.
`--no-wait` returns after safe handoff to a detached local TADX operation with one status command; it does not submit or automatically poll a Tableau job.
For a batch, the status command reports one aggregate local operation with selection-ordered per-item outcomes.
Native pulls also support `--no-wait`: the local worker saves native files and metadata, returns one aggregate check-status command, and never polls remote jobs because downloads have no Tableau job identity.
Default pulls show saved local progress until completion or the shared 20-minute cutoff; downloads continue after the cutoff.
Background execution frees the terminal, but active uploads and downloads retain the shared PAT lease, so another remote command using that PAT may wait for the transfer to finish.
Local operation inspection remains available during a transfer, and explicit Tableau job monitoring yields the credential between observations.
Local cancellation or unavailable status does not cancel the remote write or establish failure.
If an older Tableau job was accepted, use its saved receipt or exact job ID for recovery; never resubmit the publish. Exact job queries require administrator access.

Previously accepted publication jobs are not documented as cancellable through the job cancellation API.
An acknowledged destination LUID outranks a delayed name or Metadata lookup.
Inspect native payloads only when their configuration matters; publication acknowledgement and metadata inspection alone do not prove native configuration correctness.

Datasource append/replace accepts prepared `.hyper` input in V1.
TADX does not unpack packages or edit extracts to prepare that input, and it never silently substitutes another mode.
Publish-endpoint append requires a matching source/destination extract schema; its documented multiple-tables restriction is not a general prohibition on publishing multi-table Hyper files.
Do not substitute the separate live-to-Hyper update API or apply append restrictions to other publish modes without evidence.
See Tableau's [publishing append contract](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_publish.htm#appending-data-to-an-existing-data-source) when interpreting an upstream incompatibility.

## Schema and lineage evidence

Published datasource schema describes fields and logical tables, not data values or verified cardinalities.
Embedded workbook sources are not independently published datasources.
Use `content workbook schema --id <workbook-luid> --descriptions --tags` to inspect embedded and connected published fields together.
Use `--datasource <exact-name>` to narrow ambiguous field identifiers; local overrides retain links to published source fields.
This schema selector does not rebind published workbook dependencies; use `--datasource-binding` for that.
Workbook schema is live-only; cached workbook inspection explicitly identifies datasource details not captured by the cache.
Use `catalog audit --type workbook --id <workbook-luid>` for description and supported tag checks, separated into local and published-source groups.
Field tags are not independently authored metadata; reported upstream column tags retain their source identity.
Field IDs, upstream REST LUIDs, and GraphQL metadata IDs identify different things; use the identity returned for the intended surface.
Literal slashes in project names can resemble nested paths; select a LUID when paths are ambiguous.

Native pulls retain available bounded lineage in metadata; a lineage-only pull avoids downloading the native package.
Use `catalog lineage pull --kind workbook --id <workbook-luid> --fields` for bounded field ownership, source-field, calculation-reference, and upstream-column relationships.
Field detail uses the same graph and bounds; it does not change the selected asset traversal depth.
Lineage describes only the observed direction, depth, permission scope, and bounds.
`complete` means the selected visible bounded capture, not every real-world dependency or a current metadata index.
Claim a dependency only from a retrieved relationship between the exact identities; otherwise call it unconfirmed.
Failed relationships retain confirmed partial nodes and edges with their failure evidence.
Absent edges do not prove independence.
Routine enrichment diagnostics stay in metadata and expanded output; an explicitly requested dependency acquisition must not be treated as complete when it failed.
