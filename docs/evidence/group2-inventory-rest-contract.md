# Group 2 inventory REST contract

## Status

Workbook, published datasource, flow, project, user, and group inventory behavior is contract-verified for the bounded REST fields admitted by TADX.
Catalog refresh uses the ported TabGet collector and parser design to hydrate eight selectable scopes through TADX authentication and transport.
Catalog generations are stored in one versioned SQLite database with transactional publication, environment and site isolation, and indexed local queries.
Catalog status and `search.run --catalog` use the architecture-locked SQLite generation contract.
Live content terms in `search.run` use Tableau's released `GET /api/-/search` content exploration endpoint.
Administration and Pulse search retain their dedicated adapters because the native endpoint does not return those TADX lifecycle types.
`search.run --catalog` remains a local SQLite read with no live fallback.

## Captured upstream behavior

The repository-local official Tableau REST capture documents site workbook and datasource listing plus exact reads by LUID.
The admitted list endpoints use one-based classic pagination and support bounded page sizes up to 1,000.
The current official content exploration contract documents numeric `page` and `limit` query parameters, `terms`, type filters, a 2,000-result window, and `application/vnd.tableau.search-results.v2+json` responses.
Native search is available from REST API 3.16 and Tableau Server 2022.3, while multi-type `in` filtering requires API 3.17 or later.
The official contract shows top-level pagination fields plus dynamic `{uri, content}` items, while live Tableau Cloud also returns the same page inside a top-level `hits` object.
TADX accepts both bounded envelope forms and rejects ambiguous or inconsistent pagination.
Native Tableau Cloud returns published datasources as `unifieddatasource` results whose search LUID is not accepted by classic datasource lifecycle endpoints.
TADX batches their `repositoryUrl` values through the classic `contentUrl:in` datasource filter and returns only the resolved classic REST LUIDs.
Workbook, flow, and project results retain their authoritative native `content.luid` values.
Native numeric `projectId` and `ownerId` associations are not promoted to Tableau LUIDs or canonical project paths.
The catalog engine collects users, groups, projects, workbooks, datasources, flows, views, and workbook permissions.
Scoped refreshes collect deterministic dependency closures without claiming implicit dependencies as requested scopes.
The engine schedules independent collectors and remaining pages concurrently, then streams fixed typed batches into SQLite.
The port provenance is the owner-authorized TabGet source at commit `b4b3dc4b45683bfb98bd5638d75588821a0a3138`.
The implemented filters are limited to endpoint-supported name, owner, project, tag, type, and update-time fields.
List and exact-get responses preserve authoritative LUIDs, project identity, lifecycle metadata, tags, and Tableau request IDs.
Canonical project paths come from the separately verified project hierarchy adapter.

## Complete list and cache behavior

An unfiltered live workbook, datasource, flow, project, user, or group list collects the complete selected resource scope before rendering rows.
The command's `--limit` bounds only the rendered response.
A successful complete list atomically replaces that one catalog scope and preserves unrelated scopes.
The first response and every continuation cursor bind to the published scope snapshot.
A continuation reads the same local snapshot without repeating the remote traversal and fails if that snapshot was replaced.
An explicit `--catalog` list reads the current local scope without contacting Tableau.
Any explicitly filtered live list remains a bounded provider query and writes only observed records as a partial cache update.
Partial filtered results never establish complete scope coverage or remote absence.

## Hermetic verification

`internal/tableau/workbook/client_test.go` locks workbook routes, filters, pagination, metadata, tags, bounds, and request IDs.
`internal/resources/workbook/adapter_test.go` locks canonical project paths, stable pages, exact LUID resolution, and deterministic ambiguity failures.
`actions/workbook/list` and `actions/workbook/inspect` lock compact and full TOON projections plus filter-bound continuation tokens.
`internal/tableau/datasource/client_test.go` locks datasource routes, filters, pagination, lifecycle metadata, tags, bounds, and request IDs.
`internal/resources/datasource/adapter_test.go` locks canonical project paths, stable pages, exact LUID resolution, and deterministic ambiguity failures.
`actions/datasource/list` and `actions/datasource/inspect` lock compact and full TOON projections plus filter-bound continuation tokens.
`internal/tableau/catalog` locks selectable collectors, concurrent pagination, adaptive limits, bounded queues, strict XML parsing, permission fanout, stable totals, duplicate rejection, and request IDs.
`internal/tableau/search` locks native request construction, supported type filters, version availability, response normalization, authoritative identity, bounded pagination, and request ID and error preservation.
`internal/tableau/datasource` locks bounded batch translation from native datasource content URLs to classic REST LUIDs.
`internal/resources/search` locks opaque native and combined continuation behavior, datasource identity replacement, and rejection of missing or duplicate authoritative identity without following provider-supplied URLs.
`internal/app/search_test.go` locks native content routing, dedicated administration and Pulse routing, content-first broad ordering, bounded phase filling, and catalog isolation.
The list action and app tests lock complete scope acquisition, rendered-row limits, snapshot continuation, atomic replacement, and filtered partial cache updates.
`internal/catalog/sqlite_test.go` locks the versioned strict schema, environment and site isolation, transactional rollback, atomic publication, generation pruning, exact lookup, bounded search, staleness, corruption handling, and concurrent readers.
`internal/app/catalog_group2_test.go` proves that the engine streams through the app boundary into SQLite and that a failed collector cannot publish a partial generation.
`actions/catalog/refresh` proves that compact and full refresh receipts never contain hydrated catalog rows.

## Live verification

Build-tagged read-only workbook and datasource contract tests require a separately configured environment alias, expected site content URL, and authorized resource LUIDs.
The live workbook test verified exact inspect plus supported name and project-name list filtering against an authorized workbook LUID.
The live datasource test verified exact inspect plus bounded list and exact-name filtering against an authorized datasource LUID.
An isolated CLI lane verified workbook list/get, datasource list/get, catalog refresh/status/get, compact/full projections, and catalog identity lookup.
Live testing found and removed unsupported workbook `projectId` filtering and unsupported datasource `contentUrl` sorting before the final passing run.
No live test mutates Tableau content.
The native search live contract verified Tableau Cloud's wrapped `hits` response and authoritative workbook identities.
CLI acceptance verified native multi-type search, classic datasource LUID translation, complete inventory for all six list-backed resource types, and local snapshot continuation without credentials or another Tableau request.
The standalone TabGet baseline hydrated 1,756 rows across seven scopes and workbook permissions in 1.211 seconds using 110 successful requests.
The TADX live acceptance hydrated 1,781 rows across all eight scopes, including 25 flows, in 2.267 seconds using 105 successful requests and no failures.
The complete installed CLI invocation, including process startup, authentication, hydration, validation, and SQLite publication, completed in 3.716 seconds.

## Exclusions

Datasource Metadata API, VDS model detail, and composition mapping remain outside this evidence record.
Tableau native content types outside workbook, datasource, flow, and project remain outside the TADX search seam until a lifecycle use case admits them.
Exact project-path and owner-name search filters remain on resource-specific list and inspect paths because native search exposes numeric association IDs rather than authoritative Tableau LUIDs.
The future public SQLite schema and arbitrary-query interface remain intentionally undecided and are not exposed by this slice.
