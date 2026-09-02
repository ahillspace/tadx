# Group 2 inventory REST contract

## Status

Workbook and published datasource list and exact-get behavior is contract-verified for the bounded REST fields implemented in Group 2.
Catalog refresh uses the ported TabGet collector and parser design to hydrate eight selectable scopes through TADX authentication and transport.
Catalog generations are stored in one versioned SQLite database with transactional publication, environment and site isolation, and indexed local queries.
Catalog get, search, and status use the architecture-locked SQLite generation contract.
Generic content search and content get remain non-executable action seams until their separate normalization evidence closes.

## Captured upstream behavior

The repository-local official Tableau REST capture documents site workbook and datasource listing plus exact reads by LUID.
The admitted list endpoints use one-based classic pagination and support bounded page sizes up to 1,000.
The catalog engine collects users, groups, projects, workbooks, datasources, flows, views, and workbook permissions.
Scoped refreshes collect deterministic dependency closures without claiming implicit dependencies as requested scopes.
The engine schedules independent collectors and remaining pages concurrently, then streams fixed typed batches into SQLite.
The port provenance is the owner-authorized TabGet source at commit `b4b3dc4b45683bfb98bd5638d75588821a0a3138`.
The implemented filters are limited to endpoint-supported name, owner, project, tag, type, and update-time fields.
List and exact-get responses preserve authoritative LUIDs, project identity, lifecycle metadata, tags, and Tableau request IDs.
Canonical project paths come from the separately verified project hierarchy adapter.

## Hermetic verification

`internal/tableau/workbook/client_test.go` locks workbook routes, filters, pagination, metadata, tags, bounds, and request IDs.
`internal/resources/workbook/adapter_test.go` locks canonical project paths, stable pages, exact LUID resolution, and deterministic ambiguity failures.
`actions/workbook/list` and `actions/workbook/get` lock compact and full TOON projections plus filter-bound continuation tokens.
`internal/tableau/datasource/client_test.go` locks datasource routes, filters, pagination, lifecycle metadata, tags, bounds, and request IDs.
`internal/resources/datasource/adapter_test.go` locks canonical project paths, stable pages, exact LUID resolution, and deterministic ambiguity failures.
`actions/datasource/list` and `actions/datasource/get` lock compact and full TOON projections plus filter-bound continuation tokens.
`internal/tableau/catalog` locks selectable collectors, concurrent pagination, adaptive limits, bounded queues, strict XML parsing, permission fanout, stable totals, duplicate rejection, and request IDs.
`internal/catalog/sqlite_test.go` locks the versioned strict schema, environment and site isolation, transactional rollback, atomic publication, generation pruning, exact lookup, bounded search, staleness, corruption handling, and concurrent readers.
`internal/app/catalog_group2_test.go` proves that the engine streams through the app boundary into SQLite and that a failed collector cannot publish a partial generation.
`actions/catalog/refresh` proves that compact and full refresh receipts never contain hydrated catalog rows.

## Live verification

Build-tagged read-only workbook and datasource contract tests require a separately configured environment alias, expected site content URL, and authorized resource LUIDs.
The live workbook test verified exact get plus supported name and project-name list filtering against an authorized workbook LUID.
The live datasource test verified exact get plus bounded list and exact-name filtering against an authorized datasource LUID.
An isolated CLI lane verified workbook list/get, datasource list/get, catalog refresh/status/get, compact/full projections, and catalog identity lookup.
Live testing found and removed unsupported workbook `projectId` filtering and unsupported datasource `contentUrl` sorting before the final passing run.
No live test mutates Tableau content.
The standalone TabGet baseline hydrated 1,756 rows across seven scopes and workbook permissions in 1.211 seconds using 110 successful requests.
The TADX live acceptance hydrated 1,781 rows across all eight scopes, including 25 flows, in 2.267 seconds using 105 successful requests and no failures.
The complete installed CLI invocation, including process startup, authentication, hydration, validation, and SQLite publication, completed in 3.716 seconds.

## Exclusions

Datasource Metadata API, VDS model detail, and composition mapping remain outside this evidence record.
Generic search result normalization remains gated because the released search envelope contains type-specific dynamic content without a frozen cross-kind fixture.
The future public SQLite schema and arbitrary-query interface remain intentionally undecided and are not exposed by this slice.
