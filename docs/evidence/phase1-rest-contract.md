# Phase 1 REST contract evidence

This record freezes the upstream contract used by the Phase 1 remote capabilities.
The captured official Tableau REST API help is `Tableau API Documentation/tableau_rest_api.md`.
Its SHA-256 digest is `89bf3a33175ddcfee15e45b13facfa1833b4a06218580c66ef48e04691f1842b`.

The implementation uses these captured sections:

- PAT sign-in request, response, and errors at lines 40901 through 41166.
- Classic pagination at lines 1699 through 1751.
- Workbook listing at lines 38650 through 38925.
- Exact workbook reads at lines 27547 through 27610.
- Project listing at lines 36768 through 37060.
- Workbook download at lines 20889 through 20955.
- Datasource download at lines 20063 through 20131.
- Exact datasource reads at lines 35015 through 35168.
- Upload initiation at lines 28079 through 28146.
- Upload append at lines 10432 through 10506.
- Workbook publish at lines 34005 through 34425.
- TWB validation at lines 49525 through 49700.
- Asynchronous publish jobs at lines 3475 through 3504.
- Job queries at lines 36548 through 36643.

Contract tests use local HTTP servers and parse the multipart publish and append bodies.
They assert methods, paths, headers, payload identities, exact uploaded bytes, ordered multi-block sequence IDs, pagination, response parsing, validation warnings and errors, terminal jobs, and in-flight polling timeouts.
The local contract tests do not claim live Tableau deployment verification.
The separate live records below cover one successful workbook round trip and one cross-site rejection.

The Query Job endpoint is documented as administrator-only while workbook publish can be available to non-administrator publishers.
Synchronous publish is the default, and asynchronous polling requires the explicit `--as-job` option.
When Tableau accepts an asynchronous publish but job polling is forbidden, cancelled, or times out, the mutation outcome is unknown rather than failed.
The error retains the job and request IDs and does not advise an automatic retry.
The capture disagrees on a 1,000-block versus 10,000-block upload limit.
TADX applies the conservative 1,000-block limit.

## Live workbook publish behavior

A live Tableau Cloud workbook round trip completed on 2026-08-31 against an authorized disposable development site.
The sequence pulled a native workbook package, previewed an exact project target, applied the publish, and received a new authoritative workbook LUID.
This record omits environment aliases, site names, project names, workbook names, LUIDs, and request IDs.

### Cross-site published datasource rejection

A second live test on 2026-09-01 used an unchanged workbook package with a direct published datasource binding.
The source and target were different sites on the same Tableau Cloud pod.
Preview identified the cross-site provenance and retained the explicit target project.
Apply sent the native workbook package unchanged.

Tableau returned HTTP 400 with upstream error code `400011` because the bound published datasource was unavailable on the target site.
TADX preserved the structured upstream error and Tableau request ID in the live CLI response.
This evidence redacts the datasource name and request ID.
The follow-up preview found no workbook collision, which confirmed that the rejected publish did not create the target workbook.

This behavior makes Tableau authoritative for package compatibility at publish time.
TADX warns when artifact provenance differs from the explicit target, but it does not add a separate fail-closed published-datasource gate.
TADX does not rewrite the workbook package, rebind dependencies, or claim dependency-aware promotion.

## Workbook published datasource detection

The captured official Tableau Metadata API help is `Tableau API Documentation/tableau_metadata_api.md`.
Its SHA-256 digest is `79e5542a70484f14f5d0a8cf057f16ae7c6b7caf56a582581babb50ece745fc7`.
The captured official GraphQL schema reference is `Tableau API Documentation/tableau_metadata_api_reference.md`.
Its SHA-256 digest is `820851ed7f7172c484051d60b5deeff9eac775ecc64706f442f35fa90ec37abc`.

The detection contract uses these captured sections:

- Metadata API GraphQL endpoint and authentication at lines 139 through 183.
- JSON POST request encoding at lines 185 through 213.
- GraphQL response shape at lines 215 through 263.
- Warning and error behavior at lines 1015 through 1050.
- Permission modes at lines 1391 through 1425.
- `EmbeddedDatasource` and its connection at schema-reference lines 301 through 314.
- `PublishedDatasource` and its connection at schema-reference lines 595 through 608.
- `Workbook` and its connection at schema-reference lines 770 through 783.
- Connection ordering and filters at schema-reference lines 1067, 1270, 1277, 1826 through 1839, 2211 through 2224, and 2442 through 2455.

The authoritative identity traversal is `Workbook.luid` to `Workbook.embeddedDatasourcesConnection` to `EmbeddedDatasource.parentPublishedDatasourcesConnection` to `PublishedDatasource.luid`.
The final `PublishedDatasource.luid` is the locally unique identifier used by the Tableau REST API.
Metadata API object IDs are not used as REST identities.

The primary request is:

