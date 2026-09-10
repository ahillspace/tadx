# Lineage Metadata API contract evidence

This record freezes the bounded factual lineage contract used by `lineage.pull` and automatic workbook, datasource, and flow pull capture.
The captured official Metadata API guide is `Tableau API Documentation/tableau_metadata_api.md`.
Its official source is the [Metadata API guide](https://help.tableau.com/current/api/metadata_api/en-us/).
Its SHA-256 digest is `79e5542a70484f14f5d0a8cf057f16ae7c6b7caf56a582581babb50ece745fc7`.
The captured schema reference is `Tableau API Documentation/tableau_metadata_api_reference.md`.
Its official source is the [Metadata API schema reference](https://help.tableau.com/current/api/metadata_api/en-us/reference/).
Its SHA-256 digest is `820851ed7f7172c484051d60b5deeff9eac775ecc64706f442f35fa90ec37abc`.
The local captures and their line references are historical provenance; see [capture availability](README.md).

## Captured sections

- GraphQL transport, authentication, request, and response behavior is at guide lines 139 through 263.
- Lineage shortcuts and connection pagination are at guide lines 675 through 694.
- Linked flow semantics start at guide line 819.
- Warning and error behavior is at guide lines 1015 through 1050.
- Permission modes are at guide lines 1391 through 1425.
- `DatabaseTable` and its nullable `database` relation are at schema-reference line 203; `DatabaseTablesConnection` is at line 210.
- `DatabaseTable_Filter` supports exact Metadata-ID lookup at schema-reference line 1700.
- Guide line 1575 directs containing-database lookups to `DatabaseTable.database`, rather than the table's upstream-database shortcut.
- `Flow` is at schema-reference line 347.
- `FlowsConnection` is at schema-reference line 438.
- `LinkedFlow` is at schema-reference line 529.
- `LinkedFlowsConnection` is at schema-reference line 536.
- Root `Query` is at schema-reference line 613.
- `Workbook`, `PublishedDatasource`, and their connection fields are at schema-reference lines 595 through 608 and 770 through 783.
- `Flow_Filter` is at schema-reference line 1921.

## Frozen identity contract

The selected root is first resolved through its authoritative REST LUID.
Metadata object IDs and REST LUIDs remain separate fields.
Metadata results never replace REST identity.
Published content nodes require a separate REST LUID, checked for presence and consistency.
Physical database and table nodes use only their Metadata IDs; a missing REST LUID is expected for these kinds and is never synthesized.

## Frozen lineage scope

Lineage is factual, bounded, and non-recursive beyond the requested depth.
Supported roots are one exact workbook, published datasource, or flow.
Supported directions are `upstream`, `downstream`, and `both`.
The default depth is one and the maximum depth is three.
The artifact limit is 500 unique nodes and 1,000 unique directed edges.
Each Metadata connection page requests at most 100 nodes.

Flow-to-flow structure uses linked-flow objects when edge structure is required.
Workbook, published datasource, and flow roots include the schema's upstream database and table connections.
Published datasource and flow roots also include downstream database and table connections; Workbook has no corresponding downstream physical connections in the captured schema.
These are Tableau-reported lineage shortcuts, not assertions that every returned asset is an immediate execution input or output.
The depth limit counts traversals of these reported relationships, not hidden physical hops inside Tableau's lineage shortcuts.
At deeper requested depths, a physical table's documented database relationship can connect it to its containing database by Metadata identity.
Database and table nodes are graph members, not new REST-root selectors or downloadable content artifacts.
TADX records nodes and directed edges only.
TADX does not provide field-level lineage, acquire dependencies, infer undocumented relationships, rewrite packages, or manage flow schedules, runs, connections, credentials, parameters, or owners.

## Completeness contract

Every connection validates `totalCount`, `nodes`, `pageInfo.hasNextPage`, and `pageInfo.endCursor`.
Repeated cursors, repeated nodes within one connection, changing totals, missing fields, GraphQL errors, and incomplete-result warnings prevent a complete claim.
An incomplete capture remains explicit and retains bounded safe diagnostics.
Counts are never rendered as zero when the result is unknown.

## Verification status

Hermetic tests assert static query shape, cursor traversal, identity separation, bounds, deterministic ordering, warning classification, and malformed-response handling used by the executable commands.
The physical-lineage app regression supplies six databases and six tables with no published-content neighbors.
It exercises the real CLI, HTTP provider, resource normalization, compact/full output, standalone lineage artifact, and automatic flow-pull sidecar.
Physical IDs survive without invented REST LUIDs, while graph edges retain their upstream/downstream orientation.
The captured guide, schema, and hermetic contract tests establish contract-verified evidence without claiming deployment verification.
A build-tagged live test is available for opt-in deployment verification against a configured profile.
That test stores only the static query, issue codes, counts, and hashed identifiers.
This evidence record does not claim that the live test has run.
