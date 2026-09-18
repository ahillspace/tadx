# Content dependencies and portability

## Embedded versus published datasources

A workbook can contain embedded datasource definitions or reference independently published datasources (PDS).
A packaged workbook is not necessarily self-contained: its PDS references can still point to the source site.
Workbook pull includes extracts by default; acquiring direct PDS dependencies produces separate sibling artifacts, not an embedded or recursively rewritten workbook.
Publishing those datasources elsewhere does not itself rebind the workbook to their new identities.
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
Publication has one automatic lifecycle, with no job-mode or no-wait choice.
When supported monitoring is available, TADX saves the accepted job identity and receipt before waiting.
Bulk publications enter pooled monitoring immediately; a single publication joins after its first minute, and an active job may exceed ten minutes.
Local cancellation or unavailable status does not cancel the remote write or establish failure.
Use the saved receipt or exact job ID for recovery, never a second publish; exact job queries require administrator access.
Where supported asynchronous observation is unavailable, TADX uses synchronous publication; flow publication remains synchronous.
Publication jobs are not documented as cancellable through the job cancellation API.
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
Field IDs, upstream REST LUIDs, and GraphQL metadata IDs identify different things; use the identity returned for the intended surface.
Literal slashes in project names can resemble nested paths; select a LUID when paths are ambiguous.

Native pulls retain available bounded lineage in metadata; a lineage-only pull avoids downloading the native package.
Lineage describes only the observed direction, depth, permission scope, and bounds.
`complete` means the selected visible bounded capture, not every real-world dependency or a current metadata index.
Claim a dependency only from a retrieved relationship between the exact identities; otherwise call it unconfirmed.
Failed relationships retain confirmed partial nodes and edges with their failure evidence.
Absent edges do not prove independence.
Routine enrichment diagnostics stay in metadata and expanded output; an explicitly requested dependency acquisition must not be treated as complete when it failed.
