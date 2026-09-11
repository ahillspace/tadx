# Metadata semantics contracts

## Sources and evidence level

`internal/tableau/metadataassets` implements documented REST requests and static Metadata GraphQL queries.
The contracts below have HTTP-fixture verification; selected read paths also have live Cloud verification.
No live writes were performed for this record.
Links are official documentation, not a claim that every supported Server version or license combination was tested.

| Surface | Official source | Verification |
| --- | --- | --- |
| Database, table and column inspection, description/contact updates, tags | [REST metadata methods](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_metadata.htm) | XML identity, request fields and escaping fixtures; live database inspection |
| Content label attachment list/inspect/update/delete | [REST metadata methods](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_metadata.htm) | HTTP method, target identity, payload, read-back and delete fixtures |
| Shared label values and categories | [REST metadata methods](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_metadata.htm) | Name-addressed list/lookup/upsert/rename/create/delete fixtures |
| Paged upstream discovery | [Query](https://help.tableau.com/current/api/metadata_api/en-us/reference/query.doc.html), [database filter](https://help.tableau.com/current/api/metadata_api/en-us/reference/database_filter.doc.html), [table filter](https://help.tableau.com/current/api/metadata_api/en-us/reference/databasetable_filter.doc.html), [column filter](https://help.tableau.com/current/api/metadata_api/en-us/reference/column_filter.doc.html) | Exact filters, bounded pages, parent scope, malformed coverage and duplicate identity fixtures; live database/table/column lists |
| Datasource upstream and field descriptions | [PublishedDatasource](https://help.tableau.com/current/api/metadata_api/en-us/reference/publisheddatasource.doc.html), [Field](https://help.tableau.com/current/api/metadata_api/en-us/reference/field.doc.html), [InheritedStringResult](https://help.tableau.com/current/api/metadata_api/en-us/reference/inheritedstringresult.doc.html) | Independent column pagination/provenance fixtures; live upstream and field-enrichment reads |

## Semantic boundaries

Metadata GraphQL is read-only.
REST database/table/column methods require API 3.5 or later.
This provider uses a conservative shared Cloud/Server floor of 3.17 for labels and 3.21 for label values/categories; earlier Cloud-only introductions are not claimed.
External-asset enrichment requires the relevant Data Management and asset permissions.
Datasource certification is the documented exception to the Catalog requirement for labels.
Shared vocabulary administration does not itself require Data Management.
The provider does not silently reinterpret permission or licensing failures as empty results.

Reading an asset's labels uses POST `/labels` with a content list; it is not a mutation.
One label attachment uses its own LUID, distinct from the target asset LUID.
Shared values/categories use exact names, not invented LUIDs.
Category inspection uses exact matching within the bounded category list because no single-category GET is documented.
Only documented vocabulary request attributes are sent; response-only properties are not inferred to be writable.

Description and contact patches preserve omitted fields.
Empty description/contact clearing remains blocked until its encoding and behavior have authoritative evidence.
Removing an external asset is not a substitute for clearing its metadata.
Virtual-connection writes, monitoring triggers and legacy certification/warning command duplication are outside this implementation.

## Bounded reads and identity

Each response is capped at 8 MiB; discovery pages contain at most 100 records.
Complete datasource enrichment is capped at 10,000 records, with bounded outer and independent field-column continuation.
GraphQL errors, warnings, missing coverage, cursor cycles and conflicting identities fail explicitly.
Tag coverage is tracked separately; a truncated or missing tag collection is not an observed empty collection.
Unknown/null descriptions remain distinct from observed empty strings.
Metadata-only identities remain readable but cannot become REST mutation identities.

## Live read observation

On 2026-09-11, the current CLI completed bounded database, table and table-scoped column lists, exact database inspection, datasource upstream inspection and schema enrichment on an authorized disposable Tableau Cloud site.
The selected datasource had one upstream database, four tables and 45 fields.
Two inspected raw field identifiers were returned by GraphQL in single-bracket identifier form, consistent with `fullyQualifiedName`; this is identity syntax, not a caption match.
Private names, site identifiers and credentials are not retained here.
This observation does not verify description clearing, permission-denied variants, every supported license/version, or any mutation's server-side effects.

## Reproduction

Run `go test ./internal/tableau/metadataassets` and `go test -race ./internal/tableau/metadataassets` for isolated fixtures.
Relevant tests cover request XML, omitted properties, read-only label POST, exact attachment and vocabulary addressing, GraphQL parent filtering, malformed coverage, Metadata-only identities and independent upstream-column pagination.
Live tests remain opt-in and outside the standard test suite.