```graphql
query WorkbookDirectPublishedDatasources(
  $workbookLuid: String!
  $embeddedAfter: String
  $pageSize: Int!
) {
  workbooksConnection(
    first: 2
    filter: {luid: $workbookLuid}
    permissionMode: OBFUSCATE_RESULTS
  ) {
    totalCount
    nodes {
      luid
      embeddedDatasourcesConnection(
        first: $pageSize
        after: $embeddedAfter
        orderBy: {field: ID, direction: ASC}
        permissionMode: OBFUSCATE_RESULTS
      ) {
        totalCount
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          id
          name
          parentPublishedDatasourcesConnection(
            first: $pageSize
            orderBy: {field: ID, direction: ASC}
            permissionMode: OBFUSCATE_RESULTS
          ) {
            totalCount
            pageInfo {
              hasNextPage
              endCursor
            }
            nodes {
              luid
              name
            }
          }
        }
      }
    }
  }
}
```

The client requires exactly one workbook node with the requested REST LUID.
It exhausts embedded datasource cursors and never treats a truncated connection as complete.
A parent connection that exceeds one bounded page is incomplete until a separately filtered embedded-datasource query exhausts that parent's cursor.
Direct references are deduplicated by source site and published datasource REST LUID.
The client rejects blank LUIDs and conflicting labels for one LUID.

Any GraphQL `errors` entry makes detection incomplete, even when the response also contains partial `data`.
Warnings such as `BACKFILL_RUNNING`, `INHERITANCE_INCOMPLETE`, `LINKED_RESULTS_INCOMPLETE`, `MAX_PAGE_SIZE_EXCEEDED`, `NODE_LIMIT_EXCEEDED`, `TIME_LIMIT_EXCEEDED`, and `USER_VISIBILITY_IS_LIMITED` are not complete success.
Zero workbook nodes represent indexing lag or missing visibility, not a portable workbook.

Local HTTP contract tests freeze the request, response, pagination, error, warning, identity, and deduplication behavior before live wiring.
A redacted live Tableau Cloud request and response were captured on 2026-08-31 and close B5.

The opt-in live contract test is `TestLiveWorkbookPublishedDatasourceContract` in `internal/tableau/metadata/live_contract_test.go`.
The `live` build tag keeps it outside the standard test suite.
It uses the normal non-secret TADX profile, resolves the PAT through the profile's environment-variable references, verifies every Metadata API LUID through the REST datasource endpoint, and prints a sanitized request and response capture.
Fixture identifiers use stable truncated SHA-256 digests.
Names, messages, non-empty cursors, and Tableau request IDs are redacted or omitted.
Set `TADX_LIVE_PDS_WORKBOOK_LUID` to a workbook that directly references at least one published datasource.
Set `TADX_LIVE_ENVIRONMENT` only when the configured default environment is not the intended fixture.
Set `TADX_LIVE_CONFIG` only when the standard user configuration is not the intended configuration.
Run `go test -tags=live ./internal/tableau/metadata -run TestLiveWorkbookPublishedDatasourceContract -count=1 -v` from the repository root.

### Live Tableau Cloud capture

The live test sent `POST /api/metadata/graphql` with the exact `WorkbookDirectPublishedDatasources` query recorded above.
The request variables were:

```json
{
  "embeddedAfter": null,
  "pageSize": 100,
  "workbookLuid": "sha256:384ff0d145306388"
}
```

Tableau Cloud returned HTTP 200 with this sanitized response:

```json
{
  "data": {
    "workbooksConnection": {
      "nodes": [
        {
          "embeddedDatasourcesConnection": {
            "nodes": [
              {
                "id": "[REDACTED]",
                "name": "[REDACTED]",
                "parentPublishedDatasourcesConnection": {
                  "nodes": [
                    {
                      "luid": "sha256:e7f706ef937aeeb3",
                      "name": "[REDACTED]"
                    }
                  ],
                  "pageInfo": {
                    "endCursor": null,
                    "hasNextPage": false
                  },
                  "totalCount": 1
                }
              }
            ],
            "pageInfo": {
              "endCursor": null,
              "hasNextPage": false
            },
            "totalCount": 1
          },
          "luid": "sha256:384ff0d145306388"
        }
      ],
      "totalCount": 1
    }
  }
}
```

The workbook digest in the response matched the request selector digest.
The direct published datasource digest was `sha256:e7f706ef937aeeb3`.
The live test passed that underlying Metadata LUID to the released REST datasource get endpoint, which returned the same authoritative LUID.
The end-to-end `workbook.pull --include-pds` invocation also downloaded the workbook and its one direct published datasource as sibling native artifacts and recorded `portability: source-site-bound` with `dependencies_acquired: true`.
This one live fixture proves the direct one-parent traversal and REST identity handoff.
Pagination, warning, error, and incomplete-response branches remain covered by local contract tests.
