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
A server-side asynchronous publish still waits for completion in TADX; a timeout can leave an unknown remote outcome.

## Schema and lineage evidence

Published datasource schema describes fields and logical tables, not data values or verified cardinalities.
Embedded workbook sources are not independently published datasources.
Field IDs, upstream REST LUIDs, and GraphQL metadata IDs identify different things; use the identity returned for the intended surface.
Literal slashes in project names can resemble nested paths; select a LUID when paths are ambiguous.

Native pulls retain available bounded lineage in metadata; a lineage-only pull avoids downloading the native package.
Lineage describes only the observed direction, depth, permission scope, and bounds.
Absent edges do not prove independence.
Routine enrichment diagnostics stay in metadata and expanded output; an explicitly requested dependency acquisition must not be treated as complete when it failed.
