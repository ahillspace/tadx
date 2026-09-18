# Metadata semantics contracts

## Sources and evidence level

`internal/tableau/metadataassets` implements documented REST requests and static Metadata GraphQL queries.
The contracts below have HTTP-fixture verification; selected read paths and column-description clearing also have live Cloud verification.
The bounded column write observation is recorded separately below.
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
Documented native content-type spellings are normalized to stable CLI names before identity checks, while unknown types remain mismatches.
One label attachment uses its own LUID, distinct from the target asset LUID.
Shared values/categories use exact names, not invented LUIDs.
Category inspection uses exact matching within the bounded category list because no single-category GET is documented.
Only documented vocabulary request attributes are sent; response-only properties are not inferred to be writable.

Description and contact patches preserve omitted fields.
An explicit empty column description clears that description; omission preserves it.
Empty database/table descriptions and contact clearing remain blocked until their encoding and behavior have authoritative evidence.
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

## Live column-description observation

On 2026-09-13, an authorized Tableau Cloud test using REST API 3.29 verified a column-description round trip.
The original description was recorded before any change.
The test wrote a nonempty description, cleared it with an explicit empty XML attribute, wrote the nonempty description again, and restored the original value.
An independent GET after each update confirmed the requested description.
The final response preserved the other observed column fields.
A follow-up test through the rebuilt CLI, using separate actor and observer credentials, verified preview without mutation, explicit-empty clearing, a repeated-clear no-op, writing the description back, and restoration of the original value.
Private site identifiers, asset identifiers, credentials, and local paths are not retained here.

The request follows the documented [Update Column method](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_metadata.htm):

```xml
<tsRequest><column description=""/></tsRequest>
```

This observation establishes explicit-empty clearing for the tested column endpoint.
It does not establish database/table description clearing, contact clearing, or behavior across every supported Server version and license combination.
HTTP and CLI integration tests verify explicit-empty encoding, omission preservation, readback failures, preview behavior, and repeated-clear no-ops.

## Reproduction

Run `go test ./actions/catalog/column/update`, `go test ./internal/cli/catalog`, and `go test ./internal/tableau/metadataassets` for isolated fixtures.
Run the same three packages with `-race` for race coverage.
Relevant tests cover request XML, omitted properties, read-only label POST, exact attachment and vocabulary addressing, GraphQL parent filtering, malformed coverage, Metadata-only identities and independent upstream-column pagination.
The CLI catalog fixture also confirms that compact table receipts retain available schema and qualified-name context without issuing an extra read.
Live tests remain opt-in and outside the standard test suite.
