# Native search and complete inventory lists

Status: implemented and verified.

## Outcomes

Live content search uses Tableau's native content search endpoint instead of scanning independent resource list APIs inside TADX.

Resource list commands perform a complete live inventory of the selected resource type before rendering a bounded result page.

An explicitly filtered list retains its direct server-side Tableau filter because the result is intentionally not a complete inventory.

A successful live inventory atomically replaces only that resource scope in the local catalog and preserves unrelated catalog scopes.

`--limit` bounds rendered rows and does not limit live inventory acquisition.

A continuation cursor reads the same catalog scope snapshot and fails if that snapshot has been replaced.

`--catalog` reads the current local scope snapshot without contacting Tableau.

## Search routing

Workbook, datasource, flow, and project searches use `GET /api/-/search`.

Administration and Pulse searches retain their dedicated API adapters because Tableau content search does not provide those lifecycle resource types.

Broad top-level search combines the native content results with the dedicated administration and Pulse sources through a deterministic bounded cursor.

Tableau relevance order is preserved within native content results.

Every returned result must contain a supported type and authoritative LUID.

Native numeric project and owner association IDs are not treated as Tableau LUIDs or canonical project paths.

Unsupported server versions and content types produce explicit bounded errors or use an intentionally documented dedicated adapter.

## List routing

Workbook, datasource, flow, project, user, and group lists reuse the hardened catalog traversal engine.

Unfiltered lists use complete inventory semantics, while existing exact list filters retain bounded direct REST semantics and partial catalog write-through.

The engine performs first-page discovery, concurrent remaining-page collection, bounded retry, duplicate identity rejection, response validation, deterministic normalization, and fail-closed cancellation.

Workbook, datasource, and flow inventory resolves project paths from one collected project hierarchy instead of reloading the hierarchy for every resource.

Pulse lists remain on their dedicated bounded APIs until Pulse becomes a catalog scope.

The first rendered page contains real resource rows plus total and catalog snapshot provenance.

Subsequent pages are local catalog reads and do not repeat the live inventory.

## Catalog guarantees

Each inventory scope has independent completeness, observation time, and snapshot identity.

Replacing one scope deletes stale records for that scope while preserving every unrelated scope.

Scope replacement and freshness publication occur in one SQLite transaction.

Failed or cancelled inventory cannot become visible.

A full catalog refresh retains its existing generation contract.

## Verification

Hermetic tests cover native search request construction, response normalization, pagination, unsupported filters, invalid identities, upstream errors, and bounded bodies.

Hermetic tests cover concurrent inventory traversal, project-path normalization, atomic scoped replacement, stale-row deletion, unrelated-scope preservation, snapshot cursor invalidation, and failed-run rollback.

Live acceptance compares native search results and complete list counts against an authorized disposable Tableau Cloud site.

Live acceptance also verifies that native `unifieddatasource` results are translated to classic REST LUIDs before they reach CLI output.

Performance acceptance records request counts and elapsed time for a bounded search, a workbook inventory, and a full catalog refresh.

The implementation must not read or modify `docs/tadx-capability-map.html`.
