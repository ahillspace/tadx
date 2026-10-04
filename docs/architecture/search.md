# Search architecture

Scope: the `search.run` command, its live and local sources, and the separate catalog metadata search.
This is a compact source map; update it when routing, identity, or source contracts change.

## Entry points and data flow

```text
internal/cli/search.go
  -> actions/search/service.go: validate and select the source contract
     -> actions/search/live_source.go: route native and dedicated sources
        -> internal/resources/search/native.go -> internal/tableau/search/client.go
        -> internal/resources/search/adapter.go -> typed REST/Pulse list readers
     -> actions/search/complete_lists.go -> injected resource Service.SearchPage methods
     -> internal/resources/search/cache.go -> internal/cache
  -> shared CLI renderer: compact/full projection, TOON or JSON encoding
```

The CLI accepts a term, `--type`, `--environment`, `--cache`, and `--limit` (default 20, maximum 2,000).
`--cursor` remains hidden for compatibility; provider continuation is normally internal.
`actions/search/{action,validation,types}.go` owns selector expansion, input and cursor validation, result bounds, duplicate checks, output fields, and contextual follow-up commands.
`actions/search/service.go` selects the cached, complete-list, or live source after validating input.
`internal/app/search.go` constructs those sources and binds the selected environment; it does not own search routing or result conversion.
Resource actions own their typed search projections, and app binds them through neutral `value.SearchPage` callbacks without cross-action imports.
Live branches use the command's authenticated Tableau connection and shared transport; `--cache` resolves local configuration without authenticating to Tableau.

## Source selection

| Request | Execution | Matching and order |
| --- | --- | --- |
| Term, `--type content` or workbook/datasource/flow/project | Native search adapter | Tableau relevance order; matching can extend beyond names. |
| Term, `--type admin`, user, or group | Dedicated classic REST inventories | TADX case-insensitive name substring match. |
| Term, `--type pulse`, definition, or metric | Dedicated Pulse inventories | TADX case-insensitive name substring match. |
| Term, no type | Native content phase, then dedicated definition/group/metric/user phase | Content is returned first; no globally merged relevance score. |
| No term, content/admin or a concrete non-Pulse type | `actions/search.CompleteLists` calls injected list service methods | Bounded inventories, without relevance search. |
| No term, pulse/definition/metric | Dedicated Pulse inventories | Same list traversal, without term filtering. |
| `--cache` | `resources/search.CacheSource` and its private cache reader | Local resource records; same substring matcher as dedicated inventories. |

For dedicated and cache sources, `internal/resources/search/adapter.go` sorts types lexically and items within each provider page by name, then LUID.
This is not a global sort across all provider pages.
Its internal project-path and owner filters are exact matches; the public search command does not expose those filters.
The native adapter rejects those unsupported internal filters rather than approximating them.

## Tableau requests and content filtering

`internal/tableau/search/client.go` calls `GET /api/-/search` with search-results v2 JSON, the authenticated site header, `terms`, `limit`, `page`, and a type filter.
One content type uses `type:eq:<type>`; multiple types use `type:in:[...]`.
The client gates native search at REST 3.16 and multi-type search at 3.17, validates response pagination, and never follows the server's next URL directly.
These request versions and filters are documented in Tableau's [content exploration reference](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_content_exploration.htm) and [OpenAPI search contract](https://help.tableau.com/current/api/rest_api/en-us/REST/TAG/index.html).

TADX requests only workbook, datasource, flow, and project from native search.
Views, databases, tables, virtual connections, collections, data roles, and lenses have no selector or result model in this command.
Filtering therefore starts in the request; the decoder also checks returned types against the requested set.
Tableau's documented native categories do not include users, groups, or Pulse definitions/metrics, so those TADX results require additional APIs rather than just a different UI presentation.
The OpenAPI contract lists no collapse/grouping parameter or published-datasource filter; arbitrary content properties are not a promise of supported request filters.

| Additional source | Internal implementation | Endpoint |
| --- | --- | --- |
| Users and groups | `actions/admin/{user,group}/search.go`, `internal/resources/admin`, `internal/tableau/admin/client.go` | `GET /api/{version}/sites/{site}/users` and `/groups` |
| Pulse definitions | `actions/pulse/definition/search.go`, `internal/resources/pulse/definition_read.go`, Pulse client | `GET /api/-/pulse/definitions` |
| Pulse metrics | `internal/resources/pulse/search_metrics.go`, Pulse client | List definitions, then `GET /api/-/pulse/definitions/{id}/metrics` for each definition. |
| Native datasource identity | `internal/tableau/datasource/client.go:ResolveContentURLs` | Classic REST datasource listing filtered by content URL, in batches of at most 25 URLs. |
| No-term inventories | Typed `SearchPage` methods in workbook, datasource, flow, project, user, and group actions | Corresponding classic REST workbook, datasource, flow, project, user, or group list. |

