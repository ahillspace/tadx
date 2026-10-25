# Content fixture bindings

The setup command validates bindings for the selected tests before creating remote fixtures.
`ContentBindingRequirements()` names the exact requirements for all 53 content, project, job, and workspace tests.
`ValidateContentRequirements()` checks paths, authored expectations, and advanced prerequisites.
Missing inputs produce a blocked prerequisite result, rather than an empty-data pass.

Keep target selection in the target profile.
Relative fixture paths resolve against that profile's directory.
Setup generates the core workbook, datasource, overwrite, append, and replace packs from deterministic data.
Cases create isolated projects and content through an independent API client.
Workbook inspect uses an immutable workbook and a same-name workbook in another project, prepared during setup and retained so Metadata API indexing can finish before surgical runs.
Setup resolves their IDs, names, and project IDs into `workbook-inspect-primary` and `workbook-inspect-decoy` bindings.
The inspect case independently reads these fixtures and never modifies or deletes them.
You do not supply existing content IDs for core lifecycle cases.

## Native fixtures

| Binding | Purpose |
| --- | --- |
| `workbook-native`, `datasource-native`, `flow-native` | Baseline native payload and authored expectations. |
| `workbook-native-b`, `datasource-native-b`, `flow-native-b` | Distinguishable replacement payload for overwrite. |
| `datasource-native-hyper`, `datasource-native-hyper-b` | Compatible Hyper row sets A and B. |
| `datasource-append-result` | Exact expected A plus B row multiset, including duplicates. |
| `workbook-native-extract` | Workbook with a real embedded extract that differs from its extract-free download. |
| `workbook-native-dependencies` | Workbook referencing two preprovisioned direct published datasources. |
| `workbook-native-large`, `datasource-native-large` | Valid package larger than 64 MiB after compression, with exact expected extract rows. |
| `datasource-native-composed` | Authored composed datasource with independently known immediate parents. |

Every native binding needs `path` and recognized `expected` facts.
`artifacts` entries alone do not establish the expected payload.
The verifier parses native XML, ZIP, and flow JSON independently of production parsers.
It checks Hyper data through the Tableau Hyper API.
Native comparisons ignore ZIP timestamps and ordering, plus XML formatting.
Publication checks use authored fields and exact rows because Tableau can rewrite package metadata.

For example, a baseline datasource binding has this shape:

```json
{
  "datasource-native": {
    "path": "fixtures/baseline.tdsx",
    "expected": {
      "columns": ["[ID]", "[Region]", "[Sales]"],
      "extract_count": 1,
      "rows": [[1, "East", 10], [2, "West", 25]]
    },
    "params": {
      "hyper_table": "\"Extract\".\"Extract\""
    }
  },
  "datasource-append-result": {
    "expected": {
      "rows": [[1, "East", 10], [2, "West", 25], [3, "North", 47]]
    }
  }
}
```

Workbook expectations can also include `worksheets`.
Flow expectations need `flow`, a structural subset with authored nodes and outputs, or `document`, the complete expected JSON object.
Large-upload expectations must include exact rows that distinguish the end of the payload.
Setup does not lower the production upload threshold.
Overwrite fixtures need distinguishable payloads, rather than different filenames alone.

## Ownership and metadata fixtures

`alternate-owner.id` identifies an existing licensed user for ownership-transfer tests.
The independent fixture account must retain permission to inspect and delete transferred content.
The suite does not create or delete the alternate owner's account.

Schema tests use `workbook-schema` or `datasource-schema`.
These bindings identify a preprovisioned, indexed fixture through `id`.
Their `expected.cli` object contains authored field IDs, formulas, roles, types, and description or tag provenance.
Their `expected.metadata` object contains the independent GraphQL response facts.
`params.metadata_query` contains the independent GraphQL query, with `${id}` replaced by the bound LUID.
Arrays in expected objects compare complete membership and multiplicity, while object properties compare the explicitly authored facts.

The dependency test also requires `workbook-dependencies`:

```json
{
  "workbook-dependencies": {
    "expected": {
      "direct_ids": ["<first-datasource-luid>", "<second-datasource-luid>"],
      "grandparent_id": "<upstream-datasource-luid>",
      "lineage": {
        "nodes": [
          {"metadata_id": "<workbook-metadata-id>", "kind": "workbook", "rest_luid": "<workbook-luid>"},
          {"metadata_id": "<first-datasource-metadata-id>", "kind": "datasource", "rest_luid": "<first-datasource-luid>"},
          {"metadata_id": "<second-datasource-metadata-id>", "kind": "datasource", "rest_luid": "<second-datasource-luid>"}
        ],
        "edges": [
          {"from_metadata_id": "<workbook-metadata-id>", "to_metadata_id": "<first-datasource-metadata-id>", "relationship": "<authored-relationship>"},
          {"from_metadata_id": "<workbook-metadata-id>", "to_metadata_id": "<second-datasource-metadata-id>", "relationship": "<authored-relationship>"}
        ]
      }
    }
  }
}
```

Replace the illustrative lineage identities and relationship labels with the complete authored graph from the fixture definition.
The suite compares acquired direct IDs, native payloads, provenance, and the expected graph.
It confirms that the grandparent does not become a managed artifact.
Composed datasource bindings require `params.metadata_query` and `expected.metadata` describing the immediate parent relationships.
Rebind these target-specific LUIDs when setup moves the advanced fixture pack to another site.

## Fresh job fixtures

`job-inspect`, `job-wait`, and `job-cancel` each describe a fresh independent job submission.
They require `params.submit_method` equal to `POST` and an exact server-relative `params.submit_path` starting with `/api/`.
Optional `params.submit_body` is a JSON string containing the request body.
Keep credentials out of this body.
Use a controlled refresh or flow-run fixture for cancellation, rather than a publication job.

```json
{
  "job-cancel": {
    "params": {
      "submit_method": "POST",
      "submit_path": "/api/<negotiated-version>/sites/<site-luid>/datasources/<fixture-luid>/refresh",
      "submit_body": "{}"
    }
  }
}
```

Setup must replace the placeholders with the selected target's version, site, and fixture identities.
The suite records the returned accepted job ID before observing it.
It does not resubmit an uncertain job or cancel unsupported job types during cleanup.
Cancellation races produce an inconclusive result.
A wait that first observes a terminal job covers receipt replay and reports unavailable active-monitoring coverage.

## Selection boundaries

Core lifecycle cases use generated baseline fixtures, with a distinct owned remote name for each case.
Ephemeral labels such as `base` and `source` do not require additional native files.
Replacement, extract, dependency, large-upload, and composed variants require their distinct declared inputs.
Project create, list, inspect, update, and delete need no content binding.
Project move additionally needs `workbook-native` to prove descendant content survives the move.
Both workspace cases use a real pulled workbook and preserve the remote source.

List qualification checks one bounded result, truthful `more_available`, and the complete exact scoped inventory through `--all`.
The current CLI intentionally hides continuation cursors from rendered output.
Continuation encoding and private cursor behavior remain covered by offline tests.
