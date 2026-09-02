# Group 2 inventory REST contract

## Status

Workbook and published datasource list and exact-get behavior is contract-verified for the bounded REST fields implemented in Group 2.
Catalog refresh is contract-verified as local orchestration over the already admitted project, flow, workbook, and datasource list contracts.
Catalog get and status use an architecture-locked local generation contract.
Generic content search and content get remain non-executable action seams until their separate normalization evidence closes.

## Captured upstream behavior

The repository-local official Tableau REST capture documents site workbook and datasource listing plus exact reads by LUID.
The admitted list endpoints use one-based classic pagination and support bounded page sizes up to 1,000.
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
`internal/app/catalog_group2_test.go` proves complete four-resource pagination and proves that a later-page failure publishes no catalog generation.
`internal/catalog/lifecycle_test.go` locks deterministic atomic replacement, exact lookup, staleness, and portable relative paths.

## Live verification

Build-tagged read-only workbook and datasource contract tests require a separately configured environment alias, expected site content URL, and authorized resource LUIDs.
The live workbook test verified exact get plus supported name and project-name list filtering against an authorized workbook LUID.
The live datasource test verified exact get plus bounded list and exact-name filtering against an authorized datasource LUID.
An isolated CLI lane verified workbook list/get, datasource list/get, catalog refresh/status/get, compact/full projections, and catalog identity lookup.
Live testing found and removed unsupported workbook `projectId` filtering and unsupported datasource `contentUrl` sorting before the final passing run.
No live test mutates Tableau content.

## Exclusions

Datasource Metadata API, VDS model detail, and composition mapping remain outside this evidence record.
Generic search result normalization remains gated because the released search envelope contains type-specific dynamic content without a frozen cross-kind fixture.
