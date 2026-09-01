# Lineage Metadata API contract evidence

This record freezes the bounded factual lineage contract used by `lineage.pull` and automatic workbook and flow pull capture.
Datasource pull uses the same contract when that blocked capability becomes executable.
The captured official Metadata API guide is `Tableau API Documentation/tableau_metadata_api.md`.
Its SHA-256 digest is `79e5542a70484f14f5d0a8cf057f16ae7c6b7caf56a582581babb50ece745fc7`.
The captured schema reference is `Tableau API Documentation/tableau_metadata_api_reference.md`.
Its SHA-256 digest is `820851ed7f7172c484051d60b5deeff9eac775ecc64706f442f35fa90ec37abc`.

## Captured sections

- GraphQL transport, authentication, request, and response behavior is at guide lines 139 through 263.
- Lineage shortcuts and connection pagination are at guide lines 675 through 694.
- Linked flow semantics start at guide line 819.
- Warning and error behavior is at guide lines 1015 through 1050.
- Permission modes are at guide lines 1391 through 1425.
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
Every returned REST LUID is checked for presence and consistency before persistence.

## Frozen lineage scope

Lineage is factual, bounded, and non-recursive beyond the requested depth.
Supported roots are one exact workbook, published datasource, or flow.
Supported directions are `upstream`, `downstream`, and `both`.
The default depth is one and the maximum depth is three.
The artifact limit is 500 unique nodes and 1,000 unique directed edges.
Each Metadata connection page requests at most 100 nodes.

Flow-to-flow structure uses linked-flow objects when edge structure is required.
TADX records nodes and directed edges only.
TADX does not acquire dependencies, infer undocumented relationships, rewrite packages, or manage flow schedules, runs, connections, credentials, parameters, or owners.

## Completeness contract

Every connection validates `totalCount`, `nodes`, `pageInfo.hasNextPage`, and `pageInfo.endCursor`.
Repeated cursors, repeated nodes within one connection, changing totals, missing fields, GraphQL errors, and incomplete-result warnings prevent a complete claim.
An incomplete capture remains explicit and retains bounded safe diagnostics.
Counts are never rendered as zero when the result is unknown.

## Verification status

Hermetic tests assert static query shape, cursor traversal, identity separation, bounds, deterministic ordering, warning classification, and malformed-response handling used by the executable commands.
The captured guide, schema, and hermetic contract tests establish contract-verified evidence without claiming deployment verification.
A build-tagged live test is available for opt-in deployment verification against a configured profile.
That test stores only the static query, issue codes, counts, and hashed identifiers.
This evidence record does not claim that the live test has run.