Metric names fall back to the definition name when the metric has none.
Definition pages are memoized within the command, and metric continuation records both definition and metric progress.
These calls retrieve Pulse identities/configuration, not metric values or insights, and search never delegates to Tableau MCP.

## Identity boundary

Public results identify resources by `(type, LUID)`, never by display name; equally named datasources in separate projects must remain separate.
Tableau search's datasource hit LUID can identify an indexed connection rather than the published datasource used by classic REST.
The decoder maps `unifieddatasource` to `datasource` and preserves publication, parent identity/name, and datasource update time; the resource adapter resolves `repositoryUrl` into a classic REST datasource LUID and checks explicit parent/datasource IDs against that mapping.

The adapter retains the first occurrence of each canonical content identity without local sorting or separate type queries.
Explicit unpublished datasource hits and hits with a non-Datasource parent are omitted; conflicting canonical name/project metadata still fails rather than silently selecting one version.
Ranked raw pages are scanned for the requested unique count plus one unique lookahead, so trailing duplicates or omitted connections do not falsely imply more content.
Version 2 cursors store raw page/offset and a prefix digest; replay reconstructs previously seen identities across pages, while command-local page and REST-ID caches avoid refetching during internal aggregation.
Legacy page cursors remain accepted.
Raw-hit totals are not presented as unique datasource/content totals; complete totals are supplied only when established, and the 2,000-hit provider cap remains.
Consistent raw hits repeated across native pages are retained once with a completeness warning and unresolved `more_available`; duplicate raw identities within one page remain invalid.

## Bounds, cache, and failures

Native requests use at most 100 hits per page and Tableau's 2,000-hit search window; a warning signals additional matches beyond that window.
The action expands limits above 100 through internal pages, with a 100-page scan bound.
The dedicated/cache adapter also bounds its scan to 100 source pages per call and returns continuation when that scan ends.
Cursors bind the request's source, environment/site, types, term, filters, and limit; dedicated/cache cursors additionally check source-page identity to detect changed data.
An all-types page may fill entirely with native content before any admin or Pulse request occurs.
The combined search omits a claimed aggregate total and propagates an encountered source error; it does not return a successful partial result when a later source fails.

`--cache` reads local SQLite resource records through `internal/cache/{resources,store}.go`, not Tableau's relevance index or its Metadata API.
With no type selector, unavailable cached types are omitted with warnings; explicitly requesting an unavailable cached scope fails.
A shared cache generation is reported only when all observed scopes are complete and share that generation; partial or independently refreshed records carry a warning instead.
Live search does not silently retry against cache or replace unavailable native search with inventory substring matching.
No-term search calls list services with `Limit`/`Cursor`, not `All`; those bounded calls do not themselves publish complete inventory snapshots.

`actions/search/types.go` and `internal/output/output.go` project/render the result: compact output exposes LUID, type, name, and available project path; `--full` adds available owner and modified time plus source warnings and help.
`--json` changes encoding, not search behavior or selected source.
`more_available` communicates truncation/continuation; absence from a bounded page or cache is not evidence of remote absence.

## Catalog and UI boundaries

The separate `catalog search` command uses `actions/catalog/{service,search}.go` and `internal/tableau/metadataassets/graphql.go`.
`internal/app/catalog.go` constructs the native client, which implements the action's narrow port directly; no catalog forwarding adapter remains.
It uses `POST /api/metadata/graphql` to discover databases/tables and optionally columns within a specified table, preserving Metadata API identities alongside available LUIDs.
It is distinct from both `search --cache` and `/api/-/search`; success in one path does not validate another path's decoder.
Tableau's [UI search documentation](https://help.tableau.com/current/pro/desktop/en-us/search.htm) distinguishes quick/full search and mixed/single-type views, including Tables and Objects and Databases and Files.
The observed connection records are not sufficient evidence to assign them to a particular UI tab or claim exact API/UI parity.
Future optional result categories are recorded in the [README](../../README.md).
