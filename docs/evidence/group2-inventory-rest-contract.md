# Group 2 inventory REST contract

## Status

Workbook, published datasource, flow, project, user, and group inventory behavior is contract-verified for the bounded REST fields admitted by TADX.
Cache refresh uses the ported TabGet collector and parser design to hydrate eight selectable scopes through TADX authentication and transport.
Cache generations are stored in one versioned SQLite database with transactional publication, environment and site isolation, and indexed local queries.
Cache status and `search.run --cache` use the architecture-locked SQLite generation contract.
Live content terms in `search.run` use Tableau's released `GET /api/-/search` content exploration endpoint.
Administration and Pulse search retain their dedicated adapters because the native endpoint does not return those TADX lifecycle types.
`search.run --cache` remains a local SQLite read with no live fallback.

## Captured upstream behavior

The historical local Tableau REST capture documents site workbook and datasource listing plus exact reads by LUID.
The official source is [Tableau REST API help](https://help.tableau.com/current/api/rest_api/en-us/REST/).
The optional local snapshot is `Tableau API Documentation/tableau_rest_api.md`; see [capture availability](README.md).
The admitted list endpoints use one-based classic pagination and support bounded page sizes up to 1,000.
The current official content exploration contract documents numeric `page` and `limit` query parameters, `terms`, type filters, a 2,000-result window, and `application/vnd.tableau.search-results.v2+json` responses.
Native search is available from REST API 3.16 and Tableau Server 2022.3, while multi-type `in` filtering requires API 3.17 or later.
The official contract shows top-level pagination fields plus dynamic `{uri, content}` items, while live Tableau Cloud also returns the same page inside a top-level `hits` object.
TADX accepts both bounded envelope forms and rejects ambiguous or inconsistent pagination.
Native Tableau Cloud returns published datasources as `unifieddatasource` results whose search LUID is not accepted by classic datasource lifecycle endpoints.
TADX batches their `repositoryUrl` values through the classic `contentUrl:in` datasource filter and returns only the resolved classic REST LUIDs.
Workbook, flow, and project results retain their authoritative native `content.luid` values.
Native numeric `projectId` and `ownerId` associations are not promoted to Tableau LUIDs or canonical project paths.
The cache engine collects users, groups, projects, workbooks, datasources, flows, views, and workbook permissions.
Scoped refreshes collect deterministic dependency closures without claiming implicit dependencies as requested scopes.
The engine schedules independent collectors and remaining pages concurrently, then streams fixed typed batches into SQLite.
The port provenance is the owner-authorized TabGet source at commit `b4b3dc4b45683bfb98bd5638d75588821a0a3138`.
The implemented filters are limited to endpoint-supported name, owner, project, tag, type, and update-time fields.
List and exact-get responses preserve authoritative LUIDs, project identity, lifecycle metadata, tags, and Tableau request IDs.
Canonical project paths come from the separately verified project hierarchy adapter.

## Current list and cache behavior

Ordinary live workbook, datasource, flow, project, user, and group lists use bounded provider reads without SQLite access.
The default record limit is 25; `--limit` accepts 1 through 10000.
`--all` uses the shared inventory collector and renders the live answer in memory.
It rejects incomplete coverage and results beyond 10000 records.
Complete unfiltered collections atomically replace the selected cache scope on a best-effort basis, preserving unrelated scopes.
Filtered `--all` collections save selected observations without claiming complete unfiltered coverage or remote absence.
A cache write failure preserves the live answer and adds a bounded warning.
An explicit `--cache` list reads the current local scope without contacting Tableau.
Public output reports `more_available` without exposing opaque continuation cursors.
Internal pagination and legacy partial-snapshot readers remain implementation details.
`--full` changes presentation only.
Explicit cache refresh excludes permissions by default and preserves the previous generation on failure.
Only explicit refresh rebuilds recognized older cache schemas.

## Hermetic verification

`internal/tableau/workbook/client_test.go` locks workbook routes, filters, pagination, metadata, tags, bounds, and request IDs.
`internal/resources/workbook/adapter_test.go` locks canonical project paths, stable pages, exact LUID resolution, and deterministic ambiguity failures.
The list and inspect tests in `actions/workbook` cover compact and full TOON projections and bounded result metadata.
`internal/tableau/datasource/client_test.go` locks datasource routes, filters, pagination, lifecycle metadata, tags, bounds, and request IDs.
`internal/resources/datasource/adapter_test.go` locks canonical project paths, stable pages, exact LUID resolution, and deterministic ambiguity failures.
The list and inspect tests in `actions/datasource` cover compact and full TOON projections and bounded result metadata.
`internal/tableau/cache` locks selectable collectors, concurrent pagination, adaptive limits, bounded queues, strict XML parsing, permission fanout, stable totals, duplicate rejection, and request IDs.
`internal/tableau/search` locks native request construction, supported type filters, version availability, response normalization, authoritative identity, bounded pagination, and request ID and error preservation.
`internal/tableau/datasource` locks bounded batch translation from native datasource content URLs to classic REST LUIDs.
`internal/resources/search` locks opaque native and combined continuation behavior, datasource identity replacement, and rejection of missing or duplicate authoritative identity without following provider-supplied URLs.
`internal/app/search_test.go` locks native content routing, dedicated administration and Pulse routing, content-first broad ordering, bounded phase filling, and cache isolation.
`internal/app/inventory_all_e2e_test.go` covers explicit full collection across all six list-backed resource types and subsequent cache reads.
`internal/app/read_discovery_e2e_test.go` covers public discovery without opaque cursors.
`internal/app/group1_remote_e2e_test.go` covers preservation of live results when cache publication fails.
`internal/app/inventory_snapshot_compatibility_test.go` covers legacy partial snapshots without Tableau requests or public cursor output.
`internal/cache/sqlite_test.go` locks the versioned strict schema, environment and site isolation, transactional rollback, atomic publication, generation pruning, exact lookup, bounded search, staleness, corruption handling, and concurrent readers.
`internal/app/cache_group2_test.go` proves that the engine streams through the app boundary into SQLite and that a failed collector cannot publish a partial generation.
`actions/cache/refresh` proves that compact and full refresh receipts never contain hydrated cache rows.

## Historical live verification

These observations belong to earlier builds and do not establish live verification of subsequent list and cache changes.
The dates below identify the commits recording the evidence, not independently captured run timestamps.

### Initial inventory and cache records

The initial inventory record appears in `f08ea36` on 2026-09-01.
The cache hydration record appears in `d71dd4f` on 2026-09-01.

Build-tagged read-only workbook and datasource contract tests require a separately configured environment alias, expected site content URL, and authorized resource LUIDs.
The live workbook test verified exact inspect plus supported name and project-name list filtering against an authorized workbook LUID.
The live datasource test verified exact inspect plus bounded list and exact-name filtering against an authorized datasource LUID.
An isolated CLI lane verified workbook list/get, datasource list/get, cache refresh/status/get, compact/full projections, and cache identity lookup.
Live testing found and removed unsupported workbook `projectId` filtering and unsupported datasource `contentUrl` sorting before the final passing run.
No live test mutates Tableau content.
The standalone TabGet baseline hydrated 1,756 rows across seven scopes and workbook permissions in 1.211 seconds using 110 successful requests.
The TADX live acceptance hydrated 1,781 rows across all eight scopes, including 25 flows, in 2.267 seconds using 105 successful requests and no failures.
The complete installed CLI invocation, including process startup, authentication, hydration, validation, and SQLite publication, completed in 3.716 seconds.

### Native search and complete-list record

The native-search acceptance record appears in `6701e49` on 2026-09-05.
The native search live contract verified Tableau Cloud's wrapped `hits` response and authoritative workbook identities.
CLI acceptance verified native multi-type search, classic datasource LUID translation, complete inventory for all six list-backed resource types, and local snapshot continuation without credentials or another Tableau request.
That build's ordinary complete-list and public snapshot-continuation behavior is historical; the current behavior is described above.
This documentation consolidation runs no live tests and does not change the recorded evidence level.

## Exclusions

Datasource Metadata API, VDS model detail, and composition mapping remain outside this evidence record.
Tableau native content types outside workbook, datasource, flow, and project remain outside the TADX search seam until a lifecycle use case admits them.
Exact project-path and owner-name search filters remain on resource-specific list and inspect paths because native search exposes numeric association IDs rather than authoritative Tableau LUIDs.
The future public SQLite schema and arbitrary-query interface remain intentionally undecided and are not exposed by this slice.
